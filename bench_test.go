// Package main implements a SearXNG MCP server providing web search via the Model Context Protocol.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
)

type staticRoundTripper struct {
	statusCode int
	header     http.Header
	body       string
	err        error
}

func (rt *staticRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.err != nil {
		return nil, rt.err
	}

	return &http.Response{
		StatusCode: rt.statusCode,
		Status:     fmt.Sprintf("%d %s", rt.statusCode, http.StatusText(rt.statusCode)),
		Header:     rt.header.Clone(),
		Body:       io.NopCloser(strings.NewReader(rt.body)),
		Request:    req,
	}, nil
}

// ============================================================================
// Test Fixtures
// ============================================================================

// sampleSearXNGJSON is a realistic SearXNG response payload (~2.5KB).
const sampleSearXNGJSON = `{
  "query": "golang programming language",
  "number_of_results": 12400000,
  "results": [
    {
      "title": "The Go Programming Language",
      "url": "https://go.dev/",
      "content": "` + `Go is an open source programming language supported by Google with built-in concurrency ` +
	`and a robust standard library.` + `",
      "engine": "google",
      "publishedDate": "2024-01-15"
    },
    {
      "title": "Go Tutorial - W3Schools",
      "url": "https://www.w3schools.com/go/",
      "content": "` + `Learn Go programming with examples. Go is a statically typed, compiled language designed ` +
	`at Google. Posted 3 hours ago by community.` + `",
      "engine": "google"
    },
    {
      "title": "Getting Started with Go - Go by Example",
      "url": "https://gobyexample.com/",
      "content": "` + `Go by Example is a hands-on introduction to Go using annotated example programs. ` +
	`Published yesterday by maintainers.` + `",
      "engine": "bing"
    },
    {
      "title": "Go Downloads - golang.org",
      "url": "https://go.dev/dl/",
      "content": "` + `Download Go binaries for your platform. The latest release includes security updates ` +
	`and performance improvements.` + `",
      "engine": "google",
      "publishedDate": "2024-03-01"
    },
    {
      "title": "Effective Go - golang.org",
      "url": "https://go.dev/doc/effective_go",
      "content": "` + `Effective Go gives tips for writing clear, idiomatic Go code. If you're new to Go, ` +
	`read the tutorial and then come back here.` + `",
      "engine": "google"
    },
    {
      "title": "A Tour of Go",
      "url": "https://go.dev/tour/",
      "content": "` + `An interactive introduction to Go. Learn the basics of Go through examples. ` +
	`2 days ago we added new concurrency examples.` + `",
      "engine": "duckduckgo"
    },
    {
      "title": "Go Modules Reference - golang.org",
      "url": "https://go.dev/ref/mod",
      "content": "` + `A module is a collection of related Go packages that are versioned together as a single ` +
	`unit. Modules record precise dependency requirements.` + `",
      "engine": "google"
    },
    {
      "title": "GitHub - golang/go",
      "url": "https://github.com/golang/go",
      "content": "` + `The Go programming language source repository. Contribute to golang/go development ` +
	`by creating an account on GitHub.` + `",
      "engine": "google",
      "publishedDate": "2024-02-20"
    },
    {
      "title": "Go (programming language) - Wikipedia",
      "url": "https://en.wikipedia.org/wiki/Go_(programming_language)",
      "content": "` + `Go is a statically typed, compiled high-level programming language designed at Google ` +
	`by Robert Griesemer, Rob Pike, and Ken Thompson.` + `",
      "engine": "wikipedia"
    },
    {
      "title": "Learn Go in Y Minutes",
      "url": "https://learnxinyminutes.com/docs/go/",
      "content": "` + `Go was created out of the need to get work done. It's a pragmatic language that lets ` +
	`you write code quickly and efficiently.` + `",
      "engine": "duckduckgo"
    }
  ],
  "answers": [
    {
      "answer": "` + `Go is a statically typed, compiled programming language designed at Google by Robert ` +
	`Griesemer, Rob Pike, and Ken Thompson. It provides garbage collection, CSP-style concurrency, ` +
	`and structural typing. More at Wikipedia` + `",
      "engine": "duckduckgo",
      "template": "wikipedia"
    }
  ],
  "infoboxes": [
    {
      "infobox": "Go (programming language)",
      "content": "` + `Go is a statically typed, compiled programming language designed at Google by Robert ` +
	`Griesemer, Rob Pike, and Ken Thompson. It provides garbage collection, CSP-style concurrency, ` +
	`and structural typing.` + `",
      "attributes": [
        {"label": "Paradigm", "value": "Multi-paradigm: concurrent, functional, imperative, object-oriented"},
        {"label": "Designed by", "value": "Robert Griesemer, Rob Pike, Ken Thompson"},
        {"label": "First appeared", "value": "2009"},
        {"label": "Typing discipline", "value": "Static, strong, structural, inferred"}
      ],
      "urls": [
        {"title": "Official website", "url": "https://go.dev/"},
        {"title": "GitHub", "url": "https://github.com/golang/go"}
      ]
    }
  ],
  "suggestions": ["golang tutorial", "golang concurrency", "golang vs rust", "golang generics"]
}`

