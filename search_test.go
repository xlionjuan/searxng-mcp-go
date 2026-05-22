package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
)

// --- Search tests ---

func TestSearch_CfgNil(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	_, err := testPerformSearch(ctx, t, nil, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected error for cfg == nil, got nil")
	}

	if !strings.Contains(err.Error(), "cfg cannot be nil") {
		t.Errorf("expected cfg cannot be nil error, got: %v", err)
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
	body := mustMarshalJSON(t, searchResp)

	var capturedQuery url.Values

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			_ = r.ParseForm()
			capturedQuery = r.PostForm
		} else {
			capturedQuery = r.URL.Query()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{
		Query:      "test",
		Language:   "en",
		SafeSearch: 1,
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

func TestSearch_PreservesUnresponsiveEngines(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(mustMarshalJSON(t, searxng.SearchResponse{
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
		}))
	}))
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

func TestSearch_TimeRangeParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		timeRange string
		want      string
	}{
		{name: "day", timeRange: "day", want: "day"},
		{name: "month", timeRange: "month", want: "month"},
		{name: "year", timeRange: "year", want: "year"},
		{name: "empty", timeRange: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var capturedTimeRange string

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					capturedTimeRange = r.PostFormValue("time_range")
				} else {
					capturedTimeRange = r.URL.Query().Get("time_range")
				}

				searchResp := searxng.SearchResponse{
					Results:         []searxng.SearchResult{},
					NumberOfResults: 0,
					Query:           "test",
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
			args := &searxng.SearchArgs{Query: "test", TimeRange: tt.timeRange}
			ctx := t.Context()

			_, err := testPerformSearch(ctx, t, cfg, args)
			if err != nil {
				t.Errorf("testPerformSearch() error = %v", err)

				return
			}

			if capturedTimeRange != tt.want {
				t.Errorf("time_range = %q, want %q", capturedTimeRange, tt.want)
			}
		})
	}
}

func TestSearch_DefaultLanguage(t *testing.T) {
	t.Parallel()

	var capturedLanguage string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			capturedLanguage = r.PostFormValue("language")
		} else {
			capturedLanguage = r.URL.Query().Get("language")
		}

		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
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
	args := &searxng.SearchArgs{Query: "test"} // Language is empty, should not be sent to SearXNG

	ctx := t.Context()

	_, err := testPerformSearch(ctx, t, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedLanguage != "" {
		t.Errorf("expected no language param when empty, got %q", capturedLanguage)
	}
}

