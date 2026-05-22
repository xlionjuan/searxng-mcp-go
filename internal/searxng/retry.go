package searxng

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"time"
)

func isRetryableError(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}

	var searchErr *SearXNGError
	if errors.As(err, &searchErr) {
		return false
	}

	return true
}

func isRetryableStatusCode(statusCode int) bool {
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func retryBackoff(attempt int, base, max time.Duration) time.Duration {
	if base <= 0 {
		base = DefaultRetryDelay
	}

	if max <= 0 {
		max = DefaultMaxRetryDelay
	}

	if max < base {
		max = base
	}

	delay := base
	for range attempt {
		if delay > max/2 {
			delay = max
			break
		}

		delay *= 2
	}

	if delay > max {
		delay = max
	}

	half := delay / 2
	jitterRange := half + 1
	if jitterRange <= 0 {
		return delay
	}

	return half + time.Duration(rand.Int64N(int64(jitterRange)))
}

func retryBackoffWait(ctx context.Context, attempt int, base, max time.Duration) error {
	return retryWait(ctx, retryBackoff(attempt, base, max))
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
