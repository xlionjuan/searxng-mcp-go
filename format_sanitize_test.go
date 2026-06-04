package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"searxng-mcp-go/internal/searxng"
)

// ============================================================================
// sanitizeTerminalControl unit tests
// ============================================================================

func TestSanitizeTerminalControl(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		// --- pass-through cases ---
		{name: "empty string", input: "", want: ""},
		{name: "plain ASCII", input: "hello world", want: "hello world"},
		{name: "CJK preserved", input: "日本語 中文", want: "日本語 中文"},
		{name: "emoji preserved", input: "🔥 rocket 🎉", want: "🔥 rocket 🎉"},
		{name: "mixed unicode", input: "café naïve 你好", want: "café naïve 你好"},
		{name: "tabs and newlines preserved", input: "line1\nline2\tcol", want: "line1\nline2\tcol"},
		{name: "carriage return rewritten", input: "line1\r\nline2", want: `line1\x0d` + "\nline2"},
		{name: "punctuation preserved", input: "Hello, World! 123 ?.;:", want: "Hello, World! 123 ?.;:"},

		// --- single-byte C0 controls (other than \t \n) ---
		{name: "NUL byte", input: "before\x00after", want: `before\x00after`},
		{name: "BEL", input: "before\x07after", want: `before\x07after`},
		{name: "BS backspace", input: "ab\x08cd", want: `ab\x08cd`},
		{name: "VT vertical tab", input: "ab\x0bcd", want: `ab\x0bcd`},
		{name: "FF form feed", input: "ab\x0ccd", want: `ab\x0ccd`},
		{name: "SO", input: "ab\x0ecd", want: `ab\x0ecd`},
		{name: "SI", input: "ab\x0fcd", want: `ab\x0fcd`},
		{
			name:  "DLE through US (0x10-0x1F)",
			input: "\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1c\x1d\x1e\x1f",
			want:  `\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1c\x1d\x1e\x1f`,
		},

		// --- ESC (the critical byte) ---
		{name: "ESC alone", input: "ab\x1bcd", want: `ab\x1bcd`},
		{name: "ESC at start", input: "\x1b[31mred", want: `\x1b[31mred`},
		{name: "ANSI SGR (red)", input: "\x1b[31mhello\x1b[0m", want: `\x1b[31mhello\x1b[0m`},
		{name: "ANSI cursor movement", input: "\x1b[2Ahome", want: `\x1b[2Ahome`},
		{name: "OSC 52 clipboard write attempt", input: "evil\x1b]52;c;SGVsbG8=\x07text", want: `evil\x1b]52;c;SGVsbG8=\x07text`},
		{name: "OSC title set", input: "\x1b]0;new title\x07", want: `\x1b]0;new title\x07`},
		{name: "DCS device control", input: "\x1bP1;2;3abc\x1b\\", want: `\x1bP1;2;3abc\x1b\`},
		{name: "CSI question mark", input: "\x1b[?25h", want: `\x1b[?25h`},

		// --- DEL (0x7F) ---
		{name: "DEL byte", input: "ab\x7fcd", want: `ab\x7fcd`},

		// --- C1 controls (U+0080 - U+009F) ---
		{name: "C1 U+0080 (PADDING)", input: "a\u0080b", want: `a\x80b`},
		{name: "C1 U+0085 (NEL)", input: "a\u0085b", want: `a\x85b`},
		{name: "C1 U+008B (VT in C1)", input: "a\u008bb", want: `a\x8bb`},
		{name: "C1 U+008D (RI)", input: "a\u008db", want: `a\x8db`},
		{name: "C1 U+008F (SS3)", input: "a\u008fb", want: `a\x8fb`},
		{name: "C1 U+009B (CSI)", input: "a\u009bb", want: `a\x9bb`},
		{name: "C1 U+009D (OSC)", input: "a\u009db", want: `a\x9db`},
		{name: "C1 U+009F (APC)", input: "a\u009fb", want: `a\x9fb`},
		{name: "C1 boundary U+009C", input: "a\u009cb", want: `a\x9cb`},

		// --- edge cases ---
		{name: "leading ESC", input: "\x1b", want: `\x1b`},
		{name: "trailing ESC", input: "tail\x1b", want: `tail\x1b`},
		{name: "only control bytes", input: "\x01\x02\x03", want: `\x01\x02\x03`},
		{name: "ESC + ANSI + text + BEL", input: "prefix\x1b[1;32mGREEN\x1b[0m suffix\x07", want: `prefix\x1b[1;32mGREEN\x1b[0m suffix\x07`},

		// --- invalid UTF-8 bytes ---
		{name: "invalid UTF-8 byte 0x80 alone", input: "a\x80b", want: `a\x80b`},
		{name: "invalid UTF-8 byte 0xFF", input: "a\xffb", want: `a\xffb`},
		{name: "truncated UTF-8 sequence", input: "a\xc2b", want: `a\xc2b`},
		{name: "overlong encoding of NUL", input: "a\xc0\x80b", want: `a\xc0\x80b`},

		// --- preservation guarantees ---
		{name: "long clean unicode unchanged", input: strings.Repeat("a你b", 100), want: strings.Repeat("a你b", 100)},
		{name: "boundary near 0x20 (space)", input: " \x1f 0x20 ! ", want: ` \x1f 0x20 ! `},
		{name: "boundary near 0x7F (DEL)", input: "0x7e\x7f0x80", want: `0x7e\x7f0x80`},
		{name: "high Unicode preserved", input: "\U0001F600 smile", want: "\U0001F600 smile"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sanitizeTerminalControl(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeTerminalControl(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeTerminalControl_NoTerminalControlBytes(t *testing.T) {
	t.Parallel()

	// The fast path must return the original string unchanged.
	inputs := []string{
		"",
		"plain text",
		"日本語 中文 emoji 🔥",
		"tabs\tand\nnewlines ok",
		"normal punctuation: !@#$%^&*()_+-=[]{}|;':\",./<>?",
		strings.Repeat("hello world 你好 ", 50),
	}

	for _, input := range inputs {
		got := sanitizeTerminalControl(input)
		if got != input {
			t.Errorf("sanitizeTerminalControl(%q) mutated clean input to %q", input, got)
		}
	}
}

func TestSanitizeTerminalControl_NeverEmitsLiteralControlBytes(t *testing.T) {
	t.Parallel()

	// Inputs that contain every category of dangerous control sequence
	// must never appear in the output.
	dangerous := []string{
		"\x1b[31mred",
		"\x1b]52;c;SGVsbG8=\x07",
		"\x1b[2J\x1b[H",
		"\x1b]0;evil title\x07",
		"\x1bP+q\x1b\\",
		"\x1b[?25l\x1b[?1c",
		"\x1b(B\x1b)0",
		"\x07\x08\x0b\x0c\x7f",
		"a\u0080b\u0085c\u009bd",
		"mixed \x1b[31mANSI\x1b[0m and text",
		// HTML-entity-encoded ESC: &#x1b; -> ESC (this is what
		// UnescapeIfNeeded + sanitize must neutralize).
		"&#x1b;]52;c;SGVsbG8=&#x7;",
	}

	for _, input := range dangerous {
		out := sanitizeTerminalControl(input)

		for _, b := range []byte(out) {
			if b < 0x20 && b != '\t' && b != '\n' {
				t.Errorf("sanitizeTerminalControl(%q) leaked control byte 0x%02x in output %q", input, b, out)
			}

			if b == 0x7F {
				t.Errorf("sanitizeTerminalControl(%q) leaked DEL in output %q", input, out)
			}
		}

		for _, r := range out {
			if r >= 0x80 && r <= 0x9F {
				t.Errorf("sanitizeTerminalControl(%q) leaked C1 codepoint U+%04X in output %q", input, r, out)
			}
		}
	}
}

func TestSanitizeTerminalControl_PreservesValidUTF8(t *testing.T) {
	t.Parallel()

	// When sanitization rewrites a string, the result must still be valid
	// UTF-8 (no rune boundaries are crossed, no overlong sequences produced).
	mixed := "\x1b[31m日本語 🔥\x1b[0m café naïve"
	out := sanitizeTerminalControl(mixed)

	if !utf8.ValidString(out) {
		t.Fatalf("sanitizeTerminalControl produced invalid UTF-8: %q (bytes=% x)", out, []byte(out))
	}

	if !strings.Contains(out, "日本語") {
		t.Errorf("CJK stripped from output: %q", out)
	}

	if !strings.Contains(out, "🔥") {
		t.Errorf("emoji stripped from output: %q", out)
	}

	if !strings.Contains(out, "café") {
		t.Errorf("Latin diacritics stripped from output: %q", out)
	}
}

func TestSanitizeTerminalControl_HtmlEntityEscapedESC(t *testing.T) {
	t.Parallel()

	// The vulnerability scenario: an upstream SearXNG response contains
	// HTML-entity-encoded ESC (e.g., &#x1b;) which html.UnescapeString
	// decodes to a literal ESC byte. After UnescapeIfNeeded returns the
	// decoded string, sanitizeTerminalControl must neutralize the ESC.
	decoded := searxng.UnescapeIfNeeded("&#x1b;")
	if decoded != "\x1b" {
		t.Fatalf("UnescapeIfNeeded did not produce ESC: got % x", []byte(decoded))
	}

	out := sanitizeTerminalControl(decoded)
	if out != `\x1b` {
		t.Errorf("HTML-entity-decoded ESC not sanitized: got %q, want %q", out, `\x1b`)
	}
}
