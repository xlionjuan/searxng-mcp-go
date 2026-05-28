package searxng

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

// It handles retryable errors, retryable HTTP status codes, and caps the
// backoff to a maximum delay with jitter.

// jitterHalfDivisor is the divisor used to compute half the delay for jitter range.
const (
	jitterHalfDivisor      = 2
	retryBackoffMultiplier = 2
)

type exponentialBackoffStrategy struct {
	maxRetries    int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
}

func newExponentialBackoffStrategy(maxRetries int, retryDelay, maxRetryDelay time.Duration) *exponentialBackoffStrategy {
	return &exponentialBackoffStrategy{
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
		maxRetryDelay: maxRetryDelay,
	}
}

func (s *exponentialBackoffStrategy) ShouldRetry(ctx context.Context, attempt int, resp *http.Response, err error) (bool, time.Duration) {
	if ctx.Err() != nil {
		return false, 0
	}

	if attempt >= s.maxRetries {
		return false, 0
	}

	if err != nil {
		if isRetryableError(ctx, err) {
			return true, retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
		}

		return false, 0
	}

	if resp != nil && resp.StatusCode != http.StatusOK {
		if isRetryableStatusCode(resp.StatusCode) {
			return true, retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
		}

		return false, 0
	}

	return false, 0
}

func isRetryableError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}

	var searchErr *SearXNGError

	return !errors.As(err, &searchErr)
}

func isRetryableStatusCode(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode == http.StatusRequestTimeout || statusCode >= http.StatusInternalServerError
}

func retryBackoff(attempt int, base, maxDelay time.Duration) time.Duration {
	if base <= 0 {
		base = DefaultRetryDelay
	}

	if maxDelay <= 0 {
		maxDelay = DefaultMaxRetryDelay
	}

	if maxDelay < base {
		maxDelay = base
	}

	delay := base
	for range attempt {
		if delay > maxDelay/2 {
			delay = maxDelay

			break
		}

		delay = min(delay*retryBackoffMultiplier, maxDelay)
	}

	if delay > maxDelay {
		delay = maxDelay
	}

	// Ensure a minimum delay floor to avoid zero-duration backoffs
	// when configured with very small base delays (e.g., 0 or 1ns).
	if delay < time.Millisecond {
		delay = time.Millisecond
	}

	half := delay / jitterHalfDivisor

	jitterRange := half + 1
	if jitterRange <= 0 {
		return delay
	}

	//nolint:gosec // jitter doesn't need cryptographic randomness; math/rand/v2 is correct here
	return half + time.Duration(rand.Int64N(int64(jitterRange)))
}

func retryWait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
