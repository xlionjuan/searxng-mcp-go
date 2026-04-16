package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// --- formatResults tests ---

func TestFormatResults(t *testing.T) {
	tests := []struct {
		name           string
		resp           *SearchResponse
		wantContains   []string
		wantNotContain string
		wantResult      string
	}{
		{
			name: "normal results with content",
			resp: &SearchResponse{
				Results: []SearchResult{
					{
						Title:   "Test Title 1",
						URL:     "https://example.com/1",
						Content: "Test content 1",
						Engine:  "google",
					},
					{
						Title:   "Test Title 2",
						URL:     "https://example.com/2",
						Content: "",
						Engine:  "bing",
					},
				},
				NumberOfResults: 2,
				Query:           "test query",
			},
			wantContains: []string{"Found 2 results", "test query", "Test Title 1", "https://example.com/1", "Test content 1", "Test Title 2", "Summary:"},
		},
		{
			name:      "empty results",
			resp:      &SearchResponse{Results: []SearchResult{}, NumberOfResults: 0, Query: "empty query"},
			wantResult: "No results found.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatResults(tt.resp)

			if tt.wantResult != "" {
				if result != tt.wantResult {
					t.Errorf("expected %q, got: %s", tt.wantResult, result)
				}
				return
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result, want) {
					t.Errorf("expected %q in output, got: %s", want, result)
				}
			}
			if tt.wantNotContain != "" && strings.Contains(result, tt.wantNotContain) {
				t.Errorf("did not expect %q in output, got: %s", tt.wantNotContain, result)
			}
		})
	}
}

// --- performSearch tests ---

func TestPerformSearch_Success(t *testing.T) {
	searchResp := SearchResponse{
		Results: []SearchResult{
			{Title: "Result 1", URL: "https://example.com/1", Content: "Content 1", Engine: "google"},
			{Title: "Result 2", URL: "https://example.com/2", Content: "Content 2", Engine: "bing"},
		},
		NumberOfResults: 2,
		Query:           "test",
	}
	body, _ := json.Marshal(searchResp)

	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{
		Query:      "test",
		Language:   "en",
		SafeSearch: 1,
	}

	ctx := context.Background()
	result, err := performSearch(ctx, cfg, args)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(result.Results))
	}
	if result.Results[0].Title != "Result 1" {
		t.Errorf("expected 'Result 1', got %s", result.Results[0].Title)
	}

	// Verify request parameters using captured query
	tests := []struct {
		name string
		param string
		want  string
	}{
		{"query", "q", "test"},
		{"format", "format", "json"},
		{"language", "language", "en"},
		{"safesearch", "safesearch", "1"},
	}
	for _, tt := range tests {
		if capturedQuery.Get(tt.param) != tt.want {
			t.Errorf("expected %s=%q, got %q", tt.param, tt.want, capturedQuery.Get(tt.param))
		}
	}
}

func TestPerformSearch_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Close connection immediately to simulate network error
	}))
	server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected network error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to execute search request") {
		t.Errorf("expected specific error message, got: %v", err)
	}
}

func TestPerformSearch_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected error for non-OK status, got nil")
	}
	if !strings.Contains(err.Error(), "searxng error (status 500)") {
		t.Errorf("expected searxng error (status 500), got: %v", err)
	}
}