func TestSearch_OptionalParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    *searxng.SearchArgs
		param   string
		wantVal string
	}{
		{"categories forwarded", &searxng.SearchArgs{Query: "test", Categories: "general,news"}, "categories", "general,news"},
		{"engines forwarded", &searxng.SearchArgs{Query: "test", Engines: "google,bing"}, "engines", "google,bing"},
		{"pageno forwarded", &searxng.SearchArgs{Query: "test", Pageno: new(2)}, "pageno", "2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var capturedParams url.Values

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					_ = r.ParseForm()
					capturedParams = r.PostForm
				} else {
					capturedParams = r.URL.Query()
				}

				searchResp := searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "test"}
				body := mustMarshalJSON(t, searchResp)

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write(body)
			}))
			defer server.Close()

			cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
			ctx := t.Context()

			_, err := testPerformSearch(ctx, t, cfg, tt.args)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if capturedParams.Get(tt.param) != tt.wantVal {
				t.Errorf("param %q = %q, want %q", tt.param, capturedParams.Get(tt.param), tt.wantVal)
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

func TestSearch_UnsupportedBodySizes(t *testing.T) {
	t.Parallel()

	t.Run("oversized error body", func(t *testing.T) {
		t.Parallel()

		body := strings.Repeat("e", searxng.MaxErrorBodySize+1)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("response writer does not support hijacking")

				return
			}

			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack() failed: %v", err)

				return
			}

			defer func() { _ = conn.Close() }()

			header := fmt.Sprintf("HTTP/1.1 500 Internal Server Error\r\nContent-Type: text/plain\r\nContent-Length: %d\r\n\r\n", len(body))
			_, _ = conn.Write([]byte(header))
			_, _ = conn.Write([]byte(body))
		}))
		defer server.Close()

		cfg := &searxng.Config{
			SearXNGURL: server.URL,
			Timeout:    30 * time.Second,
		}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var searxngErr *searxng.SearXNGError
		if !errors.As(err, &searxngErr) {
			t.Fatalf("expected *SearXNGError, got %T", err)
		}

		if searxngErr.StatusCode != http.StatusInternalServerError {
			t.Fatalf("StatusCode = %d, want %d", searxngErr.StatusCode, http.StatusInternalServerError)
		}

		if len(searxngErr.ResponseBody) != searxng.MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want %d", len(searxngErr.ResponseBody), searxng.MaxErrorDisplayChars)
		}

		if !strings.HasPrefix(searxngErr.ResponseBody, "eee") {
			t.Fatalf("ResponseBody does not contain expected body preview: %q", searxngErr.ResponseBody)
		}
	})

	t.Run("oversized success body", func(t *testing.T) {
		t.Parallel()

		body := `{"query":"test","number_of_results":1,"results":[{"title":"Result",` +
			`"url":"https://example.com","content":"` +
			strings.Repeat("s", searxng.MaxResponseBodySize+1) +
			`","engine":"google"}],"suggestions":[]}`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Errorf("response writer does not support hijacking")

				return
			}

			conn, _, err := hj.Hijack()
			if err != nil {
				t.Errorf("Hijack() failed: %v", err)

				return
			}

			defer func() { _ = conn.Close() }()

			header := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n", len(body))
			_, _ = conn.Write([]byte(header))
			_, _ = conn.Write([]byte(body))
		}))
		defer server.Close()

		cfg := &searxng.Config{
			SearXNGURL: server.URL,
			Timeout:    30 * time.Second,
		}

		_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}

		var searxngErr *searxng.SearXNGError
		if !errors.As(err, &searxngErr) {
			t.Fatalf("expected *SearXNGError, got %T", err)
		}

		if !strings.Contains(err.Error(), "response body exceeded maximum size limit") {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(searxngErr.ResponseBody) != searxng.MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want %d", len(searxngErr.ResponseBody), searxng.MaxErrorDisplayChars)
		}
	})
}

