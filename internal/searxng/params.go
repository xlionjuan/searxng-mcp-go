package searxng

import (
	"errors"
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

	// MaxLength is an optional JSON Schema maxLength constraint for strings.
	MaxLength *int

	// Examples is an optional list of example values for JSON Schema examples.
	Examples []string

	// DefaultInt is the typed integer default for the JSON Schema "default" keyword.
	// Populate this when the GoType is "int" and a default exists.
	DefaultInt *int

	// Required indicates whether the parameter is required.
	Required bool
}

var (
	paramMinSafeSearch   = MinSafeSearch
	paramMaxSafeSearch   = MaxSafeSearch
	paramMinPage         = MinPageno
	paramMinLimit        = MinResultLimit
	paramMaxLimit        = MaxResultLimit
	paramDefaultLimit    = DefaultResultLimit
	paramMaxQueryLength  = MaxQueryLength
)

// errUnexpectedGoType is a sentinel error for FlagDefault when a ParamDef
// declares an unsupported GoType.
var errUnexpectedGoType = errors.New("unexpected GoType")

// cliHelpPadding is the minimum width reserved for the flag expression
// column in CLI help output, matching the CLI consumer's expectation.
const cliHelpPadding = 18

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
		MaxLength:   &paramMaxQueryLength,
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
		DefaultInt:  &paramMinSafeSearch,
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
		DefaultInt:  &paramMinPage,
		Minimum:     &paramMinPage,
	},
	{
		Name: "limit", GoType: "int", Default: strconv.Itoa(DefaultResultLimit),
		Description: fmt.Sprintf("Maximum number of results to return (%d-%d)", MinResultLimit, MaxResultLimit),
		CLIHelp: fmt.Sprintf(
			"Maximum number of results to return (%d-%d) [default: %d]",
			MinResultLimit, MaxResultLimit, DefaultResultLimit),
		CLIType:    "N",
		MCPType:    "integer",
		DefaultInt: &paramDefaultLimit,
		Minimum:    &paramMinLimit,
		Maximum:    &paramMaxLimit,
	},
}

// JSONSchema returns the JSON Schema property map for this parameter.
func (p ParamDef) JSONSchema() map[string]any {
	prop := map[string]any{
		"type": p.MCPType,
	}

	if p.Description != "" {
		prop["description"] = p.Description
	}

	if p.Enum != nil {
		enum := make([]string, len(p.Enum))
		copy(enum, p.Enum)
		prop["enum"] = enum
	}

	if p.Minimum != nil {
		prop["minimum"] = *p.Minimum
	}

	if p.Maximum != nil {
		prop["maximum"] = *p.Maximum
	}

	if p.MaxLength != nil {
		prop["maxLength"] = *p.MaxLength
	}

	if len(p.Examples) > 0 {
		examples := make([]string, len(p.Examples))
		copy(examples, p.Examples)
		prop["examples"] = examples
	}

	if p.DefaultInt != nil {
		prop["default"] = *p.DefaultInt
	}

	if p.Nullable {
		prop["type"] = []string{"null", p.MCPType}
	}

	return prop
}

// FlagDefault returns the parsed default value for use with Go's flag package.
// The returned type matches the ParamDef.GoType: string or int.
// An unparseable int default is treated as a programming error and reported.
func (p ParamDef) FlagDefault() (any, error) {
	switch p.GoType {
	case "string":
		return p.Default, nil
	case "int":
		return strconv.Atoi(p.Default)
	default:
		return nil, fmt.Errorf("%w %q for param %q", errUnexpectedGoType, p.GoType, p.Name)
	}
}

// CLIHelpLine returns the formatted CLI help line for this parameter.
func (p ParamDef) CLIHelpLine() string {
	flagExpr := "--" + p.Name + " " + p.CLIType
	padding := max(cliHelpPadding-len(flagExpr), 1)

	return fmt.Sprintf("  %s%s%s", flagExpr, strings.Repeat(" ", padding), p.CLIHelp)
}
