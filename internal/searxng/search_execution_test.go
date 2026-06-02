package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

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

			return
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

			return
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

			return
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

			return
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

			return
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

			return
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

			return
		}

		defer closeBody(resp)

		if callCount != 2 {
			t.Fatalf("RoundTrip callCount = %d, want 2 (POST + GET fallback)", callCount)
		}

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty query string")
		}
	})

	t.Run("build error from missing precomputed endpoint propagates", func(t *testing.T) {
		t.Parallel()

		// A SearXNGSearcher constructed directly (bypassing NewSearXNGSearcher)
		// without setting searchEndpoint causes buildSearchRequest to return
		// errSearchEndpointNotPrecomputed; doSearchAttempt must propagate it
		// without invoking the transport.
		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return nil, errTransportNotExpected
				}),
			},
			debug: false,
		}

		resp, _, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		defer closeBody(resp)

		if err == nil {
			t.Fatal("doSearchAttempt() error = nil, want errSearchEndpointNotPrecomputed")
		}

		if !errors.Is(err, errSearchEndpointNotPrecomputed) {
			t.Fatalf("doSearchAttempt() error = %v, want errSearchEndpointNotPrecomputed", err)
		}

		if resp != nil {
			t.Fatal("doSearchAttempt() resp != nil, want nil on build error")
		}
	})

	t.Run("debug=true with POST does not panic", func(t *testing.T) {
		t.Parallel()

		endpoint, err := computeSearchEndpoint("https://search.example.com")
		if err != nil {
			t.Fatalf("computeSearchEndpoint() error = %v", err)
		}

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: roundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			searchEndpoint: endpoint,
			debug:          true,
			maxRetries:     0,
		}

		resp, bodyStr, err := s.doSearchAttempt(context.Background(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")

			return
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

			return
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
