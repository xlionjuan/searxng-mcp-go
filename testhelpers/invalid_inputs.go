package testhelpers

import (
	"maps"
	"strings"

	"searxng-mcp-go/internal/searxng"
)

// InvalidInputCase represents a test case for invalid input validation.
type InvalidInputCase struct {
	Name          string
	Arguments     map[string]any
	WantField     string
	WantSchemaErr bool // true if the error should come from JSON Schema validation
}

var baseArgs = map[string]any{"query": "framework computer inc"}

// MCPInvalidInputCases returns the standard set of invalid input test cases
// for MCP handler validation. These are the exhaustive cases that test both
// schema-level and handler-level validation.
func MCPInvalidInputCases() []InvalidInputCase {
	return []InvalidInputCase{
		{Name: "whitespace query", Arguments: map[string]any{"query": "   "}, WantField: "query"},
		{Name: "control characters in query", Arguments: map[string]any{"query": "golang\x00search"}, WantField: "query"},
		{Name: "long query", Arguments: map[string]any{"query": strings.Repeat("a", searxng.MaxQueryLength+1)}, WantField: "query"},
		{Name: "limit too high", Arguments: mergedLimitTooHigh(), WantField: "limit", WantSchemaErr: true},
		{Name: "limit too low", Arguments: mergedArgs(map[string]any{"limit": 0}), WantField: "limit", WantSchemaErr: true},
		{Name: "pageno too low", Arguments: mergedArgs(map[string]any{"pageno": 0}), WantField: "pageno", WantSchemaErr: true},
		{Name: "invalid time range", Arguments: mergedArgs(map[string]any{"time_range": "week"}), WantField: "time_range", WantSchemaErr: true},
		{Name: "invalid safesearch", Arguments: mergedSafesearchTooHigh(), WantField: "safesearch", WantSchemaErr: true},
		{Name: "safesearch negative", Arguments: mergedArgs(map[string]any{"safesearch": -1}), WantField: "safesearch", WantSchemaErr: true},
		{Name: "invalid language", Arguments: mergedArgs(map[string]any{"language": "not a valid language code"}), WantField: "language"},
		{Name: "invalid categories", Arguments: mergedArgs(map[string]any{"categories": "general/../../x"}), WantField: "categories"},
		{Name: "invalid engines", Arguments: mergedArgs(map[string]any{"engines": "bing/../../x"}), WantField: "engines"},
	}
}

// IncorrectParamTypeCases returns test cases for incorrect parameter types
// (e.g., string instead of int). These are schema-level validation errors.
func IncorrectParamTypeCases() []InvalidInputCase {
	return []InvalidInputCase{
		{Name: "wrong type limit", Arguments: mergedArgs(map[string]any{"limit": "twenty"}), WantField: "limit", WantSchemaErr: true},
		{Name: "wrong type safesearch", Arguments: mergedArgs(map[string]any{"safesearch": "two"}), WantField: "safesearch", WantSchemaErr: true},
		{Name: "unexpected parameter", Arguments: mergedUnexpectedParam(), WantField: "unknown_param", WantSchemaErr: true},
	}
}

// mergedLimitTooHigh returns args with limit exceeding MaxResultLimit.
func mergedLimitTooHigh() map[string]any {
	return mergedArgs(map[string]any{"limit": searxng.MaxResultLimit + 1})
}

// mergedSafesearchTooHigh returns args with safesearch exceeding MaxSafeSearch.
func mergedSafesearchTooHigh() map[string]any {
	return mergedArgs(map[string]any{"safesearch": searxng.MaxSafeSearch + 1})
}

// mergedUnexpectedParam returns args with an unexpected parameter.
func mergedUnexpectedParam() map[string]any {
	return mergedArgs(map[string]any{"unknown_param": "value"})
}

// mergedArgs merges baseArgs with the given overrides.
func mergedArgs(overrides map[string]any) map[string]any {
	result := maps.Clone(baseArgs)
	maps.Copy(result, overrides)

	return result
}
