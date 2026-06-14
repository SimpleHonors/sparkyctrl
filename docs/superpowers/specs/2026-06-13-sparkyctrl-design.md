# Sparkyctrl — Design Spec

**Date:** 2026-06-13
**Status:** Approved for planning
**Author:** Hermes Host (with operator)

## 1. Problem

AI agents driving remote machines over SSH suffer **command mangling**: a command is
built as a single text string and re-interpreted by every shell it passes through
(local shell → ssh transport → remote shell). Quotes, `$`, backslashes, globs, and
heredocs are eaten differently at each hop. This has caused destructive mistakes,
including wiped files, because a mangled argument changed what a command actually did.

The operator wants a single tool they can install on any LAN machine — bare Linux,
LXC, Docker, VMs, Windows, and an Unraid NAS — that lets an AI agent perform sysadmin
tasks and transfer files **reliably**, without the mangling class of bug, without an
authentication system, and **without adding meaningful context cost** to the agent
(the agent already loads many MCP tool schemas).

## 2. Goal & Non-Goals

### Goal
A small, single-binary service ("the worker") installable on any LAN host, plus a
slim CLI client the agent drives through the terminal, that together provide:
- **Mangle-proof command execution** (arguments passed as a structured array straight
  to the OS exec call — no shell on the default path).
- **File operations and transfer** (read, write, list, push, pull) — binary-safe.
- **Cross-platform** operation (Linux + Windows) from one codebase.
- **Near-zero persistent context cost** to the agent (CLI, not a loaded MCP server).

### Non-Goals (v1 — YAGNI)
- Streaming/live output (run-to-completion only for v1; clean path to add later).
- Full interactive PTY (no `top`, no mid-command prompts).
- Any authentication/identity system (LAN-only trust; optional shared token only).
- Internet/remote exposure or TLS.
- A persistent MCP server (explicitly avoided to protect context budget).
- Unraid GraphQL integration, Windows service installer (both possible later).

## 2a. Design philosophy — a sharp tool, not a nanny

sparkyctrl is built for a **single trusted operator on a trusted LAN**. Its guiding
principle: **"if you want to do stupid things, we won't stop you."** Many remote-admin
tools spend most of their complexity on guardrails, approval gates, and permission
systems precisely to prevent the operator from shooting their own foot. sparkyctrl
deliberately does not. That complexity is the very thing that makes new machines
painful to onboard, and it is unnecessary when the only user is the person who owns
every box on the wire.

What this means concretely:
- **No prevention, but we keep receipts.** The audit log (§6) is not a guardrail — it
  exists so that when something destructive *does* happen, there is an exact record of
  what was run. "We won't stop you" is paired with "we will always tell you what you
  did."
- **The fence is opt-in and operator-chosen** (§6). It is not the tool protecting the
  operator from themselves; it is the operator choosing to scope a particular worker
  (e.g. "the NAS worker only ever touches the data shares"). Off by default.
- **Therefore, "add a safety check here" is usually out of scope by design.** Review
  feedback that proposes blocking, confirming, or sandboxing an operation should be
  weighed against this philosophy — accountability (audit) and opt-in scoping (fence)
  are the only safety mechanisms; everything else trusts the operator.

This is a conscious trade-off, recorded here so it is not "fixed" later by someone who
mistakes the absence of guardrails for an oversight.

## 3. Core Design Decision — why mangling cannot happen

The default execution path accepts an **argv array**, e.g.
`["systemctl", "restart", "nginx"]`, and hands it directly to the OS process-exec
call (`exec.Command(argv[0], argv[1:]...)` in Go) with **no shell involved**. There is
no string for a shell to re-parse, so quoting/escaping/glob/heredoc mangling is
structurally impossible on this path.

A **separate, explicit `shell` operation** exists for the genuine cases that need shell
features (pipes, redirects, globs, `&&`). It runs a raw script through `/bin/sh` (Linux)
or PowerShell (Windows). It is a distinct operation, distinctly logged, so its use is
always intentional and visible. Safe by default; full power on demand.

## 4. Architecture

Two roles, **one binary** (`sparkyctrl`):

