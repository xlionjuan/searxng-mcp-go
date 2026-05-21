package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	searxng "searxng-mcp-go/internal/searxng"
)

var (
	errSomeTestError          = errors.New("some error")
	errNotValidationTestError = errors.New("not a validation error")
	errBadRequestTest         = errors.New("bad request")
	errConnectionRefusedTest  = errors.New("connection refused")
)

const testHelloBody = "hello"

// --- TEST-02: ValidationError tests ---

func TestValidationError(t *testing.T) {
	t.Parallel()

	t.Run("Is matches same field and message", func(t *testing.T) {
		t.Parallel()

		err1 := searxng.NewValidationError("query", "is required")
		err2 := searxng.NewValidationError("query", "is required")
		err3 := searxng.NewValidationError("query", "must be longer")
		err4 := searxng.NewValidationError("other_field", "is required")

		if !err1.Is(err2) {
			t.Errorf("err1.Is(err2) = false, want true (same field and message)")
		}

		if err1.Is(err3) {
			t.Errorf("err1.Is(err3) = true, want false (different message)")
		}

		if err1.Is(err4) {
			t.Errorf("err1.Is(err4) = true, want false (different field)")
		}
		// Non-ValidationError target
		if err1.Is(errSomeTestError) {
			t.Errorf("err1.Is(non-ValidationError) = true, want false")
		}
	})

	t.Run("IsValidationError detects ValidationError", func(t *testing.T) {
		t.Parallel()

		err := searxng.NewValidationError("query", "is required")

		if !searxng.IsValidationError(err) {
			t.Errorf("IsValidationError(err) = false, want true")
		}

		if searxng.IsValidationError(errNotValidationTestError) {
			t.Errorf("IsValidationError(non-ValidationError) = true, want false")
		}

		if searxng.IsValidationError(nil) {
			t.Errorf("IsValidationError(nil) = true, want false")
		}
	})

	t.Run("ValidationError wraps with Unwrap", func(t *testing.T) {
		t.Parallel()
		// Create a ValidationError wrapped in another error using fmt.Errorf
		validationErr := searxng.NewValidationError("test", "test message")
		// Use errors.Join to create a wrapped error (Go 1.20+)
		wrappedErr := fmt.Errorf("operation failed: %w", validationErr)

		// errors.Is should work through the wrapped error
		if !errors.Is(wrappedErr, validationErr) {
			t.Errorf("errors.Is(wrappedErr, validationErr) = false, want true")
		}

		// IsValidationError should detect it
		if !searxng.IsValidationError(wrappedErr) {
			t.Errorf("IsValidationError(wrappedErr) = false, want true")
		}
	})
}

func TestHTTPStatusError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode  int
		contentType string
		body        []byte
		errContains string
	}{
		{400, "text/html", []byte("Bad Request"), "searxng error (status 400)"},
		{401, "text/html", []byte("Unauthorized"), "searxng error (status 401)"},
		{403, "text/html", []byte("Forbidden"), "searxng error (status 403)"},
		{404, "text/html", []byte("Not Found"), "searxng error (status 404)"},
		{429, "text/html", []byte("Rate Limited"), "searxng error (status 429)"},
		{500, "text/html", []byte("Internal Error"), "searxng error (status 500)"},
		{502, "text/html", []byte("Bad Gateway"), "searxng error (status 502)"},
		{503, "text/html", []byte("Unavailable"), "searxng error (status 503)"},
		{504, "text/html", []byte("Timeout"), "searxng error (status 504)"},
		{418, "text/html", []byte("I'm a teapot"), "searxng error (status 418)"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			t.Parallel()

			err := searxng.HTTPStatusError(tc.statusCode, tc.contentType, tc.body)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.statusCode)
			}

			if !strings.Contains(strings.ToLower(err.Error()), tc.errContains) {
				t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
			}

			var searxngErr *searxng.SearXNGError
			if !errors.As(err, &searxngErr) {
				t.Fatalf("expected *SearXNGError, got type %T", err)
			}

			if searxngErr.StatusCode != tc.statusCode {
				t.Errorf("SearXNGError.StatusCode = %d, want %d", searxngErr.StatusCode, tc.statusCode)
			}

			if searxngErr.RespContentType != tc.contentType {
				t.Errorf("SearXNGError.RespContentType = %q, want %q", searxngErr.RespContentType, tc.contentType)
			}
		})
	}
}

func TestHTTPStatusError_HTMLBodyNotInErrorMessage(t *testing.T) {
	t.Parallel()

	htmlBody := []byte("<!DOCTYPE html><html><body>Internal Server Error</body></html>")

	err := searxng.HTTPStatusError(500, "text/html", htmlBody)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "<!DOCTYPE") || strings.Contains(errMsg, "<html>") || strings.Contains(errMsg, "Internal Server Error") {
		t.Errorf("error message should not contain HTML body content, got: %s", errMsg)
	}

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.ResponseBody != "" && (strings.Contains(searxngErr.ResponseBody, "<!DOCTYPE") || strings.Contains(searxngErr.ResponseBody, "<html>")) {
		t.Logf("ResponseBody contains HTML for debugging (this is allowed in structured field): %s", searxngErr.ResponseBody)
	}
}

func TestSearXNGError_ResponseBodyField(t *testing.T) {
	t.Parallel()

	err := searxng.NewSearXNGError(400, "text/html", "error details here", errBadRequestTest)

	var searxngErr *searxng.SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.ResponseBody != "error details here" {
		t.Fatalf("ResponseBody = %q, want %q", searxngErr.ResponseBody, "error details here")
	}
}

