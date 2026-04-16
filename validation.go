package main

import "strings"

// ============================================================================
// Centralized Validation
// ============================================================================

// MaxQueryLength is the maximum allowed length for search queries
const MaxQueryLength = 500

// validTimeRanges contains the set of valid time range values
var validTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// validLanguages contains the set of valid language codes
var validLanguages = map[string]bool{
	"en": true, "zh": true, "zh-tw": true, "ja": true, "ko": true,
	"fr": true, "de": true, "es": true, "it": true, "pt": true,
	"ru": true, "ar": true, "hi": true, "nl": true, "pl": true,
	"sv": true, "da": true, "fi": true, "no": true, "tr": true,
}

// containsControlCharacters checks if a string contains control characters
// (characters in the range \x00-\x1f and \x7f)
func containsControlCharacters(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

// ValidateSearchArgs validates the search arguments and returns a ValidationError if invalid
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	if strings.TrimSpace(args.Query) == "" {
		return NewValidationError("query", "search query cannot be only whitespace")
	}

	if len(args.Query) > MaxQueryLength {
		return NewValidationError("query", "must be 500 characters or less")
	}

	if containsControlCharacters(args.Query) {
		return NewValidationError("query", "contains invalid control characters")
	}

	if args.TimeRange != "" && !validTimeRanges[args.TimeRange] {
		return NewValidationError("time_range", "must be one of day, month or year")
	}

	if args.SafeSearch < 0 || args.SafeSearch > 2 {
		return NewValidationError("safesearch", "must be 0 off, 1 moderate, or 2 strict")
	}

	if args.Pageno != nil && *args.Pageno < 1 {
		return NewValidationError("pageno", "must be >= 1")
	}

	if args.Categories != "" && containsControlCharacters(args.Categories) {
		return NewValidationError("categories", "contains invalid control characters")
	}

	if args.Engines != "" && containsControlCharacters(args.Engines) {
		return NewValidationError("engines", "contains invalid control characters")
	}

	if args.Language != "" && !validLanguages[args.Language] {
		return NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja)")
	}

	return nil
}
