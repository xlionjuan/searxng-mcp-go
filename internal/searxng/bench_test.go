package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func readSampleResponse(tb testing.TB) []byte {
	tb.Helper()

	data, err := os.ReadFile("../../testdata/sample_response.json")
	if err != nil {
		tb.Fatal(err)
	}

	return data
}

// makeSearchResults generates n SearchResult entries with API dates for benchmarking.
func makeSearchResults(n int) []SearchResult {
	results := make([]SearchResult, n)
	contents := []string{
		"Posted 3 hours ago by community",
		"Published yesterday by maintainers",
		"2 days ago we added new features",
		"5 days ago this was released",
		"Random content without any date information",
		"Last week there was an announcement",
	}

	for i := range n {
		date := "2024-01-15"
		results[i] = SearchResult{
			Title:         fmt.Sprintf("Search Result Title %d", i),
			URL:           fmt.Sprintf("https://example.com/result/%d", i),
			Content:       fmt.Sprintf("This is the content for result number %d. %s", i, contents[i%len(contents)]),
			Engine:        []string{"google", "bing", "duckduckgo"}[i%3],
			PublishedDate: &date,
		}
	}

	return results
}

// makeLargeSearchResponse creates a SearchResponse with n results for benchmarking.
func makeLargeSearchResponse(n int) *SearchResponse {
	answers := []Answer{
		{Answer: "42", Engine: "calculator"},
		{Answer: "192.168.1.1", Engine: "ip_plugin"},
	}
	infoboxes := []Infobox{
		{
			Infobox: "Test Topic",
			Content: strings.Repeat("Go is a programming language. ", 20),
			Attributes: []InfoboxAttribute{
				{Label: "Type", Value: "Language"},
				{Label: "Year", Value: "2009"},
			},
			URLs: []InfoboxURL{
				{Title: "Official", URL: "https://go.dev"},
			},
		},
	}
	suggestions := []string{"golang tutorial", "golang concurrency", "golang vs rust"}

	return &SearchResponse{
		Query:           "golang programming",
		NumberOfResults: n,
		Answers:         answers,
		Infoboxes:       infoboxes,
		Results:         makeSearchResults(n),
		Suggestions:     suggestions,
	}
}

// ============================================================================
// JSON Unmarshal Benchmarks
// ============================================================================

func BenchmarkJSONUnmarshal(b *testing.B) {
	data := readSampleResponse(b)

	b.ReportAllocs()

	for b.Loop() {
		var resp SearchResponse

		err := json.Unmarshal(data, &resp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONUnmarshalLarge(b *testing.B) {
	// Create a large JSON payload (100 results)
	largeResp := makeLargeSearchResponse(100)

	data, err := json.Marshal(largeResp)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		var resp SearchResponse

		err := json.Unmarshal(data, &resp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// MarshalJSON Benchmarks
// ============================================================================

func BenchmarkMarshalJSON(b *testing.B) {
	resp := makeLargeSearchResponse(10)

	b.ReportAllocs()

	for b.Loop() {
		_, err := resp.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalJSONLarge(b *testing.B) {
	resp := makeLargeSearchResponse(100)

	b.ReportAllocs()

	for b.Loop() {
		_, err := resp.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Standard json.Marshal for comparison.
func BenchmarkStdMarshalJSON(b *testing.B) {
	type stdSearchResponse SearchResponse

	resp := stdSearchResponse(*makeLargeSearchResponse(10))

	b.ReportAllocs()

	for b.Loop() {
		_, err := json.Marshal(resp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnescapeIfNeeded(b *testing.B) {
	inputs := []string{
		"Simple string without entities",
		"String with &amp; &lt; &gt; entities",
		"&quot;Quoted&quot; &apos;text&apos; here",
		"a]normal string",
	}

	b.ReportAllocs()

	for _, input := range inputs {
		b.Run(fmt.Sprintf("len_%d", len(input)), func(b *testing.B) {
			for b.Loop() {
				_ = UnescapeIfNeeded(input)
			}
		})
	}
}

// ============================================================================
// Validation Benchmarks
// ============================================================================

func BenchmarkValidateSearchArgs(b *testing.B) {
	pageno := 1
	args := &SearchArgs{
		Query:      "golang programming",
		Language:   "en",
		SafeSearch: 0,
		TimeRange:  "month",
		Categories: "general,it",
		Engines:    "google,bing",
		Pageno:     &pageno,
	}

	b.ReportAllocs()

	for b.Loop() {
		err := ValidateSearchArgs(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateSearchArgsMinimal(b *testing.B) {
	args := &SearchArgs{Query: "test"}

	b.ReportAllocs()

	for b.Loop() {
		err := ValidateSearchArgs(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}
