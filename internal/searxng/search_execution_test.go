package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
)

// ---------------------------------------------------------------------------
// executeGETfallback tests
// ---------------------------------------------------------------------------

//nolint:gocognit,gocyclo,cyclop // table-driven test covering GET fallback and error scenarios
func TestExecuteGETfallback(t *testing.T) {
	t.Parallel()

	t.Run("successful GET fallback after 405", func(t *testing.T) {
		t.Parallel()

		var gotMethod, gotURL string

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
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
			t.Context(),
			http.MethodPost,
			"https://search.example.com/search",
			http.NoBody,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(t.Context(), origResp, postReq, "q=test")
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
				Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
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
			t.Context(),
			http.MethodPost,
			"https://search.example.com/search",
			http.NoBody,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(t.Context(), origResp, postReq, "q=test&format=json")
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
				Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
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
			t.Context(),
			http.MethodPost,
			"https://search.example.com/search",
			http.NoBody,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusMethodNotAllowed,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(t.Context(), origResp, postReq, "q=test")
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

	t.Run("GET fallback transport error preserves errSearchMethodRejected in chain", func(t *testing.T) {
		// Regression test for #244: when POST returns 405/501 and the GET
		// fallback transport call also fails, the final error must contain
		// both errGETFallbackUsed and errSearchMethodRejected so callers can
		// detect the actionable hint ("set SEARXNG_ALLOW_GET_FALLBACK=1").
		t.Parallel()

		tests := []struct {
			name       string
			postStatus int
		}{
			{name: "POST 405", postStatus: http.StatusMethodNotAllowed},
			{name: "POST 501", postStatus: http.StatusNotImplemented},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				s := &SearXNGSearcher{
					client: &http.Client{
						Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
							return nil, errRetryTestConnectionReset
						}),
					},
					debug: false,
				}

				postReq, err := http.NewRequestWithContext(
					t.Context(),
					http.MethodPost,
					"https://search.example.com/search",
					http.NoBody,
				)
				if err != nil {
					t.Fatalf("failed to create post request: %v", err)
				}

				origResp := &http.Response{
					StatusCode: tt.postStatus,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}

				_, getFallbackErr := s.executeGETfallback(t.Context(), origResp, postReq, "q=test")
				if getFallbackErr == nil {
					t.Fatal("executeGETfallback() error = nil, want error")
				}

				if !errors.Is(getFallbackErr, errGETFallbackUsed) {
					t.Fatalf("errors.Is(err, errGETFallbackUsed) = false, want true; err = %v", getFallbackErr)
				}

				if !errors.Is(getFallbackErr, errSearchMethodRejected) {
					t.Fatalf("errors.Is(err, errSearchMethodRejected) = false, want true; err = %v", getFallbackErr)
				}
			})
		}
	})

	t.Run("GET fallback does not modify URL path", func(t *testing.T) {
		t.Parallel()

		var capturedURL string

		s := &SearXNGSearcher{
			client: &http.Client{
				Transport: testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
					capturedURL = req.URL.String()

					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			debug: false,
		}

		postReq, err := http.NewRequestWithContext(
			t.Context(),
			http.MethodPost,
			"https://search.example.com/custom/search",
			http.NoBody,
		)
		if err != nil {
			t.Fatalf("failed to create post request: %v", err)
		}

		origResp := &http.Response{
			StatusCode: http.StatusNotImplemented,
			Body:       io.NopCloser(strings.NewReader("")),
		}

		resp, err := s.executeGETfallback(t.Context(), origResp, postReq, "q=test")
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

//nolint:gocognit,gocyclo,cyclop,maintidx // comprehensive test covering retry, fallback, and error scenarios
func TestDoSearchAttempt(t *testing.T) {
	t.Parallel()

	t.Run("successful POST request", func(t *testing.T) {
		t.Parallel()

		var gotMethod string

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			gotMethod = req.Method

			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		resp, bodyStr, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "test"})
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

	t.Run("POST returns 405 does not trigger GET fallback by default", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return &http.Response{
				StatusCode: http.StatusMethodNotAllowed,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("")),
			}, nil
		}), 0)

		resp, bodyStr, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("doSearchAttempt() error = %v, want nil", err)
		}

		if resp == nil {
			t.Fatal("doSearchAttempt() resp = nil, want non-nil")

			return
		}

		defer closeBody(resp)

		if callCount != 1 {
			t.Fatalf("RoundTrip callCount = %d, want 1 (POST only)", callCount)
		}

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("StatusCode = %d, want 405", resp.StatusCode)
		}

		if bodyStr == "" {
			t.Fatal("bodyStr = empty, want non-empty query string")
		}
	})

	t.Run("POST method rejection triggers GET fallback when enabled", func(t *testing.T) {
		t.Parallel()

		tests := []struct {
			name   string
			status int
		}{
			{name: "405", status: http.StatusMethodNotAllowed},
			{name: "501", status: http.StatusNotImplemented},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()

				callCount := 0
				s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					callCount++

					if callCount == 1 {
						return &http.Response{
							StatusCode: tt.status,
							Header:     http.Header{"Content-Type": []string{"text/html"}},
							Body:       io.NopCloser(strings.NewReader("")),
						}, nil
					}

					return makeJSONResponse(minimalJSONBody), nil
				}), 0)
				s.allowGETFallback = true

				resp, bodyStr, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "test"})
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
		}
	})

	t.Run("GET fallback transport error redacts query", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			callCount++

			if req.Method == http.MethodPost {
				return &http.Response{
					StatusCode: http.StatusMethodNotAllowed,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}

			return nil, errRetryTestConnectionReset
		}), 0)
		s.allowGETFallback = true

		resp, _, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "sensitive search"})
		closeBody(resp)

		if err == nil {
			t.Fatal("doSearchAttempt() error = nil, want GET fallback error")
		}

		errText := err.Error()
		if strings.Contains(errText, "sensitive search") || strings.Contains(errText, "q=sensitive") {
			t.Fatalf("error leaked query: %v", err)
		}

		if !strings.Contains(errText, "GET fallback was used") {
			t.Fatalf("error = %q, want GET fallback warning context", errText)
		}

		if callCount != 2 {
			t.Fatalf("RoundTrip callCount = %d, want 2 (POST + GET fallback)", callCount)
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
				Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return nil, errTransportNotExpected
				}),
			},
			debug: false,
		}

		resp, _, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "test"})
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
				Transport: testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
					return makeJSONResponse(minimalJSONBody), nil
				}),
			},
			searchEndpoint: endpoint,
			debug:          true,
			retryStrategy:  newExponentialBackoffStrategy(0, time.Microsecond, time.Microsecond),
		}

		resp, bodyStr, err := s.doSearchAttempt(t.Context(), &SearchArgs{Query: "test"})
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

//nolint:gocognit,gocyclo // subtests cover each response-finishing branch explicitly
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

	t.Run("nil response returns errNilFinishResponse without panic", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{}

		// Should not panic on nil response — defensive nil guard
		result, err := s.finishResponse(nil, &SearchArgs{})
		if err == nil {
			t.Fatal("finishResponse(nil) error = nil, want errNilFinishResponse")
		}

		if result != nil {
			t.Fatalf("finishResponse(nil) result = %v, want nil", result)
		}

		if !errors.Is(err, errNilFinishResponse) {
			t.Fatalf("finishResponse(nil) error = %v, want errors.Is(_, errNilFinishResponse)", err)
		}
	})
}
