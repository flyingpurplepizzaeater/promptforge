#!/usr/bin/env bash
# Cross-compile promptforge for every supported OS/arch into dist/.
# Usage:  ./build.sh          (requires Go on PATH, or set GO=/path/to/go)
set -euo pipefail
GO="${GO:-go}"

mkdir -p dist

build() {
  local goos="$1" goarch="$2" ext="${3:-}"
  local out="dist/promptforge-${goos}-${goarch}${ext}"
  echo "building $out"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO" build -trimpath -ldflags "-s -w" -o "$out" .
}

build windows amd64 .exe
build windows arm64 .exe
build darwin  amd64
build darwin  arm64
build linux   amd64
build linux   arm64

echo "done -> dist/"
ls -lh dist