func TestPerformSearch_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not valid json{"))
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

func TestPerformSearch_InvalidURL(t *testing.T) {
	cfg := &Config{
		SearXNGURL: "://invalid-url",
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "NewSearXNGSearcher") {
		t.Errorf("expected NewSearXNGSearcher error message, got: %v", err)
	}
}

func TestPerformSearch_TimeRangeParam(t *testing.T) {
	var capturedTimeRange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedTimeRange = r.URL.Query().Get("time_range")
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

	tests := []struct {
		timeRange string
		want      string
	}{
		{"day", "day"},
		{"month", "month"},
		{"year", "year"},
		{"", ""},
	}

	for _, tt := range tests {
		capturedTimeRange = ""
		args := &SearchArgs{Query: "test", TimeRange: tt.timeRange}
		ctx := context.Background()
		_, err := performSearch(ctx, cfg, args)
		if err != nil {
			t.Errorf("performSearch() error = %v", err)
			continue
		}
		if capturedTimeRange != tt.want {
			t.Errorf("time_range = %q, want %q", capturedTimeRange, tt.want)
		}
	}
}

func TestPerformSearch_DefaultLanguage(t *testing.T) {
	var capturedLanguage string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedLanguage = r.URL.Query().Get("language")
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
	args := &SearchArgs{Query: "test"} // Language is empty, should default to en

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedLanguage != "en" {
		t.Errorf("expected default language 'en', got %q", capturedLanguage)
	}
}

func TestPerformSearch_Categories(t *testing.T) {
	var capturedCategories string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedCategories = r.URL.Query().Get("categories")
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
	args := &SearchArgs{Query: "test", Categories: "general,news"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedCategories != "general,news" {
		t.Errorf("expected categories 'general,news', got %q", capturedCategories)
	}
}

func TestPerformSearch_Engines(t *testing.T) {
	var capturedEngines string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedEngines = r.URL.Query().Get("engines")
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
	args := &SearchArgs{Query: "test", Engines: "google,bing"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedEngines != "google,bing" {
		t.Errorf("expected engines 'google,bing', got %q", capturedEngines)
	}
}

func TestPerformSearch_Pageno(t *testing.T) {
	var capturedPageno string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPageno = r.URL.Query().Get("pageno")
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
	args := &SearchArgs{Query: "test", Pageno: intPtr(2)}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPageno != "2" {
		t.Errorf("expected pageno '2', got %q", capturedPageno)
	}
}

func TestPerformSearch_HTMLResponseError(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		body        string
		desc        string
	}{
		{
			name:        "HTML content-type header",
			contentType: "text/html",
			body:        "<!DOCTYPE html><html><head><title>Search</title></head><body>JSON output not enabled</body></html>",
			desc:        "HTMLResponseError triggered by text/html content-type",
		},
		{
			name:        "body starts with DOCTYPE",
			contentType: "text/html; charset=utf-8",
			body:        "<!DOCTYPE html><html><body>Please enable JSON output</body></html>",
			desc:        "HTMLResponseError triggered by <!DOCTYPE prefix",
		},
		{
			name:        "body starts with html tag",
			contentType: "application/octet-stream",
			body:        "<html><body>JSON output not enabled on this instance</body></html>",
			desc:        "HTMLResponseError triggered by <html> prefix even without HTML content-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(http.StatusOK)
				w.Write([]byte(tt.body))
			}))
			defer server.Close()

			cfg := &Config{
				SearXNGURL: server.URL,
				Timeout:    30 * time.Second,
			}
			args := &SearchArgs{Query: "test"}

			ctx := context.Background()
			_, err := performSearch(ctx, cfg, args)

			if err == nil {
				t.Fatal("expected HTMLResponseError, got nil")
			}

			var htmlErr *HTMLResponseError
			if !errors.As(err, &htmlErr) {
				t.Errorf("expected HTMLResponseError, got: %v", err)
			}

			if !strings.Contains(err.Error(), "searxng returned html instead of json") {
				t.Errorf("expected HTML response error message, got: %v", err)
			}
		})
	}
}

// --- helper functions and test utilities ---

func intPtr(i int) *int {
	return &i
}

// --- parseRelativeDate tests ---

