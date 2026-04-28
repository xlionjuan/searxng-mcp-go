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
		_, _ = w.Write(body)
	}
}

func newTestSearcher(t *testing.T, handler http.HandlerFunc) (*SearXNGSearcher, func()) {
	t.Helper()

	mockServer := httptest.NewServer(handler)

	searcher, err := NewSearXNGSearcher(mockServer.URL, 30*time.Second, nil)
	if err != nil {
		mockServer.Close()
		t.Fatalf("failed to create searcher: %v", err)
	}

	cleanup := func() {
		searcher.Close()
		mockServer.Close()
	}

	return searcher, cleanup
}

func TestSearchInputSchema(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal([]byte(searchInputSchema), &schema); err != nil {
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
	if !ok || len(required) == 0 {
		t.Fatalf("expected non-empty required list, got %T: %#v", schema["required"], schema["required"])
	}
	if required[0] != "query" {
		t.Fatalf("required[0] = %v, want query", required[0])
	}
}

func TestNewSearchToolHandler(t *testing.T) {
	t.Run("creates handler", func(t *testing.T) {
		if handler := NewSearchToolHandler(&SearXNGSearcher{}); handler == nil {
			t.Fatal("expected handler function")
		}
	})

	t.Run("validates input", func(t *testing.T) {
		handler := NewSearchToolHandler(&SearXNGSearcher{})
		for _, tt := range []struct {
			name string
			args SearchArgs
		}{
			{name: "empty query", args: SearchArgs{Query: ""}},
			{name: "whitespace query", args: SearchArgs{Query: "   "}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				result, _, err := handler(context.Background(), nil, tt.args)
				if err != nil {
					t.Fatalf("call tool failed: %v", err)
				}
				if !result.IsError {
					t.Fatalf("expected IsError=true, got false with content: %v", result.Content)
				}
				textContent, ok := result.Content[0].(*mcp.TextContent)
				if !ok {
					t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
				}
				if !strings.Contains(textContent.Text, "validation error") {
					t.Fatalf("expected validation error, got: %s", textContent.Text)
				}
			})
		}
	})

	t.Run("returns JSON result", func(t *testing.T) {
		searcher, cleanup := newTestSearcher(t, mockSearXNGHandler())
		defer cleanup()

		handler := NewSearchToolHandler(searcher)
		result, _, err := handler(context.Background(), nil, SearchArgs{Query: "golang"})
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
			t.Fatalf("query = %q, want golang", parsed.Query)
		}
		if len(parsed.Results) != 1 || parsed.Results[0].Title != "Go" || parsed.Results[0].URL != "https://go.dev" {
			t.Fatalf("unexpected results: %#v", parsed.Results)
		}
		if len(parsed.Suggestions) != 1 || parsed.Suggestions[0] != "golang tutorial" {
			t.Fatalf("unexpected suggestions: %#v", parsed.Suggestions)
		}
	})

	t.Run("forwards optional parameters", func(t *testing.T) {
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

		searcher, cleanup := newTestSearcher(t, handler)
		defer cleanup()

		handlerFn := NewSearchToolHandler(searcher)
		pageno := 3
		result, _, err := handlerFn(context.Background(), nil, SearchArgs{
			Query:      "golang",
			Language:   "en",
			SafeSearch: 2,
			TimeRange:  "year",
			Categories: "general,news",
			Engines:    "google,bing",
			Pageno:     &pageno,
		})
		if err != nil {
			t.Fatalf("call tool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected IsError=false, got true with content: %v", result.Content)
		}

		for key, want := range map[string]string{
			"language":   "en",
			"safesearch": "2",
			"time_range": "year",
			"categories": "general,news",
			"engines":    "google,bing",
			"pageno":     "3",
		} {
			if got := capturedParams[key]; got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("returns search errors", func(t *testing.T) {
		searcher, cleanup := newTestSearcher(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		defer cleanup()

		handler := NewSearchToolHandler(searcher)
		result, _, err := handler(context.Background(), nil, SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("call tool failed: %v", err)
		}
		if !result.IsError {
			t.Fatalf("expected IsError=true, got false with content: %v", result.Content)
		}
		textContent, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
		}
		if !strings.Contains(textContent.Text, "Search error") {
			t.Fatalf("expected search error, got: %s", textContent.Text)
		}
	})
}

func TestMCP_DebugGatesUnresponsiveEngines(t *testing.T) {
	sr := SearchResponse{
		Query:               "golang",
		NumberOfResults:     1,
		Results:             []SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
		Suggestions:         []string{},
		UnresponsiveEngines: [][]string{{"brave", `Suspended: " too many "requests"`}},
	}

	t.Run("debug_off", func(t *testing.T) {
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
		sr.Debug = true
		data, err := sr.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		var decoded map[string]any
		if err := json.Unmarshal(data, &decoded); err != nil {
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
