package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"encoding/json"
	"fmt"
	"os"
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
	fixtures := DefaultBenchmarkFixtures()
	results := make([]SearchResult, n)
	date := "2024-01-15"

	for i := range n {
		results[i] = SearchResult{
			Title:         fmt.Sprintf("Search Result Title %d", i),
			URL:           fmt.Sprintf("https://example.com/result/%d", i),
			Content:       fmt.Sprintf("This is the content for result number %d. %s", i, fixtures.Contents[i%len(fixtures.Contents)]),
			Engine:        []string{"google", "bing", "duckduckgo"}[i%3],
			PublishedDate: &date,
		}
	}

	return results
}

// makeLargeSearchResponse creates a SearchResponse with n results for benchmarking.
func makeLargeSearchResponse(n int) *SearchResponse {
	fixtures := DefaultBenchmarkFixtures()

	return &SearchResponse{
		Query:           "golang programming",
		NumberOfResults: n,
		Answers:         fixtures.Answers,
		Infoboxes:       fixtures.Infoboxes,
		Results:         makeSearchResults(n),
		Suggestions:     fixtures.Suggestions,
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

// ============================================================================
// Normalize Response Benchmarks (CAND-33fe0b85-RUNTIME-003)
// ============================================================================
//
// The MaxAnswers / MaxInfoboxes caps in normalizeResponse are the
// single point of bounded-work protection for answer/infobox
// deduplication. These benchmarks exercise the worst-case path so a
// future regression in the cap or dedup loop is visible in the perf
// dashboard.

func makeOversizedSearchResponse(nAnswers, nInfoboxes int) *SearchResponse {
	answers := make([]Answer, nAnswers)
	for i := range answers {
		answers[i] = Answer{Answer: "answer text", Engine: "e"}
	}

	infoboxes := make([]Infobox, nInfoboxes)
	for i := range infoboxes {
		infoboxes[i] = Infobox{Infobox: "t", Content: "content"}
	}

	return &SearchResponse{Answers: answers, Infoboxes: infoboxes}
}

func BenchmarkNormalizeResponseAnswersInfoboxesBounded(b *testing.B) {
	cases := []struct {
		name       string
		nAnswers   int
		nInfoboxes int
	}{
		{"at_cap_100x100", MaxAnswers, MaxInfoboxes},
		{"oversized_10x_cap_10x_cap", 10 * MaxAnswers, 10 * MaxInfoboxes},
		{"oversized_50x_cap_50x_cap", 50 * MaxAnswers, 50 * MaxInfoboxes},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			resp := makeOversizedSearchResponse(tc.nAnswers, tc.nInfoboxes)

			s := &SearXNGSearcher{debug: false}
			args := &SearchArgs{}

			b.ReportAllocs()

			for b.Loop() {
				fresh := *resp
				s.normalizeResponse(&fresh, args)
			}
		})
	}
}
