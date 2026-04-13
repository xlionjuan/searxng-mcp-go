package main

// ============================================================================
// Centralized Validation
// ============================================================================

// ValidTimeRanges contains the set of valid time range values
var ValidTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// ValidateSearchArgs validates the search arguments and returns a ValidationError if invalid
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	if args.Query == "" {
		return NewValidationError("query", "search query is required")
	}

	if args.TimeRange != "" && !ValidTimeRanges[args.TimeRange] {
		return NewValidationError("time_range", "must be one of: day, month, year")
	}

	if args.SafeSearch != nil {
		ss := *args.SafeSearch
		if ss < 0 || ss > 2 {
			return NewValidationError("safesearch", "must be 0 (Off), 1 (Moderate), or 2 (Strict)")
		}
	}

	return nil
}
