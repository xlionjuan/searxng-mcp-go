package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// --- unescapeIfNeeded tests ---

func TestUnescapeIfNeeded(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty string", input: "", want: ""},
		{name: "no HTML entity", input: "hello world", want: "hello world"},
		{name: "ampersand entity", input: "hello &amp; world", want: "hello & world"},
		{name: "lt and gt entities", input: "&lt;div&gt;", want: "<div>"},
		{name: "quot entity", input: `&quot;quoted&quot;`, want: `"quoted"`},
		{name: "numeric entity &#39;", input: "&#39;", want: "'"},
		{name: "hex entity &#x27;", input: "&#x27;", want: "'"},
		{name: "mixed entities", input: "a &amp; b &lt; c", want: "a & b < c"},
		{name: "bare ampersand no valid entity", input: "only & symbol", want: "only & symbol"},
		{name: "unicode with entities", input: "日本語 &amp; 中文", want: "日本語 & 中文"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := unescapeIfNeeded(tt.input)
			if got != tt.want {
				t.Errorf("unescapeIfNeeded(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- formatResults tests ---

func TestFormatResults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		resp           *searxng.SearchResponse
		wantContains   []string
		wantNotContain string
		wantResult     string
	}{
		{
			name: "single answer with engine",
			resp: &searxng.SearchResponse{
				Query: "sha512 hello",
				Answers: []searxng.Answer{
					{Answer: "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043", Engine: "plugin:hash_plugin"},
				},
				Results: []searxng.SearchResult{
					{Title: "Hash Result", URL: "https://example.com", Content: "Some content", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantContains: []string{"=== Answers ===", "9b71d224bd62f3785d96d46ad3ea3d73319bfbc2890caadae2dff72519673ca72323c3d99ba5c11d7c7acc6e14b8c5da0c4663475c2e5c3adef46f73bcdec043", "Engine: plugin:hash_plugin", "=== Results ===", "Hash Result"},
		},
		{
			name: "multiple answers",
			resp: &searxng.SearchResponse{
				Query: "random",
				Answers: []searxng.Answer{
					{Answer: "42", Engine: "random_plugin"},
					{Answer: "3.14", Engine: "math_plugin"},
				},
				Results: []searxng.SearchResult{},
			},
			wantContains: []string{"=== Answers ===", "[1] 42", "Engine: random_plugin", "[2] 3.14", "Engine: math_plugin"},
		},
		{
			name: "answer without engine",
			resp: &searxng.SearchResponse{
				Query: "ip",
				Answers: []searxng.Answer{
					{Answer: "203.0.113.42"},
				},
				Results: []searxng.SearchResult{},
			},
			wantContains:   []string{"=== Answers ===", "203.0.113.42"},
			wantNotContain: "Engine:",
		},
		{
			name: "answers only no results",
			resp: &searxng.SearchResponse{
				Query: "avg 123 548 2.04 24.2",
				Answers: []searxng.Answer{
					{Answer: "174.31", Engine: "stats_plugin"},
				},
				Results:         []searxng.SearchResult{},
				NumberOfResults: 0,
			},
			wantContains:   []string{"=== Answers ===", "174.31"},
			wantNotContain: "No results found.",
		},
		{
			name: "answers before infoboxes before results",
			resp: &searxng.SearchResponse{
				Query: "apple",
				Answers: []searxng.Answer{
					{Answer: "192.168.1.1", Engine: "ip_plugin"},
				},
				Infoboxes: []searxng.Infobox{
					{Infobox: "Apple", Content: "A fruit."},
				},
				Results: []searxng.SearchResult{
					{Title: "Apple - Fruit", URL: "https://example.com/apple", Content: "An apple is a fruit.", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantContains: []string{"=== Answers ===", "192.168.1.1", "=== Infoboxes ===", "Apple", "=== Results ===", "Found 1 results", "Apple - Fruit"},
		},
		{
			name: "no answers when empty",
			resp: &searxng.SearchResponse{
				Query: "test query",
				Results: []searxng.SearchResult{
					{Title: "Test", URL: "https://example.com", Content: "Test content", Engine: "google"},
				},
				NumberOfResults: 1,
			},
			wantNotContain: "=== Answers ===",
		},
		{
			name: "normal results with content",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "Test Title 1",
						URL:     "https://example.com/1",
						Content: "Test content 1",
						Engine:  "google",
						PublishedDate: func() *string {
							s := "2026-04-20"

							return &s
						}(),
					},
					{
						Title:   "Test Title 2",
						URL:     "https://example.com/2",
						Content: "",
						Engine:  "bing",
					},
				},
				NumberOfResults: 2,
				Query:           "test query",
			},
			wantContains: []string{"=== Results ===", "Found 2 results", "test query", "Test Title 1", "https://example.com/1", "1. Test Title 1\n   URL: https://example.com/1\n   Summary: Test content 1\n   Published date: 2026-04-20", "Test Title 2"},
		},
		{
			name:       "empty results",
			resp:       &searxng.SearchResponse{Results: []searxng.SearchResult{}, NumberOfResults: 0, Query: "empty query"},
			wantResult: "No results found.",
		},
		{
			name: "content exceeding 4000 runes is truncated",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "Long Content Test",
						URL:     "https://example.com/long",
						Content: strings.Repeat("x", 4500),
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "long content",
			},
			wantContains:   []string{"=== Results ===", strings.Repeat("x", searxng.MaxContentRunes), "Long Content Test"},
			wantNotContain: strings.Repeat("x", searxng.MaxContentRunes+1),
		},
		{
			name: "HTML entities are unescaped in content",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "HTML Test &amp; More &lt;stuff&gt;",
						URL:     "https://example.com/html",
						Content: "Test &amp; &lt; &gt; entities",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "html entities",
			},
			wantContains: []string{"=== Results ===", "HTML Test & More <stuff>", "Test & < > entities"},
		},
		{
			name: "empty content is handled correctly",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "No Content",
						URL:     "https://example.com/empty",
						Content: "",
						Engine:  "bing",
					},
				},
				NumberOfResults: 1,
				Query:           "empty",
			},
			wantNotContain: "Summary:",
		},
		{
			name: "NumberOfResults greater than len(Results) - paginated response",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
					{
						Title:   "Result 2",
						URL:     "https://example.com/2",
						Content: "Content 2",
						Engine:  "bing",
					},
				},
				NumberOfResults: 100,
				Query:           "test",
			},
			wantContains: []string{"=== Results ===", "Found 100 results", "Result 1", "Result 2"},
		},
		{
			name: "suggestions are displayed after results",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "test",
				Suggestions:     []string{"suggested query 1", "suggested query 2"},
			},
			wantContains: []string{"=== Results ===", "=== Search Suggestions ===", "  - suggested query 1", "  - suggested query 2"},
		},
		{
			name: "no suggestions block when empty",
			resp: &searxng.SearchResponse{
				Results: []searxng.SearchResult{
					{
						Title:   "Result 1",
						URL:     "https://example.com/1",
						Content: "Content 1",
						Engine:  "google",
					},
				},
				NumberOfResults: 1,
				Query:           "test",
				Suggestions:     nil,
			},
			wantNotContain: "=== Search Suggestions ===",
		},
		{
			name: "infoboxes displayed before results",
			resp: &searxng.SearchResponse{
				Query:           "apple inc",
				NumberOfResults: 2,
				Infoboxes: []searxng.Infobox{
					{
						Infobox: "Apple Inc.",
						Content: "Apple Inc. is an American technology company.",
						Attributes: []searxng.InfoboxAttribute{
							{Label: "Type", Value: "Public"},
							{Label: "Industry", Value: "Technology"},
						},
						URLs: []searxng.InfoboxURL{
							{Title: "Official site", URL: "https://www.apple.com"},
							{Title: "Wikipedia", URL: "https://en.wikipedia.org/wiki/Apple_Inc."},
						},
					},
				},
				Results: []searxng.SearchResult{
					{
						Title:   "Apple - Official Site",
						URL:     "https://www.apple.com",
						Content: "Think Different.",
						Engine:  "google",
					},
				},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Apple Inc.",
				"Apple Inc. is an American technology company.",
				"Attributes:",
				"- Type: Public",
				"- Industry: Technology",
				"URLs:",
				"- Official site: https://www.apple.com",
				"- Wikipedia: https://en.wikipedia.org/wiki/Apple_Inc.",
				"=== Results ===",
				"Found 2 results for 'apple inc'",
				"Apple - Official Site",
			},
		},
		{
			name: "infoboxes without results still show infoboxes",
			resp: &searxng.SearchResponse{
				Query:           "test",
				NumberOfResults: 0,
				Infoboxes: []searxng.Infobox{
					{
						Infobox: "Test Infobox",
						Content: "Some info content",
					},
				},
				Results: []searxng.SearchResult{},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Test Infobox",
				"Some info content",
			},
			wantNotContain: "No results found.",
		},
		{
			name: "infobox with empty content",
			resp: &searxng.SearchResponse{
				Query:           "test",
				NumberOfResults: 1,
				Infoboxes: []searxng.Infobox{
					{
						Infobox: "Minimal Infobox",
						Content: "",
						Attributes: []searxng.InfoboxAttribute{
							{Label: "Key", Value: "Value"},
						},
					},
				},
				Results: []searxng.SearchResult{
					{
						Title:  "Result 1",
						URL:    "https://example.com/1",
						Engine: "google",
					},
				},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"[1] Minimal Infobox",
				"Attributes:",
				"- Key: Value",
				"=== Results ===",
			},
		},
		{
			name: "infoboxes appear before suggestions",
			resp: &searxng.SearchResponse{
				Query:           "test",
				NumberOfResults: 1,
				Infoboxes: []searxng.Infobox{
					{
						Infobox: "Test IB",
						Content: "Info content",
					},
				},
				Results: []searxng.SearchResult{
					{
						Title:  "Result 1",
						URL:    "https://example.com/1",
						Engine: "google",
					},
				},
				Suggestions: []string{"suggestion 1"},
			},
			wantContains: []string{
				"=== Infoboxes ===",
				"=== Results ===",
				"Found 1 results",
				"=== Search Suggestions ===",
			},
		},
		{
			name: "suggestions only should not return no results found",
			resp: &searxng.SearchResponse{
				Query:       "typoed qurey",
				Suggestions: []string{"typoed query", "typed query", "torpedoed"},
			},
			wantContains:   []string{"=== Search Suggestions ===", "  - typoed query", "  - typed query", "  - torpedoed"},
			wantNotContain: "No results found.",
		},
		{
			name: "section order: answers then infoboxes then results then suggestions",
			resp: &searxng.SearchResponse{
				Query: "full",
				Answers: []searxng.Answer{
					{Answer: "42", Engine: "calc"},
				},
				Infoboxes: []searxng.Infobox{
					{Infobox: "Info", Content: "Some info."},
				},
				Results: []searxng.SearchResult{
					{Title: "Result", URL: "https://example.com", Content: "Content", Engine: "google"},
				},
				NumberOfResults: 1,
				Suggestions:     []string{"related query"},
			},
			wantResult: "ORDERED", // sentinel: handled by index-based check below
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := formatResults(tt.resp)

			if tt.wantResult != "" {
				if tt.wantResult == "ORDERED" {
					// Index-based assertion: verify section headers appear in order
					headers := []string{"=== Answers ===", "=== Infoboxes ===", "=== Results ===", "=== Search Suggestions ==="}
					for i := 1; i < len(headers); i++ {
						prev := strings.Index(result, headers[i-1])

						curr := strings.Index(result, headers[i])
			switch {
			case prev == -1:
				t.Errorf("expected %q in output", headers[i-1])
			case curr == -1:
				t.Errorf("expected %q in output", headers[i])
			case curr < prev:
				t.Errorf("section order wrong: %q (pos %d) should come after %q (pos %d)", headers[i], curr, headers[i-1], prev)
			}
					}

					return
				}

				if result != tt.wantResult {
					t.Errorf("expected %q, got: %s", tt.wantResult, result)
				}

				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("expected %q in output, got: %s", want, result)
				}
			}

			if tt.name == "normal results with content" {
				firstTitle := strings.Index(result, "1. Test Title 1")
				firstSummary := strings.Index(result, "   Summary: Test content 1")
				firstPublished := strings.Index(result, "   Published date: 2026-04-20")

				secondTitle := strings.Index(result, "2. Test Title 2")
				if firstTitle == -1 || firstSummary == -1 || firstPublished == -1 || secondTitle == -1 {
					t.Fatalf("expected both result blocks and the first summary in output, got: %s", result)
				}

				if firstTitle >= firstSummary || firstSummary >= firstPublished || firstPublished >= secondTitle {
					t.Fatalf("expected first result summary and published date to belong to the first result block, got: %s", result)
				}
			}

			if tt.wantNotContain != "" && strings.Contains(result, tt.wantNotContain) {
				t.Errorf("did not expect %q in output, got: %s", tt.wantNotContain, result)
			}
		})
	}
}

