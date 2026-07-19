package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
	"searxng-mcp-go/internal/testhelper"
)

func TestSearch_TimeoutZeroWithBackgroundContext(t *testing.T) {
	t.Parallel()

	searchResp := searxng.SearchResponse{
		Results: []searxng.SearchResult{
			{Title: "Result", URL: "https://example.com/1", Content: "Content 1", Engine: "test"},
		},
		NumberOfResults: 1,
		Query:           "test",
	}

	server := newJSONTestServer(t, searchResp)
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    0,
	}
	args := &searxng.SearchArgs{
		Query: "test",
	}

	result, err := testPerformSearch(context.Background(), t, cfg, args)
	if err != nil {
		t.Fatalf("Search() error = %v, want nil (timeout=0 should not cancel)", err)
	}

	if len(result.Results) != 1 {
		t.Fatalf("results length = %d, want 1", len(result.Results))
	}
}

func TestSearch_RetryAfterRequestTimeout(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		n := attempts.Add(1)
		if n < 3 {
			// Simulate a context deadline exceeded (timeout) without real sleep
			return nil, context.DeadlineExceeded
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"query":"test","number_of_results":1,` +
					`"results":[{"title":"OK","url":"https://example.com","content":"ok","engine":"test"}],` +
					`"suggestions":[]}`)),
		}, nil
	})

	searcher := newFastRetrySearcher(t, "https://search.example.com", transport, 2)

	result, err := searcher.Search(context.Background(), &searxng.SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v, want nil (retries should not be preempted by request timeout)", err)
	}

	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempts = %d, want 3 (2 request timeouts + 1 success)", got)
	}

	if len(result.Results) != 1 {
		t.Fatalf("results length = %d, want 1", len(result.Results))
	}
}

func TestSearch_CallerContextCancellationStopsRetries(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		attempts.Add(1)
		// Return a retryable error; the retry loop will attempt to back off
		// and the caller context cancellation should abort it.
		return nil, errTestConnectionReset
	})

	// Use a searcher with 10ms retries — fast enough for tests but slow
	// enough that the 50ms caller context fires before all 10 retries exhaust.
	searcher := searxng.NewCustomRetrySearcher(
		"https://search.example.com", transport, 10, 10*time.Millisecond, 10*time.Millisecond)
	if searcher == nil {
		t.Fatal("NewCustomRetrySearcher returned nil")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	_, err := searcher.Search(ctx, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("Search() error = nil, want context-canceled error")
	}

	if got := attempts.Load(); got < 1 {
		t.Fatalf("attempts = %d, want at least 1", got)
	}

	// The caller context deadline is much shorter than the retry budget, so
	// only a small number of attempts should run before cancellation.
	if got := attempts.Load(); got > 6 {
		t.Fatalf("attempts = %d, want <= 6 (caller context should stop retries early)", got)
	}
}

func TestSearch_Success(t *testing.T) {
	t.Parallel()

	searchResp := searxng.SearchResponse{
		Results: []searxng.SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
			{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
		},
		NumberOfResults: 2,
		Query:           "test",
	}

	server := newJSONTestServer(t, searchResp)
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{
		Query: "test",
	}

	ctx := t.Context()

	result, err := testPerformSearch(ctx, t, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}

	if result.Results[0].Title != "Result 1" {
		t.Errorf("expected 'Result 1', got %s", result.Results[0].Title)
	}
}

func TestSearch_PreservesUnresponsiveEngines(t *testing.T) {
	t.Parallel()

	searchResp := searxng.SearchResponse{
		Query:           "test",
		NumberOfResults: 1,
		Results: []searxng.SearchResult{
			{
				Title:   "Result 1",
				URL:     "https://example.com/1",
				Content: "Content 1",
				Engine:  "google",
			},
		},
		Suggestions: []string{},
		UnresponsiveEngines: [][]string{
			{"brave", `Suspended:" too many "requests`},
			{"startpage", `Suspended:" "CAPTCHA`},
		},
		Debug: true,
	}

	server := newJSONTestServer(t, searchResp)
	defer server.Close()

	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
	ctx := t.Context()

	result, err := testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.UnresponsiveEngines) != 2 {
		t.Fatalf("expected 2 unresponsive engines, got %#v", result.UnresponsiveEngines)
	}

	if result.UnresponsiveEngines[0][0] != "brave" || result.UnresponsiveEngines[1][0] != "startpage" {
		t.Fatalf("unexpected unresponsive engines: %#v", result.UnresponsiveEngines)
	}
}

//nolint:gocognit // table-driven test covering all search request parameter combinations
func TestSearch_RequestParameters(t *testing.T) {
	t.Parallel()

	page := 2

	tests := []struct {
		name       string
		args       *searxng.SearchArgs
		wantParams map[string]string
		wantAbsent []string
	}{
		{
			name: "basic parameters",
			args: &searxng.SearchArgs{Query: "test", Language: "en", SafeSearch: 1},
			wantParams: map[string]string{
				"q":          "test",
				"format":     "json",
				"language":   "en",
				"safesearch": "1",
			},
		},
		{
			name:       "default language omitted",
			args:       &searxng.SearchArgs{Query: "test"},
			wantParams: map[string]string{"q": "test", "format": "json"},
			wantAbsent: []string{"language"},
		},
		{
			name:       "time range day forwarded",
			args:       &searxng.SearchArgs{Query: "test", TimeRange: "day"},
			wantParams: map[string]string{"time_range": "day"},
		},
		{
			name:       "time range month forwarded",
			args:       &searxng.SearchArgs{Query: "test", TimeRange: "month"},
			wantParams: map[string]string{"time_range": "month"},
		},
		{
			name:       "time range year forwarded",
			args:       &searxng.SearchArgs{Query: "test", TimeRange: "year"},
			wantParams: map[string]string{"time_range": "year"},
		},
		{
			name:       "empty time range omitted",
			args:       &searxng.SearchArgs{Query: "test", TimeRange: ""},
			wantAbsent: []string{"time_range"},
		},
		{
			name:       "categories forwarded",
			args:       &searxng.SearchArgs{Query: "test", Categories: "general,news"},
			wantParams: map[string]string{"categories": "general,news"},
		},
		{
			name:       "engines forwarded",
			args:       &searxng.SearchArgs{Query: "test", Engines: "google,bing"},
			wantParams: map[string]string{"engines": "google,bing"},
		},
		{
			name:       "pageno forwarded",
			args:       &searxng.SearchArgs{Query: "test", Pageno: &page},
			wantParams: map[string]string{"pageno": "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			capturedParams := make(chan url.Values, 1)
			searchResp := searxng.SearchResponse{
				Results:         []searxng.SearchResult{{Title: "test", URL: "https://example.com"}},
				NumberOfResults: 1,
				Query:           "test",
			}
			body := mustMarshalJSON(t, searchResp)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var params url.Values

				if r.Method == http.MethodPost {
					_ = r.ParseForm() //nolint:errcheck // test reads form best-effort; failure does not affect test outcome

					params = make(url.Values, len(r.PostForm))
					for key, values := range r.PostForm {
						params[key] = append([]string(nil), values...)
					}
				} else {
					params = r.URL.Query()
				}

				capturedParams <- params

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
			}))
			defer server.Close()

			cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

			_, err := testPerformSearch(t.Context(), t, cfg, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			params := <-capturedParams

			for key, want := range tt.wantParams {
				if got := params.Get(key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}

			for _, key := range tt.wantAbsent {
				if _, ok := params[key]; ok {
					t.Errorf("expected %s to be omitted, got values %v", key, params[key])
				}
			}
		})
	}
}

func TestSearch_SearchPathNormalization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		wantPath string
	}{
		{name: "root path", baseURL: "", wantPath: "/search"},
		{name: "nested path", baseURL: "/api/", wantPath: "/api/search"},
		{name: "existing search suffix", baseURL: "/search", wantPath: "/search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotPath string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				searchResp := searxng.SearchResponse{
					Results:         []searxng.SearchResult{{Title: "x", URL: "https://x"}},
					NumberOfResults: 1,
					Query:           "test",
				}
				body := mustMarshalJSON(t, searchResp)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
			}))
			defer server.Close()

			cfg := &searxng.Config{SearXNGURL: server.URL + tt.baseURL, Timeout: 30 * time.Second}

			_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if gotPath != tt.wantPath {
				t.Fatalf("request path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

// --- helper functions and test utilities ---

// Test that Search properly encodes query parameters.
func TestSearch_QueryEncoding(t *testing.T) {
	t.Parallel()

	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			capturedQuery = r.PostFormValue("q")
		} else {
			capturedQuery = r.URL.Query().Get("q")
		}

		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{{Title: "x", URL: "https://x"}},
			NumberOfResults: 1,
			Query:           capturedQuery,
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	// Test query with special characters that need URL encoding
	args := &searxng.SearchArgs{Query: "test query with spaces & special=chars"}

	ctx := t.Context()

	_, err := testPerformSearch(ctx, t, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedQuery != "test query with spaces & special=chars" {
		t.Errorf("expected decoded query 'test query with spaces & special=chars', got %q", capturedQuery)
	}
}

// Test URL validation edge cases.
func TestNewSearXNGSearcher_URLValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseURL   string
		wantErr   bool
		errSubstr string
	}{
		{"valid URL", "https://search.example.com", false, ""},
		{"invalid URL is wrapped", "search.example.com", true, "newSearXNGSearcher: url must use http or https scheme"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: tt.baseURL, Timeout: 30 * time.Second}, false)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// Test NumberOfResults=0 with actual results (SearXNG quirk).
func TestSearch_NumberOfResultsZeroWithResults(t *testing.T) {
	t.Parallel()

	// SearXNG may return number_of_results=0 even when results exist
	searchResp := searxng.SearchResponse{
		Results: []searxng.SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
			{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
		},
		NumberOfResults: 0, // API returns 0 but has results
		Query:           "test",
	}

	server := newJSONTestServer(t, searchResp)
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := t.Context()

	result, err := testPerformSearch(ctx, t, cfg, args)
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

func TestSearXNGSearcher_Close_Idempotent(t *testing.T) {
	t.Parallel()

	t.Run("nil client", func(t *testing.T) {
		t.Parallel()

		searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: "https://example.com"}, false)
		if err != nil {
			t.Fatal(err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("first Close() = %v, want nil", err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("second Close() = %v, want nil", err)
		}
	})

	t.Run("shared default client", func(t *testing.T) {
		t.Parallel()

		searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: "https://example.com"}, false)
		if err != nil {
			t.Fatal(err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("first Close() = %v, want nil", err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("second Close() = %v, want nil", err)
		}
	})

	t.Run("custom client", func(t *testing.T) {
		t.Parallel()

		customClient := &http.Client{Timeout: 30 * time.Second}

		searcher, err := searxng.NewSearXNGSearcher(
			&searxng.Config{SearXNGURL: "https://example.com", HTTPClient: customClient}, false)
		if err != nil {
			t.Fatal(err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("first Close() = %v, want nil", err)
		}

		err = searcher.Close()
		if err != nil {
			t.Fatalf("second Close() = %v, want nil", err)
		}
	})
}

//nolint:gocognit,gocyclo,cyclop // table-driven test covering POST-to-GET fallback with various HTTP responses
func TestSearch_POSTtoGETFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "405 fallback", statusCode: http.StatusMethodNotAllowed},
		{name: "501 fallback", statusCode: http.StatusNotImplemented},
	}

	for _, tt := range tests {
		t.Run(tt.name+" default disabled", func(t *testing.T) {
			t.Parallel()

			var (
				postReq *http.Request
				getReq  *http.Request
			)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					postReq = r

					w.WriteHeader(tt.statusCode)

					return
				}

				if r.Method == http.MethodGet {
					getReq = r
				}

				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
			args := &searxng.SearchArgs{Query: "sensitive search", Language: "en", SafeSearch: 1}

			_, err := testPerformSearch(t.Context(), t, cfg, args)
			if err == nil {
				t.Fatal("expected error when GET fallback is disabled")
			}

			if postReq == nil {
				t.Fatal("POST request was never made")
			}

			if getReq != nil {
				t.Fatal("GET fallback was called even though it is disabled")
			}

			if postReq.URL.RawQuery != "" {
				t.Error("POST request had query params in URI - query should only be in body")
			}

			errText := err.Error()
			if strings.Contains(errText, "sensitive search") || strings.Contains(errText, "q=sensitive") {
				t.Fatalf("error leaked query: %v", err)
			}

			if !strings.Contains(errText, "SEARXNG_ALLOW_GET_FALLBACK=1") {
				t.Fatalf("error = %q, want opt-in guidance", errText)
			}
		})

		t.Run(tt.name+" opt-in enabled", func(t *testing.T) {
			t.Parallel()

			var (
				postReq *http.Request
				getReq  *http.Request
			)

			searchResp := searxng.SearchResponse{
				Results:         []searxng.SearchResult{{Title: "fallback", URL: "https://example.com"}},
				NumberOfResults: 1,
				Query:           "test",
			}
			body := mustMarshalJSON(t, searchResp)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					postReq = r

					w.WriteHeader(tt.statusCode)

					return
				}

				if r.Method == http.MethodGet {
					getReq = r

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome

					return
				}

				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second, AllowGETFallback: true}
			args := &searxng.SearchArgs{Query: "test search", Language: "en", SafeSearch: 1}

			result, err := testPerformSearch(t.Context(), t, cfg, args)
			if err != nil {
				t.Fatalf("unexpected error after fallback: %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			if postReq == nil {
				t.Fatal("POST request was never made")
			}

			if getReq == nil {
				t.Fatal("GET fallback was never called")
			}

			if postReq.URL.RawQuery != "" {
				t.Error("POST request had query params in URI - query should only be in body")
			}

			getQuery := getReq.URL.Query()
			if getQuery.Get("q") != "test search" ||
				getQuery.Get("format") != "json" ||
				getQuery.Get("language") != "en" ||
				getQuery.Get("safesearch") != "1" {
				t.Fatalf("unexpected GET query params: %v", getQuery)
			}
		})
	}
}

func TestSearch_RetriesRetryableStatus(t *testing.T) {
	t.Parallel()

	const successResponseBody = `{"query":"test","number_of_results":1,` +
		`"results":[{"title":"Result","url":"https://example.com","content":"ok","engine":"test"}],` +
		`"suggestions":[]}`

	tests := []struct {
		name   string
		status int
	}{
		{name: "429", status: http.StatusTooManyRequests},
		{name: "500", status: http.StatusInternalServerError},
		{name: "502", status: http.StatusBadGateway},
		{name: "503", status: http.StatusServiceUnavailable},
		{name: "504", status: http.StatusGatewayTimeout},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var attempts atomic.Int32

			transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
				if attempts.Add(1) == 1 {
					return &http.Response{
						StatusCode: tt.status,
						Body:       io.NopCloser(strings.NewReader("")),
					}, nil
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(successResponseBody)),
				}, nil
			})

			searcher := newFastRetrySearcher(t, "https://search.example.com", transport, 1)

			result, err := searcher.Search(t.Context(), &searxng.SearchArgs{Query: "test"})
			if err != nil {
				t.Fatalf("Search() error = %v, want nil", err)
			}

			if got := attempts.Load(); got != 2 {
				t.Fatalf("attempts = %d, want 2", got)
			}

			if len(result.Results) != 1 {
				t.Fatalf("results length = %d, want 1", len(result.Results))
			}
		})
	}
}

func TestSearch_RetriesEmptySearchResponse(t *testing.T) {
	t.Parallel()

	const successResponseBody = `{"query":"test","number_of_results":1,` +
		`"results":[{"title":"Result","url":"https://example.com","content":"ok","engine":"test"}],` +
		`"suggestions":[]}`

	var attempts atomic.Int32

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		attempt := attempts.Add(1)

		if attempt == 1 {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"query":"test","results":[],"suggestions":[]}`)),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(successResponseBody)),
		}, nil
	})

	searcher := newFastRetrySearcher(t, "https://search.example.com", transport, 1)

	result, err := searcher.Search(t.Context(), &searxng.SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("Search() error = %v, want nil", err)
	}

	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2", got)
	}

	if len(result.Results) != 1 {
		t.Fatalf("results length = %d, want 1", len(result.Results))
	}
}

