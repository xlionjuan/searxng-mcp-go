package main

import (
	"context"
	"encoding/json"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// testPerformSearch is a test helper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// It is only available in tests.
func testPerformSearch(
	ctx context.Context, tb testing.TB, cfg *searxng.Config,
	args *searxng.SearchArgs,
) (*searxng.SearchResponse, error) {
	tb.Helper()

	if cfg == nil {
		return nil, searxng.NewSearXNGError(0, "", "", errTestConfigRequired)
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
