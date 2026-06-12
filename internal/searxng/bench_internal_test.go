package searxng

import (
	"encoding/json"
	"fmt"
	"testing"

	"searxng-mcp-go/internal/testhelper"
)

// loadSearchResponse loads a SearchResponse from a JSON fixture in testdata/.
func loadSearchResponse(tb testing.TB, fixture string) *SearchResponse {
	tb.Helper()

	var resp SearchResponse

	testhelper.LoadJSONFixture(tb, "../../testdata/"+fixture, &resp)

	return &resp
}

// ============================================================================
// JSON Unmarshal Benchmarks
// ============================================================================

func BenchmarkJSONUnmarshal(b *testing.B) {
	data := testhelper.ReadFixture(b, "../../testdata/sample_response.json")

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
	data := testhelper.ReadFixture(b, "../../testdata/large_response_100.json")

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
	resp := loadSearchResponse(b, "large_response_10.json")

	b.ReportAllocs()

	for b.Loop() {
		_, err := resp.MarshalJSON()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalJSONLarge(b *testing.B) {
	resp := loadSearchResponse(b, "large_response_100.json")

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
	resp := loadSearchResponse(b, "large_response_10.json")

	type stdSearchResponse SearchResponse

	stdResp := stdSearchResponse(*resp)

	b.ReportAllocs()

	for b.Loop() {
		_, err := json.Marshal(stdResp)
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
		_, err := ValidateSearchArgs(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateSearchArgsMinimal(b *testing.B) {
	args := &SearchArgs{Query: "test"}

	b.ReportAllocs()

	for b.Loop() {
		_, err := ValidateSearchArgs(args)
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
