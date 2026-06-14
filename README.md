# sparkyctrl

**A single-binary remote command-and-file daemon for AI agents on a trusted LAN.**

> ## 💡 What you need to know
>
> This is a **remote-code-execution daemon** you install on purpose. It runs commands as root
> with a **shared token for auth**. It is built strictly for a **trusted LAN**. 
> If you expose this port to the internet, you have not deployed a tool—you have published a root shell.
>
> It solves a real problem: AI agents are genuinely bad at shells. It does this by being a sharp 
> tool rather than a padded room. Know exactly what you're installing.

## What this actually is

sparkyctrl lets an AI agent run commands and move files on another machine with **no shell in
the middle to mangle them.** Commands go out as a structured argument array straight to the OS
`exec` call. This prevents the quoting and escaping disasters that happen when an agent drives 
a box over SSH.

It's a sharp tool built for a specific operator on a specific network. `exec` and `shell` run as
**root**, unfenced, by design — that is the product, not an oversight. File verbs (read, write, 
edit, ls) can be confined to a directory via an opt-in fence. Authentication is enforced via a 
shared token generated at install time, which can be disabled if your network is your boundary.

## Security reality

sparkyctrl gives agents root on your machines. The token keeps honest agents honest on a
trusted LAN, and that's its entire job. If that sentence doesn't sit right, don't install it.

- **If this port touches the internet, you've published a root shell.** No exceptions.
- Token auth is **a lock on the door, not a bank vault.** Disable it (`--no-auth`) only on a network you physically control.
- `exec` and `shell` are **unfenced by design.** The path fence limits file verbs only.
- If you wouldn't hand the token to a stranger with a root prompt, this tool is not for you.

## Why this exists anyway

Every layer — local shell, SSH, the remote shell — re-parses a command. One stray backtick or unquoted
`$(...)` can turn "list the logs" into something that halts the machine. sparkyctrl removes the
shells from the default path so that class of bug cannot happen.

## Install

The installer **asks where to fence the worker's file operations** — it does not bake in a
path. Answer the prompt with a directory to confine to, or `none` for full filesystem access;
or pass `--fence DIR` / `--no-fence` up front for an unattended install. It also generates a
worker token file by default and prints a matching `export SPARKYCTRL_TOKEN="..."` snippet for the
client side.

**Linux** — installs as **root** and starts listening:

```sh
curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.sh \
  | sudo bash -s -- --start
```

**Windows** — in an **elevated** PowerShell; runs as **SYSTEM**, opens a Private-profile
firewall rule (never Public/internet), and registers an auto-restarting scheduled task:

```powershell
$ProgressPreference = 'SilentlyContinue'   # else the download progress bar can look frozen
irm https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.ps1 -OutFile install.ps1
.\install.ps1 -Start
```

Both auto-download the right prebuilt binary from the latest release, are safe to **re-run to
update** (they stop the worker, swap the binary, restart it), and **uninstall** the same way
(`--uninstall` / `-Uninstall`).

Flags are parallel across platforms (Linux `--lower-case`, Windows `-PascalCase`, same meaning):

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

**Linux-only** (these need systemd): `--mode hardened` runs the worker as a dedicated
unprivileged user with zero capabilities and a read-only filesystem except the fence + audit
log (`exec`/`shell` no longer run as root); add `--container` for that mode inside an
unprivileged LXC. On Windows the worker always runs as SYSTEM (admin-equivalent). Build from
source with `./deploy/build.sh`.

> Windows is less battle-tested than Linux: if something is going to be weird, it'll be weird
> on Windows first.

## Using it

From the agent side — a CLI, so it adds ~nothing to an agent's context budget:

- `sparkyctrl exec  <host> -- <argv...>` — run a command, mangle-proof; stdout/stderr/exit code back.
- `sparkyctrl shell <host> <script>` — explicit, logged shell path for pipes/globs/etc.
  (`cmd` on Windows, `/bin/sh` on Unix; use `--shell powershell` for PS scripts).
  Pipe scripts via stdin: `echo script | sparkyctrl shell host`.
- `sparkyctrl read|write|ls|push|pull <host> ...` — binary-safe file operations.
- `sparkyctrl edit  <host> <remote> --old X --new Y [--all]`
  — surgical exact-string replacement; refuses on no-match or ambiguous match (atomic write).
  On failure returns a diagnostic showing what was searched for and the file head, so
  mismatches (CRLF vs LF, missing whitespace) are immediately visible without re-reading the file.
  Multiline or binaryish strings use `--old-file PATH` / `--new-file PATH` in place of `--old`/`--new`.
- `sparkyctrl info  <host>` — worker info.
- `sparkyctrl mcp` — stdio MCP server (see MCP section below).
- `sparkyctrl --version` — print the version.

`<host>` is a name from `~/.sparkyctrl/hosts.toml` or a literal `host:port`. Add `--json` to any
verb for raw output.

## MCP server (optional)

If your agent speaks MCP natively, `sparkyctrl mcp` exposes every verb as an MCP tool — no
CLI required. It's a local stdio server, not a network service. It reuses your existing
`~/.sparkyctrl/hosts.toml` and `SPARKYCTRL_TOKEN`.

**Wire into your agent's MCP config:**

```json
{
  "mcpServers": {
    "sparkyctrl": {
      "command": "sparkyctrl",
      "args": ["mcp"],
      "env": { "SPARKYCTRL_TOKEN": "<your-token>" }
    }
  }
}
```

**Tools exposed:** `exec`, `shell`, `read`, `write`, `edit`, `ls`, `info` — each takes a `host`
parameter plus verb-specific arguments. Token auth passes through automatically from the
environment.

## The audit log keeps receipts, not guarantees

Every request — including denied ones — is logged with its source IP and outcome. This tells you
**what happened.** It does **not** prevent anything, and anyone with `exec` access can truncate
the log through the same channel it audits. It is a flight recorder, not a seatbelt.

## Philosophy

A sharp tool for a trusted operator. No guardrails beyond an opt-in fence; the audit log keeps
receipts rather than preventing. Design details live in `docs/superpowers/specs/`.

## Status

v0.1.13 — public. Built, tested, and running on exactly one person's trusted LAN.
