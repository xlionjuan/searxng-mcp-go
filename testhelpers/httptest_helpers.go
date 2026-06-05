package testhelpers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// MinimalJSONBody is the body content for the simplest valid SearchResponse JSON.
const MinimalJSONBody = `{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`

const defaultSearcherTimeout = 30

// MakeJSONResponse creates an http.Response with a JSON body and OK status.
func MakeJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// MakeSearchResponseJSON creates body content for a SearchResponse JSON with results.
func MakeSearchResponseJSON(results int) string {
	rs := make([]string, 0, results)

	for i := range results {
		title := fmt.Sprintf("Result %d", i)
		urlStr := fmt.Sprintf("https://example.com/%d", i)
		rs = append(rs, `{"title":"`+title+`","url":"`+urlStr+`","content":"content","engine":"google"}`)
	}

	return `{"query":"test","results":[` + strings.Join(rs, ",") + `],"suggestions":[],"answers":[],"infoboxes":[]}`
}

// NewMockSearchServer creates a mock SearXNG server that returns the given response.
func NewMockSearchServer(t *testing.T, response *searxng.SearchResponse) *httptest.Server {
	t.Helper()

	body, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

// NewMockSearchServerWithHandler creates a mock SearXNG server with a custom handler.
func NewMockSearchServerWithHandler(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()

	return httptest.NewServer(handler)
}

// NewTestSearcherWithMockServer creates a SearXNGSearcher pointing to a mock server.
func NewTestSearcherWithMockServer(t *testing.T, server *httptest.Server) *searxng.SearXNGSearcher {
	t.Helper()

	searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: server.URL, Timeout: defaultSearcherTimeout}, false)
	if err != nil {
		t.Fatalf("failed to create searcher: %v", err)
	}

	return searcher
}

// CloseBody is a helper to close response bodies in tests.
func CloseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// ErrorReader always fails to read.
type ErrorReader struct{}

var errReadFailed = &testError{}

type testError struct{}

func (e *testError) Error() string {
	return "read failure"
}

func (e *ErrorReader) Read(_ []byte) (int, error) {
	return 0, errReadFailed
}

// Close implements io.Closer for ErrorReader.
func (e *ErrorReader) Close() error {
	return nil
}
