//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

// =============================================================================
// Session lifecycle helpers
// =============================================================================

// validMCPInitialize is a minimal valid MCP initialize message to pass
// the stdin validation gate so we reach config validation.
const validMCPInitialize = `{"jsonrpc":"2.0","method":"initialize"}` + "\n"

// buildE2EMCPBinary compiles the binary and returns its path.
func buildE2EMCPBinary(ctx context.Context, t *testing.T) string {
	t.Helper()

	binaryPath := t.TempDir() + "/searxng-mcp-go"
	cmd := exec.CommandContext(ctx, "go", "build", "-o", binaryPath, ".")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build fallback MCP binary failed: %v\noutput:\n%s", err, string(output))
	}

	return binaryPath
}

// e2eMCPEnv builds the environment variables for an MCP stdio session.
func e2eMCPEnv(searxngURL string, extra ...string) []string {
	env := append(os.Environ(), "SEARXNG_URL="+searxngURL, "SEARXNG_MAX_RETRIES=2")
	env = append(env, extra...)

	return env
}

// removeEnv returns a copy of env with the variable named key removed.
func removeEnv(env []string, key string) []string {
	var result []string
	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}
	return result
}

// newMCPSession creates an MCP client, connects over stdio, and returns the
// session. The caller is responsible for cleanup.
func newMCPSession(ctx context.Context, t *testing.T, cmd *exec.Cmd, stderr *bytes.Buffer, clientName string) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    clientName,
		Version: version,
	}, nil)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	return session
}

// startMCPSession starts an MCP stdio session with shared lifecycle. It
// resolves the binary path, builds the command, connects, and registers
// cleanup. Returns the session, stderr buffer, and command (for optional
// post-cleanup inspection).
//
// This helper uses the client name "searxng-mcp-go-e2e-test".
func startMCPSession(ctx context.Context, t *testing.T, searxngURL string, extraEnv ...string) (*mcp.ClientSession, *bytes.Buffer, *exec.Cmd) {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using MCP binary: %s", binaryPath)

	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL, extraEnv...)
	cmd.Stderr = &stderr

	var session *mcp.ClientSession
	t.Cleanup(func() {
		if session != nil {
			if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
				t.Logf("close MCP session: %v\nstderr:\n%s", closeErr, stderr.String())
			}
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	session = newMCPSession(ctx, t, cmd, &stderr, "searxng-mcp-go-e2e-test")
	t.Log("MCP stdio session connected")

	return session, &stderr, cmd
}

// =============================================================================
// Tool call helpers
// =============================================================================

// findSearchTool lists tools and returns the "search" tool, verifying its schema.
func findSearchTool(ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr *bytes.Buffer) *mcp.Tool {
	t.Helper()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Logf("tools/list returned %d tools", len(tools.Tools))

	for _, tool := range tools.Tools {
		t.Logf("  tool: %s - %s", tool.Name, tool.Description)
		if tool.Name == "search" {
			requireSearchToolSchema(t, tool, stderr)

			return tool
		}
	}

	t.Fatalf("tools/list did not include search tool; got %#v\nstderr:\n%s", tools.Tools, stderr.String())
	panic("unreachable")
}

// callSearchTool calls the search tool with the given arguments.
func callSearchTool(
	ctx context.Context,
	t *testing.T,
	session *mcp.ClientSession,
	arguments map[string]any,
	stderr *bytes.Buffer,
) *mcp.CallToolResult {
	t.Helper()

	result, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search",
		Arguments: arguments,
	})
	if err != nil {
		t.Fatalf("tools/call search failed with arguments %#v: %v\nstderr:\n%s", arguments, err, stderr.String())
	}

	return result
}

// requireSearchResponse calls the search tool, asserts no tool error, parses
// the JSON response, and returns it. name is used for logging.
func requireSearchResponse(
	ctx context.Context,
	t *testing.T,
	session *mcp.ClientSession,
	arguments map[string]any,
	stderr *bytes.Buffer,
	name string,
) searxng.SearchResponse {
	t.Helper()

	t.Logf("%s: sending arguments %#v", name, arguments)
	result := callSearchTool(ctx, t, session, arguments, stderr)
	if result.IsError {
		t.Fatalf("%s returned tool error: %s\nstderr:\n%s", name, toolText(t, result), stderr.String())
	}

	response := parseSearchResponse(t, result, stderr)
	t.Logf("%s parsed: query=%q, results=%d, answers=%d, infoboxes=%d, suggestions=%d",
		name, response.Query, len(response.Results), len(response.Answers), len(response.Infoboxes), len(response.Suggestions))

	return response
}

// parseSearchResponse unmarshals the tool result text into a SearchResponse.
func parseSearchResponse(t *testing.T, result *mcp.CallToolResult, stderr *bytes.Buffer) searxng.SearchResponse {
	t.Helper()

	text := toolText(t, result)

	var response searxng.SearchResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("search tool text is not SearchResponse JSON: %v\ntext:\n%s\nstderr:\n%s", err, text, stderr.String())
	}

	return response
}

