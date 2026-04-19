package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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
		if r.Method == "POST" {
			r.ParseForm()
			capturedQuery = r.PostForm
		} else {
			capturedQuery = r.URL.Query()
		}
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

func TestPerformSearch_PreservesUnresponsiveEngines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"query":"test","number_of_results":1,"results":[{"title":"Result 1","url":"https://example.com/1","content":"Content 1","engine":"google"}],"suggestions":[],"unresponsive_engines":[["brave","Suspended:\" too many \"requests"],["startpage","Suspended:\" \"CAPTCHA"]]}`))
	}))
	defer server.Close()

	cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
	ctx := t.Context()
	result, err := performSearch(ctx, cfg, &SearchArgs{Query: "test"})
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

func TestPerformSearch_TimeRangeParam(t *testing.T) {
	var capturedTimeRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			capturedTimeRange = r.PostFormValue("time_range")
		} else {
			capturedTimeRange = r.URL.Query().Get("time_range")
		}
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
		if r.Method == "POST" {
			capturedLanguage = r.PostFormValue("language")
		} else {
			capturedLanguage = r.URL.Query().Get("language")
		}
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
	args := &SearchArgs{Query: "test"} // Language is empty, should not be sent to SearXNG

	ctx := t.Context()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLanguage != "" {
		t.Errorf("expected no language param when empty, got %q", capturedLanguage)
	}
}

func TestPerformSearch_OptionalParams(t *testing.T) {
	var capturedParams url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			r.ParseForm()
			capturedParams = r.PostForm
		} else {
			capturedParams = r.URL.Query()
		}
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

func TestPerformSearch_SearchPathNormalization(t *testing.T) {
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
			var gotPath string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				searchResp := SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "test"}
				body, _ := json.Marshal(searchResp)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				w.Write(body)
			}))
			defer server.Close()

			cfg := &Config{SearXNGURL: server.URL + tt.baseURL, Timeout: 30 * time.Second}
			_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if gotPath != tt.wantPath {
				t.Fatalf("request path = %q, want %q", gotPath, tt.wantPath)
			}
		})
	}
}

func TestPerformSearch_UnsupportedBodySizes(t *testing.T) {
	t.Run("oversized error body", func(t *testing.T) {
		body := strings.Repeat("e", MaxErrorBodySize+1)

		cfg := &Config{
			SearXNGURL: "https://example.com",
			Timeout:    30 * time.Second,
			HTTPClient: newStaticResponseClient(http.StatusInternalServerError, "text/plain", body),
		}
		_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var searxngErr *SearXNGError
		if !errors.As(err, &searxngErr) {
			t.Fatalf("expected *SearXNGError, got %T", err)
		}
		if !strings.Contains(err.Error(), "error response body exceeded maximum size limit") {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(searxngErr.ResponseBody) != MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want %d", len(searxngErr.ResponseBody), MaxErrorDisplayChars)
		}
	})

	t.Run("oversized success body", func(t *testing.T) {
		body := strings.Repeat("s", MaxResponseBodySize+1)

		cfg := &Config{
			SearXNGURL: "https://example.com",
			Timeout:    30 * time.Second,
			HTTPClient: newStaticResponseClient(http.StatusOK, "application/json", body),
		}
		_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var searxngErr *SearXNGError
		if !errors.As(err, &searxngErr) {
			t.Fatalf("expected *SearXNGError, got %T", err)
		}
		if !strings.Contains(err.Error(), "response body exceeded maximum size limit") {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(searxngErr.ResponseBody) != MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want %d", len(searxngErr.ResponseBody), MaxErrorDisplayChars)
		}
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func newStaticResponseClient(statusCode int, contentType, body string) *http.Client {
	return &http.Client{
		Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			headers := make(http.Header)
			headers.Set("Content-Type", contentType)
			return &http.Response{
				StatusCode: statusCode,
				Header:     headers,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		}),
	}
}

func TestPerformSearch_EmptyHTMLBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
	_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var htmlErr *HTMLResponseError
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

		// RFC 6890 special-purpose IPv4 ranges
		{"0.0.0.0", "0.0.0.0", true},
		{"0.255.255.255", "0.255.255.255", true},
		{"1.0.0.0", "1.0.0.0", false},
		{"100.64.0.0", "100.64.0.0", true},
		{"100.127.255.255", "100.127.255.255", true},
		{"100.63.255.255", "100.63.255.255", false},
		{"100.128.0.0", "100.128.0.0", false},
		{"192.0.0.0", "192.0.0.0", true},
		{"192.0.0.255", "192.0.0.255", true},
		{"192.0.1.0", "192.0.1.0", false},
		{"192.0.2.1", "192.0.2.1", true},
		{"198.18.0.0", "198.18.0.0", true},
		{"198.19.255.255", "198.19.255.255", true},
		{"198.17.255.255", "198.17.255.255", false},
		{"198.20.0.0", "198.20.0.0", false},
		{"198.51.100.1", "198.51.100.1", true},
		{"203.0.113.1", "203.0.113.1", true},
		{"224.0.0.1", "224.0.0.1", true},
		{"239.255.255.255", "239.255.255.255", true},
		{"240.0.0.1", "240.0.0.1", true},
		{"255.255.255.255", "255.255.255.255", true},
		{"223.255.255.255", "223.255.255.255", false},

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

// --- getDefaultHTTPClient tests ---

func TestGetDefaultHTTPClient_Singleton(t *testing.T) {
	client1 := getDefaultHTTPClient()
	client2 := getDefaultHTTPClient()

	if client1 != client2 {
		t.Errorf("getDefaultHTTPClient() did not return same instance")
	}
}

func TestGetDefaultHTTPClient_Transport(t *testing.T) {
	client := getDefaultHTTPClient()
	transport := client.Transport.(*http.Transport)

	if transport.TLSClientConfig != nil && transport.TLSClientConfig.InsecureSkipVerify {
		t.Errorf("Default client should not have InsecureSkipVerify")
	}
}

// --- helper functions and test utilities ---

func intPtr(i int) *int {
	return &i
}

// Test that performSearch properly encodes query parameters
func TestPerformSearch_QueryEncoding(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			capturedQuery = r.PostFormValue("q")
		} else {
			capturedQuery = r.URL.Query().Get("q")
		}
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
		{"valid URL", "https://search.example.com", false, ""},
		{"invalid URL is wrapped", "search.example.com", true, "NewSearXNGSearcher: url must use http or https scheme"},
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

func TestSearXNGSearcher_Close_Idempotent(t *testing.T) {
	t.Run("nil client", func(t *testing.T) {
		searcher := &SearXNGSearcher{client: nil, baseURL: "https://example.com"}
		searcher.Close()
		searcher.Close()
	})

	t.Run("shared default client", func(t *testing.T) {
		searcher := &SearXNGSearcher{client: getDefaultHTTPClient(), baseURL: "https://example.com"}
		searcher.Close()
		searcher.Close()
	})

	t.Run("custom client", func(t *testing.T) {
		customClient := &http.Client{Timeout: 30 * time.Second}
		searcher := &SearXNGSearcher{client: customClient, baseURL: "https://example.com"}
		searcher.Close()
		searcher.Close()
	})
}

func TestPerformSearch_POSTtoGETFallback(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "405 fallback", statusCode: http.StatusMethodNotAllowed},
		{name: "501 fallback", statusCode: http.StatusNotImplemented},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var postReq *http.Request
			var getReq *http.Request
			searchResp := SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "test"}
			body, _ := json.Marshal(searchResp)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					postReq = r
					w.WriteHeader(tt.statusCode)
					return
				}
				if r.Method == "GET" {
					getReq = r
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					w.Write(body)
					return
				}
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
			args := &SearchArgs{Query: "test search", Language: "en", SafeSearch: 1}

			result, err := performSearch(t.Context(), cfg, args)
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
			if getQuery.Get("q") != "test search" || getQuery.Get("format") != "json" || getQuery.Get("language") != "en" || getQuery.Get("safesearch") != "1" {
				t.Fatalf("unexpected GET query params: %v", getQuery)
			}
		})
	}
}

func TestPerformSearch_BrowserHeaders(t *testing.T) {
	t.Run("POST request headers", func(t *testing.T) {
		var capturedHeaders http.Header
		searchResp := SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "test"}
		body, _ := json.Marshal(searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		}))
		defer server.Close()

		cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
		_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
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
		var capturedHeaders http.Header
		searchResp := SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "test"}
		body, _ := json.Marshal(searchResp)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			capturedHeaders = r.Header
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
		}))
		defer server.Close()

		cfg := &Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}
		_, err := performSearch(t.Context(), cfg, &SearchArgs{Query: "test"})
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

// --- deduplicateAnswers tests ---

func TestDeduplicateAnswers_EmptyInputs(t *testing.T) {
	// Both empty
	result := deduplicateAnswers(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Empty answers
	result = deduplicateAnswers(nil, []Infobox{{Infobox: "test", Content: "content"}})
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Empty infoboxes
	answers := []Answer{{Answer: "test", Engine: "duckduckgo"}}
	result = deduplicateAnswers(answers, nil)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestDeduplicateAnswers_RemovesDuplicateWikipedia(t *testing.T) {
	// Simulate DuckDuckGo putting Wikipedia summary in both answers and infoboxes
	wikiSummary := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."
	answers := []Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: wikiSummary},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (duplicate removed), got %d: %+v", len(result), result)
	}
}

func TestDeduplicateAnswers_RemovesPrefixMatch(t *testing.T) {
	// Answer is a prefix of infobox content (truncated answer)
	answers := []Answer{
		{Answer: "Apple Inc. is an American multinational technology company", Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (prefix match removed), got %d", len(result))
	}
}

func TestDeduplicateAnswers_KeepsDistinctAnswer(t *testing.T) {
	// "ip" query: answer is an IP address, infobox has unrelated content
	answers := []Answer{
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	infoboxes := []Infobox{
		{Infobox: "IP Address", Content: "An Internet Protocol address is a numerical label."},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (distinct answer kept), got %d", len(result))
	}
	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_CaseInsensitive(t *testing.T) {
	answers := []Answer{
		{Answer: "apple inc. is an american company", Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: "Apple Inc. is an American company headquartered in California."},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (case-insensitive match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_InfoboxContentOnly(t *testing.T) {
	// Infobox with empty content should not cause filtering
	answers := []Answer{
		{Answer: "test answer", Engine: "test"},
	}
	infoboxes := []Infobox{
		{Infobox: "Test", Content: ""},
		{Infobox: "Test2", Content: ""},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (no content to match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_MultipleAnswersMixed(t *testing.T) {
	wikiSummary := "Apple Inc. is an American multinational technology company."
	answers := []Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: wikiSummary},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (only IP answer kept), got %d", len(result))
	}
	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_DDGSuffixMoreAtWikipedia(t *testing.T) {
	// DuckDuckGo appends "More at Wikipedia" to the answer, which breaks
	// the old Contains(answer, infobox) check. Prefix matching fixes this.
	infoboxContent := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California. Apple is one of the Big Tech companies, alongside Amazon, Google, Meta, and Microsoft."
	answer := infoboxContent + " More at Wikipedia"
	answers := []Answer{
		{Answer: answer, Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: infoboxContent},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (DDG answer with 'More at Wikipedia' suffix should be deduplicated), got %d: %+v", len(result), result)
	}
}

func TestDeduplicateAnswers_EmptyAnswerSkipped(t *testing.T) {
	answers := []Answer{
		{Answer: "", Engine: "duckduckgo"},
		{Answer: "valid answer", Engine: "test"},
	}
	infoboxes := []Infobox{
		{Infobox: "Test", Content: "some content"},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
	if result[0].Answer != "valid answer" {
		t.Errorf("expected 'valid answer', got %q", result[0].Answer)
	}
}

// --- SearchResponse.MarshalJSON tests ---

func TestSearchResponse_MarshalJSON_NilSlices(t *testing.T) {
	resp := SearchResponse{
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
	resp := SearchResponse{
		Query:           "test",
		Answers:         []Answer{{Answer: "42", Engine: "calc"}},
		NumberOfResults: 1,
		Infoboxes:       []Infobox{{Infobox: "Info", Content: "content"}},
		Results:         []SearchResult{{Title: "R1", URL: "https://example.com", Engine: "google"}},
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
	resp := SearchResponse{
		Query:           "test",
		NumberOfResults: 0,
		Results:         []SearchResult{},
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
	resp := SearchResponse{
		Query:               "test",
		NumberOfResults:     1,
		Results:             []SearchResult{{Title: "R1", URL: "https://example.com", Engine: "google"}},
		Suggestions:         []string{},
		UnresponsiveEngines: [][]string{{"brave", `Suspended:" too many "requests`}, {"startpage", `Suspended:" "CAPTCHA`}},
		Debug:               true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("MarshalJSON error: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
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
