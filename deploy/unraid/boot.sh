#!/usr/bin/env bash
# sparkyctrl boot bootstrap. Invoked by /boot/config/go at every Unraid boot.
# Stages the token from FAT32 flash into a RAM path with real perms, then launches the supervisor.
set -u

SC_FLASH_DIR="${SC_FLASH_DIR:-/boot/config/sparkyctrl}"
SC_TOKEN_DIR="${SC_TOKEN_DIR:-/etc/sparkyctrl}"
SC_SUPERVISOR="${SC_SUPERVISOR:-$SC_FLASH_DIR/supervisor.sh}"
SC_SUP_LOG="${SC_SUP_LOG:-/var/log/sparkyctrl-supervisor.log}"
SC_BOOT_NO_LAUNCH="${SC_BOOT_NO_LAUNCH:-0}"  # test hook: stage only, do not launch

install -d -m 700 "$SC_TOKEN_DIR"
if [ -f "$SC_FLASH_DIR/token" ]; then
    install -m 600 "$SC_FLASH_DIR/token" "$SC_TOKEN_DIR/token"
fi

[ "$SC_BOOT_NO_LAUNCH" = "1" ] && exit 0

setsid bash "$SC_SUPERVISOR" >>"$SC_SUP_LOG" 2>&1 < /dev/null &
exit 0
