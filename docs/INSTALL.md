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

> **Warning:** Using `http://` for a non-private SearXNG host (any hostname that is not `localhost`, a loopback address, or an RFC 1918 private IP range) triggers a non-suppressible warning on every invocation: *"Using HTTP for non-private host. Search queries may be transmitted in clear text. Search results could be intercepted and modified by a MITM attacker."* Prefer HTTPS unless the SearXNG instance is on a private/local network.

### Max Retries

The default retry count is 5 retries after the initial search attempt. Set `SEARXNG_MAX_RETRIES` to a non-negative integer (maximum 20); in CLI mode, `--max-retries` overrides the environment variable. Use `--max-retries=0` to disable retries in CLI mode.

### Timeout

The default timeout for search requests is 8 seconds. Set `SEARXNG_TIMEOUT` to a Go duration such as `8s`; in CLI mode, `--timeout` overrides the environment variable.

### POST to GET Fallback

Search requests use POST by default. If POST `/search` returns
`405 Method Not Allowed` or `501 Not Implemented`, the server returns an
actionable error by default instead of silently retrying with GET.

Set `SEARXNG_ALLOW_GET_FALLBACK=1` only when you need compatibility with a
deployment that rejects POST and you accept the risk: GET sends search
parameters in the URL, so queries may be recorded by SearXNG, reverse proxies,
CDNs, or access logs. When enabled, the server logs a warning on startup and
again whenever the fallback is used. In CLI mode, `--allow-get-fallback`
overrides the environment variable.

### Invalid Environment Variable Values

`SEARXNG_TIMEOUT`, `SEARXNG_MAX_RETRIES`, and
`SEARXNG_ALLOW_GET_FALLBACK` accept fixed formats (a Go duration, a
non-negative integer, and `0`/`1`, respectively). When a value is set to
something the server cannot parse — for example `SEARXNG_TIMEOUT=abc`,
`SEARXNG_MAX_RETRIES=-1`, or `SEARXNG_ALLOW_GET_FALLBACK=true` — the server
writes a warning line to stderr that names the offending variable and value,
and then the server falls back to whichever value takes precedence:
if a corresponding CLI flag is set (e.g. `--timeout 30s`,
`--max-retries 10`, `--allow-get-fallback`), that value is used;
otherwise the built-in default is used. The process
**continues running** and does not exit. In MCP stdio mode, most MCP clients do
not surface the stderr stream, so end users typically do not see the warning at
all.

If you need strict validation (for example in CI), prefer the
`--timeout` and `--max-retries` CLI flags instead of the environment
variables. Invalid CLI values follow two distinct paths depending on
where they are rejected:

- **Flag parse errors** — values the command-line parser cannot
  interpret at all (for example `--timeout=abc`) are caught before
  any search flow runs. The error is reported on stderr, the CLI help
  is printed, and the process exits with a non-zero status. No search
  request is issued.
- **Semantic validation errors** — values that parse successfully but
  fall outside the documented range (for example `--max-retries=-1`,
  or a value above the maximum allowed retries) are accepted by the
  parser and only fail later, during configuration validation inside
  the CLI search flow. The error is reported on stderr, the process
  exits with a non-zero status, and **no CLI help is printed**. No
  search request is issued.

> **Note:** If you provide a custom `HTTPClient` (for example, when using the library programmatically), the `Timeout` setting is ignored and the provided client is used as-is. Either set `Timeout` or supply a custom `HTTPClient`, not both.

---

## MCP Server Configuration

### Basic Execution

In MCP mode, `searxng-mcp-go` is meant to be launched by an MCP client host (such as an AI agent framework), not run directly in a terminal. The client provides the configured environment variables, starts the process, and sends the required MCP JSON-RPC `initialize` message on stdin before tool calls are available.

The server uses stdio transport, meaning it communicates via stdin/stdout after the MCP client starts it.

#### Stdin Validation

The server reads the first line of stdin and validates it is a JSON-RPC 2.0 `initialize` message before starting:

- **Invalid input** (non-MCP input, empty stdin, malformed JSON): the server prints `ERROR: stdin does not contain a valid MCP initialize message` to stderr and exits with code 2.
- **Oversized input** (first line exceeds 1 MB): treated as invalid — same error and exit code 2.

MCP clients that correctly send the `initialize` message are not affected.

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
        "SEARXNG_MAX_RETRIES": "5"
      }
    }
  }
}
```

Add `"SEARXNG_ALLOW_GET_FALLBACK": "1"` only as a temporary compatibility
escape hatch for a deployment that rejects POST `/search`.

#### Debug Mode (opt-in, do not enable by default)

> **Warning:** Debug mode writes HTTP request and response details, including
> the full query text, to stderr in plain text. Do not enable it for production
> use, shared hosts, or any workload where queries may be sensitive.

To enable debug logging temporarily for troubleshooting, add `"DEBUG": "1"` to
the `env` block above. Remove it once the issue is diagnosed.

### CLI Mode Configuration

When running in CLI mode (with a query argument), command-line flags can be used directly:

```bash
# Custom SearXNG server via flag
searxng-mcp-go "query" --searxng-url=https://your-searxng-instance.example.com

# Debug mode via flag
searxng-mcp-go "query" --debug

# HTTP timeout and retry tuning via flags
searxng-mcp-go "query" --timeout=8s --max-retries=5

# GET fallback opt-in via flag
searxng-mcp-go "query" --allow-get-fallback
```

---

## Related Documentation

- [README.md](../README.md) — Quick start and usage overview
- [MCP Tools Reference](MCP_TOOLS.md) — Full tool parameters and response format
