package searxng

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- setBrowserHeaders tests ---

func TestSetBrowserHeaders(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://search.example.com/search", http.NoBody)
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

//nolint:gocyclo,gocognit // table-driven test with 11 independent assertion cases
func TestBuildSearchRequest_BasicParams(t *testing.T) {
	t.Parallel()

	pageno := 2
	searcher := newRequestTestSearcher(t, "https://search.example.com")

	args := &SearchArgs{
		Query:      "golang testing",
		Language:   "en",
		SafeSearch: 1,
		TimeRange:  "month",
		Categories: "general,news",
		Engines:    "google,bing",
		Pageno:     &pageno,
	}

	req, bodyStr, err := searcher.buildSearchRequest(t.Context(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	if req == nil {
		t.Fatal("buildSearchRequest() req = nil")
	}

	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "HTTP method is POST",
			check: func(t *testing.T) {
				t.Helper()

				if req.Method != http.MethodPost {
					t.Fatalf("Method = %s, want POST", req.Method)
				}
			},
		},
		{
			name: "request URL",
			check: func(t *testing.T) {
				t.Helper()

				if req.URL.String() != "https://search.example.com/search" {
					t.Fatalf("URL = %s, want https://search.example.com/search", req.URL.String())
				}
			},
		},
		{
			name: "Content-Type header",
			check: func(t *testing.T) {
				t.Helper()

				if req.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
					t.Fatalf("Content-Type = %q, want application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
				}
			},
		},
		{
			name: "body contains query parameter",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "q=golang+testing") {
					t.Fatalf("body = %q, want to contain 'q=golang+testing'", bodyStr)
				}
			},
		},
		{
			name: "body contains language parameter",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "language=en") {
					t.Fatalf("body = %q, want to contain 'language=en'", bodyStr)
				}
			},
		},
		{
			name: "body contains safesearch parameter",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "safesearch=1") {
					t.Fatalf("body = %q, want to contain 'safesearch=1'", bodyStr)
				}
			},
		},
		{
			name: "body contains time_range parameter",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "time_range=month") {
					t.Fatalf("body = %q, want to contain 'time_range=month'", bodyStr)
				}
			},
		},
		{
			name: "body contains categories parameter (URL-encoded)",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "categories=general%2Cnews") {
					t.Fatalf("body does not contain categories=general%%2Cnews; got %s", bodyStr)
				}
			},
		},
		{
			name: "body contains engines parameter (URL-encoded)",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "engines=google%2Cbing") {
					t.Fatalf("body does not contain engines=google%%2Cbing; got %s", bodyStr)
				}
			},
		},
		{
			name: "body contains pageno parameter",
			check: func(t *testing.T) {
				t.Helper()

				if !strings.Contains(bodyStr, "pageno=2") {
					t.Fatalf("body = %q, want to contain 'pageno=2'", bodyStr)
				}
			},
		},
		{
			name: "User-Agent header is set",
			check: func(t *testing.T) {
				t.Helper()

				if req.Header.Get("User-Agent") == "" {
					t.Fatal("User-Agent header is empty, setBrowserHeaders should have been called")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, tt.check)
	}
}

