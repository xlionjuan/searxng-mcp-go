package searxng

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
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

func TestSearch_NilArgs(t *testing.T) {
	t.Parallel()

	s := &SearXNGSearcher{}
	_, err := s.Search(t.Context(), nil)
	requireValidationError(t, err, "args")
}

// --- log debug methods (coverage-only, verify no panic) ---

func TestLogDebugMethods(t *testing.T) {
	t.Parallel()

	t.Run("debug=false does nothing", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		//nolint:errcheck // test request with valid URL/method; error impossible
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com", strings.NewReader("body"))
		// Should not panic
		s.logDebugRequest(req, "test body")
		s.logDebugResponse(&http.Response{StatusCode: http.StatusOK}, nil)
		s.logDebugRetry(0, 2, time.Millisecond, nil)
	})

	t.Run("debug=true with valid data", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}

		//nolint:errcheck // test request with valid URL/method; error impossible
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com", strings.NewReader("body"))
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

		//nolint:errcheck // test request with valid URL/method; error impossible
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://example.com?q=test", http.NoBody)
		req.Header.Set("Accept", "text/html")
		// Should not panic
		s.logDebugRequest(req, "")
	})

	t.Run("debug=true with long body truncated", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}

		//nolint:errcheck // test request with valid URL/method; error impossible
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.com", http.NoBody)
		longBody := strings.Repeat("x", DebugBodyPreviewBytes+100)
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

func TestSearch_EmptyResultsAfterRetryExhaustion(t *testing.T) {
	t.Parallel()

	t.Run("exhausted retries return wrapped empty-results error", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return makeJSONResponse(minimalJSONBody), nil
		}), 2)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "my-secret-password"})
		if err == nil {
			t.Fatal("Search() error = nil, want empty-results error")
		}

		if !errors.Is(err, errSearchEmptyResults) {
			t.Fatalf("Search() error = %v, want errSearchEmptyResults in chain", err)
		}

		if result != nil {
			t.Fatalf("Search() result = %#v, want nil", result)
		}

		if callCount != 3 {
			t.Fatalf("callCount = %d, want 3", callCount)
		}

		if strings.Contains(err.Error(), "my-secret-password") {
			t.Fatalf("Search() error = %q, must not contain raw query", err)
		}
	})

	t.Run("zero retries returns wrapped empty-results error", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want empty-results error")
		}

		if !errors.Is(err, errSearchEmptyResults) {
			t.Fatalf("Search() error = %v, want errSearchEmptyResults in chain", err)
		}

		if result != nil {
			t.Fatalf("Search() result = %#v, want nil", result)
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1", callCount)
		}
	})
}

func TestAllowGETFallbackLogsWarnings(t *testing.T) {
	t.Run("startup warning", func(t *testing.T) {
		var buf bytes.Buffer

		logger := slog.New(slog.NewTextHandler(&buf, nil))

		searcher, err := NewSearXNGSearcher(&Config{
			SearXNGURL:       "https://search.example.com",
			AllowGETFallback: true,
			Logger:           logger,
		}, false)
		if err != nil {
			t.Fatalf("NewSearXNGSearcher() error = %v, want nil", err)
		}

		_ = searcher.Close() //nolint:errcheck // test cleanup; error is non-actionable

		logOutput := buf.String()
		if !strings.Contains(logOutput, "GET fallback is enabled") {
			t.Fatalf("log output = %q, want startup warning", logOutput)
		}
	})

	t.Run("per-use warning", func(t *testing.T) {
		var buf bytes.Buffer

		logger := slog.New(slog.NewTextHandler(&buf, nil))

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       http.NoBody,
		}

		postReq, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"https://search.example.com/search",
			strings.NewReader("q=test&format=json"),
		)
		if err != nil {
			t.Fatalf("NewRequestWithContext() error = %v, want nil", err)
		}

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			logger:           logger,
			allowGETFallback: true,
		}

		resp, err := s.executeGETfallback(t.Context(), origResp, postReq, "q=test&format=json")
		if err != nil {
			t.Fatalf("executeGETfallback() error = %v, want nil", err)
		}

		closeBody(resp)

		logOutput := buf.String()
		if !strings.Contains(logOutput, "GET fallback used after POST search was rejected") {
			t.Fatalf("log output = %q, want per-use warning", logOutput)
		}

		if strings.Contains(logOutput, "q=test") {
			t.Fatalf("log output leaked query: %q", logOutput)
		}
	})
}

func TestNewFastRetrySearcher_Success(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		attempt.Add(1)

		return makeJSONResponse(makeSearchResponseJSON(1)), nil
	})

	s := NewFastRetrySearcher("https://search.example.com", transport, 2)
	if s == nil {
		t.Fatal("NewFastRetrySearcher returned nil")
	}
	defer s.Close() //nolint:errcheck // test cleanup

	result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(result.Results))
	}
}

func TestNewFastRetrySearcher_InvalidURL(t *testing.T) {
	t.Parallel()

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		resp := makeJSONResponse(makeSearchResponseJSON(1))

		return resp, nil
	})

	s := NewFastRetrySearcher("://", transport, 2)
	if s != nil {
		t.Fatal("NewFastRetrySearcher with invalid URL = non-nil, want nil")
	}
}

func TestNewCustomRetrySearcher_Success(t *testing.T) {
	t.Parallel()

	var attempt atomic.Int32

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		attempt.Add(1)

		return makeJSONResponse(makeSearchResponseJSON(1)), nil
	})

	s := NewCustomRetrySearcher("https://search.example.com", transport, 2, 10*time.Millisecond, 10*time.Millisecond)
	if s == nil {
		t.Fatal("NewCustomRetrySearcher returned nil")
	}
	defer s.Close() //nolint:errcheck // test cleanup

	result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(result.Results))
	}
}

func TestNewCustomRetrySearcher_InvalidURL(t *testing.T) {
	t.Parallel()

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		resp := makeJSONResponse(makeSearchResponseJSON(1))

		return resp, nil
	})

	s := NewCustomRetrySearcher("://", transport, 2, time.Second, 30*time.Second)
	if s != nil {
		t.Fatal("NewCustomRetrySearcher with invalid URL = non-nil, want nil")
	}
}
