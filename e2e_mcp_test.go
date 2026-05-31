//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

func TestMCPStdioE2E(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using MCP binary: %s", binaryPath)

	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL)
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

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-e2e-test",
		Version: version,
	}, nil)

	var err error
	session, err = client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Log("MCP stdio session connected")

	searchTool := findSearchTool(ctx, t, session, &stderr)
	t.Logf("found tool: %s", searchTool.Name)

	t.Run("basic search", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "golang",
			"limit": 3,
		}, &stderr, "basic search")

		if len(response.Results) == 0 {
			if len(response.Answers) == 0 && len(response.Infoboxes) == 0 {
				t.Fatalf("basic search returned no results, answers, or infoboxes\nresponse: %#v\nstderr:\n%s", response, stderr.String())
			}
		}
		if strings.TrimSpace(response.Query) == "" {
			t.Fatalf("basic search query is empty\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
	})

	t.Run("answer response", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "sha512 hello",
			"limit": 1,
		}, &stderr, "answer response")

		if len(response.Answers) == 0 {
			t.Fatalf("answer response answers length = 0\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
	})

	t.Run("infobox response", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "apple inc",
			"limit": 3,
		}, &stderr, "infobox response")

		if len(response.Infoboxes) == 0 {
			t.Fatalf("infobox response infoboxes length = 0\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
	})

	t.Run("optional parameter forwarding", func(t *testing.T) {
		wantEngines := []string{"google", "bing", "yahoo", "ddg definitions"}
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query":      "golang",
			"language":   "en",
			"safesearch": 1,
			"categories": "general",
			"engines":    strings.Join(wantEngines, ","),
			"pageno":     1,
			"limit":      5,
		}, &stderr, "optional parameter forwarding")

		if response.Query != "golang" {
			t.Fatalf("query = %q, want golang\nresponse: %#v\nstderr:\n%s", response.Query, response, stderr.String())
		}
		if len(response.Results) == 0 || len(response.Results) > 5 {
			t.Fatalf("results length = %d, want 1..5\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
		}

		sawEngine := false
		for i, result := range response.Results {
			if strings.TrimSpace(result.Title) == "" {
				t.Fatalf("result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			if strings.TrimSpace(result.URL) == "" {
				t.Fatalf("result[%d] url is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			if strings.TrimSpace(result.Engine) == "" {
				t.Fatalf("result[%d] engine is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			for _, we := range wantEngines {
				if strings.EqualFold(result.Engine, we) {
					sawEngine = true
				}
			}
		}
		if !sawEngine {
			t.Fatalf("no result from any of %v\nresponse: %#v\nstderr:\n%s", wantEngines, response, stderr.String())
		}
	})

	t.Run("validation errors", func(t *testing.T) {
		tests := []struct {
			name          string
			argument      map[string]any
			wantField     string
			wantSchemaErr bool
		}{
			{name: "whitespace query", argument: map[string]any{"query": "   "}, wantField: "query"},
			{name: "limit too high", argument: map[string]any{"query": "golang", "limit": 21}, wantField: "limit", wantSchemaErr: true},
			{name: "pageno too low", argument: map[string]any{"query": "golang", "pageno": 0}, wantField: "pageno", wantSchemaErr: true},
			{name: "invalid time range", argument: map[string]any{"query": "golang", "time_range": "week"}, wantField: "time_range", wantSchemaErr: true},
			{name: "invalid safesearch", argument: map[string]any{"query": "golang", "safesearch": 3}, wantField: "safesearch", wantSchemaErr: true},
			{name: "invalid language", argument: map[string]any{"query": "golang", "language": "not a valid language code"}, wantField: "language"},
			{name: "invalid categories", argument: map[string]any{"query": "golang", "categories": "general/../../x"}, wantField: "categories"},
			{name: "invalid engines", argument: map[string]any{"query": "golang", "engines": "bing/../../x"}, wantField: "engines"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := callSearchTool(ctx, t, session, tt.argument, &stderr)
				if !result.IsError {
					t.Fatalf("IsError = false, want true\nresult: %#v\nstderr:\n%s", result, stderr.String())
				}
				if len(result.Content) != 1 {
					t.Fatalf("content length = %d, want 1\nresult: %#v\nstderr:\n%s", len(result.Content), result, stderr.String())
				}

				text := toolText(t, result)
				assertMCPValidationText(t, text, tt.wantField, tt.wantSchemaErr, stderr.String())
			})
		}
	})

	t.Run("news category with time range", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query":      "technology",
			"language":   "en",
			"categories": "news",
			"time_range": "month",
			"limit":      5,
		}, &stderr, "news category with time range")

		if len(response.Results) == 0 || len(response.Results) > 5 {
			t.Fatalf("results length = %d, want 1..5\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
		}

		hasPublishedDate := false
		hasHTTPURL := false
		for _, result := range response.Results {
			if result.PublishedDate != nil && strings.TrimSpace(*result.PublishedDate) != "" {
				hasPublishedDate = true
			}
			if strings.HasPrefix(result.URL, "http://") || strings.HasPrefix(result.URL, "https://") {
				hasHTTPURL = true
			}
		}
		if !hasPublishedDate {
			t.Fatalf("no result had publishedDate\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
		if !hasHTTPURL {
			t.Fatalf("no result had an http(s) URL\nresponse: %#v\nstderr:\n%s", response, stderr.String())
		}
	})

	t.Run("unicode query round trip", func(t *testing.T) {
		query := "日本 golang \"type parameters\" site:go.dev"
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query":      query,
			"language":   "ja",
			"engines":    "bing",
			"categories": "general",
			"limit":      5,
		}, &stderr, "unicode query round trip")

		if response.Query != query {
			t.Fatalf("query = %q, want %q\nresponse: %#v\nstderr:\n%s", response.Query, query, response, stderr.String())
		}
		if len(response.Results) > 5 {
			t.Fatalf("results length = %d, want <= 5\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
		}
		for i, result := range response.Results {
			if strings.TrimSpace(result.URL) == "" {
				t.Fatalf("result[%d] URL is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
		}
	})

	t.Run("response format invariants", func(t *testing.T) {
		result := callSearchTool(ctx, t, session, map[string]any{
			"query":      "site:example.invalid unlikely-no-real-result-codex-e2e",
			"engines":    "bing",
			"categories": "general",
			"limit":      3,
		}, &stderr)
		text := toolText(t, result)
		if result.IsError {
			t.Fatalf("response format query returned tool error: %s\nstderr:\n%s", text, stderr.String())
		}
		if !strings.Contains(text, `"results":[]`) {
			t.Fatalf("raw JSON does not contain empty results array\ntext:\n%s\nstderr:\n%s", text, stderr.String())
		}
		if !strings.Contains(text, `"suggestions":[]`) {
			t.Fatalf("raw JSON does not contain empty suggestions array\ntext:\n%s\nstderr:\n%s", text, stderr.String())
		}

		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(text), &raw); err != nil {
			t.Fatalf("response is not JSON: %v\ntext:\n%s\nstderr:\n%s", err, text, stderr.String())
		}
		for _, field := range []string{"answers", "infoboxes", "unresponsive_engines"} {
			if _, ok := raw[field]; ok {
				t.Fatalf("field %q present, want omitted when empty/debug-off\ntext:\n%s\nstderr:\n%s", field, text, stderr.String())
			}
		}

		response := parseSearchResponse(t, result, &stderr)
		if len(response.Results) != 0 {
			t.Fatalf("results length = %d, want 0\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
		}
		if len(response.Suggestions) != 0 {
			t.Fatalf("suggestions length = %d, want 0\nresponse: %#v\nstderr:\n%s", len(response.Suggestions), response, stderr.String())
		}
	})

	t.Run("concurrent searches", func(t *testing.T) {
		queries := []string{"golang", "rust language", "python typing"}
		var wg sync.WaitGroup
		errs := make(chan string, len(queries))

		for _, query := range queries {
			query := query
			wg.Add(1)
			go func() {
				defer wg.Done()

				result, err := session.CallTool(ctx, &mcp.CallToolParams{
					Name: "search",
					Arguments: map[string]any{
						"query": query,
						"limit": 3,
					},
				})
				if err != nil {
					errs <- "tools/call search failed for " + query + ": " + err.Error()
					return
				}
				if result.IsError {
					errs <- "search returned tool error for " + query + ": " + toolText(t, result)
					return
				}

				response := parseSearchResponse(t, result, &stderr)
				if len(response.Results) > 3 {
					errs <- "search returned too many results for " + query
				}
			}()
		}

		wg.Wait()
		close(errs)

		for errText := range errs {
			t.Errorf("%s\nstderr:\n%s", errText, stderr.String())
		}
	})

	t.Log("MCP stdio session lifecycle verified")
}

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

func e2eMCPEnv(searxngURL string, extra ...string) []string {
	env := append(os.Environ(), "SEARXNG_URL="+searxngURL, "SEARXNG_MAX_RETRIES=0")
	env = append(env, extra...)

	return env
}

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

	return nil
}

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