func TestSearch_CanceledDuringRequest(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		callCount.Add(1)
		cancel()

		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body: io.NopCloser(strings.NewReader(
				`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`)),
		}, nil
	})

	searcher := newFastRetrySearcher(t, "https://search.example.com", transport, 1)

	_, err := searcher.Search(ctx, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("Search() error = nil, want context cancellation error")
	}

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Search() error = %v, want context.Canceled", err)
	}

	if got := callCount.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestSearch_RetryWaitCanceled(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	// Use a 1-hour retry delay so the retryWait timer will never fire before
	// the 1ms context timeout. This eliminates the wall-clock race between
	// the two timers — ctx.Done always wins the select.
	ctx, cancel := context.WithTimeout(t.Context(), time.Millisecond)
	defer cancel()

	transport := testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		callCount.Add(1)

		return nil, errTestConnectionReset
	})

	searcher := searxng.NewCustomRetrySearcher(
		"https://search.example.com", transport, 10, time.Hour, time.Hour)
	if searcher == nil {
		t.Fatal("NewCustomRetrySearcher returned nil")
	}

	_, err := searcher.Search(ctx, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("Search() error = nil, want context deadline exceeded error")
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Search() error = %v, want context.DeadlineExceeded", err)
	}

	// Only the first attempt completes; the retryWait fires the context error.
	if got := callCount.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

