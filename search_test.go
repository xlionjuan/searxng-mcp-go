package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// --- performSearch tests ---

func TestPerformSearch_Success(t *testing.T) {
	searchResp := SearchResponse{
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
			{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
		},
		NumberOfResults: 2,
		Query:           "test",
	}
	body, _ := json.Marshal(searchResp)

	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{
		Query:      "test",
		Language:   "en",
		SafeSearch: 1,
	}

	ctx := t.Context()
	result, err := performSearch(ctx, cfg, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Title != "Result 1" {
		t.Errorf("expected 'Result 1', got %s", result.Results[0].Title)
	}

	// Verify request parameters using captured query
	tests := []struct {
		name  string
		param string
		want  string
	}{
		{"query", "q", "test"},
		{"format", "format", "json"},
		{"language", "language", "en"},
		{"safesearch", "safesearch", "1"},
	}
	for _, tt := range tests {
		if capturedQuery.Get(tt.param) != tt.want {
			t.Errorf("expected %s=%q, got %q", tt.param, tt.want, capturedQuery.Get(tt.param))
		}
	}
}

func TestPerformSearch_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate network error
	}))
	server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	var searxngErr *SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *SearXNGError, got: %v", err)
	}
}

func TestPerformSearch_InvalidURL(t *testing.T) {
	cfg := &Config{
		SearXNGURL: "://invalid-url",
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		t.Errorf("expected *url.Error, got: %v", err)
	}
}

func TestPerformSearch_TimeRangeParam(t *testing.T) {
	var capturedTimeRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTimeRange = r.URL.Query().Get("time_range")
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	tests := []struct {
		timeRange string
		want      string
	}{
		{"day", "day"},
		{"month", "month"},
		{"year", "year"},
		{"", ""},
	}

	for _, tt := range tests {
		capturedTimeRange = ""
		args := &SearchArgs{Query: "test", TimeRange: tt.timeRange}
		ctx := t.Context()
		_, err := performSearch(ctx, cfg, args)
		if err != nil {
			t.Errorf("performSearch() error = %v", err)
			continue
		}
		if capturedTimeRange != tt.want {
			t.Errorf("time_range = %q, want %q", capturedTimeRange, tt.want)
		}
	}
}

func TestPerformSearch_DefaultLanguage(t *testing.T) {
	var capturedLanguage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLanguage = r.URL.Query().Get("language")
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"} // Language is empty, should default to en

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLanguage != "en" {
		t.Errorf("expected default language 'en', got %q", capturedLanguage)
	}
}

func TestPerformSearch_OptionalParams(t *testing.T) {
	var capturedParams url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedParams = r.URL.Query()
		searchResp := SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "test"}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

	tests := []struct {
		name    string
		args    *SearchArgs
		param   string
		wantVal string
	}{
		{"categories forwarded", &SearchArgs{Query: "test", Categories: "general,news"}, "categories", "general,news"},
		{"engines forwarded", &SearchArgs{Query: "test", Engines: "google,bing"}, "engines", "google,bing"},
		{"pageno forwarded", &SearchArgs{Query: "test", Pageno: intPtr(2)}, "pageno", "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			capturedParams = nil
			ctx := t.Context()
			_, err := performSearch(ctx, cfg, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if capturedParams.Get(tt.param) != tt.wantVal {
				t.Errorf("param %q = %q, want %q", tt.param, capturedParams.Get(tt.param), tt.wantVal)
			}
		})
	}
}

func TestPerformSearch_HTMLResponseError(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		desc        string
	}{
		{
			name:        "HTML content-type header",
			contentType: "text/html",
			body:        "<!DOCTYPE html><html><head><title>Search</title></head><body>JSON output not enabled</body></html>",
			desc:        "HTMLResponseError triggered by text/html content-type",
		},
		{
			name:        "body starts with DOCTYPE",
			contentType: "text/html; charset=utf-8",
			body:        "<!DOCTYPE html><html><body>Please enable JSON output</body></html>",
			desc:        "HTMLResponseError triggered by <!DOCTYPE prefix",
		},
		{
			name:        "body starts with html tag",
			contentType: "application/octet-stream",
			body:        "<html><body>JSON output not enabled on this instance</body></html>",
			desc:        "HTMLResponseError triggered by <html> prefix even without HTML content-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			cfg := &Config{
				SearXNGURL: server.URL,
				Timeout:    30 * time.Second,
			}
			args := &SearchArgs{Query: "test"}

			ctx := t.Context()
			_, err := performSearch(ctx, cfg, args)

			if err == nil {
				t.Fatal("expected HTMLResponseError, got nil")
			}

			var htmlErr *HTMLResponseError
			if !errors.As(err, &htmlErr) {
				t.Errorf("expected HTMLResponseError, got: %v", err)
			}

			if !strings.Contains(err.Error(), "searxng returned html instead of json") {
				t.Errorf("expected HTML response error message, got: %v", err)
			}
		})
	}
}

