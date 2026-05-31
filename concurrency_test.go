//go:build stress

package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"searxng-mcp-go/internal/searxng"
)

// --- Concurrent Search Stress Tests ---

// TestConcurrentSearches runs multiple searches simultaneously with different parameters.
func TestConcurrentSearches(t *testing.T) {
	t.Parallel()

	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		searchResp := searxng.SearchResponse{
			Results: []searxng.SearchResult{
				{Title: "Result", URL: "https://example.com/1", Content: "Content", Engine: "google"},
			},
			NumberOfResults: 1,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	searcher, err := searxng.NewSearXNGSearcher(cfg, false)
	if err != nil {
		t.Fatalf("NewSearXNGSearcher() error = %v", err)
	}
	defer func() { _ = searcher.Close() }()

	const (
		numGoroutines       = 20
		queriesPerGoroutine = 5
	)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			for range queriesPerGoroutine {
				query := "query"
				args := &searxng.SearchArgs{Query: query}

				ctx := context.Background()

				_, err := searcher.Search(ctx, args)
				if err != nil {
					t.Errorf("Search error for %s: %v", query, err)
				}
			}
		}(i)
	}

	wg.Wait()

	if requestCount != numGoroutines*queriesPerGoroutine {
		t.Errorf("Expected %d requests, got %d", numGoroutines*queriesPerGoroutine, requestCount)
	}
}

// TestConcurrentContextCancellation tests concurrent context cancellation.
func TestConcurrentContextCancellation(t *testing.T) {
	t.Parallel()

	requestCount := int64(0)
	canceledCount := int64(0)

	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt64(&requestCount, 1)
			<-r.Context().Done()
			atomic.AddInt64(&canceledCount, 1)

			return nil, r.Context().Err()
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}

	const numGoroutines = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var (
		successCount     int64
		errorCount       int64
		cancelledCount   int64
		unexpectedErrors []error
		mu               sync.Mutex
	)

	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})
			if err != nil {
				atomic.AddInt64(&errorCount, 1)

				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					atomic.AddInt64(&cancelledCount, 1)

					return
				}

				mu.Lock()

				unexpectedErrors = append(unexpectedErrors, err)
				mu.Unlock()

				return
			}

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	wg.Wait()

	count := atomic.LoadInt64(&requestCount)
	if count == 0 {
		t.Fatal("expected at least one request to start before cancellation")
	}

	if atomic.LoadInt64(&canceledCount) == 0 {
		t.Fatal("expected at least one request context to be canceled")
	}

	if successCount != 0 {
		t.Fatalf("successCount = %d, want 0", successCount)
	}

	if errorCount != numGoroutines {
		t.Fatalf("errorCount = %d, want %d", errorCount, numGoroutines)
	}

	if cancelledCount != numGoroutines {
		t.Fatalf("cancelledCount = %d, want %d", cancelledCount, numGoroutines)
	}

	if len(unexpectedErrors) > 0 {
		t.Fatalf("unexpected non-context errors: %v", unexpectedErrors)
	}
}

// TestChannelDeadlockDetection tests that channels don't deadlock.
func TestChannelDeadlockDetection(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	cfg := &searxng.Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	var wg sync.WaitGroup

	const numGoroutines = 50

	// Launch many concurrent searches
	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			ctx := context.Background()
			_, _ = testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})
		}(i)
	}

	// Wait with timeout to detect deadlock
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Normal completion
	case <-time.After(5 * time.Second):
		t.Fatal("Deadlock detected: goroutines did not complete in time")
	}
}