// toolText extracts the text content from a tool result, failing if empty.
func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	text, ok := toolTextFromResult(result)
	if !ok {
		t.Fatal("tool result has no content")
	}

	return text
}

// toolTextFromResult extracts text content from a tool result, returning
// false if content is missing or not text.
func toolTextFromResult(result *mcp.CallToolResult) (string, bool) {
	if len(result.Content) == 0 {
		return "", false
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return "", false
	}

	return textContent.Text, true
}

// =============================================================================
// Schema assertion helpers
// =============================================================================

// requireSearchToolSchema verifies the search tool's JSON Schema constraints.
func requireSearchToolSchema(t *testing.T, tool *mcp.Tool, stderr *bytes.Buffer) {
	t.Helper()

	schema := requireSchemaMap(t, tool.InputSchema, stderr)

	if got := schema["type"]; got != "object" {
		t.Fatalf("search schema type = %#v, want object\nschema: %#v\nstderr:\n%s", got, schema, stderr.String())
	}
	if got := schema["additionalProperties"]; got != false {
		t.Fatalf("search schema additionalProperties = %#v, want false\nschema: %#v\nstderr:\n%s", got, schema, stderr.String())
	}

	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("search schema required type = %T, want []any\nschema: %#v\nstderr:\n%s", schema["required"], schema, stderr.String())
	}
	if len(required) != 1 || required[0] != "query" {
		t.Fatalf("search schema required = %#v, want [query]\nschema: %#v\nstderr:\n%s", required, schema, stderr.String())
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("search schema properties type = %T, want map[string]any\nschema: %#v\nstderr:\n%s", schema["properties"], schema, stderr.String())
	}

	limit := requireProperty(t, props, "limit", stderr)
	requirePropertyType(t, limit, "integer", stderr)
	requireNumber(t, limit, "minimum", float64(searxng.MinResultLimit), stderr)
	requireNumber(t, limit, "maximum", float64(searxng.MaxResultLimit), stderr)

	safesearch := requireProperty(t, props, "safesearch", stderr)
	requirePropertyType(t, safesearch, "integer", stderr)
	requireNumber(t, safesearch, "minimum", float64(searxng.MinSafeSearch), stderr)
	requireNumber(t, safesearch, "maximum", float64(searxng.MaxSafeSearch), stderr)

	pageno := requireProperty(t, props, "pageno", stderr)
	requirePropertyUnionType(t, pageno, []string{"null", "integer"}, stderr)
	requireNumber(t, pageno, "minimum", float64(searxng.MinPageno), stderr)

	timeRange := requireProperty(t, props, "time_range", stderr)
	requireStringEnum(t, timeRange, "enum", append([]string{""}, searxng.ValidTimeRanges()...), stderr)
}

func requireSchemaMap(t *testing.T, schema any, stderr *bytes.Buffer) map[string]any {
	t.Helper()

	if schemaMap, ok := schema.(map[string]any); ok {
		return schemaMap
	}

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal InputSchema failed: %v\nschema type: %T\nstderr:\n%s", err, schema, stderr.String())
	}

	var schemaMap map[string]any
	if err := json.Unmarshal(data, &schemaMap); err != nil {
		t.Fatalf("unmarshal InputSchema failed: %v\nschema JSON: %s\nstderr:\n%s", err, string(data), stderr.String())
	}

	return schemaMap
}

func requireProperty(t *testing.T, props map[string]any, name string, stderr *bytes.Buffer) map[string]any {
	t.Helper()

	prop, ok := props[name].(map[string]any)
	if !ok {
		t.Fatalf("schema property %q type = %T, want map[string]any\nproperties: %#v\nstderr:\n%s", name, props[name], props, stderr.String())
	}

	return prop
}

func requirePropertyType(t *testing.T, prop map[string]any, want string, stderr *bytes.Buffer) {
	t.Helper()

	if got := prop["type"]; got != want {
		t.Fatalf("property type = %#v, want %q\nproperty: %#v\nstderr:\n%s", got, want, prop, stderr.String())
	}
}

func requirePropertyUnionType(t *testing.T, prop map[string]any, want []string, stderr *bytes.Buffer) {
	t.Helper()

	got, ok := prop["type"].([]any)
	if !ok {
		t.Fatalf("property type = %T, want []any\nproperty: %#v\nstderr:\n%s", prop["type"], prop, stderr.String())
	}
	if len(got) != len(want) {
		t.Fatalf("property union type = %#v, want %#v\nproperty: %#v\nstderr:\n%s", got, want, prop, stderr.String())
	}
	for i, wantValue := range want {
		if got[i] != wantValue {
			t.Fatalf("property union type = %#v, want %#v\nproperty: %#v\nstderr:\n%s", got, want, prop, stderr.String())
		}
	}
}

