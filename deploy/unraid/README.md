# sparkyctrl on Unraid (reboot-persistent)

Unraid runs its OS from RAM and rebuilds it from the boot USB on every reboot, so anything
installed into `/usr/local/bin`, `/etc`, or a service manager is wiped on restart. This deploy
path makes the worker permanent the Unraid-native way: persistent files live on the flash, and
`/boot/config/go` re-plants them on every boot.

## Why it is built this way

- **The flash is FAT32** — it cannot store an execute bit or restrictive file perms. So the
  binary and token are copied off flash into RAM at boot, where perms are applied.
- **`/boot/config/go` runs as root at boot.** A single marked block there launches `boot.sh`.
- **`supervisor.sh` keeps `serve` alive** with a crash-loop guard: 30s between retries, gives up
  after 10 *rapid* failures, and a run that stays healthy for 5 min resets that counter.

## First-time bootstrap (on the box)

No working client token is required for this path — run it directly on the Unraid terminal.

1. Copy the `deploy/unraid/` directory and a linux/amd64 `sparkyctrl` binary onto the box
   (SMB share, USB, or `scp`), e.g. into `/tmp/scu/`.
2. Run the installer:
   ```sh
   cd /tmp/scu && SC_SRC_BIN=/tmp/scu/sparkyctrl bash install.sh
   ```
   This stages files to `/boot/config/sparkyctrl/`, generates a token if none exists, adds the
   `go` hook, and starts the worker immediately. Read the generated token from
   `/boot/config/sparkyctrl/token` and set it as `SPARKYCTRL_TOKEN` on your control box.
3. Verify: `sparkyctrl info <host>` from the control box.

## Updating later (from the control box)

```sh
./deploy/unraid/update.sh <host> ./sparkyctrl
```
Pushes the new binary to flash and bounces the worker; the supervisor relaunches on it.

## Recovery / uninstall

- The `go` hook is wrapped in `# --- sparkyctrl (managed) ---` / `# --- end sparkyctrl ---`
  markers in `/boot/config/go`. Delete that block to disable auto-start.
- Logs (RAM, reset on reboot): `/var/log/sparkyctrl-supervisor.log`,
  `/var/log/sparkyctrl-audit.log`.
- If the supervisor gave up (crash loop), its log shows `giving up after N rapid failures`.

## Validation checklist

- [ ] `install.sh` run; `sparkyctrl info <host>` succeeds.
- [ ] Reboot the box; worker answers again with no manual steps.
- [ ] `pkill -f 'sparkyctrl serve'` on the box; supervisor relaunches within ~30s.
