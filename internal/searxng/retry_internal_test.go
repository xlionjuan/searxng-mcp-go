package searxng

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryableStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{name: "too many requests", statusCode: http.StatusTooManyRequests, want: true},
		{name: "internal server error", statusCode: http.StatusInternalServerError, want: true},
		{name: "bad gateway", statusCode: http.StatusBadGateway, want: true},
		{name: "bad request", statusCode: http.StatusBadRequest, want: false},
		{name: "forbidden", statusCode: http.StatusForbidden, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isRetryableStatusCode(tt.statusCode); got != tt.want {
				t.Fatalf("isRetryableStatusCode(%d) = %v, want %v", tt.statusCode, got, tt.want)
			}
		})
	}
}

func TestRetryableError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	if !isRetryableError(ctx, errors.New("connection reset")) {
		t.Fatal("plain network error should be retryable")
	}

	if isRetryableError(ctx, NewSearXNGError(0, "", "", errors.New("request creation failed"))) {
		t.Fatal("SearXNGError should not be retryable")
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if isRetryableError(canceledCtx, errors.New("connection reset")) {
		t.Fatal("errors should not be retryable after context cancellation")
	}
}

func TestRetryBackoffBounds(t *testing.T) {
	t.Parallel()

	base := 10 * time.Millisecond
	maxDelay := 15 * time.Millisecond

	for attempt := 0; attempt < 10; attempt++ {
		delay := retryBackoff(attempt, base, maxDelay)
		if delay < base/2 {
			t.Fatalf("retryBackoff(%d) = %v, want at least %v", attempt, delay, base/2)
		}

		if delay > maxDelay {
			t.Fatalf("retryBackoff(%d) = %v, want at most %v", attempt, delay, maxDelay)
		}
	}
}
