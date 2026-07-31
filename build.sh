#!/usr/bin/env bash
# Cross-compile promptforge for every supported OS/arch into dist/.
# Usage:  ./build.sh          (requires Go on PATH, or set GO=/path/to/go)
set -euo pipefail
GO="${GO:-go}"

targets=(
  "windows amd64 .exe"
  "windows arm64 .exe"
  "darwin  amd64 ''"
  "darwin  arm64 ''"
  "linux   amd64 ''"
  "linux   arm64 ''"
)

mkdir -p dist
for t in "${targets[@]}"; do
  read -r goos goarch ext <<<"$t"
  [ "$ext" = "''" ] && ext=""
  out="dist/promptforge-${goos}-${goarch}${ext}"
  echo "building $out"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    "$GO" build -trimpath -ldflags "-s -w" -o "$out" .
done
echo "done -> dist/"
ls -lh dist
