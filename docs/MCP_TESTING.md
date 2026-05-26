# MCP Stdio Testing Guide

How to properly test the MCP stdio server in searxng-mcp-go.

## Why Pipe-Based Testing Is Wrong

Using `echo '{"jsonrpc":...}' | ./searxng-mcp-go` to test MCP is fundamentally flawed:

- MCP requires a **persistent bidirectional session** over stdin/stdout
- A pipe is one-shot: it sends data then EOF, killing the session immediately
- Each test must re-initialize from scratch
- Multi-step flows, notifications, and session behavior cannot be tested

## Recommended: InMemoryTransport (from official SDK)

The `modelcontextprotocol/go-sdk` (v1.6.0) provides `NewInMemoryTransports()` specifically for in-process testing. No subprocess, no pipe, no network — just direct function calls.

### Basic Setup

```go
import (
    "context"
    "testing"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func setupTest(t *testing.T) (*mcp.ClientSession, *mcp.ServerSession, func()) {
    ctx := context.Background()

    // 1. Create server
    server := mcp.NewServer(&mcp.Implementation{
        Name:    "searxng-mcp-go",
        Version: "1.0.0",
    }, nil)
    // ... add tools to server ...

    // 2. Create paired transports (net.Pipe internally)
    serverTransport, clientTransport := mcp.NewInMemoryTransports()

    // 3. Server connects FIRST (client sends initialize on Connect)
    serverSession, err := server.Connect(ctx, serverTransport, nil)
    if err != nil {
        t.Fatal(err)
    }

    // 4. Client connects second
    client := mcp.NewClient(&mcp.Implementation{
        Name:    "test-client",
        Version: "1.0",
    }, nil)
    clientSession, err := client.Connect(ctx, clientTransport, nil)
    if err != nil {
        t.Fatal(err)
    }

    cleanup := func() {
        clientSession.Close()
        serverSession.Wait()
    }

    return clientSession, serverSession, cleanup
}
```

### Testing a Tool Call

```go
func TestSearchTool(t *testing.T) {
    clientSession, _, cleanup := setupTest(t)
    defer cleanup()
    ctx := context.Background()

    // List tools
    tools, err := clientSession.ListTools(ctx, nil)
    if err != nil {
        t.Fatal(err)
    }
    // Verify "search" tool exists with expected schema

    // Call the search tool
    result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{
        Name: "search",
        Arguments: map[string]any{
            "query": "golang",
        },
    })
    if err != nil {
        t.Fatal(err)
    }

    // Verify result structure
    textContent := result.Content[0].(*mcp.TextContent)
    // Parse JSON from textContent.Text and assert
}
```

### Testing Error Handling

```go
func TestSearchValidation(t *testing.T) {
    clientSession, _, cleanup := setupTest(t)
    defer cleanup()

    // Empty query should return error
    result, err := clientSession.CallTool(context.Background(), &mcp.CallToolParams{
        Name:      "search",
        Arguments: map[string]any{"query": ""},
    })

    if err != nil {
        t.Fatal(err) // MCP transport error, not tool error
    }

    if !result.IsError {
        t.Error("expected IsError=true for empty query")
    }
}
```

## Alternative: IOTransport (custom io streams)

If you need to test with actual io streams instead of in-memory:

```go
// Create paired pipes
serverReader, clientWriter := io.Pipe()
clientReader, serverWriter := io.Pipe()

serverTransport := &mcp.IOTransport{Reader: serverReader, Writer: serverWriter}
clientTransport := &mcp.IOTransport{Reader: clientReader, Writer: clientWriter}
```

But `NewInMemoryTransports()` is preferred — it uses `net.Pipe()` internally and is designed for this exact purpose.

## MCP Inspector (manual testing)

```bash
# Interactive web UI
SEARXNG_URL=http://127.0.0.1:8888 npx @modelcontextprotocol/inspector ./searxng-mcp-go

# Or with a built binary
go build -o searxng-mcp-go .
SEARXNG_URL=http://127.0.0.1:8888 npx @modelcontextprotocol/inspector ./searxng-mcp-go
```

Opens a web UI where you can:
- View all registered tools and their schemas
- Call tools interactively
- See all JSON-RPC messages in real time

## Testing Strategy

| Layer | Method | What It Tests |
|-------|--------|---------------|
| Unit tests | `NewSearchToolHandler()` | Search logic, handler validation, JSON tool responses |
| MCP integration | `NewInMemoryTransports()` | MCP protocol, tool registration, session lifecycle |
| CLI integration | `exec.Command` + binary | End-to-end CLI behavior, exit codes, output format |
| Manual/CI | MCP Inspector | Interactive verification, smoke test |

## Key Gotchas

1. **Server must Connect before Client** — the client sends `initialize` on Connect(), so the server must be ready to receive it
2. **Always call `serverSession.Wait()` in cleanup** — ensures the server goroutine exits cleanly
3. **`clientSession.Close()` before `serverSession.Wait()`** — client initiates shutdown

## References

- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- MCP Inspector: https://github.com/modelcontextprotocol/inspector
- SDK test examples: go-sdk/mcp/mcp_test.go (65KB, 25 test files)
