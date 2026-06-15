#!/usr/bin/env bash
# sparkyctrl worker supervisor: restart loop with crash-loop guard.
# Re-stages the binary from flash each loop, so updating = replace flash binary + kill worker.
# All paths/bounds are env-overridable for testing.
set -u

SC_FLASH_DIR="${SC_FLASH_DIR:-/boot/config/sparkyctrl}"
SC_BIN="${SC_BIN:-/usr/local/bin/sparkyctrl}"
SC_ADDR="${SC_ADDR:-0.0.0.0:7766}"
SC_AUDIT="${SC_AUDIT:-/var/log/sparkyctrl-audit.log}"
SC_MAX_FAILS="${SC_MAX_FAILS:-10}"        # max rapid (unhealthy) failures before giving up
SC_RETRY_DELAY="${SC_RETRY_DELAY:-30}"    # seconds between retries
SC_HEALTHY_SECS="${SC_HEALTHY_SECS:-300}" # a run this long resets the fail counter
SC_MAX_LOOPS="${SC_MAX_LOOPS:-0}"         # 0 = infinite; >0 bounds iterations (testing only)

now() { date '+%Y-%m-%dT%H:%M:%S'; }
log() { echo "$(now) supervisor: $*" >&2; }

fails=0
loops=0
while true; do
    loops=$((loops + 1))
    if ! install -m 0755 "$SC_FLASH_DIR/sparkyctrl" "$SC_BIN" 2>/dev/null; then
        log "ERROR: cannot stage binary from $SC_FLASH_DIR/sparkyctrl"
        exit 2
    fi
    start=$(date +%s)
    "$SC_BIN" serve --addr "$SC_ADDR" --audit "$SC_AUDIT"
    rc=$?
    ran=$(( $(date +%s) - start ))
    if [ "$ran" -ge "$SC_HEALTHY_SECS" ]; then
        fails=0
    else
        fails=$((fails + 1))
    fi
    log "worker exited rc=$rc ran=${ran}s fails=$fails/$SC_MAX_FAILS"
    if [ "$fails" -ge "$SC_MAX_FAILS" ]; then
        log "crash-loop: giving up after $fails rapid failures"
        exit 1
    fi
    if [ "$SC_MAX_LOOPS" -gt 0 ] && [ "$loops" -ge "$SC_MAX_LOOPS" ]; then
        log "reached SC_MAX_LOOPS=$SC_MAX_LOOPS (test bound); exiting 0"
        exit 0
    fi
    sleep "$SC_RETRY_DELAY"
done
