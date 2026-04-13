# Installation and Build Guide

## Prerequisites

- Go 1.23 or later (developed with Go 1.26.2)
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

# Build with version info (edit main.go to set version)
go build -ldflags="-s -w" -o searxng-mcp-go .
```

## Running

### Basic execution

```bash
./searxng-mcp-go
```

The server uses stdio transport, meaning it communicates via stdin/stdout. It is designed to be invoked by an MCP client host (such as an AI agent framework).

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

### SearXNG Instance

By default, the server uses `https://search-4.xlion.dev`. You can configure the SearXNG URL at runtime using either an environment variable or a command-line flag:

**Environment variable (recommended):**
```bash
export SEARXNG_URL=https://your-searxng-instance.example.com
./searxng-mcp-go
```

**Command-line flag:**
```bash
./searxng-mcp-go -searxng-url=https://your-searxng-instance.example.com
```

**Priority:** command-line flag > environment variable > default hardcoded value

### Timeout

The default timeout for search requests is 30 seconds. To adjust, you would need to modify the source code in `main.go` and rebuild:

```go
Timeout: 60 * time.Second, // increase to 60 seconds
```

## Verifying the Build

```bash
# Check the binary exists and is executable
ls -la searxng-mcp-go

# Verify it starts (press Ctrl+C to exit)
./searxng-mcp-go
```

The server will log "Starting SearXNG MCP server..." and then wait for input via stdio.

## Docker (Optional)

To run in a container:

```dockerfile
FROM golang:1.21-alpine AS builder
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
