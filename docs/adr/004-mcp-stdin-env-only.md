# ADR-001: MCP Stdin Mode Rejects All CLI Arguments

- **Date**: 2026-04-19
- **Status**: Accepted

## Context

The searxng-mcp-go server operates in two modes:

1. **CLI mode**: Interactive command-line usage with flags like `--query`, `--json`, `--searxng-url`, etc.
2. **MCP stdin mode**: Headless operation communicating via stdio with MCP clients (e.g., Claude Desktop).

Previously, MCP stdin mode silently accepted CLI flags (e.g., `--searxng-url`, `--language`, `--debug`) and applied them alongside environment variables. This created several problems:

- **Configuration ambiguity**: Users could not tell whether a setting came from a CLI flag or an environment variable, making debugging MCP connection issues harder.
- **Inconsistent UX across MCP clients**: Different MCP client launchers (Claude Desktop, Cursor, etc.) may or may not pass CLI arguments, leading to unpredictable behavior.
- **Security surface**: Accepting arbitrary CLI arguments in a headless stdin mode increases the attack surface without clear benefit.
- **Convention mismatch**: The MCP ecosystem convention is that stdio servers are configured entirely through environment variables, not command-line flags. CLI flags are for human-facing interactive use.

## Decision

In MCP stdin mode, the server **rejects all command-line arguments** with a clear error message. All configuration must come from environment variables:

| Environment Variable | Purpose                      |
|----------------------|------------------------------|
| `SEARXNG_URL`        | SearXNG server URL           |
| `DEBUG`              | Set to `1` for debug logging |

The only exceptions are `--help` and `--version`, which trigger CLI mode (informational output) and are not considered MCP stdin mode arguments.

### Detection Logic

```
MCP stdin mode = no --query, no --json, no --help, no --version, no positional args
```

If any flags are present when in MCP stdin mode, the server exits with:

```
ERROR: MCP stdin mode does not accept command-line arguments; use environment variables (SEARXNG_URL, DEBUG) instead
```

### CLI mode is unaffected

All existing CLI flags continue to work normally. Only the MCP stdin code path enforces the env-only rule.

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