func TestParseRelativeDate(t *testing.T) {
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	h1 := time.Date(2024, 6, 15, 11, 0, 0, 0, time.UTC)
	h2 := time.Date(2024, 6, 15, 10, 0, 0, 0, time.UTC)
	h3 := time.Date(2024, 6, 15, 9, 0, 0, 0, time.UTC)
	h24 := time.Date(2024, 6, 14, 12, 0, 0, 0, time.UTC)
	h5st := time.Date(2024, 6, 15, 7, 0, 0, 0, time.UTC)
	h1st := time.Date(2024, 6, 15, 11, 0, 0, 0, time.UTC)
	d1 := time.Date(2024, 6, 14, 12, 0, 0, 0, time.UTC)
	d5 := time.Date(2024, 6, 10, 12, 0, 0, 0, time.UTC)
	d3t := time.Date(2024, 6, 12, 12, 0, 0, 0, time.UTC)
	d1t := time.Date(2024, 6, 14, 12, 0, 0, 0, time.UTC)
	y := time.Date(2024, 6, 14, 12, 0, 0, 0, time.UTC)
	vg := time.Date(2024, 6, 13, 12, 0, 0, 0, time.UTC)
	lw := time.Date(2024, 6, 8, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		content  string
		wantDate *time.Time
	}{
		{"empty content", "", nil},
		{"no date keywords", "This is some random content without any date information", nil},
		// Exact boundary: 1 hour
		{"1 hour ago", "Posted 1 hour ago by admin", &h1},
		{"2 hours ago", "Posted 2 hours ago by admin", &h2},
		{"3 hours ago", "3 hours ago, the news was published", &h3},
		{"24 hours h", "Article from 24 hours h ago", &h24},
		// German hour boundaries
		{"German 1 stunde ago", "Nachricht vor 1 stunde veröffentlicht", &h1st},
		{"German 5 stunden vor", "Nachricht vor 5 stunden veröffentlicht", &h5st},
		// Day boundaries
		{"1 day ago", "News from 1 day ago", &d1},
		{"5 days ago", "Published 5 days ago", &d5},
		// German day boundaries
		{"German 1 tag vor", "Vor 1 tag wurde berichtet", &d1t},
		{"German 3 tagen vor", "Vor 3 tagen wurde berichtet", &d3t},
		// Special keywords
		{"yesterday", "Article posted yesterday", &y},
		{"German vorgestern", "Vorgestern wurde bekannt gegeben", &vg},
		{"last week", "Report from last week suggests", &lw},
		{"German vor woche", "Vor woche gab es eine ankündigung", &lw},
		// Weeks patterns not implemented in parseRelativeDate (regex exists but unused)
		{"2 weeks ago - not implemented", "Article from 2 weeks ago", nil},
		{"2 wochen ago - not implemented", "Nachricht vor 2 wochen", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseRelativeDate(tt.content, baseTime)
			if tt.wantDate == nil {
				if got != nil {
					t.Errorf("parseRelativeDate() = %v, want nil", *got)
				}
			} else {
				if got == nil {
					t.Errorf("parseRelativeDate() = nil, want %v", *tt.wantDate)
				} else if !got.Equal(*tt.wantDate) {
					t.Errorf("parseRelativeDate() = %v, want %v", *got, *tt.wantDate)
				}
			}
		})
	}
}

func TestParseRelativeDate_ZeroHours(t *testing.T) {
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// "0 hours ago" should return nil because hours must be > 0
	result := parseRelativeDate("Posted 0 hours ago", baseTime)
	if result != nil {
		t.Errorf("parseRelativeDate() with 0 hours should return nil, got %v", *result)
	}
}

func TestParseRelativeDate_UpperBoundaries(t *testing.T) {
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	// 48 hours is the upper limit for hours
	h48 := time.Date(2024, 6, 13, 12, 0, 0, 0, time.UTC)
	result := parseRelativeDate("Posted 48 hours ago", baseTime)
	if result == nil {
		t.Error("parseRelativeDate() with 48 hours should return a date")
	} else if !result.Equal(h48) {
		t.Errorf("parseRelativeDate() with 48 hours = %v, want %v", *result, h48)
	}
}

func TestParseRelativeDate_FutureDate(t *testing.T) {
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	result := parseRelativeDate("Published 100 hours ago", baseTime)
	if result != nil {
		t.Errorf("future date should be discarded, got %v", *result)
	}
}

func TestParseRelativeDate_TooOld(t *testing.T) {
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	result := parseRelativeDate("Published 500 days ago", baseTime)
	if result != nil {
		t.Errorf("date before 2000 should be discarded, got %v", *result)
	}
}

// --- inferDates tests ---

