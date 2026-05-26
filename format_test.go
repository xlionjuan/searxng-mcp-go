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
