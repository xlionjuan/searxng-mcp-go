package main

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
	"searxng-mcp-go/internal/testhelper"
)

// loadSearchResponse loads a SearchResponse from a JSON fixture in testdata/.
func loadSearchResponse(tb testing.TB, fixture string) *searxng.SearchResponse {
	tb.Helper()

	var resp searxng.SearchResponse

	testhelper.LoadJSONFixture(tb, "testdata/"+fixture, &resp)

	return &resp
}

// ============================================================================
// Format Benchmarks
// ============================================================================

func BenchmarkFormatResults(b *testing.B) {
	resp := loadSearchResponse(b, "large_response_10.json")

	b.ReportAllocs()

	for b.Loop() {
		_ = formatResults(resp)
	}
}

func BenchmarkSearch(b *testing.B) {
	body := string(testhelper.ReadFixture(b, "testdata/sample_response.json"))

	cfg := &searxng.Config{
		SearXNGURL: "http://127.0.0.1",
		Timeout:    30 * time.Second,
		HTTPClient: &http.Client{
			Transport: testhelper.RoundTripperFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(body)),
					Request:    r,
				}, nil
			}),
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
	resp := loadSearchResponse(b, "large_response_100.json")

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
