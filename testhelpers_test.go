package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

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

	defer func() { _ = s.Close() }()

	return s.Search(ctx, args)
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
		_, _ = w.Write(body)
	}))
}