// assertBrowserHeaders checks that common browser-like headers are set on search requests.
func assertBrowserHeaders(t *testing.T, headers http.Header, label string) {
	t.Helper()

	if headers.Get("User-Agent") == "" {
		t.Errorf("User-Agent header should be set%s", label)
	}

	if headers.Get("Accept") == "" {
		t.Errorf("Accept header should be set%s", label)
	}

	if !strings.Contains(headers.Get("Accept"), "text/html") {
		t.Errorf("Accept header should contain text/html%s, got: %s", label, headers.Get("Accept"))
	}

	if headers.Get("Accept-Language") == "" {
		t.Errorf("Accept-Language header should be set%s", label)
	}

	if headers.Get("Accept-Encoding") == "" {
		t.Errorf("Accept-Encoding header should be set%s", label)
	}

	if headers.Get("Sec-Fetch-Mode") != "navigate" {
		t.Errorf("Sec-Fetch-Mode should be navigate%s, got: %s", label, headers.Get("Sec-Fetch-Mode"))
	}
}

func TestSearch_BrowserHeaders(t *testing.T) {
	t.Parallel()

	t.Run("POST request headers", func(t *testing.T) {
		t.Parallel()

		var capturedHeaders http.Header

		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{{Title: "x", URL: "https://x"}},
			NumberOfResults: 1,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
		}))
		defer server.Close()

		cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertBrowserHeaders(t, capturedHeaders, "")
	})

	t.Run("GET fallback headers", func(t *testing.T) {
		t.Parallel()

		var capturedHeaders http.Header

		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{{Title: "x", URL: "https://x"}},
			NumberOfResults: 1,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)

				return
			}

			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
		}))
		defer server.Close()

		cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second, AllowGETFallback: true}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		assertBrowserHeaders(t, capturedHeaders, " on GET fallback")
	})
}
