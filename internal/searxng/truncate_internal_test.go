package searxng

import "testing"

// TestTruncateRunes covers the rune-safe truncation helper that is shared
// between the searxng deduplication pass and the root CLI text formatter.
// The cases lock the documented contract: never split a multi-byte UTF-8
// rune, return "" for non-positive limits, and return the original string
// unchanged when the input already fits within the limit.
func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("", 10); got != "" {
			t.Errorf("TruncateRunes(%q, 10) = %q, want %q", "", got, "")
		}
	})

	t.Run("limit zero", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("hello", 0); got != "" {
			t.Errorf("TruncateRunes(%q, 0) = %q, want %q", "hello", got, "")
		}
	})

	t.Run("limit negative", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("hello", -1); got != "" {
			t.Errorf("TruncateRunes(%q, -1) = %q, want %q", "hello", got, "")
		}
	})

	t.Run("shorter than limit", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("hi", 10); got != "hi" {
			t.Errorf("TruncateRunes(%q, 10) = %q, want %q", "hi", got, "hi")
		}
	})

	t.Run("exactly limit", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("hello", 5); got != "hello" {
			t.Errorf("TruncateRunes(%q, 5) = %q, want %q", "hello", got, "hello")
		}
	})

	t.Run("longer than limit", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("hello world", 5); got != "hello" {
			t.Errorf("TruncateRunes(%q, 5) = %q, want %q", "hello world", got, "hello")
		}
	})

	t.Run("unicode CJK - exactly limit", func(t *testing.T) {
		t.Parallel()

		// "你好世界" = 4 runes
		if got := TruncateRunes("你好世界", 4); got != "你好世界" {
			t.Errorf("TruncateRunes(%q, 4) = %q, want %q", "你好世界", got, "你好世界")
		}
	})

	t.Run("unicode CJK - over limit", func(t *testing.T) {
		t.Parallel()

		// "你好世界" = 4 runes, limit 2 → "你好"
		if got := TruncateRunes("你好世界", 2); got != "你好" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "你好世界", got, "你好")
		}
	})

	t.Run("unicode emoji - over limit", func(t *testing.T) {
		t.Parallel()

		// "🔥🔥🔥" = 3 runes, limit 2 → "🔥🔥"
		if got := TruncateRunes("🔥🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "🔥🔥🔥", got, "🔥🔥")
		}
	})

	t.Run("unicode emoji - exactly limit", func(t *testing.T) {
		t.Parallel()

		if got := TruncateRunes("🔥🔥", 2); got != "🔥🔥" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", "🔥🔥", got, "🔥🔥")
		}
	})

	t.Run("mixed ASCII and unicode", func(t *testing.T) {
		t.Parallel()

		// "a你b好" = 4 runes
		if got := TruncateRunes("a你b好", 3); got != "a你b" {
			t.Errorf("TruncateRunes(%q, 3) = %q, want %q", "a你b好", got, "a你b")
		}
	})

	t.Run("multi-byte boundary never splits rune", func(t *testing.T) {
		t.Parallel()

		// "你" is 3 bytes in UTF-8. TruncateRunes works on runes,
		// so it should never split a multi-byte character.
		s := "你你你"

		got := TruncateRunes(s, 2)
		if got != "你你" {
			t.Errorf("TruncateRunes(%q, 2) = %q, want %q", s, got, "你你")
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
		if got := TruncateRunes(longString, 9999); got != longString {
			t.Errorf("TruncateRunes(long, 9999) should return original string, got %q", got)
		}
	})
}
