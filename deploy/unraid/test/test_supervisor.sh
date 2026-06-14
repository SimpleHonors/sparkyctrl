# Tests for supervisor.sh crash-loop guard and healthy-reset.
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
. "$HERE/helpers.sh"
SUP="$HERE/../supervisor.sh"

# --- crash-loop: fake serve exits immediately, must give up after SC_MAX_FAILS ---
sbx=$(sandbox)
cat > "$sbx/sparkyctrl" <<'EOF'
#!/usr/bin/env bash
exit 1
EOF
chmod +x "$sbx/sparkyctrl"
out=$(SC_FLASH_DIR="$sbx" SC_BIN="$sbx/bin" SC_AUDIT="$sbx/audit.log" \
      SC_MAX_FAILS=3 SC_RETRY_DELAY=0 SC_HEALTHY_SECS=300 \
      bash "$SUP" 2>&1); rc=$?
assert_eq 1 "$rc" "crash-loop gives up with exit 1"
assert_contains "$out" "giving up after 3" "crash-loop logs give-up after 3 failures"
rm -rf "$sbx"

# --- healthy-reset: fake serve stays up >= healthy threshold, never gives up ---
sbx=$(sandbox)
cat > "$sbx/sparkyctrl" <<'EOF'
#!/usr/bin/env bash
sleep 1
exit 0
EOF
chmod +x "$sbx/sparkyctrl"
out=$(SC_FLASH_DIR="$sbx" SC_BIN="$sbx/bin" SC_AUDIT="$sbx/audit.log" \
      SC_MAX_FAILS=2 SC_RETRY_DELAY=0 SC_HEALTHY_SECS=1 SC_MAX_LOOPS=4 \
      bash "$SUP" 2>&1); rc=$?
assert_eq 0 "$rc" "healthy runs never trip the guard (exit 0 at loop bound)"
rm -rf "$sbx"

# --- missing binary on flash exits 2, not an infinite spin ---
sbx=$(sandbox)
out=$(SC_FLASH_DIR="$sbx" SC_BIN="$sbx/bin" SC_RETRY_DELAY=0 \
      bash "$SUP" 2>&1); rc=$?
assert_eq 2 "$rc" "missing flash binary exits 2"
rm -rf "$sbx"
