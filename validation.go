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

// validCategories contains the set of valid category values
var validCategories = map[string]bool{
	"general": true, "news": true, "music": true,
}

// validEngines contains the set of valid engine values
var validEngines = map[string]bool{
	"google": true, "bing": true, "duckduckgo": true,
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

// isValidIdentifier checks if a string is non-empty, trim-empty, alphanumeric, and valid
func isValidIdentifier(value string, validSet map[string]bool) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return false
	}
	if !validSet[trimmed] {
		return false
	}
	for _, r := range trimmed {
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

// ValidateCategories validates a comma-separated list of categories
func ValidateCategories(categories string) error {
	if categories == "" {
		return nil
	}
	for _, cat := range strings.Split(categories, ",") {
		if !isValidIdentifier(cat, validCategories) {
			return NewValidationError("categories", "contains invalid category")
		}
	}
	return nil
}

// ValidateEngines validates a comma-separated list of engines
func ValidateEngines(engines string) error {
	if engines == "" {
		return nil
	}
	for _, eng := range strings.Split(engines, ",") {
		if !isValidIdentifier(eng, validEngines) {
			return NewValidationError("engines", "contains invalid engine")
		}
	}
	return nil
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

	if args.Categories != "" {
		if containsControlCharacters(args.Categories) {
			return NewValidationError("categories", "contains invalid control characters")
		}
		if err := ValidateCategories(args.Categories); err != nil {
			return err
		}
	}

	if args.Engines != "" {
		if containsControlCharacters(args.Engines) {
			return NewValidationError("engines", "contains invalid control characters")
		}
		if err := ValidateEngines(args.Engines); err != nil {
			return err
		}
	}

	if args.Language != "" && !validLanguages[args.Language] {
		return NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja)")
	}

	return nil
}
