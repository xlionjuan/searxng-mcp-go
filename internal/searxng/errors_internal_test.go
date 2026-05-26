package searxng

import (
	"errors"
	"fmt"
	"testing"
)

var errNotValidationTestError = errors.New("not a validation error")

// --- Private truncateBody tests ---

func TestTruncateBody(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()

		if got := truncateBody(nil, 10); got != "" {
			t.Errorf("truncateBody(nil, 10) = %q, want %q", got, "")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		if got := truncateBody([]byte{}, 10); got != "" {
			t.Errorf("truncateBody({}, 10) = %q, want %q", got, "")
		}
	})

	t.Run("shorter than maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := truncateBody(body, 10); got != "hello" {
			t.Errorf("truncateBody(hello, 10) = %q, want %q", got, "hello")
		}
	})

	t.Run("exactly maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := truncateBody(body, 5); got != "hello" {
			t.Errorf("truncateBody(hello, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("longer than maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello world")
		if got := truncateBody(body, 5); got != "hello" {
			t.Errorf("truncateBody(hello world, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("unicode multi-byte - cut mid-character", func(t *testing.T) {
		t.Parallel()
		// "你好世界" is 12 bytes in UTF-8 (3 bytes each)
		body := []byte("你好世界")
		// maxLen=5 will cut in the middle of the second character
		got := truncateBody(body, 5)
		// Expect only the first complete rune "你" (3 bytes) — the function
		// applies string() to the byte slice, which may produce replacement
		// characters for incomplete UTF-8. We just verify it doesn't panic
		// and returns at most maxLen bytes.
		if len([]byte(got)) > 5 {
			t.Errorf("truncateBody(你好世界, 5) returned %d bytes, want <= 5 bytes", len([]byte(got)))
		}
		// The first character "你" should be preserved if at least 3 bytes.
		if !containsRune(got, '你') && len([]byte(got)) >= 3 {
			t.Logf("truncateBody(你好世界, 5) = %q (bytes: %d) — incomplete UTF-8 may produce replacement chars", got, len([]byte(got)))
		}
	})

	t.Run("unicode multi-byte - safe boundary", func(t *testing.T) {
		t.Parallel()
		// "abc你" is 6 bytes (3 ASCII + 3 for 你)
		body := []byte("abc你")

		got := truncateBody(body, 6)
		if got != "abc你" {
			t.Errorf("truncateBody(abc你, 6) = %q, want %q", got, "abc你")
		}
	})

	t.Run("zero maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := truncateBody(body, 0); got != "" {
			t.Errorf("truncateBody(hello, 0) = %q, want %q", got, "")
		}
	})

	t.Run("negative maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := truncateBody(body, -1); got != "" {
			t.Errorf("truncateBody(hello, -1) = %q, want %q", got, "")
		}
	})

	t.Run("unicode emoji", func(t *testing.T) {
		t.Parallel()
		// "🔥" is 4 bytes in UTF-8
		body := []byte("🔥🔥🔥")
		// maxLen=5 cuts in the middle of the second emoji
		got := truncateBody(body, 5)
		if len([]byte(got)) > 5 {
			t.Errorf("truncateBody(🔥🔥🔥, 5) returned %d bytes, want <= 5", len([]byte(got)))
		}
	})
}

// containsRune checks if a string contains a specific rune.
func containsRune(s string, r rune) bool {
	for _, c := range s {
		if c == r {
			return true
		}
	}

	return false
}

// --- Private isValidationError tests ---

func TestIsValidationError(t *testing.T) {
	t.Parallel()

	t.Run("detects ValidationError", func(t *testing.T) {
		t.Parallel()

		err := NewValidationError("query", "is required")

		if !isValidationError(err) {
			t.Errorf("isValidationError(err) = false, want true")
		}

		if isValidationError(errNotValidationTestError) {
			t.Errorf("isValidationError(non-ValidationError) = true, want false")
		}

		if isValidationError(nil) {
			t.Errorf("isValidationError(nil) = true, want false")
		}
	})

	t.Run("detects wrapped ValidationError", func(t *testing.T) {
		t.Parallel()

		validationErr := NewValidationError("test", "test message")
		wrappedErr := fmt.Errorf("operation failed: %w", validationErr)

		if !isValidationError(wrappedErr) {
			t.Errorf("isValidationError(wrappedErr) = false, want true")
		}
	})
}
