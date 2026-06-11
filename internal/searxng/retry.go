package searxng

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

// Outcome classifies a single search attempt result for retry decision-making.
type Outcome int

const (
	// OutcomeSuccess indicates a successful response, including empty results on the final attempt.
	OutcomeSuccess Outcome = iota
	// OutcomeRetry indicates a transient error or retryable HTTP status.
	OutcomeRetry
	// OutcomeEmptyRetry indicates a successful HTTP call but empty response — retryable.
	OutcomeEmptyRetry
	// OutcomeAbort indicates a non-retryable outcome: SearXNGError, non-retryable HTTP status, or canceled context.
	OutcomeAbort
)

// jitterHalfDivisor is the divisor used to compute half the delay for jitter range.
const (
	jitterHalfDivisor      = 2
	retryBackoffMultiplier = 2
)

// exponentialBackoffStrategy handles retryable errors, retryable HTTP status codes,
// and caps the backoff to a maximum delay with jitter.
type exponentialBackoffStrategy struct {
	maxRetries    int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
}

func (s *exponentialBackoffStrategy) MaxRetries() int {
	return s.maxRetries
}

func newExponentialBackoffStrategy(
	maxRetries int, retryDelay, maxRetryDelay time.Duration,
) *exponentialBackoffStrategy {
	return &exponentialBackoffStrategy{
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
		maxRetryDelay: maxRetryDelay,
	}
}

// ShouldRetry determines whether to retry based on the outcome of a search attempt.
func (s *exponentialBackoffStrategy) ShouldRetry(
	ctx context.Context, attempt int, outcome Outcome,
) (bool, time.Duration) {
	if ctx.Err() != nil {
		return false, 0
	}

	if attempt >= s.maxRetries {
		return false, 0
	}

	if outcome == OutcomeRetry || outcome == OutcomeEmptyRetry {
		return true, retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
	}

	return false, 0
}

// classifyOutcome determines the Outcome of a single search attempt.
func classifyOutcome(
	ctx context.Context,
	attempt, maxRetries int,
	resp *http.Response,
	err error,
	isEmpty bool,
) Outcome {
	if ctx.Err() != nil {
		return OutcomeAbort
	}

	if err != nil {
		var se *SearXNGError
		if errors.As(err, &se) {
			return OutcomeAbort
		}

		return OutcomeRetry
	}

	if resp != nil && resp.StatusCode != http.StatusOK {
		if isRetryableStatusCode(resp.StatusCode) {
			return OutcomeRetry
		}

		return OutcomeAbort
	}

	if isEmpty && attempt < maxRetries {
		return OutcomeEmptyRetry
	}

	return OutcomeSuccess
}

func isRetryableStatusCode(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests ||
		statusCode == http.StatusRequestTimeout ||
		statusCode >= http.StatusInternalServerError
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
