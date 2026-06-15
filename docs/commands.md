# Command reference

Every client verb, with examples. `<host>` is a name from the [address book](configuration.md)
or a literal `host:port`. Add `--json` to any client verb for raw JSON output.

- [exec](#exec) · [shell](#shell) · [ls](#ls) · [read](#read) · [write](#write) · [push](#push) · [pull](#pull) · [edit](#edit) · [info](#info) · [mcp](#mcp)
- [serve](#serve) (worker)

## exec

Run a command with **no shell in the middle**. Arguments after `--` go straight to the OS `exec`
call as an argument array, so quoting/escaping can't be mangled. Returns stdout, stderr, and the
exit code.

```sh
sparkyctrl exec nas2 -- uname -a
sparkyctrl exec nas2 -- ls -la "/path/with spaces"
sparkyctrl exec nas2 -- systemctl restart nginx
```

Everything after `--` is the literal argv — no shell features (no pipes, globs, `$VAR`, `>`). For
those, use [`shell`](#shell).

## shell

Run a script through a **real shell** (logged as such). Use when you actually want pipes, globs,
redirects, or environment expansion.

```sh
sparkyctrl shell nas2 'ps aux | grep -c sparkyctrl'
echo 'for f in /tmp/*.log; do wc -l "$f"; done' | sparkyctrl shell nas2
```

Default shell is `/bin/sh` on Unix, `cmd` on Windows. Use `--shell powershell` for PowerShell
scripts on Windows.

## ls

List a directory (subject to the worker's fence, if any).

```sh
sparkyctrl ls nas2 /var/log
```

## read

Print a remote file to stdout (binary-safe).

```sh
sparkyctrl read nas2 /etc/hostname
sparkyctrl read nas2 /tmp/photo.jpg > photo.jpg
```

## write

Write stdin to a remote file (binary-safe, atomic).

```sh
echo "hello" | sparkyctrl write nas2 /tmp/greeting.txt
sparkyctrl write nas2 /tmp/blob.bin < local.bin
```

## push

Upload a local file to the worker.

```sh
sparkyctrl push nas2 ./build.tar.gz /tmp/build.tar.gz
```

## pull

Download a remote file to the local machine.

```sh
sparkyctrl pull nas2 /var/log/syslog ./syslog.txt
```

## edit

Surgical exact-string replacement. Refuses on **no match** or **ambiguous match** (writes
atomically). On failure it returns a diagnostic showing what was searched for and the file head,
so mismatches (CRLF vs LF, missing whitespace) are visible without re-reading the file.

```sh
sparkyctrl edit nas2 /etc/app.conf --old "debug = false" --new "debug = true"
sparkyctrl edit nas2 /etc/hosts --old "127.0.0.1 old" --new "127.0.0.1 new" --all
```

| Flag | Meaning |
|---|---|
| `--old S` / `--new S` | The exact string to find / its replacement |
| `--all` | Replace every occurrence (default: refuse if more than one) |
| `--old-file PATH` / `--new-file PATH` | Use file contents instead of `--old`/`--new` (for multiline or binaryish strings) |

## info

Show worker OS, architecture, version, and fence.

```sh
sparkyctrl info nas2
# linux/amd64  host=NAS2  version=0.1.13  fence="none (FULL access)"
```

## mcp

Run as a stdio MCP server exposing every verb as a tool. See [mcp.md](mcp.md).

```sh
sparkyctrl mcp
```

## serve

The worker. Full flag table in [configuration.md](configuration.md#worker-flags).

```sh
sparkyctrl serve --addr 0.0.0.0:7766 --audit /var/log/sparkyctrl-audit.log
sparkyctrl serve --fence /srv/share --no-auth        # fenced file server, no token
```

## Other

```sh
sparkyctrl --version      # print the version
```
