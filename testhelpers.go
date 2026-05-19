package main

import (
	"context"
	"fmt"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// testPerformSearch is a test helper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// It is only available in tests.
func testPerformSearch(t testing.TB, ctx context.Context, cfg *searxng.Config, args *searxng.SearchArgs) (*searxng.SearchResponse, error) {
	if cfg == nil {
		return nil, searxng.NewSearXNGError(0, "", "", fmt.Errorf("testPerformSearch: cfg cannot be nil"))
	}
	s, err := searxng.NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient, false)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return s.Search(ctx, args)
}
