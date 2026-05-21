package searxng

import (
	"regexp"
	"strings"
)

// MaxQueryLength is the maximum allowed length for search queries.
const MaxQueryLength = 500

// validTimeRanges contains the set of valid time range values.
var validTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// languagePattern validates common BCP47-like language tags used by SearXNG.
// Empty values are handled separately as "auto" mode.
var languagePattern = regexp.MustCompile(`^[\p{L}]{2,35}(?:-[\p{L}\p{N}]{1,35})*$`)

const maxLanguageLength = 35

// containsControlCharacters checks if a string contains control characters
// (characters in the range \x00-\x1f and \x7f).
func containsControlCharacters(s string) bool {
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

	if len(trimmed) > maxIdentifierLength {
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

// ValidateCategories validates a comma-separated list of categories.
func ValidateCategories(categories string) error {
	if categories == "" {
		return nil
	}

	for cat := range strings.SplitSeq(categories, ",") {
		if strings.TrimSpace(cat) == "" {
			return NewValidationError("categories", "contains invalid category")
		}

		if !isValidCategoryOrEngine(cat) {
			return NewValidationError("categories", "contains invalid category")
		}
	}

	return nil
}

// ValidateEngines validates a comma-separated list of engines.
func ValidateEngines(engines string) error {
	if engines == "" {
		return nil
	}

	for eng := range strings.SplitSeq(engines, ",") {
		if strings.TrimSpace(eng) == "" {
			return NewValidationError("engines", "contains invalid engine")
		}

		if !isValidCategoryOrEngine(eng) {
			return NewValidationError("engines", "contains invalid engine")
		}
	}

	return nil
}

// ValidateSearchArgs validates and normalizes search arguments, returning a ValidationError if invalid.
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	if err := validateQuery(args.Query); err != nil {
		return err
	}

	if err := validateTimeRange(args.TimeRange); err != nil {
		return err
	}

	if err := validateSafesearch(args.SafeSearch); err != nil {
		return err
	}

	if err := validatePagination(args.Pageno, args.Limit); err != nil {
		return err
	}

	if err := validateCategories(args.Categories); err != nil {
		return err
	}

	if err := validateEngines(args.Engines); err != nil {
		return err
	}

	if err := validateLanguage(args); err != nil {
		return err
	}

	return nil
}

func validateQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return NewValidationError("query", "search query cannot be only whitespace")
	}

	if len(query) > MaxQueryLength {
		return NewValidationError("query", "must be 500 characters or less")
	}

	if containsControlCharacters(query) {
		return NewValidationError("query", "contains invalid control characters")
	}

	return nil
}

func validateCategories(categories string) error {
	if categories == "" {
		return nil
	}

	return ValidateCategories(categories)
}

func validateEngines(engines string) error {
	if engines == "" {
		return nil
	}

	return ValidateEngines(engines)
}

func validateLanguage(args *SearchArgs) error {
	if args.Language == "" {
		return nil
	}

	if strings.EqualFold(args.Language, "auto") {
		args.Language = ""

		return nil
	}

	if len(args.Language) > maxLanguageLength {
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
