package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"searxng-mcp-go/internal/searxng"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Error Path Tests - Network Failures
// ============================================================================

// TestSearch_DNSFailure tests that DNS failures are properly wrapped with context.
func TestSearch_DNSFailure(t *testing.T) {
	client := &http.Client{
		Transport: cancelRoundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, &net.DNSError{
				Err:        "no such host",
				Name:       "nonexistent.invalid-domain.test",
				IsNotFound: true,
			}
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "http://example.com",
		Timeout:    5 * time.Second,
		HTTPClient: client,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected DNS failure error, got nil")
	}

	// Verify error is wrapped with context (not just a sentinel error)
	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *searxng.SearXNGError, got type %T: %v", err, err)
	}

	// Verify there's underlying error information
	if searxngErr != nil && searxngErr.UnderlyingErr == nil {
		t.Errorf("SearXNGError.UnderlyingErr is nil for DNS failure")
	}

	// Check error message contains useful context
	if !strings.Contains(err.Error(), "failed to execute search request") {
		t.Errorf("error should contain 'failed to execute search request', got: %v", err)
	}
}

// TestSearch_ConnectionRefused tests connection refused errors.
func TestSearch_ConnectionRefused(t *testing.T) {
	// Create a server and immediately close it to simulate connection refused
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    5 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected connection refused error, got nil")
	}

	// Verify error is wrapped with context
	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *searxng.SearXNGError, got type %T: %v", err, err)
	}

	// Should have underlying error
	if searxngErr != nil && searxngErr.UnderlyingErr == nil {
		t.Errorf("SearXNGError.UnderlyingErr is nil")
	}
}

// ============================================================================
// Error Path Tests - HTTP Status Codes
// ============================================================================

// TestSearch_EmptyResponse tests handling of empty response body.
func TestSearch_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Write empty body
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := testPerformSearch(t, ctx, cfg, args)

	// Empty JSON body should cause a JSON parse error
	if err == nil {
		t.Fatal("expected error for empty response body, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

// TestSearch_EmptyBodyWith200 tests empty JSON object response.
func TestSearch_EmptyJSONObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{}"))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()
	result, err := testPerformSearch(t, ctx, cfg, args)
	// Empty JSON object is technically valid but has no results
	if err != nil {
		t.Errorf("expected no error for empty but valid JSON, got: %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if len(result.Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(result.Results))
	}
}

// TestSearch_UnexpectedContentType tests response with unexpected content type.
func TestSearch_UnexpectedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("plain text response"))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected error for unexpected content type, got nil")
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *searxng.SearXNGError, got type %T: %v", err, err)

		return
	}

	if searxngErr.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d", searxngErr.StatusCode, http.StatusOK)
	}

	if searxngErr.RespContentType != "text/plain" {
		t.Fatalf("RespContentType = %q, want %q", searxngErr.RespContentType, "text/plain")
	}

	if searxngErr.UnderlyingErr == nil || searxngErr.UnderlyingErr.Error() != "unexpected content type: expected application/json" {
		t.Fatalf("UnderlyingErr = %v, want unexpected content type error", searxngErr.UnderlyingErr)
	}

	if got, want := err.Error(),
		"searxng error (status 200) - content-type text/plain: unexpected content type: expected application/json";
		got != want {
		t.Fatalf("error = %q, want %q", got, want)
	}
}

// ============================================================================
// Error Path Tests - Malformed JSON
// ============================================================================

