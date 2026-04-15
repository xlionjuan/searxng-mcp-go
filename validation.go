package main

// ============================================================================
// Centralized Validation
// ============================================================================

// MaxQueryLength is the maximum allowed length for search queries
const MaxQueryLength = 500

// validTimeRanges contains the set of valid time range values
var validTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// ValidateSearchArgs validates the search arguments and returns a ValidationError if invalid
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	if args.Query == "" {
		return NewValidationError("query", "search query is required")
	}

	if len(args.Query) > MaxQueryLength {
		return NewValidationError("query", "must be 500 characters or less")
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

	return nil
}
