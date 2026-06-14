//go:build e2e

package main

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMCPStdioE2E_BasicSearch(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query": "framework computer inc",
		"limit": 3,
	}, stderr, "basic search")

	if len(response.Results) == 0 {
		warning := "basic search results length = 0"
		warnings.Addf("%s", warning)
		t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
	}

	if strings.TrimSpace(response.Query) == "" {
		t.Fatalf("basic search query is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	warnings.Report(t)
	t.Log("MCP stdio basic search verified")
}

func TestMCPStdioE2E_AnswerResponse(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query": "sha512 hello",
		"limit": 1,
	}, stderr, "answer response")

	if len(response.Answers) == 0 {
		t.Fatalf("answer response answers length = 0\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	t.Log("MCP stdio answer response verified")
}

func TestMCPStdioE2E_InfoboxResponse(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	var warnings e2eWarnings

	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query": "apple inc",
		"limit": 3,
	}, stderr, "infobox response")

	// Empty infoboxes are the typical flakiness case (engine may
	// rotate and drop the Wikipedia infobox), so route through
	// WARNING SUMMARY instead of failing.
	if len(response.Infoboxes) == 0 {
		warnings.Addf("infobox response infoboxes length = 0")
		t.Logf("infobox response infoboxes length = 0\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	warnings.Report(t)
	t.Log("MCP stdio infobox response verified")
}

func TestMCPStdioE2E_ParameterForwarding(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	wantEngines := []string{"google", "bing", "yahoo", "ddg definitions"}
	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query":      "framework computer inc",
		"language":   "en",
		"safesearch": 1,
		"categories": "general",
		"engines":    strings.Join(wantEngines, ","),
		"pageno":     1,
		"limit":      5,
	}, stderr, "optional parameter forwarding")

	if response.Query != "framework computer inc" {
		t.Fatalf("query = %q, want %q\nresponse: %#v\nstderr:\n%s", response.Query, "framework computer inc", response, stderr.String())
	}

	if len(response.Results) == 0 || len(response.Results) > 5 {
		t.Fatalf("results length = %d, want 1..5"+
			"\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
	}

	sawEngine := false

	for i, result := range response.Results {
		if strings.TrimSpace(result.Title) == "" {
			t.Fatalf("result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
		}

		if strings.TrimSpace(result.URL) == "" {
			t.Fatalf("result[%d] url is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
		}

		if strings.TrimSpace(result.Engine) == "" {
			t.Fatalf("result[%d] engine is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
		}

		for _, we := range wantEngines {
			if strings.EqualFold(result.Engine, we) {
				sawEngine = true
			}
		}
	}

	if !sawEngine {
		t.Fatalf("no result from any of %v\nresponse: %#v\nstderr:\n%s", wantEngines, response, stderr.String())
	}

	t.Log("MCP stdio parameter forwarding verified")
}

// TestMCPStdioE2E_ValidationSampling is a small integration check that
// validation is enforced through the live MCP stdio session. Exhaustive
// coverage lives in TestMCPErrors_InvalidInputs.
func TestMCPStdioE2E_ValidationSampling(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	sampleNames := map[string]struct{}{
		"whitespace query": {},
		"limit too high":   {},
		"invalid language": {},
	}

	for _, tt := range SharedInvalidInputCases {
		if _, ok := sampleNames[tt.Name]; !ok {
			continue
		}

		t.Run(tt.Name, func(t *testing.T) {
			result := callSearchTool(ctx, t, session, tt.Arguments, stderr)
			if !result.IsError {
				t.Fatalf("IsError = false, want true\nresult: %#v\nstderr:\n%s", result, stderr.String())
			}

			if len(result.Content) != 1 {
				t.Fatalf("content length = %d, want 1\nresult: %#v\nstderr:\n%s", len(result.Content), result, stderr.String())
			}

			text := toolText(t, result)
			assertMCPValidationText(t, text, tt.WantField, tt.WantSchemaErr, stderr.String())
		})
	}

	t.Log("MCP stdio validation sampling verified")
}

func TestMCPStdioE2E_NewsCategoryTimeRange(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query":      "technology",
		"language":   "en",
		"categories": "news",
		"time_range": "month",
		"limit":      5,
	}, stderr, "news category with time range")

	// The news + time_range=month combination is unstable on the CI
	// SearXNG instance: it can persistently return 0 results. Retry
	// cannot fix this (it is not transient), so route the zero-result
	// outcome through WARNING SUMMARY. Non-empty shape checks
	// (limit ceiling, publishedDate, http URL) remain strict when
	// results are present.
	if len(response.Results) == 0 {
		warning := "news category with time range results length = 0, want 1..5"
		warnings.Addf("%s", warning)
		t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
	} else {
		if len(response.Results) > 5 {
			t.Fatalf("results length = %d, want 1..5"+
				"\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
		}

		hasPublishedDate := false
		hasHTTPURL := false

		for _, result := range response.Results {
			if result.PublishedDate != nil && strings.TrimSpace(*result.PublishedDate) != "" {
				hasPublishedDate = true
			}

			if strings.HasPrefix(result.URL, "http://") || strings.HasPrefix(result.URL, "https://") {
				hasHTTPURL = true
			}
		}

		if !hasPublishedDate {
			t.Fatalf("no result had publishedDate\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}

		if !hasHTTPURL {
			t.Fatalf("no result had an http(s) URL\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
	}

	warnings.Report(t)
	t.Log("MCP stdio news category + time range verified")
}

func TestMCPStdioE2E_UnicodeQueryRoundTrip(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	query := "日本 golang \"type parameters\" site:go.dev"
	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query":      query,
		"language":   "ja",
		"engines":    "bing",
		"categories": "general",
		"limit":      5,
	}, stderr, "unicode query round trip")

	if response.Query != query {
		t.Fatalf("query = %q, want %q\nresponse: %#v\nstderr:\n%s", response.Query, query, response, stderr.String())
	}

	if len(response.Results) == 0 {
		warning := "unicode query round trip results length = 0, want > 0"
		warnings.Addf("%s", warning)
		t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
	}

	if len(response.Results) > 5 {
		t.Fatalf("results length = %d, want <= 5"+
			"\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
	}

	for i, result := range response.Results {
		if strings.TrimSpace(result.URL) == "" {
			t.Fatalf("result[%d] URL is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
		}
	}

	warnings.Report(t)
	t.Log("MCP stdio unicode query round trip verified")
}

func TestMCPStdioE2E_ResponseFormatInvariants(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	searchTool := findSearchTool(ctx, t, session, stderr)
	t.Logf("found tool: %s", searchTool.Name)

	result := callSearchTool(ctx, t, session, map[string]any{
		"query":      "site:example.invalid unlikely-no-real-result-codex-e2e",
		"engines":    "bing",
		"categories": "general",
		"limit":      3,
	}, stderr)
	text := toolText(t, result)

	if result.IsError {
		t.Fatalf("response format query returned tool error: %s\nstderr:\n%s", text, stderr.String())
	}

	if !strings.Contains(text, `"results":[]`) {
		t.Fatalf("raw JSON does not contain empty results array\ntext:\n%s\nstderr:\n%s", text, stderr.String())
	}

	if !strings.Contains(text, `"suggestions":[]`) {
		t.Fatalf("raw JSON does not contain empty suggestions array\ntext:\n%s\nstderr:\n%s", text, stderr.String())
	}

	var raw map[string]json.RawMessage

	err := json.Unmarshal([]byte(text), &raw)
	if err != nil {
		t.Fatalf("response is not JSON: %v\ntext:\n%s\nstderr:\n%s", err, text, stderr.String())
	}

	for _, field := range []string{"answers", "infoboxes", "unresponsive_engines"} {
		if _, ok := raw[field]; ok {
			t.Fatalf("field %q present, want omitted when empty/debug-off"+
				"\ntext:\n%s\nstderr:\n%s", field, text, stderr.String())
		}
	}

	response := parseSearchResponse(t, result, stderr)
	if len(response.Results) != 0 {
		t.Fatalf("results length = %d, want 0"+
			"\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
	}

	if len(response.Suggestions) != 0 {
		t.Fatalf("suggestions length = %d, want 0"+
			"\nresponse: %#v\nstderr:\n%s", len(response.Suggestions), response, stderr.String())
	}

	t.Log("MCP stdio response format invariants verified")
}
