package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

// mockSearcher is a minimal implementation of the searcher interface for tests.
type mockSearcher struct {
	searchFunc func(context.Context, *searxng.SearchArgs) (*searxng.SearchResponse, error)
}

func (m *mockSearcher) Search(ctx context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error) {
	return m.searchFunc(ctx, args)
}

var errMockSearcherBoom = errors.New("boom")

func mockSearXNGHandler(tb testing.TB) http.HandlerFunc {
	tb.Helper()

	body := mustMarshalJSON(tb, searxng.SearchResponse{
		Query:   "golang",
		Answers: nil,
		Results: []searxng.SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
		},
		Infoboxes:           nil,
		Suggestions:         []string{"golang tutorial"},
		NumberOfResults:     1,
		UnresponsiveEngines: nil,
		Debug:               false,
	})

	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
	}
}

func newTestSearcher(t *testing.T, handler http.HandlerFunc) (*searxng.SearXNGSearcher, func()) {
	t.Helper()

	mockServer := httptest.NewServer(handler)

	searcher, err := searxng.NewSearXNGSearcher(
		&searxng.Config{SearXNGURL: mockServer.URL, Timeout: 30 * time.Second}, false,
	)
	if err != nil {
		mockServer.Close()
		t.Fatalf("failed to create searcher: %v", err)
	}

	cleanup := func() {
		_ = searcher.Close() //nolint:errcheck // test cleanup; error is non-actionable
		mockServer.Close()
	}

	return searcher, cleanup
}

func setupMCPSession(t *testing.T, handler http.HandlerFunc) (*mcp.ClientSession, func()) {
	t.Helper()

	searcher, cleanupSearcher := newTestSearcher(t, handler)

	schema, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema failed: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "test-server",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG meta-search engine.",
		InputSchema: schema,
	}, NewSearchToolHandler(searcher))

	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		cleanupSearcher()
		t.Fatalf("server connect failed: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0",
	}, nil)

	clientSession, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		cleanupSearcher()
		t.Fatalf("client connect failed: %v", err)
	}

	cleanup := func() {
		err := clientSession.Close()
		if err != nil {
			t.Errorf("client session close failed: %v", err)
		}

		err = serverSession.Wait()
		if err != nil {
			t.Errorf("server session wait failed: %v", err)
		}

		cleanupSearcher()
	}

	return clientSession, cleanup
}

func TestSearchInputSchema(t *testing.T) {
	t.Parallel()

	var schema map[string]any

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse search input schema: %v", err)
	}

	if got := schema["type"]; got != "object" {
		t.Fatalf("schema type = %v, want object", got)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected schema properties map, got %T", schema["properties"])
	}

	query, ok := props["query"].(map[string]any)
	if !ok {
		t.Fatalf("expected query property map, got %T", props["query"])
	}

	if got := query["type"]; got != "string" {
		t.Fatalf("query type = %v, want string", got)
	}

	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("expected non-empty required list, got %T: %#v", schema["required"], schema["required"])
	}

	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("required = %#v, want [query]", required)
	}
}

func TestMCP_Initialize(t *testing.T) {
	t.Parallel()

	session, cleanup := setupMCPSession(t, mockSearXNGHandler(t))
	defer cleanup()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools failed (session not initialized?): %v", err)
	}

	if len(tools.Tools) == 0 {
		t.Fatal("expected at least one tool after initialize")
	}
}

func TestMCP_ToolsList(t *testing.T) {
	t.Parallel()

	session, cleanup := setupMCPSession(t, mockSearXNGHandler(t))
	defer cleanup()

	tools, err := session.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatalf("list tools failed: %v", err)
	}

	if len(tools.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools.Tools))
	}

	searchTool := tools.Tools[0]
	if searchTool.Name != "search" {
		t.Errorf("expected tool name 'search', got %q", searchTool.Name)
	}

	if searchTool.Description == "" {
		t.Error("expected non-empty tool description")
	}

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
		t.Errorf("expected first required field to be 'query', got %v", reqList[0])
	}
}

// callToolHandler calls the tool handler and fails the test on call error.
func callToolHandler(
	t *testing.T,
	handler func(context.Context, *mcp.CallToolRequest, searxng.SearchArgs) (*mcp.CallToolResult, any, error),
	args searxng.SearchArgs,
) *mcp.CallToolResult {
	t.Helper()

	result, _, err := handler(t.Context(), nil, args)
	if err != nil {
		t.Fatalf("call tool failed: %v", err)
	}

	return result
}

// assertTextContent extracts the single TextContent from a result, failing if absent.
func assertTextContent(t *testing.T, result *mcp.CallToolResult) *mcp.TextContent {
	t.Helper()

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}

	return textContent
}