func TestInferDates(t *testing.T) {
	t.Run("api date preserved", func(t *testing.T) {
		apiDate := "2024-06-10"
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "some content", PublishedDate: &apiDate},
			},
		}
		inferDates(resp)
		if resp.Results[0].DateSource != DateSourceAPI {
			t.Errorf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceAPI)
		}
	})

	t.Run("inferred date", func(t *testing.T) {
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Posted 2 days ago"},
			},
		}
		inferDates(resp)
		if resp.Results[0].DateSource != DateSourceInferred {
			t.Errorf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}
		if resp.Results[0].PublishedDate == nil {
			t.Errorf("PublishedDate should be set")
		}
	})

	t.Run("no date possible", func(t *testing.T) {
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Random content without dates"},
			},
		}
		inferDates(resp)
		if resp.Results[0].DateSource != DateSourceNone {
			t.Errorf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceNone)
		}
	})

	// Accuracy tests: verify calculated dates are correct
	t.Run("2 hours ago accuracy", func(t *testing.T) {
		before := time.Now()
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Posted 2 hours ago"},
			},
		}
		inferDates(resp)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// For hours, the date is stored as YYYY-MM-DD without time,
		// so we need to check if the date matches (with +/- 1 day tolerance for edge cases)
		// Since "2 hours ago" could cross midnight, we check the date is reasonable
		expectedDate := before.AddDate(0, 0, 0).Format("2006-01-02") // today
		dayDiff := before.Sub(inferredDate) / (24 * time.Hour)

		// Should be either today or yesterday depending on when run
		if dayDiff < 0 || dayDiff > 1 {
			t.Errorf("Inferred date too far from expected: got %v, expected around %v",
				resp.Results[0].PublishedDate, expectedDate)
		}
	})

	t.Run("5 days ago accuracy", func(t *testing.T) {
		before := time.Now()
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "5 days ago news"},
			},
		}
		inferDates(resp)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// Expected date should be approximately 5 days ago
		expectedDate := before.AddDate(0, 0, -5)
		dayDiff := (before.Sub(inferredDate) / (24 * time.Hour))

		// Allow 1 day tolerance for time of day crossing midnight
		if dayDiff < 4 || dayDiff > 6 {
			t.Errorf("Inferred date not 5 days ago: got %v (dayDiff=%v), expected around %v",
				resp.Results[0].PublishedDate, dayDiff, expectedDate.Format("2006-01-02"))
		}
	})

	t.Run("german vor 2 tagen accuracy", func(t *testing.T) {
		before := time.Now()
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Nachricht vor 2 tagen veröffentlicht"},
			},
		}
		inferDates(resp)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		dayDiff := (before.Sub(inferredDate) / (24 * time.Hour))
		if dayDiff < 1 || dayDiff > 3 {
			t.Errorf("German 'vor 2 tagen' not ~2 days ago: got %v (dayDiff=%v)",
				resp.Results[0].PublishedDate, dayDiff)
		}
	})

	t.Run("german vor 3 stunden accuracy", func(t *testing.T) {
		before := time.Now()
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Artikel vor 3 stunden geschrieben"},
			},
		}
		inferDates(resp)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// "vor 3 stunden" (3 hours ago) - since date only has day resolution,
		// it should be today or yesterday
		dayDiff := (before.Sub(inferredDate) / (24 * time.Hour))
		if dayDiff < 0 || dayDiff > 1 {
			t.Errorf("German 'vor 3 stunden' not close to today: got %v (dayDiff=%v)",
				resp.Results[0].PublishedDate, dayDiff)
		}
	})
}

func TestPerformSearch_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second) // Simulate slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected context deadline exceeded error, got nil")
	}
	// The error should be related to context cancellation/timeout
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "request canceled") {
		t.Errorf("expected context-related error, got: %v", err)
	}
}

// Test performSearch with empty query (edge case - query parameter still set)
// Test HTTP status code errors
func TestPerformSearch_HTTPStatusCodes(t *testing.T) {
	statusCodes := []struct {
		code       int
		statusName string
		errPart    string
	}{
		{400, "Bad Request", "searxng error (status 400)"},
		{401, "Unauthorized", "searxng error (status 401)"},
		{403, "Forbidden", "searxng error (status 403)"},
		{404, "Not Found", "searxng error (status 404)"},
		{429, "Too Many Requests", "searxng error (status 429)"},
		{500, "Internal Server Error", "searxng error (status 500)"},
		{502, "Bad Gateway", "searxng error (status 502)"},
		{503, "Service Unavailable", "searxng error (status 503)"},
		{504, "Gateway Timeout", "searxng error (status 504)"},
	}

	for _, tc := range statusCodes {
		t.Run(tc.statusName, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.code)
				w.Write([]byte(fmt.Sprintf("Error %d: %s", tc.code, tc.statusName)))
			}))
			defer server.Close()

			cfg := &Config{
				SearXNGURL: server.URL,
				Timeout:    30 * time.Second,
			}
			args := &SearchArgs{Query: "test"}

			ctx := context.Background()
			_, err := performSearch(ctx, cfg, args)

			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.code)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.errPart) {
				t.Errorf("expected error containing %q, got: %v", tc.errPart, err)
			}
		})
	}
}

