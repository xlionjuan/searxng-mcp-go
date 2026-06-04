package main

import (
	"bytes"
	"log/slog"
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

			got := searxng.UnescapeIfNeeded(tt.input)
			if got != tt.want {
				t.Errorf("searxng.UnescapeIfNeeded(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatResults_TypedAnswerFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		response   *searxng.SearchResponse
		wantAnswer string
		wantEngine string
	}{
		{
			name: "translation",
			response: &searxng.SearchResponse{
				Answers: []searxng.Answer{
					{
						Answer:   "Translation: bonjour",
						Engine:   "libretranslate",
						Template: "answer/translations.html",
					},
				},
			},
			wantAnswer: "[1] Translation: bonjour",
			wantEngine: "Engine: libretranslate",
		},
		{
			name: "weather",
			response: &searxng.SearchResponse{
				Answers: []searxng.Answer{
					{
						Answer:   "Weather: Berlin, 11.2 °C, partly cloudy",
						Engine:   "open_meteo",
						Template: "answer/weather.html",
					},
				},
			},
			wantAnswer: "[1] Weather: Berlin, 11.2 °C, partly cloudy",
			wantEngine: "Engine: open_meteo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatResults(tt.response)
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

// TestFormatResults_NeutralizesTerminalControl verifies that formatResults
// strips or visibly encodes terminal control bytes (ESC, C0, C1) in
// upstream-supplied fields before writing to stdout. This is a defense
// against a malicious or compromised SearXNG instance that returns
// ANSI / OSC sequences in result text, query echo, suggestions, or any
// other field. JSON output is intentionally not covered here because this
// test only locks the CLI text path.
func TestFormatResults_NeutralizesTerminalControl(t *testing.T) {
	t.Parallel()

	resp := &searxng.SearchResponse{
		Query: "evil\x1b[31mred\x1b[0m\rquery",
		Answers: []searxng.Answer{
			{
				Answer: "ans\x1b]52;c;SGVsbG8=\x07here",
				Engine: "eng\x1b[2J",
			},
		},
		Infoboxes: []searxng.Infobox{
			{
				Infobox: "ib\x1b[31mtitle\x1b[0m\r",
				Content: "ib content with \x1b[2J\x1b[H CSI",
				Attributes: []searxng.InfoboxAttribute{
					{Label: "label\x07", Value: "value\x1b]0;evil\x07"},
				},
				URLs: []searxng.InfoboxURL{
					{Title: "url title\x1b", URL: "https://example.com/?x=\x1b"},
				},
			},
		},
		Results: []searxng.SearchResult{
			{
				Title:         "title\x1b[31mRED\x1b[0m\rspoof",
				URL:           "https://example.com/\x1b]52;c;SGVsbG8=\x07",
				Content:       "content with \x1bP+q\x1b\\ DCS and \u0085 NEL",
				Engine:        "engine\x07",
				PublishedDate: &[]string{"date\x1b"}[0],
			},
		},
		NumberOfResults: 1,
		Suggestions: []string{
			"sug\x1b[31mGESTION\x1b[0m\rspoof",
			"clean suggestion",
		},
	}

	out := formatResults(resp)

	// No literal ESC, BEL, BS, VT, FF, SO, SI, DLE..US, or DEL may
	// survive in the output.
	for _, b := range []byte(out) {
		switch {
		case b == '\t', b == '\n':
			// allowed layout whitespace
		case b < 0x20:
			t.Errorf("formatResults leaked C0 control byte 0x%02x in output:\n%s", b, out)
		case b == 0x7F:
			t.Errorf("formatResults leaked DEL in output:\n%s", out)
		}
	}

	// No C1 codepoints (U+0080..U+009F) may survive.
	for _, r := range out {
		if r >= 0x80 && r <= 0x9F {
			t.Errorf("formatResults leaked C1 codepoint U+%04X in output:\n%s", r, out)
		}
	}

	// The clean suggestion must appear verbatim (no accidental rewrite).
	if !strings.Contains(out, "- clean suggestion\n") {
		t.Errorf("clean suggestion missing or altered:\n%s", out)
	}

	// The number-of-results and section headers must be intact.
	for _, want := range []string{
		"=== Web Search Results ===",
		"=== Answers ===",
		"=== Infoboxes ===",
		"=== Results ===",
		"=== Search Suggestions ===",
		"Found 1 results for '",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected section %q missing in output:\n%s", want, out)
		}
	}
}

// TestFormatResults_HtmlEntityEscapedControlIsNeutralized reproduces the
// specific scenario from the original report: an upstream SearXNG response
// contains HTML-entity-encoded ESC (e.g., &#x1b;) which html.UnescapeString
// decodes into a literal ESC byte. After UnescapeIfNeeded, the field still
// contains ESC and would otherwise be emitted to the terminal.
func TestFormatResults_HtmlEntityEscapedControlIsNeutralized(t *testing.T) {
	t.Parallel()

	// Build a response whose title contains the entity-encoded OSC 52
	// clipboard payload. UnescapeIfNeeded turns &#x1b; into ESC, so
	// the test verifies that the format layer's sanitization catches it.
	resp := &searxng.SearchResponse{
		Query: "&#x1b;]52;c;SGVsbG8=&#x7;",
		Results: []searxng.SearchResult{
			{
				Title:   "&#x1b;[31mRED&#x1b;[0m",
				URL:     "https://example.com/",
				Content: "&#x1b;[2J&#x1b;[H wipe screen",
				Engine:  "google",
			},
		},
		NumberOfResults: 1,
	}

	out := formatResults(resp)

	for _, b := range []byte(out) {
		switch {
		case b == '\t', b == '\n':
		case b < 0x20:
			t.Errorf("formatResults leaked control byte 0x%02x after HTML entity decode:\n%s", b, out)
		case b == 0x7F:
			t.Errorf("formatResults leaked DEL after HTML entity decode:\n%s", out)
		}
	}
}

// TestFormatResults_UnicodePreserved ensures that the sanitizer does not
// alter ordinary Unicode (CJK, emoji, accented Latin). This guards against
// over-aggressive escaping that would damage legitimate result content.
func TestFormatResults_UnicodePreserved(t *testing.T) {
	t.Parallel()

	resp := &searxng.SearchResponse{
		Query: "café 日本語 🔥",
		Results: []searxng.SearchResult{
			{
				Title:   "Golang 教程 — 面向中文开发者",
				URL:     "https://go.dev/zh-hans/",
				Content: "Go 是一种开源编程语言 🔥🚀\n支持 unicode。",
				Engine:  "google",
			},
		},
		NumberOfResults: 1,
	}

	out := formatResults(resp)

	for _, want := range []string{
		"café 日本語 🔥",
		"Golang 教程 — 面向中文开发者",
		"https://go.dev/zh-hans/",
		"Go 是一种开源编程语言 🔥🚀",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected Unicode fragment %q in output, got:\n%s", want, out)
		}
	}
}
