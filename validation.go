package main

import (
	"regexp"
	"strings"
)

// ============================================================================
// Centralized Validation
// ============================================================================

// MaxQueryLength is the maximum allowed length for search queries
const MaxQueryLength = 500

// validTimeRanges contains the set of valid time range values
var validTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// languagePattern validates common BCP47-like language tags used by SearXNG.
// Empty values are handled separately as "auto" mode.
var languagePattern = regexp.MustCompile(`^[\p{L}]{2,35}(?:-[\p{L}\p{N}]{1,35})*$`)

const maxLanguageLength = 35

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

const maxIdentifierLength = 50

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

func isValidCategoryOrEngine(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return false
	}
	if len(trimmed) > maxIdentifierLength {
		return false
	}
	for _, r := range trimmed {
		if r < 32 || r == 127 {
			return false
		}
		if !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-') {
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
		if strings.TrimSpace(cat) == "" {
			return NewValidationError("categories", "contains invalid category")
		}
		if !isValidCategoryOrEngine(cat) {
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
		if strings.TrimSpace(eng) == "" {
			return NewValidationError("engines", "contains invalid engine")
		}
		if !isValidCategoryOrEngine(eng) {
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
		if err := ValidateCategories(args.Categories); err != nil {
			return err
		}
	}

	if args.Engines != "" {
		if err := ValidateEngines(args.Engines); err != nil {
			return err
		}
	}

	if args.Language != "" {
		if len(args.Language) > maxLanguageLength {
			return NewValidationError("language", "must be 35 characters or less")
		}
		if strings.EqualFold(args.Language, "auto") {
			return NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja, en-US)")
		}
		if !languagePattern.MatchString(args.Language) {
			return NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja, en-US)")
		}
	}

	return nil
}
