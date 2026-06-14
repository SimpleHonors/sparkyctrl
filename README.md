# sparkyctrl

**A single-binary remote command-and-file daemon for AI agents on a trusted LAN.**

> ## ☢️ STOP. READ THIS BEFORE YOU SCROLL.
>
> This is a **remote-code-execution daemon you install on purpose.** It runs commands as
> **root**, over the network, with **no real authentication.** If that sentence did not make
> you wince, you are precisely the person who should not run this.
>
> There is a **~99% chance the correct move is to close this tab.** We mean it. Keep reading
> only if you are the specific, paranoid, fully-consenting operator this was built for.

## What this actually is

sparkyctrl lets an AI agent run commands and move files on another machine with **no shell in
the middle to mangle them.** Commands go out as a structured argument array straight to the OS
`exec` call, so the quoting and escaping disasters that happen when an agent drives a box over
SSH simply cannot.

It solves a real problem. It solves it by being a loaded gun with the safety welded off. To put
it plainly: **it is a backdoor, and we built it that way on purpose.** `exec` and `shell` run as
**root**, unfenced, by design. That is not an oversight we intend to fix. That is the product.

## Should you use this? (Almost certainly not.)

Walk the checklist. Any **No** means *close the tab.*

- Is the target on a **private, trusted LAN** with zero untrusted devices on it? → No → **close the tab.**
- Is it **never, under any circumstances, reachable from the internet**? → No → **close the tab.**
- Are you the **sole operator**, and do you trust **every agent** you will ever point at it? → No → **close the tab.**
- Would you hand a **stranger a root shell** on this machine? Because that is the threat model the instant any assumption above is wrong. → No → **close the tab.**
- Do you understand that the path "fence" is a **convenience, not a security boundary**, and that `exec` walks straight past it? → No → **close the tab.**

Still here? Suspicious. But fine.

## Why this exists anyway

AI agents are genuinely bad at shells. Every layer — local shell, SSH, the remote shell —
re-parses a command, and one stray backtick or unquoted `$(...)` can turn "list the logs" into
something that takes the machine down. (Tools in this exact genre have run `halt` on the wrong
box because an agent pasted untrusted text into a shell. Ask us how we know.) sparkyctrl removes
the shells from the default path so that class of bug cannot happen. That is the whole pitch: a
sharp tool for a trusted operator on a trusted network — and **nothing about it pretends to be
safe.**

## What it is **not**

Read this part twice.

- **Not authenticated.** There is an optional shared token. That is it. No users, no roles, no
  TLS, no meaningful rate limiting. The token is a speed bump, not a wall.
- **Not contained.** `exec`/`shell` run as root with the full capability set. They can read
  `/etc/shadow`, rewrite `/etc/sudoers`, and yes, halt the machine. The fence governs the *file*
  verbs only, and only if you opt in.
- **Not a security boundary.** The fence is symlink-safe and handy for keeping file ops tidy. It
  is **not** what stands between an attacker and your host. The real wall is **OS-level
  isolation** — run the worker in a container with only the shares you mean to expose
  bind-mounted in.
- **Not for the internet. Ever.** If this port is reachable from outside your LAN, you have not
  deployed a tool. You have published a root shell.

---

> ## ☢️ LAST CHANCE.
>
> The command below installs a **root-level remote shell** and sets it **listening on your
> network.** This is the dangerous thing. There is no undo — only `systemctl stop` and regret.
> By running it you accept every inevitability described above.

## Install (you were warned)

```sh
curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.sh \
  | sudo bash -s -- --mode admin --fence /srv/share --start
```

That installs the worker as **root** (admin mode) and starts it listening. Want to be slightly
less reckless? Swap `--mode admin` for **`--mode hardened`** — a dedicated unprivileged user,
zero capabilities, a read-only filesystem except the fence and audit log, and `exec`/`shell`
that no longer run as root. Inside an unprivileged LXC, add `--container` (the mount-namespace
isolation cannot be set up there).

The installer auto-downloads the right prebuilt binary for your CPU from the latest release.
Re-run any time to switch modes. `./deploy/install.sh --help` lists every flag (`--addr`,
`--audit`, `--token`, `--user`, `--no-enable`). Build from source with `./deploy/build.sh`.

### Windows (yes, really — but less battle-tested)

The bash installer is Linux-only; on Windows use the PowerShell installer. In an **elevated**
PowerShell:

```powershell
$ProgressPreference = 'SilentlyContinue'   # else the download progress bar can look frozen
irm https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.ps1 -OutFile install.ps1
.\install.ps1 -Fence C:\share -Start
```

It installs the worker under `Program Files`, opens a **Private-profile** firewall rule for the
port (never Public/internet), and registers a Scheduled Task that runs it as **SYSTEM** at
startup with auto-restart — the rough equivalent of Linux admin mode. Remove everything with
`.\install.ps1 -Uninstall`; `Get-Help .\install.ps1 -Detailed` lists the flags (`-Addr`,
`-Audit`, `-Token`, `-Version`, `-Binary`, ...).

Windows is less battle-tested than Linux here: if something is going to be weird, it'll be weird
on Windows first.

## Using it

From the agent side — a CLI, so it adds ~nothing to an agent's context budget:

- `sparkyctrl exec  <host> -- <argv...>` — run a command, mangle-proof; stdout/stderr/exit code back.
- `sparkyctrl shell <host> <script>` — explicit, logged shell path for pipes/globs/etc.
- `sparkyctrl read|write|ls|push|pull <host> ...` — binary-safe file operations.
- `sparkyctrl edit  <host> <remote> --old X --new Y [--all]` — surgical exact-string replacement (refuses on no-match or non-unique match; atomic write).
- `sparkyctrl info  <host>` — worker info.

`<host>` is a name from `~/.sparkyctrl/hosts.toml` or a literal `host:port`. Add `--json` to any
verb for raw output.

## The audit log keeps receipts, not guarantees

Every request — including denied ones — is logged with its source IP and outcome. This tells you
**what happened.** It does **not** prevent anything, and anyone with `exec` access can truncate
the log through the same channel it audits. It is a flight recorder, not a seatbelt.

## Philosophy

A sharp tool for a trusted operator: *if you want to do something stupid, we will not stop you.*
No guardrails beyond an opt-in fence; the audit log keeps receipts rather than preventing.
Design details live in `docs/superpowers/specs/`.

## Status

v0.1.0 — public. Built, tested, and running on exactly one person's trusted LAN. Use at your own
considerable risk.