func parseSearchResponse(t *testing.T, result *mcp.CallToolResult, stderr *bytes.Buffer) searxng.SearchResponse {
	t.Helper()

	text := toolText(t, result)

	var response searxng.SearchResponse
	if err := json.Unmarshal([]byte(text), &response); err != nil {
		t.Fatalf("search tool text is not SearchResponse JSON: %v\ntext:\n%s\nstderr:\n%s", err, text, stderr.String())
	}

	return response
}

func toolText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()

	if len(result.Content) == 0 {
		t.Fatal("tool result has no content")
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("tool result content[0] type = %T, want *mcp.TextContent", result.Content[0])
	}

	return textContent.Text
}

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
	requireNumber(t, limit, "minimum", 1, stderr)
	requireNumber(t, limit, "maximum", 20, stderr)

	safesearch := requireProperty(t, props, "safesearch", stderr)
	requirePropertyType(t, safesearch, "integer", stderr)
	requireNumber(t, safesearch, "minimum", 0, stderr)
	requireNumber(t, safesearch, "maximum", 2, stderr)

	pageno := requireProperty(t, props, "pageno", stderr)
	requirePropertyUnionType(t, pageno, []string{"null", "integer"}, stderr)
	requireNumber(t, pageno, "minimum", 1, stderr)

	timeRange := requireProperty(t, props, "time_range", stderr)
	requireStringEnum(t, timeRange, "enum", []string{"", "day", "month", "year"}, stderr)
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
