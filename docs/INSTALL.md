# Installation and Build Guide

## Prerequisites

- Go 1.26.2 or later (developed with Go 1.26.2)
- Access to a SearXNG instance (default: `https://search-4.xlion.dev`)

## Installing

### Clone and build from source

```bash
git clone <repository-url>
cd searxng-mcp-go
go build -o searxng-mcp-go .
```

### Install dependencies

```bash
go mod download
go mod tidy
```

## Building

```bash
# Build the server binary
go build -o searxng-mcp-go .

# Build for a different output name
go build -o my-search-server .

# Build with stripped debug info
go build -ldflags="-s -w" -o searxng-mcp-go .
```

## Running

### Basic execution

```bash
./searxng-mcp-go
```

The server uses stdio transport, meaning it communicates via stdin/stdout. It is designed to be invoked by an MCP client host (such as an AI agent framework).

### MCP Server Configuration

Configure the MCP server via environment variables before launching it from your MCP client (e.g., Cursor, Claude Desktop, Hermes):

```bash
# Set SearXNG instance URL
export SEARXNG_URL=https://your-searxng-instance.example.com

# Enable debug logging (verbose HTTP request/response output)
export DEBUG=1

# Then run the server (or let your MCP client launch it)
./searxng-mcp-go
```

Note: `DEBUG=1` logs search queries and HTTP requests in plain text. Avoid using it with sensitive queries.

In an MCP client configuration, set `env` in the server definition:

```json
{
  "mcpServers": {
    "searxng": {
      "command": "/path/to/searxng-mcp-go",
      "env": {
        "SEARXNG_URL": "https://your-searxng-instance.example.com",
        "DEBUG": "1"
      }
    }
  }
}
```

**Priority:** command-line flag > environment variable > default hardcoded value

Note: in MCP stdin mode, command-line flags are rejected entirely; use environment variables only (see [ADR-004](adr/004-mcp-stdin-env-only.md)).

### CLI Mode Configuration

When running in CLI mode (with a query argument), command-line flags can be used directly:

```bash
# Custom SearXNG server via flag
./searxng-mcp-go "query" --searxng-url=https://your-searxng-instance.example.com

# Debug mode via flag
./searxng-mcp-go "query" --debug
```

### Testing with MCP Inspector

The Model Context Protocol Inspector allows you to test the server interactively:

```bash
# Install inspector (requires Node.js)
npx @modelcontextprotocol/inspector ./searxng-mcp-go
```

This opens a web interface where you can:
- List available tools
- Call the search tool with different parameters
- Inspect request/response payloads

## Configuration

### Default SearXNG Instance

By default, the server uses `https://search-4.xlion.dev`. See the [MCP Server Configuration](#mcp-server-configuration) section above for how to override it via environment variables or CLI flags.

### Timeout

The default timeout for search requests is 30 seconds. This value is configurable in the source code but cannot be adjusted via MCP client parameters.

### POST→GET Fallback

當 POST 請求失敗時（例如部分 SearXNG 設定回傳 405 Method Not Allowed），伺服器會自動以 GET 重試 `/search` 請求，確保與不同 SearXNG 部署的相容性。

## Running Tests

### Race Detector

The `-race` flag for race condition detection requires CGO to be enabled. Some environments (such as Linuxbrew) have CGO disabled by default, which may cause `go test -race` to fail locally.

If you encounter issues running `go test -race` locally, consider:
- Using Docker where CGO is available
- Running tests in the CI environment
- Or simply run `go test ./...` without the `-race` flag for local development

## Verifying the Build

```bash
# Check the binary exists and is executable
ls -la searxng-mcp-go

# Verify it starts (press Ctrl+C to exit)
./searxng-mcp-go
```

The server starts and then waits for input via stdio.

## Docker (Optional)

To run in a container:

```dockerfile
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o searxng-mcp-go .

FROM alpine:latest
COPY --from=builder /app/searxng-mcp-go /usr/local/bin/
ENTRYPOINT ["searxng-mcp-go"]
```

Build and run:
```bash
docker build -t searxng-mcp .
docker run --rm searxng-mcp
```
