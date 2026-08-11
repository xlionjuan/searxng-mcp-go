package searxng

import (
	"fmt"
	"strings"
	"testing"
)

func TestDeduplicateAnswers_ProjectsInfoboxContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		infobox  Infobox
		wantKept bool
	}{
		{
			name: "matching title does not substitute for content",
			infobox: Infobox{
				Infobox: "duplicate answer",
				Content: "unrelated content",
			},
			wantKept: true,
		},
		{
			name: "matching content is projected",
			infobox: Infobox{
				Infobox: "Unrelated title",
				Content: "duplicate answer appears in content",
			},
			wantKept: false,
		},
		{
			name: "empty content is ignored",
			infobox: Infobox{
				Infobox: "duplicate answer",
			},
			wantKept: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			answers := []Answer{{Answer: "duplicate answer", Engine: "duckduckgo"}}
			got := deduplicateAnswers(answers, []Infobox{tt.infobox})

			if kept := len(got) == 1; kept != tt.wantKept {
				t.Fatalf("deduplicateAnswers() kept answer = %t, want %t; result = %+v", kept, tt.wantKept, got)
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
