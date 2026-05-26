// Package main implements a SearXNG MCP server providing web search via the Model Context Protocol.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
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

func readSampleResponse(tb testing.TB) []byte {
	tb.Helper()

	data, err := os.ReadFile("testdata/sample_response.json")
	if err != nil {
		tb.Fatal(err)
	}

	return data
}

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
	data := readSampleResponse(b)

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
	body := string(readSampleResponse(b))

	cfg := &searxng.Config{
		SearXNGURL: "http://127.0.0.1",
		Timeout:    30 * time.Second,
		HTTPClient: &http.Client{
			Transport: &staticRoundTripper{
				statusCode: http.StatusOK,
				header:     http.Header{"Content-Type": []string{"application/json"}},
				body:       body,
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
