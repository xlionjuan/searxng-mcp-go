package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- performSearch tests ---

func TestPerformSearch_CfgNil(t *testing.T) {
	ctx := t.Context()
	_, err := performSearch(ctx, nil, &SearchArgs{Query: "test"})

	if err == nil {
		t.Fatal("expected error for cfg == nil, got nil")
	}
	if !strings.Contains(err.Error(), "cfg cannot be nil") {
		t.Errorf("expected cfg cannot be nil error, got: %v", err)
	}
}

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

// --- isPrivateHost tests ---

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		isPrivate bool
	}{
		// IPv4 private ranges (10.x)
		{"10.0.0.0", "10.0.0.0", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"10.1.2.3", "10.1.2.3", true},

		// IPv4 private ranges (172.16-31.x)
		{"172.16.0.0", "172.16.0.0", true},
		{"172.16.0.1", "172.16.0.1", true},
		{"172.31.255.255", "172.31.255.255", true},
		{"172.20.1.1", "172.20.1.1", true},
		{"172.30.0.1", "172.30.0.1", true},

		// Outside 172.16-31 range
		{"172.15.255.255", "172.15.255.255", false},
		{"172.32.0.1", "172.32.0.1", false},

		// IPv4 private ranges (192.168.x)
		{"192.168.0.0", "192.168.0.0", true},
		{"192.168.1.1", "192.168.1.1", true},
		{"192.168.255.255", "192.168.255.255", true},

		// 192.168.x is private, other 192.x is not
		{"192.169.0.1", "192.169.0.1", false},

		// IPv4 loopback (127.x)
		{"127.0.0.0", "127.0.0.0", true},
		{"127.0.0.1", "127.0.0.1", true},
		{"127.255.255.255", "127.255.255.255", true},

		// IPv4 link-local (169.254.x)
		{"169.254.0.0", "169.254.0.0", true},
		{"169.254.1.2", "169.254.1.2", true},
		{"169.254.255.255", "169.254.255.255", true},

		// 169.254.x is link-local, other 169.x is not
		{"169.255.0.1", "169.255.0.1", false},

		// Public IPs
		{"8.8.8.8", "8.8.8.8", false},
		{"1.1.1.1", "1.1.1.1", false},
		{"93.184.216.34", "93.184.216.34", false},

		// IPv6 loopback
		{"::1", "::1", true},

		// IPv6 unique local (fc00::/7)
		{"fc00::1", "fc00::1", true},
		{"fd00::1", "fd00::1", true},
		{"fe00::1", "fe00::1", false},

		// IPv6 link-local (fe80::/10)
		{"fe80::1", "fe80::1", true},
		{"fe80::ffff", "fe80::ffff", true},
		{"fea0::1", "fea0::1", true},
		{"feb0::1", "feb0::1", true},
		{"fec0::1", "fec0::1", false},

		// IPv6 public
		{"2001:4860:4860::8888", "2001:4860:4860::8888", false},
		{"2606:4700:4700::1111", "2606:4700:4700::1111", false},

		// TLD-based private domains
		{"server.lan", "server.lan", true},
		{"host.internal", "host.internal", true},
		{"machine.local", "machine.local", true},
		{"router.home", "router.home", true},
		{"EXAMPLE.LAN", "EXAMPLE.LAN", true},

		// Non-private domains
		{"example.com", "example.com", false},
		{"google.com", "google.com", false},
		{"search.example.org", "search.example.org", false},

		// Hosts with ports (should be parsed and checked)
		{"192.168.1.1:8080", "192.168.1.1:8080", true},
		{"10.0.0.1:443", "10.0.0.1:443", true},
		{"server.lan:3000", "server.lan:3000", true},
		{"example.com:443", "example.com:443", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrivateHost(tt.host)
			if got != tt.isPrivate {
				t.Errorf("isPrivateHost(%q) = %v, want %v", tt.host, got, tt.isPrivate)
			}
		})
	}
}

// --- validateBaseURL tests ---

