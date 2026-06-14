# Tests for boot.sh token staging. Uses SC_BOOT_NO_LAUNCH to skip launching the supervisor.
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/helpers.sh"
BOOT="$HERE/../boot.sh"

sbx=$(sandbox)
printf 'secrettoken123' > "$sbx/flash_token"
mkdir -p "$sbx/flash"
cp "$sbx/flash_token" "$sbx/flash/token"

SC_FLASH_DIR="$sbx/flash" SC_TOKEN_DIR="$sbx/etc" SC_BOOT_NO_LAUNCH=1 \
    bash "$BOOT"; rc=$?
assert_eq 0 "$rc" "boot.sh exits 0 in no-launch mode"
assert_eq "secrettoken123" "$(cat "$sbx/etc/token")" "token staged to RAM path"
assert_file_mode "$sbx/etc/token" 600 "staged token is mode 600"
assert_file_mode "$sbx/etc" 700 "token dir is mode 700"
rm -rf "$sbx"

# --- no token on flash: still succeeds, just stages nothing ---
sbx=$(sandbox); mkdir -p "$sbx/flash"
SC_FLASH_DIR="$sbx/flash" SC_TOKEN_DIR="$sbx/etc" SC_BOOT_NO_LAUNCH=1 \
    bash "$BOOT"; rc=$?
assert_eq 0 "$rc" "boot.sh tolerates missing flash token"
rm -rf "$sbx"
