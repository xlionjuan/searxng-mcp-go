package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"searxng-mcp-go/internal/searxng"
	"searxng-mcp-go/internal/testhelper"
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
			fixture:    "testdata/typed_translation_answer.json",
			wantAnswer: "[1] Translation: bonjour",
			wantEngine: "Engine: libretranslate",
		},
		{
			name:       "weather",
			fixture:    "testdata/typed_weather_answer.json",
			wantAnswer: "[1] Weather: Berlin, 11.2 °C, partly cloudy",
			wantEngine: "Engine: open_meteo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var resp searxng.SearchResponse

			testhelper.LoadJSONFixture(t, tt.fixture, &resp)

			// Apply typed answer fallback (normally done in normalizeResponse).
			for i := range resp.Answers {
				searxng.EnsureAnswerFallback(&resp.Answers[i])
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

// TestTypedAnswerFixture_EnsureAnswerFallback guards the typed-translation
// example in docs/MCP_TOOLS.md. After EnsureAnswerFallback, the JSON
// serialization of the fixture must contain "answer":"Translation: bonjour"
// and "engine":"libretranslate".
func TestTypedAnswerFixture_EnsureAnswerFallback(t *testing.T) {
	t.Parallel()

	var resp searxng.SearchResponse

	testhelper.LoadJSONFixture(t, "testdata/typed_translation_answer.json", &resp)

	if len(resp.Answers) == 0 {
		t.Fatal("fixture must contain at least one answer")
	}

	for i := range resp.Answers {
		searxng.EnsureAnswerFallback(&resp.Answers[i])
	}

	got, err := json.Marshal(resp.Answers[0])
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	for _, want := range []string{`"answer":"Translation: bonjour"`, `"engine":"libretranslate"`} {
		if !bytes.Contains(got, []byte(want)) {
			t.Fatalf("JSON answer missing %q in:\n%s", want, got)
		}
	}
}

func TestFormatResults_NilInput(t *testing.T) {
	t.Parallel()

	wantPrefix := "=== Web Search Results ===\nWarning: " + searxng.ExternalContentWarning + "\n\n"
	if got := formatResults(nil); got != wantPrefix+noResultsFound {
		t.Fatalf("formatResults(nil) = %q, want %q", got, wantPrefix+"No results found.")
	}
}

// TestFormatResults_ResultCountFormat locks the "Found N results for '...'"
// line format across these scenarios:
//   - mismatch: NumberOfResults > len(Results) → "Found N total (showing M)"
//   - match:    NumberOfResults == len(Results) → "Found N results"
//   - zero:     NumberOfResults == 0 → normalized to len(Results)
//   - under:    NumberOfResults < len(Results) → normalized to len(Results)
//   - negative: NumberOfResults < 0 → normalized to len(Results)
func TestFormatResults_ResultCountFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		resp   *searxng.SearchResponse
		want   string // substring the output must contain
		notWnt string // substring the output must NOT contain
	}{
		{
			name: "server count exceeds rendered",
			resp: &searxng.SearchResponse{
				Query:           "overflow",
				NumberOfResults: 100,
				Results: []searxng.SearchResult{
					{Title: "r1", URL: "https://e.com/1", Engine: "g"},
					{Title: "r2", URL: "https://e.com/2", Engine: "g"},
				},
			},
			want:   "Found 100 total (showing 2) results for 'overflow'",
			notWnt: "Found 100 results for 'overflow'",
		},
		{
			name: "server count matches rendered",
			resp: &searxng.SearchResponse{
				Query:           "exact",
				NumberOfResults: 10,
				Results: []searxng.SearchResult{
					{Title: "r1", URL: "https://e.com/1", Engine: "g"},
					{Title: "r2", URL: "https://e.com/2", Engine: "g"},
					{Title: "r3", URL: "https://e.com/3", Engine: "g"},
					{Title: "r4", URL: "https://e.com/4", Engine: "g"},
					{Title: "r5", URL: "https://e.com/5", Engine: "g"},
					{Title: "r6", URL: "https://e.com/6", Engine: "g"},
					{Title: "r7", URL: "https://e.com/7", Engine: "g"},
					{Title: "r8", URL: "https://e.com/8", Engine: "g"},
					{Title: "r9", URL: "https://e.com/9", Engine: "g"},
					{Title: "r10", URL: "https://e.com/10", Engine: "g"},
				},
			},
			want:   "Found 10 results for 'exact'",
			notWnt: "total (showing",
		},
		{
			name: "server count zero normalized to rendered",
			resp: &searxng.SearchResponse{
				Query:           "zero",
				NumberOfResults: 0,
				Results: []searxng.SearchResult{
					{Title: "r1", URL: "https://e.com/1", Engine: "g"},
					{Title: "r2", URL: "https://e.com/2", Engine: "g"},
					{Title: "r3", URL: "https://e.com/3", Engine: "g"},
					{Title: "r4", URL: "https://e.com/4", Engine: "g"},
					{Title: "r5", URL: "https://e.com/5", Engine: "g"},
				},
			},
			want:   "Found 5 results for 'zero'",
			notWnt: "total (showing",
		},
		{
			name: "server count less than rendered normalized",
			resp: &searxng.SearchResponse{
				Query:           "under",
				NumberOfResults: 1,
				Results: []searxng.SearchResult{
					{Title: "r1", URL: "https://e.com/1", Engine: "g"},
					{Title: "r2", URL: "https://e.com/2", Engine: "g"},
					{Title: "r3", URL: "https://e.com/3", Engine: "g"},
				},
			},
			want:   "Found 3 results for 'under'",
			notWnt: "total (showing",
		},
		{
			name: "negative server count normalized to rendered",
			resp: &searxng.SearchResponse{
				Query:           "neg",
				NumberOfResults: -5,
				Results: []searxng.SearchResult{
					{Title: "r1", URL: "https://e.com/1", Engine: "g"},
					{Title: "r2", URL: "https://e.com/2", Engine: "g"},
					{Title: "r3", URL: "https://e.com/3", Engine: "g"},
					{Title: "r4", URL: "https://e.com/4", Engine: "g"},
				},
			},
			want:   "Found 4 results for 'neg'",
			notWnt: "total (showing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := formatResults(tt.resp)

			if !strings.Contains(out, tt.want) {
				t.Errorf("formatResults() output missing %q\noutput:\n%s", tt.want, out)
			}

			if tt.notWnt != "" && strings.Contains(out, tt.notWnt) {
				t.Errorf("formatResults() output contains unexpected %q\noutput:\n%s", tt.notWnt, out)
			}
		})
	}
}

