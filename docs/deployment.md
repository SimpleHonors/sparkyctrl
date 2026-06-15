# Deployment

How to run the worker so it stays up. Three supported targets:

- [systemd (Linux)](#systemd)
- [Unraid (reboot-persistent)](#unraid)
- [Docker (file-serving)](#docker)

## systemd

The Linux installer registers an auto-restarting systemd service. The unit ([reference](../deploy/sparkyctrl.service)):

```ini
[Unit]
Description=sparkyctrl worker
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=/usr/local/bin/sparkyctrl serve --addr 0.0.0.0:7766 --audit /var/log/sparkyctrl-audit.log
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
```

Manage it the usual way:

```sh
systemctl status sparkyctrl
systemctl restart sparkyctrl
journalctl -u sparkyctrl -f
```

The worker reads its token from `/etc/sparkyctrl/token` automatically (no flag needed). Adjust
`--fence` / `--audit` by editing the unit, or re-run the installer with new flags.

## Unraid

**Unraid runs its OS from RAM and rebuilds it from the boot USB on every reboot.** Anything you
install into `/usr/local/bin`, `/etc`, or a service manager is wiped on restart — and Unraid has
no systemd. A plain worker therefore vanishes whenever the box reboots.

The [`deploy/unraid/`](../deploy/unraid/README.md) kit makes the worker permanent the
Unraid-native way:

- The binary, token, and two small scripts live on the **flash** (`/boot/config/sparkyctrl/`),
  which survives reboot.
- `/boot/config/go` (which Unraid runs as root at every boot) re-plants them: because the flash
  is FAT32 and can't hold an exec bit or restrictive perms, the binary and token are copied into
  RAM at boot and locked down there.
- A **supervisor** keeps `serve` alive with a crash-loop guard: 30s between retries, gives up
  after 10 *rapid* failures, and a run that stays healthy for 5 minutes resets that counter.

Install it on the box (no client token required for the bootstrap path):

```sh
# copy deploy/unraid/ + a linux/amd64 sparkyctrl binary onto the box, e.g. /tmp/scu/
cd /tmp/scu && SC_SRC_BIN=/tmp/scu/sparkyctrl bash install.sh
```

Update later from a control box (pushes a new binary and bounces the worker):

```sh
./deploy/unraid/update.sh nas2 ./sparkyctrl
```

Full bootstrap, recovery, and validation steps are in
[deploy/unraid/README.md](../deploy/unraid/README.md).

## Docker

The image at [`deploy/Dockerfile`](../deploy/Dockerfile) is the **file-serving** use case: a
static binary on a `scratch` base, intended to run with a data share bind-mounted and `--fence`
pointed at the share root. It is **not** for managing the host itself — a scratch container has
no view of the host filesystem or processes.

```sh
docker build -t sparkyctrl -f deploy/Dockerfile .
docker run -d --name sparkyctrl -p 7766:7766 \
  -v /mnt/share:/data \
  sparkyctrl /sparkyctrl serve --addr 0.0.0.0:7766 --fence /data
```

> To manage an Unraid **host** (root, reboot-persistent), use the [Unraid](#unraid) path above,
> not this container.
