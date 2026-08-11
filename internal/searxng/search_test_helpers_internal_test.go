package searxng

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
)

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

// newTestSearcher creates a SearXNGSearcher through the canonical constructor
// and then installs a deterministic retry strategy for internal white-box tests.
func newTestSearcher(t *testing.T, rt testhelper.RoundTripperFunc, maxRetries int) *SearXNGSearcher {
	t.Helper()

	s, err := NewSearXNGSearcher(&Config{
		SearXNGURL: "https://search.example.com",
		HTTPClient: &http.Client{Transport: rt},
		MaxRetries: maxRetries,
	}, false)
	if err != nil {
		t.Fatalf("NewSearXNGSearcher() error = %v", err)
	}

	t.Cleanup(func() {
		err := s.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	s.retryStrategy = newExponentialBackoffStrategy(maxRetries, time.Microsecond, time.Microsecond)

	return s
}

// newRequestTestSearcher creates a SearXNGSearcher through the canonical
// constructor for tests that exercise buildSearchRequest.
func newRequestTestSearcher(t *testing.T, baseURL string) *SearXNGSearcher {
	t.Helper()

	s, err := NewSearXNGSearcher(&Config{
		SearXNGURL: baseURL,
		HTTPClient: http.DefaultClient,
	}, false)
	if err != nil {
		t.Fatalf("NewSearXNGSearcher() error = %v", err)
	}

	t.Cleanup(func() {
		err := s.Close()
		if err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})

	return s
}

// closeBody is a helper to close response bodies in tests.
func closeBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close() //nolint:errcheck // test cleanup; error is non-actionable
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