func TestLogUnresponsiveEngines(t *testing.T) {
	var buf bytes.Buffer

	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	resp := &searxng.SearchResponse{
		Query:               "test",
		Results:             []searxng.SearchResult{},
		NumberOfResults:     0,
		UnresponsiveEngines: [][]string{{"brave", `Suspended:" too many "requests`}},
		Debug:               true,
	}

	logUnresponsiveEngines(logger, resp)

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

func TestFormatResults_TruncatesUnicodeAtRuneBoundary(t *testing.T) {
	t.Parallel()

	wantContent := strings.Repeat("🔥", searxng.MaxContentRunes)
	tests := []struct {
		name   string
		prefix string
		resp   *searxng.SearchResponse
	}{
		{
			name:   "result content",
			prefix: "   Summary: ",
			resp: &searxng.SearchResponse{
				Query: "unicode result truncation",
				Results: []searxng.SearchResult{
					{
						Title:   "Long Unicode content",
						URL:     "https://example.com/unicode",
						Content: wantContent + "界",
						Engine:  "test",
					},
				},
				NumberOfResults: 1,
			},
		},
		{
			name:   "infobox content",
			prefix: "[1] Long Unicode infobox\n    ",
			resp: &searxng.SearchResponse{
				Query: "unicode infobox truncation",
				Infoboxes: []searxng.Infobox{
					{
						Infobox: "Long Unicode infobox",
						Content: wantContent + "界",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			out := formatResults(tt.resp)

			_, content, found := strings.Cut(out, tt.prefix)
			if !found {
				t.Fatalf("formatResults() missing content prefix %q in:\n%s", tt.prefix, out)
			}

			if end := strings.IndexByte(content, '\n'); end >= 0 {
				content = content[:end]
			}

			if !utf8.ValidString(content) {
				t.Fatal("formatted content is not valid UTF-8")
			}

			if content != wantContent {
				t.Fatalf("formatted content has %d runes, want %d intact emoji runes",
					utf8.RuneCountInString(content), searxng.MaxContentRunes)
			}
		})
	}
}
