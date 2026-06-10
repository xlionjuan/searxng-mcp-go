package searxng

import (
	"fmt"
	"strconv"
	"strings"
)

// ParamDef describes a single search parameter for use across CLI and MCP layers.
type ParamDef struct {
	// Name is the parameter name (flag name, JSON key).
	Name string

	// GoType is the Go flag type: "string" or "int".
	GoType string

	// Default is the default value as a string.
	Default string

	// Description is a general description used as the flag description in
	// go's flag package and as the JSON Schema description.
	Description string

	// CLIHelp is the help text suffix for the CLI --help output (right-hand
	// column). It should include default indicators where appropriate.
	CLIHelp string

	// CLIType is the human-friendly type indicator for CLI help, e.g.
	// "string", "LANG", "0-2", "N", "RANGE", "CAT", "ENG".
	CLIType string

	// MCPType is the JSON Schema type: "string", "integer", or "boolean".
	MCPType string

	// Nullable indicates that the MCP type allows null (union type).
	Nullable bool

	// Enum is an optional list of valid values for JSON Schema enum.
	Enum []string

	// Minimum is an optional JSON Schema minimum constraint.
	Minimum *int

	// Maximum is an optional JSON Schema maximum constraint.
	Maximum *int

	// Examples is an optional list of example values for JSON Schema examples.
	Examples []string

	// Required indicates whether the parameter is required.
	Required bool
}

var (
	paramMinSafeSearch = MinSafeSearch
	paramMaxSafeSearch = MaxSafeSearch
	paramMinPage       = MinPageno
	paramMinLimit      = MinResultLimit
	paramMaxLimit      = MaxResultLimit
)

// SearchParams is the canonical list of all search parameters used by both CLI
// and MCP layers. Adding or changing a parameter here propagates to flag
// registration, help text, and MCP schema generation automatically.
//
// The Default, Minimum, Maximum, and Enum fields must agree with the runtime
// validators in validation.go. The single source of truth for those
// constraints lives in bounds.go; the drift tests in
// params_validation_drift_test.go verify that schema and validator stay in
// sync.
var SearchParams = []ParamDef{
	{
		Name: "query", GoType: "string", Default: "", Required: true,
		Description: "Search query string",
		CLIHelp:     "Search query string (alternative to positional argument)",
		CLIType:     "string",
		MCPType:     "string",
	},
	{
		Name: "language", GoType: "string", Default: "",
		Description: "Language code for results (e.g., en, zh-tw, ja). Leave empty or pass \"auto\" to let SearXNG decide",
		CLIHelp:     "Language code for results (e.g., en, zh-tw, ja) [default: \"\"]",
		CLIType:     "LANG",
		MCPType:     "string",
	},
	{
		Name: "safesearch", GoType: "int", Default: strconv.Itoa(MinSafeSearch),
		Description: "SafeSearch level: 0=Off, 1=Moderate, 2=Strict",
		CLIHelp:     fmt.Sprintf("SafeSearch level: 0=Off, 1=Moderate, 2=Strict [default: %d]", MinSafeSearch),
		CLIType:     "0-2",
		MCPType:     "integer",
		Minimum:     &paramMinSafeSearch,
		Maximum:     &paramMaxSafeSearch,
	},
	{
		Name: "time_range", GoType: "string", Default: "",
		Description: "Time range filter: " + strings.Join(ValidTimeRanges(), ", "),
		CLIHelp:     "Time range filter: " + strings.Join(ValidTimeRanges(), ", "),
		CLIType:     "RANGE",
		MCPType:     "string",
		// Enum is derived from ValidTimeRanges() plus the empty
		// string, which means "no time restriction" and is short-circuited
		// by validateTimeRange.
		Enum: append([]string{""}, ValidTimeRanges()...),
	},
	{
		Name: "categories", GoType: "string", Default: "",
		Description: `Comma-separated list of SearXNG categories. "general" covers most queries. ` +
			`Other values (it, science, news, map, music, files, social media — note the space) ` +
			`also work but are rarely needed.`,
		CLIHelp:  "Comma-separated list of categories to search [max 4096 bytes]",
		CLIType:  "CAT",
		MCPType:  "string",
		Examples: []string{"general", "images", "videos"},
	},
	{
		Name: "engines", GoType: "string", Default: "",
		Description: `Comma-separated list of SearXNG engine names. Common engines: google, bing, duckduckgo.`,
		CLIHelp:     "Comma-separated list of search engines to use [max 4096 bytes]",
		CLIType:     "ENG",
		MCPType:     "string",
		Examples:    []string{"google", "bing", "duckduckgo"},
	},
	{
		Name: "pageno", GoType: "int", Default: strconv.Itoa(MinPageno),
		Description: "Page number for pagination",
		CLIHelp:     fmt.Sprintf("Page number for pagination [default: %d]", MinPageno),
		CLIType:     "N",
		MCPType:     "integer",
		Nullable:    true,
		Minimum:     &paramMinPage,
	},
	{
		Name: "limit", GoType: "int", Default: strconv.Itoa(DefaultResultLimit),
		Description: fmt.Sprintf("Maximum number of results to return (%d-%d)", MinResultLimit, MaxResultLimit),
		CLIHelp: fmt.Sprintf(
			"Maximum number of results to return (%d-%d) [default: %d]",
			MinResultLimit, MaxResultLimit, DefaultResultLimit),
		CLIType: "N",
		MCPType: "integer",
		Minimum: &paramMinLimit,
		Maximum: &paramMaxLimit,
	},
}
