package main

import (
	"testing"
	"time"
)

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
		inferDates(resp, nil)
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
		inferDates(resp, nil)
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
		inferDates(resp, nil)
		if resp.Results[0].DateSource != DateSourceNone {
			t.Errorf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceNone)
		}
	})

	// Accuracy tests: verify calculated dates are correct with fixed baseTime
	t.Run("2 hours ago accuracy", func(t *testing.T) {
		// Fixed baseTime: 2024-06-15 12:00:00 UTC
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Posted 2 hours ago"},
			},
		}
		inferDates(resp, &baseTime)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// baseTime is 2024-06-15, 2 hours ago = 2024-06-15 10:00:00
		// Date stored as YYYY-MM-DD = "2024-06-15"
		expectedDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		dayDiff := inferredDate.Sub(expectedDate) / (24 * time.Hour)

		// Should be same day (0 days difference)
		if dayDiff != 0 {
			t.Errorf("Inferred date = %v, expected %v (dayDiff=%v)",
				*resp.Results[0].PublishedDate, expectedDate.Format("2006-01-02"), dayDiff)
		}
	})

	t.Run("5 days ago accuracy", func(t *testing.T) {
		// Fixed baseTime: 2024-06-15 12:00:00 UTC
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "5 days ago news"},
			},
		}
		inferDates(resp, &baseTime)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// baseTime is 2024-06-15, 5 days ago = 2024-06-10
		expectedDate := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
		dayDiff := inferredDate.Sub(expectedDate) / (24 * time.Hour)

		if dayDiff != 0 {
			t.Errorf("Inferred date = %v, expected %v (dayDiff=%v)",
				*resp.Results[0].PublishedDate, expectedDate.Format("2006-01-02"), dayDiff)
		}
	})

	t.Run("german vor 2 tagen accuracy", func(t *testing.T) {
		// Fixed baseTime: 2024-06-15 12:00:00 UTC
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Nachricht vor 2 tagen veröffentlicht"},
			},
		}
		inferDates(resp, &baseTime)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// baseTime is 2024-06-15, "vor 2 tagen" = 2 days ago = 2024-06-13
		expectedDate := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)
		dayDiff := inferredDate.Sub(expectedDate) / (24 * time.Hour)

		if dayDiff != 0 {
			t.Errorf("Inferred date = %v, expected %v (dayDiff=%v)",
				*resp.Results[0].PublishedDate, expectedDate.Format("2006-01-02"), dayDiff)
		}
	})

	t.Run("german vor 3 stunden accuracy", func(t *testing.T) {
		// Fixed baseTime: 2024-06-15 12:00:00 UTC
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		resp := &SearchResponse{
			Results: []SearchResult{
				{Title: "Test", URL: "https://example.com", Content: "Artikel vor 3 stunden geschrieben"},
			},
		}
		inferDates(resp, &baseTime)

		if resp.Results[0].DateSource != DateSourceInferred {
			t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
		}

		inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
		if err != nil {
			t.Fatalf("Failed to parse PublishedDate: %v", err)
		}

		// baseTime is 2024-06-15, 3 hours ago = 2024-06-15 09:00:00
		// Date stored as YYYY-MM-DD = "2024-06-15"
		expectedDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		dayDiff := inferredDate.Sub(expectedDate) / (24 * time.Hour)

		if dayDiff != 0 {
			t.Errorf("Inferred date = %v, expected %v (dayDiff=%v)",
				*resp.Results[0].PublishedDate, expectedDate.Format("2006-01-02"), dayDiff)
		}
	})
}
