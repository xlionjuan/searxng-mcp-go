//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"searxng-mcp-go/internal/searxng"
)

// setupMCPSession creates an in-memory MCP server+client pair for testing.
// The server has the "search" tool registered with a mock SearXNG backend.
// Returns the client session, a cleanup function, and the mock HTTP server.
func setupMCPSession(t *testing.T, handler http.HandlerFunc) (*mcp.ClientSession, func(), *httptest.Server) {
	t.Helper()

	mockServer := httptest.NewServer(handler)

	searcher, err := searxng.NewSearXNGSearcher(mockServer.URL, 30*time.Second, nil, false)
	if err != nil {
		mockServer.Close()
		t.Fatalf("failed to create searcher: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG meta-search engine.",
		InputSchema: json.RawMessage(searchInputSchema),
	}, NewSearchToolHandler(searcher))

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	// Server must connect first (client sends initialize on Connect)
	serverSession, err := server.Connect(context.Background(), serverTransport, nil)
	if err != nil {
		searcher.Close()
		mockServer.Close()
		t.Fatalf("server connect failed: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0",
	}, nil)

	clientSession, err := client.Connect(context.Background(), clientTransport, nil)
	if err != nil {
		searcher.Close()
		mockServer.Close()
		t.Fatalf("client connect failed: %v", err)
	}

	cleanup := func() {
		clientSession.Close()
		serverSession.Wait()
		searcher.Close()
		mockServer.Close()
	}

	return clientSession, cleanup, mockServer
}

func TestMCP_Initialize(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, mockSearXNGHandler())
	defer cleanup()

	// After Connect, the client session should already be initialized.
	// Verify by listing tools (requires an active session).
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools failed (session not initialized?): %v", err)
	}
	if len(tools.Tools) == 0 {
		t.Fatal("expected at least one tool after initialize")
	}
}

func TestMCP_ToolsList(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, mockSearXNGHandler())
	defer cleanup()

	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}

	if len(tools.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools.Tools))
	}

	searchTool := tools.Tools[0]
	if searchTool.Name != "search" {
		t.Errorf("expected tool name 'search', got '%s'", searchTool.Name)
	}
	if searchTool.Description == "" {
		t.Error("expected non-empty tool description")
	}

	// Verify the input schema has the required "query" field.
	// From the client, InputSchema is deserialized as map[string]any.
	schema, ok := searchTool.InputSchema.(map[string]any)
	if !ok {
		t.Fatalf("expected InputSchema to be map[string]any, got %T", searchTool.InputSchema)
	}
	required, ok := schema["required"]
	if !ok {
		t.Fatal("expected 'required' field in input schema")
	}
	reqList, ok := required.([]any)
	if !ok || len(reqList) == 0 {
		t.Fatal("expected non-empty required array")
	}
	if reqList[0] != "query" {
		t.Errorf("expected first required field to be 'query', got '%v'", reqList[0])
	}
}