```
  Agent (terminal)            LAN                 Target machine
  ┌───────────────┐                              ┌──────────────────┐
  │ sparkyctrl <verb> │  ── HTTP/JSON over LAN ──▶   │  sparkyctrl serve     │
  │  (CLI client) │  ◀── structured result ──    │  (worker daemon)  │
  └───────────────┘                              │  exec / files     │
                                                 └──────────────────┘
```

- **Worker** (`sparkyctrl serve`): an HTTP/JSON daemon listening on a configurable port
  (default `7766`), bound to the LAN interface. Executes structured requests and
  returns structured results. Stateless except for an append-only audit log.
- **Client** (`sparkyctrl exec|shell|read|write|ls|push|pull|ping|info`): invoked by the
  agent through the normal terminal. Resolves a friendly host name to an address via a
  small local address book, makes the HTTP call, prints the result.

### Address book (client side)
A small config file, default `~/.sparkyctrl/hosts.toml`, mapping names to addresses:
```toml
# real addresses live here (gitignored); repo ships an example only
nas2   = "HOST:7766"
ct-x   = "HOST:7766"
```
So the agent types `sparkyctrl exec nas2 -- systemctl restart nginx`.

## 5. Wire Protocol (HTTP/JSON)

All request/response bodies are JSON. Binary file payloads stream as the raw body.

| Endpoint        | Request                                                      | Response |
|-----------------|-------------------------------------------------------------|----------|
| `GET  /v1/info` | —                                                           | `{os, arch, hostname, version, fence}` |
| `POST /v1/exec` | `{argv:[...], cwd?, env?{}, stdin?, timeout_sec?}`           | `{stdout, stderr, exit_code, duration_ms}` |
| `POST /v1/shell`| `{script, shell?("sh"\|"powershell"), cwd?, timeout_sec?}`   | `{stdout, stderr, exit_code, duration_ms}` |
| `POST /v1/ls`   | `{path}`                                                     | `{entries:[{name, size, mode, is_dir, mtime}]}` |
| `POST /v1/download?path=` | — (path in query)                                 | streamed file bytes (binary-safe) |
| `POST /v1/upload?path=&mode=` | raw body bytes (client→worker)                | `{status:"ok"}` |

**File transfer uses two endpoints, not four.** Transport-wise the spec's four
file verbs collapse to two: `download` (worker→client bytes) backs both the
`read` and `pull` CLI verbs; `upload` (client→worker bytes) backs both `write`
and `push`. The four friendly CLI verbs are preserved; only the wire surface is
consolidated (DRY). `path` (and `mode`, octal, for upload) travel as query
parameters; the file bytes stream as the raw request/response body.

- `exec`/`shell` never auto-spawn each other. `exec` never invokes a shell.
- Timeouts default to a sane value (e.g. 60s); long jobs pass an explicit `timeout_sec`.
- Exit codes propagate to the client process exit code.

## 6. Safety

- **argv default, no shell** — the foundational guarantee (§3).
- **Explicit, logged `shell`** — visible whenever used.
- **Optional path fence** — per-worker config `fence = "/mnt/user"`. When set, file
  operations (`ls/read/write/push/pull`) refuse any path outside the fence
  (after symlink/`..` resolution). Off by default; recommended for the NAS worker
  (fence to the data shares). Does not restrict `exec`/`shell` working dir in v1.
- **`exec`/`shell` + `cwd` are unfenced by design.** The fence governs only the
  file verbs (`ls/read/write/push/pull`); `exec`/`shell` and their `cwd` field are
  intentionally *not* fence-checked. An unrestricted `exec` can `cd` anywhere on
  its own, so fencing the working directory would be security theatre, not a
  boundary. Containment for `exec`/`shell` is the operator's responsibility and
  relies on **OS-level isolation** — run the worker inside a container with only
  the intended shares bind-mounted in. This is consistent with the "sharp tool,
  not a nanny" philosophy (§2a).
- **Bind address is the operator's choice** — the worker binds to whatever `--addr`
  the operator passes. The default is `0.0.0.0:7766` (all interfaces) to keep
  onboarding frictionless on a trusted LAN; the operator can pin a specific LAN IP
  with `--addr <lan-ip>:7766`. Consistent with the "sharp tool, not a nanny"
  philosophy, the worker does not refuse to bind a public interface — but the serve
  banner states plainly when it is listening on `0.0.0.0` (all interfaces) so the
  exposure is visible, never silent. The tool is intended for trusted LANs only and
  must not be exposed to the internet.