func TestBuildSearchRequest_MinimalParams(t *testing.T) {
	t.Parallel()

	searcher := newRequestTestSearcher(t, "https://search.example.com/search")

	args := &SearchArgs{
		Query: "hello",
	}

	req, bodyStr, err := searcher.buildSearchRequest(t.Context(), args)
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

	// URL should be left unchanged when base URL path already ends with /search
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

			searcher := newRequestTestSearcher(t, tt.baseURL)

			req, _, err := searcher.buildSearchRequest(t.Context(), &SearchArgs{Query: "test"})
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

	searcher := newRequestTestSearcher(t, "https://search.example.com")

	args := &SearchArgs{
		Query: "test",
	}

	_, bodyStr, err := searcher.buildSearchRequest(t.Context(), args)
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

	t.Run("search endpoint not precomputed", func(t *testing.T) {
		t.Parallel()

		// Programmer error: a SearXNGSearcher constructed directly (bypassing
		// NewSearXNGSearcher) without setting searchEndpoint. buildSearchRequest
		// must surface this as an error rather than panic.
		searcher := &SearXNGSearcher{}

		_, _, err := searcher.buildSearchRequest(t.Context(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("buildSearchRequest() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "search endpoint not precomputed") {
			t.Fatalf("error = %q, want to contain 'search endpoint not precomputed'", err.Error())
		}
	})
}

func TestBuildSearchRequest_TimeoutConfig(t *testing.T) {
	t.Parallel()

	searcher := newRequestTestSearcher(t, "https://search.example.com")
	searcher.client = &http.Client{Timeout: 5 * time.Second}

	args := &SearchArgs{Query: "test"}

	req, _, err := searcher.buildSearchRequest(t.Context(), args)
	if err != nil {
		t.Fatalf("buildSearchRequest() error = %v", err)
	}

	if req.Context().Err() != nil {
		t.Fatal("request context should not be canceled")
	}
}

// --- computeSearchEndpoint tests ---

func TestComputeSearchEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{
			name:    "path ends with /search",
			baseURL: "https://search.example.com/search",
			want:    "https://search.example.com/search",
		},
		{
			name:    "path ends with slash",
			baseURL: "https://search.example.com/searxng/",
			want:    "https://search.example.com/searxng/search",
		},
		{
			name:    "no trailing slash",
			baseURL: "https://search.example.com/searxng",
			want:    "https://search.example.com/searxng/search",
		},
		{name: "root path", baseURL: "https://search.example.com", want: "https://search.example.com/search"},
		{
			name:    "root path with trailing slash",
			baseURL: "https://search.example.com/",
			want:    "https://search.example.com/search",
		},
		{
			name:    "drops trailing query",
			baseURL: "https://search.example.com/?foo=bar",
			want:    "https://search.example.com/search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := computeSearchEndpoint(tt.baseURL)
			if err != nil {
				t.Fatalf("computeSearchEndpoint(%q) error = %v", tt.baseURL, err)
			}

			if got == nil {
				t.Fatal("computeSearchEndpoint() = nil, want non-nil")
			}

			if got.String() != tt.want {
				t.Fatalf("computeSearchEndpoint(%q) = %q, want %q", tt.baseURL, got.String(), tt.want)
			}

			if got.RawQuery != "" {
				t.Fatalf("RawQuery = %q, want empty", got.RawQuery)
			}
		})
	}
}

func TestComputeSearchEndpoint_ParseError(t *testing.T) {
	t.Parallel()

	_, err := computeSearchEndpoint("://invalid")
	if err == nil {
		t.Fatal("computeSearchEndpoint() error = nil, want parse error")
	}
}

// TestBuildSearchRequest_DoesNotMutatePrecomputedURL verifies that the
// per-request URL returned by buildSearchRequest is a clone, so mutating it
// (as the opt-in GET fallback does when it sets RawQuery) does not leak back
// into s.searchEndpoint.
func TestBuildSearchRequest_DoesNotMutatePrecomputedURL(t *testing.T) {
	t.Parallel()

	searcher := newRequestTestSearcher(t, "https://search.example.com")

	originalURL := searcher.searchEndpoint.String()

	for range 5 {
		req, _, err := searcher.buildSearchRequest(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("buildSearchRequest() error = %v", err)
		}

		// Simulate the opt-in GET fallback path mutating the per-request URL.
		req.URL.RawQuery = "q=test&format=json"
		req.URL.Path = "/mutated"
	}

	if got := searcher.searchEndpoint.String(); got != originalURL {
		t.Fatalf("searchEndpoint mutated: was %q, now %q", originalURL, got)
	}
}
