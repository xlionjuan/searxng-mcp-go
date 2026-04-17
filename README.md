# searxng-mcp-go

A Model Context Protocol (MCP) server that provides web search via the [SearXNG](https://github.com/searxng/searxng) meta-search engine. Designed for AI agents that need web search capabilities without direct internet access.

Runs in two modes: **CLI** for direct terminal use, and **MCP stdio** for integration with AI clients like Claude Desktop, Cursor, and other MCP-compatible tools.

## Usage

### CLI Mode

When you pass a query or flags, the server runs as a one-shot CLI tool and exits.

```bash
# Basic search (positional argument)
searxng-mcp-go "golang programming"

# Search with flags
searxng-mcp-go --query "web development" --language en --safesearch 1

# JSON output (for piping/scripting)
searxng-mcp-go "machine learning" --json

# Filter by time range
searxng-mcp-go "AI news" --time_range day

# Search specific engines and categories
searxng-mcp-go "linux kernel" --engines "google,bing" --categories general

# Use a custom SearXNG instance
searxng-mcp-go --searxng-url https://my-searxng.example.com "search terms"

# Paginate results
searxng-mcp-go "recipes" --pageno 2
```

**CLI Flags:**

| Flag              | Type   | Default | Description                                          |
|-------------------|--------|---------|------------------------------------------------------|
| `--query`         | string |         | Search query (alternative to positional argument)     |
| `--json`          | bool   | false   | Output results as formatted JSON                      |
| `--searxng-url`   | string |         | SearXNG instance URL (overrides `SEARXNG_URL` env)    |
| `--language`      | string | auto    | Language code (en, zh-tw, ja, etc.); empty = SearXNG decides |
| `--safesearch`    | int    | 0       | SafeSearch: 0=Off, 1=Moderate, 2=Strict               |
| `--time_range`    | string |         | Time filter: day, month, year                         |
| `--categories`    | string |         | Comma-separated categories (general, news, music)     |
| `--engines`       | string |         | Comma-separated engines (google, bing, duckduckgo)    |
| `--pageno`        | int    | 1       | Page number (>= 1)                                    |
| `--help`          | bool   | false   | Show help message                                     |
| `--version`       | bool   | false   | Show version                                          |
| `--debug`         | bool   | false   | Enable verbose HTTP request/response logging          |

### MCP Mode

When run without CLI arguments, the server starts in **MCP stdio mode** — it reads JSON-RPC messages from stdin and writes responses to stdout.

```bash
# Start MCP server (no arguments = MCP mode)
searxng-mcp-go

# With custom SearXNG URL
SEARXNG_URL=https://my-instance.example.com searxng-mcp-go
```

**Claude Desktop / MCP Client Configuration:**

```json
{
  "mcpServers": {
    "searxng": {
      "command": "searxng-mcp-go",
      "env": {
        "SEARXNG_URL": "https://search-4.xlion.dev"
      }
    }
  }
}
```

**Available MCP Tool: `search`**

| Parameter    | Type          | Required | Default | Description                                              |
|--------------|---------------|----------|---------|----------------------------------------------------------|
| `query`      | string        | Yes      |         | Search query string                                      |
| `language`   | string        | No       | auto    | Language code (en, zh-tw, ja, etc.); empty = SearXNG decides |
| `safesearch` | integer       | No       | 0       | 0=Off, 1=Moderate, 2=Strict                              |
| `time_range` | string        | No       |         | day, month, year                                         |
| `categories` | string        | No       |         | Comma-separated categories                               |
| `engines`    | string        | No       |         | Comma-separated engines                                  |
| `pageno`     | integer/null  | No       | 1       | Page number (>= 1)                                       |

See [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) for full tool documentation and response format details.

## Environment Variables

| Variable       | Required | Default                        | Description                          |
|----------------|----------|--------------------------------|--------------------------------------|
| `SEARXNG_URL`  | No       | `https://search-4.xlion.dev`  | SearXNG instance URL to query        |
| `DEBUG`        | No       |                                | Set to `1` to enable verbose HTTP request/response logging |

Configuration priority: **CLI flag** > **environment variable** > **default value**.

```bash
# Set via environment
export SEARXNG_URL=https://my-searxng.example.com
searxng-mcp-go "search query"

# Or override per-invocation
searxng-mcp-go --searxng-url https://other-instance.example.com "search query"
```

**Note:** The default server (`search-4.xlion.dev`) is provided for convenience. For production use, deploy your own SearXNG instance and set `SEARXNG_URL`.

## Error Handling

- Empty or whitespace-only queries are rejected at validation
- Invalid `time_range` values (must be: day, month, year)
- Network/connectivity failures return descriptive error messages
- SearXNG returning HTML instead of JSON produces an `HTMLResponseError`

## Links

- [MCP Tool Reference](docs/MCP_TOOLS.md)
- [Installation & Build Guide](docs/INSTALL.md)
- [AGENTS.md](AGENTS.md) — full project documentation
