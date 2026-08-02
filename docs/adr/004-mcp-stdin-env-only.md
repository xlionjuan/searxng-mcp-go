# ADR-004: MCP Stdin Mode Is Detected From Input

- **Date**: 2026-04-19
- **Status**: Accepted

## Context

The searxng-mcp-go server operates in two modes:

1. **CLI mode**: Interactive command-line usage with flags like `--query`, `--json`, `--searxng-url`, etc.
2. **MCP stdin mode**: Headless operation communicating via stdio with MCP clients (e.g., Claude Desktop).

The server now detects MCP stdin mode by inspecting the first line of stdin for a supported MCP first message. The preflight accepts a legacy `initialize` request, a `server/discover` request, or a stateless request carrying `params._meta["io.modelcontextprotocol/protocolVersion"]`. It is a bounded structural gate; complete protocol metadata validation remains with the MCP SDK. If the process was launched with CLI arguments, it runs in CLI mode instead. This avoids treating normal command-line usage as MCP input and matches the actual startup flow:

## Detection Flow

1. CLI arguments are parsed first.
2. Any argument selects CLI mode, including positional queries, search flags, configuration flags such as `--searxng-url` or `--timeout`, `--debug`, `--help`, and `--version`.
3. Otherwise the process reads the first line of stdin and requires one of the supported MCP first-message shapes: legacy `initialize`, `server/discover`, or stateless protocol-version metadata in `params._meta["io.modelcontextprotocol/protocolVersion"]`.
4. If stdin does not look like MCP input, startup fails with an MCP-specific error.

## Decision

In MCP stdin mode, configuration comes from environment variables and MCP input, not from standalone CLI search flags. The supported environment variables are:

| Environment Variable | Purpose |
|----------------------|---------|
| `SEARXNG_URL` | SearXNG server URL |
| `DEBUG` | Set to `1` for debug logging |
| `SEARXNG_TIMEOUT` | Sets the search timeout (default 8s) |
| `SEARXNG_MAX_RETRIES` | Sets the max retry count (default 5) |
| `SEARXNG_ALLOW_GET_FALLBACK` | Set to `1` to opt in to POST→GET fallback |

`--help` and `--version` trigger CLI mode for informational output. Like every
other CLI flag, they are not accepted in MCP stdin mode.

The JSON first-message wire bytes have a fixed 1 MiB transport bound,
independent of the message type, payload, or configuration. An optional
trailing newline delimiter is excluded from the count. The preflight only
checks safe structure; the MCP SDK validates the complete protocol metadata
after startup.

### Detection Logic

```
MCP stdin mode = no command-line arguments
```

If stdin does not contain a supported MCP first message, the server exits with:

```
stdin does not contain a valid MCP first message
```

### CLI mode is unaffected

All existing CLI flags continue to work normally. Only the MCP stdin code path enforces the stdin protocol check.

## Consequences

### Positive

- **Clear configuration contract**: MCP users know exactly where to configure the server (environment variables only).
- **Predictable behavior**: No silent failures or ignored flags in MCP mode.
- **Better error messages**: Users who accidentally pass flags get an immediate, actionable error instead of silent misconfiguration.
- **Easier debugging**: When troubleshooting MCP issues, the configuration source is unambiguous.
- **Convention compliance**: Aligns with how other MCP servers are typically configured.

### Negative

- **Migration required**: Existing setups that pass CLI flags in MCP mode (e.g., `./searxng-mcp-go --searxng-url https://example.com`) must migrate to environment variables.
- **Less flexible**: Users who preferred CLI flags for MCP configuration lose that option. However, environment variables are equally powerful and more portable across MCP client launchers.
