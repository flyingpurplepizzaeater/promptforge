// promptforge — rough ask in, structured prompt out.
//
// A small, offline, dependency-free CLI. Point it at a folder, give it a rough
// development ask, and it prints a paste-ready prompt that turns that ask into a
// rigorous spec — Goal / Context / Scope / Done-when / Verification — for any
// AI/LLM (ChatGPT, Claude, Gemini, a local model, whatever you use).
//
// It does the deterministic parts a machine is good at (scan the repo, structure
// the request, guess the shape) and leaves the reasoning to whichever model you
// paste the output into. No API keys, no network, one static binary.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const version = "1.0.0"

// kv is one detected context fact, rendered as a "- key: value" bullet.
type kv struct{ key, val string }

func main() {
	dir := flag.String("dir", ".", "directory to scan for project context")
	flag.StringVar(dir, "d", ".", "shorthand for --dir")
	tmpl := flag.Bool("template", false, "emit a blank fill-in template instead of an LLM meta-prompt")
	flag.BoolVar(tmpl, "t", false, "shorthand for --template")
	outPath := flag.String("out", "", "write output to this file instead of stdout")
	flag.StringVar(outPath, "o", "", "shorthand for --out")
	doCopy := flag.Bool("copy", false, "copy output to the system clipboard")
	flag.BoolVar(doCopy, "c", false, "shorthand for --copy")
	noCtx := flag.Bool("no-context", false, "skip repo scanning; just structure the ask")
	showVer := flag.Bool("version", false, "print version and exit")
	flag.Usage = usage
	flag.Parse()

	if *showVer {
		fmt.Println("promptforge " + version)
		return
	}

	ask := readAsk(flag.Args())
	if ask == "" {
		fmt.Fprintln(os.Stderr, `promptforge: no ask given. Try: promptforge "add a dark-mode toggle"`)
		fmt.Fprintln(os.Stderr, "Run 'promptforge --help' for usage.")
		os.Exit(2)
	}

	absDir, err := filepath.Abs(*dir)
	if err != nil {
		absDir = *dir
	}

	var ctx []kv
	if !*noCtx {
		ctx = detectContext(absDir)
	}

	shape, shapeWhy := guessShape(ask, ctx)
	verif := guessVerification(ask)

	var out string
	if *tmpl {
		out = buildTemplate(ask, ctx, shape, verif)
	} else {
		out = buildMetaPrompt(ask, ctx, shape, shapeWhy, verif)
	}

	if *outPath != "" {
		if err := os.WriteFile(*outPath, []byte(out), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "promptforge: cannot write file:", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "promptforge: wrote "+*outPath)
	} else {
		fmt.Println(out)
	}

	if *doCopy {
		if err := copyToClipboard(out); err != nil {
			fmt.Fprintln(os.Stderr, "promptforge: clipboard unavailable ("+err.Error()+")")
		} else {
			fmt.Fprintln(os.Stderr, "promptforge: copied to clipboard")
		}
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `promptforge `+version+` — rough ask in, structured prompt out.

Scans the folder you run it in and turns a rough development ask into a rigorous,
paste-ready prompt (Goal / Context / Scope / Done-when / Verification) for any AI/LLM.
Offline. No API keys.

USAGE
  promptforge [flags] "your rough ask"
  echo "your rough ask" | promptforge [flags]

FLAGS
  -d, --dir PATH    directory to scan (default ".")
  -t, --template    emit a blank fill-in template instead of an LLM meta-prompt
  -o, --out FILE    write output to FILE instead of stdout
  -c, --copy        copy output to the system clipboard
      --no-context  skip repo scanning; just structure the ask
      --version     print version
  -h, --help        this help

  Put flags before the ask: promptforge -c "fix the login loop"

EXAMPLES
  promptforge "add a dark-mode toggle to the settings page"
  promptforge -d ../myapp -c "fix the login redirect loop"
  promptforge --template "migrate the API from REST to GraphQL" > spec.md

Paste the output into ChatGPT, Claude, Gemini, or a local model. It replies with the
finished spec — asking you a batched round of questions first only if it needs to.
`)
}