// TestRaceConditionOnSharedState tests for race conditions on shared HTTP client.
func TestRaceConditionOnSharedState(t *testing.T) {
	t.Parallel()

	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		searchResp := searxng.SearchResponse{
			Results: []searxng.SearchResult{
				{Title: "Result", URL: "https://example.com/1", Content: "Content", Engine: "google"},
			},
			NumberOfResults: 1,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	// Create a shared HTTP client
	sharedClient := &http.Client{Timeout: 30 * time.Second}

	// Create multiple searchers sharing the same HTTP client
	var searchers [5]*searxng.SearXNGSearcher

	for i := range 5 {
		searcher, err := searxng.NewSearXNGSearcher(
			&searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second, HTTPClient: sharedClient},
			false,
		)
		if err != nil {
			t.Fatalf("Failed to create searcher: %v", err)
		}

		searchers[i] = searcher
	}

	var wg sync.WaitGroup

	const numGoroutines = 50

	var errorCount int64

	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(id int) {
			defer wg.Done()

			searcher := searchers[id%len(searchers)]
			ctx := context.Background()

			_, err := searcher.Search(ctx, &searxng.SearchArgs{Query: "race_test"})
			if err != nil {
				atomic.AddInt64(&errorCount, 1)
				t.Errorf("Search error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if errorCount != 0 {
		t.Fatalf("errorCount = %d, want 0", errorCount)
	}

	if got := atomic.LoadInt64(&requestCount); got != numGoroutines {
		t.Fatalf("requestCount = %d, want %d", got, numGoroutines)
	}
}

// --- Graceful Shutdown and Signal Handling Tests ---

// TestGracefulShutdownWithContextCancel tests that search operations respect context cancellation.
// It uses a custom RoundTripper that blocks until context cancellation, simulating a slow SearXNG
// instance that gets interrupted by a graceful shutdown. When the shared context is canceled,
// all in-flight search calls must return context.Canceled.
func TestGracefulShutdownWithContextCancel(t *testing.T) {
	t.Parallel()

	const numGoroutines = 20

	requestCount := int64(0)
	sentCount := int64(0)
	successCount := int64(0)
	cancelledCount := int64(0)
	allRequestsEntered := make(chan struct{})

	var closeOnce sync.Once

	ctx, cancel := context.WithCancel(context.Background())

	// Use a custom RoundTripper that blocks until context cancellation,
	// simulating a slow SearXNG that gets interrupted. This ensures
	// the HTTP client returns context.Canceled (not an HTTP-level error).
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if atomic.AddInt64(&requestCount, 1) == numGoroutines {
				closeOnce.Do(func() {
					close(allRequestsEntered)
				})
			}

			<-r.Context().Done()

			return nil, r.Context().Err()
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}

	var wg sync.WaitGroup

	wg.Add(numGoroutines)

	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			atomic.AddInt64(&sentCount, 1)

			_, err := testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
					atomic.AddInt64(&cancelledCount, 1)

					return
				}

				t.Errorf("unexpected search error: %v", err)

				return
			}

			atomic.AddInt64(&successCount, 1)
		}(i)
	}

	<-allRequestsEntered
	cancel()

	// Wait with timeout
	done := make(chan struct{})

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Normal shutdown
	case <-time.After(5 * time.Second):
		t.Fatal("Graceful shutdown timed out")
	}

	reqCount := atomic.LoadInt64(&requestCount)
	sent := atomic.LoadInt64(&sentCount)
	success := atomic.LoadInt64(&successCount)
	canceled := atomic.LoadInt64(&cancelledCount)
	compCount := success + canceled

	if sent != numGoroutines {
		t.Fatalf("sentCount = %d, want %d", sent, numGoroutines)
	}

	if reqCount != numGoroutines {
		t.Fatalf("requestCount = %d, want %d", reqCount, numGoroutines)
	}

	if success != 0 {
		t.Fatalf("successCount = %d, want 0", success)
	}

	if canceled != numGoroutines {
		t.Fatalf("cancelledCount = %d, want %d", canceled, numGoroutines)
	}

	if compCount != numGoroutines {
		t.Fatalf("completed requests = %d, want %d", compCount, numGoroutines)
	}
}

// TestContextDeadlineExceededDuringSearch tests behavior when context deadline expires during search.
func TestContextDeadlineExceededDuringSearch(t *testing.T) {
	t.Parallel()

	// Use a custom RoundTripper that blocks until context cancellation,
	// avoiding httptest.Server.Close() blocking on active connections.
	client := &http.Client{
		Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			<-r.Context().Done()

			return nil, r.Context().Err()
		}),
	}

	cfg := &searxng.Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := testPerformSearch(ctx, t, cfg, &searxng.SearchArgs{Query: "test"})
	if err == nil {
		t.Error("Expected error due to context deadline, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "context deadline exceeded") &&
		!strings.Contains(errStr, "timeout") &&
		!strings.Contains(errStr, "request canceled") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// --- Connection Pool Exhaustion Tests ---

// TestConcurrentValidationAndSearch tests concurrent validation and search operations.
func TestConcurrentValidationAndSearch(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	searcher, _ := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}, false)

	var wg sync.WaitGroup

	const numGoroutines = 50

	var (
		validationErrors int64
		searchErrors     int64
	)

	wg.Add(numGoroutines * 2)

	// Concurrent validation calls
	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			args := &searxng.SearchArgs{
				Query:      "test",
				Language:   "en",
				SafeSearch: 0,
			}

			err := searxng.ValidateSearchArgs(args)
			if err != nil {
				atomic.AddInt64(&validationErrors, 1)
				t.Errorf("unexpected validation error: %v", err)
			}
		}(i)
	}

	// Concurrent search calls
	for i := range numGoroutines {
		go func(_ int) {
			defer wg.Done()

			ctx := context.Background()

			_, err := searcher.Search(ctx, &searxng.SearchArgs{Query: "test"})
			if err != nil {
				atomic.AddInt64(&searchErrors, 1)
				t.Errorf("search error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	if validationErrors != 0 {
		t.Fatalf("validationErrors = %d, want 0", validationErrors)
	}

	if searchErrors != 0 {
		t.Fatalf("searchErrors = %d, want 0", searchErrors)
	}
}

func TestSearchCloseDuringInFlightSearch(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	release := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}

		<-release

		searchResp := searxng.SearchResponse{
			Results:         []searxng.SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
			Suggestions:     []string{},
		}
		body := mustMarshalJSON(t, searchResp)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
	defer server.Close()

	searcher, err := searxng.NewSearXNGSearcher(&searxng.Config{SearXNGURL: server.URL, Timeout: 30 * time.Second}, false)
	if err != nil {
		t.Fatalf("failed to create searcher: %v", err)
	}

	done := make(chan error, 1)

	go func() {
		_, err = searcher.Search(context.Background(), &searxng.SearchArgs{Query: "test"})
		done <- err
	}()

	<-started

	err = searcher.Close()
	if err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	close(release)

	err = <-done
	if err != nil {
		t.Fatalf("Search() returned error: %v", err)
	}
}
