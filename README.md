# promptforge

[![release](https://github.com/flyingpurplepizzaeater/promptforge/actions/workflows/release.yml/badge.svg)](https://github.com/flyingpurplepizzaeater/promptforge/actions/workflows/release.yml)

**Rough ask in, structured prompt out.** A tiny, offline CLI that turns a vague
development request into a rigorous, paste-ready prompt for **any** AI/LLM —
ChatGPT, Claude, Gemini, or a local model. No API keys, no network, one small binary.

It does the parts a machine is good at — scan the repo you're standing in, structure
the request, guess the kind of work — and leaves the reasoning to whichever model you
paste the output into.

## Why

A vague ask ("add dark mode", "fix the login thing") makes an AI guess at scope,
context, and what "done" means — and it usually guesses wrong. promptforge front-loads
the boring structure so the model spends its effort on the actual problem. Every prompt
comes out shaped as:

- **Goal** — one concrete sentence
- **Context** — what it touches (auto-detected from your folder)
- **Scope** — explicitly in *and* out (the "out" line is what stops scope creep)
- **Done when** — observable, checkable criteria
- **Verification** — Light / Standard / Heavy, with the actual commands

## Install

Download the binary for your OS from `dist/` (or build it — see below), then run it.
There's nothing to install and no runtime to set up.

| OS | File |
|----|------|
| Windows | `promptforge-windows-amd64.exe` |
| macOS (Apple Silicon) | `promptforge-darwin-arm64` |
| macOS (Intel) | `promptforge-darwin-amd64` |
| Linux | `promptforge-linux-amd64` |

On macOS/Linux, make it executable once: `chmod +x promptforge-*` (and optionally
rename it to `promptforge` and drop it on your `PATH`).

## Use

```
promptforge "add a dark-mode toggle to the settings page"
```

Then paste the output into any AI/LLM. It replies with the finished spec — and asks
you one batched round of questions first only if the ask is genuinely ambiguous.

```
promptforge [flags] "your rough ask"
echo "your rough ask" | promptforge

  -d, --dir PATH    directory to scan (default ".")
  -t, --template    emit a blank fill-in template instead of an LLM meta-prompt
  -o, --out FILE    write output to FILE instead of stdout
  -c, --copy        copy output to the system clipboard
      --no-context  skip repo scanning; just structure the ask
      --version     print version
  -h, --help        full help
```

Examples:

```
promptforge -d ../myapp -c "fix the login redirect loop"
promptforge --template "migrate the API from REST to GraphQL" > spec.md
```

## What it detects

Running in a project folder, promptforge picks up: git branch + dirty state, language/
build stack (Go, Node, Python, Rust, JVM, .NET, Ruby, PHP, C/C++, Dart, Elixir, Swift),
notable frameworks (from `package.json` deps and Python requirements), a one-line README
summary, which AI tools the repo is already configured for (CLAUDE.md, Cursor, Copilot,
…), Docker/CI/Makefile signals, and the top-level layout. All best-effort — anything it
can't read just gets omitted.

## Build from source

Requires [Go](https://go.dev/dl/) 1.21+.

```
go build -o promptforge .
```

Cross-compile every release binary into `dist/`:

```
# Windows PowerShell
./build.ps1

# macOS / Linux
./build.sh
```

## Releases

Releases are built by CI. Push a version tag and GitHub Actions cross-compiles all
six binaries (plus `SHA256SUMS.txt`) and publishes them to a GitHub Release:

```
git tag v1.1.0
git push origin v1.1.0
```

To test the build without publishing, trigger it manually from the **Actions → release
→ Run workflow** button — it uploads the binaries as run artifacts instead.

## Design notes

- **Single file, standard library only.** No third-party dependencies — the binary is
  a few MB, cross-compiles anywhere, and has no supply-chain surface.
- **Offline by design.** promptforge never calls a model itself; it produces text you
  route to whatever LLM you already use. That's what makes it work with *any* AI.
- Derived from a personal Claude Code skill (`/p`), generalized so it's useful to
  anyone, in any repo, with any assistant.

## License

MIT — see `LICENSE`.
