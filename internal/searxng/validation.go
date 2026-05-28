package searxng

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxQueryLength is the maximum allowed length (in runes) for search queries.
const MaxQueryLength = 500

// validTimeRanges contains the set of valid time range values.
var validTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// languagePattern validates common BCP47-like language tags used by SearXNG.
// Empty values are handled separately as "auto" mode.
var languagePattern = regexp.MustCompile(`^[\p{L}]{2,35}(?:-[\p{L}\p{N}]{1,35})*$`)

const maxLanguageLength = 35

// containsASCIIControlCharacters checks if a string contains ASCII control characters
// (characters in the range \x00-\x1f and \x7f).
func containsASCIIControlCharacters(s string) bool {
	for _, r := range s {
		if r < 32 || r == 127 {
			return true
		}
	}

	return false
}

const maxIdentifierLength = 50

func isValidCategoryOrEngine(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return false
	}

	if utf8.RuneCountInString(trimmed) > maxIdentifierLength {
		return false
	}

	for _, r := range trimmed {
		if r < 32 || r == 127 {
			return false
		}

		if r == '/' || r == '\\' {
			return false
		}
	}

	return true
}

func validateCSVIdentifiers(value, field, noun string) error {
	if value == "" {
		return nil
	}

	// Bound total input length to prevent abuse with multi-megabyte strings.
	const maxCSVInputLength = 4096
	if len(value) > maxCSVInputLength {
		return NewValidationError(field, noun+" input too long")
	}

	for item := range strings.SplitSeq(value, ",") {
		if strings.TrimSpace(item) == "" {
			return NewValidationError(field, "contains invalid "+noun)
		}

		if !isValidCategoryOrEngine(item) {
			return NewValidationError(field, "contains invalid "+noun)
		}
	}

	return nil
}

// ValidateSearchArgs validates and normalizes search arguments, returning a ValidationError if invalid.
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	err := validateQuery(args.Query)
	if err != nil {
		return err
	}

	err = validateTimeRange(args.TimeRange)
	if err != nil {
		return err
	}

	err = validateSafesearch(args.SafeSearch)
	if err != nil {
		return err
	}

	err = validatePagination(args.Pageno, args.Limit)
	if err != nil {
		return err
	}

	err = validateCSVIdentifiers(args.Categories, "categories", "category")
	if err != nil {
		return err
	}

	err = validateCSVIdentifiers(args.Engines, "engines", "engine")
	if err != nil {
		return err
	}

	err = validateLanguage(args)
	if err != nil {
		return err
	}

	return nil
}

func validateQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return NewValidationError("query", "search query cannot be only whitespace")
	}

	if utf8.RuneCountInString(query) > MaxQueryLength {
		return NewValidationError("query", "must be 500 characters or less")
	}

	if containsASCIIControlCharacters(query) {
		return NewValidationError("query", "contains invalid control characters")
	}

	return nil
}

func validateLanguage(args *SearchArgs) error {
	if args.Language == "" {
		return nil
	}

	if strings.EqualFold(args.Language, "auto") {
		args.Language = ""

		return nil
	}

	if utf8.RuneCountInString(args.Language) > maxLanguageLength {
		return NewValidationError("language", "must be 35 characters or less")
	}

	if !languagePattern.MatchString(args.Language) {
		return NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja, en-US)")
	}

	return nil
}

func validatePagination(pageno *int, limit *int) error {
	if pageno != nil && *pageno < 1 {
		return NewValidationError("pageno", "must be >= 1")
	}

	if limit != nil && (*limit < 1 || *limit > 20) {
		return NewValidationError("limit", "must be between 1 and 20")
	}

	return nil
}

func validateTimeRange(timeRange string) error {
	if timeRange != "" && !validTimeRanges[timeRange] {
		return NewValidationError("time_range", "must be one of day, month or year")
	}

	return nil
}

func validateSafesearch(safeSearch int) error {
	if safeSearch < 0 || safeSearch > 2 {
		return NewValidationError("safesearch", "must be 0 off, 1 moderate, or 2 strict")
	}

	return nil
}
