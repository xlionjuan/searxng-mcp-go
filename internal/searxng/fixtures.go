package searxng

import (
	"fmt"
	"strings"
)

// BenchmarkFixtures provides reusable benchmark data structures.
// These are package-level so both root bench_test.go and internal/searxng/bench_test.go
// can import them without creating import cycles.
type BenchmarkFixtures struct {
	Answers     []Answer
	Infoboxes   []Infobox
	Suggestions []string
	Contents    []string
}

const (
	// infoboxContentRepeat is the number of times to repeat the base infobox content.
	infoboxContentRepeat = 20
	// longContentRepeat is the number of times to repeat the base long content string.
	longContentRepeat = 200
	// entityOffset1 is the first entity offset used in MakeEntityResults.
	entityOffset1 = 1
	// entityOffset2 is the second entity offset used in MakeEntityResults.
	entityOffset2 = 2
	// entityOffset3 is the third entity offset used in MakeEntityResults.
	entityOffset3 = 3
	// entityOffset4 is the fourth entity offset used in MakeEntityResults.
	entityOffset4 = 4
)

// DefaultBenchmarkFixtures returns the standard benchmark fixtures.
func DefaultBenchmarkFixtures() BenchmarkFixtures {
	return BenchmarkFixtures{
		Answers: []Answer{
			{Answer: "42", Engine: "calculator"},
			{Answer: "192.168.1.1", Engine: "ip_plugin"},
		},
		Infoboxes: []Infobox{
			{
				Infobox: "Test Topic",
				Content: strings.Repeat("Go is a programming language. ", infoboxContentRepeat),
				Attributes: []InfoboxAttribute{
					{Label: "Type", Value: "Language"},
					{Label: "Year", Value: "2009"},
				},
				URLs: []InfoboxURL{
					{Title: "Official", URL: "https://go.dev"},
				},
			},
		},
		Suggestions: []string{"golang tutorial", "golang concurrency", "golang vs rust"},
		Contents: []string{
			"Posted 3 hours ago by community",
			"Published yesterday by maintainers",
			"2 days ago we added new features",
			"5 days ago this was released",
			"Random content without any date information",
			"Last week there was an announcement",
		},
	}
}

// MakeSearchResults generates n SearchResult entries with API dates for benchmarking.
func MakeSearchResults(n int, fixtures BenchmarkFixtures) []SearchResult {
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

// MakeLargeSearchResponse creates a SearchResponse with n results for benchmarking.
func MakeLargeSearchResponse(n int, fixtures BenchmarkFixtures) *SearchResponse {
	return &SearchResponse{
		Query:           "golang programming",
		NumberOfResults: n,
		Answers:         fixtures.Answers,
		Infoboxes:       fixtures.Infoboxes,
		Results:         MakeSearchResults(n, fixtures),
		Suggestions:     fixtures.Suggestions,
	}
}

// MakeLongContentResults generates n results with content well above MaxContentRunes
// including multi-byte UTF-8 characters.
func MakeLongContentResults(n int) *SearchResponse {
	results := make([]SearchResult, n)

	base := strings.Repeat("這是一段很長的中文測試內容，包含多位元組字元。Go is a programming language. 日本語テスト。🎉🚀📊 ", longContentRepeat)
	for i := range n {
		results[i] = SearchResult{
			Title:   fmt.Sprintf("長內容測試結果 #%d 🌐", i),
			URL:     fmt.Sprintf("https://example.com/long/%d", i),
			Content: base + fmt.Sprintf(" Result index %d.", i),
			Engine:  "google",
		}
	}

	return &SearchResponse{
		Query:           "long content test",
		NumberOfResults: n,
		Results:         results,
	}
}

// MakeEntityResults generates n results with heavy HTML entities in titles and content.
func MakeEntityResults(n int) *SearchResponse {
	results := make([]SearchResult, n)

	entities := []string{"&amp;", "&quot;", "&lt;", "&gt;", "&#39;"}
	for i := range n {
		e := entities[i%len(entities)]
		results[i] = SearchResult{
			Title: fmt.Sprintf(
				"A %s B %s C %s D",
				e,
				entities[(i+entityOffset1)%len(entities)],
				entities[(i+entityOffset2)%len(entities)],
			),
			URL: fmt.Sprintf("https://example.com/entity/%d", i),
			Content: fmt.Sprintf(
				"Content with %s symbols & %s more %s entities %s here %s end.",
				e,
				entities[(i+entityOffset1)%len(entities)],
				entities[(i+entityOffset2)%len(entities)],
				entities[(i+entityOffset3)%len(entities)],
				entities[(i+entityOffset4)%len(entities)],
			),
			Engine: "google",
		}
	}

	return &SearchResponse{
		Query:           "entity test",
		NumberOfResults: n,
		Results:         results,
	}
}

// MakeOversizedSearchResponse creates a response with many answers and infoboxes
// for testing normalization bounds.
func MakeOversizedSearchResponse(nAnswers, nInfoboxes int) *SearchResponse {
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
