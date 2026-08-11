package searxng

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
)

var (
	errRetryTestConnectionReset        = errors.New("connection reset")
	errRetryTestRequestCreationFailure = errors.New("request creation failed")
)

//nolint:gocognit,gocyclo // per-branch subtests; extracting them would add indirection without improving clarity
func TestClassifyOutcome(t *testing.T) {
	t.Parallel()

	t.Run("canceled context returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		outcome := classifyOutcome(ctx, 0, 2, nil, errRetryTestConnectionReset, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort", outcome)
		}
	})

	t.Run("SearXNGError returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := NewSearXNGError(0, "", "", errRetryTestRequestCreationFailure)
		outcome := classifyOutcome(ctx, 0, 2, nil, err, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort", outcome)
		}
	})

	t.Run("HTMLResponseError returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := &HTMLResponseError{Body: "<html><body>error</body></html>"}
		outcome := classifyOutcome(ctx, 0, 2, nil, err, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for HTMLResponseError", outcome)
		}
	})

	t.Run("plain error returns OutcomeRetry", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		outcome := classifyOutcome(ctx, 0, 2, nil, errRetryTestConnectionReset, false)

		if outcome != OutcomeRetry {
			t.Fatalf("classifyOutcome() = %v, want OutcomeRetry", outcome)
		}
	})

	t.Run("retryable status code returns OutcomeRetry", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusTooManyRequests}
		outcome := classifyOutcome(ctx, 0, 2, resp, nil, false)

		if outcome != OutcomeRetry {
			t.Fatalf("classifyOutcome() = %v, want OutcomeRetry for 429", outcome)
		}
	})

	t.Run("non-retryable status code returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusNotFound}
		outcome := classifyOutcome(ctx, 0, 2, resp, nil, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for 404", outcome)
		}
	})

	t.Run("501 Not Implemented returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusNotImplemented}
		outcome := classifyOutcome(ctx, 0, 2, resp, nil, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for 501", outcome)
		}
	})

	t.Run("empty with retries remaining returns OutcomeEmptyRetry", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusOK}
		outcome := classifyOutcome(ctx, 0, 2, resp, nil, true)

		if outcome != OutcomeEmptyRetry {
			t.Fatalf("classifyOutcome() = %v, want OutcomeEmptyRetry for empty response with retries", outcome)
		}
	})

	t.Run("empty on last attempt returns OutcomeEmptyExhausted", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusOK}
		// attempt == maxRetries means no retries left
		outcome := classifyOutcome(ctx, 2, 2, resp, nil, true)

		if outcome != OutcomeEmptyExhausted {
			t.Fatalf("classifyOutcome() = %v, want OutcomeEmptyExhausted for empty on last attempt", outcome)
		}
	})

	t.Run("successful response returns OutcomeSuccess", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{StatusCode: http.StatusOK}
		outcome := classifyOutcome(ctx, 0, 2, resp, nil, false)

		if outcome != OutcomeSuccess {
			t.Fatalf("classifyOutcome() = %v, want OutcomeSuccess", outcome)
		}
	})

	t.Run("errRedirectDifferentHost wrapped in url.Error returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := &url.Error{
			Op:  "Post",
			URL: "https://search.example.com/search",
			Err: fmt.Errorf("redirect to different host blocked: %w", errRedirectDifferentHost),
		}
		outcome := classifyOutcome(ctx, 0, 2, nil, err, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for errRedirectDifferentHost", outcome)
		}
	})

	t.Run("errRedirectSchemeDowngrade wrapped in url.Error returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := &url.Error{
			Op:  "Post",
			URL: "https://search.example.com/search",
			Err: fmt.Errorf("https to http downgrade blocked: %w", errRedirectSchemeDowngrade),
		}
		outcome := classifyOutcome(ctx, 0, 2, nil, err, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for errRedirectSchemeDowngrade", outcome)
		}
	})

	t.Run("errTooManyRedirects wrapped in url.Error returns OutcomeAbort", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		err := &url.Error{
			Op:  "Post",
			URL: "https://search.example.com/search",
			Err: errTooManyRedirects,
		}
		outcome := classifyOutcome(ctx, 0, 2, nil, err, false)

		if outcome != OutcomeAbort {
			t.Fatalf("classifyOutcome() = %v, want OutcomeAbort for errTooManyRedirects", outcome)
		}
	})
}

