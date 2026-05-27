package searxng

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- isEmptyResponse tests ---

func TestIsEmptyResponse(t *testing.T) {
	t.Parallel()

	s := &SearXNGSearcher{}

	t.Run("all empty", func(t *testing.T) {
		t.Parallel()

		if !s.isEmptyResponse(&SearchResponse{}) {
			t.Fatal("isEmptyResponse() = false, want true for empty response")
		}
	})

	t.Run("with results", func(t *testing.T) {
		t.Parallel()

		if s.isEmptyResponse(&SearchResponse{
			Results: []SearchResult{{Title: "A"}},
		}) {
			t.Fatal("isEmptyResponse() = true, want false for response with results")
		}
	})

	t.Run("with answers", func(t *testing.T) {
		t.Parallel()

		if s.isEmptyResponse(&SearchResponse{
			Answers: []Answer{{Answer: "test"}},
		}) {
			t.Fatal("isEmptyResponse() = true, want false for response with answers")
		}
	})

	t.Run("with infoboxes", func(t *testing.T) {
		t.Parallel()

		if s.isEmptyResponse(&SearchResponse{
			Infoboxes: []Infobox{{Infobox: "test"}},
		}) {
			t.Fatal("isEmptyResponse() = true, want false for response with infoboxes")
		}
	})

	t.Run("with suggestions", func(t *testing.T) {
		t.Parallel()

		if s.isEmptyResponse(&SearchResponse{
			Suggestions: []string{"test"},
		}) {
			t.Fatal("isEmptyResponse() = true, want false for response with suggestions")
		}
	})

	t.Run("nil slices count as empty", func(t *testing.T) {
		t.Parallel()

		if !s.isEmptyResponse(&SearchResponse{
			Results: nil,
		}) {
			t.Fatal("isEmptyResponse() = false, want true for nil Results")
		}
	})
}

// --- Close tests ---

func TestClose_Searcher(t *testing.T) {
	t.Parallel()

	t.Run("close with transport", func(t *testing.T) {
		t.Parallel()

		searcher, err := NewSearXNGSearcher(&Config{
			SearXNGURL: "https://127.0.0.1:9999",
			Timeout:    time.Second,
		}, false)
		if err != nil {
			t.Fatalf("NewSearXNGSearcher() error = %v", err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	})

	t.Run("close with custom client", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{
			client: &http.Client{},
		}

		err := s.Close()
		if err != nil {
			t.Fatalf("Close() error = %v, want nil for custom client without transport", err)
		}
	})

	t.Run("close with nil client", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{client: nil}

		err := s.Close()
		if err != nil {
			t.Fatalf("Close() error = %v, want nil for nil client", err)
		}
	})
}

// --- log debug methods (coverage-only, verify no panic) ---

func TestLogDebugMethods(t *testing.T) {
	t.Parallel()

	t.Run("debug=false does nothing", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com", strings.NewReader("body"))
		// Should not panic
		s.logDebugRequest(req, "test body")
		s.logDebugResponse(&http.Response{StatusCode: http.StatusOK}, nil)
		s.logDebugRetry(0, 2, time.Millisecond, nil)
	})

	t.Run("debug=true with valid data", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com", strings.NewReader("body"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/plain")
		// Should not panic
		s.logDebugRequest(req, "test body")
		s.logDebugResponse(&http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil)
		s.logDebugRetry(0, 5, 100*time.Millisecond, errRetryTestConnectionReset)
	})

	t.Run("debug=true with GET request without body preview", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://example.com?q=test", nil)
		req.Header.Set("Accept", "text/html")
		// Should not panic
		s.logDebugRequest(req, "")
	})

	t.Run("debug=true with long body truncated", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}

		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "https://example.com", nil)
		longBody := strings.Repeat("x", DebugBodyPreviewChars+100)
		s.logDebugRequest(req, longBody)
	})

	t.Run("logDebugResponse with error skips logging", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		// Should not panic; error means no logging
		s.logDebugResponse(nil, errRetryTestConnectionReset)
	})

	t.Run("logDebugResponse with nil response skips logging", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		// Should not panic
		s.logDebugResponse(nil, nil)
	})
}
