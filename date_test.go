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

func assertInferredDate(t *testing.T, content string, baseTime time.Time, expectedDate time.Time) {
	t.Helper()
	resp := &SearchResponse{
		Results: []SearchResult{{Title: "Test", URL: "https://example.com", Content: content}},
	}
	inferDates(resp, &baseTime)
	if resp.Results[0].DateSource != DateSourceInferred {
		t.Fatalf("DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceInferred)
	}
	inferredDate, err := time.Parse("2006-01-02", *resp.Results[0].PublishedDate)
	if err != nil {
		t.Fatalf("Failed to parse PublishedDate: %v", err)
	}
	dayDiff := inferredDate.Sub(expectedDate) / (24 * time.Hour)
	if dayDiff != 0 {
		t.Errorf("Inferred date = %v, expected %v (dayDiff=%v)", *resp.Results[0].PublishedDate, expectedDate.Format("2006-01-02"), dayDiff)
	}
}

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
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		expectedDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		assertInferredDate(t, "Posted 2 hours ago", baseTime, expectedDate)
	})

	t.Run("5 days ago accuracy", func(t *testing.T) {
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		expectedDate := time.Date(2024, 6, 10, 0, 0, 0, 0, time.UTC)
		assertInferredDate(t, "5 days ago news", baseTime, expectedDate)
	})

	t.Run("german vor 2 tagen accuracy", func(t *testing.T) {
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		expectedDate := time.Date(2024, 6, 13, 0, 0, 0, 0, time.UTC)
		assertInferredDate(t, "Nachricht vor 2 tagen veröffentlicht", baseTime, expectedDate)
	})

	t.Run("german vor 3 stunden accuracy", func(t *testing.T) {
		baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
		expectedDate := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)
		assertInferredDate(t, "Artikel vor 3 stunden geschrieben", baseTime, expectedDate)
	})
}

// TestInferDates_MixedDateSources tests that results with different date sources are handled correctly
func TestInferDates_MixedDateSources(t *testing.T) {
	resp := &SearchResponse{
		Results: []SearchResult{
			// Result with API-provided date
			{
				Title:         "API Date Result",
				URL:           "https://example.com/1",
				Content:       "Some content",
				PublishedDate: strPtr("2024-06-10"),
			},
			// Result without date (will be inferred)
			{
				Title:         "Inferred Date Result",
				URL:           "https://example.com/2",
				Content:       "Posted 2 days ago",
			},
			// Result with API date that should be preserved
			{
				Title:         "Another API Date",
				URL:           "https://example.com/3",
				Content:       "Random content",
				PublishedDate: strPtr("2024-01-15"),
			},
		},
	}
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	inferDates(resp, &baseTime)

	// First result should have API date source
	if resp.Results[0].DateSource != DateSourceAPI {
		t.Errorf("Results[0] DateSource = %v, want %v", resp.Results[0].DateSource, DateSourceAPI)
	}
	if *resp.Results[0].PublishedDate != "2024-06-10" {
		t.Errorf("Results[0] PublishedDate = %v, want %v", *resp.Results[0].PublishedDate, "2024-06-10")
	}

	// Second result should have inferred date source
	if resp.Results[1].DateSource != DateSourceInferred {
		t.Errorf("Results[1] DateSource = %v, want %v", resp.Results[1].DateSource, DateSourceInferred)
	}
	if resp.Results[1].PublishedDate == nil {
		t.Error("Results[1] PublishedDate should be set for inferred date")
	}

	// Third result should have API date source
	if resp.Results[2].DateSource != DateSourceAPI {
		t.Errorf("Results[2] DateSource = %v, want %v", resp.Results[2].DateSource, DateSourceAPI)
	}
	if *resp.Results[2].PublishedDate != "2024-01-15" {
		t.Errorf("Results[2] PublishedDate = %v, want %v", *resp.Results[2].PublishedDate, "2024-01-15")
	}
}

func strPtr(s string) *string {
	return &s
}
