package searxng

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
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

// --- buildErrorPreview tests ---

func TestBuildErrorPreview(t *testing.T) {
	t.Parallel()

	t.Run("empty input", func(t *testing.T) {
		t.Parallel()

		if got := buildErrorPreview(nil); got != "" {
			t.Errorf("buildErrorPreview(nil) = %q, want %q", got, "")
		}

		if got := buildErrorPreview([]byte{}); got != "" {
			t.Errorf("buildErrorPreview(empty) = %q, want %q", got, "")
		}
	})

	t.Run("short ASCII unchanged", func(t *testing.T) {
		t.Parallel()

		body := []byte("hello")
		if got := buildErrorPreview(body); got != "hello" {
			t.Errorf("buildErrorPreview(hello) = %q, want %q", got, "hello")
		}
	})

	t.Run("ASCII truncated to MaxErrorDisplayChars", func(t *testing.T) {
		t.Parallel()

		body := []byte(strings.Repeat("a", MaxErrorDisplayChars+50))
		got := buildErrorPreview(body)

		if len(got) != MaxErrorDisplayChars {
			t.Fatalf("buildErrorPreview length = %d, want %d", len(got), MaxErrorDisplayChars)
		}

		if got != strings.Repeat("a", MaxErrorDisplayChars) {
			t.Errorf("buildErrorPreview content mismatch: got %q", got)
		}
	})

	t.Run("oversized multi-byte body truncates on rune boundary", func(t *testing.T) {
		t.Parallel()

		// "你好" is 6 bytes in UTF-8 (3 bytes each). Build a body of many copies
		// so it well exceeds MaxErrorDisplayChars (200 bytes) and force the
		// truncation to land inside a multi-byte rune.
		body := []byte(strings.Repeat("你好", MaxErrorDisplayChars))

		got := buildErrorPreview(body)

		if len(got) > MaxErrorDisplayChars {
			t.Fatalf("buildErrorPreview length = %d, want <= %d", len(got), MaxErrorDisplayChars)
		}

		if !utf8.ValidString(got) {
			t.Errorf("buildErrorPreview produced invalid UTF-8: %q (bytes: %d)", got, len(got))
		}

		if got == "" {
			t.Fatal("buildErrorPreview returned empty string for non-empty input")
		}
	})

	t.Run("oversized emoji body truncates on rune boundary", func(t *testing.T) {
		t.Parallel()

		// "🔥" is 4 bytes in UTF-8. Build a body of many copies so it well
		// exceeds MaxErrorDisplayChars and force truncation inside a rune.
		body := []byte(strings.Repeat("🔥", MaxErrorDisplayChars))

		got := buildErrorPreview(body)

		if len(got) > MaxErrorDisplayChars {
			t.Fatalf("buildErrorPreview length = %d, want <= %d", len(got), MaxErrorDisplayChars)
		}

		if !utf8.ValidString(got) {
			t.Errorf("buildErrorPreview produced invalid UTF-8: %q (bytes: %d)", got, len(got))
		}
	})
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

// --- HTTPStatusError tests (from coverage test file) ---

func TestHTTPStatusError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode  int
		wantMessage string
	}{
		{http.StatusBadRequest, "bad request: the query parameters may be invalid"},
		{http.StatusUnauthorized, "unauthorized: authentication is required"},
		{http.StatusForbidden, "forbidden: access denied"},
		{http.StatusNotFound, "not found: the search endpoint could not be found"},
		{http.StatusTooManyRequests, "rate limited: too many requests"},
		{http.StatusInternalServerError, "internal server error"},
		{http.StatusBadGateway, "bad gateway"},
		{http.StatusServiceUnavailable, "service unavailable"},
		{http.StatusGatewayTimeout, "gateway timeout"},
		{999, "unexpected status code received"},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.statusCode), func(t *testing.T) {
			t.Parallel()

			err := HTTPStatusError(tt.statusCode, "text/plain", []byte("error body"))
			if err == nil {
				t.Fatal("HTTPStatusError() = nil, want error")
			}

			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("HTTPStatusError(%d).Error() = %q, want it to contain %q",
					tt.statusCode, err.Error(), tt.wantMessage)
			}

			var searxErr *SearXNGError
			if !errors.As(err, &searxErr) {
				t.Fatalf("HTTPStatusError(%d) type = %T, want *SearXNGError", tt.statusCode, err)
			}

			if searxErr.StatusCode != tt.statusCode {
				t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, tt.statusCode)
			}

			if searxErr.RespContentType != "text/plain" {
				t.Fatalf("RespContentType = %q, want %q", searxErr.RespContentType, "text/plain")
			}
		})
	}

	// Special case: HTMLResponseError for JSON-not-enabled servers
	t.Run("non-retryable errors include body", func(t *testing.T) {
		t.Parallel()

		body := []byte(`{"error": "invalid query"}`)

		err := HTTPStatusError(http.StatusBadRequest, "application/json", body)
		if err == nil {
			t.Fatal("HTTPStatusError() = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("type = %T, want *SearXNGError", err)
		}

		if searxErr.ResponseBody != `{"error": "invalid query"}` {
			t.Fatalf("ResponseBody = %q, want %q", searxErr.ResponseBody, `{"error": "invalid query"}`)
		}
	})

	t.Run("error body truncated to MaxErrorDisplayChars", func(t *testing.T) {
		t.Parallel()

		longBody := []byte(strings.Repeat("x", MaxErrorDisplayChars+50))
		err := HTTPStatusError(http.StatusInternalServerError, "text/plain", longBody)

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("type = %T, want *SearXNGError", err)
		}

		if len(searxErr.ResponseBody) != MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want %d", len(searxErr.ResponseBody), MaxErrorDisplayChars)
		}
	})

	t.Run("oversized multi-byte body truncates on rune boundary", func(t *testing.T) {
		t.Parallel()

		// "你好" is 6 bytes in UTF-8 (3 bytes each). A body of many copies well
		// exceeds MaxErrorDisplayChars and forces truncation inside a rune.
		multiByteBody := []byte(strings.Repeat("你好", MaxErrorDisplayChars))

		err := HTTPStatusError(http.StatusInternalServerError, "text/plain; charset=utf-8", multiByteBody)
		if err == nil {
			t.Fatal("HTTPStatusError() = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("type = %T, want *SearXNGError", err)
		}

		if len(searxErr.ResponseBody) > MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want <= %d", len(searxErr.ResponseBody), MaxErrorDisplayChars)
		}

		if !utf8.ValidString(searxErr.ResponseBody) {
			t.Errorf("ResponseBody is not valid UTF-8: %q (bytes: %d)", searxErr.ResponseBody, len(searxErr.ResponseBody))
		}

		if !strings.Contains(err.Error(), "searxng error (status 500) - content-type text/plain; charset=utf-8") {
			t.Errorf("Error() = %q, want to contain status and content-type", err.Error())
		}
	})
}

