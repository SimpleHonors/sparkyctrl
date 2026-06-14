# sparkyctrl

A single-binary remote sysadmin tool built for AI agents on a trusted LAN.

It exists to kill one specific, expensive bug: **command mangling**. When an agent
drives a machine over SSH, its commands get re-parsed by every shell along the way,
and quoting/escaping mistakes can change what a command actually does — occasionally
destructively. Sparkyctrl sends commands as a **structured argument array** straight to
the OS exec call, with **no shell on the default path**, so that class of bug cannot
happen.

## What it does

- **`sparkyctrl serve`** — runs the worker daemon on a target machine (Linux, LXC, Docker,
  VM, Windows, Unraid). One static binary, no dependencies.
- **`sparkyctrl exec <host> -- <argv...>`** — run a command, mangle-proof, get
  stdout/stderr/exit code back.
- **`sparkyctrl shell <host> <script>`** — explicit, logged shell path for pipes/globs/etc.
- **`sparkyctrl read|write|ls|push|pull <host> ...`** — binary-safe file operations.
- **`sparkyctrl edit <host> <remote> --old X --new Y [--all]`** — surgical exact-string
  replacement in a remote file (refuses on no-match or non-unique match; atomic write).

LAN-only, single-user, no authentication system by design (optional shared token).
Driven from the terminal as a CLI, so it adds ~nothing to an agent's context budget.

**Philosophy:** a sharp tool for a trusted operator — *if you want to do stupid
things, we won't stop you.* No guardrails beyond an opt-in path fence; the audit log
keeps receipts rather than preventing. See the design spec's "Design philosophy".

See `docs/superpowers/specs/` for the design spec.

## Install

**One-liner** (downloads the prebuilt binary, installs the worker in admin mode, starts it):

```sh
curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.sh \
  | sudo bash -s -- --mode admin --fence /srv/share --start
```

Add `--mode hardened` (and `--container` inside an unprivileged LXC) for the locked-down,
non-root file-only mode. The script auto-downloads the right binary for your CPU from the
latest release; override with `SPARKYCTRL_VERSION` or a local `--binary <path>`.

`deploy/install.sh` drops the binary and a systemd unit, choosing the worker's
privilege **mode** at install time:

```sh
# Admin mode (default): worker runs as root, exec/shell unfenced — trusted sysadmin use.
sudo ./deploy/install.sh --mode admin --fence /srv/share --start

# Hardened mode: worker runs as an unprivileged user with zero capabilities and a
# read-only filesystem except the fence + audit log. File-serving only (exec/shell
# no longer run as root).
sudo ./deploy/install.sh --mode hardened --fence /srv/share --start

# In an unprivileged LXC, add --container: mount-namespace isolation can't be set up
# there (the worker would die with 226/NAMESPACE), but the non-root / no-capability
# hardening still applies.
sudo ./deploy/install.sh --mode hardened --container --fence /srv/share --start
```

Re-run any time to switch modes. Build the binary first with `./deploy/build.sh`
(or pass `--binary <path>`). Run `./deploy/install.sh --help` for all flags
(`--addr`, `--audit`, `--token`, `--user`, `--no-enable`).

## Status

Implemented and in use. Core verbs, the opt-in path fence (symlink-safe), audit
logging of denials with source IP, request hardening, and the surgical `edit`
verb are built and tested.
