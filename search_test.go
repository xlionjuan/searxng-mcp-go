package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
)

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
			searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
			body := mustMarshalJSON(t, searchResp)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var params url.Values

				if r.Method == http.MethodPost {
					_ = r.ParseForm()

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
				_, _ = w.Write(body)
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
				searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
				body := mustMarshalJSON(t, searchResp)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
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
			Results:         []searxng.SearchResult{},
			NumberOfResults: 0,
			Query:           capturedQuery,
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
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

		_ = searcher.Close()
		_ = searcher.Close()
	})

	t.Run("shared default client", func(t *testing.T) {
		t.Parallel()

		searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: "https://example.com"}, false)
		if err != nil {
			t.Fatal(err)
		}

		_ = searcher.Close()
		_ = searcher.Close()
	})

	t.Run("custom client", func(t *testing.T) {
		t.Parallel()

		customClient := &http.Client{Timeout: 30 * time.Second}

		searcher, err := searxng.NewSearXNGSearcher(
			&searxng.Config{SearXNGURL: "https://example.com", HTTPClient: customClient}, false)
		if err != nil {
			t.Fatal(err)
		}

		_ = searcher.Close()
		_ = searcher.Close()
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

			searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
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
					_, _ = w.Write(body)

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

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if attempts.Add(1) == 1 {
					w.WriteHeader(tt.status)

					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(successResponseBody))
			}))
			defer server.Close()

			cfg := &searxng.Config{
				SearXNGURL:    server.URL,
				Timeout:       30 * time.Second,
				MaxRetries:    1,
				RetryDelay:    time.Nanosecond,
				MaxRetryDelay: time.Nanosecond,
			}

			result, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := attempts.Add(1)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if attempt == 1 {
			_, _ = w.Write([]byte(`{"query":"test","results":[],"suggestions":[]}`))

			return
		}

		_, _ = w.Write([]byte(successResponseBody))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL:    server.URL,
		Timeout:       30 * time.Second,
		MaxRetries:    1,
		RetryDelay:    time.Nanosecond,
		MaxRetryDelay: time.Nanosecond,
	}

	result, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
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

func TestSearch_EmptySearchResponseRetryCanceled(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	firstResponseWritten := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			defer close(firstResponseWritten)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"query":"test","results":[],"suggestions":[]}`))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL:    server.URL,
		Timeout:       30 * time.Second,
		MaxRetries:    1,
		RetryDelay:    500 * time.Millisecond,
		MaxRetryDelay: 500 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)

		// Wait for the first attempt's response to be fully written before
		// canceling. The close fires after w.Write() on the server side,
		// which is an acceptable approximation of "fully processed by the
		// client" — a more precise signal would require test hooks in the
		// retry path. The short sleep afterwards gives the client time to
		// parse the response and enter the retry wait so the cancel
		// exercises the retry-cancellation path rather than racing the
		// server's Write call.
		<-firstResponseWritten
		time.Sleep(20 * time.Millisecond)

		cancel()
	}()

	_, err := testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})

	<-done

	if err == nil {
		t.Fatal("Search() error = nil, want context cancellation error")
	}

	if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Search() error = %v, want context.Canceled or context.DeadlineExceeded", err)
	}

	if got := attempts.Load(); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

//nolint:gocognit,gocyclo // table-driven test covering browser header scenarios for search requests
func TestSearch_BrowserHeaders(t *testing.T) {
	t.Parallel()

	t.Run("POST request headers", func(t *testing.T) {
		t.Parallel()

		var capturedHeaders http.Header

		searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
		body := mustMarshalJSON(t, searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		}))
		defer server.Close()

		cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedHeaders.Get("User-Agent") == "" {
			t.Error("User-Agent header should be set")
		}

		if capturedHeaders.Get("Accept") == "" {
			t.Error("Accept header should be set")
		}

		if !strings.Contains(capturedHeaders.Get("Accept"), "text/html") {
			t.Errorf("Accept header should contain text/html, got: %s", capturedHeaders.Get("Accept"))
		}

		if capturedHeaders.Get("Accept-Language") == "" {
			t.Error("Accept-Language header should be set")
		}

		if capturedHeaders.Get("Accept-Encoding") == "" {
			t.Error("Accept-Encoding header should be set")
		}

		if capturedHeaders.Get("Sec-Fetch-Mode") != "navigate" {
			t.Errorf("Sec-Fetch-Mode should be navigate, got: %s", capturedHeaders.Get("Sec-Fetch-Mode"))
		}
	})

	t.Run("GET fallback headers", func(t *testing.T) {
		t.Parallel()

		var capturedHeaders http.Header

		searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
		body := mustMarshalJSON(t, searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)

				return
			}

			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)
		}))
		defer server.Close()

		cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second, AllowGETFallback: true}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if capturedHeaders.Get("User-Agent") == "" {
			t.Error("User-Agent header should be set on GET fallback")
		}

		if capturedHeaders.Get("Accept") == "" {
			t.Error("Accept header should be set on GET fallback")
		}

		if !strings.Contains(capturedHeaders.Get("Accept"), "text/html") {
			t.Errorf("Accept header should contain text/html on GET fallback, got: %s", capturedHeaders.Get("Accept"))
		}

		if capturedHeaders.Get("Accept-Language") == "" {
			t.Error("Accept-Language header should be set on GET fallback")
		}

		if capturedHeaders.Get("Sec-Fetch-Mode") != "navigate" {
			t.Errorf("Sec-Fetch-Mode should be navigate on GET fallback, got: %s", capturedHeaders.Get("Sec-Fetch-Mode"))
		}
	})
}
