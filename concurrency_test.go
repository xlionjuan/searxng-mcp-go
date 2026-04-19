package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cancelRoundTripperFunc func(*http.Request) (*http.Response, error)

func (f cancelRoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

// --- Concurrent Search Stress Tests ---

// TestConcurrentSearches runs multiple searches simultaneously with different parameters
func TestConcurrentSearches(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent search stress test in short mode")
	}
	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		searchResp := SearchResponse{
			Results: []SearchResult{
				{Title: "Result", URL: "https://example.com/1", Content: "Content", Engine: "google"},
			},
			NumberOfResults: 1,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	const numGoroutines = 20
	const queriesPerGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < queriesPerGoroutine; j++ {
				query := "query"
				args := &SearchArgs{Query: query}

				ctx := context.Background()
				_, err := performSearch(ctx, cfg, args)
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

// TestConcurrentContextCancellation tests concurrent context cancellation
func TestConcurrentContextCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent cancellation stress test in short mode")
	}
	requestCount := int64(0)
	canceledCount := int64(0)

	client := &http.Client{
		Transport: cancelRoundTripperFunc(func(r *http.Request) (*http.Response, error) {
			atomic.AddInt64(&requestCount, 1)
			<-r.Context().Done()
			atomic.AddInt64(&canceledCount, 1)
			return nil, r.Context().Err()
		}),
	}

	cfg := &Config{
		SearXNGURL: "https://example.com",
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}

	const numGoroutines = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	var successCount int64
	var errorCount int64
	var cancelledCount int64
	var unexpectedErrors []error
	var mu sync.Mutex

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := performSearch(ctx, cfg, &SearchArgs{Query: "test"})
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

// TestChannelDeadlockDetection tests that channels don't deadlock
func TestChannelDeadlockDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deadlock stress test in short mode")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	var wg sync.WaitGroup
	const numGoroutines = 50

	// Launch many concurrent searches
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			_, _ = performSearch(ctx, cfg, &SearchArgs{Query: "test"})
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

// TestRaceConditionOnSharedState tests for race conditions on shared HTTP client
func TestRaceConditionOnSharedState(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping shared state race test in short mode")
	}
	requestCount := int64(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		searchResp := SearchResponse{
			Results: []SearchResult{
				{Title: "Result", URL: "https://example.com/1", Content: "Content", Engine: "google"},
			},
			NumberOfResults: 1,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	// Create a shared HTTP client
	sharedClient := &http.Client{Timeout: 30 * time.Second}

	// Create multiple searchers sharing the same HTTP client
	var searchers [5]*SearXNGSearcher
	for i := 0; i < 5; i++ {
		searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, sharedClient)
		if err != nil {
			t.Fatalf("Failed to create searcher: %v", err)
		}
		searchers[i] = searcher
	}

	var wg sync.WaitGroup
	const numGoroutines = 50
	var errorCount int64

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			searcher := searchers[id%len(searchers)]
			ctx := context.Background()
			_, err := searcher.Search(ctx, &SearchArgs{Query: "race_test"})
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

// TestGracefulShutdownWithContextCancel tests that search operations respect context cancellation
func TestGracefulShutdownWithContextCancel(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping graceful shutdown stress test in short mode")
	}
	requestCount := int64(0)
	completedCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		time.Sleep(500 * time.Millisecond) // Simulate work
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"results":[],"number_of_results":0,"query":"test"}`))
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	const numGoroutines = 20

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, _ = performSearch(ctx, cfg, &SearchArgs{Query: "test"})
			atomic.AddInt64(&completedCount, 1)
		}(i)
	}

	// Cancel context after some requests have started
	time.Sleep(100 * time.Millisecond)
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
	compCount := atomic.LoadInt64(&completedCount)
	if reqCount == 0 {
		t.Fatal("expected at least one request to be made")
	}
	if compCount > reqCount {
		t.Fatalf("completedCount=%d > requestCount=%d", compCount, reqCount)
	}
}

// TestContextDeadlineExceededDuringSearch tests behavior when context deadline expires during search
func TestContextDeadlineExceededDuringSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deadline stress test in short mode")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Longer than any reasonable timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := performSearch(ctx, cfg, &SearchArgs{Query: "test"})
	if err == nil {
		t.Error("Expected error due to context deadline, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "context deadline exceeded") && !strings.Contains(errStr, "timeout") && !strings.Contains(errStr, "request canceled") {
		t.Errorf("Expected context-related error, got: %v", err)
	}
}

// --- Connection Pool Exhaustion Tests ---

// TestConcurrentValidationAndSearch tests concurrent validation and search operations
func TestConcurrentValidationAndSearch(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping validation/search concurrency stress test in short mode")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           "test",
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	searcher, _ := NewSearXNGSearcher(server.URL, 30*time.Second, nil)

	var wg sync.WaitGroup
	const numGoroutines = 50
	var validationErrors int64
	var searchErrors int64

	wg.Add(numGoroutines * 2)

	// Concurrent validation calls
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			args := &SearchArgs{
				Query:      "test",
				Language:   "en",
				SafeSearch: 0,
			}
			if err := ValidateSearchArgs(args); err != nil {
				atomic.AddInt64(&validationErrors, 1)
				t.Errorf("unexpected validation error: %v", err)
			}
		}(i)
	}

	// Concurrent search calls
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			if _, err := searcher.Search(ctx, &SearchArgs{Query: "test"}); err != nil {
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