// readAsk resolves the rough ask from (in priority order) CLI args, piped stdin,
// or an interactive prompt.
func readAsk(args []string) string {
	if len(args) > 0 {
		return strings.TrimSpace(strings.Join(args, " "))
	}
	if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		b, _ := io.ReadAll(os.Stdin)
		return strings.TrimSpace(string(b))
	}
	fmt.Fprint(os.Stderr, "Rough ask> ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	return strings.TrimSpace(line)
}

// detectContext gathers a best-effort picture of the project in dir. Every probe
// degrades to empty rather than failing, so an unreadable or exotic repo just
// yields fewer bullets — never a crash.
func detectContext(dir string) []kv {
	var ctx []kv
	add := func(k, v string) {
		if strings.TrimSpace(v) != "" {
			ctx = append(ctx, kv{k, v})
		}
	}

	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		add("Path", dir+" (not a readable directory — context skipped)")
		return ctx
	}
	add("Path", dir)
	add("Git", gitInfo(dir))
	add("Stack", strings.Join(detectStacks(dir), ", "))
	add("Frameworks", strings.Join(detectFrameworks(dir), ", "))
	add("README", readmeSummary(dir))
	add("AI config present", strings.Join(detectAIConfig(dir), ", "))
	add("Also present", strings.Join(detectSignals(dir), ", "))
	add("Top level", strings.Join(topLevel(dir), "  "))
	return ctx
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// gitInfo reports branch and dirty state. It prefers the git binary for accuracy
// but falls back to reading .git/HEAD so it still works where git isn't on PATH.
func gitInfo(dir string) string {
	if !exists(filepath.Join(dir, ".git")) {
		return ""
	}
	if _, err := exec.LookPath("git"); err == nil {
		branch := strings.TrimSpace(runGit(dir, "rev-parse", "--abbrev-ref", "HEAD"))
		status := runGit(dir, "status", "--porcelain")
		dirty := "clean"
		if s := strings.TrimSpace(status); s != "" {
			dirty = fmt.Sprintf("%d uncommitted file(s)", len(strings.Split(s, "\n")))
		}
		if branch == "" {
			branch = "(unknown branch)"
		}
		return branch + ", " + dirty
	}
	if b, err := os.ReadFile(filepath.Join(dir, ".git", "HEAD")); err == nil {
		line := strings.TrimSpace(string(b))
		if strings.HasPrefix(line, "ref: refs/heads/") {
			return strings.TrimPrefix(line, "ref: refs/heads/") + " (git status unavailable)"
		}
	}
	return "git repo (details unavailable)"
}

