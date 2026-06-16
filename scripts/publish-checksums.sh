#!/usr/bin/env bash
# Generate SHA256SUMS for the release binaries in a dist dir.
# Usage: scripts/publish-checksums.sh <dist-dir>
#   then: gh release upload <tag> <dist-dir>/SHA256SUMS
set -euo pipefail

dir="${1:?usage: publish-checksums.sh <dist-dir>}"
cd "$dir"
: > SHA256SUMS
for f in sparkyctrl-*; do
  [ "$f" = "SHA256SUMS" ] && continue
  sha256sum "$f" >> SHA256SUMS
done
echo "wrote $dir/SHA256SUMS:"
cat SHA256SUMS
