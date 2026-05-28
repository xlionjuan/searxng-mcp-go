package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// handleNonOKResponse tests
// ---------------------------------------------------------------------------

func TestHandleNonOKResponse(t *testing.T) {
	t.Parallel()

	t.Run("400 Bad Request returns error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
			Body:       io.NopCloser(strings.NewReader("invalid query")),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if searxErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("404 Not Found returns error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html>not found</html>")),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if searxErr.StatusCode != http.StatusNotFound {
			t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("500 Internal Server Error with normal body", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("server error")),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if searxErr.StatusCode != http.StatusInternalServerError {
			t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusInternalServerError)
		}
	})

	t.Run("body exceeds MaxErrorBodySize returns truncated error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		largeBody := strings.Repeat("x", int(MaxErrorBodySize)+100)
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(largeBody)),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "error response body exceeded maximum size limit") {
			t.Fatalf("error = %q, want error body exceeded maximum size limit", err.Error())
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if len(searxErr.ResponseBody) > MaxErrorDisplayChars {
			t.Fatalf("ResponseBody length = %d, want <= %d", len(searxErr.ResponseBody), MaxErrorDisplayChars)
		}
	})

	t.Run("body read failure returns error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(&errorReader{}),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "failed to read error response body") {
			t.Fatalf("error = %q, want 'failed to read error response body'", err.Error())
		}
	})

	t.Run("debug=true does not panic", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("bad request")),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}
	})

	t.Run("debug=true with long body does not panic", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		longBody := strings.Repeat("x", DebugBodyPreviewChars+100)
		resp := &http.Response{
			StatusCode: http.StatusInternalServerError,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader(longBody)),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}
	})

	t.Run("debug=false does not log", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		resp := &http.Response{
			StatusCode: http.StatusForbidden,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("forbidden")),
		}

		err := s.handleNonOKResponse(resp)
		if err == nil {
			t.Fatal("handleNonOKResponse() error = nil, want error")
		}
	})
}
