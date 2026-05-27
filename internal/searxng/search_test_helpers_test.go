package searxng //nolint:testpackage // white-box access to internal types for shared test helpers

import (
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

	return &SearXNGSearcher{
		client: &http.Client{
			Transport: rt,
		},
		baseURL:       "https://search.example.com",
		debug:         false,
		maxRetries:    maxRetries,
		retryStrategy: newExponentialBackoffStrategy(maxRetries, time.Microsecond, time.Microsecond),
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

// AsError is a test helper for errors.As with generics-like assertion.
func AsError[T error](t *testing.T, err error, target *T) bool {
	t.Helper()

	return As(err, target)
}

// As wraps errors.As for test helper compatibility.
func As[T error](err error, target *T) bool {
	//nolint:errorlint // errors.As with generic target
	if e, ok := err.(T); ok {
		*target = e

		return true
	}

	//nolint:errorlint // errors.As with generic target via unwrapping
	for e := err; e != nil; {
		if e2, ok := e.(T); ok {
			*target = e2

			return true
		}

		if unwrapper, ok := e.(interface{ Unwrap() error }); ok {
			e = unwrapper.Unwrap()
		} else {
			break
		}
	}

	return false
}