// assertIsError asserts that result.IsError is true.
func assertIsError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	if !result.IsError {
		t.Fatalf("expected IsError=true, got false with content: %v", result.Content)
	}
}

// assertNotError asserts that result.IsError is false.
func assertNotError(t *testing.T, result *mcp.CallToolResult) {
	t.Helper()

	if result.IsError {
		t.Fatalf("expected IsError=false, got true with content: %v", result.Content)
	}
}

// assertContains asserts that s contains substr.
func assertContains(t *testing.T, s, substr string) {
	t.Helper()

	if !strings.Contains(s, substr) {
		t.Fatalf("expected %q to contain %q", s, substr)
	}
}

func TestNewSearchToolHandler(t *testing.T) {
	t.Parallel()

	t.Run("creates handler", func(t *testing.T) {
		t.Parallel()

		if handler := NewSearchToolHandler(&mockSearcher{}); handler == nil {
			t.Fatal("expected handler function")
		}
	})

	t.Run("validates input", testNewSearchToolHandlerValidatesInput)

	t.Run("returns JSON result", testNewSearchToolHandlerReturnsJSON)

	t.Run("applies default limit when omitted", testNewSearchToolHandlerDefaultLimit)

	t.Run("forwards optional parameters", testNewSearchToolHandlerForwardsParams)

	t.Run("returns search errors", func(t *testing.T) {
		t.Parallel()

		handler := NewSearchToolHandler(&mockSearcher{
			searchFunc: func(_ context.Context, _ *searxng.SearchArgs) (*searxng.SearchResponse, error) {
				return nil, errMockSearcherBoom
			},
		})

		result := callToolHandler(t, handler, searxng.SearchArgs{Query: "test"})
		assertIsError(t, result)
		tc := assertTextContent(t, result)
		assertContains(t, tc.Text, "Search error")
	})

	t.Run("returns SearXNGError details", func(t *testing.T) {
		t.Parallel()

		searxngErr := searxng.NewSearXNGError(500, "text/plain", "internal error", errMockSearcherBoom)

		handler := NewSearchToolHandler(&mockSearcher{
			searchFunc: func(_ context.Context, _ *searxng.SearchArgs) (*searxng.SearchResponse, error) {
				return nil, searxngErr
			},
		})

		result := callToolHandler(t, handler, searxng.SearchArgs{Query: "test"})
		assertIsError(t, result)
		tc := assertTextContent(t, result)
		assertContains(t, tc.Text, "Search error: searxng error (status 500) - content-type text/plain: boom")
	})

	t.Run("returns generic message for non-SearXNGError", func(t *testing.T) {
		t.Parallel()

		handler := NewSearchToolHandler(&mockSearcher{
			searchFunc: func(_ context.Context, _ *searxng.SearchArgs) (*searxng.SearchResponse, error) {
				return nil, errMockSearcherBoom
			},
		})

		result := callToolHandler(t, handler, searxng.SearchArgs{Query: "test"})
		assertIsError(t, result)
		tc := assertTextContent(t, result)
		assertContains(t, tc.Text, "Search error: request failed")
	})
}

// testNewSearchToolHandlerValidatesInput is a subtest helper for input validation.
func testNewSearchToolHandlerValidatesInput(t *testing.T) {
	t.Helper()
	t.Parallel()

	handler := NewSearchToolHandler(&mockSearcher{})

	for _, tt := range []struct {
		name string
		args searxng.SearchArgs
	}{
		{name: "empty query", args: searxng.SearchArgs{Query: ""}},
		{name: "whitespace query", args: searxng.SearchArgs{Query: "   "}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := callToolHandler(t, handler, tt.args)
			assertIsError(t, result)
			tc := assertTextContent(t, result)
			assertContains(t, tc.Text, "Validation error")
		})
	}
}

