package searxng //nolint:testpackage // white-box access to internal types for shared test helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// roundTripperFunc adapts a function to the http.RoundTripper interface.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// errTransportNotExpected is returned by mock transports that should not be called.
var errTransportNotExpected = errors.New("transport should not be called in this test")

// makeJSONResponse creates an http.Response with a JSON body and OK status.
func makeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// minimalJSONBody is the body content for the simplest valid SearchResponse JSON.
const minimalJSONBody = `{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`

// makeSearchResponseJSON creates body content for a SearchResponse JSON with results.
func makeSearchResponseJSON(results int) string {
	rs := make([]string, 0, results)

	for i := range results {
		title := fmt.Sprintf("Result %d", i)
		urlStr := fmt.Sprintf("https://example.com/%d", i)
		rs = append(rs, `{"title":"`+title+`","url":"`+urlStr+`","content":"content","engine":"google"}`)
	}

	r := `{"query":"test","results":[` + strings.Join(rs, ",") + `],"suggestions":[],"answers":[],"infoboxes":[]}`

	return r
}

// newTestSearcher creates a SearXNGSearcher with a mock RoundTripper for HTTP-level tests.
func newTestSearcher(t *testing.T, rt roundTripperFunc, maxRetries int) *SearXNGSearcher {
	t.Helper()

	endpoint, err := computeSearchEndpoint("https://search.example.com")
	if err != nil {
		t.Fatalf("computeSearchEndpoint() error = %v", err)
	}

	s := &SearXNGSearcher{
		client: &http.Client{
			Transport: rt,
		},
		searchEndpoint: endpoint,
		debug:          false,
		retryStrategy:  newExponentialBackoffStrategy(maxRetries, time.Microsecond, time.Microsecond),
		ownsTransport:  true,
	}
	s.baseCtx, s.cancel = context.WithCancel(context.Background())
	return s
}

// newRequestTestSearcher creates a SearXNGSearcher with a precomputed search
// endpoint derived from baseURL. It is intended for tests that exercise
// buildSearchRequest without going through NewSearXNGSearcher.
func newRequestTestSearcher(t *testing.T, baseURL string) *SearXNGSearcher {
	t.Helper()

	endpoint, err := computeSearchEndpoint(baseURL)
	if err != nil {
		t.Fatalf("computeSearchEndpoint(%q) error = %v", baseURL, err)
	}

	return &SearXNGSearcher{
		searchEndpoint: endpoint,
		client:         http.DefaultClient,
	}
}

// closeBody is a helper to close response bodies in tests.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// errorReadFailed is a static error for read failure tests.
var errReadFailed = errors.New("read failure")

// errorReader always fails to read.
type errorReader struct{}

func (e *errorReader) Read(_ []byte) (int, error) {
	return 0, errReadFailed
}

func (e *errorReader) Close() error {
	return nil
}
