package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// Date Parsing
// ============================================================================

// Package-level regex patterns (compiled once)
var (
	hourPattern      = regexp.MustCompile(`(\d+)\s*(hour|hours|h|stunde|stunden)\s*(ago|vor)?`)
	dayPattern       = regexp.MustCompile(`(\d+)\s*(day|days|d|tag|tagen)\s*(ago|vor)?`)
	vorHoursPattern  = regexp.MustCompile(`vor\s+(\d+)\s*(stunde|stunden)\b`)
	vorDaysPattern   = regexp.MustCompile(`vor\s+(\d+)\s*(tag|tagen)\b`)
)

func parseRelativeDate(content string, currentTime time.Time) *time.Time {
	if content == "" {
		return nil
	}

	lower := strings.ToLower(content)

	if strings.Contains(lower, "vorgestern") {
		t := currentTime.AddDate(0, 0, -2)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "yesterday") {
		t := currentTime.AddDate(0, 0, -1)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "vor woche") || strings.Contains(lower, "last week") {
		t := currentTime.AddDate(0, 0, -7)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	if matches := hourPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if hours, err := strconv.Atoi(matches[1]); err == nil && hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	if matches := dayPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if days, err := strconv.Atoi(matches[1]); err == nil && days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	if matches := vorHoursPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if hours, err := strconv.Atoi(matches[1]); err == nil && hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	if matches := vorDaysPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if days, err := strconv.Atoi(matches[1]); err == nil && days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	return nil
}

// inferDates attempts to infer publication dates for search results that lack explicit dates
func inferDates(resp *SearchResponse) {
	now := time.Now()
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			r.DateSource = DateSourceAPI
		} else {
			parsed := parseRelativeDate(r.Content, now)
			if parsed != nil {
				formatted := parsed.Format("2006-01-02")
				r.PublishedDate = &formatted
				r.DateSource = DateSourceInferred
			} else {
				r.DateSource = DateSourceNone
			}
		}
	}
}
