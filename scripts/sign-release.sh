#!/usr/bin/env bash
# Sign a sparkyctrl release dist directory.
#
# Usage:
#   scripts/sign-release.sh <dist-dir>
#
# Reads the private key from $SPARKYCTRL_SIGNING_KEY (path to the .key file)
# or from $SPARKYCTRL_SIGNING_KEY_PASSPHRASE (interactive prompt if neither
# is set). The private key is NEVER committed to the repo — store it in your
# CI secret store (GitHub Actions encrypted secret, 1Password CLI, etc.) and
# pass the path here at release time.
#
# The script signs:
#   - Each sparkyctrl-<goos>-<goarch>[.exe] binary
#   - SHA256SUMS (the same file publish-checksums.sh just generated)
# And writes the .minisig files next to each. Upload ALL of them alongside
# the binary + SHA256SUMS to the GitHub release.
#
# Pair with scripts/publish-checksums.sh: typically you run publish-checksums
# first (to write SHA256SUMS), then sign-release (to sign SHA256SUMS + each
# binary), then `gh release upload <tag> <dist-dir>/*`.
#
# Dependencies: minisign (apt: minisign / brew: minisign / dnf: minisign).
set -euo pipefail

dir="${1:?usage: sign-release.sh <dist-dir>}"
[ -d "$dir" ] || { echo "sign-release.sh: not a directory: $dir" >&2; exit 1; }

if ! command -v minisign >/dev/null 2>&1; then
  echo "sign-release.sh: minisign is required (apt-get install minisign / brew install minisign)" >&2
  exit 1
fi

# Resolve the private key path. Order of precedence:
#   1. $SPARKYCTRL_SIGNING_KEY env var (path to .key file)
#   2. ./sparkyctrl-release.key in the current dir (operator-local convenience)
#   3. interactive prompt
if [ -n "${SPARKYCTRL_SIGNING_KEY:-}" ]; then
  key="$SPARKYCTRL_SIGNING_KEY"
elif [ -r "./sparkyctrl-release.key" ]; then
  key="./sparkyctrl-release.key"
else
  printf 'path to minisign private key (.key file): '
  read -r key
fi
[ -r "$key" ] || { echo "sign-release.sh: cannot read private key at $key" >&2; exit 1; }

# Passphrase handling. We don't want to leak the passphrase via `ps`, so we
# only pass it via env to minisign if the env var is set; otherwise we let
# minisign prompt (which is what an operator doing a manual release would
# expect).
sign_one() {
  local f="$1"
  local args=(-S -s "$key" -m "$f")
  if [ -n "${SPARKYCTRL_SIGNING_KEY_PASSPHRASE:-}" ]; then
    args+=(-W)
  fi
  echo "signing: $f"
  if [ -n "${SPARKYCTRL_SIGNING_KEY_PASSPHRASE:-}" ]; then
    printf '%s\n' "$SPARKYCTRL_SIGNING_KEY_PASSPHRASE" | minisign "${args[@]}" >/dev/null
  else
    minisign "${args[@]}" >/dev/null
  fi
}

cd "$dir"

# Sign every binary. We deliberately do NOT skip the .minisig files in this
# loop (there shouldn't be any yet — signing happens before this script
# uploads) but the name filter is `sparkyctrl-*` to match the published asset
# naming.
signed=0
for f in sparkyctrl-*; do
  case "$f" in
    *.minisig) continue ;;  # never sign a signature
  esac
  sign_one "$f"
  signed=$((signed + 1))
done

# Sign SHA256SUMS last so an attacker who can replace SHA256SUMS still can't
# replace the signature on it (the .minisig is what binds the sums to the key).
if [ -r SHA256SUMS ]; then
  sign_one SHA256SUMS
  signed=$((signed + 1))
fi

if [ "$signed" -eq 0 ]; then
  echo "sign-release.sh: no sparkyctrl-* files or SHA256SUMS found in $dir" >&2
  exit 1
fi

echo "signed $signed files. Upload these to the GitHub release:"
ls -1 "$dir" | grep -E '(sparkyctrl-|SHA256SUMS)'
