# Installation and Configuration Guide

## User Installation

### Homebrew (Linux, recommended)

```bash
brew install --cask xlionjuan/tap/searxng-mcp-go
```

*Currently supports Linux only (x86_64 and arm64). macOS is not yet supported.*

After installation, find the binary's absolute path — MCP clients often don't inherit your shell PATH:

```bash
which searxng-mcp-go
```

### Manual (Download from GitHub Releases)

1. Visit the [Releases page](https://github.com/xlionjuan/searxng-mcp-go/releases)
2. Download the tarball matching your architecture:
   - Linux amd64: `searxng-mcp-go_v<VERSION>_linux_amd64.tar.zst`
   - Linux arm64: `searxng-mcp-go_v<VERSION>_linux_arm64.tar.zst`
3. Extract and install:

```bash
tar --zstd -xvf searxng-mcp-go_v<VERSION>_linux_amd64.tar.zst
mkdir -p ~/.local/bin && mv searxng-mcp-go ~/.local/bin/
```

### Verify Installation

```bash
searxng-mcp-go --version
```

---

## Configuration

### SearXNG Instance

A SearXNG instance URL is **required** — there is no default. Set it via the `SEARXNG_URL` environment variable or `--searxng-url` CLI flag.

```bash
export SEARXNG_URL=https://your-searxng-instance.example.com
```

### Max Retries

The default retry count is 5 retries after the initial search attempt. Set `SEARXNG_MAX_RETRIES` to a non-negative integer (maximum 20); in CLI mode, `--max-retries` overrides the environment variable. Use `--max-retries=0` to disable retries in CLI mode.

### Timeout

The default timeout for search requests is 8 seconds. Set `SEARXNG_TIMEOUT` to a Go duration such as `8s`; in CLI mode, `--timeout` overrides the environment variable.

### Invalid Environment Variable Values

`SEARXNG_TIMEOUT` and `SEARXNG_MAX_RETRIES` accept a fixed format (a Go
duration and a non-negative integer, respectively). When a value is set
to something the server cannot parse — for example
`SEARXNG_TIMEOUT=abc` or `SEARXNG_MAX_RETRIES=-1` — the server writes a
warning line to stderr that names the offending variable and value, and
then silently falls back to the built-in default. The process
**continues running** and does not exit. In MCP stdio mode, most MCP
clients do not surface the stderr stream, so end users typically do
not see the warning at all.

If you need strict validation (for example in CI), prefer the
`--timeout` and `--max-retries` CLI flags instead of the environment
variables. An invalid value supplied on the command line is reported
on stderr, the CLI help is printed, and the process exits with a
non-zero status — no search request is issued.

> **Note:** If you provide a custom `HTTPClient` (for example, when using the library programmatically), the `Timeout` setting is ignored and the provided client is used as-is. Either set `Timeout` or supply a custom `HTTPClient`, not both.

### POST→GET Fallback

When a POST request receives `405 Method Not Allowed` or `501 Not Implemented`, the server automatically retries the `/search` request with GET, ensuring compatibility with SearXNG deployments that do not support POST search requests.

---

## MCP Server Configuration

### Basic Execution

In MCP mode, `searxng-mcp-go` is meant to be launched by an MCP client host (such as an AI agent framework), not run directly in a terminal. The client provides the configured environment variables, starts the process, and sends the required MCP JSON-RPC `initialize` message on stdin before tool calls are available.

The server uses stdio transport, meaning it communicates via stdin/stdout after the MCP client starts it.

### MCP Client Configuration

Configure the MCP server by adding it to your client's `mcpServers`. Use the **absolute path** to the binary — MCP clients often don't inherit your shell PATH.

```bash
which searxng-mcp-go
```

```json
{
  "mcpServers": {
    "searxng": {
      "command": "/home/linuxbrew/.linuxbrew/bin/searxng-mcp-go",
      "env": {
        "SEARXNG_URL": "https://your-searxng-instance.example.com",
        "SEARXNG_TIMEOUT": "8s",
        "SEARXNG_MAX_RETRIES": "5",
        "DEBUG": "1"
      }
    }
  }
}
```

When debug mode (`DEBUG=1`) is enabled, the server logs HTTP request/response details, including query text. On startup the server emits the following warning to stderr:

```text
WARN debug mode logs search queries and HTTP requests in plain text; avoid sensitive searches
```

Most MCP clients do not surface stderr to the user, so do not rely on the warning to communicate the privacy risk. Avoid enabling debug mode with sensitive queries.

### CLI Mode Configuration

When running in CLI mode (with a query argument), command-line flags can be used directly:

```bash
# Custom SearXNG server via flag
searxng-mcp-go "query" --searxng-url=https://your-searxng-instance.example.com

# Debug mode via flag
searxng-mcp-go "query" --debug

# HTTP timeout and retry tuning via flags
searxng-mcp-go "query" --timeout=8s --max-retries=5
```

---

## Related Documentation

- [README.md](../README.md) — Quick start and usage overview
- [MCP Tools Reference](MCP_TOOLS.md) — Full tool parameters and response format
