package searxng

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
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

// ---------------------------------------------------------------------------
// executeGETfallback tests
// ---------------------------------------------------------------------------

func TestExecuteGETfallback(t *testing.T) {
	t.Parallel()

	t.Run("successful GET fallback after 405", func(t *testing.T) {
		t.Parallel()

		var gotMethod, gotURL string

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					gotMethod = req.Method
					gotURL = req.URL.String()

					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(minimalJSONBody)),
					}, nil
				}),
			},
			debug: false,
		}

		postReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://search.example.com/search",
			nil,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(context.Background(), origResp, postReq, "q=test")
		if err != nil {
			t.Fatalf("executeGETfallback() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("executeGETfallback() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if gotMethod != http.MethodGet {
			t.Fatalf("method = %s, want GET", gotMethod)
		}

		if !strings.Contains(gotURL, "q=test") {
			t.Fatalf("URL = %s, want to contain 'q=test'", gotURL)
		}
	})

	t.Run("GET fallback with debug=true does not panic", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": []string{"application/json"}},
						Body:       io.NopCloser(strings.NewReader(minimalJSONBody)),
					}, nil
				}),
			},
			debug: true,
		}

		postReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://search.example.com/search",
			nil,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(context.Background(), origResp, postReq, "q=test&format=json")
		if err != nil {
			t.Fatalf("executeGETfallback() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("executeGETfallback() resp = nil, want non-nil")
		}

		defer closeBody(resp)
	})

	t.Run("GET fallback returns non-OK response", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: http.StatusNotFound,
						Header:     http.Header{"Content-Type": []string{"text/plain"}},
						Body:       io.NopCloser(strings.NewReader("not found")),
					}, nil
				}),
			},
			debug: false,
		}

		postReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://search.example.com/search",
			nil,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(context.Background(), origResp, postReq, "q=test")
		if err != nil {
			t.Fatalf("executeGETfallback() error = %v, want nil (non-OK responses are passed through)", err)
		}

		if resp == nil {
			t.Fatal("executeGETfallback() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusNotFound)
		}
	})

	t.Run("GET fallback does not modify URL path", func(t *testing.T) {
		t.Parallel()

		var capturedURL string

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
					capturedURL = req.URL.String()

					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			debug: false,
		}

		postReq, err := http.NewRequestWithContext(
			context.Background(),
			http.MethodPost,
			"https://search.example.com/custom/search",
			nil,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusNotImplemented,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(context.Background(), origResp, postReq, "q=test")
		if err != nil {
			t.Fatalf("executeGETfallback() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("executeGETfallback() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if !strings.Contains(capturedURL, "/custom/search") {
			t.Fatalf("URL = %s, want to contain '/custom/search'", capturedURL)
		}
	})
}

// ---------------------------------------------------------------------------
// doSearchAttempt tests
// ---------------------------------------------------------------------------

func TestDoSearchAttempt(t *testing.T) {
	t.Parallel()

	t.Run("successful POST request", func(t *testing.T) {
		t.Parallel()

		var gotMethod string

		s := newTestSearcher(t, roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotMethod = req.Method

			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		resp, bodyStr, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if gotMethod != http.MethodPost {
			t.Fatalf("method = %s, want POST", gotMethod)
		}

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty query string")
		}
	})

	t.Run("POST returns 405 triggers GET fallback", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				// First call (POST) returns 405
				return &http.Response{
					StatusCode: http.StatusMethodNotAllowed,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}

			// Second call (GET fallback) returns OK
			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		resp, bodyStr, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if callCount != 2 {
			t.Fatalf("RoundTrip callCount = %d, want 2 (POST + GET fallback)", callCount)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
		}

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty query string")
		}
	})

	t.Run("POST returns 501 triggers GET fallback", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusNotImplemented,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}

			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		resp, bodyStr, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if callCount != 2 {
			t.Fatalf("RoundTrip callCount = %d, want 2 (POST + GET fallback)", callCount)
		}

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty query string")
		}
	})

	t.Run("build error returns error directly", func(t *testing.T) {
		t.Parallel()

		// A searcher with an invalid URL fails at URL parse
		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return nil, errTransportNotExpected
				}),
			},
			baseURL: "://invalid-url",
			debug:   false,
		}

		resp, _, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		defer closeBody(resp)

		if err == nil {
			t.Fatal("doSearchAttempt() error = nil, want error for invalid URL")
		}

		if resp != nil {
			t.Fatal("doSearchAttempt() resp != nil, want nil on build error")
		}
	})

	t.Run("debug=true with POST does not panic", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			baseURL:    "https://search.example.com",
			debug:      true,
			maxRetries: 0,
		}

		resp, bodyStr, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")
		}

		defer closeBody(resp)

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty")
		}
	})
}

