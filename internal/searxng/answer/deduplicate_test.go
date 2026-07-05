package answer_test

import (
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng/answer"
)

func TestDeduplicateAnswers_EmptyInputs(t *testing.T) {
	t.Run("both empty", func(t *testing.T) {
		result := answer.DeduplicateAnswers(nil, nil)

		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("empty answers", func(t *testing.T) {
		result := answer.DeduplicateAnswers(nil, []string{"content"})

		if len(result) != 0 {
			t.Errorf("expected 0, got %d", len(result))
		}
	})

	t.Run("empty infoboxes", func(t *testing.T) {
		answers := []answer.Answer{{Answer: "test", Engine: "duckduckgo"}}
		result := answer.DeduplicateAnswers(answers, nil)

		if len(result) != 1 {
			t.Errorf("expected 1, got %d", len(result))
		}
	})
}

func TestDeduplicateAnswers_RemovesDuplicateWikipedia(t *testing.T) {
	wikiSummary := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."
	answers := []answer.Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
	}
	result := answer.DeduplicateAnswers(answers, []string{wikiSummary})

	if len(result) != 0 {
		t.Errorf("expected 0 (duplicate removed), got %d", len(result))
	}
}

func TestDeduplicateAnswers_RemovesPrefixMatch(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "Apple Inc. is an American multinational technology company", Engine: "duckduckgo"},
	}
	infoboxContent := "Apple Inc. is an American multinational" +
		" technology company headquartered in Cupertino, California."
	result := answer.DeduplicateAnswers(answers, []string{infoboxContent})

	if len(result) != 0 {
		t.Errorf("expected 0 (prefix match removed), got %d", len(result))
	}
}

func TestDeduplicateAnswers_KeepsDistinctAnswer(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	result := answer.DeduplicateAnswers(answers, []string{"An Internet Protocol address is a numerical label."})

	if len(result) != 1 {
		t.Errorf("expected 1 (distinct answer kept), got %d", len(result))
	}

	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_CaseInsensitive(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "apple inc. is an american company", Engine: "duckduckgo"},
	}
	content := "Apple Inc. is an American company headquartered in California."
	result := answer.DeduplicateAnswers(answers, []string{content})

	if len(result) != 0 {
		t.Errorf("expected 0 (case-insensitive match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_FiltersEmptyInfoboxContent(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "test answer", Engine: "test"},
	}
	result := answer.DeduplicateAnswers(answers, []string{"", ""})

	if len(result) != 1 {
		t.Errorf("expected 1 (no content to match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_MultipleAnswersMixed(t *testing.T) {
	wikiSummary := "Apple Inc. is an American multinational technology company."
	answers := []answer.Answer{
		{Answer: wikiSummary, Engine: "duckduckgo"},
		{Answer: "203.0.113.42", Engine: "ip"},
	}
	result := answer.DeduplicateAnswers(answers, []string{wikiSummary})

	if len(result) != 1 {
		t.Errorf("expected 1 (only IP answer kept), got %d", len(result))
	}

	if result[0].Answer != "203.0.113.42" {
		t.Errorf("expected '203.0.113.42', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_DDGSuffixMoreAtWikipedia(t *testing.T) {
	infoboxContent := "Apple Inc. is an American multinational technology company headquartered in Cupertino, " +
		"California. Apple is one of the Big Tech companies, alongside Amazon, Google, Meta, and Microsoft."
	a := infoboxContent + " More at Wikipedia"
	answers := []answer.Answer{
		{Answer: a, Engine: "duckduckgo"},
	}
	result := answer.DeduplicateAnswers(answers, []string{infoboxContent})

	if len(result) != 0 {
		t.Errorf("DDG answer with 'More at Wikipedia' suffix should be deduplicated, got %d", len(result))
	}
}

func TestDeduplicateAnswers_EmptyAnswerSkipped(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "", Engine: "duckduckgo"},
		{Answer: "valid answer", Engine: "test"},
	}
	result := answer.DeduplicateAnswers(answers, []string{"some content"})

	if len(result) != 1 {
		t.Errorf("expected 1, got %d", len(result))
	}

	if result[0].Answer != "valid answer" {
		t.Errorf("expected 'valid answer', got %q", result[0].Answer)
	}
}

func TestDeduplicateAnswers_LowercaseLazyBuild(t *testing.T) {
	answers := []answer.Answer{
		{Answer: "Apple Inc. Uses Swift", Engine: "duckduckgo"},
		{Answer: "Apple Inc. Is Great", Engine: "duckduckgo"},
	}
	result := answer.DeduplicateAnswers(answers, []string{
		"Apple Inc. uses swift for its app development. Apple Inc. is great for many reasons.",
	})

	if len(result) != 0 {
		t.Errorf("expected 0 (both matched case-insensitively), got %d", len(result))
	}
}

func TestDeduplicateAnswers_TruncationBoundary(t *testing.T) {
	var sb strings.Builder
	for range 300 {
		sb.WriteByte('x')
	}

	longNeedle := sb.String()

	answers := []answer.Answer{
		{Answer: longNeedle + " More at Wikipedia", Engine: "duckduckgo"},
	}

	haystack := longNeedle[:200] + "some more text"
	result := answer.DeduplicateAnswers(answers, []string{haystack})

	if len(result) != 0 {
		t.Errorf("expected 0 (truncated match), got %d", len(result))
	}
}

func TestDeduplicateAnswers_NonMatchingTruncation(t *testing.T) {
	var sb strings.Builder
	for range 300 {
		sb.WriteByte('x')
	}

	longNeedle := sb.String()

	answers := []answer.Answer{
		{Answer: longNeedle, Engine: "test"},
	}

	result := answer.DeduplicateAnswers(answers, []string{"something completely different"})

	if len(result) != 1 {
		t.Errorf("expected 1 (no match after truncation), got %d", len(result))
	}
}
