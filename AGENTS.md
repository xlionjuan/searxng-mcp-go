# SearXNG MCP Server

A Model Context Protocol (MCP) server that provides web search capabilities using the SearXNG meta-search engine.

## Overview

This project implements an MCP server in Go that exposes a `search` tool. AI agents (like Hermes) can call this server to perform web searches without needing direct internet access. The server proxies search requests to a SearXNG instance and returns formatted results.

## Project Structure

```
searxng-mcp-go/
├── main.go           # Main server implementation (350 lines)
├── search.go         # Search functionality, HTTP client (274 lines)
├── errors.go         # Error types and handling (131 lines)
├── format.go         # Output formatting (39 lines)
├── validation.go     # Input validation (40 lines)
├── date.go           # Date/time utilities (105 lines)
├── main_test.go      # Test suite
├── .golangci.yml     # Linter configuration
├── go.mod            # Go module definition
├── go.sum            # Go dependencies checksum
├── coverage.out      # Test coverage report
├── AGENTS.md         # This file - AI agent instructions
└── docs/
    ├── INSTALL.md        # Installation and build guide
    ├── MCP_TOOLS.md      # MCP tool documentation
    └── AI_UX_TEST_GUIDE.md  # AI UX testing guide
```

## Building

```bash
go build -o searxng-mcp-go .
```

This produces a standalone binary `searxng-mcp-go`.

## Running / Testing

### Direct execution

```bash
./searxng-mcp-go
```

The server uses stdio transport, so it communicates via stdin/stdout. This is designed to be invoked by an MCP client host.

### Testing with MCP Inspector

You can test the server using the official MCP Inspector:

```bash
npx @modelcontextprotocol/inspector ./searxng-mcp-go
```

## MCP Server Tools

### `search`

Search the web using SearXNG meta-search engine.

**Parameters:**

| Parameter    | Type    | Required | Default | Description                                      |
|--------------|---------|----------|---------|--------------------------------------------------|
| `query`      | string  | Yes      | -       | Search query string                              |
| `language`   | string  | No       | en      | Language code (e.g., en, zh-tw, ja)              |
| `safesearch` | integer | No       | 0       | SafeSearch level: 0=Off, 1=Moderate, 2=Strict    |
| `time_range` | string  | No       | -       | Time range: day, month, year                     |
| `categories` | string  | No       | -       | Comma-separated categories (e.g., general,news) |
| `engines`    | string  | No       | -       | Comma-separated engines (e.g., google,bing)      |
| `pageno`     | integer | No       | 1       | Page number for pagination                        |

**Example Response:**

```
Found 3 results for 'Golang MCP server':

1. Building MCP Servers with Go
   URL: https://example.com/golang-mcp
   Summary: A tutorial on building Model Context Protocol servers in Go...
   Date: 2024-01-15
   Engine: google
```

## Development Commands

```bash
# Build
go build -o searxng-mcp-go .

# Tidy dependencies
go mod tidy

# Run tests (if any)
go test ./...

# Format code
go fmt ./...
```

## Configuration

The server uses the following default configuration:

- **SearXNG URL**: `https://search-4.xlion.dev`
- **Timeout**: 30 seconds per request
- **Transport**: Stdio (stdin/stdout)

### SearXNG Instance Configuration

The SearXNG URL can be configured at runtime using either an environment variable or a command-line flag:

**Environment variable:**
```bash
export SEARXNG_URL=https://your-searxng-instance.example.com
./searxng-mcp-go
```

**Command-line flag:**
```bash
./searxng-mcp-go -searxng-url=https://your-searxng-instance.example.com
```

**JSON output:**
```bash
./searxng-mcp-go "search query" --json
```

**Priority:** command-line flag > environment variable > default hardcoded value

## Error Handling

The server returns meaningful error messages for:
- Missing required `query` parameter
- Invalid `time_range` values
- Network/connectivity failures
- SearXNG API errors (non-200 responses, malformed JSON)

## Testing

**Build & Test Status:** ✓ All tests passing

```bash
go build -o searxng-mcp-go . && go test ./...
```

**Test Coverage:** Available in `coverage.out`

## Known Limitations

1. **Pagination**: SearXNG API pagination starts at page 1 (not 0). The server validates `pageno >= 1`.

2. **Date Inference**: Publication dates are inferred from content when not provided by the API. This uses relative date parsing (e.g., "2 hours ago", "yesterday", "last week"). The inference is best-effort and may not always be accurate.

3. **HTML Detection**: If the SearXNG instance returns HTML instead of JSON (typically when JSON output is not enabled on the instance), the server returns a specific `HTMLResponseError` with guidance.

4. **Y2K Threshold**: Date parsing uses a Y2K_THRESHOLD of 2000 to handle ambiguous 2-digit years. This will need updating before 2038 when 32-bit signed int overflow occurs.

5. **Content Length**: Summaries are truncated to 4000 UTF-8 runes in formatted output to prevent excessively long responses.
