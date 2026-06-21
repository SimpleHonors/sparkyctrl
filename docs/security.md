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

## Why no shell is itself a safety feature

`exec` sends a structured argument array straight to the OS `exec` call — no shell re-parses it.
That removes an entire class of catastrophe: a stray backtick or unquoted `$(...)` can't be
re-interpreted into a destructive command on the way to the target. `shell` exists when you
*want* a shell (pipes, globs), and it is logged explicitly as such.

## Release integrity (signatures + checksums)

Every installer and every `sparkyctrl upgrade` self-update verifies the downloaded artifact
against **two** independent integrity checks:

1. **Minisign signature** against the pinned release public key at `deploy/sparkyctrl-release.pub`
   (also compiled into the binary at `internal/upgrade/sigverify.go:MinisignPublicKey` so the
   `upgrade` path needs no key file on disk). The signature is checked first, before any hash
   comparison — a release whose `.minisig` doesn't match the pinned key is refused outright.
2. **SHA-256** against the published `SHA256SUMS` (also signed by the same minisign key, so a
   tampered sums file is caught by the signature check above even if the sums file's hash matches
   the binary's).

The two together close the gap that "download a binary and `chmod +x` it" used to leave:
checksum-only proves consistency with what the publisher hashed, but it does *not* prove the
publisher is who you think they are. A signature does. If you want to trust the pinned key,
`curl -fsSL https://raw.githubusercontent.com/SimpleHonors/sparkyctrl/master/deploy/sparkyctrl-release.pub`
and store it on the target host (or pass `--binary <local-file>` to skip the network verify
entirely for offline installs).

**Why minisign and not cosign/sigstore?** Minisign is a single ~50 KB tool with an offline
verify (no Rekor/Fulcio calls, no network dependency at verify time). The pinned-key model
means a repo compromise can be detected by operators comparing the on-disk public key against
their own copy; the same key file ships at `deploy/sparkyctrl-release.pub` and as the constant
`MinisignPublicKey` in the binary. To rotate, generate a fresh keypair (`minisign -G -p new.pub
-s new.key`), update both the file and the constant in a new release, and re-sign.

**The verification path refuses to silently downgrade.** A release missing the `.minisig`
artifact is rejected (the installer dies with a clear message; the Go `upgrade` returns an
error). There is no `--allow-unsigned` or env var to bypass — the threat model assumes the
attacker who can ship a binary can also remove the signature, so any opt-out re-opens the
original gap.

