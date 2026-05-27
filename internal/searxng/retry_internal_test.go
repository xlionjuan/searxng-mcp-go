package searxng

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

var (
	errRetryTestConnectionReset        = errors.New("connection reset")
	errRetryTestRequestCreationFailure = errors.New("request creation failed")
)

func TestRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if !isRetryableError(ctx, errRetryTestConnectionReset) {
		t.Fatal("plain network error should be retryable")
	}

	if isRetryableError(ctx, NewSearXNGError(0, "", "", errRetryTestRequestCreationFailure)) {
		t.Fatal("SearXNGError should not be retryable")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if isRetryableError(canceledCtx, errRetryTestConnectionReset) {
		t.Fatal("errors should not be retryable after context cancellation")
	}
}

func TestShouldRetryHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	strategy := newExponentialBackoffStrategy(1, time.Millisecond, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	shouldRetry, delay := strategy.ShouldRetry(ctx, 0, nil, errRetryTestConnectionReset)
	if shouldRetry {
		t.Fatalf("ShouldRetry() shouldRetry = true, want false")
	}

	if delay != 0 {
		t.Fatalf("ShouldRetry() delay = %v, want 0", delay)
	}
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
		ctx := context.Background()

		// attempt=2 means we've already used up both retries (attempt 0 and 1)
		shouldRetry, _ := strategy.ShouldRetry(ctx, 2, nil, errRetryTestConnectionReset)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false when attempt >= maxRetries")
		}
	})

	t.Run("within max retries", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := context.Background()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, nil, errRetryTestConnectionReset)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true when within max retries")
		}
	})

	t.Run("exact boundary attempt == maxRetries", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := context.Background()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 2, nil, errRetryTestConnectionReset)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false when attempt == maxRetries (attempt is 0-indexed)")
		}
	})
}

func TestShouldRetry_ErrorTypes(t *testing.T) {
	t.Parallel()

	t.Run("nil error and nil response does not retry", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := context.Background()

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, nil, nil)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for nil error and nil response")
		}
	})

	t.Run("non-retryable SearXNGError does not retry", func(t *testing.T) {
		t.Parallel()

		strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
		ctx := context.Background()

		err := NewSearXNGError(http.StatusBadRequest, "text/plain", "bad request", errRetryTestRequestCreationFailure)

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, nil, err)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for SearXNGError")
		}
	})
}

func TestShouldRetry_StatusCodes(t *testing.T) {
	t.Parallel()

	strategy := newExponentialBackoffStrategy(2, time.Millisecond, time.Millisecond)
	ctx := context.Background()

	t.Run("retryable status code 429", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusTooManyRequests}

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, resp, nil)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true for 429")
		}
	})

	t.Run("retryable status code 503", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusServiceUnavailable}

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, resp, nil)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true for 503")
		}
	})

	t.Run("non-retryable status code 404", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusNotFound}

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, resp, nil)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for 404")
		}
	})

	t.Run("error takes precedence over nil response", func(t *testing.T) {
		t.Parallel()

		// When both err and resp are provided, error should be checked first
		resp := &http.Response{StatusCode: http.StatusOK}

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, resp, errRetryTestConnectionReset)
		if !shouldRetry {
			t.Fatal("ShouldRetry() = false, want true when retryable error (error takes precedence)")
		}
	})

	t.Run("non-retryable error with retryable status code", func(t *testing.T) {
		t.Parallel()

		// SearXNGError is not retryable even with retryable status
		err := NewSearXNGError(http.StatusBadGateway, "text/plain", "bad gateway", errRetryTestConnectionReset)
		resp := &http.Response{StatusCode: http.StatusBadGateway}

		shouldRetry, _ := strategy.ShouldRetry(ctx, 0, resp, err)
		if shouldRetry {
			t.Fatal("ShouldRetry() = true, want false for SearXNGError even with retryable status code")
		}
	})
}

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
}

func TestRetryWait(t *testing.T) {
	t.Parallel()

	t.Run("waits for delay", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		err := retryWait(context.Background(), 5*time.Millisecond)
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

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := retryWait(ctx, time.Second)
		if err == nil {
			t.Fatal("retryWait() error = nil, want context.Canceled")
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
