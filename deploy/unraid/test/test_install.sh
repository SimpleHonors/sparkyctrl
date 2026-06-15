# Tests for install.sh: file staging, idempotent go-hook, token generation/preservation.
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/helpers.sh"
INSTALL="$HERE/../install.sh"

# --- fresh install: stages scripts, generates token, adds exactly one go-hook block ---
sbx=$(sandbox)
printf 'FAKEBINARY' > "$sbx/sparkyctrl"
SC_FLASH_DIR="$sbx/flash" SC_GO_FILE="$sbx/go" SC_SRC_BIN="$sbx/sparkyctrl" \
    SC_INSTALL_NO_START=1 bash "$INSTALL"; rc=$?
assert_eq 0 "$rc" "fresh install exits 0"
assert_eq "FAKEBINARY" "$(cat "$sbx/flash/sparkyctrl")" "binary staged to flash"
[ -f "$sbx/flash/boot.sh" ] && echo "  ok: boot.sh staged" || fail "boot.sh staged"
[ -f "$sbx/flash/supervisor.sh" ] && echo "  ok: supervisor.sh staged" || fail "supervisor.sh staged"
[ -s "$sbx/flash/token" ] && echo "  ok: token generated non-empty" || fail "token generated non-empty"
assert_eq 1 "$(count_occurrences "$sbx/go" "# --- sparkyctrl (managed) ---")" "go-hook added once"

# --- second run is idempotent: still exactly one hook block, token unchanged ---
tok1=$(cat "$sbx/flash/token")
SC_FLASH_DIR="$sbx/flash" SC_GO_FILE="$sbx/go" SC_SRC_BIN="$sbx/sparkyctrl" \
    SC_INSTALL_NO_START=1 bash "$INSTALL"
assert_eq 1 "$(count_occurrences "$sbx/go" "# --- sparkyctrl (managed) ---")" "go-hook still single after re-run"
assert_eq "$tok1" "$(cat "$sbx/flash/token")" "existing token preserved on re-run"
rm -rf "$sbx"

# --- explicit token via SC_TOKEN is honored ---
sbx=$(sandbox); printf 'FAKEBINARY' > "$sbx/sparkyctrl"
SC_FLASH_DIR="$sbx/flash" SC_GO_FILE="$sbx/go" SC_SRC_BIN="$sbx/sparkyctrl" \
    SC_TOKEN="mytoken" SC_INSTALL_NO_START=1 bash "$INSTALL"
assert_eq "mytoken" "$(cat "$sbx/flash/token")" "explicit SC_TOKEN written"
rm -rf "$sbx"
