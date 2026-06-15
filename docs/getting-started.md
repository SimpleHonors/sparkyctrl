# Getting started

This guide installs the **worker** on a machine you want to drive, sets up the **client** on the
agent side, and runs your first command. For the security model behind all of this, read
[security.md](security.md) first.

- [Install the worker](#install-the-worker)
  - [Linux](#linux)
  - [Windows](#windows)
  - [Docker](#docker)
  - [Build from source](#build-from-source)
- [Set up the client](#set-up-the-client)
- [Your first command](#your-first-command)
- [Troubleshooting](#troubleshooting)

## Install the worker

The installer **asks where to fence the worker's file operations** — it does not bake in a path.
Answer with a directory to confine to, or `none` for full filesystem access; or pass `--fence DIR`
/ `--no-fence` up front for an unattended install. It also generates a worker token file by
default and prints a matching client snippet.

Both installers auto-download the right prebuilt binary from the latest release, are safe to
**re-run to update** (stop worker → swap binary → restart), and **uninstall** the same way.

### Linux

Installs as **root** and starts listening:

```sh
curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.sh \
  | sudo bash -s -- --start
```

The worker writes its token to `/etc/sparkyctrl/token` and listens on `0.0.0.0:7766` by default.
Uninstall with `--uninstall`.

**Hardened mode (systemd only):** `--mode hardened` runs the worker as a dedicated unprivileged
user with zero capabilities and a read-only filesystem except the fence + audit log — `exec` and
`shell` no longer run as root. Add `--container` for that mode inside an unprivileged LXC.

### Windows

In an **elevated** PowerShell. Runs as **SYSTEM**, opens a Private-profile firewall rule (never
Public/internet), and registers an auto-restarting scheduled task:

```powershell
$ProgressPreference = 'SilentlyContinue'   # else the download progress bar can look frozen
irm https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.ps1 -OutFile install.ps1
.\install.ps1 -Start
```

Uninstall with `-Uninstall`.

> Windows is less battle-tested than Linux: if something is going to be weird, it'll be weird on
> Windows first.

Installer flags are parallel across platforms (Linux `--lower-case`, Windows `-PascalCase`):

| | Linux | Windows |
|---|---|---|
| confine file ops | `--fence DIR` | `-Fence DIR` |
| full access (no fence) | `--no-fence` | `-NoFence` |
| listen address | `--addr H:P` | `-Addr H:P` |
| audit log path | `--audit FILE` | `-Audit FILE` |
| override token | `--token T` | `-Token T` |
| disable auth | `--no-auth` | `-NoAuth` |
| release version | `--version V` | `-Version V` |
| use a local binary | `--binary PATH` | `-Binary PATH` |
| start now | `--start` | `-Start` |
| uninstall | `--uninstall` | `-Uninstall` |

### Docker

A scratch image for the **fenced file-serving** use case (not host management) lives at
[`deploy/Dockerfile`](../deploy/Dockerfile). See [deployment.md](deployment.md#docker).

### Build from source

```sh
git clone https://github.com/SimpleHonors/sparkyctrl.git
cd sparkyctrl
go build -o sparkyctrl .        # the client+worker are one binary
./deploy/build.sh               # or cross-compile release binaries
```

## Set up the client

The client is the same binary. Give it two small files in `~/.sparkyctrl/` (or the current
directory, or paths from `SPARKYCTRL_HOSTS` / `SPARKYCTRL_TOKENS`):

```sh
mkdir -p ~/.sparkyctrl

# Address book: name -> host:port
cat > ~/.sparkyctrl/hosts.toml <<'EOF'
nas2 = "192.0.2.50:7766"
web  = "192.0.2.60:7766"
EOF

# Per-host tokens: name -> token. Keep the secret OFF the command line.
printf 'nas2 = "PASTE_NAS2_TOKEN"\n' > ~/.sparkyctrl/tokens
chmod 600 ~/.sparkyctrl/tokens
```

The client reads the matching token by host name, so you never type it. Full details in
[configuration.md](configuration.md).

## Your first command

```sh
sparkyctrl info nas2                       # OS/arch/version/fence
sparkyctrl exec nas2 -- uname -a           # mangle-proof: argv straight to exec
sparkyctrl exec nas2 -- ls -la /var/log
sparkyctrl push nas2 ./notes.txt /tmp/notes.txt
sparkyctrl read nas2 /etc/hostname
```

You can also use a literal `host:port` instead of a name:

```sh
sparkyctrl info 192.0.2.50:7766
```

## Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `info failed (401): invalid or missing token` | Client token doesn't match the worker. Check `~/.sparkyctrl/tokens` has the right entry for this host, or that `SPARKYCTRL_TOKEN` (if set) is correct. The worker's token is in `/etc/sparkyctrl/token`. |
| `unknown host "x"` | `x` isn't in `hosts.toml` and isn't a `host:port`. Add it, or pass a literal address. |
| `warning: <file> is readable by others (chmod 600)` | Your tokens file is group/other-readable. `chmod 600 ~/.sparkyctrl/tokens`. |
| Connection refused / timeout | Worker not running or wrong address/port. On the worker host, confirm it's listening on `:7766`. |
| File verb fails with a fence error | The path is outside the worker's `--fence` directory. `exec`/`shell` are never fenced; file verbs are. |
| Token keeps appearing in your terminal | You're passing it inline or via `SPARKYCTRL_TOKEN`. Use the tokens file instead (see [configuration.md](configuration.md)). |

For the Unraid "worker vanishes on reboot" case, see [deployment.md](deployment.md#unraid).
