package main

import (
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
			wantContains: []string{"Found 2 results", "test query", "Test Title 1", "https://example.com/1", "Test content 1", "Test Title 2", "Summary:"},
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
						Content: strings.Repeat("x", 4500), // 4500 chars > 4000 limit
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "long content",
			},
			wantContains:   []string{strings.Repeat("x", MaxContentRunes), "Long Content Test"},
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
			wantContains: []string{"HTML Test & More <stuff>", "Test & < > entities"},
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
			wantNotContain: "Summary:", // Empty content should not show Summary line
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
				NumberOfResults: 100, // Total matches is 100, but only 2 on this page
				Query:           "test",
			},
			wantContains: []string{"Found 100 results", "Result 1", "Result 2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResults(tt.resp)

			if tt.wantResult != "" {
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
			if tt.wantNotContain != "" && strings.Contains(result, tt.wantNotContain) {
				t.Errorf("did not expect %q in output, got: %s", tt.wantNotContain, result)
			}
		})
	}
}
