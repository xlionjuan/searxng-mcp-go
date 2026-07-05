package answer_test

import (
	"testing"

	"searxng-mcp-go/internal/searxng/answer"
)

func TestTruncateRunes_EmptyString(t *testing.T) {
	if got := answer.TruncateRunes("", 10); got != "" {
		t.Errorf("TruncateRunes(%q, 10) = %q, want %q", "", got, "")
	}
}

func TestTruncateRunes_NonPositiveLimit(t *testing.T) {
	t.Run("zero limit", func(t *testing.T) {
		if got := answer.TruncateRunes("hello", 0); got != "" {
			t.Errorf("TruncateRunes(%q, 0) = %q, want %q", "hello", got, "")
		}
	})

	t.Run("negative limit", func(t *testing.T) {
		if got := answer.TruncateRunes("hello", -1); got != "" {
			t.Errorf("TruncateRunes(%q, -1) = %q, want %q", "hello", got, "")
		}
	})
}

func TestTruncateRunes_WithinLimit(t *testing.T) {
	t.Run("shorter than limit", func(t *testing.T) {
		if got := answer.TruncateRunes("hi", 10); got != "hi" {
			t.Errorf("TruncateRunes(%q, 10) = %q, want %q", "hi", got, "hi")
		}
	})

	t.Run("exactly limit", func(t *testing.T) {
		if got := answer.TruncateRunes("hello", 5); got != "hello" {
			t.Errorf("TruncateRunes(%q, 5) = %q, want %q", "hello", got, "hello")
		}
	})

	t.Run("large limit no truncation", func(t *testing.T) {
		s := "hello world, this is a longer string for testing"

		if got := answer.TruncateRunes(s, 9999); got != s {
			t.Errorf("TruncateRunes(long, 9999) should return original string, got %q", got)
		}
	})
}

func TestTruncateRunes_ExceedsLimit(t *testing.T) {
	if got := answer.TruncateRunes("hello world", 5); got != "hello" {
		t.Errorf("TruncateRunes(%q, 5) = %q, want %q", "hello world", got, "hello")
	}
}

func TestTruncateRunes_Unicode(t *testing.T) {
	t.Run("CJK exactly limit", func(t *testing.T) {
		if got := answer.TruncateRunes("你好世界", 4); got != "你好世界" {
			t.Errorf("TruncateRunes(%q, 4) = %q, want %q", "你好世界", got, "你好世界")
		}
	})

	t.Run("CJK over limit", func(t *testing.T) {
		if got := answer.TruncateRunes("你好世界", 2); got != "你好" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "你好世界", got, "你好")
		}
	})

	t.Run("emoji over limit", func(t *testing.T) {
		if got := answer.TruncateRunes("🔥🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "🔥🔥🔥", got, "🔥🔥")
		}
	})

	t.Run("emoji exactly limit", func(t *testing.T) {
		if got := answer.TruncateRunes("🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "🔥🔥", got, "🔥🔥")
		}
	})

	t.Run("mixed ascii and unicode", func(t *testing.T) {
		if got := answer.TruncateRunes("a你b好", 3); got != "a你b" {
			t.Errorf("TruncateRunes(%q, 3) = %q, want %q", "a你b好", got, "a你b")
		}
	})

	t.Run("multi-byte boundary never splits rune", func(t *testing.T) {
		s := "你你你"
		got := answer.TruncateRunes(s, 2)

		if got != "你你" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", s, got, "你你")
		}

		for i, r := range got {
			if r != '你' {
				t.Errorf("rune at position %d = %U, want %U", i, r, '你')
			}
		}
	})
}