func TestNewExponentialBackoffStrategy(t *testing.T) {
	t.Parallel()

	s := newExponentialBackoffStrategy(3, time.Second, 30*time.Second)
	if s.maxRetries != 3 {
		t.Fatalf("maxRetries = %d, want 3", s.maxRetries)
	}

	if s.retryDelay != time.Second {
		t.Fatalf("retryDelay = %v, want 1s", s.retryDelay)
	}

	if s.maxRetryDelay != 30*time.Second {
		t.Fatalf("maxRetryDelay = %v, want 30s", s.maxRetryDelay)
	}
}

func TestShouldRetry_AttemptLimit(t *testing.T) {
	t.Parallel()

	t.Run("exceeds max retries", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		// attempt=2 means we've already used up both retries (attempt 0 and 1)
		shouldRetry, _ := strategy.ShouldRetry(ctx, 2, OutcomeRetry)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false when attempt >= maxRetries")
		}
	})

	t.Run("within max retries", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeRetry)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true when within max retries")
		}
	})

	t.Run("exact boundary attempt == maxRetries", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 2, OutcomeRetry)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false when attempt == maxRetries (attempt is 0-indexed)")
		}
	})
}

func TestShouldRetry_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("OutcomeSuccess does not retry", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeSuccess)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for OutcomeSuccess")
		}
	})

	t.Run("OutcomeAbort does not retry", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeAbort)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for OutcomeAbort")
		}
	})

	t.Run("OutcomeEmptyExhausted does not retry", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := t.Context()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeEmptyExhausted)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for OutcomeEmptyExhausted")
		}
	})
}

func TestShouldRetry_StatusCodes(t *testing.T) {
	t.Parallel()

	strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
	ctx := t.Context()

	t.Run("OutcomeRetry is retried", func(t *testing.T) {
		t.Parallel()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeRetry)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true for OutcomeRetry")
		}
	})

	t.Run("OutcomeEmptyRetry is retried", func(t *testing.T) {
		t.Parallel()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeEmptyRetry)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true for OutcomeEmptyRetry")
		}
	})

	t.Run("OutcomeAbort is not retried", func(t *testing.T) {
		t.Parallel()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeAbort)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for OutcomeAbort")
		}
	})

	t.Run("OutcomeSuccess is not retried", func(t *testing.T) {
		t.Parallel()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, OutcomeSuccess)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for OutcomeSuccess")
		}
	})
}

