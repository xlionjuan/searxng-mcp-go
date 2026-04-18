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

// Y2K_THRESHOLD is the cutoff year for detecting ambiguous 2-digit years.
// 32-bit signed int overflow occurs in 2038, so this needs to be updated before then.
const Y2K_THRESHOLD = 2000

// Package-level regex patterns (compiled once)
var (
	hourPattern = mustCompile(`(?i)(\d+)\s*(hour|hours|h|stunde|stunden)\s*(ago|vor)?`)
	dayPattern  = mustCompile(`(?i)(\d+)\s*(day|days|d|tag|tagen)\s*(ago|vor)?`)
)

// mustCompile wraps regexp.Compile and panics on error for clarity
func mustCompile(pattern string) *regexp.Regexp {
	re, err := regexp.Compile(pattern)
	if err != nil {
		panic("invalid regex: " + pattern)
	}
	return re
}

// asciiCaseInsensitiveContains performs case-insensitive substring search
// for ASCII-only patterns without allocating (no strings.ToLower).
func asciiCaseInsensitiveContains(s, substr string) bool {
	if substr == "" {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	subLen := len(substr)
	for i := 0; i <= len(s)-subLen; i++ {
		found := true
		for j := 0; j < subLen; j++ {
			sc := s[i+j]
			pc := substr[j]
			if sc != pc {
				if sc >= 'A' && sc <= 'Z' {
					sc += 32
				} else if sc >= 'a' && sc <= 'z' {
					sc -= 32
				} else {
					found = false
					break
				}
				if sc != pc {
					found = false
					break
				}
			}
		}
		if found {
			return true
		}
	}
	return false
}

// mayContainRelativeDate checks for common relative-date keywords (case-insensitive).
// This fast path avoids expensive ToLower + regex when content clearly has no date hint.
func mayContainRelativeDate(content string) bool {
	if content == "" {
		return false
	}
	return asciiCaseInsensitiveContains(content, "ago") ||
		asciiCaseInsensitiveContains(content, "yesterday") ||
		asciiCaseInsensitiveContains(content, "last week") ||
		asciiCaseInsensitiveContains(content, "vor") ||
		asciiCaseInsensitiveContains(content, "vorgestern") ||
		asciiCaseInsensitiveContains(content, "stunde") ||
		asciiCaseInsensitiveContains(content, "stunden") ||
		asciiCaseInsensitiveContains(content, "tag") ||
		asciiCaseInsensitiveContains(content, "tagen")
}

func parseRelativeDate(content string, currentTime time.Time) *time.Time {
	if content == "" {
		return nil
	}

	// Fast path: skip ToLower + regex if no relative-date keyword is present
	if !mayContainRelativeDate(content) {
		return nil
	}

	lower := strings.ToLower(content)

	if strings.Contains(lower, "vorgestern") {
		t := currentTime.AddDate(0, 0, -2)
		if t.Year() < Y2K_THRESHOLD {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "yesterday") {
		t := currentTime.AddDate(0, 0, -1)
		if t.Year() < Y2K_THRESHOLD {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "vor woche") || strings.Contains(lower, "last week") {
		t := currentTime.AddDate(0, 0, -7)
		if t.Year() < Y2K_THRESHOLD {
			return nil
		}
		return &t
	}

	if matches := hourPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if hours, err := strconv.Atoi(matches[1]); err == nil && hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < Y2K_THRESHOLD {
				return nil
			}
			return &t
		}
	}

	if matches := dayPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		if days, err := strconv.Atoi(matches[1]); err == nil && days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < Y2K_THRESHOLD {
				return nil
			}
			return &t
		}
	}

	return nil
}

// inferDates attempts to infer publication dates for search results that lack explicit dates
// If currentTime is nil, time.Now() is used.
func inferDates(resp *SearchResponse, currentTime *time.Time) {
	if resp == nil {
		return
	}
	if currentTime == nil {
		now := time.Now()
		currentTime = &now
	}
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			r.DateSource = DateSourceAPI
		} else {
			parsed := parseRelativeDate(r.Content, *currentTime)
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
