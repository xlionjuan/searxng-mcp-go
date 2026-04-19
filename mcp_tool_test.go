package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// setupMCPSession creates an in-memory MCP server+client pair for testing.
// The server has the "search" tool registered with a mock SearXNG backend.
// Returns the client session, a cleanup function, and the mock HTTP server.
func setupMCPSession(t *testing.T, handler http.HandlerFunc) (*mcp.ClientSession, func(), *httptest.Server) {
	t.Helper()

	mockServer := httptest.NewServer(handler)

	searcher, err := NewSearXNGSearcher(mockServer.URL, 30*time.Second, nil)
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
	_, err = server.Connect(context.Background(), serverTransport, nil)
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
		searcher.Close()
		mockServer.Close()
	}

	return clientSession, cleanup, mockServer
}

// mockSearXNGHandler returns a handler that responds with a valid SearXNG JSON response.
func mockSearXNGHandler() http.HandlerFunc {
	body, _ := json.Marshal(SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results: []SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
		},
		Suggestions: []string{"golang tutorial"},
	})
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}
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

func TestMCP_ToolsCall_Search(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, mockSearXNGHandler())
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search",
		Arguments: map[string]any{
			"query": "golang",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got true with content: %v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}

	var parsed SearchResponse
	if err := json.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
		t.Fatalf("expected valid JSON, got error: %v\nbody: %s", err, textContent.Text)
	}
	if parsed.Query != "golang" {
		t.Errorf("expected query 'golang', got '%s'", parsed.Query)
	}
	if len(parsed.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed.Results))
	}
	if parsed.Results[0].Title != "Go" {
		t.Errorf("expected title 'Go', got '%s'", parsed.Results[0].Title)
	}
	if parsed.Results[0].URL != "https://go.dev" {
		t.Errorf("expected URL 'https://go.dev', got '%s'", parsed.Results[0].URL)
	}
	if len(parsed.Suggestions) != 1 || parsed.Suggestions[0] != "golang tutorial" {
		t.Errorf("expected suggestions ['golang tutorial'], got %v", parsed.Suggestions)
	}
}

func TestMCP_ToolsCall_OptionalParameters(t *testing.T) {
	tests := []struct {
		name       string
		arguments  map[string]any
		wantParams  map[string]string
	}{
		{
			name: "language",
			arguments: map[string]any{
				"query":    "golang",
				"language": "en",
			},
			wantParams: map[string]string{"language": "en"},
		},
		{
			name: "safesearch",
			arguments: map[string]any{
				"query":      "golang",
				"safesearch":  2,
			},
			wantParams: map[string]string{"safesearch": "2"},
		},
		{
			name: "time_range",
			arguments: map[string]any{
				"query":      "golang",
				"time_range": "year",
			},
			wantParams: map[string]string{"time_range": "year"},
		},
		{
			name: "categories",
			arguments: map[string]any{
				"query":      "golang",
				"categories": "general,news",
			},
			wantParams: map[string]string{"categories": "general,news"},
		},
		{
			name: "engines",
			arguments: map[string]any{
				"query":   "golang",
				"engines": "google,bing",
			},
			wantParams: map[string]string{"engines": "google,bing"},
		},
		{
			name: "pageno",
			arguments: map[string]any{
				"query":  "golang",
				"pageno": 3,
			},
			wantParams: map[string]string{"pageno": "3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedParams map[string]string
			handler := func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_ = r.ParseForm()
					capturedParams = make(map[string]string, len(r.PostForm))
					for key, values := range r.PostForm {
						if len(values) > 0 {
							capturedParams[key] = values[0]
						}
					}
				} else {
					query := r.URL.Query()
					capturedParams = make(map[string]string, len(query))
					for key, values := range query {
						if len(values) > 0 {
							capturedParams[key] = values[0]
						}
					}
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"query":"golang","number_of_results":0,"results":[]}`))
			}

			session, cleanup, _ := setupMCPSession(t, handler)
			defer cleanup()

			result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
				Name:      "search",
				Arguments: tt.arguments,
			})
			if err != nil {
				t.Fatalf("call tool failed: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected IsError=false, got true with content: %v", result.Content)
			}

			for key, want := range tt.wantParams {
				if got := capturedParams[key]; got != want {
					t.Fatalf("%s = %q, want %q", key, got, want)
				}
			}
		})
	}
}

func TestMCP_ToolsCall_EmptyQuery(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, mockSearXNGHandler())
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "search",
		Arguments: map[string]any{
			"query": "",
		},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for empty query")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected error content")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "validation error") {
		t.Errorf("expected text to contain 'validation error', got: %s", textContent.Text)
	}
}

func TestMCP_ToolsCall_WhitespaceQuery(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, mockSearXNGHandler())
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "   "},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for whitespace-only query")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "validation error") {
		t.Errorf("expected text to contain 'validation error', got: %s", textContent.Text)
	}
}

func TestMCP_ToolsCall_SearchError(t *testing.T) {
	session, cleanup, _ := setupMCPSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer cleanup()

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "search",
		Arguments: map[string]any{"query": "test"},
	})
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for search error")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "Search error") {
		t.Errorf("expected text to contain 'Search error', got: %s", textContent.Text)
	}
}

func TestMCP_DebugGatesUnresponsiveEngines(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"query":"golang",
			"number_of_results":1,
			"results":[{"title":"Go","url":"https://go.dev","content":"Go language","engine":"google"}],
			"suggestions":[],
			"unresponsive_engines":[["brave","Suspended:\" too many \"requests"]]
		}`))
	}

	oldDebug := debugMode
	defer func() { debugMode = oldDebug }()

	run := func(t *testing.T, debug bool) map[string]any {
		debugMode = debug
		session, cleanup, _ := setupMCPSession(t, handler)
		defer cleanup()

		result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
			Name:      "search",
			Arguments: map[string]any{"query": "golang"},
		})
		if err != nil {
			t.Fatalf("call tool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected IsError=false, got true: %v", result.Content)
		}
		textContent, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &decoded); err != nil {
			t.Fatalf("expected valid JSON: %v\nbody: %s", err, textContent.Text)
		}
		return decoded
	}

	t.Run("debug_off", func(t *testing.T) {
		noDebug := run(t, false)
		if _, ok := noDebug["unresponsive_engines"]; ok {
			t.Fatal("expected unresponsive_engines to be omitted when debug is off")
		}
	})

	t.Run("debug_on", func(t *testing.T) {
		withDebug := run(t, true)
		value, ok := withDebug["unresponsive_engines"]
		if !ok {
			t.Fatal("expected unresponsive_engines when debug is on")
		}
		entries, ok := value.([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("expected one unresponsive engine entry, got: %#v", value)
		}
		pair, ok := entries[0].([]any)
		if !ok || len(pair) != 2 {
			t.Fatalf("expected [engine_name, error_message] pair, got: %#v", entries[0])
		}
		if pair[0] != "brave" {
			t.Fatalf("expected engine name brave, got: %#v", pair[0])
		}
	})
}
