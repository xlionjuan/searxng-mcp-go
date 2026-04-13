package main

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ============================================================================
// Date Parsing
// ============================================================================

// Regex patterns for relative date parsing
var (
	daysAgoRegex        = regexp.MustCompile(`(\d+)\s*(day|days|d|tag|tagen)\s*(ago|vor)?`)
	weeksAgoRegex       = regexp.MustCompile(`(\d+)\s*(week|weeks|w|woche|wochen)\s*(ago|vor)?`)
	germanWeeksAgoRegex = regexp.MustCompile(`vor\s+(\d+)\s*(woche|wochen)\b`)
	germanDaysAgoRegex  = regexp.MustCompile(`vor\s+(\d+)\s*(tag|tagen)\b`)
	germanHoursAgoRegex = regexp.MustCompile(`vor\s+(\d+)\s*(stunde|stunden)\b`)
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

	hourPattern := regexp.MustCompile(`(\d+)\s*(hour|hours|h|stunde|stunden)\s*(ago|vor)?`)
	if matches := hourPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		hours := 0
		fmt.Sscanf(matches[1], "%d", &hours)
		if hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	dayPattern := regexp.MustCompile(`(\d+)\s*(day|days|d|tag|tagen)\s*(ago|vor)?`)
	if matches := dayPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		days := 0
		fmt.Sscanf(matches[1], "%d", &days)
		if days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	vorHoursPattern := regexp.MustCompile(`vor\s+(\d+)\s*(stunde|stunden)\b`)
	if matches := vorHoursPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		hours := 0
		fmt.Sscanf(matches[1], "%d", &hours)
		if hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	vorDaysPattern := regexp.MustCompile(`vor\s+(\d+)\s*(tag|tagen)\b`)
	if matches := vorDaysPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		days := 0
		fmt.Sscanf(matches[1], "%d", &days)
		if days > 0 && days <= 365 {
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
