package searxng

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxQueryLength is the maximum allowed length (in runes) for search queries.
var MaxQueryLength = 500

// validTimeRanges is the package-private set-lookup form of the shared
// validTimeRangesList (bounds.go). The empty string is intentionally
// excluded: validateTimeRange short-circuits on "" (no restriction) before
// consulting this map. Keep validTimeRangesList and the time_range ParamDef Enum
// in params.go in sync; the drift test in
// params_validation_drift_test.go enforces this.
var validTimeRanges = func() map[string]bool {
	ranges := ValidTimeRanges()

	m := make(map[string]bool, len(ranges))
	for _, r := range ranges {
		m[r] = true
	}

	return m
}()

var validTimeRangesText = strings.Join(ValidTimeRanges(), ", ")

// languagePattern validates common BCP47-like language tags used by SearXNG.
// It also accepts grandfathered tags (starting with "i-") and private-use
// tags (starting with "x-"). Empty values are handled separately as "auto" mode.
// The regex quantifiers limit individual subtags; validateLanguage applies the
// separate MaxLanguageLength bound to the complete value.
var languagePattern = regexp.MustCompile(`^(?:[ix]-[\p{L}\p{N}_-]{1,35}|[\p{L}]{2,35}(?:-[\p{L}\p{N}]{1,35})*)$`)

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

func validateCSVIdentifiers(value, field, noun string) error {
	if value == "" {
		return nil
	}

	// Bound total UTF-8 byte length to prevent abuse with multi-megabyte strings.
	if len(value) > MaxCSVInputBytes {
		return NewValidationError(field, noun+" input too long")
	}

	for item := range strings.SplitSeq(value, ",") {
		trimmed := strings.TrimSpace(item)
		if containsASCIIControlCharacters(item) {
			return NewValidationError(
				field,
				fmt.Sprintf("control characters (ASCII <32 or 127) not allowed in %s name", noun),
			)
		}

		if trimmed == "" {
			return NewValidationError(field, "empty item in comma-separated list")
		}

		if utf8.RuneCountInString(trimmed) > maxIdentifierLength {
			return NewValidationError(
				field,
				fmt.Sprintf("%s name exceeds %d runes: %q", noun, maxIdentifierLength, trimmed),
			)
		}

		if strings.ContainsAny(trimmed, `/\\`) {
			return NewValidationError(field, fmt.Sprintf("slash not allowed in %s name: %q", noun, trimmed))
		}
	}

	return nil
}

// ValidateSearchArgs validates search arguments and returns a normalized copy
// with Language rewritten (the literal "auto", case-insensitive, becomes ""
// so downstream request building omits the language parameter). The original
// *SearchArgs is never mutated, making this function safe to call with a
// struct shared across goroutines.
func ValidateSearchArgs(args *SearchArgs) (*SearchArgs, error) {
	if args == nil {
		return nil, NewValidationError("args", "search arguments must not be nil")
	}

	// Shallow copy so mutation of the result does not affect the caller.
	result := *args

	// Deep-copy non-nil pointer fields to prevent aliasing the caller's
	// pointers after the function returns.
	if args.Pageno != nil {
		pageno := *args.Pageno
		result.Pageno = &pageno
	}

	if args.Limit != nil {
		limit := *args.Limit
		result.Limit = &limit
	}

	err := validateQuery(result.Query)
	if err != nil {
		return nil, err
	}

	err = validateTimeRange(result.TimeRange)
	if err != nil {
		return nil, err
	}

	err = validateSafesearch(result.SafeSearch)
	if err != nil {
		return nil, err
	}

	err = validatePagination(result.Pageno, result.Limit)
	if err != nil {
		return nil, err
	}

	err = validateCSVIdentifiers(result.Categories, "categories", "category")
	if err != nil {
		return nil, err
	}

	err = validateCSVIdentifiers(result.Engines, "engines", "engine")
	if err != nil {
		return nil, err
	}

	normalizedLang, err := validateLanguage(result.Language)
	if err != nil {
		return nil, err
	}

	result.Language = normalizedLang

	return &result, nil
}

func validateQuery(query string) error {
	if strings.TrimSpace(query) == "" {
		return NewValidationError("query", "search query cannot be only whitespace")
	}

	if utf8.RuneCountInString(query) > MaxQueryLength {
		return NewValidationError("query", fmt.Sprintf("must be %d runes or less", MaxQueryLength))
	}

	if containsASCIIControlCharacters(query) {
		return NewValidationError("query", "contains invalid control characters")
	}

	return nil
}

// validateLanguage validates a language code and returns its normalized form.
// The literal "auto" (case-insensitive) is normalized to the empty string so
// downstream request building can omit the language parameter. The function
// is pure: it does not mutate any caller-visible state.
func validateLanguage(value string) (string, error) {
	if value == "" {
		return "", nil
	}

	if strings.EqualFold(value, "auto") {
		return "", nil
	}

	if utf8.RuneCountInString(value) > MaxLanguageLength {
		return value, NewValidationError(
			"language",
			fmt.Sprintf("must be %d runes or less", MaxLanguageLength),
		)
	}

	if !languagePattern.MatchString(value) {
		return value, NewValidationError("language", "must be a valid language code (e.g., en, zh-tw, ja, en-US)")
	}

	return value, nil
}

func validatePagination(pageno, limit *int) error {
	if pageno != nil && *pageno < MinPageno {
		return NewValidationError("pageno", fmt.Sprintf("must be >= %d", MinPageno))
	}

	if limit != nil && (*limit < MinResultLimit || *limit > MaxResultLimit) {
		return NewValidationError(
			"limit",
			fmt.Sprintf("must be between %d and %d", MinResultLimit, MaxResultLimit),
		)
	}

	return nil
}

func validateTimeRange(timeRange string) error {
	if timeRange != "" && !validTimeRanges[timeRange] {
		return NewValidationError("time_range", "must be one of "+validTimeRangesText)
	}

	return nil
}

func validateSafesearch(safeSearch int) error {
	if safeSearch < MinSafeSearch || safeSearch > MaxSafeSearch {
		return NewValidationError(
			"safesearch",
			fmt.Sprintf("must be %d off, %d moderate, or %d strict", MinSafeSearch, MinSafeSearch+1, MaxSafeSearch),
		)
	}

	return nil
}
