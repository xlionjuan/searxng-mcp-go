# searxng-mcp-go

A Model Context Protocol (MCP) server that provides web search via the [SearXNG](https://github.com/searxng/searxng) meta-search engine. Designed for AI agents that need web search capabilities. Search queries are proxied through your configured SearXNG instance, which forwards them to its configured search engines according to that instance's settings.

Runs in two modes: **CLI** for direct terminal use, and **MCP stdio** for integration with AI clients like Claude Desktop, Cursor, and other MCP-compatible tools.

## Requirements

- A **SearXNG instance** with API access and JSON output enabled in its settings. The server proxies all search queries to this instance — there is no default.
- **Build & run:** Pure Go, no OS-specific imports or build constraints.
  `go build` works on Linux, macOS, and other platforms supported by the Go
  toolchain.
- **Official distribution:** Linux only (x86_64 and arm64). The Homebrew cask
  and `goreleaser` build matrix (`goos: [linux]` in `.goreleaser.yaml`)
  intentionally target Linux. macOS is not packaged — build from source with
  `go build` if you need it.

## Installation

### Homebrew (Linux, recommended)

```bash
brew install --cask xlionjuan/tap/searxng-mcp-go
```

> Currently supports Linux only (x86_64 and arm64). The cask and goreleaser
> build matrix target Linux. macOS users can `go build` from source.

### Manual (Download from Releases)

Download the latest tarball for your architecture from the [Releases page](https://github.com/xlionjuan/searxng-mcp-go/releases), extract it, and place the binary in your PATH:

```bash
tar --zstd -xvf searxng-mcp-go_*.tar.zst
mkdir -p ~/.local/bin && mv searxng-mcp-go ~/.local/bin/
```

### More Installation Details

See [docs/INSTALL.md](docs/INSTALL.md) for the detailed installation and MCP configuration guide.

## Quick Start

### 1. Verify the installation

```bash
searxng-mcp-go --version
```

Then try a quick search to confirm the server can reach your SearXNG instance:

```bash
SEARXNG_URL=http://localhost:8888 searxng-mcp-go "hello world"
```

You should see search results printed to the terminal.

### 2. Configure your MCP client

MCP clients often don't inherit your shell PATH, so use the absolute path to the binary:

```bash
which searxng-mcp-go
# → /home/linuxbrew/.linuxbrew/bin/searxng-mcp-go
```

This is what the `mcpServers` entry looks like — check your MCP client's documentation for where and how to configure it:

```json
{
  "mcpServers": {
    "searxng": {
      "command": "/home/linuxbrew/.linuxbrew/bin/searxng-mcp-go",
      "env": {
        "SEARXNG_URL": "http://localhost:8888"
      }
    }
  }
}
```

Replace `/home/linuxbrew/.linuxbrew/bin/searxng-mcp-go` with the actual path from `which`.

## CLI Usage

When you pass a query or flags, the server runs as a one-shot CLI tool and exits.
Any argument activates CLI mode, including standalone flags such as `--json`,
`--help`, or `--version`. Run without arguments for MCP server mode.

```bash
# Basic search
searxng-mcp-go "golang programming"

# JSON output (for scripting)
searxng-mcp-go "machine learning" --json

# Filter by time range and language
searxng-mcp-go "AI news" --time_range day --language en
```

For all available flags, run `searxng-mcp-go --help` or see [docs/INSTALL.md](docs/INSTALL.md).

## MCP Mode

When launched without CLI arguments by an MCP client, the server runs in **MCP stdio mode**. The client starts the process and sends an MCP JSON-RPC first message on stdin: a legacy `initialize` request, a `server/discover` request, or a stateless request carrying `params._meta["io.modelcontextprotocol/protocolVersion"]`. The startup preflight performs only a bounded structural check; the MCP SDK performs complete protocol metadata validation. The client then exchanges tool calls over stdin/stdout.

The server exposes a single **`search`** tool. For full parameter details, response format, and error reference, see [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md).

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `SEARXNG_URL` | **Yes** | — | SearXNG instance URL |
| `SEARXNG_TIMEOUT` | No | `8s` | Per-request HTTP client timeout (e.g., `8s`). Must be positive; zero is rejected. |
| `SEARXNG_MAX_RETRIES` | No | `5` | Max retries after initial search attempt |
| `SEARXNG_ALLOW_GET_FALLBACK` | No | `0` | Set to `1` to opt in to POST→GET fallback when POST `/search` returns 405/501. This sends search parameters in the URL and may expose queries in upstream logs. |
| `DEBUG` | No | — | Set to `1` to log search queries and HTTP request/response details in plain text. The server emits a warning to stderr on startup; most MCP clients do not surface it. Avoid using it with sensitive queries. |

**Priority:** CLI flag > environment variable > hardcoded default.

> **Invalid values:** Values that cannot be parsed (e.g., `SEARXNG_TIMEOUT=abc`)
> produce a warning on stderr and fall back to the built-in default. The process
> continues running. For strict validation, use the CLI flags (`--timeout`,
> `--max-retries`, `--allow-get-fallback`), which exit with a non-zero status on
> invalid input.
>
> **Semantically invalid values** (e.g., `SEARXNG_TIMEOUT=0`, a negative
> `SEARXNG_MAX_RETRIES` value, or a value exceeding the allowed maximum) are
> **rejected with an error** from both env var and CLI flag paths — the process
> exits on startup to prevent silent misconfiguration.

## Troubleshooting

| Symptom | Likely cause |
|---------|-------------|
| MCP client cannot start the server | Binary not found in PATH. Use `which searxng-mcp-go` and set `command` to the absolute path in your client config. |
| `SEARXNG_URL` missing | The server has no default SearXNG instance. Set `SEARXNG_URL` or pass `--searxng-url`. |
| Connection refused / timeout | SearXNG instance not reachable. Check the URL and network connectivity. `SEARXNG_TIMEOUT` controls the per-request HTTP timeout. |
| `405 Method Not Allowed` or `501 Not Implemented` from search | The SearXNG endpoint or reverse proxy rejected POST `/search`. Fix the proxy to allow POST. As a compatibility escape hatch, set `SEARXNG_ALLOW_GET_FALLBACK=1`, but only if you accept that search parameters may appear in URLs and upstream logs. |
| HTML returned instead of JSON | SearXNG JSON API not enabled. Ensure `search.formats` includes `json` in your SearXNG `settings.yml`. |
| Empty or invalid query | Queries must be non-empty strings. For the full per-parameter rules (`time_range`, `categories`, `engines`, `language`, `pageno`, `limit`), see [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md). |

## More Documentation

- [Installation and Configuration Guide](docs/INSTALL.md) — User installation and MCP configuration
- [MCP Tool Reference](docs/MCP_TOOLS.md) — Full tool parameters, response format, error reference
- [AGENTS.md](AGENTS.md) — Project documentation for AI agents
