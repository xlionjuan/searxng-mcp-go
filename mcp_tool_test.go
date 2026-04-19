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

func TestMCPToolHandler_Success(t *testing.T) {
	searchResp := SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results:         []SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
	}
	body, _ := json.Marshal(searchResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	handler := NewSearchToolHandler(searcher)

	args := SearchArgs{Query: "golang"}
	result, _, err := handler(context.Background(), nil, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.IsError {
		t.Errorf("expected IsError=false on success, got true")
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
		t.Fatalf("expected valid JSON in text content, got error: %v\nbody: %s", err, textContent.Text)
	}
	if parsed.Query != "golang" {
		t.Errorf("expected query 'golang', got '%s'", parsed.Query)
	}
	if len(parsed.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(parsed.Results))
	}
	if parsed.Results[0].Title != "Go" {
		t.Errorf("expected result title 'Go', got '%s'", parsed.Results[0].Title)
	}
}

func TestMCPToolHandler_ValidationError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	handler := NewSearchToolHandler(searcher)

	args := SearchArgs{Query: "   "}
	result, _, err := handler(context.Background(), nil, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true on validation error, got false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "validation error") {
		t.Errorf("expected text to contain 'validation error', got: %s", textContent.Text)
	}
}

func TestMCPToolHandler_SearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	handler := NewSearchToolHandler(searcher)

	args := SearchArgs{Query: "test"}
	result, _, err := handler(context.Background(), nil, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if !result.IsError {
		t.Errorf("expected IsError=true on search error, got false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "Search error") {
		t.Errorf("expected text to contain 'Search error', got: %s", textContent.Text)
	}
}

func TestMCPToolHandler_DebugGatesUnresponsiveEngines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"query":"golang","number_of_results":1,"results":[{"title":"Go","url":"https://go.dev","content":"Go language","engine":"google"}],"suggestions":[],"unresponsive_engines":[["brave","Suspended:\" too many \"requests"]]}`))
	}))
	defer server.Close()

	searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	handler := NewSearchToolHandler(searcher)
	oldDebug := debugMode
	defer func() { debugMode = oldDebug }()

	run := func(debug bool) map[string]any {
		debugMode = debug
		result, _, err := handler(context.Background(), nil, SearchArgs{Query: "golang"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("expected result, got nil")
		}
		if result.IsError {
			t.Fatalf("expected IsError=false, got true: %#v", result.Content)
		}
		textContent, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
		}
		var decoded map[string]any
		if err := json.Unmarshal([]byte(textContent.Text), &decoded); err != nil {
			t.Fatalf("expected valid JSON in text content, got error: %v\nbody: %s", err, textContent.Text)
		}
		return decoded
	}

	noDebug := run(false)
	if _, ok := noDebug["unresponsive_engines"]; ok {
		t.Fatalf("expected unresponsive_engines to be omitted when debug is off, got: %v", noDebug)
	}

	withDebug := run(true)
	value, ok := withDebug["unresponsive_engines"]
	if !ok {
		t.Fatalf("expected unresponsive_engines when debug is on, got: %v", withDebug)
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
}
