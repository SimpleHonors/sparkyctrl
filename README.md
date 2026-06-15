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

---

## Contents

- [What this is](#what-this-is)
- [Quickstart](#quickstart) — install and run a command in under a minute
- [Command cheat-sheet](#command-cheat-sheet)
- [Deployment](#deployment) — systemd · Unraid · Docker
- [Documentation](#documentation)
- [Security](#security)
- [Philosophy & status](#philosophy--status)

---

## What this is

sparkyctrl lets an AI agent run commands and move files on another machine with **no shell in
the middle to mangle them.** Commands go out as a structured argument array straight to the OS
`exec` call. This prevents the quoting and escaping disasters that happen when an agent drives a
box over SSH — one stray backtick or unquoted `$(...)` can't turn "list the logs" into something
that halts the machine.

It's a sharp tool built for a specific operator on a specific network. `exec` and `shell` run as
**root**, unfenced, by design. File verbs (`read`, `write`, `edit`, `ls`) can be confined to a
directory via an opt-in fence. Auth is a shared token. Read the [security model](docs/security.md)
before you install — it is not optional reading for this tool.

## Quickstart

**1. Install the worker** on the machine you want to drive (Linux, runs as root, starts listening):

```sh
curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/install.sh \
  | sudo bash -s -- --start
```

The installer prints a token and a matching client snippet. (Windows, Docker, fence options, and
hardened mode are in the [installation guide](docs/getting-started.md).)

**2. Set up the client** on the agent side — name the host and store its token in a file so the
secret never lands on a command line:

```sh
mkdir -p ~/.sparkyctrl
echo 'nas2 = "192.0.2.50:7766"' > ~/.sparkyctrl/hosts.toml      # name -> address
printf 'nas2 = "PASTE_TOKEN"\n' > ~/.sparkyctrl/tokens          # name -> token
chmod 600 ~/.sparkyctrl/tokens
```

**3. Drive it** — no token typed, no shell to mangle the command:

```sh
sparkyctrl info nas2
sparkyctrl exec nas2 -- uname -a
sparkyctrl push nas2 ./build.tar.gz /tmp/build.tar.gz
```

## Command cheat-sheet

| Command | What it does |
|---|---|
| `sparkyctrl exec  <host> -- <argv...>` | Run a command, mangle-proof; stdout/stderr/exit code returned |
| `sparkyctrl shell <host> <script>` | Explicit, logged shell path for pipes/globs/redirects |
| `sparkyctrl ls    <host> <path>` | List a directory |
| `sparkyctrl read  <host> <remote>` | Print a remote file to stdout (binary-safe) |
| `sparkyctrl write <host> <remote>` | Write stdin to a remote file |
| `sparkyctrl push  <host> <local> <remote>` | Upload a file |
| `sparkyctrl pull  <host> <remote> <local>` | Download a file |
| `sparkyctrl edit  <host> <remote> --old S --new S [--all]` | Exact-string edit; refuses on no/ambiguous match |
| `sparkyctrl info  <host>` | Worker info (OS/arch/version/fence) |
| `sparkyctrl mcp` | Run as a stdio MCP server |

Add `--json` to any client verb for raw output. Full reference with examples and flags:
[docs/commands.md](docs/commands.md).

## Deployment

| Target | One-liner | Guide |
|---|---|---|
| **Linux (systemd)** | install script registers an auto-restart service | [deployment.md](docs/deployment.md#systemd) |
| **Unraid** | reboot-persistent worker that survives the RAM-disk wipe | [deployment.md](docs/deployment.md#unraid) · [deploy/unraid/](deploy/unraid/README.md) |
| **Docker** | scratch image for the fenced file-serving use case | [deployment.md](docs/deployment.md#docker) |

## Documentation

- [**Getting started**](docs/getting-started.md) — install the worker (Linux/Windows/Docker) and client, run your first command, troubleshooting.
- [**Deployment**](docs/deployment.md) — systemd, Unraid reboot-persistence, Docker.
- [**Security model**](docs/security.md) — threat model, token auth, the fence, `--no-auth`, the audit log.
- [**Configuration**](docs/configuration.md) — `hosts.toml`, the tokens file, environment variables, worker flags.
- [**Commands**](docs/commands.md) — every verb with copy-paste examples and full flags.
- [**MCP server**](docs/mcp.md) — expose every verb as MCP tools.

## Security

sparkyctrl gives agents root on your machines. The token keeps honest agents honest on a trusted
LAN, and that's its entire job. If that sentence doesn't sit right, don't install it.

- **If this port touches the internet, you've published a root shell.** No exceptions.
- Token auth is **a lock on the door, not a bank vault.** Disable it (`--no-auth`) only on a network you physically control.
- `exec` and `shell` are **unfenced by design.** The path fence limits file verbs only.
- The audit log is **a flight recorder, not a seatbelt** — it records what happened, it prevents nothing.

The full reasoning lives in [docs/security.md](docs/security.md). Read it.

## Philosophy & status

A sharp tool for a trusted operator. No guardrails beyond an opt-in fence; the audit log keeps
receipts rather than preventing. Design notes live in `docs/superpowers/specs/`.

**v0.1.13** — built, tested, and running on a trusted LAN.
