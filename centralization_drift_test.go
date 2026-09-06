package main

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

// TestCLIHelpTimeoutDefaultDerivesFromConstant guards issue #24: the CLI
// help text for --timeout must show searxng.DefaultTimeout, not a
// hardcoded literal. If a refactor accidentally hardcodes "8s" in the
// help text while the constant moves to a different value, this test
// fails.
func TestCLIHelpTimeoutDefaultDerivesFromConstant(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp(os.Stdout)
	})

	wantSubstr := "[default: " + searxng.DefaultTimeout.String() + "]"
	if !strings.Contains(output, wantSubstr) {
		t.Errorf("CLI help missing %q (searxng.DefaultTimeout = %v)",
			wantSubstr, searxng.DefaultTimeout)
	}

	// Sanity check: a stale hardcoded "8s" should not appear unless the
	// constant actually equals 8s. This catches drift in the opposite
	// direction (hardcoded literal present alongside the derived form).
	if searxng.DefaultTimeout.String() != "8s" && strings.Contains(output, "[default: 8s]") {
		t.Errorf("CLI help shows hardcoded \"[default: 8s]\" but searxng.DefaultTimeout = %v",
			searxng.DefaultTimeout)
	}
}

// TestCLIHelpMaxRetriesDefaultDerivesFromConstant guards issue #24: the
// CLI help text for --max-retries must show searxng.DefaultMaxRetries,
// not a hardcoded literal.
func TestCLIHelpMaxRetriesDefaultDerivesFromConstant(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp(os.Stdout)
	})

	wantSubstr := "[default: " + strconv.Itoa(searxng.DefaultMaxRetries) + "]"
	if !strings.Contains(output, wantSubstr) {
		t.Errorf("CLI help missing %q (searxng.DefaultMaxRetries = %d)",
			wantSubstr, searxng.DefaultMaxRetries)
	}

	if searxng.DefaultMaxRetries != 5 && strings.Contains(output, "[default: 5]") {
		t.Errorf("CLI help shows hardcoded \"[default: 5]\" but searxng.DefaultMaxRetries = %d",
			searxng.DefaultMaxRetries)
	}
}

// TestCLIHelpNoStaleDefaultLiterals is a coarse smoke test that the CLI
// help no longer contains the previously hardcoded "[default: 8s]" and
// "[default: 5]" markers, as long as the underlying constants equal those
// values. If a future refactor reintroduces a hardcoded literal, the
// finer-grained TestCLIHelp*DefaultDerivesFromConstant tests above
// catch the divergence at the constant level.
func TestCLIHelpNoStaleDefaultLiterals(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp(os.Stdout)
	})

	// Both derived and hardcoded forms happen to match when the
	// constants equal 8s/5; this test is a no-op in that case. The
	// point is to fail loudly if the literal reappears next to a
	// different constant value.
	if searxng.DefaultTimeout.String() != "8s" &&
		strings.Contains(output, "[default: 8s]") {
		t.Errorf("CLI help contains stale \"[default: 8s]\" marker")
	}

	if searxng.DefaultMaxRetries != 5 &&
		strings.Contains(output, "[default: 5]") {
		t.Errorf("CLI help contains stale \"[default: 5]\" marker")
	}
}

// TestCLIHelpLimitHelpTextMatchesBounds guards the limit CLIHelp text in
// the SearchParams table. The text is built from MinResultLimit,
// MaxResultLimit, and DefaultResultLimit at table-init time, so this test
// is a smoke check that the wiring still produces a sensible message
// and that the default in the help text agrees with searxng.DefaultResultLimit.
func TestCLIHelpLimitHelpTextMatchesBounds(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp(os.Stdout)
	})

	wantDefaultStr := "[default: " + strconv.Itoa(searxng.DefaultResultLimit) + "]"
	if !strings.Contains(output, wantDefaultStr) {
		t.Errorf("CLI help limit text missing %q", wantDefaultStr)
	}

	wantRangeStr := "(" + strconv.Itoa(searxng.MinResultLimit) + "-" +
		strconv.Itoa(searxng.MaxResultLimit) + ")"
	if !strings.Contains(output, wantRangeStr) {
		t.Errorf("CLI help limit text missing range %q", wantRangeStr)
	}
}

// TestMCPSearchSchemaLimitDefaultDerivesFromConstant guards the MCP JSON
// Schema for the `limit` parameter. The schema's description is built
// from MinResultLimit and MaxResultLimit via fmt.Sprintf, and the
// minimum/maximum constraints are derived from the same shared bounds,
// so the JSON Schema must reflect the bounds.
func TestMCPSearchSchemaLimitDefaultDerivesFromConstant(t *testing.T) {
	t.Parallel()

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	var schema map[string]any

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties has unexpected type %T", schema["properties"])
	}

	rawLimit, ok := props["limit"].(map[string]any)
	if !ok {
		t.Fatal("schema missing limit property")
	}

	desc, ok := rawLimit["description"].(string)
	if !ok {
		t.Fatal("limit description is not a string")
	}

	wantRange := "(" + strconv.Itoa(searxng.MinResultLimit) + "-" +
		strconv.Itoa(searxng.MaxResultLimit) + ")"
	if !strings.Contains(desc, wantRange) {
		t.Errorf("limit description %q missing range %q", desc, wantRange)
	}

	// Schema must declare minimum/maximum equal to the shared bounds.
	verifyJSONNumber(t, rawLimit, "minimum", float64(searxng.MinResultLimit))
	verifyJSONNumber(t, rawLimit, "maximum", float64(searxng.MaxResultLimit))

	// Schema must declare default equal to DefaultResultLimit.
	verifyJSONNumber(t, rawLimit, "default", float64(searxng.DefaultResultLimit))
}

