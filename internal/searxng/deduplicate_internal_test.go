package searxng

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- DeduplicateAnswers tests migrated from root package ---

func TestDeduplicateAnswers_EmptyInputs(t *testing.T) {
	t.Parallel()

	// Both empty
	result := deduplicateAnswers(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Empty answers
	result = deduplicateAnswers(nil, []Infobox{{Infobox: "test", Content: "content"}})
	if len(result) != 0 {
		t.Errorf("expected 0, got %d", len(result))
	}

	// Empty infoboxes
	answers := []Answer{{Answer: "test", Engine: "duckduckgo"}}

	result = deduplicateAnswers(answers, nil)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}
}

func TestDeduplicateAnswers_RemovesDuplicateWikipedia(t *testing.T) {
	t.Parallel()

	// Simulate DuckDuckGo putting Wikipedia summary in both answers and infoboxes
	wikiSummary := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."
	answers := []Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: wikiSummary},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (duplicate removed), got %d: %+v", len(result), result)
	}
}

func TestDeduplicateAnswers_RemovesPrefixMatch(t *testing.T) {
	t.Parallel()

	// Answer is a prefix of infobox content (truncated answer)
	answers := []Answer{
		{Answer: "Apple Inc. is an American multinational technology company", Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{
			Infobox: "Apple Inc.",
			Content: "Apple Inc. is an American multinational" +
				" technology company headquartered in Cupertino, California.",
		},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (prefix match removed), got %d", len(result))
	}
}

func TestDeduplicateAnswers_KeepsDistinctAnswer(t *testing.T) {
	t.Parallel()

	// "ip" query: answer is an IP address, infobox has unrelated content
	answers := []Answer{
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	infoboxes := []Infobox{
		{Infobox: "IP Address", Content: "An Internet Protocol address is a numerical label."},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (distinct answer kept), got %d", len(result))
	}

	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_CaseInsensitive(t *testing.T) {
	t.Parallel()

	answers := []Answer{
		{Answer: "apple inc. is an american company", Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: "Apple Inc. is an American company headquartered in California."},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf("expected 0 (case-insensitive match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_InfoboxContentOnly(t *testing.T) {
	t.Parallel()

	// Infobox with empty content should not cause filtering
	answers := []Answer{
		{Answer: "test answer", Engine: "test"},
	}
	infoboxes := []Infobox{
		{Infobox: "Test", Content: ""},
		{Infobox: "Test2", Content: ""},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (no content to match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_MultipleAnswersMixed(t *testing.T) {
	t.Parallel()

	wikiSummary := "Apple Inc. is an American multinational technology company."
	answers := []Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: wikiSummary},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1 (only IP answer kept), got %d", len(result))
	}

	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_DDGSuffixMoreAtWikipedia(t *testing.T) {
	t.Parallel()

	// DuckDuckGo appends "More at Wikipedia" to the answer, which breaks
	// the old Contains(answer, infobox) check. Prefix matching fixes this.
	infoboxContent := "Apple Inc. is an American multinational technology company headquartered in Cupertino, " +
		"California. Apple is one of the Big Tech companies, alongside Amazon, Google, Meta, and Microsoft."
	answer := infoboxContent + " More at Wikipedia"
	answers := []Answer{
		{Answer: answer, Engine: "duckduckgo"},
	}
	infoboxes := []Infobox{
		{Infobox: "Apple Inc.", Content: infoboxContent},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 0 {
		t.Errorf(
			"DDG answer with 'More at Wikipedia' suffix "+
				"should be deduplicated, got %d: %+v", len(result), result)
	}
}

func TestDeduplicateAnswers_EmptyAnswerSkipped(t *testing.T) {
	t.Parallel()

	answers := []Answer{
		{Answer: "", Engine: "duckduckgo"},
		{Answer: "valid answer", Engine: "test"},
	}
	infoboxes := []Infobox{
		{Infobox: "Test", Content: "some content"},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}

	if result[0].Answer != "valid answer" {
		t.Errorf("expected 'valid answer', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_TypedAnswersGetFallbackText(t *testing.T) {
	t.Parallel()

	answers := []Answer{
		{
			Answer:   "Translation: bonjour",
			Engine:   "libretranslate",
			Template: "answer/translations.html",
			Translations: []TranslationItem{
				{Text: "bonjour"},
			},
		},
		{
			Answer:   "Weather: Berlin, 11.2 °C, partly cloudy",
			Engine:   "open_meteo",
			Template: "answer/weather.html",
			Current: &WeatherItem{
				Location:    WeatherLocation{Name: "Berlin"},
				Temperature: WeatherMeasure{Val: 11.2, Unit: "°C"},
				Condition:   "partly cloudy",
			},
		},
	}
	infoboxes := []Infobox{
		{Infobox: "Other", Content: "unrelated infobox content"},
	}

	result := deduplicateAnswers(answers, infoboxes)
	if len(result) != 2 {
		t.Fatalf("expected 2 typed answers, got %d: %+v", len(result), result)
	}

	if result[0].Answer != "Translation: bonjour" {
		t.Fatalf("translation fallback = %q", result[0].Answer)
	}

	if result[1].Answer != "Weather: Berlin, 11.2 °C, partly cloudy" {
		t.Fatalf("weather fallback = %q", result[1].Answer)
	}
}

func TestTypedAnswerFixturesSurviveDeduplication(t *testing.T) {
	t.Parallel()

	for _, fixture := range []string{"typed_translation_answer.json", "typed_weather_answer.json"} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("..", "..", "testdata", fixture)) //nolint:gosec // test reads fixture files
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			var resp SearchResponse

			err = json.Unmarshal(body, &resp)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			// Apply typed answer fallback (normally done in normalizeResponse).
			for i := range resp.Answers {
				ensureAnswerFallback(&resp.Answers[i])
			}

			got := deduplicateAnswers(resp.Answers, []Infobox{{Infobox: "Other", Content: "unrelated"}})
			if len(got) != 1 {
				t.Fatalf("deduplicateAnswers() length = %d, want 1", len(got))
			}

			if got[0].Answer == "" {
				t.Fatal("deduplicateAnswers() kept an empty typed answer")
			}
		})
	}
}

// --- DeduplicateAnswers benchmarks migrated from bench_test.go (root) ---

func BenchmarkDeduplicateAnswers(b *testing.B) {
	answers := []Answer{
		{
			Answer: "Go is a statically typed, compiled programming language designed at Google. " +
				"It provides garbage collection, CSP-style concurrency, and structural typing. More at Wikipedia",
			Engine: "duckduckgo",
		},
		{Answer: "42", Engine: "calculator"},
		{Answer: "192.168.1.1", Engine: "ip_plugin"},
	}
	infoboxes := []Infobox{
		{
			Infobox: "Go",
			Content: strings.Repeat(
				"Go is a statically typed, compiled programming language designed at Google. "+
					"It provides garbage collection, CSP-style concurrency, and structural typing. ",
				10,
			),
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = deduplicateAnswers(answers, infoboxes)
	}
}

func BenchmarkDeduplicateAnswersManyInfoboxes(b *testing.B) {
	answers := []Answer{
		{Answer: "Some answer text that might match", Engine: "test"},
		{Answer: "Another unique answer", Engine: "test"},
		{Answer: "Third answer with content", Engine: "test"},
	}
	// 10 infoboxes with long content
	infoboxes := make([]Infobox, 10)
	for i := range infoboxes {
		infoboxes[i] = Infobox{
			Infobox: fmt.Sprintf("Infobox %d", i),
			Content: strings.Repeat(fmt.Sprintf("Content paragraph %d for testing. ", i), 30),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = deduplicateAnswers(answers, infoboxes)
	}
}

func BenchmarkDeduplicateAnswersNoInfoboxes(b *testing.B) {
	answers := []Answer{
		{Answer: "answer 1", Engine: "e1"},
		{Answer: "answer 2", Engine: "e2"},
	}
	// Fast path: empty infoboxes
	b.ReportAllocs()

	for b.Loop() {
		_ = deduplicateAnswers(answers, nil)
	}
}

func BenchmarkDeduplicateAnswersScale(b *testing.B) {
	cases := []struct {
		name       string
		nAnswers   int
		nInfoboxes int
	}{
		{"answers_3_infoboxes_10", 3, 10},
		{"answers_10_infoboxes_50", 10, 50},
		{"answers_25_infoboxes_100", 25, 100},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			answers := make([]Answer, tc.nAnswers)
			for i := range answers {
				answers[i] = Answer{
					Answer: fmt.Sprintf("Answer text number %d with some content about topic %d", i, i),
					Engine: fmt.Sprintf("engine_%d", i%3),
				}
			}

			infoboxes := make([]Infobox, tc.nInfoboxes)
			for i := range infoboxes {
				infoboxes[i] = Infobox{
					Infobox: fmt.Sprintf("Topic %d", i),
					Content: strings.Repeat(fmt.Sprintf("Content paragraph %d for testing deduplication. ", i), 20),
				}
			}

			b.ReportAllocs()

			for b.Loop() {
				_ = deduplicateAnswers(answers, infoboxes)
			}
		})
	}
}