// makeSearchResults generates n SearchResult entries with API dates for benchmarking.
func makeSearchResults(n int) []searxng.SearchResult {
	results := make([]searxng.SearchResult, n)
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
		results[i] = searxng.SearchResult{
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
func makeLargeSearchResponse(n int) *searxng.SearchResponse {
	answers := []searxng.Answer{
		{Answer: "42", Engine: "calculator"},
		{Answer: "192.168.1.1", Engine: "ip_plugin"},
	}
	infoboxes := []searxng.Infobox{
		{
			Infobox: "Test Topic",
			Content: strings.Repeat("Go is a programming language. ", 20),
			Attributes: []searxng.InfoboxAttribute{
				{Label: "Type", Value: "Language"},
				{Label: "Year", Value: "2009"},
			},
			URLs: []searxng.InfoboxURL{
				{Title: "Official", URL: "https://go.dev"},
			},
		},
	}
	suggestions := []string{"golang tutorial", "golang concurrency", "golang vs rust"}

	return &searxng.SearchResponse{
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
	data := []byte(sampleSearXNGJSON)

	b.ReportAllocs()

	for b.Loop() {
		var resp searxng.SearchResponse

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
		var resp searxng.SearchResponse

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
	type stdSearchResponse searxng.SearchResponse

	resp := stdSearchResponse(*makeLargeSearchResponse(10))

	b.ReportAllocs()

	for b.Loop() {
		_, err := json.Marshal(resp)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Format Benchmarks
// ============================================================================

func BenchmarkFormatResults(b *testing.B) {
	resp := makeLargeSearchResponse(10)

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
	}
}

func BenchmarkSearch(b *testing.B) {
	cfg := &searxng.Config{
		SearXNGURL: "http://127.0.0.1",
		Timeout:    30 * time.Second,
		HTTPClient: &http.Client{
			Transport: &staticRoundTripper{
				statusCode: http.StatusOK,
				header:     http.Header{"Content-Type": []string{"application/json"}},
				body:       sampleSearXNGJSON,
			},
		},
	}
	args := &searxng.SearchArgs{Query: "golang programming", Language: "en", SafeSearch: 1}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, err := testPerformSearch(b.Context(), b, cfg, args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFormatResultsLarge(b *testing.B) {
	resp := makeLargeSearchResponse(100)

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
	}
}

func BenchmarkFormatResultsInfoboxes(b *testing.B) {
	resp := &searxng.SearchResponse{
		Query:           "test",
		NumberOfResults: 1,
		Infoboxes: []searxng.Infobox{
			{
				Infobox: "Test",
				Content: strings.Repeat("Content paragraph. ", 50),
				Attributes: []searxng.InfoboxAttribute{
					{Label: "Key1", Value: "Value1"},
					{Label: "Key2", Value: "Value2"},
				},
				URLs: []searxng.InfoboxURL{
					{Title: "Link", URL: "https://example.com"},
				},
			},
		},
		Results: []searxng.SearchResult{
			{Title: "Result", URL: "https://example.com", Content: "Content", Engine: "google"},
		},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
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
				_ = unescapeIfNeeded(input)
			}
		})
	}
}

// ============================================================================
// Dedup Benchmarks
// ============================================================================

func BenchmarkDeduplicateAnswers(b *testing.B) {
	answers := []searxng.Answer{
		{
			Answer: "Go is a statically typed, compiled programming language designed at Google. " +
				"It provides garbage collection, CSP-style concurrency, and structural typing. More at Wikipedia",
			Engine: "duckduckgo",
		},
		{Answer: "42", Engine: "calculator"},
		{Answer: "192.168.1.1", Engine: "ip_plugin"},
	}
	infoboxes := []searxng.Infobox{
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
		_ = searxng.DeduplicateAnswers(answers, infoboxes)
	}
}

func BenchmarkDeduplicateAnswersManyInfoboxes(b *testing.B) {
	answers := []searxng.Answer{
		{Answer: "Some answer text that might match", Engine: "test"},
		{Answer: "Another unique answer", Engine: "test"},
		{Answer: "Third answer with content", Engine: "test"},
	}
	// 10 infoboxes with long content
	infoboxes := make([]searxng.Infobox, 10)
	for i := range infoboxes {
		infoboxes[i] = searxng.Infobox{
			Infobox: fmt.Sprintf("Infobox %d", i),
			Content: strings.Repeat(fmt.Sprintf("Content paragraph %d for testing. ", i), 30),
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = searxng.DeduplicateAnswers(answers, infoboxes)
	}
}

func BenchmarkDeduplicateAnswersNoInfoboxes(b *testing.B) {
	answers := []searxng.Answer{
		{Answer: "answer 1", Engine: "e1"},
		{Answer: "answer 2", Engine: "e2"},
	}
	// Fast path: empty infoboxes
	b.ReportAllocs()

	for b.Loop() {
		_ = searxng.DeduplicateAnswers(answers, nil)
	}
}

// ============================================================================
// Validation Benchmarks
// ============================================================================

func BenchmarkValidateSearchArgs(b *testing.B) {
	pageno := 1
	args := &searxng.SearchArgs{
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
		err := searxng.ValidateSearchArgs(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateSearchArgsMinimal(b *testing.B) {
	args := &searxng.SearchArgs{Query: "test"}

	b.ReportAllocs()

	for b.Loop() {
		err := searxng.ValidateSearchArgs(args)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Additional Benchmarks (REPORT_PERF 2026-04-19)
// ============================================================================

// makeLongContentResults generates n results with content well above MaxContentRunes
// including multi-byte UTF-8 characters.
func makeLongContentResults(n int) *searxng.SearchResponse {
	results := make([]searxng.SearchResult, n)

	base := strings.Repeat("這是一段很長的中文測試內容，包含多位元組字元。Go is a programming language. 日本語テスト。🎉🚀📊 ", 200)
	for i := range n {
		results[i] = searxng.SearchResult{
			Title:   fmt.Sprintf("長內容測試結果 #%d 🌐", i),
			URL:     fmt.Sprintf("https://example.com/long/%d", i),
			Content: base + fmt.Sprintf(" Result index %d.", i),
			Engine:  "google",
		}
	}

	return &searxng.SearchResponse{
		Query:           "long content test",
		NumberOfResults: n,
		Results:         results,
	}
}

func BenchmarkFormatResultsLongContent(b *testing.B) {
	resp := makeLongContentResults(20)

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
	}
}

// makeEntityResults generates n results with heavy HTML entities in titles and content.
func makeEntityResults(n int) *searxng.SearchResponse {
	results := make([]searxng.SearchResult, n)

	entities := []string{"&amp;", "&quot;", "&lt;", "&gt;", "&#39;"}
	for i := range n {
		e := entities[i%len(entities)]
		results[i] = searxng.SearchResult{
			Title: fmt.Sprintf(
				"A %s B %s C %s D",
				e,
				entities[(i+1)%len(entities)],
				entities[(i+2)%len(entities)],
			),
			URL: fmt.Sprintf("https://example.com/entity/%d", i),
			Content: fmt.Sprintf(
				"Content with %s symbols & %s more %s entities %s here %s end.",
				e,
				entities[(i+1)%len(entities)],
				entities[(i+2)%len(entities)],
				entities[(i+3)%len(entities)],
				entities[(i+4)%len(entities)],
			),
			Engine: "google",
		}
	}

	return &searxng.SearchResponse{
		Query:           "entity test",
		NumberOfResults: n,
		Results:         results,
	}
}

func BenchmarkFormatResultsWithEntities(b *testing.B) {
	resp := makeEntityResults(50)

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
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
			answers := make([]searxng.Answer, tc.nAnswers)
			for i := range answers {
				answers[i] = searxng.Answer{
					Answer: fmt.Sprintf("Answer text number %d with some content about topic %d", i, i),
					Engine: fmt.Sprintf("engine_%d", i%3),
				}
			}

			infoboxes := make([]searxng.Infobox, tc.nInfoboxes)
			for i := range infoboxes {
				infoboxes[i] = searxng.Infobox{
					Infobox: fmt.Sprintf("Topic %d", i),
					Content: strings.Repeat(fmt.Sprintf("Content paragraph %d for testing deduplication. ", i), 20),
				}
			}

			b.ReportAllocs()

			for b.Loop() {
				_ = searxng.DeduplicateAnswers(answers, infoboxes)
			}
		})
	}
}
