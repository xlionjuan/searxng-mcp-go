package main

import (
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// TestFormatResultsGoldenOutput is a byte-for-byte regression test that locks
// the full output format of formatResults(). If any formatting change is made
// (intentional or accidental), this test will fail until the golden string is
// updated.
//
// The mock SearchResponse exercises all formatting paths:
//   - answers (with engine)
//   - infoboxes (with content, attributes, URLs; HTML entities in title)
//   - results (3 items: HTML entities in title/content, long content truncation, empty content)
//   - suggestions
//   - HTML entity unescaping (&amp; &lt; &gt; &quot;) in infobox titles and queries
//   - content exceeding searxng.MaxContentRunes (4000) is truncated
func TestFormatResultsGoldenOutput(t *testing.T) {
	t.Parallel()

	longContent := strings.Repeat("x", 4500)

	resp := &searxng.SearchResponse{
		Query: "golang &amp; html &quot;entities&quot;",
		Answers: []searxng.Answer{
			{Answer: "42.0", Engine: "calculator"},
		},
		Infoboxes: []searxng.Infobox{
			{
				Infobox: "Go &amp; Golang",
				Content: "Go is a statically typed, compiled programming language.\n" +
					"It was designed at Google.\n" +
					"It is syntactically similar to C.",
				Attributes: []searxng.InfoboxAttribute{
					{Label: "Type", Value: "Statically typed"},
					{Label: "Paradigm", Value: "Concurrent"},
				},
				URLs: []searxng.InfoboxURL{
					{Title: "Official site", URL: "https://go.dev"},
				},
			},
		},
		Results: []searxng.SearchResult{
			{
				Title:   "First &amp; Result &lt;test&gt;",
				URL:     "https://example.com/1",
				Content: "Some content &amp; more.",
				Engine:  "google",
			},
			{
				Title:   "Second Result",
				URL:     "https://example.com/2",
				Content: longContent,
				Engine:  "bing",
			},
			{
				Title:   "Third &quot;Result&quot;",
				URL:     "https://example.com/3",
				Content: "",
				Engine:  "duckduckgo",
			},
		},
		NumberOfResults: 42,
		Suggestions: []string{
			"golang tutorial",
			"html parsing",
		},
	}

	got := formatResults(resp)

	// Build expected output programmatically.
	// The long run of 'x' is generated via strings.Repeat to keep the source
	// readable; everything else uses direct string-literal fragments that
	// match the captured golden output byte-for-byte.
	x4000 := strings.Repeat("x", searxng.MaxContentRunes)
	golden := "" +
		"=== Web Search Results ===\n" +
		"Warning: " + searxng.ExternalContentWarning + "\n" +
		"\n" +
		"=== Answers ===\n" +
		"\n" +
		"[1] 42.0\n" +
		"    Engine: calculator\n" +
		"\n" +
		"=== Infoboxes ===\n" +
		"\n" +
		"[1] Go & Golang\n" +
		"    Go is a statically typed, compiled programming language.\n" +
		"It was designed at Google.\n" +
		"It is syntactically similar to C.\n" +
		"    Attributes:\n" +
		"      - Type: Statically typed\n" +
		"      - Paradigm: Concurrent\n" +
		"    URLs:\n" +
		"      - Official site: https://go.dev\n" +
		"\n" +
		"=== Results ===\n" +
		"\n" +
		"Found 42 total (showing 3) results for 'golang & html \"entities\"':\n" +
		"\n" +
		"1. First & Result <test>\n" +
		"   URL: https://example.com/1\n" +
		"   Summary: Some content & more.\n" +
		"   Engine: google\n" +
		"\n" +
		"2. Second Result\n" +
		"   URL: https://example.com/2\n" +
		"   Summary: " + x4000 + "\n" +
		"   Engine: bing\n" +
		"\n" +
		"3. Third \"Result\"\n" +
		"   URL: https://example.com/3\n" +
		"   Engine: duckduckgo\n" +
		"\n" +
		"=== Search Suggestions ===\n" +
		"\n" +
		"  - golang tutorial\n" +
		"  - html parsing\n"

	if got != golden {
		// Find first differing byte for diagnostics.
		minLen := min(len(golden), len(got))

		diffAt := -1

		for i := range minLen {
			if got[i] != golden[i] {
				diffAt = i

				break
			}
		}

		if diffAt == -1 {
			diffAt = minLen
		}

		t.Fatalf("golden output mismatch at byte %d (got len=%d, want len=%d):\n  got [%d..%d]: %q\n  want[%d..%d]: %q",
			diffAt, len(got), len(golden),
			max(0, diffAt-20), min(len(got), diffAt+20),
			got[max(0, diffAt-20):min(len(got), diffAt+20)],
			max(0, diffAt-20), min(len(golden), diffAt+20),
			golden[max(0, diffAt-20):min(len(golden), diffAt+20)])
	}
}