//nolint:gocognit // sequential subtests covering edge cases of retryBackoff
func TestRetryBackoff_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("zero base delay uses default", func(t *testing.T) {
		t.Parallel()

		delay := retryBackoff(0, 0, 0)
		if delay < DefaultRetryDelay/2 {
			t.Fatalf("retryBackoff(0, 0, 0) = %v, want at least %v", delay, DefaultRetryDelay/2)
		}
	})

	t.Run("zero max delay defaults to DefaultMaxRetryDelay", func(t *testing.T) {
		t.Parallel()

		delay := retryBackoff(0, DefaultRetryDelay, 0)
		if delay < DefaultRetryDelay/2 || delay > DefaultMaxRetryDelay {
			t.Fatalf("retryBackoff(0, %v, 0) = %v, want between %v and %v",
				DefaultRetryDelay, delay, DefaultRetryDelay/2, DefaultMaxRetryDelay)
		}
	})

	t.Run("max delay less than base clamped to base", func(t *testing.T) {
		t.Parallel()

		delay := retryBackoff(0, 100*time.Millisecond, 50*time.Millisecond)
		if delay < 50*time.Millisecond || delay > 100*time.Millisecond {
			t.Fatalf("retryBackoff(0, 100ms, 50ms) = %v, want between 50ms and 100ms", delay)
		}
	})

	t.Run("high attempt caps at max delay", func(t *testing.T) {
		t.Parallel()

		delay := retryBackoff(10, 1*time.Millisecond, 5*time.Millisecond)
		if delay > 5*time.Millisecond {
			t.Fatalf("retryBackoff(10, 1ms, 5ms) = %v, want <= 5ms", delay)
		}
	})

	t.Run("delay never exceeds max", func(t *testing.T) {
		t.Parallel()

		for attempt := range 20 {
			delay := retryBackoff(attempt, 100*time.Millisecond, time.Second)
			if delay > time.Second {
				t.Fatalf("retryBackoff(%d, 100ms, 1s) = %v, want <= 1s", attempt, delay)
			}
		}
	})

	t.Run("sub-millisecond base and max capped at maxDelay", func(t *testing.T) {
		t.Parallel()

		for attempt := range 5 {
			delay := retryBackoff(attempt, time.Nanosecond, time.Nanosecond)
			if delay > time.Nanosecond {
				t.Fatalf("retryBackoff(%d, 1ns, 1ns) = %v, want <= 1ns", attempt, delay)
			}
		}
	})

	t.Run("sub-millisecond base with millisecond max still respects cap", func(t *testing.T) {
		t.Parallel()

		for attempt := range 5 {
			delay := retryBackoff(attempt, time.Nanosecond, 100*time.Microsecond)
			if delay > 100*time.Microsecond {
				t.Fatalf("retryBackoff(%d, 1ns, 100µs) = %v, want <= 100µs", attempt, delay)
			}
		}
	})
}

func TestRetryWait(t *testing.T) {
	t.Parallel()

	t.Run("waits for delay", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		err := retryWait(t.Context(), 5*time.Millisecond)
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("retryWait() error = %v, want nil", err)
		}

		if duration < 5*time.Millisecond {
			t.Fatalf("retryWait() slept for %v, want at least %v", duration, 5*time.Millisecond)
		}
	})

	t.Run("canceled context returns error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		err := retryWait(ctx, time.Second)
		if err == nil {
			t.Fatal("retryWait() error = nil, want context.Canceled")
		}
	})
}

func TestSearchRedirectPolicyNotRetried(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rtErr    func(*http.Request) error
		wantErr  error
		wantCall int
	}{
		{
			name: "cross-host redirect error aborts without retry",
			rtErr: func(req *http.Request) error {
				return &url.Error{
					Op:  req.Method,
					URL: req.URL.String(),
					Err: fmt.Errorf("redirect to different host blocked: %w", errRedirectDifferentHost),
				}
			},
			wantErr:  errRedirectDifferentHost,
			wantCall: 1,
		},
		{
			name: "scheme downgrade error aborts without retry",
			rtErr: func(req *http.Request) error {
				return &url.Error{
					Op:  req.Method,
					URL: req.URL.String(),
					Err: fmt.Errorf("https to http downgrade blocked: %w", errRedirectSchemeDowngrade),
				}
			},
			wantErr:  errRedirectSchemeDowngrade,
			wantCall: 1,
		},
		{
			name: "too many redirects error aborts without retry",
			rtErr: func(req *http.Request) error {
				return &url.Error{
					Op:  req.Method,
					URL: req.URL.String(),
					Err: errTooManyRedirects,
				}
			},
			wantErr:  errTooManyRedirects,
			wantCall: 1,
		},
		{
			name: "method change redirect error aborts without retry",
			rtErr: func(req *http.Request) error {
				return &url.Error{
					Op:  req.Method,
					URL: req.URL.String(),
					Err: fmt.Errorf("POST -> GET: %w", errRedirectMethodChanged),
				}
			},
			wantErr:  errRedirectMethodChanged,
			wantCall: 1,
		},
		{
			name:     "transient transport error still retries",
			rtErr:    func(*http.Request) error { return errRetryTestConnectionReset },
			wantErr:  errRetryTestConnectionReset,
			wantCall: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			callCount := 0
			transport := testhelper.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
				callCount++

				return nil, tt.rtErr(req)
			})

			s := newTestSearcher(t, transport, 2)

			_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
			if err == nil {
				t.Fatal("Search() error = nil, want error")
			}

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want sentinel %v", err, tt.wantErr)
			}

			if callCount != tt.wantCall {
				t.Fatalf("RoundTrip callCount = %d, want %d", callCount, tt.wantCall)
			}
		})
	}
}