- **Optional shared token** — `X-Sparkyctrl-Token` header, enabled by config, off by
  default. Not an identity system; a trivial speed-bump for a trusted LAN.
- **Append-only audit log** — every request logged on the worker (timestamp, op,
  argv or script, paths, exit code), with `shell` calls clearly marked. This is the
  accountability trail that makes post-mortems possible after any future incident.

## 7. Context-cost strategy (a primary requirement)

- **No persistent MCP server.** The agent drives the CLI through the terminal, so no
  tool schemas are loaded into context every turn. This follows the operator's own
  documented rule: long-tail tools go through the CLI/`mcp2cli`, not a loaded MCP.
- **Discoverability** is solved by a **single line in the agent's global instructions
  (CLAUDE.md)** pointing at `sparkyctrl --help`, plus optionally a small skill that
  triggers on remote/NAS intent. One sentence of always-on context, no schemas.
- **Complements the existing filesystem MCP mesh.** For file *editing* on hosts already
  in that mesh, the agent keeps using it. Sparkyctrl's new value is **command execution**
  (the mesh has none) and reaching **hosts the mesh does not cover** (the NAS, Windows).
- The worker is also a plain REST API, so `mcp2cli` can call it ad hoc — but the
  `sparkyctrl` CLI is the everyday path.

## 8. Build & Install

- **Language:** Go. One static binary, no runtime dependencies. Cross-compiles to
  `linux/amd64`, `linux/arm64`, `windows/amd64` from one codebase
  (`GOOS=… GOARCH=… go build`).
- **Linux / LXC / VM:** copy the binary; run `sparkyctrl serve` directly or as a systemd
  unit (a unit file ships with the repo).
- **Unraid (NAS):** run as a **Docker container** (survives reboots; Unraid's OS is
  RAM-resident) using the `linux/amd64` binary, with the data shares bind-mounted and
  the fence set to the share root. (`/boot/config/go` script is a documented fallback.)
- **Windows:** run the `.exe` directly for v1; a service wrapper is a later add-on.

## 9. Testing

- **Mangling torture test (the headline test):** round-trip arguments and paths
  containing spaces, single/double quotes, `$`, backticks, `;`, `|`, `&`, `*`,
  newlines, and unicode through `exec`; assert each argument arrives byte-for-byte
  intact and that no shell expansion occurred.
- **exec vs shell distinction:** confirm `exec` does not expand globs/pipes; confirm
  `shell` does.
- **File round-trip:** push/pull and read/write of binary files; assert byte-exact.
- **Fence enforcement:** attempts to escape the fence via `..` and symlinks are refused.
- **Integration:** spin a worker on localhost, exercise every client verb against it.
- **Cross-compile smoke:** the Windows and arm64 binaries build cleanly in CI.

## 10. Out-of-scope follow-ups (noted, not built now)

Streaming output; PTY/interactive; Windows service installer; Unraid GraphQL
integration for NAS-native operations; TLS; multi-user/auth. Each is an additive
layer that does not require reworking the v1 protocol.

### v2: surgical file editing (requested 2026-06-13)

Add server-side surgical edit operations so the agent can modify remote files
mangle-free without a full read/rewrite round-trip:

- **`edit`** — exact-string replacement: `{path, old_string, new_string,
  replace_all?}`. By default require `old_string` to match exactly once
  (uniqueness check) and fail otherwise — same safety contract as the agent's
  native Edit tool. Bytes are matched literally; no regex, no shell.
- **`edit_lines`** — replace an inclusive line range: `{path, start, end,
  new_content}`. 1-indexed, validated against file length.
- Both are fence-checked (§6) and audited (§6) like every other file op, and
  exposed as CLI verbs (`sparkyctrl edit <host> ...`, `sparkyctrl edit-lines
  <host> ...`). New endpoint pair on the existing protocol — no rework of v1.
  Gets its own spec → plan → build cycle.
- **Research first:** this exact-string-replace-with-uniqueness-check pattern is
  well-established (e.g. Claude Code's Edit tool, aider search/replace blocks,
  diff/patch libraries). Survey existing implementations and reuse a proven
  algorithm rather than reinventing the matching/ambiguity logic.