func TestSearch_MalformedJSON_Truncated(t *testing.T) {
	truncatedJSON := []byte(`{"results": [{"title": "test",`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(truncatedJSON)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

func TestSearch_WrongJSONType(t *testing.T) {
	// Valid JSON with wrong types: results should be an array, but we provide a string
	// because "results" is expected to be an array, not a string
	wrongTypeJSON := []byte(`{"results": "not an array", "number_of_results": "not a number"}`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(wrongTypeJSON)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := testPerformSearch(t, ctx, cfg, args)

	// Go's json.Unmarshal will fail when trying to unmarshal string into []SearchResult
	if err == nil {
		t.Fatal("expected JSON parse error for wrong-type JSON, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

func TestSearch_TrailingGarbage(t *testing.T) {
	// Valid JSON followed by garbage
	garbageJSON := []byte(`{"results":[],"number_of_results":0,"query":"test"}TRAILING GARBAGE`)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(garbageJSON)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected JSON parse error for trailing garbage, got nil")
	}

	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

// ============================================================================
// Error Path Tests - Error Wrapping Context
// ============================================================================

func TestSearch_500Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    5 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected 500 error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "searxng error (status 500)") {
		t.Errorf("error %q does not contain expected context 'searxng error (status 500)'", errStr)
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *searxng.SearXNGError, got type %T", err)
	}

	if searxngErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("SearXNGError.StatusCode = %d, want 500", searxngErr.StatusCode)
	}
}

func TestSearch_404Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    5 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected 404 error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "searxng error (status 404)") {
		t.Errorf("error %q does not contain expected context 'searxng error (status 404)'", errStr)
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *searxng.SearXNGError, got type %T", err)
	}

	if searxngErr.StatusCode != http.StatusNotFound {
		t.Errorf("SearXNGError.StatusCode = %d, want 404", searxngErr.StatusCode)
	}
}

func TestSearch_NetworkError_ConnectionClose(t *testing.T) {
	// Create a server and immediately close it to simulate network error
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		// Close immediately without responding
	}))
	server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    5 * time.Second,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx := context.Background()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected network error, got nil")
	}

	// Verify error is wrapped with context
	errStr := err.Error()
	if !strings.Contains(errStr, "failed to execute search request") {
		t.Errorf("error %q does not contain expected context 'failed to execute search request'", errStr)
	}

	// Verify it's a proper wrapped error type
	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *searxng.SearXNGError, got type %T", err)
	}
}

// ============================================================================
// Error Path Tests - Timeout Behavior
// ============================================================================

func TestSearch_TimeoutExceeded(t *testing.T) {
	// Use a custom RoundTripper that blocks until context cancellation,
	// avoiding httptest.Server.Close() blocking on active connections.
	client := &http.Client{
		Transport: cancelRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()

			return nil, r.Context().Err()
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "context deadline exceeded") &&
		!strings.Contains(errStr, "request canceled") &&
		!strings.Contains(errStr, "timeout") &&
		!strings.Contains(errStr, "failed to execute") {
		t.Errorf("expected timeout-related error, got: %v", err)
	}
}

func TestSearch_ContextDeadlineExceeded(t *testing.T) {
	// Use a custom RoundTripper that blocks until context cancellation,
	// avoiding httptest.Server.Close() blocking on active connections.
	client := &http.Client{
		Transport: cancelRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()

			return nil, r.Context().Err()
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}
	args := &searxng.SearchArgs{Query: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := testPerformSearch(t, ctx, cfg, args)
	if err == nil {
		t.Fatal("expected context deadline exceeded error, got nil")
	}

	// Verify error is wrapped
	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Errorf("expected *searxng.SearXNGError, got type %T: %v", err, err)
	}
}

// ============================================================================
// Error Path Tests - HTTP Status Error Messages
// ============================================================================

func TestSearch_HTTPStatusErrors(t *testing.T) {
	tests := []struct {
		statusCode   int
		wantContains string
	}{
		{400, "searxng error (status 400)"},
		{401, "searxng error (status 401)"},
		{403, "searxng error (status 403)"},
		{404, "searxng error (status 404)"},
		{418, "searxng error (status 418)"},
		{429, "searxng error (status 429)"},
		{500, "searxng error (status 500)"},
		{502, "searxng error (status 502)"},
		{503, "searxng error (status 503)"},
		{504, "searxng error (status 504)"},
	}

	for _, tc := range tests {
		t.Run(http.StatusText(tc.statusCode), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.statusCode)
				_, _ = w.Write([]byte(http.StatusText(tc.statusCode)))
			}))
			defer server.Close()

			cfg := &searxng.Config{
				SearXNGURL: server.URL,
				Timeout:    5 * time.Second,
			}
			args := &searxng.SearchArgs{Query: "test"}

			ctx := context.Background()

			_, err := testPerformSearch(t, ctx, cfg, args)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.statusCode)
			}

			if !strings.Contains(err.Error(), tc.wantContains) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantContains)
			}

			var searxngErr *searxng.SearXNGError
			if !errors.As(err, &searxngErr) {
				t.Fatalf("expected *searxng.SearXNGError, got type %T", err)
			}

			if searxngErr.StatusCode != tc.statusCode {
				t.Errorf("SearXNGError.StatusCode = %d, want %d", searxngErr.StatusCode, tc.statusCode)
			}
		})
	}
}

func TestSearch_RedirectStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "/search/redirected")
		w.WriteHeader(http.StatusFound)
		_, _ = w.Write([]byte("redirect"))
	}))
	defer server.Close()

	client := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 5 * time.Second, HTTPClient: client}

	_, err := testPerformSearch(t, context.Background(), cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected redirect error, got nil")
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *searxng.SearXNGError, got %T", err)
	}

	if searxngErr.StatusCode != http.StatusFound {
		t.Fatalf("StatusCode = %d, want %d", searxngErr.StatusCode, http.StatusFound)
	}
}

