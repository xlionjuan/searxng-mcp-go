package searxng

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
)

// ---------------------------------------------------------------------------
// Search function tests (full flow via mock HTTP client)
// ---------------------------------------------------------------------------

func TestSearch_ValidationError(t *testing.T) {
	t.Parallel()

	t.Run("nil args returns validation error", func(t *testing.T) {
		t.Parallel()

		// Transport should never be called
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errTransportNotExpected
		}), 0)

		_, err := s.Search(t.Context(), nil)
		if err == nil {
			t.Fatal("Search() error = nil, want validation error")
		}
	})

	t.Run("empty query returns validation error", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, errTransportNotExpected
		}), 0)

		_, err := s.Search(t.Context(), &SearchArgs{Query: ""})
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

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(minimalJSONBody), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("search with args passes through all fields", func(t *testing.T) {
		t.Parallel()

		limit := 5
		pageno := 1

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(makeSearchResponseJSON(3)), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{
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

			return
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(makeSearchResponseJSON(5)), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{
			Query: "test results",
			Limit: &limit,
		})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return nil, errRetryTestConnectionReset
			}

			return makeJSONResponse(makeSearchResponseJSON(1)), nil
		}), 2)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (initial + retry)", callCount)
		}
	})

	t.Run("all retries exhausted returns error", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		maxRetries := 2
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return nil, errRetryTestConnectionReset
		}), maxRetries)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return nil, errRetryTestConnectionReset
		}), 5)

		ctx, cancel := context.WithCancel(t.Context())
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
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

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (initial + retry)", callCount)
		}
	})

	t.Run("non-retryable status code 404 does not retry", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		}), 2)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				// Return empty results — will retry
				return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
			}

			// Return with results
			return makeJSONResponse(makeSearchResponseJSON(1)), nil
		}), 2)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
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

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
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

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("invalid params")),
			}, nil
		}), 2)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html"}},
				Body:       io.NopCloser(strings.NewReader("<html><body>JSON not enabled</body></html>")),
			}, nil
		}), 0)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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
		s.done = make(chan struct{})

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
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
		s.allowGETFallback = true

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
		}

		if callCount != 2 {
			t.Fatalf("callCount = %d, want 2 (POST 405 + GET fallback)", callCount)
		}
	})
}

func TestSearch_GETfallbackErrorPreservesStatusCode(t *testing.T) {
	t.Parallel()

	t.Run("GET fallback error preserves original 405 status code in final SearXNGError", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method == http.MethodPost {
				return &http.Response{
					StatusCode: http.StatusMethodNotAllowed,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader("")),
				}, nil
			}

			// GET fallback fails with a transport error
			return nil, errRetryTestConnectionReset
		}), 0)
		s.allowGETFallback = true

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error")
		}

		// The error must contain errSearchRequestFailed (from Search's final wrapping)
		if !strings.Contains(err.Error(), "failed to execute search request") {
			t.Fatalf("error = %q, want 'failed to execute search request'", err.Error())
		}

		// The SearXNGError inside the error chain must preserve the original 405 status code
		// (not 0 from an unnecessary outer NewSearXNGError wrapper).
		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError accessible via errors.As", err)
		}

		if searxErr.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("StatusCode = %d, want %d (original 405 preserved, not 0)",
				searxErr.StatusCode, http.StatusMethodNotAllowed)
		}

		// The error must contain the original method-rejected hint
		if !strings.Contains(err.Error(), "search method rejected") {
			t.Fatalf("error = %q, want 'search method rejected' hint", err.Error())
		}
	})
}

func TestSearch_RetryWithEmptyResponseFallback(t *testing.T) {
	t.Parallel()

	t.Run("retry on error then empty response retry also fires", func(t *testing.T) {
		t.Parallel()

		callCount := 0
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			if callCount == 1 {
				return nil, errRetryTestConnectionReset
			}

			// Second and subsequent attempts return empty
			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 2)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(largeBody)),
			}, nil
		}), 0)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Header:     http.Header{"Content-Type": []string{"text/plain"}},
				Body:       io.NopCloser(strings.NewReader("bad request")),
			}, nil
		}), 2)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
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
		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			callCount++

			return makeJSONResponse(`{"query":"test","results":[],"suggestions":[],"answers":[],"infoboxes":[]}`), nil
		}), 0)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil", err)
		}

		if callCount != 1 {
			t.Fatalf("callCount = %d, want 1", callCount)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")

			return
		}
	})
}