func requireNumber(t *testing.T, prop map[string]any, field string, want float64, stderr *bytes.Buffer) {
	t.Helper()

	got, ok := prop[field].(float64)
	if !ok {
		t.Fatalf("property %s = %T, want number\nproperty: %#v\nstderr:\n%s", field, prop[field], prop, stderr.String())
	}
	if got != want {
		t.Fatalf("property %s = %v, want %v\nproperty: %#v\nstderr:\n%s", field, got, want, prop, stderr.String())
	}
}

func requireStringEnum(t *testing.T, prop map[string]any, field string, want []string, stderr *bytes.Buffer) {
	t.Helper()

	got, ok := prop[field].([]any)
	if !ok {
		t.Fatalf("property %s = %T, want []any\nproperty: %#v\nstderr:\n%s", field, prop[field], prop, stderr.String())
	}
	if len(got) != len(want) {
		t.Fatalf("property %s = %#v, want %#v\nproperty: %#v\nstderr:\n%s", field, got, want, prop, stderr.String())
	}
	for i, wantValue := range want {
		if got[i] != wantValue {
			t.Fatalf("property %s = %#v, want %#v\nproperty: %#v\nstderr:\n%s", field, got, want, prop, stderr.String())
		}
	}
}

// =============================================================================
// MCP validation text assertion (shared between integration & coverage tests)
// =============================================================================

// assertMCPValidationText checks that the error text mentions the expected
// field and, depending on wantSchemaErr, expects either "argument" (SDK
// schema rejection) or "validation" (project-level validation) in the text.
func assertMCPValidationText(t *testing.T, text, wantField string, wantSchemaErr bool, stderr string) {
	t.Helper()

	if !strings.Contains(text, wantField) {
		t.Fatalf("error text = %q, want field %q\nstderr:\n%s", text, wantField, stderr)
	}

	lowerText := strings.ToLower(text)
	if wantSchemaErr {
		if !strings.Contains(lowerText, "argument") {
			t.Fatalf("error text = %q, want schema/arguments validation context\nstderr:\n%s", text, stderr)
		}

		return
	}

	if !strings.Contains(lowerText, "validation") {
		t.Fatalf("error text = %q, want project validation context\nstderr:\n%s", text, stderr)
	}
}

// =============================================================================
// Warning summary helpers (issues #110, #111)
// =============================================================================

// e2eWarnings is a goroutine-safe accumulator for test warnings. Tests should
// create one per top-level test function and call Report at the end.
type e2eWarnings struct {
	mu       sync.Mutex
	warnings []string
}

// Add records a warning message. It is safe for concurrent calls.
func (w *e2eWarnings) Add(format string, args ...any) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.warnings = append(w.warnings, fmt.Sprintf(format, args...))
}

// Report prints the WARNING SUMMARY block if any warnings were collected.
func (w *e2eWarnings) Report(t *testing.T) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.warnings) > 0 {
		t.Logf("--- WARNING SUMMARY ---")
		for _, warning := range w.warnings {
			t.Logf("  WARN: %s", warning)
		}
	}
}

// =============================================================================
// Shared invalid-input case definitions (issue #112)
// =============================================================================

// invalidInputCase describes an invalid MCP search input that should produce
// a tool error mentioning the expected field name.
type invalidInputCase struct {
	Name          string
	Arguments     map[string]any
	WantField     string
	WantSchemaErr bool
}

// SharedInvalidInputCases is the overlapping set of invalid-input test cases
// that are exercised by both TestMCPStdioE2E (integration through a live MCP
// stdio session) and TestMCPErrors_InvalidInputs (exhaustive coverage). Each
// test file appends its own extra cases.
var SharedInvalidInputCases = []invalidInputCase{
	{Name: "whitespace query", Arguments: map[string]any{"query": "   "}, WantField: "query"},
	{Name: "limit too high", Arguments: map[string]any{"query": "framework computer inc", "limit": 21}, WantField: "limit", WantSchemaErr: true},
	{Name: "pageno too low", Arguments: map[string]any{"query": "framework computer inc", "pageno": 0}, WantField: "pageno", WantSchemaErr: true},
	{Name: "invalid time range", Arguments: map[string]any{"query": "framework computer inc", "time_range": "week"}, WantField: "time_range", WantSchemaErr: true},
	{Name: "invalid safesearch", Arguments: map[string]any{"query": "framework computer inc", "safesearch": 3}, WantField: "safesearch", WantSchemaErr: true},
	{Name: "invalid language", Arguments: map[string]any{"query": "framework computer inc", "language": "not a valid language code"}, WantField: "language"},
	{Name: "invalid categories", Arguments: map[string]any{"query": "framework computer inc", "categories": "general/../../x"}, WantField: "categories"},
	{Name: "invalid engines", Arguments: map[string]any{"query": "framework computer inc", "engines": "bing/../../x"}, WantField: "engines"},
}
