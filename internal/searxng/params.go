package searxng

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

	// Required indicates whether the parameter is required.
	Required bool
}

var (
	paramMinSafeSearch = 0
	paramMaxSafeSearch = 2
	paramMinPage       = 1
	paramMinLimit      = 1
	paramMaxLimit      = 20
)

// SearchParams is the canonical list of all search parameters used by both CLI
// and MCP layers. Adding or changing a parameter here propagates to flag
// registration, help text, and MCP schema generation automatically.
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
		CLIHelp:     "Language code for results (e.g., en, zh-tw, ja) [default: auto]",
		CLIType:     "LANG",
		MCPType:     "string",
	},
	{
		Name: "safesearch", GoType: "int", Default: "0",
		Description: "SafeSearch level: 0=Off, 1=Moderate, 2=Strict",
		CLIHelp:     "SafeSearch level: 0=Off, 1=Moderate, 2=Strict [default: 0]",
		CLIType:     "0-2",
		MCPType:     "integer",
		Minimum:     &paramMinSafeSearch,
		Maximum:     &paramMaxSafeSearch,
	},
	{
		Name: "time_range", GoType: "string", Default: "",
		Description: "Time range filter: day, month, year",
		CLIHelp:     "Time range filter: day, month, year",
		CLIType:     "RANGE",
		MCPType:     "string",
		// Keep in sync with validTimeRanges in validation.go.
		// The empty string means "no time restriction" and is short-circuited
		// by validateTimeRange, so it intentionally has no entry in that map.
		Enum: []string{"", "day", "month", "year"},
	},
	{
		Name: "categories", GoType: "string", Default: "",
		Description: "Comma-separated list of categories to search (max 4096 bytes for the full string)",
		CLIHelp:     "Comma-separated list of categories to search",
		CLIType:     "CAT",
		MCPType:     "string",
	},
	{
		Name: "engines", GoType: "string", Default: "",
		Description: "Comma-separated list of search engines to use (max 4096 bytes for the full string)",
		CLIHelp:     "Comma-separated list of search engines to use",
		CLIType:     "ENG",
		MCPType:     "string",
	},
	{
		Name: "pageno", GoType: "int", Default: "1",
		Description: "Page number for pagination",
		CLIHelp:     "Page number for pagination [default: 1]",
		CLIType:     "N",
		MCPType:     "integer",
		Nullable:    true,
		Minimum:     &paramMinPage,
	},
	{
		Name: "limit", GoType: "int", Default: "10",
		Description: "Maximum number of results to return (1-20)",
		CLIHelp:     "Maximum number of results to return (1-20) [default: 10]",
		CLIType:     "N",
		MCPType:     "integer",
		Minimum:     &paramMinLimit,
		Maximum:     &paramMaxLimit,
	},
}