// Test non-JSON responses
func TestPerformSearch_NonJSONResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("<html><body>Not Found</body></html>"))
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected error for non-JSON response, got nil")
	}
	if !strings.Contains(err.Error(), "html instead of json") {
		t.Errorf("expected HTML/JSON error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "json output likely not enabled") {
		t.Errorf("expected JSON not enabled error, got: %v", err)
	}
}

// Test JSON parse error with helpful message
func TestPerformSearch_JSONParseError(t *testing.T) {
	invalidJSON := []byte(`{"results": [{"title": "test", `) // truncated/invalid JSON
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(invalidJSON)
	}))
	defer server.Close()

	cfg := &Config{
		SearXNGURL: server.URL,
		Timeout:    30 * time.Second,
	}
	args := &SearchArgs{Query: "test"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)

	if err == nil {
		t.Fatal("expected JSON parse error, got nil")
	}
	if !strings.Contains(err.Error(), "failed to parse JSON response") {
		t.Errorf("expected JSON parse error, got: %v", err)
	}
}

// Test httpStatusError function directly
func TestHTTPStatusError(t *testing.T) {
	tests := []struct {
		statusCode  int
		contentType string
		body        []byte
		errContains string
	}{
		{400, "text/html", []byte("Bad Request"), "searxng error (status 400)"},
		{401, "text/html", []byte("Unauthorized"), "searxng error (status 401)"},
		{403, "text/html", []byte("Forbidden"), "searxng error (status 403)"},
		{404, "text/html", []byte("Not Found"), "searxng error (status 404)"},
		{429, "text/html", []byte("Rate Limited"), "searxng error (status 429)"},
		{500, "text/html", []byte("Internal Error"), "searxng error (status 500)"},
		{502, "text/html", []byte("Bad Gateway"), "searxng error (status 502)"},
		{503, "text/html", []byte("Unavailable"), "searxng error (status 503)"},
		{504, "text/html", []byte("Timeout"), "searxng error (status 504)"},
		{418, "text/html", []byte("I'm a teapot"), "searxng error (status 418)"},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("status_%d", tc.statusCode), func(t *testing.T) {
			err := HTTPStatusError(tc.statusCode, tc.contentType, tc.body)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.statusCode)
			}
			if !strings.Contains(strings.ToLower(err.Error()), tc.errContains) {
				t.Errorf("expected error containing %q, got: %v", tc.errContains, err)
			}
		})
	}
}

// Verify formatResults with various content lengths