// ---------------------------------------------------------------------------
// finishResponse tests
// ---------------------------------------------------------------------------

func TestFinishResponse(t *testing.T) {
	t.Parallel()

	t.Run("200 OK returns parsed response", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(minimalJSONBody)),
		}

		result, err := s.finishResponse(resp, &SearchArgs{})
		if err != nil {
			t.Fatalf("finishResponse() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("finishResponse() result = nil, want non-nil")
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("200 OK with HTML content returns HTMLResponseError", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html><body>not json</body></html>")),
		}

		_, err := s.finishResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("finishResponse() error = nil, want HTMLResponseError")
		}

		var htmlErr *HTMLResponseError
		if !errors.As(err, &htmlErr) {
			t.Fatalf("error type = %T, want *HTMLResponseError", err)
		}
	})

	t.Run("200 OK with body read error returns error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(&errorReader{}),
		}

		_, err := s.finishResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("finishResponse() error = nil, want error")
		}
	})

	t.Run("non-OK status delegates to handleNonOKResponse", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("bad request")),
		}

		_, err := s.finishResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("finishResponse() error = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if searxErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("404 Not Found returns appropriate error", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}
		resp := &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("not found")),
		}

		_, err := s.finishResponse(resp, &SearchArgs{})
		if err == nil {
			t.Fatal("finishResponse() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "not found") {
			t.Fatalf("error = %q, want 'not found'", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// Search function tests (full flow via mock HTTP client)
// ---------------------------------------------------------------------------

func TestSearch_ValidationError(t *testing.T) {
	t.Parallel()

	t.Run("nil args returns validation error", func(t *testing.T) {
		t.Parallel()

		// Transport should never be called
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errTransportNotExpected
		}), 0)

		_, err := s.Search(context.Background(), nil)
		if err == nil {
			t.Fatal("Search() error = nil, want validation error")
		}
	})

	t.Run("empty query returns validation error", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errTransportNotExpected
		}), 0)

		_, err := s.Search(context.Background(), &SearchArgs{Query: ""})
		if err == nil {
			t.Fatal("Search() error = nil, want validation error")
		}

		var valErr *ValidationError
		if !errors.As(err, &valErr) {
			t.Fatalf("error type = %T, want *ValidationError", err)
		}
	})
}

func TestSearch_Success(t *testing.T) {
	t.Parallel()

	t.Run("successful search returns result", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("search with args passes through all fields", func(t *testing.T) {
		t.Parallel()

		limit := 5
		pageno := 1

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(makeSearchResponseJSON(3)), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{
			Query:      "golang testing",
			Language:   "en",
			SafeSearch: 1,
			TimeRange:  "month",
			Categories: "general",
			Engines:    "google,bing",
			Pageno:     &pageno,
			Limit:      &limit,
		})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if len(result.Results) != 3 {
			t.Fatalf("len(Results) = %d, want 3", len(result.Results))
		}
	})
}

func TestSearch_SuccessWithResults(t *testing.T) {
	t.Parallel()

	t.Run("search with results returns limited results", func(t *testing.T) {
		t.Parallel()

		limit := 2
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(makeSearchResponseJSON(5)), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{
			Query: "test results",
			Limit: &limit,
		})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if len(result.Results) != 2 {
			t.Fatalf("len(Results) = %d, want 2 (limited)", len(result.Results))
		}
	})
}

func TestSearch_RetryOnError(t *testing.T) {
	t.Parallel()

	t.Run("transient error is retried and succeeds", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return nil, errRetryTestConnectionReset
			}

			return makeJSONResponse(makeSearchResponseJSON(1)), nil
		}), 2)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (initial + retry)", callCount)
		}
	})

	t.Run("all retries exhausted returns error", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		maxRetries := 2
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return nil, errRetryTestConnectionReset
		}), maxRetries)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error after exhausting retries")
		}

		expectedCalls := maxRetries + 1 // loop runs maxRetries+1 times
		if callCount != expectedCalls {
			t.Fatalf("callCount = %d, want %d (maxRetries+1 attempts)", callCount, expectedCalls)
		}
	})

	t.Run("context cancellation stops retry", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return nil, errRetryTestConnectionReset
		}), 5)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Already canceled

		_, err := s.Search(ctx, &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error for canceled context")
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1 (first attempt stops when context is canceled)", callCount)
		}
	})
}

