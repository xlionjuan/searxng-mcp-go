package main

import (
	"context"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// testPerformSearch is a test helper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// It is only available in tests.
func testPerformSearch(t testing.TB, ctx context.Context, cfg *searxng.Config, args *searxng.SearchArgs) (*searxng.SearchResponse, error) {
	if cfg == nil {
		return nil, searxng.NewSearXNGError(0, "", "", errTestConfigRequired)
	}
	s, err := searxng.NewSearXNGSearcher(cfg, false)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return s.Search(ctx, args)
}
