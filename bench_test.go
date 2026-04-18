package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// Test Fixtures
// ============================================================================

// sampleSearXNGJSON is a realistic SearXNG response payload (~2.5KB)
const sampleSearXNGJSON = `{
  "query": "golang programming language",
  "number_of_results": 12400000,
  "results": [
    {
      "title": "The Go Programming Language",
      "url": "https://go.dev/",
      "content": "Go is an open source programming language supported by Google with built-in concurrency and a robust standard library.",
      "engine": "google",
      "publishedDate": "2024-01-15"
    },
    {
      "title": "Go Tutorial - W3Schools",
      "url": "https://www.w3schools.com/go/",
      "content": "Learn Go programming with examples. Go is a statically typed, compiled language designed at Google. Posted 3 hours ago by community.",
      "engine": "google"
    },
    {
      "title": "Getting Started with Go - Go by Example",
      "url": "https://gobyexample.com/",
      "content": "Go by Example is a hands-on introduction to Go using annotated example programs. Published yesterday by maintainers.",
      "engine": "bing"
    },
    {
      "title": "Go Downloads - golang.org",
      "url": "https://go.dev/dl/",
      "content": "Download Go binaries for your platform. The latest release includes security updates and performance improvements.",
      "engine": "google",
      "publishedDate": "2024-03-01"
    },
    {
      "title": "Effective Go - golang.org",
      "url": "https://go.dev/doc/effective_go",
      "content": "Effective Go gives tips for writing clear, idiomatic Go code. If you're new to Go, read the tutorial and then come back here.",
      "engine": "google"
    },
    {
      "title": "A Tour of Go",
      "url": "https://go.dev/tour/",
      "content": "An interactive introduction to Go. Learn the basics of Go through examples. 2 days ago we added new concurrency examples.",
      "engine": "duckduckgo"
    },
    {
      "title": "Go Modules Reference - golang.org",
      "url": "https://go.dev/ref/mod",
      "content": "A module is a collection of related Go packages that are versioned together as a single unit. Modules record precise dependency requirements.",
      "engine": "google"
    },
    {
      "title": "GitHub - golang/go",
      "url": "https://github.com/golang/go",
      "content": "The Go programming language source repository. Contribute to golang/go development by creating an account on GitHub.",
      "engine": "google",
      "publishedDate": "2024-02-20"
    },
    {
      "title": "Go (programming language) - Wikipedia",
      "url": "https://en.wikipedia.org/wiki/Go_(programming_language)",
      "content": "Go is a statically typed, compiled high-level programming language designed at Google by Robert Griesemer, Rob Pike, and Ken Thompson.",
      "engine": "wikipedia"
    },
    {
      "title": "Learn Go in Y Minutes",
      "url": "https://learnxinyminutes.com/docs/go/",
      "content": "Go was created out of the need to get work done. It's a pragmatic language that lets you write code quickly and efficiently.",
      "engine": "duckduckgo"
    }
  ],
  "answers": [
    {
      "answer": "Go is a statically typed, compiled programming language designed at Google by Robert Griesemer, Rob Pike, and Ken Thompson. It provides garbage collection, CSP-style concurrency, and structural typing. More at Wikipedia",
      "engine": "duckduckgo",
      "template": "wikipedia"
    }
  ],
  "infoboxes": [
    {
      "infobox": "Go (programming language)",
      "content": "Go is a statically typed, compiled programming language designed at Google by Robert Griesemer, Rob Pike, and Ken Thompson. It provides garbage collection, CSP-style concurrency, and structural typing.",
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
	for i := 0; i < n; i++ {
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

// makeSearchResultsNoDates generates n SearchResult entries WITHOUT PublishedDate.
// Content includes relative date phrases so inferDates exercises the parsing path.
func makeSearchResultsNoDates(n int) []SearchResult {
	results := make([]SearchResult, n)
	contents := []string{
		"Posted 3 hours ago by community",
		"Published yesterday by maintainers",
		"2 days ago we added new features",
		"5 days ago this was released",
		"Last week there was an announcement",
		"Report from last month about changes",
	}
	for i := 0; i < n; i++ {
		results[i] = SearchResult{
			Title:   fmt.Sprintf("Search Result Title %d", i),
			URL:     fmt.Sprintf("https://example.com/result/%d", i),
			Content: fmt.Sprintf("This is the content for result number %d. %s", i, contents[i%len(contents)]),
			Engine:  []string{"google", "bing", "duckduckgo"}[i%3],
		}
	}
	return results
}

// makeMixedSearchResults generates n SearchResult entries: 1/3 with API dates,
// 1/3 with date phrases in content (inferable), 1/3 with no dates at all.
func makeMixedSearchResults(n int) []SearchResult {
	results := make([]SearchResult, n)
	date := "2024-01-15"
	for i := 0; i < n; i++ {
		var r SearchResult
		r.Title = fmt.Sprintf("Search Result Title %d", i)
		r.URL = fmt.Sprintf("https://example.com/result/%d", i)
		r.Engine = []string{"google", "bing", "duckduckgo"}[i%3]
		switch i % 3 {
		case 0: // API date
			r.Content = fmt.Sprintf("This is the content for result number %d with no date phrase.", i)
			r.PublishedDate = &date
		case 1: // inferable date
			r.Content = fmt.Sprintf("This is the content for result number %d. Posted %d days ago.", i, (i%30)+1)
		default: // no date at all
			r.Content = fmt.Sprintf("This is the content for result number %d with no date information.", i)
		}
		results[i] = r
	}
	return results
}

// makeLargeSearchResponse creates a SearchResponse with n results for benchmarking
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
	data := []byte(sampleSearXNGJSON)
	b.ReportAllocs()
	for b.Loop() {
		var resp SearchResponse
		if err := json.Unmarshal(data, &resp); err != nil {
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
		if err := json.Unmarshal(data, &resp); err != nil {
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
		if _, err := resp.MarshalJSON(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalJSONLarge(b *testing.B) {
	resp := makeLargeSearchResponse(100)
	b.ReportAllocs()
	for b.Loop() {
		if _, err := resp.MarshalJSON(); err != nil {
			b.Fatal(err)
		}
	}
}

// Standard json.Marshal for comparison
func BenchmarkStdMarshalJSON(b *testing.B) {
	type stdSearchResponse SearchResponse
	resp := stdSearchResponse(*makeLargeSearchResponse(10))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(resp); err != nil {
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

func BenchmarkFormatResultsLarge(b *testing.B) {
	resp := makeLargeSearchResponse(100)
	b.ReportAllocs()
	for b.Loop() {
		_ = formatResults(resp)
	}
}

func BenchmarkFormatResultsInfoboxes(b *testing.B) {
	resp := &SearchResponse{
		Query:           "test",
		NumberOfResults: 1,
		Infoboxes: []Infobox{
			{
				Infobox: "Test",
				Content: strings.Repeat("Content paragraph. ", 50),
				Attributes: []InfoboxAttribute{
					{Label: "Key1", Value: "Value1"},
					{Label: "Key2", Value: "Value2"},
				},
				URLs: []InfoboxURL{
					{Title: "Link", URL: "https://example.com"},
				},
			},
		},
		Results: []SearchResult{
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
// Date Inference Benchmarks
// ============================================================================

func BenchmarkParseRelativeDate(b *testing.B) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		content string
	}{
		{"no_date", "This is some random content without date information at all"},
		{"hours_ago", "Posted 3 hours ago by the community"},
		{"days_ago", "Published 5 days ago by maintainers"},
		{"yesterday", "Article posted yesterday about the news"},
		{"last_week", "Report from last week about the changes"},
		{"german", "Nachricht vor 2 tagen veröffentlicht"},
		{"vorgestern", "Vorgestern wurde bekannt gegeben"},
	}
	b.ReportAllocs()
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			for b.Loop() {
				_ = parseRelativeDate(tc.content, now)
			}
		})
	}
}

func BenchmarkInferDates(b *testing.B) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	results := []SearchResult{
		{Title: "1", URL: "https://a.com", Content: "Posted 2 hours ago", Engine: "google"},
		{Title: "2", URL: "https://b.com", Content: "Published yesterday", Engine: "bing"},
		{Title: "3", URL: "https://c.com", Content: "5 days ago this happened", Engine: "duckduckgo"},
		{Title: "4", URL: "https://d.com", Content: "No date here", Engine: "google"},
		{Title: "5", URL: "https://e.com", Content: "Last week there was a report", Engine: "bing"},
		{Title: "6", URL: "https://f.com", Content: "Vorgestern wurde bekannt gegeben", Engine: "duckduckgo"},
		{Title: "7", URL: "https://g.com", Content: "Random content", Engine: "google"},
		{Title: "8", URL: "https://h.com", Content: "1 hour ago this was published", Engine: "bing"},
		{Title: "9", URL: "https://i.com", Content: "Nachricht vor 3 stunden veröffentlicht", Engine: "duckduckgo"},
		{Title: "10", URL: "https://j.com", Content: "Posted 10 days ago", Engine: "google"},
	}
	b.ReportAllocs()
	for b.Loop() {
		resp := &SearchResponse{Results: make([]SearchResult, len(results))}
		copy(resp.Results, results)
		inferDates(resp, &now)
	}
}

func BenchmarkInferDatesLarge(b *testing.B) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// 100 results without PublishedDate so inferDates exercises the parsing path
	results := makeSearchResultsNoDates(100)
	b.ReportAllocs()
	for b.Loop() {
		resp := &SearchResponse{Results: make([]SearchResult, len(results))}
		copy(resp.Results, results)
		inferDates(resp, &now)
	}
}

// ============================================================================
// Dedup Benchmarks
// ============================================================================

func BenchmarkDeduplicateAnswers(b *testing.B) {
	answers := []Answer{
		{Answer: "Go is a statically typed, compiled programming language designed at Google. It provides garbage collection, CSP-style concurrency, and structural typing. More at Wikipedia", Engine: "duckduckgo"},
		{Answer: "42", Engine: "calculator"},
		{Answer: "192.168.1.1", Engine: "ip_plugin"},
	}
	infoboxes := []Infobox{
		{
			Infobox: "Go",
			Content: strings.Repeat("Go is a statically typed, compiled programming language designed at Google. It provides garbage collection, CSP-style concurrency, and structural typing. ", 10),
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
		if err := ValidateSearchArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateSearchArgsMinimal(b *testing.B) {
	args := &SearchArgs{Query: "test"}
	b.ReportAllocs()
	for b.Loop() {
		if err := ValidateSearchArgs(args); err != nil {
			b.Fatal(err)
		}
	}
}

// ============================================================================
// Additional Benchmarks (REPORT_PERF 2026-04-19)
// ============================================================================

// makeLongContentResults generates n results with content well above MaxContentRunes
// including multi-byte UTF-8 characters.
func makeLongContentResults(n int) *SearchResponse {
	results := make([]SearchResult, n)
	base := strings.Repeat("這是一段很長的中文測試內容，包含多位元組字元。Go is a programming language. 日本語テスト。🎉🚀📊 ", 200)
	for i := 0; i < n; i++ {
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

func BenchmarkFormatResultsLongContent(b *testing.B) {
	resp := makeLongContentResults(20)
	b.ReportAllocs()
	for b.Loop() {
		_ = formatResults(resp)
	}
}

// makeNoDateResults generates n results with no date keywords and no PublishedDate.
func makeNoDateResults(n int) []SearchResult {
	results := make([]SearchResult, n)
	for i := 0; i < n; i++ {
		results[i] = SearchResult{
			Title:   fmt.Sprintf("Technical Document #%d", i),
			URL:     fmt.Sprintf("https://docs.example.com/page/%d", i),
			Content: fmt.Sprintf("This document describes configuration parameters and system architecture details for module %d.", i),
			Engine:  "google",
		}
	}
	return results
}

func BenchmarkInferDatesNoDatesLarge(b *testing.B) {
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	results := makeNoDateResults(100)
	b.ReportAllocs()
	for b.Loop() {
		resp := &SearchResponse{Results: make([]SearchResult, len(results))}
		copy(resp.Results, results)
		inferDates(resp, &now)
	}
}

// makeEntityResults generates n results with heavy HTML entities in titles and content.
func makeEntityResults(n int) *SearchResponse {
	results := make([]SearchResult, n)
	entities := []string{"&amp;", "&quot;", "&lt;", "&gt;", "&#39;"}
	for i := 0; i < n; i++ {
		e := entities[i%len(entities)]
		results[i] = SearchResult{
			Title:   fmt.Sprintf("A %s B %s C %s D", e, entities[(i+1)%len(entities)], entities[(i+2)%len(entities)]),
			URL:     fmt.Sprintf("https://example.com/entity/%d", i),
			Content: fmt.Sprintf("Content with %s symbols & %s more %s entities %s here %s end.", e, entities[(i+1)%len(entities)], entities[(i+2)%len(entities)], entities[(i+3)%len(entities)], entities[(i+4)%len(entities)]),
			Engine:  "google",
		}
	}
	return &SearchResponse{
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

func BenchmarkFullPipelineLarge(b *testing.B) {
	// Build a mixed SearchResponse: API dates, inferable dates, no dates
	results := makeMixedSearchResults(100)
	largeResp := &SearchResponse{
		Query:           "golang programming",
		NumberOfResults: 100,
		Answers: []Answer{
			{Answer: "42", Engine: "calculator"},
			{Answer: "192.168.1.1", Engine: "ip_plugin"},
		},
		Infoboxes: []Infobox{
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
		},
		Suggestions: []string{"golang tutorial", "golang concurrency", "golang vs rust"},
		Results:     results,
	}
	data, err := json.Marshal(largeResp)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for b.Loop() {
		var resp SearchResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
		resp.Answers = deduplicateAnswers(resp.Answers, resp.Infoboxes)
		inferDates(&resp, &now)
		_ = formatResults(&resp)
	}
}

// ============================================================================
// Full Pipeline Benchmark (unmarshal -> dedup -> infer -> format)
// ============================================================================

func BenchmarkFullPipeline(b *testing.B) {
	data := []byte(sampleSearXNGJSON)
	now := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	b.ReportAllocs()
	for b.Loop() {
		var resp SearchResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			b.Fatal(err)
		}
		resp.Answers = deduplicateAnswers(resp.Answers, resp.Infoboxes)
		inferDates(&resp, &now)
		_ = formatResults(&resp)
	}
}
