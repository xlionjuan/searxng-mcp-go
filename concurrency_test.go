package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- Concurrent Search Stress Tests ---

// TestConcurrentSearches runs multiple searches simultaneously with different parameters
func TestConcurrentSearches(t *testing.T) {
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

// TestConcurrentMapAccess tests concurrent access to shared state in validation
func TestConcurrentMapAccess(t *testing.T) {
	var wg sync.WaitGroup
	const numGoroutines = 50

	// Concurrent reads of validLanguages map from validation.go
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = validLanguages["en"]
				_ = validLanguages["zh"]
				_ = validLanguages["ja"]
			}
		}(i)
	}

	wg.Wait()

	// Test actual validation concurrent access
	validLang := "en"
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				args := &SearchArgs{
					Query:    "test",
					Language: validLang,
				}
				_ = ValidateSearchArgs(args)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentSearcherCreation tests creating many searchers concurrently
func TestConcurrentSearcherCreation(t *testing.T) {
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

// TestGoroutineLeakDetection tests that no goroutines are leaked by search operations
func TestGoroutineLeakDetection(t *testing.T) {
	initialGoroutines := runtime.NumGoroutine()

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

	// Run many searches
	const iterations = 50
	for i := 0; i < iterations; i++ {
		ctx := context.Background()
		_, _ = performSearch(ctx, cfg, &SearchArgs{Query: "test"})
	}

	// Give any background goroutines time to clean up
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// Allow some variance but should be close to initial
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak: started with %d, ended with %d after %d iterations",
			initialGoroutines, finalGoroutines, iterations)
	}
}

// TestChannelDeadlockDetection tests that channels don't deadlock
func TestChannelDeadlockDetection(t *testing.T) {
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

// TestConcurrentTransportUsage tests that the HTTP transport doesn't have races
func TestConcurrentTransportUsage(t *testing.T) {
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

	// Create custom transport
	transport := &http.Transport{}
	client := &http.Client{Transport: transport}

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
		HTTPClient: client,
	}

	var wg sync.WaitGroup
	const numGoroutines = 30

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			ctx := context.Background()
			_, _ = performSearch(ctx, cfg, &SearchArgs{Query: "test"})
		}(i)
	}

	wg.Wait()
}

// TestSearcherThreadSafety tests that a single searcher can handle concurrent requests
func TestSearcherThreadSafety(t *testing.T) {
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
