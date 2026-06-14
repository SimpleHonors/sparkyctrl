#!/usr/bin/env bash
# Cross-compile sparkyctrl for all target platforms into ./dist.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p dist
build() {
  local goos=$1 goarch=$2 ext=${3:-}
  echo "building ${goos}/${goarch}"
  GOOS=$goos GOARCH=$goarch CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w" \
    -o "dist/sparkyctrl-${goos}-${goarch}${ext}" .
}
build linux amd64
build linux arm64
build windows amd64 .exe
echo "done:"; ls -la dist
