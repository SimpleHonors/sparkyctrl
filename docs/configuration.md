# Configuration

Everything the client and worker read: the address book, the tokens file, environment variables,
and worker flags.

- [Address book (`hosts.toml`)](#address-book)
- [Tokens file](#tokens-file)
- [Environment variables](#environment-variables)
- [Worker flags (`serve`)](#worker-flags)
- [File resolution order](#file-resolution-order)

## Address book

A minimal `name = "host:port"` file. Blank lines and `#` comments are ignored; surrounding quotes
are trimmed.

```toml
# ~/.sparkyctrl/hosts.toml
nas2 = "192.0.2.50:7766"
web  = "192.0.2.60:7766"
```

Any verb then takes the name: `sparkyctrl info nas2`. A literal `host:port` that isn't in the book
works too: `sparkyctrl info 192.0.2.50:7766`.

## Tokens file

Per-host client tokens, **same format** as the address book. This is how you keep the secret off
the command line: the client looks up the token by host name.

```toml
# ~/.sparkyctrl/tokens   (chmod 600)
nas2 = "a1b2c3...the-nas2-token"
web  = "d4e5f6...the-web-token"
```

```sh
printf 'nas2 = "PASTE_TOKEN"\n' > ~/.sparkyctrl/tokens
chmod 600 ~/.sparkyctrl/tokens
```

- One distinct token per host is recommended, so compromising one box doesn't expose another.
- The client warns to stderr if this file is readable by group/other.
- A starter file ships as [`tokens.example`](../tokens.example).

## Environment variables

| Variable | Used by | Meaning |
|---|---|---|
| `SPARKYCTRL_HOSTS` | client | Path to the address book (overrides default lookup) |
| `SPARKYCTRL_TOKENS` | client | Path to the tokens file (overrides default lookup) |
| `SPARKYCTRL_TOKEN` | client | A token that **overrides** the tokens file (for scripts/automation). Appears on the process environment, so prefer the file for interactive use. |

## Worker flags

`sparkyctrl serve [flags]`:

| Flag | Default | Meaning |
|---|---|---|
| `--addr H:P` | `0.0.0.0:7766` | Listen address. `0.0.0.0` = all interfaces (trusted LAN only). |
| `--fence DIR` | none (full access) | Confine **file verbs** to `DIR`. Does not affect `exec`/`shell`. |
| `--token T` | reads token file | Explicit token override. **Warning:** this puts the secret in the process command line (visible via `ps`). Prefer the token file or `SPARKYCTRL_TOKEN` env var. |
| `--no-auth` | off | Disable token auth entirely. Only on a network you physically control. |
| `--audit FILE` | none | Append every request (incl. denied) with source IP + outcome. |

The worker's default token file is `/etc/sparkyctrl/token` (Linux) or
`C:\ProgramData\sparkyctrl\token.txt` (Windows).

## File resolution order

Both the address book and the tokens file resolve the same way:

1. The matching environment variable (`SPARKYCTRL_HOSTS` / `SPARKYCTRL_TOKENS`), if set.
2. A file in the current directory (`hosts.toml` / `tokens`), if it exists.
3. `~/.sparkyctrl/hosts.toml` / `~/.sparkyctrl/tokens`.

Token precedence for a given host: `SPARKYCTRL_TOKEN` (if set) → the per-host entry in the tokens
file → none (the worker returns `401` if it requires auth).
