# ADR-004: MCP Stdin Mode Is Detected From Input

- **Date**: 2026-04-19
- **Status**: Accepted

## Context

The searxng-mcp-go server operates in two modes:

1. **CLI mode**: Interactive command-line usage with flags like `--query`, `--json`, `--searxng-url`, etc.
2. **MCP stdin mode**: Headless operation communicating via stdio with MCP clients (e.g., Claude Desktop).

The server now detects MCP stdin mode by inspecting stdin for a valid MCP `initialize` message. If the process was launched with CLI arguments, it runs in CLI mode instead. This avoids treating normal command-line usage as MCP input and matches the actual startup flow:

## Detection Flow

1. CLI arguments are parsed first.
2. Any CLI arguments, `--help`, `--version`, or explicit search flags select CLI mode.
3. Otherwise the process reads stdin and requires the first message to be a valid MCP `initialize` request.
4. If stdin does not look like MCP input, startup fails with an MCP-specific error.

## Decision

In MCP stdin mode, configuration comes from environment variables and MCP input, not from standalone CLI search flags. The supported environment variables are:

|| Environment Variable | Purpose                      |
||----------------------|------------------------------|
|| `SEARXNG_URL`        | SearXNG server URL           |
|| `DEBUG`              | Set to `1` for debug logging |
|| `SEARXNG_TIMEOUT`    | Sets the search timeout (default 8s) |
|| `SEARXNG_MAX_RETRIES` | Sets the max retry count (default 5) |

The only exceptions are `--help` and `--version`, which trigger CLI mode (informational output) and are not considered MCP stdin mode arguments.

### Detection Logic

```
MCP stdin mode = no --query, no --json, no --help, no --version, no positional args
```

If stdin does not contain a valid MCP `initialize` message, the server exits with:

```
ERROR: stdin does not contain a valid MCP initialize message
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
