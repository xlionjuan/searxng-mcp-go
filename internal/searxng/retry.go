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
	// OutcomeSuccess indicates a successful response containing search data.
	OutcomeSuccess Outcome = iota
	// OutcomeRetry indicates a transient error or retryable HTTP status.
	OutcomeRetry
	// OutcomeEmptyRetry indicates a successful HTTP call but empty response — retryable.
	OutcomeEmptyRetry
	// OutcomeAbort indicates a non-retryable outcome: SearXNGError, non-retryable HTTP status, or canceled context.
	OutcomeAbort
	// OutcomeEmptyExhausted indicates an empty response with no retries remaining.
	OutcomeEmptyExhausted
)

// jitterHalfDivisor is the divisor used to compute half the delay for jitter range.
const (
	jitterHalfDivisor      = 2
	retryBackoffMultiplier = 2
	retryMinWait           = time.Second
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

//nolint:gocyclo // classifyOutcome: many failure-mode branches; each is direct and well-understood
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

		var he *HTMLResponseError
		if errors.As(err, &he) {
			return OutcomeAbort
		}

		// Redirect policy failures are deterministic — retrying would submit
		// the same request to the same blocked redirect on every attempt.
		if errors.Is(err, errRedirectDifferentHost) ||
			errors.Is(err, errRedirectSchemeDowngrade) ||
			errors.Is(err, errRedirectMethodChanged) ||
			errors.Is(err, errTooManyRedirects) {
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

	if isEmpty {
		if attempt < maxRetries {
			return OutcomeEmptyRetry
		}

		return OutcomeEmptyExhausted
	}

	return OutcomeSuccess
}

func isRetryableStatusCode(statusCode int) bool {
	// 501 Not Implemented is never retryable: it's a deterministic
	// method-rejection response. When GET fallback is enabled,
	// doSearchAttempt intercepts 501 before the retry layer runs;
	// when disabled, retrying would send the same POST to the same
	// rejection. 405 is also deterministic but is < 500, so the
	// generic >=500 rule already excludes it.
	if statusCode == http.StatusNotImplemented {
		return false
	}

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

	half := delay / jitterHalfDivisor

	jitterRange := half + 1
	if jitterRange <= 0 {
		return delay
	}

	//nolint:gosec // jitter doesn't need cryptographic randomness; math/rand/v2 is correct here
	wait := half + time.Duration(rand.Int64N(int64(jitterRange)))

	// Ensure the final jittered wait never drops below the operationally
	// meaningful minimum. Sub-second retries provide no recovery window
	// for an upstream metasearch service that may fan out to multiple engines.
	wait = max(wait, retryMinWait)

	wait = min(wait, maxDelay)

	return wait
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
