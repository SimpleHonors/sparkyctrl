#!/usr/bin/env bash
# sparkyctrl on-box installer for Unraid. Run this ON the Unraid box (or via `sparkyctrl exec`).
# Stages files to flash, wires an idempotent /boot/config/go hook, ensures a token, starts now.
# Env overrides (for testing/customisation):
#   SC_FLASH_DIR  (default /boot/config/sparkyctrl)
#   SC_GO_FILE    (default /boot/config/go)
#   SC_SRC_BIN    (default <script dir>/sparkyctrl)
#   SC_TOKEN      (explicit token; else a random one is generated if none exists)
#   SC_INSTALL_NO_START=1  (stage only; do not launch)
set -eu

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SC_FLASH_DIR="${SC_FLASH_DIR:-/boot/config/sparkyctrl}"
SC_GO_FILE="${SC_GO_FILE:-/boot/config/go}"
SC_SRC_BIN="${SC_SRC_BIN:-$HERE/sparkyctrl}"
SC_INSTALL_NO_START="${SC_INSTALL_NO_START:-0}"
MARK_BEGIN="# --- sparkyctrl (managed) ---"
MARK_END="# --- end sparkyctrl ---"

mkdir -p "$SC_FLASH_DIR"
install -m 0644 "$HERE/boot.sh" "$SC_FLASH_DIR/boot.sh"
install -m 0644 "$HERE/supervisor.sh" "$SC_FLASH_DIR/supervisor.sh"
if [ -f "$SC_SRC_BIN" ]; then
    install -m 0644 "$SC_SRC_BIN" "$SC_FLASH_DIR/sparkyctrl"
fi

# Token: keep existing; else use SC_TOKEN; else generate 64 hex chars from /dev/urandom.
if [ ! -f "$SC_FLASH_DIR/token" ]; then
    if [ -n "${SC_TOKEN:-}" ]; then
        printf '%s' "$SC_TOKEN" > "$SC_FLASH_DIR/token"
    else
        head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n' > "$SC_FLASH_DIR/token"
    fi
fi

# Idempotent go hook (matched by marker).
touch "$SC_GO_FILE"
if ! grep -qF "$MARK_BEGIN" "$SC_GO_FILE"; then
    {
        echo ""
        echo "$MARK_BEGIN"
        echo "[ -f $SC_FLASH_DIR/boot.sh ] && bash $SC_FLASH_DIR/boot.sh &"
        echo "$MARK_END"
    } >> "$SC_GO_FILE"
fi

if [ "$SC_INSTALL_NO_START" = "1" ]; then
    echo "staged to $SC_FLASH_DIR (start skipped)"
    exit 0
fi

SC_FLASH_DIR="$SC_FLASH_DIR" bash "$SC_FLASH_DIR/boot.sh"
echo "sparkyctrl installed and started; go-hook wired in $SC_GO_FILE"