// --- SearXNGError.Error() tests for edge cases ---

func TestSearXNGErrorEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("error with content type", func(t *testing.T) {
		t.Parallel()

		err := NewSearXNGError(200, "application/json", "ok", errPlainTestError)
		msg := err.Error()

		if !strings.Contains(msg, "content-type application/json") {
			t.Fatalf("Error() = %q, want to contain content-type", msg)
		}
	})

	t.Run("error without content type", func(t *testing.T) {
		t.Parallel()

		err := NewSearXNGError(500, "", "", errNetworkTestError)
		msg := err.Error()

		if strings.Contains(msg, "content-type") {
			t.Fatalf("Error() = %q, should not contain content-type when empty", msg)
		}
	})

	t.Run("error without underlying", func(t *testing.T) {
		t.Parallel()

		err := NewSearXNGError(200, "text/html", "", nil)
		msg := err.Error()

		if !strings.Contains(msg, "content-type: text/html") {
			t.Fatalf("Error() = %q, want to contain content-type: text/html", msg)
		}
	})
}

// --- logDebugBody tests ---

func TestLogDebugBody(t *testing.T) {
	t.Parallel()

	t.Run("debug=false does nothing", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		// Should not panic
		s.logDebugBody(resp, []byte(`{"key": "value"}`))
	})

	t.Run("debug=true with short body", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		// Should not panic
		s.logDebugBody(resp, []byte(`{"key": "value"}`))
	})

	t.Run("debug=true with long body truncated", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}
		longBody := []byte(strings.Repeat("x", DebugBodyPreviewChars+100))
		// Should not panic
		s.logDebugBody(resp, longBody)
	})
}

// --- parseSearchResponse edge cases ---

func TestParseSearchEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("HTML content type", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html><body>not json</body></html>")),
		}

		_, err := s.parseSearchResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("parseSearchResponse() error = nil, want HTMLResponseError")
		}

		var htmlErr *HTMLResponseError
		if !errors.As(err, &htmlErr) {
			t.Fatalf("error type = %T, want *HTMLResponseError", err)
		}
	})

	t.Run("read error on body", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(&errorReader{}),
		}

		_, err := s.parseSearchResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("parseSearchResponse() error = nil, want error from body read failure")
		}
	})
}
