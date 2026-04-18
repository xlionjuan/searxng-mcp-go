package main

import (
	"strings"
	"testing"
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
//   - HTML entity unescaping (&amp; &lt; &gt; &quot;)
//   - content exceeding MaxContentRunes (4000) is truncated
func TestFormatResultsGoldenOutput(t *testing.T) {
	longContent := strings.Repeat("x", 4500)

	resp := &SearchResponse{
		Query: "golang &amp; html &quot;entities&quot;",
		Answers: []Answer{
			{Answer: "42.0", Engine: "calculator"},
		},
		Infoboxes: []Infobox{
			{
				Infobox: "Go &amp; Golang",
				Content: "Go is a statically typed, compiled programming language.\nIt was designed at Google.\nIt is syntactically similar to C.",
				Attributes: []InfoboxAttribute{
					{Label: "Type", Value: "Statically typed"},
					{Label: "Paradigm", Value: "Concurrent"},
				},
				URLs: []InfoboxURL{
					{Title: "Official site", URL: "https://go.dev"},
				},
			},
		},
		Results: []SearchResult{
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
	x4000 := strings.Repeat("x", MaxContentRunes)
	golden := "" +
		"=== Answers ===\n" +
		"\n" +
		"[1] 42.0\n" +
		"    Engine: calculator\n" +
		"\n" +
		"=== Infoboxes ===\n" +
		"\n" +
		"[1] Go &amp; Golang\n" +
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
		"Found 42 results for 'golang &amp; html &quot;entities&quot;':\n" +
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
		minLen := len(got)
		if len(golden) < minLen {
			minLen = len(golden)
		}
		diffAt := -1
		for i := 0; i < minLen; i++ {
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
