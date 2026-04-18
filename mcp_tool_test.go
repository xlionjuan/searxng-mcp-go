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

	handler := buildSearchToolHandler(searcher)

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
	if !strings.Contains(textContent.Text, "Go") {
		t.Errorf("expected formatted text to contain 'Go', got: %s", textContent.Text)
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

	handler := buildSearchToolHandler(searcher)

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

	handler := buildSearchToolHandler(searcher)

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

func buildSearchToolHandler(searcher *SearXNGSearcher) func(context.Context, *mcp.CallToolRequest, SearchArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
		if err := ValidateSearchArgs(&args); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "validation error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		resp, err := searcher.Search(ctx, &args)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Search error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatResults(resp)},
			},
		}, nil, nil
	}
}