func TestSearXNGError_Unwrap(t *testing.T) {
	t.Parallel()

	underlying := errConnectionRefusedTest
	err := searxng.NewSearXNGError(0, "", "", underlying)

	unwrapped := errors.Unwrap(err)
	if !errors.Is(unwrapped, underlying) {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, underlying)
	}

	if !errors.Is(err, underlying) {
		t.Errorf("errors.Is(err, underlying) = false, want true")
	}
}

func TestHTMLResponseError_HTMLBodyNotInMessage(t *testing.T) {
	t.Parallel()

	htmlBody := "<!DOCTYPE html><html><head><title>Error</title></head><body>JSON not enabled</body></html>"
	err := &searxng.HTMLResponseError{Body: htmlBody, UnderlyingErr: nil}

	errMsg := err.Error()
	if strings.Contains(errMsg, "<!DOCTYPE") || strings.Contains(errMsg, "<html>") || strings.Contains(errMsg, "JSON not enabled") {
		t.Errorf("HTMLResponseError.Error() should not contain HTML body, got: %s", errMsg)
	}
}

// --- truncateBody 獨立單元測試 ---

func TestTruncateBody(t *testing.T) {
	t.Parallel()

	t.Run("nil input", func(t *testing.T) {
		t.Parallel()

		if got := searxng.TruncateBody(nil, 10); got != "" {
			t.Errorf("TruncateBody(nil, 10) = %q, want %q", got, "")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		if got := searxng.TruncateBody([]byte{}, 10); got != "" {
			t.Errorf("TruncateBody({}, 10) = %q, want %q", got, "")
		}
	})

	t.Run("shorter than maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := searxng.TruncateBody(body, 10); got != "hello" {
			t.Errorf("TruncateBody(hello, 10) = %q, want %q", got, "hello")
		}
	})

	t.Run("exactly maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := searxng.TruncateBody(body, 5); got != "hello" {
			t.Errorf("TruncateBody(hello, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("longer than maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello world")
		if got := searxng.TruncateBody(body, 5); got != "hello" {
			t.Errorf("TruncateBody(hello world, 5) = %q, want %q", got, "hello")
		}
	})

	t.Run("unicode multi-byte - cut mid-character", func(t *testing.T) {
		t.Parallel()
		// "你好世界" is 12 bytes in UTF-8 (3 bytes each)
		body := []byte("你好世界")
		// maxLen=5 will cut in the middle of the second character
		got := searxng.TruncateBody(body, 5)
		// Expect only the first complete rune "你" (3 bytes) — the function
		// applies string() to the byte slice, which may produce replacement
		// characters for incomplete UTF-8. We just verify it doesn't panic
		// and returns at most maxLen bytes.
		if len([]byte(got)) > 5 {
			t.Errorf("TruncateBody(你好世界, 5) returned %d bytes, want <= 5 bytes", len([]byte(got)))
		}
		// The first character "你" should be preserved if at least 3 bytes.
		if !containsRune(got, '你') && len([]byte(got)) >= 3 {
			t.Logf("TruncateBody(你好世界, 5) = %q (bytes: %d) — incomplete UTF-8 may produce replacement chars", got, len([]byte(got)))
		}
	})

	t.Run("unicode multi-byte - safe boundary", func(t *testing.T) {
		t.Parallel()
		// "abc你" is 6 bytes (3 ASCII + 3 for 你)
		body := []byte("abc你")

		got := searxng.TruncateBody(body, 6)
		if got != "abc你" {
			t.Errorf("TruncateBody(abc你, 6) = %q, want %q", got, "abc你")
		}
	})

	t.Run("zero maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := searxng.TruncateBody(body, 0); got != "" {
			t.Errorf("TruncateBody(hello, 0) = %q, want %q", got, "")
		}
	})

	t.Run("negative maxLen", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := searxng.TruncateBody(body, -1); got != "" {
			t.Errorf("TruncateBody(hello, -1) = %q, want %q", got, "")
		}
	})

	t.Run("unicode emoji", func(t *testing.T) {
		t.Parallel()
		// "🔥" is 4 bytes in UTF-8
		body := []byte("🔥🔥🔥")
		// maxLen=5 cuts in the middle of the second emoji
		got := searxng.TruncateBody(body, 5)
		if len([]byte(got)) > 5 {
			t.Errorf("TruncateBody(🔥🔥🔥, 5) returned %d bytes, want <= 5", len([]byte(got)))
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

func TestSearXNGError_Error_NilUnderlying(t *testing.T) {
	t.Parallel()

	err := searxng.NewSearXNGError(500, "text/html", "", nil)

	errMsg := err.Error()
	if errMsg != "searxng error: status 500, content-type: text/html" {
		t.Errorf("unexpected error message: %s", errMsg)
	}
}

func TestSearXNGError_Error_WithUnderlying(t *testing.T) {
	t.Parallel()

	underlying := errConnectionRefusedTest
	err := searxng.NewSearXNGError(500, "text/html", "", underlying)

	errMsg := err.Error()
	if !strings.Contains(errMsg, "searxng error (status 500)") {
		t.Errorf("expected error to contain 'searxng error (status 500)', got: %s", errMsg)
	}

	if !strings.Contains(errMsg, "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got: %s", errMsg)
	}
}
