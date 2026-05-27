package searxng

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- setBrowserHeaders tests ---

func TestSetBrowserHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://search.example.com/search", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	setBrowserHeaders(req)

	expectedHeaders := map[string]string{
		"User-Agent":         "Mozilla/5.0",
		"Accept":             "text/html",
		"Accept-Language":    "en-US",
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Dest":     "document",
		"Sec-Fetch-Site":     "none",
		"Sec-Fetch-User":     "?1",
		"Sec-Ch-Ua":          "Google Chrome",
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": "Linux",
		"Priority":           "u=0, i",
	}

	for key, wantSubstr := range expectedHeaders {
		got := req.Header.Get(key)
		if got == "" {
			t.Fatalf("Header %q is empty, expected to be set", key)
		}

		if !strings.Contains(got, wantSubstr) {
			t.Fatalf("Header %q = %q, want it to contain %q", key, got, wantSubstr)
		}
	}
}

// --- buildSearchRequest tests ---

func TestBuildSearchRequest_BasicParams(t *testing.T) {
	t.Parallel()

	pageno := 2
	searcher := &SearXNGSearcher{
		baseURL: "https://search.example.com",
		client:  http.DefaultClient,
	}

	args := &SearchArgs{
		Query:      "golang testing",
		Language:   "en",
		SafeSearch: 1,
		TimeRange:  "month",
		Categories: "general,news",
		Engines:    "google,bing",
		Pageno:     &pageno,
	}

	req, bodyStr, err := searcher.buildSearchRequest(context.Background(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	if req == nil {
		t.Fatal("buildSearchRequest() req = nil")

		return
	}

	if req.Method != http.MethodPost {
		t.Fatalf("Method = %s, want POST", req.Method)
	}

	if req.URL.String() != "https://search.example.com/search" {
		t.Fatalf("URL = %s, want https://search.example.com/search", req.URL.String())
	}

	if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		t.Fatalf("Content-Type = %q, want application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
	}

	// Verify body contains expected parameters
	if !strings.Contains(bodyStr, "q=golang+testing") {
		t.Fatalf("body = %q, want to contain 'q=golang+testing'", bodyStr)
	}

	if !strings.Contains(bodyStr, "language=en") {
		t.Fatalf("body = %q, want to contain 'language=en'", bodyStr)
	}

	if !strings.Contains(bodyStr, "safesearch=1") {
		t.Fatalf("body = %q, want to contain 'safesearch=1'", bodyStr)
	}

	if !strings.Contains(bodyStr, "time_range=month") {
		t.Fatalf("body = %q, want to contain 'time_range=month'", bodyStr)
	}

	if !strings.Contains(bodyStr, "categories=general%2Cnews") {
		t.Fatalf("body does not contain categories=general%%2Cnews; got %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "engines=google%2Cbing") {
		t.Fatalf("body does not contain engines=google%%2Cbing; got %s", bodyStr)
	}

	if !strings.Contains(bodyStr, "pageno=2") {
		t.Fatalf("body = %q, want to contain 'pageno=2'", bodyStr)
	}

	// Verify browser headers are set
	if req.Header.Get("User-Agent") == "" {
		t.Fatal("User-Agent header is empty, setBrowserHeaders should have been called")
	}
}

func TestBuildSearchRequest_MinimalParams(t *testing.T) {
	t.Parallel()

	searcher := &SearXNGSearcher{
		baseURL: "https://search.example.com/search",
		client:  http.DefaultClient,
	}

	args := &SearchArgs{
		Query: "hello",
	}

	req, bodyStr, err := searcher.buildSearchRequest(context.Background(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	if !strings.Contains(bodyStr, "q=hello") {
		t.Fatalf("body = %q, want to contain 'q=hello'", bodyStr)
	}

	// Should NOT contain optional params
	if strings.Contains(bodyStr, "language=") {
		t.Fatalf("body = %q, should not contain language", bodyStr)
	}

	if strings.Contains(bodyStr, "time_range=") {
		t.Fatalf("body = %q, should not contain time_range", bodyStr)
	}

	// safesearch should always be present (defaults to 0)
	if !strings.Contains(bodyStr, "safesearch=0") {
		t.Fatalf("body = %q, want to contain 'safesearch=0'", bodyStr)
	}

	// format should always be present
	if !strings.Contains(bodyStr, "format=json") {
		t.Fatalf("body = %q, want to contain 'format=json'", bodyStr)
	}

	// URL should append /search when path already ends with /search
	if req.URL.String() != "https://search.example.com/search" {
		t.Fatalf("URL = %s, want https://search.example.com/search", req.URL.String())
	}
}

func TestBuildSearchRequest_URLPathHandling(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		wantURL string
	}{
		{
			name:    "path ends with /search",
			baseURL: "https://search.example.com/search",
			wantURL: "https://search.example.com/search",
		},
		{
			name:    "path ends with slash",
			baseURL: "https://search.example.com/searxng/",
			wantURL: "https://search.example.com/searxng/search",
		},
		{
			name:    "no trailing slash",
			baseURL: "https://search.example.com/searxng",
			wantURL: "https://search.example.com/searxng/search",
		},
		{
			name:    "root path",
			baseURL: "https://search.example.com",
			wantURL: "https://search.example.com/search",
		},
		{
			name:    "root path with trailing slash",
			baseURL: "https://search.example.com/",
			wantURL: "https://search.example.com/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			searcher := &SearXNGSearcher{
				baseURL: tt.baseURL,
				client:  http.DefaultClient,
			}

			req, _, err := searcher.buildSearchRequest(context.Background(), &SearchArgs{Query: "test"})
			if err != nil {
				t.Fatalf("buildSearchRequest() error = %v", err)
			}

			if req.URL.String() != tt.wantURL {
				t.Fatalf("URL = %s, want %s", req.URL.String(), tt.wantURL)
			}
		})
	}
}

func TestBuildSearchRequest_WithNilPageno(t *testing.T) {
	t.Parallel()

	searcher := &SearXNGSearcher{
		baseURL: "https://search.example.com",
		client:  http.DefaultClient,
	}

	args := &SearchArgs{
		Query: "test",
	}

	_, bodyStr, err := searcher.buildSearchRequest(context.Background(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	// With nil pageno, pageno should not be in the body
	if strings.Contains(bodyStr, "pageno") {
		t.Fatalf("body = %q, should not contain pageno when nil", bodyStr)
	}
}

func TestBuildSearchRequest_ErrorCases(t *testing.T) {
	t.Parallel()

	t.Run("invalid base URL", func(t *testing.T) {
		t.Parallel()

		searcher := &SearXNGSearcher{
			baseURL: "://invalid",
			client:  http.DefaultClient,
		}

		_, _, err := searcher.buildSearchRequest(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("buildSearchRequest() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "invalid SearXNG URL") {
			t.Fatalf("error = %q, want to contain 'invalid SearXNG URL'", err.Error())
		}
	})
}

func TestBuildSearchRequest_TimeoutConfig(t *testing.T) {
	t.Parallel()

	searcher := &SearXNGSearcher{
		baseURL: "https://search.example.com",
		client:  &http.Client{Timeout: 5 * time.Second},
	}

	args := &SearchArgs{Query: "test"}

	req, _, err := searcher.buildSearchRequest(context.Background(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	if req.Context().Err() != nil {
		t.Fatal("request context should not be canceled")
	}
}