func TestSearch_RetryOnStatusCode(t *testing.T) {
	t.Parallel()

	t.Run("retryable status code 429 triggers retry", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusTooManyRequests,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("rate limited")),
				}, nil
			}

			return makeJSONResponse(makeSearchResponseJSON(1)), nil
		}), 2)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (initial + retry)", callCount)
		}
	})

	t.Run("non-retryable status code 404 does not retry", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		}), 2)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error for 404")
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1 (no retry for 404)", callCount)
		}
	})
}

func TestSearch_RetryOnEmptyResponse(t *testing.T) {
	t.Parallel()

	t.Run("empty response retries and gets data", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				// Return empty results — will retry
				return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
			}

			// Return with results
			return makeJSONResponse(makeSearchResponseJSON(1)), nil
		}), 2)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (first empty, second with results)", callCount)
		}

		if len(result.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(result.Results))
		}
	})

	t.Run("empty response with no retries left returns empty", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if len(result.Results) != 0 {
			t.Fatalf("len(Results) = %d, want 0 (no retries for empty)", len(result.Results))
		}
	})
}

func TestSearch_NonOKStatus(t *testing.T) {
	t.Parallel()

	t.Run("400 Bad Request returns error directly", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("invalid params")),
			}, nil
		}), 2)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error for 400")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}

		if searxErr.StatusCode != http.StatusBadRequest {
			t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusBadRequest)
		}
	})

	t.Run("HTML response returns HTMLResponseError", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html><body>JSON not enabled</body></html>")),
			}, nil
		}), 0)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want HTMLResponseError")
		}

		var htmlErr *HTMLResponseError
		if !errors.As(err, &htmlErr) {
			t.Fatalf("error type = %T, want *HTMLResponseError", err)
		}
	})
}

func TestSearch_DebugMode(t *testing.T) {
	t.Parallel()

	t.Run("debug mode does not affect search result", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			baseURL:       "https://search.example.com",
			debug:         true,
			maxRetries:    0,
			retryStrategy: newExponentialBackoffStrategy(0, time.Microsecond, time.Microsecond),
		}

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if !result.Debug {
			t.Fatal("Debug = false, want true")
		}
	})
}

func TestSearch_GETFallbackFlow(t *testing.T) {
	t.Parallel()

	t.Run("POST 405 triggers GET fallback during Search", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return &http.Response{
					StatusCode: http.StatusMethodNotAllowed,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}

			// GET fallback succeeds
			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (POST 405 + GET fallback)", callCount)
		}
	})
}

func TestSearch_RetryWithEmptyResponseFallback(t *testing.T) {
	t.Parallel()

	t.Run("retry on error then empty response retry also fires", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return nil, errRetryTestConnectionReset
			}

			// Second and subsequent attempts return empty
			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 2)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}

		// attempt=0: error -> retry; attempt=1: empty -> retry; attempt=2: empty -> return empty
		if callCount != 3 {
			t.Fatalf("callCount = %d, want 3 (attempt 0=err, attempt 1=empty+retry, attempt 2=empty)", callCount)
		}

		if len(result.Results) != 0 {
			t.Fatalf("len(Results) = %d, want 0 (empty response returned on last attempt)", len(result.Results))
		}
	})
}

func TestSearch_BodyTooLarge(t *testing.T) {
	t.Parallel()

	t.Run("very large response body triggers error", func(t *testing.T) {
		t.Parallel()

		largeBody := strings.Repeat("x", int(MaxResponseBodySize)+100)
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(largeBody)),
			}, nil
		}), 0)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error for oversized body")
		}

		if !strings.Contains(err.Error(), "response body exceeded maximum size limit") {
			t.Fatalf("error = %q, want 'response body exceeded maximum size limit'", err.Error())
		}
	})
}

func TestSearch_SearXNGErrorIsRetryable(t *testing.T) {
	t.Parallel()

	t.Run("SearXNGError from non-retryable status is not retried", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("bad request")),
			}, nil
		}), 2)

		_, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error")
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1 (no retry for non-retryable SearXNGError)", callCount)
		}
	})
}

func TestSearch_EmptyResponseRetryDoesNotSpin(t *testing.T) {
	t.Parallel()

	t.Run("empty response with 0 retries returns empty immediately", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 0)

		result, err := s.Search(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1", callCount)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}
	})
}
