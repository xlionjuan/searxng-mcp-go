package main

import (
	"context"
	"fmt"
	"testing"
)

// testPerformSearch is a test helper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// It is only available in tests.
func testPerformSearch(t testing.TB, ctx context.Context, cfg *Config, args *SearchArgs) (*SearchResponse, error) {
	if cfg == nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("testPerformSearch: cfg cannot be nil"))
	}
	s, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	return s.Search(ctx, args)
}