func TestSearch_CustomHTTPClientWithoutRedirectPolicyBlocksCrossHost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://example.com/search")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 5 * time.Second, HTTPClient: client}

	_, err := testPerformSearch(t, context.Background(), cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected cross-host redirect error, got nil")
	}

	if client.CheckRedirect != nil {
		t.Fatal("NewSearXNGSearcher mutated caller's custom HTTP client")
	}

	if !strings.Contains(err.Error(), "redirect to different host blocked") {
		t.Fatalf("error %q does not contain redirect block message", err.Error())
	}
}

func TestSearch_CustomHTTPClientSameHostRedirectAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search/redirected" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"query":"test","number_of_results":0,"results":[],"suggestions":[]}`))

			return
		}

		w.Header().Set("Location", "/search/redirected")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 5 * time.Second, HTTPClient: client}

	result, err := testPerformSearch(t, context.Background(), cfg, &searxng.SearchArgs{Query: "test"})
	if err != nil {
		t.Fatalf("unexpected error for same-host redirect: %v", err)
	}

	if result.Query != "test" {
		t.Fatalf("Query = %q, want test", result.Query)
	}

	if client.CheckRedirect != nil {
		t.Fatal("NewSearXNGSearcher mutated caller's custom HTTP client")
	}
}

func TestSearch_CustomHTTPClientCrossHostRedirectBlockedBeforeCustomPolicy(t *testing.T) {
	customPolicyCalled := false
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			customPolicyCalled = true

			return nil
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://example.com/search")
		w.WriteHeader(http.StatusFound)
	}))
	defer server.Close()

	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 5 * time.Second, HTTPClient: client}

	_, err := testPerformSearch(t, context.Background(), cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected cross-host redirect error, got nil")
	}

	if customPolicyCalled {
		t.Fatal("custom redirect policy ran before cross-host redirect was blocked")
	}

	if !strings.Contains(err.Error(), "redirect to different host blocked") {
		t.Fatalf("error %q does not contain redirect block message", err.Error())
	}
}

func TestSearch_ConnectionResetMidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Fatal("response writer does not support hijacking")
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Fatalf("Hijack() failed: %v", err)
		}

		defer func() { _ = conn.Close() }()

		_, _ = conn.Write([]byte(
			"HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 128\r\n\r\n{\"query\":\"test\",\"results\":[\"",
		))
	}))
	defer server.Close()

	cfg := &searxng.Config{SearXNGURL: server.URL, Timeout: 5 * time.Second}

	_, err := testPerformSearch(t, context.Background(), cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Fatal("expected connection reset error, got nil")
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *searxng.SearXNGError, got %T", err)
	}

	if searxngErr.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode = %d, want %d", searxngErr.StatusCode, http.StatusOK)
	}

	if searxngErr.UnderlyingErr == nil {
		t.Fatal("expected underlying read error, got nil")
	}
}

// ============================================================================
// Error Path Tests - Verify All Errors Are Wrapped
// ============================================================================

// TestSearch_AllErrorTypesAreWrapped verifies that all error types
// returned by Search are properly wrapped with context.
func TestSearch_AllErrorTypesAreWrapped(t *testing.T) {
	tests := []struct {
		name string
		cfg  *searxng.Config
	}{
		{
			name: "invalid URL scheme",
			cfg:  &searxng.Config{SearXNGURL: "ftp://example.com", Timeout: 30 * time.Second},
		},
		{
			name: "missing host",
			cfg:  &searxng.Config{SearXNGURL: "http://", Timeout: 30 * time.Second},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := &searxng.SearchArgs{Query: "test"}
			ctx := context.Background()

			_, err := testPerformSearch(t, ctx, tt.cfg, args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			// Every error should be a wrapped error with context
			// It should be either a SearXNGError or have context wrapped via %w
			errStr := err.Error()
			if errStr == "" {
				t.Error("error message is empty")
			}

			// For URL errors, should still be wrapped in SearXNGError
			var searxngErr *searxng.SearXNGError
			if errors.As(err, &searxngErr) {
				// Good - it's a proper wrapped error type
				return
			}

			// If not a SearXNGError, it should at least have context via fmt.Errorf
			if !strings.Contains(errStr, ":") && len(errStr) < 10 {
				t.Errorf("error %q appears to be a bare sentinel error, expected wrapped error", errStr)
			}
		})
	}
}
