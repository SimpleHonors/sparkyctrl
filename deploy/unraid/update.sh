#!/usr/bin/env bash
# Update a running Unraid worker from a control box: push a new binary to flash and bounce
# the worker. The supervisor re-stages the binary from flash on its next loop iteration.
# Usage: update.sh <host> [path-to-binary]   (default binary: ./sparkyctrl)
set -eu

HOST="${1:?usage: update.sh <host> [binary]}"
BIN="${2:-./sparkyctrl}"

[ -f "$BIN" ] || { echo "binary not found: $BIN" >&2; exit 1; }

sparkyctrl push "$HOST" "$BIN" /boot/config/sparkyctrl/sparkyctrl
sparkyctrl exec "$HOST" -- pkill -f 'sparkyctrl serve' || true
echo "pushed new binary to $HOST; supervisor will relaunch on the fresh copy"