// testNewSearchToolHandlerReturnsJSON is a subtest helper for JSON result parsing.
func testNewSearchToolHandlerReturnsJSON(t *testing.T) {
	t.Helper()
	t.Parallel()

	response := &searxng.SearchResponse{
		Query:   "golang",
		Answers: nil,
		Results: []searxng.SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
		},
		Infoboxes:           nil,
		Suggestions:         []string{"golang tutorial"},
		NumberOfResults:     1,
		UnresponsiveEngines: nil,
		Debug:               false,
	}

	handler := NewSearchToolHandler(&mockSearcher{
		searchFunc: func(_ context.Context, _ *searxng.SearchArgs) (*searxng.SearchResponse, error) {
			return response, nil
		},
	})

	result := callToolHandler(t, handler, searxng.SearchArgs{Query: "golang"})
	assertNotError(t, result)
	tc := assertTextContent(t, result)

	var parsed searxng.SearchResponse

	err := json.Unmarshal([]byte(tc.Text), &parsed)
	if err != nil {
		t.Fatalf("expected valid JSON, got error: %v\nbody: %s", err, tc.Text)
	}

	if parsed.Query != "golang" {
		t.Fatalf("query = %q, want golang", parsed.Query)
	}

	if len(parsed.Results) != 1 || parsed.Results[0].Title != "Go" || parsed.Results[0].URL != "https://go.dev" {
		t.Fatalf("unexpected results: %#v", parsed.Results)
	}

	if len(parsed.Suggestions) != 1 || parsed.Suggestions[0] != "golang tutorial" {
		t.Fatalf("unexpected suggestions: %#v", parsed.Suggestions)
	}
}

// testNewSearchToolHandlerDefaultLimit is a subtest helper for default limit assertions.
func testNewSearchToolHandlerDefaultLimit(t *testing.T) {
	t.Helper()
	t.Parallel()

	var captured *searxng.SearchArgs

	handler := NewSearchToolHandler(&mockSearcher{
		searchFunc: func(_ context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error) {
			captured = args

			return &searxng.SearchResponse{Query: args.Query}, nil
		},
	})

	_ = callToolHandler(t, handler, searxng.SearchArgs{Query: "golang"})

	if captured == nil {
		t.Fatal("expected searcher to be called")
	}

	if captured.Limit == nil {
		t.Fatal("expected default limit to be applied, got nil")
	}

	if *captured.Limit != searxng.DefaultResultLimit {
		t.Fatalf("captured limit = %d, want %d", *captured.Limit, searxng.DefaultResultLimit)
	}
}

// testNewSearchToolHandlerForwardsParams is a subtest helper for optional param forwarding.
func testNewSearchToolHandlerForwardsParams(t *testing.T) {
	t.Helper()
	t.Parallel()

	var captured *searxng.SearchArgs

	handler := NewSearchToolHandler(&mockSearcher{
		searchFunc: func(_ context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error) {
			captured = args

			return &searxng.SearchResponse{Query: args.Query}, nil
		},
	})

	pageno := 3

	result := callToolHandler(t, handler, searxng.SearchArgs{
		Query:      "golang",
		Language:   "en",
		SafeSearch: 2,
		TimeRange:  "year",
		Categories: "general,news",
		Engines:    "google,bing",
		Pageno:     &pageno,
	})
	assertNotError(t, result)

	if captured == nil {
		t.Fatal("expected searcher to be called")
	}

	if got, want := captured.Query, "golang"; got != want {
		t.Errorf("Query = %q, want %q", got, want)
	}

	if got, want := captured.Language, "en"; got != want {
		t.Errorf("Language = %q, want %q", got, want)
	}

	if got, want := captured.SafeSearch, 2; got != want {
		t.Errorf("SafeSearch = %d, want %d", got, want)
	}

	if got, want := captured.TimeRange, "year"; got != want {
		t.Errorf("TimeRange = %q, want %q", got, want)
	}

	if got, want := captured.Categories, "general,news"; got != want {
		t.Errorf("Categories = %q, want %q", got, want)
	}

	if got, want := captured.Engines, "google,bing"; got != want {
		t.Errorf("Engines = %q, want %q", got, want)
	}

	if captured.Pageno == nil {
		t.Fatal("expected Pageno to be forwarded, got nil")
	}

	if *captured.Pageno != 3 {
		t.Errorf("Pageno = %d, want 3", *captured.Pageno)
	}
}

func TestMCP_DebugGatesUnresponsiveEngines(t *testing.T) {
	t.Parallel()

	sr := searxng.SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results: []searxng.SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
		},
		Suggestions:         []string{},
		UnresponsiveEngines: [][]string{{"brave", `Suspended: " too many "requests"`}},
	}

	t.Run("debug_off", func(t *testing.T) {
		t.Parallel()

		sr := sr
		sr.Debug = false

		data, err := sr.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}

		if strings.Contains(string(data), "unresponsive_engines") {
			t.Fatal("expected unresponsive_engines to be omitted when debug is off")
		}
	})

	t.Run("debug_on", func(t *testing.T) {
		t.Parallel()

		sr := sr
		sr.Debug = true

		data, err := sr.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}

		var decoded map[string]any

		err = json.Unmarshal(data, &decoded)
		if err != nil {
			t.Fatalf("expected valid JSON: %v\nbody: %s", err, string(data))
		}

		value, ok := decoded["unresponsive_engines"]
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
