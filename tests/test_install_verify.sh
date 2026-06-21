#!/usr/bin/env bash
# Regression test for deploy/install.sh's verify_against_sums + SHA256SUMS fetch
# (board ticket 87dd3eb1). The original installer downloaded the release binary
# without any checksum or signature verification — a supply-chain risk the
# SHA256SUMS artifact already exists to prevent. This test runs the installer's
# verify_against_sums against a controlled scratch dir, with synthetic SHA256SUMS
# bodies, to lock the matching/missing/mismatched cases.
#
# The test does NOT run the full installer (it tries to write /usr/local/bin and
# a systemd unit, which would fail on CI). It sources the verify function out of
# install.sh and exercises it directly, so a regression to the parsing logic
# fails this test before it ships.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INSTALL_SH="$SCRIPT_DIR/deploy/install.sh"

if [ ! -r "$INSTALL_SH" ]; then
    echo "FAIL: install.sh not found at $INSTALL_SH" >&2
    exit 1
fi

# The installer must now contain the verification markers. A refactor that drops
# any of these fails the test before the behavioral cases even run, so the bug
# can't silently reappear in a "looks fine, parses wrong" form.
for marker in 'verify_against_sums' 'SUMS_URL' 'sparkyctrl-linux-' 'verify_against_sums "$TMP"'; do
    if ! grep -q -- "$marker" "$INSTALL_SH"; then
        echo "FAIL: install.sh missing verification marker: $marker" >&2
        exit 1
    fi
done

# Extract the verify_against_sums function and its helpers from install.sh so
# the test exercises the EXACT production parser, not a copy. We pull the
# function block AND the have_command helper it depends on, all via a single
# awk pass. Bash function syntax is regular enough that this works.
VERIFY_DEFS="$(
    awk '
        /^have_command\(\)/ { capture_hc=1 }
        capture_hc { print; if (/^}/) { capture_hc=0; print "\n" } }
        /^verify_against_sums\(\)/ { capture=1 }
        capture { print; if (/^}/) exit }
    ' "$INSTALL_SH"
)"
if [ -z "$VERIFY_DEFS" ]; then
    echo "FAIL: could not extract verify_against_sums (+ have_command) from install.sh" >&2
    exit 1
fi

# Source the helpers in a clean shell.
eval "$VERIFY_DEFS"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

# --- Test case 1: matching checksums (the happy path) ---
binary="$scratch/sparkyctrl-linux-amd64"
printf 'hello world' > "$binary"
got="$(sha256sum -- "$binary" | awk '{print $1}')"
sums="${got}  sparkyctrl-linux-amd64
${got}  sparkyctrl-linux-arm64
"
if ! verify_against_sums "$binary" "sparkyctrl-linux-amd64" "$sums"; then
    echo "FAIL: matching checksum was rejected" >&2
    exit 1
fi

# --- Test case 2: matching checksum with the binary-mode `*` marker
# (`sha256sum --binary` and BSD `sha256sum -b` write "*filename") ---
if ! verify_against_sums "$binary" "sparkyctrl-linux-amd64" "${got} *sparkyctrl-linux-amd64"; then
    echo "FAIL: matching checksum with binary-mode marker was rejected" >&2
    exit 1
fi

# --- Test case 3: wrong asset name in sums file (no entry) ---
if verify_against_sums "$binary" "sparkyctrl-linux-amd64" \
        "${got}  sparkyctrl-linux-something-else"; then
    echo "FAIL: missing asset entry was accepted" >&2
    exit 1
fi

# --- Test case 4: asset name present but hash doesn't match ---
if verify_against_sums "$binary" "sparkyctrl-linux-amd64" \
        "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  sparkyctrl-linux-amd64"; then
    echo "FAIL: mismatched checksum was accepted" >&2
    exit 1
fi

# --- Test case 5: empty sums file (no asset) ---
if verify_against_sums "$binary" "sparkyctrl-linux-amd64" ""; then
    echo "FAIL: empty sums file was accepted" >&2
    exit 1
fi

echo "PASS: verify_against_sums accepts matching checksums (plain + binary-mode marker)"
echo "PASS: verify_against_sums rejects missing/mismatched/empty checksums"
echo "PASS: install.sh now downloads + verifies SHA256SUMS for the release binary"