func runGit(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// detectStacks returns the languages/build-systems implied by marker files, in a
// stable order and de-duplicated (Python, for instance, has several markers).
func detectStacks(dir string) []string {
	markers := []struct {
		marker string
		stack  string
		glob   bool
	}{
		{"go.mod", "Go", false},
		{"package.json", "Node/JS", false},
		{"pyproject.toml", "Python", false},
		{"requirements.txt", "Python", false},
		{"setup.py", "Python", false},
		{"Pipfile", "Python", false},
		{"Cargo.toml", "Rust", false},
		{"pom.xml", "Java (Maven)", false},
		{"build.gradle", "JVM (Gradle)", false},
		{"build.gradle.kts", "JVM (Gradle)", false},
		{"Gemfile", "Ruby", false},
		{"composer.json", "PHP", false},
		{"CMakeLists.txt", "C/C++ (CMake)", false},
		{"pubspec.yaml", "Dart/Flutter", false},
		{"mix.exs", "Elixir", false},
		{"*.csproj", ".NET/C#", true},
		{"*.sln", ".NET/C#", true},
		{"*.swift", "Swift", true},
	}
	var out []string
	seen := map[string]bool{}
	for _, mk := range markers {
		hit := false
		if mk.glob {
			m, _ := filepath.Glob(filepath.Join(dir, mk.marker))
			hit = len(m) > 0
		} else {
			hit = exists(filepath.Join(dir, mk.marker))
		}
		if hit && !seen[mk.stack] {
			seen[mk.stack] = true
			out = append(out, mk.stack)
		}
	}
	return out
}

func detectFrameworks(dir string) []string {
	out := nodeFrameworks(dir)
	return append(out, pyFrameworks(dir)...)
}

// nodeFrameworks reads package.json dependencies and names the notable frameworks.
func nodeFrameworks(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(b, &pkg) != nil {
		return nil
	}
	all := map[string]bool{}
	for k := range pkg.Dependencies {
		all[k] = true
	}
	for k := range pkg.DevDependencies {
		all[k] = true
	}
	known := []struct{ dep, name string }{
		{"next", "Next.js"}, {"react", "React"}, {"vue", "Vue"},
		{"svelte", "Svelte"}, {"@angular/core", "Angular"},
		{"@nestjs/core", "NestJS"}, {"express", "Express"}, {"fastify", "Fastify"},
		{"electron", "Electron"}, {"vite", "Vite"}, {"typescript", "TypeScript"},
	}
	var out []string
	for _, k := range known {
		if all[k.dep] {
			out = append(out, k.name)
		}
	}
	return out
}

// pyFrameworks scans the Python dependency files as text (no TOML dependency) for
// well-known framework and library names.
func pyFrameworks(dir string) []string {
	var text string
	for _, f := range []string{"requirements.txt", "pyproject.toml", "Pipfile", "setup.py"} {
		if b, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			text += strings.ToLower(string(b)) + "\n"
		}
	}
	if text == "" {
		return nil
	}
	known := []struct{ needle, name string }{
		{"django", "Django"}, {"flask", "Flask"}, {"fastapi", "FastAPI"},
		{"streamlit", "Streamlit"}, {"pytorch", "PyTorch"}, {"tensorflow", "TensorFlow"},
		{"pandas", "pandas"}, {"numpy", "NumPy"}, {"pytest", "pytest"},
	}
	seen := map[string]bool{}
	var out []string
	for _, k := range known {
		if strings.Contains(text, k.needle) && !seen[k.name] {
			seen[k.name] = true
			out = append(out, k.name)
		}
	}
	return out
}

func readmeSummary(dir string) string {
	for _, name := range []string{"README.md", "readme.md", "README.rst", "README.txt", "README"} {
		if b, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if s := firstMeaningfulLine(string(b)); s != "" {
				return s
			}
		}
	}
	return ""
}

// firstMeaningfulLine returns the first real line of a README, skipping blanks,
// badge rows, and HTML comments, and stripping leading markdown heading markers.
func firstMeaningfulLine(text string) string {
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "<!--") ||
			strings.HasPrefix(line, "[![") || strings.HasPrefix(line, "![") {
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#> "))
		if line == "" {
			continue
		}
		if len(line) > 120 {
			line = line[:117] + "..."
		}
		return line
	}
	return ""
}

// detectAIConfig notes which AI coding tools this repo is already set up for — a
// hint to the receiving model about existing house style.
func detectAIConfig(dir string) []string {
	markers := []struct{ path, tool string }{
		{"CLAUDE.md", "Claude Code (CLAUDE.md)"},
		{".cursorrules", "Cursor"},
		{".cursor", "Cursor"},
		{".github/copilot-instructions.md", "GitHub Copilot"},
		{"AGENTS.md", "AGENTS.md"},
		{".windsurfrules", "Windsurf"},
		{"GEMINI.md", "Gemini CLI"},
		{".aider.conf.yml", "Aider"},
	}
	seen := map[string]bool{}
	var out []string
	for _, mk := range markers {
		if exists(filepath.Join(dir, mk.path)) && !seen[mk.tool] {
			seen[mk.tool] = true
			out = append(out, mk.tool)
		}
	}
	return out
}