// TestMCPSearchSchemaSafeSearchDerivesFromConstant guards the MCP JSON
// Schema for the `safesearch` parameter.
func TestMCPSearchSchemaSafeSearchDerivesFromConstant(t *testing.T) {
	t.Parallel()

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	var schema map[string]any

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties has unexpected type %T", schema["properties"])
	}

	raw, ok := props["safesearch"].(map[string]any)
	if !ok {
		t.Fatal("schema missing safesearch property")
	}

	verifyJSONNumber(t, raw, "minimum", float64(searxng.MinSafeSearch))
	verifyJSONNumber(t, raw, "maximum", float64(searxng.MaxSafeSearch))
	verifyJSONNumber(t, raw, "default", float64(searxng.MinSafeSearch))
}

// TestMCPSearchSchemaPagenoDerivesFromConstant guards the MCP JSON Schema
// for the `pageno` parameter.
func TestMCPSearchSchemaPagenoDerivesFromConstant(t *testing.T) {
	t.Parallel()

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	var schema map[string]any

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties has unexpected type %T", schema["properties"])
	}

	raw, ok := props["pageno"].(map[string]any)
	if !ok {
		t.Fatal("schema missing pageno property")
	}

	verifyJSONNumber(t, raw, "minimum", float64(searxng.MinPageno))
	verifyJSONNumber(t, raw, "default", float64(searxng.MinPageno))
}

// TestMCPSearchSchemaLanguageLengthDerivesFromConstant guards the MCP JSON
// Schema for the language parameter. The maximum applies to the complete
// language value, while the validator keeps per-subtag grammar limits separate.
func TestMCPSearchSchemaLanguageLengthDerivesFromConstant(t *testing.T) {
	t.Parallel()

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	var schema map[string]any

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties has unexpected type %T", schema["properties"])
	}

	rawLanguage, ok := props["language"].(map[string]any)
	if !ok {
		t.Fatal("schema missing language property")
	}

	verifyJSONNumber(t, rawLanguage, "maxLength", float64(searxng.MaxLanguageLength))

	description, ok := rawLanguage["description"].(string)
	if !ok {
		t.Fatal("language description is not a string")
	}

	wantDescription := "Maximum " + strconv.Itoa(searxng.MaxLanguageLength) + " runes for the full value"
	if !strings.Contains(description, wantDescription) {
		t.Errorf("language description %q missing %q", description, wantDescription)
	}
}

// TestCLIHelpLanguageLengthDerivesFromConstant guards the CLI help text for
// the language parameter against a stale hard-coded total-rune limit.
func TestCLIHelpLanguageLengthDerivesFromConstant(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp(os.Stdout)
	})

	want := "[max " + strconv.Itoa(searxng.MaxLanguageLength) + " runes]"
	if !strings.Contains(output, want) {
		t.Errorf("CLI help language text missing %q", want)
	}
}

// TestParseArgsDefaultLimitMatchesConstant guards the CLI defaulting policy:
// when --limit is not provided, parseArgs must default to DefaultResultLimit.
// If the CLI path diverges from the MCP path (which uses ApplyDefaults /
// NewSearchArgs), this test catches the drift.
func TestParseArgsDefaultLimitMatchesConstant(t *testing.T) {
	t.Parallel()

	_, flags, _, err := parseArgs([]string{"test query"})
	if err != nil {
		t.Fatalf("parseArgs() error = %v", err)
	}

	if flags.Limit == nil {
		t.Fatal("parseArgs left Limit = nil, want non-nil default")
	}

	if *flags.Limit != searxng.DefaultResultLimit {
		t.Fatalf("parseArgs Limit = %d, want %d (DefaultResultLimit)",
			*flags.Limit, searxng.DefaultResultLimit)
	}
}

// TestMCPSearchSchemaTimeRangeEnumDerivesFromConstant guards the MCP
// JSON Schema enum for `time_range`. The Enum field in SearchParams is
// derived from ValidTimeRanges(), so the schema must reflect that.
func TestMCPSearchSchemaTimeRangeEnumDerivesFromConstant(t *testing.T) {
	t.Parallel()

	data, err := buildSearchSchema()
	if err != nil {
		t.Fatalf("buildSearchSchema() error = %v", err)
	}

	var schema map[string]any

	err = json.Unmarshal(data, &schema)
	if err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("schema properties has unexpected type %T", schema["properties"])
	}

	raw, ok := props["time_range"].(map[string]any)
	if !ok {
		t.Fatal("schema missing time_range property")
	}

	rawEnum, ok := raw["enum"].([]any)
	if !ok {
		t.Fatalf("time_range enum has unexpected type %T", raw["enum"])
	}

	want := append([]string{""}, searxng.ValidTimeRanges()...)

	if len(rawEnum) != len(want) {
		t.Fatalf("time_range enum length = %d, want %d (got=%v, want=%v)",
			len(rawEnum), len(want), rawEnum, want)
	}

	for i, v := range want {
		if rawEnum[i] != v {
			t.Errorf("time_range enum[%d] = %v, want %q (full got=%v, want=%v)",
				i, rawEnum[i], v, rawEnum, want)
		}
	}
}

// verifyJSONNumber checks that the named field in raw is a JSON number
// equal to want. It is used to assert that minimum/maximum constraints
// in the JSON Schema match the shared bounds.
func verifyJSONNumber(t *testing.T, raw map[string]any, field string, want float64) {
	t.Helper()

	v, ok := raw[field]
	if !ok {
		t.Errorf("schema property missing %q field", field)

		return
	}

	num, ok := v.(float64)
	if !ok {
		t.Errorf("schema %q = %v (%T), want JSON number", field, v, v)

		return
	}

	if num != want {
		t.Errorf("schema %q = %v, want %v", field, num, want)
	}
}
