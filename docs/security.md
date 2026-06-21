# Security model

Read this before you install. sparkyctrl is a deliberate remote-root tool, and its safety comes
entirely from *where* you run it, not from guardrails in the code.

## The one rule

**If this port touches the internet, you have not deployed a tool — you have published a root
shell.** No exceptions. sparkyctrl is for a trusted LAN you physically control. The installers
go out of their way to avoid the internet (the Windows installer opens a Private-profile firewall
rule only), but nothing stops you from misconfiguring a router. Don't.

## Threat model

- **Trust boundary = your LAN.** The token keeps *honest* agents honest on that network. It is
  **a lock on the door, not a bank vault.** Anyone who can reach the port and holds the token
  gets what the worker can do.
- **`exec` and `shell` run as root, unfenced, by design.** That is the product. If you wouldn't
  hand the token to a stranger at a root prompt, this tool isn't for you.
- **The fence limits file verbs only.** `--fence DIR` confines `read`/`write`/`edit`/`ls` to a
  directory. It does **not** constrain `exec`/`shell` — those can still touch anything root can.
- **Hardened mode narrows the blast radius** (Linux/systemd): a dedicated unprivileged user, zero
  capabilities, read-only filesystem except the fence + audit log. `exec`/`shell` stop being
  root. Use it when you want command execution without full root.

## Authentication

Auth is a single shared token per worker, sent in the `X-Sparkyctrl-Token` header.

- **Worker side:** the token lives in a file the worker reads at startup —
  `/etc/sparkyctrl/token` on Linux, `C:\ProgramData\sparkyctrl\token.txt` on Windows. Override
  with `--token`, or turn auth off with `--no-auth`.
- **Client side:** put each host's token in a file so it never appears on a command line — see
  [configuration.md](configuration.md#tokens-file). `SPARKYCTRL_TOKEN` still works as an override.
- **`--no-auth`** disables the token entirely. Only on a network whose boundary *is* your security
  (physically controlled, no untrusted hosts). The worker prints a reminder when it listens on
  `0.0.0.0`.

Keep token files locked down (`chmod 600`). The client warns if its tokens file is readable by
others.

## The audit log keeps receipts, not guarantees

Every request — including denied ones — is logged with its source IP and outcome (`--audit FILE`).
This tells you **what happened**. It does **not** prevent anything, and anyone with `exec` access
can truncate the log through the same channel it audits. It is a flight recorder, not a seatbelt.

On Unraid the default audit path is in RAM (`/var/log/...`) and resets on reboot; point `--audit`
at a path on the array if you want it to persist.

### Tamper-evident chain (worker, since v0.1.16)

Passing `--audit <path>` auto-generates an HMAC-SHA256 key (or reuses one
already at `<path>.key`) and chains every record with a `prev_hash` + `hmac`
pair. Run `sparkyctrl verify <path>` to confirm the log has not been edited,
truncated, or reordered since it was written:

| Exit | Meaning |
|---|---|
| 0 | chain intact |
| 1 | tampered (line number printed) |
| 2 | log contains legacy un-chained records, or no key was supplied |

The chain only proves **that** tampering happened, not what the original
content was. To survive a worker compromise you still need **off-box
forwarding** (syslog, journald, append-only collector on a different host) so
the attacker cannot reach the sink. That is a separate, larger feature —
this chain is a defense-in-depth stopgap that catches casual `truncate -s0`
attempts and post-incident forensics, not a determined attacker with root.

## Why no shell is itself a safety feature

`exec` sends a structured argument array straight to the OS `exec` call — no shell re-parses it.
That removes an entire class of catastrophe: a stray backtick or unquoted `$(...)` can't be
re-interpreted into a destructive command on the way to the target. `shell` exists when you
*want* a shell (pipes, globs), and it is logged explicitly as such.
