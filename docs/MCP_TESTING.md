# MCP Testing Guide

How to test the MCP server in searxng-mcp-go at the right layer. This guide
covers three complementary approaches — in-memory transport, subprocess stdio
with `mcp.CommandTransport`, and raw `exec.Command` for CLI exit codes — and
when to reach for each.

For the layer overview, see [Test Layers](#test-layers) below.

## Why Pipe-Based Testing Is Wrong

Using `echo '{"jsonrpc":...}' | ./searxng-mcp-go` to test MCP is fundamentally flawed:

- MCP requires a **persistent bidirectional session** over stdin/stdout
- A pipe is one-shot: it sends data then EOF, killing the session immediately
- Each test must re-initialize from scratch
- Multi-step flows, notifications, and session behavior cannot be tested

## Recommended: InMemoryTransport (from official SDK)

The `modelcontextprotocol/go-sdk` provides `NewInMemoryTransports()` specifically for in-process testing. No subprocess, no pipe, no network — just direct function calls.

### Basic Setup

```go
import (
    "testing"
    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func setupTest(t *testing.T) (*mcp.ClientSession, *mcp.ServerSession, func()) {
    ctx := t.Context()

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
    ctx := t.Context()

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
    result, err := clientSession.CallTool(t.Context(), &mcp.CallToolParams{
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

## End-to-End: Subprocess stdio with CommandTransport

In-memory transports prove the protocol surface and tool wiring, but they do
not exercise the real stdio boundary, the binary's startup environment, or
process lifecycle. Most E2E tests in `e2e_*_test.go` use build tag `e2e`; the
stress tests in `e2e_stress_test.go` require `e2e && stress`. They use the
SDK's `mcp.CommandTransport`, which spawns the built binary as a subprocess
and talks JSON-RPC over its stdin/stdout. This is the only layer that catches
issues like wrong env-var parsing on startup, missing stderr flushing on
shutdown, or stdio framing regressions.

Three files are not gated by the `e2e` build tag:
`e2e_exitcode_test.go`, `e2e_cli_test.go`, and `e2e_mockserver_test.go`.
They exercise the built binary as a subprocess with no external dependencies
(no live SearXNG server, no MCP session) and run in under a second each,
so they ship in the default `go test ./...` set alongside the unit tests.

### Pattern

```go
//go:build e2e

import (
    "bytes"
    "context"
    "os"
    "os/exec"
    "testing"
    "time"

    "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestE2E(t *testing.T) {
    ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
    defer cancel()

    // 1. Reuse a pre-built binary if E2E_MCP_BINARY is set;
    //    otherwise fall back to building one in t.TempDir().
    binaryPath := os.Getenv("E2E_MCP_BINARY")
    if binaryPath == "" {
        binaryPath = buildE2EMCPBinary(ctx, t) // go build -o <tempdir>/searxng-mcp-go .
    }

    var stderr bytes.Buffer
    cmd := exec.CommandContext(ctx, binaryPath)
    cmd.Env = e2eMCPEnv(os.Getenv("SEARXNG_URL")) // adds SEARXNG_MAX_RETRIES=2
    cmd.Stderr = &stderr

    client := mcp.NewClient(&mcp.Implementation{Name: "e2e", Version: "0"}, nil)
    session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
    if err != nil {
        t.Fatalf("connect failed: %v\nstderr:\n%s", err, stderr.String())
    }
    t.Cleanup(func() {
        _ = session.Close()
        if cmd.Process != nil && cmd.ProcessState == nil {
            _ = cmd.Process.Kill()
            _, _ = cmd.Process.Wait()
        }
    })

    // ... session.ListTools(), session.CallTool(), assert results ...
}
```

### `E2E_MCP_BINARY`

`E2E_MCP_BINARY` is an optional environment variable read by every E2E test
that uses `mcp.CommandTransport` (`e2e_mcp_test.go`, `e2e_functional_test.go`,
`e2e_error_test.go`, `e2e_lifecycle_test.go`, `e2e_stress_test.go`). When set, the test reuses that
binary instead of running `go build` on every test invocation. CI sets it to
`./searxng-mcp-go` after a single explicit build step (see
`.github/workflows/e2e.yml`), which makes the in-workflow retry loop cheap.
For local development:

```bash
go build -o ./searxng-mcp-go .
E2E_MCP_BINARY=$PWD/searxng-mcp-go \
  SEARXNG_URL=http://127.0.0.1:8888 \
  go test -tags=e2e -run TestMCPStdioE2E -count=1 .
```

When `E2E_MCP_BINARY` is unset, each test builds its own binary in
`t.TempDir()` via the `buildE2EMCPBinary` helper.

`e2e_exitcode_test.go` does not use `mcp.CommandTransport` and is not gated
by the `e2e` build tag; it asserts CLI exit codes via raw `exec.Command` and
builds its own binary in `t.TempDir()` regardless of `E2E_MCP_BINARY`. It
runs as part of the default `go test ./...` set, so CI's regular test job
picks it up via the `-tags=stress`-less default run.

### `SEARXNG_MAX_RETRIES=2`

The shared `e2eMCPEnv` helper (in `e2e_mcp_test.go`) injects
`SEARXNG_MAX_RETRIES=2` on top of the inherited environment. The production
default is 5; the E2E tests deliberately lower it to keep wall-clock time
predictable while still exercising the retry path. Do not raise this for new
E2E tests without an explicit reason, and do not lower it to 0 — that would
hide regressions in the retry layer.

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

## Test Layers

The repository tests MCP at five layers, each catching a different class of
defect. The three complementary MCP testing approaches from the intro above
cover rows 2–4 of the table; unit tests and manual / smoke checks bookend
them. Pick the lowest layer that can meaningfully assert what you want to
verify.

| Layer | Method | Files | What it catches |
|-------|--------|-------|-----------------|
| Unit | direct handler call (`NewSearchToolHandler()`) | `mcp_tool_test.go`, `internal/searxng/*_test.go` | Search logic, input validation, JSON response shape |
| MCP integration | `mcp.NewInMemoryTransports()` (`net.Pipe` in-process) | (unit-style tests of the MCP server surface) | MCP protocol wiring, tool registration, schema, session lifecycle |
| CLI subprocess (default `go test ./...`) | raw `exec.Command` on the built binary | `e2e_exitcode_test.go`, `e2e_cli_test.go`, `e2e_mockserver_test.go` | CLI exit code contract, stderr/stdout split, flag parsing |
| MCP E2E (stdio, `-tags=e2e`) | `exec.Command` + `mcp.CommandTransport` (subprocess) | `e2e_mcp_test.go`, `e2e_functional_test.go`, `e2e_error_test.go`, `e2e_lifecycle_test.go` | Real stdio framing, env-var startup, process lifecycle, live SearXNG behavior |
| MCP E2E stress (stdio, `-tags='e2e stress'`) | `exec.Command` + `mcp.CommandTransport` (subprocess) | `e2e_stress_test.go` | Concurrent searches, lifecycle concurrency, goroutine leak detection |
| Manual / smoke | MCP Inspector, CI shell smoke | `.github/workflows/e2e.yml` | Interactive verification, deterministic exit-code smoke |

Rule of thumb:

- Use **InMemoryTransport** for fast, deterministic tests of the MCP protocol
  surface (no subprocess, no live network).
- Use **CommandTransport** when the assertion depends on the binary actually
  starting, reading env vars, or talking over real stdin/stdout.
- Use **raw `exec.Command`** only when the assertion is about the CLI process
  itself (exit code, stderr text) and not about MCP protocol behavior.

## Key Gotchas

1. **Server must Connect before Client** — the client sends `initialize` on Connect(), so the server must be ready to receive it
2. **Always call `serverSession.Wait()` in cleanup** — ensures the server goroutine exits cleanly
3. **`clientSession.Close()` before `serverSession.Wait()`** — client initiates shutdown

## References

- MCP Go SDK: https://github.com/modelcontextprotocol/go-sdk
- MCP Inspector: https://github.com/modelcontextprotocol/inspector
- SDK test examples: go-sdk/mcp/mcp_test.go (65KB, 25 test files)
