# Installation and Build Guide

## Prerequisites

- Go 1.26.2 or later (developed with Go 1.26.2)
- Access to a SearXNG instance (required — no default, e.g. `http://localhost:8888`)

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
When started this way, the first stdin message must be a valid JSON-RPC `initialize` request; if it is not, the server will error and exit.

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

#### Debug Output Details

When debug mode is enabled, the following information is logged for each search request:

- HTTP method, URL, Content-Type, and Accept header
- Request body
- Response status code and content-type
- Response body preview (first 500 characters)
- On error responses: `body_size` and `body_preview`

Additionally, the `unresponsive_engines` field (listing engines that failed to respond, e.g., rate-limited or CAPTCHA) is **only included in the JSON response when debug mode is enabled**; it is omitted entirely in non-debug mode (see [ADR-006](adr/006-unresponsive-engines-debug-only.md)).

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

**ENV Naming Convention ⚠️:** Environment variable names should be neutral. **Only the SearXNG server URL variable may contain `searxng`** (e.g. `SEARXNG_URL`). All other functional ENV vars must NOT use the `SEARXNG_` prefix.

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

For manual testing, use the MCP Inspector or the existing test harness rather than typing stdin messages by hand.

## Configuration

### SearXNG Instance

A SearXNG instance URL is **required** — there is no default. Set it via the `SEARXNG_URL` environment variable or `--searxng-url` CLI flag. See [MCP Server Configuration](#mcp-server-configuration) above for setup instructions.

### Timeout

The default timeout for search requests is 30 seconds. This value is configurable in the source code but cannot be adjusted via MCP client parameters.

### POST→GET Fallback

When a POST request fails (for example, some SearXNG configurations return 405 Method Not Allowed), the server automatically retries the `/search` request with GET, ensuring compatibility with different SearXNG deployments.

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