func TestFormatResults_TypedAnswerFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fixture    string
		wantAnswer string
		wantEngine string
	}{
		{
			name:       "translation",
			fixture:    "typed_translation_answer.json",
			wantAnswer: "[1] Translation: bonjour",
			wantEngine: "Engine: libretranslate",
		},
		{
			name:       "weather",
			fixture:    "typed_weather_answer.json",
			wantAnswer: "[1] Weather: Berlin, 11.2 °C, partly cloudy",
			wantEngine: "Engine: open_meteo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			var resp searxng.SearchResponse

			err = json.Unmarshal(body, &resp)
			if err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}

			got := formatResults(&resp)
			for _, want := range []string{"=== Answers ===", tt.wantAnswer, tt.wantEngine} {
				if !strings.Contains(got, want) {
					t.Fatalf("formatResults() missing %q in:\n%s", want, got)
				}
			}
		})
	}
}

func TestFormatResults_NilInput(t *testing.T) {
	t.Parallel()

	if got := formatResults(nil); got != noResultsFound {
		t.Fatalf("formatResults(nil) = %q, want %q", got, "No results found.")
	}
}

// --- truncateRunes 獨立單元測試 ---

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes("", 10); got != "" {
			t.Errorf("truncateRunes(\"\", 10) = %q, want %q", got, "")
		}
	})

	t.Run("limit zero", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes(testHelloBody, 0); got != "" {
			t.Errorf("truncateRunes(hello, 0) = %q, want %q", got, "")
		}
	})

	t.Run("limit negative", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes(testHelloBody, -1); got != "" {
			t.Errorf("truncateRunes(hello, -1) = %q, want %q", got, "")
		}
	})

	t.Run("shorter than limit", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes("hi", 10); got != "hi" {
			t.Errorf("truncateRunes(hi, 10) = %q, want %q", got, "hi")
		}
	})

	t.Run("exactly limit", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes("hello", 5); got != testHelloBody {
			t.Errorf("truncateRunes(hello, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("longer than limit", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes("hello world", 5); got != "hello" {
			t.Errorf("truncateRunes(hello world, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("unicode CJK - exactly limit", func(t *testing.T) {
		t.Parallel()
		// "你好世界" = 4 runes
		if got := truncateRunes("你好世界", 4); got != "你好世界" {
			t.Errorf("truncateRunes(你好世界, 4) = %q, want %q", got, "你好世界")
		}
	})

	t.Run("unicode CJK - over limit", func(t *testing.T) {
		t.Parallel()
		// "你好世界" = 4 runes, limit 2 → "你好"
		if got := truncateRunes("你好世界", 2); got != "你好" {
			t.Errorf("truncateRunes(你好世界, 2) = %q, want %q", got, "你好")
		}
	})

	t.Run("unicode emoji - over limit", func(t *testing.T) {
		t.Parallel()
		// "🔥🔥🔥" = 3 runes, limit 2 → "🔥🔥"
		if got := truncateRunes("🔥🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("truncateRunes(🔥🔥🔥, 2) = %q, want %q", got, "🔥🔥")
		}
	})

	t.Run("unicode emoji - exactly limit", func(t *testing.T) {
		t.Parallel()

		if got := truncateRunes("🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("truncateRunes(🔥🔥, 2) = %q, want %q", got, "🔥🔥")
		}
	})

	t.Run("mixed ASCII and unicode", func(t *testing.T) {
		t.Parallel()
		// "a你b好" = 4 runes
		if got := truncateRunes("a你b好", 3); got != "a你b" {
			t.Errorf("truncateRunes(a你b好, 3) = %q, want %q", got, "a你b")
		}
	})

	t.Run("multi-byte boundary never splits rune", func(t *testing.T) {
		t.Parallel()
		// "你" is 3 bytes in UTF-8. truncateRunes works on runes,
		// so it should never split a multi-byte character.
		s := "你你你"

		got := truncateRunes(s, 2)
		if got != "你你" {
			t.Errorf("truncateRunes(你你你, 2) = %q, want %q", got, "你你")
		}
		// Verify that the result is valid UTF-8 and each rune is intact.
		for i, r := range got {
			if r != '你' {
				t.Errorf("rune at position %d = %U, want %U", i, r, '你')
			}
		}
	})

	t.Run("large limit - no truncation", func(t *testing.T) {
		t.Parallel()

		longString := "hello world, this is a longer string for testing"
		if got := truncateRunes(longString, 9999); got != longString {
			t.Errorf("truncateRunes(long, 9999) should return original string, got %q", got)
		}
	})
}

func TestFormatResults_DebugLogsUnresponsiveEngines(t *testing.T) {
	var buf bytes.Buffer

	old := slog.Default()

	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(old)

	resp := &searxng.SearchResponse{
		Query:               "test",
		Results:             []searxng.SearchResult{},
		NumberOfResults:     0,
		UnresponsiveEngines: [][]string{{"brave", `Suspended:" too many "requests`}},
		Debug:               true,
	}

	_ = formatResults(resp)

	if !strings.Contains(buf.String(), "unresponsive engine") {
		t.Fatalf("expected debug log for unresponsive engines, got: %s", buf.String())
	}
}
