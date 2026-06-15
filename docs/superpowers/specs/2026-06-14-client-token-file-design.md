# Client token file — keep the secret off the command line

**Date:** 2026-06-14
**Status:** Approved design, ready for implementation plan
**Component:** `internal/cli`, `internal/client` (client side only; worker untouched)

## Problem

The client only reads its auth token from `os.Getenv("SPARKYCTRL_TOKEN")` (`cli.go:106`,
inside `mkClient`). To use it, an operator must put the secret in an environment variable or a
`--token`-style argument — both of which render the token on the command line and in terminal
transcripts. The worker already reads its token from a *file* (`/etc/sparkyctrl/token`); the
client never got the same treatment.

## Goal

Let the client read a per-host token from a file on disk, so the secret never has to appear on
a command line. Keep the existing env var working for scripts/automation.

## Non-goals

- No `--token` flag on client verbs (would reintroduce the command-line leak).
- No keychain/secret-manager integration.
- No change to the wire protocol or the worker.

## Design

### The tokens file

A new optional file listing one token per host, in the *same* `name = "value"` format the
address book already uses:

```
# ~/.sparkyctrl/tokens
nas2   = "<token>"
unraid = "<token>"
```

Located exactly like `hosts.toml` is, via a new `tokensPath()` that mirrors `hostsPath()`:

1. `SPARKYCTRL_TOKENS` env var, if set.
2. `./tokens` in the current directory, if it exists.
3. `~/.sparkyctrl/tokens`.

### Parser reuse

`client.LoadHosts` is already a generic `name = "value"` reader (blank/`#` lines skipped,
quotes trimmed). Extract its body into an unexported `loadPairs(path)` and have both
`LoadHosts` and a new `LoadTokens(path)` call it. No new parsing logic, identical semantics.

### Token resolution

Replace the bare `os.Getenv` in `mkClient` with a testable resolver. Precedence:

1. `SPARKYCTRL_TOKEN` env — wins if set (backward-compatible; for scripts/automation/one-offs).
2. Per-host entry from the tokens file (looked up by the host *name* the user passed).
3. Empty string (the worker returns a clean 401 if it requires auth).

A host given as a raw `host:port` (not a name in the address book) has no name to look up, so
only the env var applies to it. This is acceptable and documented.

Signature (in `internal/cli`, since it already owns path resolution and env):

```go
func resolveToken(host string) string
```

It reads `SPARKYCTRL_TOKEN`; if empty, loads the tokens file via `tokensPath()` and returns
`tokens[host]`. `mkClient` calls it: `client.New(base, resolveToken(host))`.

### Permission warning

If the tokens file exists and is readable by group or other (mode bits `0o077` set), print one
non-fatal line to stderr: `sparkyctrl: warning: <path> is readable by others (chmod 600)`.
Done once per invocation, only when the file is actually used.

### Docs & hygiene

- Update the `Env:` help line in `cli.go` and the host-resolution help line to mention the
  tokens file and `SPARKYCTRL_TOKENS`.
- Add a `tokens.example` file at repo root.
- Add `tokens` and `.sparkyctrl/tokens` to `.gitignore` so a real one is never committed.
- Note the file in `README.md` client-auth wording.

## Testing

Unit tests (Go, table-driven where natural):

- `LoadTokens`: parses `name = "token"` pairs; missing file → empty map, no error; ignores
  blanks/comments; trims quotes. (Mirror existing hosts parser tests.)
- `loadPairs` shared behavior preserved: existing `LoadHosts` tests still pass unchanged.
- `resolveToken` precedence: env set → returns env regardless of file; env empty + file has
  host → returns file token; env empty + file missing host → empty; unknown/raw host → env-only.
- Permission warning: file with `0o644` triggers the warning string; `0o600` does not.
  (Test the predicate function, not stderr plumbing.)

## Rollout

1. Implement + tests on a branch.
2. Create `~/.sparkyctrl/tokens` on the control box with the nas2 entry; remove the habit of
   exporting `SPARKYCTRL_TOKEN` inline.
3. Verify `sparkyctrl info nas2` works with no token on the command line.

## Open items (resolved by operator)

- Token model: **one token per host** (sidecar tokens file), not a single shared token.
