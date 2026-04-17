package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// --- TEST-02: ValidationError tests ---

func TestValidationError(t *testing.T) {
	t.Run("Is matches same field and message", func(t *testing.T) {
		err1 := NewValidationError("query", "is required")
		err2 := NewValidationError("query", "is required")
		err3 := NewValidationError("query", "must be longer")
		err4 := NewValidationError("other_field", "is required")

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
		if err1.Is(errors.New("some error")) {
			t.Errorf("err1.Is(non-ValidationError) = true, want false")
		}
	})

	t.Run("IsValidationError detects ValidationError", func(t *testing.T) {
		err := NewValidationError("query", "is required")

		if !IsValidationError(err) {
			t.Errorf("IsValidationError(err) = false, want true")
		}
		if IsValidationError(errors.New("not a validation error")) {
			t.Errorf("IsValidationError(non-ValidationError) = true, want false")
		}
		if IsValidationError(nil) {
			t.Errorf("IsValidationError(nil) = true, want false")
		}
	})

	t.Run("ValidationError wraps with Unwrap", func(t *testing.T) {
		// Create a ValidationError wrapped in another error using fmt.Errorf
		validationErr := NewValidationError("test", "test message")
		// Use errors.Join to create a wrapped error (Go 1.20+)
		wrappedErr := fmt.Errorf("operation failed: %w", validationErr)

		// errors.Is should work through the wrapped error
		if !errors.Is(wrappedErr, validationErr) {
			t.Errorf("errors.Is(wrappedErr, validationErr) = false, want true")
		}

		// IsValidationError should detect it
		if !IsValidationError(wrappedErr) {
			t.Errorf("IsValidationError(wrappedErr) = false, want true")
		}
	})
}

func TestHTTPStatusError(t *testing.T) {
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
			err := HTTPStatusError(tc.statusCode, tc.contentType, tc.body)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.statusCode)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.errContains) {
				t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
			}

			var searxngErr *SearXNGError
			if !errors.As(err, &searxngErr) {
				t.Fatalf("expected *SearXNGError, got type %T", err)
			}

			if searxngErr.GetStatusCode() != tc.statusCode {
				t.Errorf("SearXNGError.StatusCode = %d, want %d", searxngErr.GetStatusCode(), tc.statusCode)
			}
			if searxngErr.GetContentType() != tc.contentType {
				t.Errorf("SearXNGError.ContentType = %q, want %q", searxngErr.GetContentType(), tc.contentType)
			}
		})
	}
}

func TestHTTPStatusError_HTMLBodyNotInErrorMessage(t *testing.T) {
	htmlBody := []byte("<!DOCTYPE html><html><body>Internal Server Error</body></html>")

	err := HTTPStatusError(500, "text/html", htmlBody)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errMsg := err.Error()
	if strings.Contains(errMsg, "<!DOCTYPE") || strings.Contains(errMsg, "<html>") || strings.Contains(errMsg, "Internal Server Error") {
		t.Errorf("error message should not contain HTML body content, got: %s", errMsg)
	}

	var searxngErr *SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.ResponseBody != "" && (strings.Contains(searxngErr.ResponseBody, "<!DOCTYPE") || strings.Contains(searxngErr.ResponseBody, "<html>")) {
		t.Logf("ResponseBody contains HTML for debugging (this is allowed in structured field): %s", searxngErr.ResponseBody)
	}
}

func TestSearXNGError_ResponseBodyField(t *testing.T) {
	err := NewSearXNGError(400, "text/html", "error details here", errors.New("bad request"))

	var searxngErr *SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.ResponseBody == "" {
		t.Error("ResponseBody should not be empty when body is provided")
	}
	if searxngErr.ResponseBody == "error details here" {
		t.Log("ResponseBody is exact match - truncation works correctly")
	}
}

func TestSearXNGError_GetStatusCode(t *testing.T) {
	err := NewSearXNGError(403, "application/json", "", errors.New("forbidden"))

	var searxngErr *SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.GetStatusCode() != 403 {
		t.Errorf("GetStatusCode() = %d, want 403", searxngErr.GetStatusCode())
	}
}

func TestSearXNGError_GetContentType(t *testing.T) {
	err := NewSearXNGError(500, "text/plain", "", errors.New("server error"))

	var searxngErr *SearXNGError
	if !errors.As(err, &searxngErr) {
		t.Fatalf("expected *SearXNGError, got type %T", err)
	}

	if searxngErr.GetContentType() != "text/plain" {
		t.Errorf("GetContentType() = %q, want %q", searxngErr.GetContentType(), "text/plain")
	}
}

func TestSearXNGError_Unwrap(t *testing.T) {
	underlying := errors.New("connection refused")
	err := NewSearXNGError(0, "", "", underlying)

	unwrapped := errors.Unwrap(err)
	if unwrapped != underlying {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, underlying)
	}

	if !errors.Is(err, underlying) {
		t.Errorf("errors.Is(err, underlying) = false, want true")
	}
}

func TestHTMLResponseError_HTMLBodyNotInMessage(t *testing.T) {
	htmlBody := "<!DOCTYPE html><html><head><title>Error</title></head><body>JSON not enabled</body></html>"
	err := &HTMLResponseError{Body: htmlBody, UnderlyingErr: nil}

	errMsg := err.Error()
	if strings.Contains(errMsg, "<!DOCTYPE") || strings.Contains(errMsg, "<html>") || strings.Contains(errMsg, "JSON not enabled") {
		t.Errorf("HTMLResponseError.Error() should not contain HTML body, got: %s", errMsg)
	}
}
