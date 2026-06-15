# MCP server

If your agent speaks [MCP](https://modelcontextprotocol.io) natively, `sparkyctrl mcp` exposes
every verb as an MCP tool — no CLI wrapping required. It's a **local stdio server, not a network
service**, and it reuses your existing client configuration (`~/.sparkyctrl/hosts.toml`, the
tokens file, and `SPARKYCTRL_TOKEN`).

## Wiring it into an agent

```json
{
  "mcpServers": {
    "sparkyctrl": {
      "command": "sparkyctrl",
      "args": ["mcp"]
    }
  }
}
```

That's the clean setup: with a [tokens file](configuration.md#tokens-file) in place, the MCP
server resolves each host's token from disk — nothing secret in the config.

If you must pass a token through the environment instead (single shared token, automation):

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

## Tools exposed

`exec`, `shell`, `read`, `write`, `edit`, `ls`, `info` — each takes a `host` parameter plus the
verb-specific arguments (see [commands.md](commands.md)). Token auth is applied automatically per
host from your client configuration.

## Notes

- The MCP server is stdio only; it does not open a port. The network hop is still the worker on
  the target machine.
- The same trust boundary applies: the hosts it can reach are the ones in your address book, and
  the worker still runs commands as root. Read [security.md](security.md).
