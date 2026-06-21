#!/usr/bin/env bash
# Regression test for deploy/install.sh's verify_against_signature + minisign
# integration (board ticket af4487db — signature-verification follow-up to
# 87dd3eb1). The original installer (and the 87dd3eb1 fix) verified the
# SHA256SUMS hash, but not a cryptographic signature. A compromised release
# or tampered SHA256SUMS would still pass the hash check, so a minisign
# verify was added on top. This test runs the installer's
# verify_against_signature against a controlled scratch dir, with synthetic
# signed files, to lock the matching/missing/mismatched cases.
#
# Same approach as test_install_verify.sh: source the function out of
# install.sh and exercise it directly, so a regression to the parser/CLI
# invocation fails this test before it ships.
#
# Skip when minisign is not installed (apt: minisign).

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
INSTALL_SH="$SCRIPT_DIR/deploy/install.sh"

if [ ! -r "$INSTALL_SH" ]; then
    echo "FAIL: install.sh not found at $INSTALL_SH" >&2
    exit 1
fi

if ! command -v minisign >/dev/null 2>&1; then
    echo "SKIP: minisign not installed; cannot exercise verify_against_signature"
    exit 0
fi

# The installer must now contain the sig-verify markers. A refactor that drops
# any of these fails the test before the behavioral cases even run, so the
# bug can't silently reappear in a "looks fine, calls wrong binary" form.
for marker in 'verify_against_signature' 'SIG_URL' 'SUMS_SIG_URL' 'sparkyctrl-release.pub' 'minisign -Vm "$file"'; do
    if ! grep -q -- "$marker" "$INSTALL_SH"; then
        echo "FAIL: install.sh missing signature-verify marker: $marker" >&2
        exit 1
    fi
done

# Extract the verify_against_signature function and its helpers from
# install.sh. We pull the function block AND the have_command helper it
# depends on, all via a single awk pass.
VERIFY_DEFS="$(
    awk '
        /^have_command\(\)/ { capture_hc=1 }
        capture_hc { print; if (/^}/) { capture_hc=0; print "\n" } }
        /^verify_against_signature\(\)/ { capture=1 }
        capture { print; if (/^}/) exit }
    ' "$INSTALL_SH"
)"
if [ -z "$VERIFY_DEFS" ]; then
    echo "FAIL: could not extract verify_against_signature (+ have_command) from install.sh" >&2
    exit 1
fi

eval "$VERIFY_DEFS"

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

# Generate a keypair in scratch so the test is self-contained. An empty
# passphrase (stdin newline) is fine for a throwaway key.
minisign -G -p "$scratch/test.pub" -s "$scratch/test.key" -W >/dev/null <<< ""

# --- Test case 1: valid signature is accepted (happy path) ---
payload="$scratch/payload.bin"
printf 'hello sparkyctrl sig verify\n' > "$payload"
minisign -S -s "$scratch/test.key" -m "$payload" -W >/dev/null <<< ""
if ! verify_against_signature "$payload" "$payload.minisig" "$scratch/test.pub"; then
    echo "FAIL: valid signature was rejected" >&2
    exit 1
fi

# --- Test case 2: tampered file (signature now invalid) ---
tampered="$scratch/tampered.bin"
printf 'TAMPERED sparkyctrl payload\n' > "$tampered"
# Reuse the .minisig from the original (now wrong).
if verify_against_signature "$tampered" "$payload.minisig" "$scratch/test.pub"; then
    echo "FAIL: tampered file with original signature was accepted" >&2
    exit 1
fi

# --- Test case 3: wrong keypair (sig from key A, verify with key B) ---
wrong_pub="$scratch/wrong.pub"
wrong_key="$scratch/wrong.key"
minisign -G -p "$wrong_pub" -s "$wrong_key" -W >/dev/null <<< ""
other="$scratch/other.bin"
printf 'another payload' > "$other"
minisign -S -s "$scratch/test.key" -m "$other" -W >/dev/null <<< ""  # signed with the RIGHT key
if verify_against_signature "$other" "$other.minisig" "$wrong_pub"; then
    echo "FAIL: signature from a different key was accepted by the wrong pubkey" >&2
    exit 1
fi
# Sanity: the right pubkey still accepts it.
if ! verify_against_signature "$other" "$other.minisig" "$scratch/test.pub"; then
    echo "FAIL: signature rejected by the correct pubkey (false negative)" >&2
    exit 1
fi

# --- Test case 4: missing signature file ---
if verify_against_signature "$payload" "$scratch/nonexistent.minisig" "$scratch/test.pub"; then
    echo "FAIL: missing signature file was accepted" >&2
    exit 1
fi

# --- Test case 5: missing public key ---
if verify_against_signature "$payload" "$payload.minisig" "$scratch/nonexistent.pub"; then
    echo "FAIL: missing pubkey file was accepted" >&2
    exit 1
fi

echo "PASS: verify_against_signature accepts valid signatures"
echo "PASS: verify_against_signature rejects tampered files + wrong keys"
echo "PASS: install.sh now downloads + verifies minisign signatures for the release binary + SHA256SUMS"
