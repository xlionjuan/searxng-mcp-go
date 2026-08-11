package searxng

// Search-parameter bounds shared by the schema metadata in params.go and the
// runtime validators in validation.go. These are mutable variables rather
// than constants so that ParamDef fields in params.go can reference them by
// pointer (&MinSafeSearch, &MaxSafeSearch, etc.), making schema/validator
// consistency structurally guaranteed.
//
// Adding a new bounded parameter? Define the bounds here and reference them
// from both ParamDef (schema side) and the matching validate* function
// (runtime side).

// SafeSearch bounds: 0 (Off) / 1 (Moderate) / 2 (Strict).
var (
	MinSafeSearch = 0
	MaxSafeSearch = 2
)

// MinPageno is the minimum (and default) page number.
var MinPageno = 1

// ResultLimit bounds: inclusive minimum and maximum number of results.
var (
	MinResultLimit = 1
	MaxResultLimit = 20
)

// MaxCSVInputBytes is the maximum UTF-8 byte length accepted for the
// comma-separated categories and engines parameters. JSON Schema maxLength is
// intentionally not used for this bound because it does not express a
// UTF-8 byte limit.
const MaxCSVInputBytes = 4096

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