func detectSignals(dir string) []string {
	markers := []struct{ path, label string }{
		{"Dockerfile", "Docker"},
		{"docker-compose.yml", "Docker Compose"},
		{"compose.yaml", "Docker Compose"},
		{"Makefile", "Makefile"},
		{".github/workflows", "GitHub Actions CI"},
		{".gitlab-ci.yml", "GitLab CI"},
		{".editorconfig", "EditorConfig"},
	}
	seen := map[string]bool{}
	var out []string
	for _, mk := range markers {
		if exists(filepath.Join(dir, mk.path)) && !seen[mk.label] {
			seen[mk.label] = true
			out = append(out, mk.label)
		}
	}
	return out
}

// topLevel lists the meaningful top-level entries, skipping build/tooling noise,
// so the model gets a sense of layout without a full tree.
func topLevel(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	skip := map[string]bool{
		"node_modules": true, ".venv": true, "venv": true, "__pycache__": true,
		"dist": true, "build": true, "target": true, ".next": true,
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if skip[name] || strings.HasPrefix(name, ".") {
			continue
		}
		if e.IsDir() {
			name += "/"
		}
		out = append(out, name)
		if len(out) >= 14 {
			out = append(out, "...")
			break
		}
	}
	return out
}

// guessShape makes a cheap keyword-based guess at the kind of work, and a one-line
// reason. It is a hint for the receiving model to confirm, never a decision.
func guessShape(ask string, ctx []kv) (string, string) {
	a := strings.ToLower(ask)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(a, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("should we", "should i", "what if", "explore", "consider", "evaluate",
		"compare", "worth it", "pros and cons", "which approach", "decide whether"):
		return "Exploration", "the ask contains an undecided question, not a decided change — settle the decision before building"
	case has("fix", "bug", "broken", "crash", "error", "fails", "failing",
		"regression", "doesn't work", "not working", "unexpected"):
		return "Bug", "the ask names a symptom of existing wrong behavior — reproduce first, then fix"
	case has("refactor", "clean up", "cleanup", "simplify", "rename",
		"restructure", "deduplicate", "tidy", "extract"):
		return "Refactor", "the ask changes structure without changing behavior — keep behavior identical, lean on tests"
	case has("document", "docs", "readme", "changelog", "write-up", "write up"):
		return "Docs", "the ask is a documentation deliverable, not a code-behavior change"
	case has("unit test", "e2e", "integration test", "coverage", "add tests", "write tests"):
		return "Tests", "the ask is about test coverage — define exactly what behavior must be pinned down"
	case (len(ctx) > 0 && emptyRepo(ctx)) || has("new project", "from scratch",
		"greenfield", "scaffold", "bootstrap", "set up a new"):
		return "New project", "little or no existing code detected — greenfield setup; decide stack and structure up front"
	default:
		return "Feature / change", "the ask adds or alters behavior in existing code"
	}
}

// emptyRepo is true when a scan found neither a stack nor any top-level content.
func emptyRepo(ctx []kv) bool {
	for _, c := range ctx {
		if c.key == "Stack" || c.key == "Top level" {
			return false
		}
	}
	return true
}

func guessVerification(ask string) string {
	a := strings.ToLower(ask)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(a, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("ui", "page", "screen", "button", "frontend", "css", "style", "layout",
		"form", "modal", "dashboard", "user-facing", "endpoint", "route", "cli",
		"command", "flow", "dark mode", "dark-mode"):
		return "Heavy"
	case has("rename", "typo", "comment", "bump", "constant", "config value",
		"log message", "whitespace", "reformat"):
		return "Light"
	default:
		return "Standard"
	}
}

const marker = "----------------------------------------------------------------"

