# Documentation overhaul — lean README + docs/ guides

**Date:** 2026-06-14
**Status:** Approved (operator delegated), implementing
**Scope:** `README.md` + new `docs/` user guides. Private repo; never published upstream.

## Problem

The README is a single 156-line prose file. It does not cover the two newest features (the
per-host client **tokens file** and the **Unraid reboot-persistent deploy path**), there are no
separate user-facing docs (only internal design specs under `docs/superpowers/`), and the
landing page is hard to skim — no quickstart, no command cheat-sheet, no links out.

## Goal

Make the repo present well online: a scannable README that links to focused `docs/` guides,
full coverage of every feature (including the new ones), and concrete copy-paste examples plus
a complete command/config reference. Keep the existing sharp, opinionated voice.

## Approach (chosen)

Lean README + a `docs/` folder of focused guides (approach B). Not one giant README (A), not a
docs-site build pipeline (C — overkill for a private repo).

## Deliverables

### README.md (landing page)

Restructured, skimmable, voice preserved:

1. Title + one-line tagline.
2. The security warning box — **kept prominent** (most important content).
3. "What this is" — 3–4 tight sentences.
4. **Quickstart** — install worker, install client, run one command; one screen.
5. **Command cheat-sheet** — a table: verb → one-line description.
6. **Deployment options** — systemd · Unraid · Docker, each one line → links to `docs/`.
7. **Docs index** — links to every `docs/` page.
8. Philosophy + Status — short.

### docs/ guides (6 pages)

```
docs/
├── getting-started.md   # install worker (Linux/systemd, Windows, Docker) + client; first command; a short Troubleshooting section
├── deployment.md        # systemd; Unraid reboot-persistent (-> deploy/unraid/README.md); Docker file-serving
├── security.md          # threat model; token auth; fence; --no-auth; never-expose-to-internet; audit log honesty
├── configuration.md     # hosts.toml; tokens file (~/.sparkyctrl/tokens, per-host); env vars (HOSTS/TOKENS/TOKEN); serve flags
└── commands.md          # every verb with copy-paste examples + full flag reference
└── mcp.md               # MCP server usage (sparkyctrl mcp)
```

Troubleshooting lives as a section inside `getting-started.md`; promote to its own page only if
it grows. New features get first-class coverage: tokens file in `configuration.md` +
`security.md`; Unraid in `deployment.md`.

### Cross-cutting

- Voice: opinionated and direct, tighter sentences, more code blocks, fewer walls of prose.
- Badges: minimal or none (private repo; CI/coverage badges add little). A plain tagline line.
- Every internal `docs/` link must resolve (relative paths).
- All command examples use placeholder hosts (`nas2`, `web`) and **never** real tokens or
  private IPs.

## Non-goals

- No docs-site generator, no hosting, no CI for docs.
- No new product features; documentation only (may fix small doc-facing inaccuracies found
  while writing).

## Verification

- Manual read-through: README skims in under a minute; every section has a home.
- Link check: every `docs/` link and `deploy/` reference resolves.
- Sanitization re-audit (token / 10.0.0.x / /root / internal hostnames / email) over the full
  tracked tree before pushing to origin.
- `go build ./...` still green (docs-only changes shouldn't affect it, but confirm).

## Rollout

1. (done) Merge `unraid-persistence` into master so `deploy/unraid/` is documentable.
2. Rewrite README; write the six `docs/` pages.
3. Link + leak audit.
4. Commit; push to origin master (never upstream).
