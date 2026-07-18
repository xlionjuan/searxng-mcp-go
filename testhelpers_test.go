package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"searxng-mcp-go/internal/searxng"
	"searxng-mcp-go/internal/testhelper"
)

// errTestConnectionReset is a static test error used in retry and cancellation tests.
var errTestConnectionReset = errors.New("connection reset")

// testPerformSearch is a test helper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// It is only available in tests.
func testPerformSearch(
	ctx context.Context, tb testing.TB, cfg *searxng.Config,
	args *searxng.SearchArgs,
) (*searxng.SearchResponse, error) {
	tb.Helper()

	if cfg == nil {
		tb.Fatal("testPerformSearch: cfg cannot be nil")

		return nil, nil //nolint:nilnil // tb.Fatal terminates the goroutine
	}

	s, err := searxng.NewSearXNGSearcher(cfg, false)
	if err != nil {
		return nil, err
	}

	defer func() { _ = s.Close() }() //nolint:errcheck // cleanup in defer; error is non-actionable

	return s.Search(ctx, args)
}

// newFastRetrySearcher creates a SearXNGSearcher with sub-second retry delays
// intended for fast test execution. It bypasses Config validation so the
// retry delay can be set below the production 1-second minimum.
//
//nolint:unparam // baseURL kept as parameter for future test flexibility
func newFastRetrySearcher(
	tb testing.TB, baseURL string, transport testhelper.RoundTripperFunc, maxRetries int,
) *searxng.SearXNGSearcher {
	tb.Helper()

	return searxng.NewFastRetrySearcher(baseURL, transport, maxRetries)
}

func mustMarshalJSON(tb testing.TB, v any) []byte {
	tb.Helper()

	body, err := json.Marshal(v)
	if err != nil {
		tb.Fatalf("json.Marshal() error = %v", err)
	}

	return body
}

// newJSONTestServer creates an httptest.Server that responds with the JSON
// encoding of resp on all requests, using the application/json content type.
func newJSONTestServer(tb testing.TB, resp searxng.SearchResponse) *httptest.Server {
	tb.Helper()

	body := mustMarshalJSON(tb, resp)

	return newJSONRawTestServer(tb, body)
}

// newJSONRawTestServer creates an httptest.Server that responds with body on all
// requests, using the application/json content type.
func newJSONRawTestServer(tb testing.TB, body []byte) *httptest.Server {
	tb.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body) //nolint:errcheck // test fixture write; failure does not affect test outcome
	}))
}
