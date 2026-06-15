# sparkyctrl `upgrade` command — design

**Date:** 2026-06-15
**Status:** Approved (shape), pending spec review

## Problem

Upgrading a worker today means piping `install.sh` (or `install.ps1`) off the internet through `curl`/`bash` and per-host shell. A fleet rollout exposed how fragile that is:

- `curl` is not present on minimal containers → download fails.
- `install.sh` prompts interactively for the fence → fails under `pct exec` / no TTY.
- The running binary can't be overwritten in place → `Text file busy` / Windows `EBUSY`.
- `install.sh` assumes systemd → fails on Unraid/flash hosts; the flash supervisor then re-stages the **old** binary on restart.
- Restart orchestration was ad-hoc (`killall` + sleeps, failed SSH).

The root issue: platform knowledge lives in external shell. The fix is to **internalize it into the binary** as a self-update subcommand.

## Goal

A single command — `sparkyctrl upgrade` — that updates the worker in place, non-interactively, on every platform we run (systemd Linux, Unraid/flash Linux, Windows scheduled task), reusing the existing trusted channel for fleet rollout. No `curl`, no TTY, no manual restart dance.

## Non-goals (v1)

- A new remote-trigger RPC / `/v1/upgrade` endpoint. Fleet rollout reuses the existing authenticated `exec` channel; no new network surface.
- Downgrade safety guarantees beyond keeping a single backup.
- Arbitrary download sources. Only the pinned official release is ever fetched.

## Command

`sparkyctrl upgrade` — a **local** subcommand (same group as `serve`/`mcp`), run on the host it updates.

Fleet rollout: `sparkyctrl exec <host> -- sparkyctrl upgrade`, looped over hosts. The exec call may drop mid-restart; that is expected and benign.

Flags:
- `--version <vX.Y.Z>` — target version (default: latest release).
- `--check` — report current vs. available and exit; make no changes.
- `--no-restart` — swap the binary but skip the service restart (operator restarts).

The official repo is a pinned constant — there is no arbitrary-URL flag.

## Steps the command performs

1. **Resolve target.** Query the official GitHub releases API for `latest` (or use `--version`). Select the asset for this host: `sparkyctrl-<goos>-<goarch>[.exe]` from `runtime.GOOS`/`GOARCH`.
2. **Short-circuit.** If the current version already equals the target, print and exit 0.
3. **Download** the asset over HTTPS using the binary's own HTTP client (no `curl`), to a temp file in the **same directory** as the running binary (so the later rename is atomic on one filesystem).
4. **Verify.** Check the download against the release's published `SHA256SUMS`. As a second gate, run `<tempbinary> version` and require it to print the expected target version. Never swap in a binary that fails either check.
5. **Back up + atomic swap.**
   - Resolve the running binary path via `os.Executable()`.
   - Copy current → `<path>.bak`.
   - Replace the binary atomically:
     - Linux: `rename(temp, path)` — succeeds even while the old binary is running.
     - Windows: rename the running `.exe` → `<path>.old`, then move the new file into `path` (Windows permits renaming a running executable).
   - `chmod 0755`.
6. **Flash hosts (Unraid).** If a flash install is detected (e.g. `/boot/config/sparkyctrl/` present), also update the flash copy so the supervisor does not re-stage the old binary.
7. **Restart via the detected manager:**
   - **systemd:** `systemctl restart <service>` (default service name `sparkyctrl`).
   - **Unraid/flash supervisor:** restart via the supervisor (flash copy already updated in step 6).
   - **Windows scheduled task:** stop then start the task (`Stop-ScheduledTask` / `Start-ScheduledTask`, or `schtasks /End` + `/Run`). **In v1.**
   - **Unknown:** skip restart and print clearly: binary updated to `<version>`, restart the service yourself.
8. **Post-restart health check (best effort).** Briefly wait, then confirm the worker is back and reports the target version (or the task is `Running` on Windows). Report success or a clear failure.
9. **Backup retained** at `<path>.bak` / `<path>.old` for manual rollback. (Auto-rollback on failed health check is a possible enhancement, not required for v1.)

## Security

- Pinned official repo only; no arbitrary URLs.
- HTTPS download + SHA256 checksum verification (requires publishing `SHA256SUMS` as a release asset — a small addition to the release process).
- Run-and-version sanity gate before swap.
- Reuses the existing `exec` auth boundary for remote rollout; opens no new network surface. (`upgrade` is no more privileged than the arbitrary command execution the token already authorizes.)

## Code structure

New `internal/upgrade` package, small focused units:

- `release.go` — resolve target version + asset URL from the releases API.
- `download.go` — download + checksum verify to a temp file.
- `swap.go` — backup + atomic replace; platform branch (Linux rename-over vs. Windows rename-running-then-move) via build-tagged files.
- `restart.go` (+ `restart_linux.go`, `restart_windows.go`) — detect the manager and restart; systemd / Unraid-flash / Windows-task / fallback.
- `upgrade.go` — orchestrator.

`internal/cli/cli.go` — add `case "upgrade": return runUpgrade(args)` to the local-command switch; `runUpgrade` parses flags and calls the package.

Release process — add a step that publishes `SHA256SUMS` alongside the binaries.

## Testing

- The init-system detector and the command runner are injected behind interfaces, so tests assert behavior without running real `systemctl`/`schtasks`:
  - systemd detected → calls `systemctl restart sparkyctrl`.
  - Unraid/flash detected → updates flash copy + supervisor restart.
  - Windows task detected → stop+start task.
  - unknown → no restart, warning printed.
- Asset-name selection per `GOOS`/`GOARCH`.
- Checksum verification: good and bad checksum cases.
- The run-and-version sanity gate: rejects a binary that reports the wrong version.
- Swap logic against temp files (no real binary needed).

## Rollout of the feature itself

Once built: cut a release that contains `upgrade`, then this is the **last** time the `curl | install.sh` dance is needed — subsequent upgrades use `sparkyctrl upgrade`.
