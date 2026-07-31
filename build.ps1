# Cross-compile promptforge for every supported OS/arch into dist/.
# Usage:  ./build.ps1        (requires Go on PATH, or pass -Go <path-to-go.exe>)
param([string]$Go = "go")

$ErrorActionPreference = "Stop"
$targets = @(
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe" },
    @{ GOOS = "darwin";  GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "darwin";  GOARCH = "arm64"; Ext = "" },
    @{ GOOS = "linux";   GOARCH = "amd64"; Ext = "" },
    @{ GOOS = "linux";   GOARCH = "arm64"; Ext = "" }
)

New-Item -ItemType Directory -Force -Path "dist" | Out-Null
foreach ($t in $targets) {
    $out = "dist/promptforge-$($t.GOOS)-$($t.GOARCH)$($t.Ext)"
    Write-Host "building $out"
    $env:GOOS = $t.GOOS
    $env:GOARCH = $t.GOARCH
    $env:CGO_ENABLED = "0"
    & $Go build -trimpath -ldflags "-s -w" -o $out .
    if ($LASTEXITCODE -ne 0) { throw "build failed for $($t.GOOS)/$($t.GOARCH)" }
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
Write-Host "done -> dist/"
Get-ChildItem dist | Select-Object Name, @{N="MB";E={[math]::Round($_.Length/1MB,2)}}