func TestClassifyAttempt_BodyReadRetry(t *testing.T) {
	t.Parallel()

	s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errTransportNotExpected
	}), 2)

	t.Run("body read failure returns OutcomeRetry", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &errorReader{},
		}

		ar, err := s.classifyAttempt(ctx, 0, 2, resp, nil, &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("classifyAttempt() error = %v, want nil", err)
		}

		if ar.outcome != OutcomeRetry {
			t.Fatalf("classifyAttempt() outcome = %v, want OutcomeRetry", ar.outcome)
		}
	})

	t.Run("HTML response error is still propagated as hard error", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html><body>error</body></html>")),
		}

		_, err := s.classifyAttempt(ctx, 0, 2, resp, nil, &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("classifyAttempt() error = nil, want error for HTML response")
		}

		var he *HTMLResponseError
		if !errors.As(err, &he) {
			t.Fatalf("classifyAttempt() error type = %T, want *HTMLResponseError", err)
		}
	})
}

func TestSearch_BodyReadRetry(t *testing.T) {
	t.Parallel()

	t.Run("body read failure followed by success", func(t *testing.T) {
		t.Parallel()

		var attempt int

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			attempt++
			switch attempt {
			case 1:
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       &errorReader{},
				}, nil
			default:
				return makeJSONResponse(makeSearchResponseJSON(1)), nil
			}
		}), 1)

		result, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err != nil {
			t.Fatalf("Search() error = %v, want nil after retry", err)
		}

		if result == nil {
			t.Fatal("Search() result = nil, want non-nil")
		}
	})

	t.Run("exhausted body read failures preserve underlying error", func(t *testing.T) {
		t.Parallel()

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       &errorReader{},
			}, nil
		}), 1)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error")
		}

		if !errors.Is(err, errReadFailed) {
			t.Fatalf("Search() error = %v, want unwrapping to %v", err, errReadFailed)
		}
	})

	t.Run("body read failure closes body exactly once per attempt", func(t *testing.T) {
		t.Parallel()

		var (
			mu              sync.Mutex
			totalCloseCalls int
		)

		s := newTestSearcher(t, testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
			body := &closeCounter{
				ReadCloser: &errorReader{},
				mu:         &mu,
				total:      &totalCloseCalls,
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       body,
			}, nil
		}), 1)

		_, err := s.Search(t.Context(), &SearchArgs{Query: "test"})
		if err == nil {
			t.Fatal("Search() error = nil, want error")
		}

		// 2 HTTP requests (initial + 1 retry), each body closed exactly once.
		if totalCloseCalls != 2 {
			t.Fatalf("total body Close() calls = %d, want 2 (one per HTTP request)", totalCloseCalls)
		}
	})
}

func TestIsRetryableStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code int
		want bool
	}{
		{http.StatusTooManyRequests, true},
		{http.StatusInternalServerError, true},
		{http.StatusBadGateway, true},
		{http.StatusServiceUnavailable, true},
		{http.StatusGatewayTimeout, true},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusForbidden, false},
		{http.StatusNotFound, false},
		{http.StatusMethodNotAllowed, false},
		{http.StatusNotImplemented, false},
		{http.StatusOK, false},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.code), func(t *testing.T) {
			t.Parallel()

			if got := isRetryableStatusCode(tt.code); got != tt.want {
				t.Fatalf("isRetryableStatusCode(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}