// Test that performSearch properly encodes query parameters
func TestPerformSearch_QueryEncoding(t *testing.T) {
	var capturedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query().Get("q")
		searchResp := SearchResponse{
			Results:         []SearchResult{},
			NumberOfResults: 0,
			Query:           capturedQuery,
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

	// Test query with special characters that need URL encoding
	args := &SearchArgs{Query: "test query with spaces & special=chars"}

	ctx := context.Background()
	_, err := performSearch(ctx, cfg, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedQuery != "test query with spaces & special=chars" {
		t.Errorf("expected decoded query 'test query with spaces & special=chars', got %q", capturedQuery)
	}
}

// --- ValidateSearchArgs tests ---

// --- printCLIHelp tests ---

func TestPrintCLIHelp(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printCLIHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	output := buf.String()

	// Verify help content includes expected sections
	expectedContent := []string{
		"SearXNG MCP Server - CLI Mode",
		"USAGE:",
		"OPTIONS:",
		"--query",
		"--json",
		"--searxng-url",
		"--language",
		"--safesearch",
		"--time_range",
		"--categories",
		"--engines",
		"--pageno",
		"--help",
		"--version",
		"ARGUMENTS:",
		"QUERY",
		"OUTPUT:",
		"EXAMPLES:",
		"MCP MODE:",
		"EXIT CODES:",
		"https://github.com/xlionjuan/searxng-mcp-go",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(output, expected) {
			t.Errorf("printCLIHelp() output missing expected content %q", expected)
		}
	}
}

// --- runCLIMode integration test notes ---
// runCLIMode() is challenging to unit test directly because:
// 1. It calls os.Exit() on errors, which terminates the test process
// 2. It uses global flag variables (*cliHelp, *cliVersion, *cliQuery, etc.)
// 3. It makes HTTP calls to SearXNG
//
// Integration test approach (as a separate binary or script):
// - Build the binary: go build -o searxng-mcp-go .
// - Test help: ./searxng-mcp-go --help (check exit code 0 and help output)
// - Test version: ./searxng-mcp-go --version (check exit code 0 and version output)
// - Test query with mock server:
//   - Start a mock SearXNG server on localhost
//   - Run: ./searxng-mcp-go --searxng-url=http://localhost:port "test query"
//   - Verify search results output or JSON format
// - Test missing query: ./searxng-mcp-go (check exit code 1 and error message)
//
// Example integration test using exec.Command in Go:
//
// func TestRunCLIModeIntegration(t *testing.T) {
//     // Test --help flag
//     cmd := exec.Command("./searxng-mcp-go", "--help")
//     output, err := cmd.Output()
//     if err != nil {
//         t.Fatalf("expected no error for --help, got: %v", err)
//     }
//     if !strings.Contains(string(output), "SearXNG MCP Server") {
//         t.Errorf("expected help header in output")
//     }
// }

// --- runCLIMode tests ---

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_MissingQuery tests that runCLIMode returns an error when no query is provided
// instead of calling os.Exit
func TestRunCLIMode_MissingQuery(t *testing.T) {
	// Reset flag values to ensure clean state
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""
	// Reset pageno to default value
	cliPageno = flag.Int("pageno", 1, "Page number for pagination")

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected error for missing query, got nil")
	}
	if !strings.Contains(err.Error(), "search query is required") {
		t.Errorf("expected 'search query is required' error, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_ValidationError tests that runCLIMode returns validation errors
// instead of calling os.Exit
func TestRunCLIMode_ValidationError(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = "" // Will be overridden by positional arg
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = -1 // Invalid: should trigger validation error
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""

	// Test with safesearch = -1 (invalid)
	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for safesearch=-1, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "safesearch") {
		t.Errorf("expected safesearch in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_InvalidTimeRange tests that invalid time_range returns validation error
func TestRunCLIMode_InvalidTimeRange(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = "invalid" // Invalid time_range
	*cliCategories = ""
	*cliEngines = ""

	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for invalid time_range, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "time_range") {
		t.Errorf("expected time_range in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_InvalidPageno tests that invalid pageno returns validation error
func TestRunCLIMode_InvalidPageno(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""

	// Set pageno to invalid value (0)
	*cliPageno = 0

	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for pageno=0, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "pageno") {
		t.Errorf("expected pageno in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_QueryTooLong tests that query > 500 chars returns validation error
func TestRunCLIMode_QueryTooLong(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = strings.Repeat("a", 501) // Too long query
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected validation error for query too long, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "500 characters") {
		t.Errorf("expected '500 characters' in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_HelpFlag tests that --help returns without error
func TestRunCLIMode_HelpFlag(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = true // Setting help should return nil
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""

	err := runCLIMode([]string{})
	if err != nil {
		t.Errorf("runCLIMode() with --help should return nil, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_VersionFlag tests that --version returns without error
func TestRunCLIMode_VersionFlag(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = false
	*cliVersion = true // Setting version should return nil
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""

	err := runCLIMode([]string{})
	if err != nil {
		t.Errorf("runCLIMode() with --version should return nil, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_SearchErrorReturnsError tests that search errors are returned as errors
// and not cause os.Exit to be called
func TestRunCLIMode_SearchErrorReturnsError(t *testing.T) {
	// Reset flag values
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = "test query"
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""
	*cliSearXNGURL = "http://localhost:99999" // Valid URL scheme but unreachable
	*cliPageno = 1

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	// Should NOT be a validation error - it should be a search error about invalid URL
	if !strings.Contains(err.Error(), "search error") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected search error with 'invalid', got: %v", err)
	}
}

// --- ValidateSearchArgs tests ---

func TestValidateSearchArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        *SearchArgs
		wantErr     bool
		errField    string
		errContains string // Optional substring that should be in error message
	}{
		// nil args
		{
			name:    "nil args",
			args:    nil,
			wantErr: true,
			errField: "args",
		},
		// empty query
		{
			name:        "empty query",
			args:        &SearchArgs{Query: ""},
			wantErr:     true,
			errField:    "query",
			errContains: "search query is required",
		},
		// query too long (> 500 characters)
		{
			name:        "query too long",
			args:        &SearchArgs{Query: strings.Repeat("a", 501)},
			wantErr:     true,
			errField:    "query",
			errContains: "must be 500 characters or less",
		},
		// query exactly 500 characters (should pass)
		{
			name:    "query exactly 500 characters",
			args:    &SearchArgs{Query: strings.Repeat("a", 500)},
			wantErr: false,
		},
		// invalid time_range
		{
			name:    "invalid time_range hour",
			args:    &SearchArgs{Query: "test", TimeRange: "hour"},
			wantErr: true,
			errField: "time_range",
		},
		{
			name:    "invalid time_range week",
			args:    &SearchArgs{Query: "test", TimeRange: "week"},
			wantErr: true,
			errField: "time_range",
		},
		{
			name:    "invalid time_range invalid",
			args:    &SearchArgs{Query: "test", TimeRange: "invalid"},
			wantErr: true,
			errField: "time_range",
		},
		// valid time_range values
		{
			name:    "valid time_range day",
			args:    &SearchArgs{Query: "test", TimeRange: "day"},
			wantErr: false,
		},
		{
			name:    "valid time_range month",
			args:    &SearchArgs{Query: "test", TimeRange: "month"},
			wantErr: false,
		},
		{
			name:    "valid time_range year",
			args:    &SearchArgs{Query: "test", TimeRange: "year"},
			wantErr: false,
		},
		// empty time_range is valid (optional)
		{
			name:    "empty time_range is valid",
			args:    &SearchArgs{Query: "test", TimeRange: ""},
			wantErr: false,
		},
		// safesearch out of range
		{
			name:    "safesearch negative",
			args:    &SearchArgs{Query: "test", SafeSearch: -1},
			wantErr: true,
			errField: "safesearch",
		},
		{
			name:    "safesearch too large",
			args:    &SearchArgs{Query: "test", SafeSearch: 3},
			wantErr: true,
			errField: "safesearch",
		},
		// safesearch valid values
		{
			name:    "safesearch 0",
			args:    &SearchArgs{Query: "test", SafeSearch: 0},
			wantErr: false,
		},
		{
			name:    "safesearch 1",
			args:    &SearchArgs{Query: "test", SafeSearch: 1},
			wantErr: false,
		},
		{
			name:    "safesearch 2",
			args:    &SearchArgs{Query: "test", SafeSearch: 2},
			wantErr: false,
		},
		// normal valid args
		{
			name:    "valid args with all fields",
			args:    &SearchArgs{Query: "golang mcp", Language: "en", SafeSearch: 1, TimeRange: "month"},
			wantErr: false,
		},
		{
			name:    "valid args with only query",
			args:    &SearchArgs{Query: "test search"},
			wantErr: false,
		},
		// pageno validation
		{
			name:    "pageno zero",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(0)},
			wantErr: true,
			errField: "pageno",
		},
		{
			name:    "pageno negative",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(-1)},
			wantErr: true,
			errField: "pageno",
		},
		{
			name:    "pageno valid 1",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(1)},
			wantErr: false,
		},
		{
			name:    "pageno valid 5",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(5)},
			wantErr: false,
		},
		{
			name:    "pageno nil is valid",
			args:    &SearchArgs{Query: "test", Pageno: nil},
			wantErr: false,
		},
		// language validation
		{
			name:    "language empty is valid",
			args:    &SearchArgs{Query: "test", Language: ""},
			wantErr: false,
		},
		{
			name:    "language en is valid",
			args:    &SearchArgs{Query: "test", Language: "en"},
			wantErr: false,
		},
		{
			name:    "language zh-tw is valid",
			args:    &SearchArgs{Query: "test", Language: "zh-tw"},
			wantErr: false,
		},
		{
			name:    "language ja is valid",
			args:    &SearchArgs{Query: "test", Language: "ja"},
			wantErr: false,
		},
		{
			name:    "language invalid",
			args:    &SearchArgs{Query: "test", Language: "INVALID_LANG"},
			wantErr: true,
			errField: "language",
		},
		{
			name:    "language invalid2",
			args:    &SearchArgs{Query: "test", Language: "xyz"},
			wantErr: true,
			errField: "language",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSearchArgs() expected error, got nil")
					return
				}
				ve, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("ValidateSearchArgs() expected ValidationError, got %T", err)
					return
				}
				if ve.Field != tt.errField {
					t.Errorf("ValidateSearchArgs() error field = %q, want %q", ve.Field, tt.errField)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateSearchArgs() error message = %q, want to contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSearchArgs() unexpected error: %v", err)
				}
			}
		})
	}
}
