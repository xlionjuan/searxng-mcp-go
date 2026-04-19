package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// --- formatResults tests ---

func TestFormatResults(t *testing.T) {
	tests := []struct {
		name           string
		resp           *SearchResponse
		wantContains   []string
		wantNotContain string
		wantResult     string
	}{
		{
			name: "single answer with engine",
			resp: &SearchResponse{
				Query: "sha512 hello",
				Answers: []Answer{
					{Answer: "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043", Engine: "plugin:hash_plugin"},
				},
				Results: []SearchResult{
					{Title: "Hash Result", URL: "https://example.com", Content: "Some content", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantContains: []string{"=== Answers ===", "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043", "Engine: plugin:hash_plugin", "=== Results ===", "Hash Result"},
		},
		{
			name: "multiple answers",
			resp: &SearchResponse{
				Query: "random",
				Answers: []Answer{
					{Answer: "42", Engine: "random_plugin"},
					{Answer: "3.14", Engine: "math_plugin"},
				},
				Results: []SearchResult{},
			},
			wantContains: []string{"=== Answers ===", "[1] 42", "Engine: random_plugin", "[2] 3.14", "Engine: math_plugin"},
		},
		{
			name: "answer without engine",
			resp: &SearchResponse{
				Query: "ip",
				Answers: []Answer{
					{Answer: "203.0.113.42"},
				},
				Results: []SearchResult{},
			},
			wantContains:   []string{"=== Answers ===", "203.0.113.42"},
			wantNotContain: "Engine:",
		},
		{
			name: "answers only no results",
			resp: &SearchResponse{
				Query: "avg 123 548 2.04 24.2",
				Answers: []Answer{
					{Answer: "174.31", Engine: "stats_plugin"},
				},
				Results:         []SearchResult{},
				NumberOfResults: 0,
			},
			wantContains:   []string{"=== Answers ===", "174.31"},
			wantNotContain: "No results found.",
		},
		{
			name: "answers before infoboxes before results",
			resp: &SearchResponse{
				Query: "apple",
				Answers: []Answer{
					{Answer: "192.168.1.1", Engine: "ip_plugin"},
				},
				Infoboxes: []Infobox{
					{Infobox: "Apple", Content: "A fruit."},
				},
				Results: []SearchResult{
					{Title: "Apple - Fruit", URL: "https://example.com/apple", Content: "An apple is a fruit.", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantContains: []string{"=== Answers ===", "192.168.1.1", "=== Infoboxes ===", "Apple", "=== Results ===", "Found 1 results", "Apple - Fruit"},
		},
		{
			name: "no answers when empty",
			resp: &SearchResponse{
				Query: "test query",
				Results: []SearchResult{
					{Title: "Test", URL: "https://example.com", Content: "Test content", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantNotContain: "=== Answers ===",
		},
		{
			name: "normal results with content",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Test Title 1",
						URL:     "https://example.com/1",
						Content: "Test content 1",
						Engine:  "google",
					},
					{
						Title:   "Test Title 2",
						URL:     "https://example.com/2",
						Content: "",
						Engine:  "bing",
					},
				},
				NumberOfResults: 2,
				Query:           "test query",
			},
			wantContains: []string{"=== Results ===", "Found 2 results", "test query", "Test Title 1", "https://example.com/1", "1. Test Title 1\n   URL: https://example.com/1\n   Summary: Test content 1", "Test Title 2"},
		},
		{
			name:       "empty results",
			resp:       &SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "empty query"},
			wantResult: "No results found.",
		},
		{
			name: "content exceeding 4000 runes is truncated",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Long Content Test",
						URL:     "https://example.com/long",
						Content: strings.Repeat("x", 4500),
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "long content",
			},
			wantContains:   []string{"=== Results ===", strings.Repeat("x", MaxContentRunes), "Long Content Test"},
			wantNotContain: strings.Repeat("x", MaxContentRunes+1),
		},
		{
			name: "HTML entities are unescaped in content",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "HTML Test &amp; More &lt;stuff&gt;",
						URL:     "https://example.com/html",
						Content: "Test &amp; &lt; &gt; entities",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "html entities",
			},
			wantContains: []string{"=== Results ===", "HTML Test & More <stuff>", "Test & < > entities"},
		},
		{
			name: "empty content is handled correctly",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "No Content",
						URL:     "https://example.com/empty",
						Content: "",
						Engine:  "bing",
					},
				},
				NumberOfResults: 1,
				Query:           "empty",
			},
			wantNotContain: "Summary:",
		},
		{
			name: "NumberOfResults greater than len(Results) - paginated response",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
					{
						Title:   "Result 2",
						URL:     "https://example.com/2",
						Content: "Content 2",
						Engine:  "bing",
					},
				},
				NumberOfResults: 100,
				Query:           "test",
			},
			wantContains: []string{"=== Results ===", "Found 100 results", "Result 1", "Result 2"},
		},
		{
			name: "suggestions are displayed after results",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "test",
				Suggestions:     []string{"suggested query 1", "suggested query 2"},
			},
			wantContains: []string{"=== Results ===", "=== Search Suggestions ===", "  - suggested query 1", "  - suggested query 2"},
		},
		{
			name: "no suggestions block when empty",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "test",
				Suggestions:     nil,
			},
			wantNotContain: "=== Search Suggestions ===",
		},
		{
			name: "infoboxes displayed before results",
			resp: &SearchResponse{
				Query:           "apple inc",
				NumberOfResults: 2,
				Infoboxes: []Infobox{
					{
						Infobox: "Apple Inc.",
						Content: "Apple Inc. is an American technology company.",
						Attributes: []InfoboxAttribute{
							{Label: "Type", Value: "Public"},
							{Label: "Industry", Value: "Technology"},
						},
						URLs: []InfoboxURL{
							{Title: "Official site", URL: "https://www.apple.com"},
							{Title: "Wikipedia", URL: "https://en.wikipedia.org/wiki/Apple_Inc."},
						},
					},
				},
				Results: []SearchResult{
					{
						Title:   "Apple - Official Site",
						URL:     "https://www.apple.com",
						Content: "Think Different.",
						Engine:  "google",
					},
				},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Apple Inc.",
				"Apple Inc. is an American technology company.",
				"Attributes:",
				"- Type: Public",
				"- Industry: Technology",
				"URLs:",
				"- Official site: https://www.apple.com",
				"- Wikipedia: https://en.wikipedia.org/wiki/Apple_Inc.",
				"=== Results ===",
				"Found 2 results for 'apple inc'",
				"Apple - Official Site",
			},
		},
		{
			name: "infoboxes without results still show infoboxes",
			resp: &SearchResponse{
				Query:           "test",
				NumberOfResults: 0,
				Infoboxes: []Infobox{
					{
						Infobox: "Test Infobox",
						Content: "Some info content",
					},
				},
				Results: []SearchResult{},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Test Infobox",
				"Some info content",
			},
			wantNotContain: "No results found.",
		},
		{
			name: "infobox with empty content",
			resp: &SearchResponse{
				Query:           "test",
				NumberOfResults: 1,
				Infoboxes: []Infobox{
					{
						Infobox: "Minimal Infobox",
						Content: "",
						Attributes: []InfoboxAttribute{
							{Label: "Key", Value: "Value"},
						},
					},
				},
				Results: []SearchResult{
					{
						Title:  "Result 1",
						URL:    "https://example.com/1",
						Engine: "google",
					},
				},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Minimal Infobox",
				"Attributes:",
				"- Key: Value",
				"=== Results ===",
			},
		},
		{
			name: "infoboxes appear before suggestions",
			resp: &SearchResponse{
				Query:           "test",
				NumberOfResults: 1,
				Infoboxes: []Infobox{
					{
						Infobox: "Test IB",
						Content: "Info content",
					},
				},
				Results: []SearchResult{
					{
						Title:  "Result 1",
						URL:    "https://example.com/1",
						Engine: "google",
					},
				},
				Suggestions: []string{"suggestion 1"},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"=== Results ===",
				"Found 1 results",
				"=== Search Suggestions ===",
			},
		},
		{
			name: "suggestions only should not return no results found",
			resp: &SearchResponse{
				Query:       "typoed qurey",
				Suggestions: []string{"typoed query", "typed query", "torpedoed"},
			},
			wantContains:   []string{"=== Search Suggestions ===", "  - typoed query", "  - typed query", "  - torpedoed"},
			wantNotContain: "No results found.",
		},
		{
			name: "section order: answers then infoboxes then results then suggestions",
			resp: &SearchResponse{
				Query: "full",
				Answers: []Answer{
					{Answer: "42", Engine: "calc"},
				},
				Infoboxes: []Infobox{
					{Infobox: "Info", Content: "Some info."},
				},
				Results: []SearchResult{
					{Title: "Result", URL: "https://example.com", Content: "Content", Engine: "google"},
				},
				NumberOfResults: 1,
				Suggestions:     []string{"related query"},
			},
			wantResult: "ORDERED", // sentinel: handled by index-based check below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResults(tt.resp)

			if tt.wantResult != "" {
				if tt.wantResult == "ORDERED" {
					// Index-based assertion: verify section headers appear in order
					headers := []string{"=== Answers ===", "=== Infoboxes ===", "=== Results ===", "=== Search Suggestions ==="}
					for i := 1; i < len(headers); i++ {
						prev := strings.Index(result, headers[i-1])
						curr := strings.Index(result, headers[i])
						if prev == -1 {
							t.Errorf("expected %q in output", headers[i-1])
						} else if curr == -1 {
							t.Errorf("expected %q in output", headers[i])
						} else if curr < prev {
							t.Errorf("section order wrong: %q (pos %d) should come after %q (pos %d)", headers[i], curr, headers[i-1], prev)
						}
					}
					return
				}
				if result != tt.wantResult {
					t.Errorf("expected %q, got: %s", tt.wantResult, result)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("expected %q in output, got: %s", want, result)
				}
			}
			if tt.name == "normal results with content" {
				firstTitle := strings.Index(result, "1. Test Title 1")
				firstSummary := strings.Index(result, "   Summary: Test content 1")
				secondTitle := strings.Index(result, "2. Test Title 2")
				if firstTitle == -1 || firstSummary == -1 || secondTitle == -1 {
					t.Fatalf("expected both result blocks and the first summary in output, got: %s", result)
				}
				if !(firstTitle < firstSummary && firstSummary < secondTitle) {
					t.Fatalf("expected first result summary to belong to the first result block, got: %s", result)
				}
			}
			if tt.wantNotContain != "" && strings.Contains(result, tt.wantNotContain) {
				t.Errorf("did not expect %q in output, got: %s", tt.wantNotContain, result)
			}
		})
	}
}

func TestFormatResults_NilInput(t *testing.T) {
	if got := formatResults(nil); got != "No results found." {
		t.Fatalf("formatResults(nil) = %q, want %q", got, "No results found.")
	}
}

func TestFormatResults_DebugLogsUnresponsiveEngines(t *testing.T) {
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(old)

	resp := &SearchResponse{
		Query:               "test",
		Results:             []SearchResult{},
		NumberOfResults:     0,
		UnresponsiveEngines: [][]string{{"brave", `Suspended:" too many "requests`}},
		Debug:               true,
	}

	_ = formatResults(resp)

	if !strings.Contains(buf.String(), "unresponsive engine") {
		t.Fatalf("expected debug log for unresponsive engines, got: %s", buf.String())
	}
}