// --- helper functions and test utilities ---

func intPtr(i int) *int {
	return &i
}

func TestPerformSearch_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected context deadline exceeded error, got nil")
	}
	// The error should be related to context cancellation/timeout
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "request canceled") {
		t.Errorf("expected context-related error, got: %v", err)
	}
}

// Test performSearch with empty query (edge case - query parameter still set)
// Test HTTP status code errors
func TestPerformSearch_HTTPStatusCodes(t *testing.T) {
	statusCodes := []struct {
		code       int
		statusName string
		errPart    string
	}{
		{400, "Bad Request", "searxng error (status 400)"},
		{401, "Unauthorized", "searxng error (status 401)"},
		{403, "Forbidden", "searxng error (status 403)"},
		{404, "Not Found", "searxng error (status 404)"},
		{429, "Too Many Requests", "searxng error (status 429)"},
		{500, "Internal Server Error", "searxng error (status 500)"},
		{502, "Bad Gateway", "searxng error (status 502)"},
		{503, "Service Unavailable", "searxng error (status 503)"},
		{504, "Gateway Timeout", "searxng error (status 504)"},
	}

	for _, tc := range statusCodes {
		t.Run(tc.statusName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write([]byte(fmt.Sprintf("Error %d: %s", tc.code, tc.statusName)))
			}))
			defer server.Close()

			cfg := &Config{
				SearXNGURL: server.URL,
				Timeout:    30 * time.Second,
			}
			args := &SearchArgs{Query: "test"}

			ctx := t.Context()
			_, err := performSearch(ctx, cfg, args)

			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.code)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.errPart) {
				t.Errorf("expected error containing %q, got: %v", tc.errPart, err)
			}
		})
	}
}

// Test JSON parse error with helpful message
func TestPerformSearch_JSONParseError(t *testing.T) {
	invalidJSON := []byte(`{"results": [{"title": "test", `) // truncated/invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(invalidJSON)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

// Test that performSearch properly encodes query parameters
func TestPerformSearch_QueryEncoding(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("q")
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           capturedQuery,
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	// Test query with special characters that need URL encoding
	args := &SearchArgs{Query: "test query with spaces & special=chars"}

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedQuery != "test query with spaces & special=chars" {
		t.Errorf("expected decoded query 'test query with spaces & special=chars', got %q", capturedQuery)
	}
}

// Test URL validation edge cases
func TestNewSearXNGSearcher_URLValidation(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantErr   bool
		errSubstr string
	}{
		{"empty URL", "", true, "cannot be empty"},
		{"no scheme", "search.example.com", true, "http or https scheme"},
		{"ftp scheme", "ftp://search.example.com", true, "http or https scheme"},
		{"relative path only", "/search", true, "http or https scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSearXNGSearcher(tt.baseURL, 30*time.Second, nil)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

// Test NumberOfResults=0 with actual results (SearXNG quirk)
func TestPerformSearch_NumberOfResultsZeroWithResults(t *testing.T) {
	// SearXNG may return number_of_results=0 even when results exist
	searchResp := SearchResponse{
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
			{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
		},
		NumberOfResults: 0, // API returns 0 but has results
		Query:           "test",
	}
	body, _ := json.Marshal(searchResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := t.Context()
	result, err := performSearch(ctx, cfg, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should have corrected NumberOfResults to match actual result count
	if result.NumberOfResults != 2 {
		t.Errorf("NumberOfResults = %d, want 2", result.NumberOfResults)
	}
	if len(result.Results) != 2 {
		t.Errorf("len(Results) = %d, want 2", len(result.Results))
	}
}
