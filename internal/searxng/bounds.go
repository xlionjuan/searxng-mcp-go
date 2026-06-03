package searxng

// Search-parameter bounds shared by the schema metadata in params.go and the
// runtime validators in validation.go. Keeping these in one place prevents
// schema/validator drift when a default or limit changes: both the JSON
// Schema generated for MCP and the ValidateSearchArgs checks read from the
// same source of truth.
//
// Adding a new bounded parameter? Define the bounds here and reference them
// from both ParamDef (schema side) and the matching validate* function
// (runtime side). The drift tests in params_validation_drift_test.go fail if
// a schema entry declares a Minimum or Maximum that disagrees with these
// values, or if a runtime validator hardcodes a different bound.

// SafeSearch bounds: 0 (Off) / 1 (Moderate) / 2 (Strict).
const (
	MinSafeSearch = 0
	MaxSafeSearch = 2
)

// Pageno bounds: page numbers start at 1.
const (
	MinPageno = 1
)

// ResultLimit bounds: inclusive minimum and maximum number of results.
const (
	MinResultLimit = 1
	MaxResultLimit = 20
)

// validTimeRangesList is the canonical list of time-range values accepted by the
// time_range parameter. The empty string is handled separately as "no
// restriction" by both schema and validation, so it intentionally has no
// entry here. Keep this list in sync with the time_range ParamDef's Enum
// in params.go and with validTimeRanges in validation.go.
var validTimeRangesList = []string{"day", "month", "year"}

// ValidTimeRanges returns the canonical list of time-range values accepted by
// the time_range parameter. The returned slice is a copy and can be safely
// appended to or modified by callers without mutating package-level state.
func ValidTimeRanges() []string {
	return append([]string(nil), validTimeRangesList...)
}
