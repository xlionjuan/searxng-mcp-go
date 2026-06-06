package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	searxng "searxng-mcp-go/internal/searxng"
)

var (
	errBadRequestTest        = errors.New("bad request")
	errConnectionRefusedTest = errors.New("connection refused")
)

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

	if searxngErr.ResponseBody != "" &&
		(strings.Contains(searxngErr.ResponseBody, "<!DOCTYPE") ||
			strings.Contains(searxngErr.ResponseBody, "<html>")) {
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