// buildMetaPrompt produces the default output: a self-contained block the user
// pastes into any LLM, which then does the actual shaping reasoning.
func buildMetaPrompt(ask string, ctx []kv, shape, shapeWhy, verif string) string {
	var b strings.Builder
	b.WriteString(marker + "\n")
	b.WriteString("  COPY EVERYTHING BELOW into any AI/LLM (ChatGPT, Claude, Gemini, local).\n")
	b.WriteString(marker + "\n\n")

	b.WriteString(`You are a senior engineer. Turn my rough, underspecified request into a rigorous,
unambiguous prompt I can hand to a coding agent. Do NOT start building yet.

If — and only if — some ambiguity would materially change the work, ask me ONE
batched set of questions first (max 4, each with a recommended default). Otherwise
go straight to the spec.

## My rough ask
` + ask + "\n")

	if len(ctx) > 0 {
		b.WriteString("\n## Auto-detected context (from the folder I ran this in)\n")
		for _, c := range ctx {
			b.WriteString("- " + c.key + ": " + c.val + "\n")
		}
	}

	b.WriteString("\n## Heuristic hints (verify — don't trust these blindly)\n")
	b.WriteString("- Likely shape: " + shape + " — " + shapeWhy + ". Correct me if wrong.\n")
	b.WriteString("- Suggested verification tier: " + verif + ".\n")

	b.WriteString(`
## Produce exactly this structure

**Goal** — one concrete sentence, imperative: what is true when this is done.

**Context** — the 2-3 files or subsystems this touches, and any decisions already
made that constrain it. Use the detected context above; ask me if a key file is unclear.

**Scope**
- In: what this change covers.
- Out: the adjacent thing it deliberately does NOT cover. Name at least one — this
  single line is what stops scope creep.

**Done when** — observable, checkable criteria. Not "works correctly": prefer things
someone else could confirm by running a command or looking at a result.

**Verification** — choose one and name the specific commands or exercise:
- Light    — re-read the diff, confirm it does what was asked.
- Standard — run tests / build / typecheck / lint; fix what breaks.
- Heavy    — Standard, plus actually exercise it (run it, click it, hit the endpoint).

A rough ask becomes a good prompt exactly when the Out-of-scope line and the
Done-when criteria are both specific. Make them specific.
`)
	return b.String()
}

// buildTemplate produces a blank, pre-filled skeleton for users who want to write
// the spec by hand instead of routing it through a model.
func buildTemplate(ask string, ctx []kv, shape, verif string) string {
	var b strings.Builder
	b.WriteString("# Prompt spec\n\n")
	b.WriteString("Rough ask: " + ask + "\n\n")
	if len(ctx) > 0 {
		b.WriteString("## Context (auto-detected — edit freely)\n")
		for _, c := range ctx {
			b.WriteString("- " + c.key + ": " + c.val + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Goal\n(one concrete sentence, imperative)\n\n")
	b.WriteString("## Scope\n- In: \n- Out: \n\n")
	b.WriteString("## Done when\n- \n- \n\n")
	b.WriteString("## Verification\n" + verif + " — (name the commands or exercise)\n\n")
	b.WriteString("<!-- likely shape: " + shape + " -->\n")
	return b.String()
}

// copyToClipboard pipes text to the platform clipboard tool, degrading with a
// clear error when none is available (common on headless Linux).
func copyToClipboard(s string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "clip")
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		switch {
		case lookable("wl-copy"):
			cmd = exec.Command("wl-copy")
		case lookable("xclip"):
			cmd = exec.Command("xclip", "-selection", "clipboard")
		case lookable("xsel"):
			cmd = exec.Command("xsel", "--clipboard", "--input")
		default:
			return fmt.Errorf("no clipboard tool found; install xclip, xsel, or wl-clipboard")
		}
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := io.WriteString(in, s); err != nil {
		return err
	}
	in.Close()
	return cmd.Wait()
}

func lookable(bin string) bool { _, err := exec.LookPath(bin); return err == nil }
