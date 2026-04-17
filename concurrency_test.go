package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

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

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		time.Sleep(2 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}

	const numGoroutines = 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			_, err := performSearch(ctx, cfg, &SearchArgs{Query: "test"})
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	count := atomic.LoadInt64(&requestCount)
	t.Logf("Total requests made before cancellation: %d", count)

	// Check errors are context-related
	for err := range errChan {
		errStr := err.Error()
		if !strings.Contains(errStr, "context deadline exceeded") && !strings.Contains(errStr, "request canceled") && !strings.Contains(errStr, "timeout") {
			t.Errorf("Expected context error, got: %v", err)
		}
	}
}

// TestConcurrentSearcherCreation tests creating many searchers concurrently
func TestConcurrentSearcherCreation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping concurrent searcher creation stress test in short mode")
	}
	var wg sync.WaitGroup
	const numGoroutines = 50

	errChan := make(chan error, numGoroutines)

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, err := NewSearXNGSearcher("https://search.example.com", 30*time.Second, nil)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		t.Errorf("Unexpected error creating searcher: %v", err)
	}
}

// TestConcurrentSameSearcherUse tests using the same searcher from multiple goroutines
func TestConcurrentSameSearcherUse(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping shared searcher stress test in short mode")
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

	searcher, err := NewSearXNGSearcher(server.URL, 30*time.Second, nil)
	if err != nil {
		t.Fatalf("Failed to create searcher: %v", err)
	}

	const numGoroutines = 30
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			_, err := searcher.Search(ctx, &SearchArgs{Query: "concurrent_test"})
			if err != nil {
				t.Errorf("Search error: %v", err)
			}
		}(i)
	}

	wg.Wait()

	count := atomic.LoadInt64(&requestCount)
	if count != numGoroutines {
		t.Errorf("Expected %d requests, got %d", numGoroutines, count)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			searcher := searchers[id%len(searchers)]
			ctx := context.Background()
			_, err := searcher.Search(ctx, &SearchArgs{Query: "race_test"})
			if err != nil {
				t.Errorf("Search error: %v", err)
			}
		}(i)
	}

	wg.Wait()
}

// TestSearcherThreadSafety tests that a single searcher can handle concurrent requests
func TestSearcherThreadSafety(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping thread-safety stress test in short mode")
	}
	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		searchResp := SearchResponse{
			Results: []SearchResult{
				{Title: "Result", URL: "https://example.com/1", Content: "Content", Engine: "google"},
			},
			NumberOfResults: 1,
			Query:           r.URL.Query().Get("q"),
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	searcher, _ := NewSearXNGSearcher(server.URL, 30*time.Second, nil)

	var wg sync.WaitGroup
	const numGoroutines = 100
	const iterationsPerGoroutine = 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < iterationsPerGoroutine; j++ {
				_, err := searcher.Search(ctx, &SearchArgs{Query: "test"})
				if err != nil {
					t.Errorf("Search error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int64(numGoroutines * iterationsPerGoroutine)
	actual := atomic.LoadInt64(&requestCount)
	if actual != expected {
		t.Errorf("Expected %d requests, got %d", expected, actual)
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

	t.Logf("Requests made: %d, Completed: %d", atomic.LoadInt64(&requestCount), atomic.LoadInt64(&completedCount))
}

// TestGracefulShutdownWithSignal tests that the server handles termination signals gracefully
func TestGracefulShutdownWithSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping signal shutdown stress test in short mode")
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
		Timeout:    5 * time.Second, // Shorter timeout for signal test
	}

	// Create a cancellable context simulating SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	searcher, _ := NewSearXNGSearcher(server.URL, cfg.Timeout, nil)

	// Start multiple searches
	var wg sync.WaitGroup
	const numGoroutines = 10

	wg.Add(numGoroutines)
	errChan := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_, err := searcher.Search(ctx, &SearchArgs{Query: "test"})
			if err != nil && ctx.Err() == nil {
				errChan <- err
			}
		}(i)
	}

	// Simulate signal after a short delay
	time.Sleep(50 * time.Millisecond)
	cancel()

	wg.Wait()
	close(errChan)

	for err := range errChan {
		// Errors after cancellation are expected
		if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "request canceled") {
			t.Errorf("Unexpected error after signal: %v", err)
		}
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

// TestHighConcurrencyStress tests the system under high concurrent load
func TestHighConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		searchResp := SearchResponse{
			Results: []SearchResult{
				{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
				{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
				{Title: "Result 3", URL: "https://example.com/3", Content: "Content 3", Engine: "duckduckgo"},
			},
			NumberOfResults: 3,
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
	const numGoroutines = 200
	const iterationsPerGoroutine = 10

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < iterationsPerGoroutine; j++ {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_, err := performSearch(ctx, cfg, &SearchArgs{
					Query:    fmt.Sprintf("stress_test_%d_%d", goroutineID, j),
					Language: []string{"en", "zh", "ja"}[j%3],
				})
				cancel()
				if err != nil {
					t.Errorf("Search error: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int64(numGoroutines * iterationsPerGoroutine)
	actual := atomic.LoadInt64(&requestCount)
	t.Logf("High concurrency stress test: expected=%d, actual=%d", expected, actual)

	if actual != expected {
		t.Errorf("Expected %d requests, got %d", expected, actual)
	}
}

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
			_ = ValidateSearchArgs(args)
		}(i)
	}

	// Concurrent search calls
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			_, _ = searcher.Search(ctx, &SearchArgs{Query: "test"})
		}(i)
	}

	wg.Wait()
}

// TestSharedSearcherAcrossGoroutines tests the same searcher being used by many goroutines
func TestSharedSearcherAcrossGoroutines(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping shared searcher load test in short mode")
	}
	requestCount := int64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)
		searchResp := SearchResponse{
			Results: []SearchResult{
				{Title: "Result", URL: r.URL.Query().Get("q"), Content: "Content", Engine: "test"},
			},
			NumberOfResults: 1,
			Query:           r.URL.Query().Get("q"),
		}
		body, _ := json.Marshal(searchResp)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	searcher, _ := NewSearXNGSearcher(server.URL, 30*time.Second, nil)

	// Use searcher from many goroutines simultaneously
	var wg sync.WaitGroup
	const numGoroutines = 100
	const queriesPerGoroutine = 20

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()
			ctx := context.Background()
			for j := 0; j < queriesPerGoroutine; j++ {
				query := fmt.Sprintf("goroutine_%d_query_%d", goroutineID, j)
				_, err := searcher.Search(ctx, &SearchArgs{Query: query})
				if err != nil {
					t.Errorf("Search error for %s: %v", query, err)
				}
			}
		}(i)
	}

	wg.Wait()

	expected := int64(numGoroutines * queriesPerGoroutine)
	actual := atomic.LoadInt64(&requestCount)
	if actual != expected {
		t.Errorf("Expected %d requests, got %d", expected, actual)
	}
}