func TestValidateBaseURL(t *testing.T) {
	tests := []struct {
		name      string
		baseURL   string
		wantErr   bool
		errSubstr string
	}{
		// Valid URLs
		{"https URL", "https://search.example.com", false, ""},
		{"http URL", "http://search.example.com", false, ""},
		{"https with port", "https://search.example.com:8080", false, ""},
		{"https with path", "https://search.example.com/search", false, ""},
		{"IP address https", "https://192.168.1.1", false, ""},
		{"localhost https", "https://localhost", false, ""},

		// Invalid: empty
		{"empty", "", true, "cannot be empty"},

		// Invalid: no scheme
		{"no scheme", "search.example.com", true, "http or https scheme"},
		{"ftp scheme", "ftp://search.example.com", true, "http or https scheme"},
		{"file scheme", "file:///etc/passwd", true, "http or https scheme"},

		// Invalid: parse error
		{"invalid URL chars", "https://not a valid url", true, "invalid URL"},
		{"just spaces", "   ", true, "http or https scheme"},

		// Invalid: missing host
		{"https:///", "https:///", true, "must include a host"},
		{"http:///", "http:///", true, "must include a host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBaseURL(tt.baseURL)
			if tt.wantErr {
				if err == nil {
					t.Errorf("validateBaseURL(%q) expected error, got nil", tt.baseURL)
					return
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validateBaseURL(%q) error %q does not contain %q", tt.baseURL, err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("validateBaseURL(%q) unexpected error: %v", tt.baseURL, err)
				}
			}
		})
	}
}

// --- getCachedHTTPClient tests ---

func TestGetCachedHTTPClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		timeout time.Duration
	}{
		{"basic https", "https://search.example.com", 30 * time.Second},
		{"basic http", "http://search.example.com", 30 * time.Second},
		{"different timeout", "https://search.example.com", 60 * time.Second},
		{"different URL", "https://other.example.com", 30 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpClientCache = sync.Map{}
			os.Unsetenv("INSECURE_SKIP_VERIFY")

			client1 := getCachedHTTPClient(tt.baseURL, tt.timeout)
			client2 := getCachedHTTPClient(tt.baseURL, tt.timeout)

			if client1 != client2 {
				t.Errorf("getCachedHTTPClient(%q, %v) did not return same instance for same params", tt.baseURL, tt.timeout)
			}
		})
	}
}

func TestGetCachedHTTPClient_CacheKeyUniqueness(t *testing.T) {
	// Clear cache before test
	httpClientCache = sync.Map{}
	os.Unsetenv("INSECURE_SKIP_VERIFY")

	timeout := 30 * time.Second

	// Different base URLs should get different clients
	client1 := getCachedHTTPClient("https://search1.example.com", timeout)
	client2 := getCachedHTTPClient("https://search2.example.com", timeout)

	if client1 == client2 {
		t.Errorf("Different URLs should produce different cached clients")
	}

	// Different timeouts should get different clients
	client3 := getCachedHTTPClient("https://search.example.com", 30*time.Second)
	client4 := getCachedHTTPClient("https://search.example.com", 60*time.Second)

	if client3 == client4 {
		t.Errorf("Different timeouts should produce different cached clients")
	}
}

func TestGetCachedHTTPClient_InsecureSkipVerify(t *testing.T) {
	// Clear cache before test
	httpClientCache = sync.Map{}

	// Test without INSECURE_SKIP_VERIFY
	os.Unsetenv("INSECURE_SKIP_VERIFY")
	client1 := getCachedHTTPClient("https://search.example.com", 30*time.Second)

	// Test with INSECURE_SKIP_VERIFY=1
	os.Setenv("INSECURE_SKIP_VERIFY", "1")
	client2 := getCachedHTTPClient("https://search.example.com", 30*time.Second)

	// Should get different clients
	if client1 == client2 {
		t.Errorf("INSECURE_SKIP_VERIFY should produce different client")
	}

	// Verify the transport settings
	transport1 := client1.Transport.(*http.Transport)
	transport2 := client2.Transport.(*http.Transport)

	if transport1.TLSClientConfig != nil && transport1.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("Client without env var should not have InsecureSkipVerify")
	}

	if transport2.TLSClientConfig == nil || !transport2.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("Client with INSECURE_SKIP_VERIFY=1 should have InsecureSkipVerify")
	}

	// Clean up
	os.Unsetenv("INSECURE_SKIP_VERIFY")
	httpClientCache = sync.Map{}

	// Test INSECURE_SKIP_VERIFY=true (string "true" should also work)
	os.Setenv("INSECURE_SKIP_VERIFY", "true")
	client3 := getCachedHTTPClient("https://search.example.com", 30*time.Second)
	transport3 := client3.Transport.(*http.Transport)
	if transport3.TLSClientConfig == nil || !transport3.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("Client with INSECURE_SKIP_VERIFY=true should have InsecureSkipVerify")
	}

	os.Unsetenv("INSECURE_SKIP_VERIFY")
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
