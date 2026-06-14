# sparkyctrl on Unraid — reboot-persistent worker

**Date:** 2026-06-14
**Status:** Approved design, ready for implementation plan
**Targets:** Two Unraid hosts on the trusted LAN (referred to below as `<HOST>`). Same
treatment applied to each.

## Problem

Unraid runs its OS from RAM and rebuilds it from the boot USB on every reboot. Anything
installed into the normal filesystem (`/usr/local/bin`, a service unit, the token at
`/etc/sparkyctrl/token`) lives in RAM and is wiped on reboot. The sparkyctrl worker
therefore vanishes whenever an Unraid box restarts. We want the worker to come back
automatically after a reboot, and to survive a mid-session crash, with no manual steps.

This is not a defect in sparkyctrl — it is the Unraid persistence model. The fix lives in
*how we deploy*, not in the daemon code.

## Goals

- Worker auto-starts on every Unraid boot.
- Worker self-heals if the process dies mid-session, with a crash-loop guard.
- Token auth preserved across reboots.
- Updating the binary is a one-step push from a control box.
- Reusable across multiple Unraid hosts (not hardcoded to one).

## Non-goals

- No changes to the daemon's command/file-handling behavior.
- Not replacing the existing `scratch` Dockerfile (that serves a different, file-serving
  use case and is left in place).
- No Unraid-UI integration (Docker/plugin manager). Managed via boot script by design.

## Key constraints discovered

1. **The Unraid flash drive is FAT32.** It cannot store a Unix execute bit or restrictive
   file permissions. Consequence: the binary and token cannot be run/secured *in place* on
   `/boot`. Standard Unraid idiom — copy from flash into RAM at boot, then set perms there.
2. **No `--token-file` flag exists.** When auth is enabled and no `--token` override is
   given, `serve` auto-reads `/etc/sparkyctrl/token` (`server.ResolveAuth` →
   `LoadTokenFile`). "Persist the token" therefore means: land the token file at that path
   at boot, mode 600.
3. **Unraid runs `/boot/config/go` as root at boot**, before the array mounts. The flash is
   mounted at that point, so a binary staged on flash is reachable.

## Architecture

### Persistent layout (on flash, survives reboot)

```
/boot/config/sparkyctrl/
├── sparkyctrl        # static linux/amd64 binary
├── token             # shared auth token (plain text — flash cannot lock it)
├── supervisor.sh     # restart-loop with crash-loop guard
└── boot.sh           # bootstrap: stage token to RAM + perms, launch supervisor
```

### Boot hook

A single appended block in `/boot/config/go`:

```sh
# --- sparkyctrl (managed; do not edit between markers) ---
[ -f /boot/config/sparkyctrl/boot.sh ] && bash /boot/config/sparkyctrl/boot.sh &
# --- end sparkyctrl ---
```

Backgrounded (`&`) so boot never hangs on it. Wrapped in markers so install/uninstall can
find and remove it idempotently.

### boot.sh (bootstrap)

Runs once per boot:

1. `install -d -m 700 /etc/sparkyctrl`
2. `install -m 600 /boot/config/sparkyctrl/token /etc/sparkyctrl/token`
3. Launch `supervisor.sh` detached, logging to `/var/log/sparkyctrl-supervisor.log`
   (RAM — ephemeral, acceptable).

### supervisor.sh (self-healing core)

Restart loop with a crash-loop guard. Defaults: 30s between retries, max 10 *rapid*
failures, a run lasting ≥ 300s (5 min) counts as healthy and resets the failure counter.

```
MAX_FAILS=10
RETRY_DELAY=30
HEALTHY_SECS=300
fails=0
while true; do
    install -m 0755 /boot/config/sparkyctrl/sparkyctrl /usr/local/bin/sparkyctrl
    start=$(date +%s)
    /usr/local/bin/sparkyctrl serve \
        --addr 0.0.0.0:7766 \
        --audit /var/log/sparkyctrl-audit.log
    ran=$(( $(date +%s) - start ))
    if [ "$ran" -ge "$HEALTHY_SECS" ]; then
        fails=0
    else
        fails=$((fails + 1))
    fi
    if [ "$fails" -ge "$MAX_FAILS" ]; then
        echo "$(date -Is) crash-loop: gave up after $fails rapid failures" >&2
        exit 1
    fi
    echo "$(date -Is) worker exited (ran ${ran}s, fails=$fails); retrying in ${RETRY_DELAY}s" >&2
    sleep "$RETRY_DELAY"
done
```

**Crash-loop guard rationale:** the `MAX_FAILS` cap applies only to rapid consecutive
failures. A healthy run (≥ 5 min) resets the counter, so sporadic crashes spread over weeks
never exhaust the budget, while a genuine boot-time crash loop is hammered at most ~10× over
~5 minutes and then abandoned loudly (visible in the supervisor log).

**Update mechanism (free):** the supervisor re-stages the binary from flash at the top of
every loop. Updating the tool is therefore: push a new binary to flash, kill the running
worker; the supervisor relaunches on the fresh copy.

### Auth

Token persisted on flash → staged to `/etc/sparkyctrl/token` (mode 600) at boot → auto-read
by `serve`. **One distinct token per host**, so compromise of one box does not grant the
other. The control box selects the right token per host (client reads `SPARKYCTRL_TOKEN`).

### Audit log

`/var/log/sparkyctrl-audit.log` (RAM, ephemeral by default). If cross-reboot audit history
is later wanted, repoint `--audit` to a path on the array/cache share. Out of scope now.

## Deploy artifacts (repo)

New first-class, reusable Unraid deploy path:

```
deploy/unraid/
├── boot.sh           # staged to /boot/config/sparkyctrl/ on the host
├── supervisor.sh     # staged to /boot/config/sparkyctrl/ on the host
├── install.sh        # run from a control box OR pasted on the host; idempotent
└── README.md         # go-file edit, recovery, update flow, FAT32 caveats
```

`install.sh` responsibilities (idempotent):

- Stage `sparkyctrl`, `token`, `boot.sh`, `supervisor.sh` into `/boot/config/sparkyctrl/`.
- Generate a token if none supplied.
- Append the marked block to `/boot/config/go` only if not already present.
- Start the worker immediately (no reboot required to activate).

Two run modes:

1. **Remote** (from the control box, requires a working client token for the host): drives
   the host via the already-running worker (`push` + `exec`).
2. **On-box** (copy/paste into the Unraid terminal): pure-local, no client needed — used for
   first-time bootstrap when no working token exists yet.

The existing `deploy/Dockerfile` (scratch, file-serving) is unchanged; a one-line pointer is
added directing Unraid host-management users to `deploy/unraid/`.

## Testing / validation

- `shellcheck` clean on all new shell scripts.
- Dry-run of `install.sh` (no-op when already installed; correct files staged when not).
- End-to-end on the first host: install → confirm worker answers → reboot → confirm it comes
  back unattended → kill the process → confirm supervisor relaunches.
- Replicate to the second host.

## Rollout

1. Land `deploy/unraid/` in the repo with tests.
2. Bootstrap host #1 on-box (no token assumed), validate reboot + self-heal.
3. Bootstrap host #2 the same way.
4. Confirm the control box can update each host with a single push.

## Open items (resolved by operator delegation)

- Crash guard: **healthy-reset** variant chosen (not strict lifetime cap).
- Tokens: **one per host**.
- Install: **supports both remote and on-box**; on-box is the bootstrap path.