func TestSearch_EmptyHTMLBody(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

	_, err := testPerformSearch(t.Context(), t, cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var htmlErr *searxng.HTMLResponseError
	if !errors.As(err, &htmlErr) {
		t.Fatalf("expected *HTMLResponseError, got %T", err)
	}

	if htmlErr.Body != "" {
		t.Fatalf("Body = %q, want empty string", htmlErr.Body)
	}

	if err.Error() != "searxng returned html instead of json - json output may not be enabled on the server" {
		t.Fatalf("unexpected error text: %v", err)
	}
}

func TestSearch_HTMLResponseError(t *testing.T) {
	t.Parallel()

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
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			cfg := &searxng.Config{
				SearXNGURL: server.URL,
				Timeout:    30 * time.Second,
			}
			args := &searxng.SearchArgs{Query: "test"}

			ctx := t.Context()

			_, err := testPerformSearch(ctx, t, cfg, args)
			if err == nil {
				t.Fatal("expected HTMLResponseError, got nil")
			}

			var htmlErr *searxng.HTMLResponseError
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
	body := mustMarshalJSON(t, searchResp)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
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

		searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: "https://example.com", HTTPClient: customClient}, false)
		if err != nil {
			t.Fatal(err)
		}

		_ = searcher.Close()
		_ = searcher.Close()
	})
}

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
		t.Run(tt.name, func(t *testing.T) {
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

			cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
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

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if attempts.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"query":"test","number_of_results":1,"results":[{"title":"Result","url":"https://example.com","content":"ok","engine":"test"}],"suggestions":[]}`))
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

func TestSearch_RetriesEmptySearchResponse(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempt := attempts.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		if attempt == 1 {
			_, _ = w.Write([]byte(`{"query":"test","results":[],"suggestions":[]}`))

			return
		}

		_, _ = w.Write([]byte(`{"query":"test","number_of_results":1,"results":[{"title":"Result","url":"https://example.com","content":"ok","engine":"test"}],"suggestions":[]}`))
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

		cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}

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

// --- SearchResponse.MarshalJSON tests ---

func TestSearchResponse_MarshalJSON_NilSlices(t *testing.T) {
	t.Parallel()

	resp := searxng.SearchResponse{
		Query:           "test",
		NumberOfResults: 0,
		Results:         nil,
		Suggestions:     nil,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	raw := string(data)

	// Results and Suggestions should serialize as [] not null
	if strings.Contains(raw, `"results":null`) {
		t.Errorf("results should be [], not null: %s", raw)
	}

	if strings.Contains(raw, `"suggestions":null`) {
		t.Errorf("suggestions should be [], not null: %s", raw)
	}

	if !strings.Contains(raw, `"results":[]`) {
		t.Errorf("expected results to be [], got: %s", raw)
	}

	if !strings.Contains(raw, `"suggestions":[]`) {
		t.Errorf("expected suggestions to be [], got: %s", raw)
	}

	if strings.Contains(raw, `"unresponsive_engines"`) {
		t.Errorf("expected unresponsive_engines to be omitted when debug is off, got: %s", raw)
	}
}

func TestSearchResponse_MarshalJSON_FieldOrder(t *testing.T) {
	t.Parallel()

	resp := searxng.SearchResponse{
		Query:           "test",
		Answers:         []searxng.Answer{{Answer: "42", Engine: "calc"}},
		NumberOfResults: 1,
		Infoboxes:       []searxng.Infobox{{Infobox: "Info", Content: "content"}},
		Results:         []searxng.SearchResult{{Title: "R1", URL: "https://example.com", Engine: "google"}},
		Suggestions:     []string{"suggestion 1"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	// Verify field order by checking keys appear in expected sequence
	raw := string(data)
	indices := []struct {
		label string
		idx   int
	}{
		{`"query"`, strings.Index(raw, `"query"`)},
		{`"answers"`, strings.Index(raw, `"answers"`)},
		{`"number_of_results"`, strings.Index(raw, `"number_of_results"`)},
		{`"infoboxes"`, strings.Index(raw, `"infoboxes"`)},
		{`"results"`, strings.Index(raw, `"results"`)},
		{`"suggestions"`, strings.Index(raw, `"suggestions"`)},
	}

	for i := 1; i < len(indices); i++ {
		if indices[i].idx <= indices[i-1].idx {
			t.Errorf("field order wrong: %s (pos %d) should come after %s (pos %d) in JSON: %s",
				indices[i].label, indices[i].idx, indices[i-1].label, indices[i-1].idx, raw)
		}
	}
}

func TestSearchResponse_MarshalJSON_OmitEmpty(t *testing.T) {
	t.Parallel()

	resp := searxng.SearchResponse{
		Query:           "test",
		NumberOfResults: 0,
		Results:         []searxng.SearchResult{},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	raw := string(data)

	// answers and infoboxes with omitempty should be omitted
	if strings.Contains(raw, `"answers"`) {
		t.Errorf("expected 'answers' to be omitted when empty, got: %s", raw)
	}

	if strings.Contains(raw, `"infoboxes"`) {
		t.Errorf("expected 'infoboxes' to be omitted when empty, got: %s", raw)
	}

	// query, number_of_results, results, suggestions should always appear
	for _, field := range []string{`"query"`, `"number_of_results"`, `"results"`, `"suggestions"`} {
		if !strings.Contains(raw, field) {
			t.Errorf("expected %s to be present, got: %s", field, raw)
		}
	}
}

func TestSearchResponse_MarshalJSON_DebugIncludesUnresponsiveEngines(t *testing.T) {
	t.Parallel()

	resp := searxng.SearchResponse{
		Query:               "test",
		NumberOfResults:     1,
		Results:             []searxng.SearchResult{{Title: "R1", URL: "https://example.com", Engine: "google"}},
		Suggestions:         []string{},
		UnresponsiveEngines: [][]string{{"brave", `Suspended:" too many "requests`}, {"startpage", `Suspended:" "CAPTCHA`}},
		Debug:               true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var decoded map[string]any

	err = json.Unmarshal(data, &decoded)
	if err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}

	value, ok := decoded["unresponsive_engines"]
	if !ok {
		t.Fatalf("expected unresponsive_engines in debug JSON, got: %s", string(data))
	}

	entries, ok := value.([]any)
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 unresponsive engine entries, got: %#v", value)
	}
}
