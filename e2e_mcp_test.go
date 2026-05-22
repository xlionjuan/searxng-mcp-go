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
	cmd.Env = append(os.Environ(), "SEARXNG_URL="+searxngURL)
	cmd.Stderr = &stderr

	var session *mcp.ClientSession
	var err error
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

	session, err = client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Log("MCP stdio session connected")

	searchTool := findSearchTool(ctx, t, session, &stderr)
	t.Logf("found tool: %s", searchTool.Name)
	verifySearchToolSchema(t, searchTool.InputSchema)
	t.Log("search tool schema verified")

	golangResult := callSearchTool(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 3,
	}, &stderr)
	if golangResult.IsError {
		t.Fatalf("golang search returned tool error: %s\nstderr:\n%s", toolText(t, golangResult), stderr.String())
	}
	golangRaw := toolText(t, golangResult)
	t.Logf("golang search raw response (%d bytes):\n%s", len(golangRaw), golangRaw)

	golangResponse := parseSearchResponse(t, golangResult, &stderr)
	t.Logf("golang search parsed: query=%q, results=%d, answers=%d, infoboxes=%d, suggestions=%d",
		golangResponse.Query, len(golangResponse.Results), len(golangResponse.Answers),
		len(golangResponse.Infoboxes), len(golangResponse.Suggestions))
	if len(golangResponse.Results) > 0 {
		t.Logf("  first result: title=%q, url=%q", golangResponse.Results[0].Title, golangResponse.Results[0].URL)
	}
	if len(golangResponse.Results) == 0 {
		t.Fatalf("golang search results length = 0\nresponse: %#v\nstderr:\n%s", golangResponse, stderr.String())
	}
	if strings.TrimSpace(golangResponse.Query) == "" {
		t.Fatalf("golang search query is empty\nresponse: %#v\nstderr:\n%s", golangResponse, stderr.String())
	}

	t.Log("empty query validation: sending empty query, expecting error")
	emptyQueryResult := callSearchTool(ctx, t, session, map[string]any{"query": ""}, &stderr)
	if !emptyQueryResult.IsError {
		t.Fatalf("empty query IsError = false, want true; content: %s\nstderr:\n%s", toolText(t, emptyQueryResult), stderr.String())
	}
	if text := toolText(t, emptyQueryResult); !strings.Contains(text, "validation error") {
		t.Fatalf("empty query error text = %q, want validation error\nstderr:\n%s", text, stderr.String())
	}
	t.Logf("empty query correctly rejected with: %s", toolText(t, emptyQueryResult))

	t.Log("zh-tw search: query=測試, language=zh-tw, limit=5")
	zhTWResult := callSearchTool(ctx, t, session, map[string]any{
		"query":    "測試",
		"language": "zh-tw",
		"limit":    5,
	}, &stderr)
	if zhTWResult.IsError {
		t.Fatalf("zh-tw search returned tool error: %s\nstderr:\n%s", toolText(t, zhTWResult), stderr.String())
	}
	zhTWRaw := toolText(t, zhTWResult)
	t.Logf("zh-tw search raw response (%d bytes):\n%s", len(zhTWRaw), zhTWRaw)

	zhTWResponse := parseSearchResponse(t, zhTWResult, &stderr)
	t.Logf("zh-tw search parsed: query=%q, results=%d, answers=%d, infoboxes=%d, suggestions=%d",
		zhTWResponse.Query, len(zhTWResponse.Results), len(zhTWResponse.Answers),
		len(zhTWResponse.Infoboxes), len(zhTWResponse.Suggestions))
	if payloadItemCount(zhTWResponse) == 0 {
		t.Fatalf("zh-tw search returned no meaningful payload items\nresponse: %#v\nstderr:\n%s", zhTWResponse, stderr.String())
	}
	hasCJK := searchResponseContainsCJK(zhTWResponse)
	t.Logf("zh-tw search contains CJK: %v", hasCJK)
	if !hasCJK {
		t.Fatalf("zh-tw search response does not contain CJK characters\nresponse: %#v\nstderr:\n%s", zhTWResponse, stderr.String())
	}

	query500 := strings.Repeat("a", searxng.MaxQueryLength)
	response := requireSearchResponse(ctx, t, session, map[string]any{"query": query500, "limit": 1}, &stderr, "query exactly 500 ASCII bytes")
	if len(response.Query) != searxng.MaxQueryLength {
		t.Fatalf("query exactly 500 ASCII bytes response query bytes = %d, want %d", len(response.Query), searxng.MaxQueryLength)
	}

	cjkBoundaryQuery := strings.Repeat("測", searxng.MaxQueryLength/3)
	response = requireSearchResponse(ctx, t, session, map[string]any{"query": cjkBoundaryQuery, "limit": 1}, &stderr, "unicode CJK query just under byte boundary")
	if len(response.Query) != len(cjkBoundaryQuery) {
		t.Fatalf("unicode CJK query response query bytes = %d, want %d", len(response.Query), len(cjkBoundaryQuery))
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query":      "golang release",
		"time_range": "day",
		"limit":      3,
	}, &stderr, "time_range day")
	if len(response.Results) == 0 {
		t.Logf("time_range day returned zero results from SearXNG; verified parameter was accepted")
	} else if !resultsContainRecentDateReference(response.Results) {
		t.Logf("time_range day returned results but no obvious date reference in title/url/content")
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query":      "golang",
		"safesearch": 1,
		"limit":      1,
	}, &stderr, "safesearch moderate")
	requireMeaningfulPayload(t, response, "safesearch moderate", &stderr)

	page1 := requireSearchResponse(ctx, t, session, map[string]any{
		"query":  "golang",
		"pageno": 1,
		"limit":  1,
	}, &stderr, "pagination first page")
	page2 := requireSearchResponse(ctx, t, session, map[string]any{
		"query":  "golang",
		"pageno": 2,
		"limit":  1,
	}, &stderr, "pagination second page")
	if payloadItemCount(page2) == 0 && len(page2.Suggestions) == 0 {
		t.Log("pagination second page returned no payload from SearXNG; verified pageno was accepted")
	}
	if firstResultURL(page1) != "" && firstResultURL(page2) != "" && firstResultURL(page1) == firstResultURL(page2) {
		t.Fatalf("pagination first result URL did not change: %q\npage1: %#v\npage2: %#v", firstResultURL(page1), page1, page2)
	}

	uncategorized := requireSearchResponse(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 1,
	}, &stderr, "general category baseline")
	categorized := requireSearchResponse(ctx, t, session, map[string]any{
		"query":      "golang",
		"categories": "general",
		"limit":      1,
	}, &stderr, "general category filter")
	requireMeaningfulPayload(t, categorized, "general category filter", &stderr)
	if firstResultURL(uncategorized) != "" && firstResultURL(categorized) != "" && firstResultURL(uncategorized) != firstResultURL(categorized) {
		t.Logf("general category filter changed first URL: %q -> %q", firstResultURL(uncategorized), firstResultURL(categorized))
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query":    "golang",
		"language": "auto",
		"limit":    1,
	}, &stderr, "auto language normalization")
	if strings.Contains(response.Query, "auto") || strings.Contains(response.Query, "language") {
		t.Fatalf("auto language leaked into response query: %q", response.Query)
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query":    "テスト",
		"language": "ja",
		"limit":    1,
	}, &stderr, "japanese language code")
	if response.Query != "テスト" {
		t.Fatalf("japanese language response query = %q, want %q", response.Query, "テスト")
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query": "!@#$%^&*() golang",
		"limit": 1,
	}, &stderr, "special character query")
	if strings.TrimSpace(response.Query) == "" {
		t.Fatalf("special character query response query is empty\nresponse: %#v", response)
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query": "測",
		"limit": 1,
	}, &stderr, "single CJK character query")
	if response.Query != "測" {
		t.Fatalf("single CJK character response query = %q, want %q", response.Query, "測")
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 20,
	}, &stderr, "limit valid max")
	if payloadItemCount(response) == 0 && len(response.Suggestions) == 0 {
		t.Fatalf("limit valid max returned no payload\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query":  "golang",
		"pageno": nil,
		"limit":  1,
	}, &stderr, "pageno null")
	requireMeaningfulPayload(t, response, "pageno null", &stderr)

	response = requireSearchResponse(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 1,
	}, &stderr, "limit truncation")
	if len(response.Results) > 0 && len(response.Results) != 1 {
		t.Fatalf("limit truncation result count = %d, want 1\nresponse: %#v", len(response.Results), response)
	}
	if len(response.Results) == 0 {
		requireMeaningfulPayload(t, response, "limit truncation", &stderr)
		t.Log("limit truncation returned no web results from SearXNG; verified valid limited response with non-result payload")
	}

	runConcurrentSearches(ctx, t, session, &stderr)
	verifyDebugModeIncludesUnresponsiveEngines(ctx, t, binaryPath, searxngURL)

	validationErrorCases := []struct {
		name      string
		arguments map[string]any
		wantText  string
	}{
		{
			name: "query 501 ASCII bytes",
			arguments: map[string]any{
				"query": strings.Repeat("a", searxng.MaxQueryLength+1),
			},
			wantText: "must be 500 characters or less",
		},
		{
			name: "unicode CJK query over byte boundary",
			arguments: map[string]any{
				"query": strings.Repeat("測", searxng.MaxQueryLength/3+1),
			},
			wantText: "must be 500 characters or less",
		},
		{
			name: "invalid time_range",
			arguments: map[string]any{
				"query":      "golang",
				"time_range": "week",
			},
			wantText: "time_range",
		},
		{
			name: "invalid pageno",
			arguments: map[string]any{
				"query":  "golang",
				"pageno": 0,
			},
			wantText: "pageno",
		},
		{
			name: "invalid language code",
			arguments: map[string]any{
				"query":    "golang",
				"language": "en_US",
			},
			wantText: "language",
		},
		{
			name: "control character query",
			arguments: map[string]any{
				"query": "golang\x00test",
			},
			wantText: "control characters",
		},
		{name: "missing required query", arguments: map[string]any{}, wantText: "query"},
		{name: "unknown argument", arguments: map[string]any{"query": "golang", "extra": true}, wantText: "extra"},
		{name: "wrong query type", arguments: map[string]any{"query": 123}, wantText: "query"},
		{name: "wrong limit type", arguments: map[string]any{"query": "golang", "limit": "1"}, wantText: "limit"},
		{name: "wrong safesearch type", arguments: map[string]any{"query": "golang", "safesearch": "1"}, wantText: "safesearch"},
		{name: "wrong pageno type", arguments: map[string]any{"query": "golang", "pageno": "2"}, wantText: "pageno"},
		{name: "limit zero", arguments: map[string]any{"query": "golang", "limit": 0}, wantText: "limit"},
		{name: "limit negative", arguments: map[string]any{"query": "golang", "limit": -1}, wantText: "limit"},
		{name: "limit too large", arguments: map[string]any{"query": "golang", "limit": 21}, wantText: "limit"},
		{name: "safesearch negative", arguments: map[string]any{"query": "golang", "safesearch": -1}, wantText: "safesearch"},
		{name: "safesearch too large", arguments: map[string]any{"query": "golang", "safesearch": 3}, wantText: "safesearch"},
		{name: "invalid categories trailing comma", arguments: map[string]any{"query": "golang", "categories": "general,,"}, wantText: "categories"},
		{name: "invalid categories leading comma", arguments: map[string]any{"query": "golang", "categories": ",general"}, wantText: "categories"},
		{name: "invalid categories slash", arguments: map[string]any{"query": "golang", "categories": "general/"}, wantText: "categories"},
		{name: "invalid categories backslash", arguments: map[string]any{"query": "golang", "categories": `general\`}, wantText: "categories"},
		{name: "invalid engines trailing comma", arguments: map[string]any{"query": "golang", "engines": "google,,"}, wantText: "engines"},
		{name: "invalid engines leading comma", arguments: map[string]any{"query": "golang", "engines": ",google"}, wantText: "engines"},
		{name: "invalid engines slash", arguments: map[string]any{"query": "golang", "engines": "google/"}, wantText: "engines"},
		{name: "invalid engines backslash", arguments: map[string]any{"query": "golang", "engines": `google\`}, wantText: "engines"},
		{name: "control character categories", arguments: map[string]any{"query": "golang", "categories": "general\x00test"}, wantText: "categories"},
		{name: "control character engines", arguments: map[string]any{"query": "golang", "engines": "google\x00test"}, wantText: "engines"},
	}

	for _, tc := range validationErrorCases {
		t.Logf("%s: sending arguments %#v, expecting validation error", tc.name, tc.arguments)
		assertSearchToolRejected(ctx, t, session, tc.arguments, tc.wantText, &stderr)
	}

	t.Log("=== All E2E tests passed ===")
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

func findSearchTool(ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr *bytes.Buffer) *mcp.Tool {
	t.Helper()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("tools/list failed: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Logf("tools/list returned %d tools", len(tools.Tools))
	for _, tool := range tools.Tools {
		t.Logf("  tool: %s — %s", tool.Name, tool.Description)
	}

	for _, tool := range tools.Tools {
		if tool.Name == "search" {
			return tool
		}
	}

	t.Fatalf("tools/list did not include search tool; got %#v\nstderr:\n%s", tools.Tools, stderr.String())

	return nil
}

func verifySearchToolSchema(t *testing.T, inputSchema any) {
	t.Helper()

	schema, ok := inputSchema.(map[string]any)
	if !ok {
		t.Fatalf("inputSchema type = %T, want map[string]any", inputSchema)
	}

	required, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("inputSchema.required type = %T, want []any", schema["required"])
	}
	if !containsAnyString(required, "query") {
		t.Fatalf("inputSchema.required = %#v, want query", required)
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema.properties type = %T, want map[string]any", schema["properties"])
	}

	for _, field := range []string{"language", "safesearch", "time_range", "categories", "engines", "pageno", "limit"} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("inputSchema.properties missing optional field %q", field)
		}
	}

	if got := schema["additionalProperties"]; got != false {
		t.Fatalf("inputSchema.additionalProperties = %#v, want false", got)
	}

	limit := requireSchemaProperty(t, properties, "limit")
	requireSchemaNumber(t, limit, "minimum", 1)
	requireSchemaNumber(t, limit, "maximum", 20)

	pageno := requireSchemaProperty(t, properties, "pageno")
	if !schemaTypeAllows(pageno["type"], "integer") || !schemaTypeAllows(pageno["type"], "null") {
		t.Fatalf("inputSchema.properties.pageno.type = %#v, want nullable integer", pageno["type"])
	}

	safesearch := requireSchemaProperty(t, properties, "safesearch")
	requireSchemaNumber(t, safesearch, "minimum", 0)
	requireSchemaNumber(t, safesearch, "maximum", 2)

	timeRange := requireSchemaProperty(t, properties, "time_range")
	enum, ok := timeRange["enum"].([]any)
	if !ok {
		t.Fatalf("inputSchema.properties.time_range.enum type = %T, want []any", timeRange["enum"])
	}
	for _, want := range []string{"day", "month", "year"} {
		if !containsAnyString(enum, want) {
			t.Fatalf("inputSchema.properties.time_range.enum = %#v, want %q", enum, want)
		}
	}
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}

	return false
}

func requireSchemaProperty(t *testing.T, properties map[string]any, name string) map[string]any {
	t.Helper()

	property, ok := properties[name].(map[string]any)
	if !ok {
		t.Fatalf("inputSchema.properties.%s type = %T, want map[string]any", name, properties[name])
	}

	return property
}

func requireSchemaNumber(t *testing.T, schema map[string]any, name string, want float64) {
	t.Helper()

	got, ok := schema[name].(float64)
	if !ok {
		t.Fatalf("schema.%s type = %T, want float64", name, schema[name])
	}
	if got != want {
		t.Fatalf("schema.%s = %v, want %v", name, got, want)
	}
}

func schemaTypeAllows(schemaType any, want string) bool {
	switch typed := schemaType.(type) {
	case string:
		return typed == want
	case []any:
		return containsAnyString(typed, want)
	default:
		return false
	}
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

func callSearchToolRaw(
	ctx context.Context,
	session *mcp.ClientSession,
	arguments map[string]any,
) (*mcp.CallToolResult, error) {
	return session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "search",
		Arguments: arguments,
	})
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
	if strings.TrimSpace(response.Query) == "" {
		t.Fatalf("%s response query is empty\nresponse: %#v\nstderr:\n%s", name, response, stderr.String())
	}

	return response
}

func assertSearchToolRejected(
	ctx context.Context,
	t *testing.T,
	session *mcp.ClientSession,
	arguments map[string]any,
	wantText string,
	stderr *bytes.Buffer,
) {
	t.Helper()

	result, err := callSearchToolRaw(ctx, session, arguments)
	if err != nil {
		text := err.Error()
		if !strings.Contains(text, wantText) {
			t.Fatalf("tools/call search error = %q, want containing %q\nstderr:\n%s", text, wantText, stderr.String())
		}
		t.Logf("rejected with MCP error: %s", text)

		return
	}

	if !result.IsError {
		t.Fatalf("IsError = false, want true; content: %s\nstderr:\n%s", toolText(t, result), stderr.String())
	}

	text := toolText(t, result)
	if !strings.Contains(text, wantText) ||
		(!strings.Contains(text, "validation error") && !strings.Contains(text, "validating") && !strings.Contains(text, "invalid")) {
		t.Fatalf("error text = %q, want rejection containing %q\nstderr:\n%s", text, wantText, stderr.String())
	}
	t.Logf("correctly rejected with: %s", text)
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

func searchResponseContainsCJK(response searxng.SearchResponse) bool {
	for _, answer := range response.Answers {
		if containsCJK(answer.Answer) || containsCJK(answer.Engine) || containsCJK(answer.Template) || containsCJK(answer.URL) {
			return true
		}

		for _, translation := range answer.Translations {
			if containsCJK(translation.Text) || containsCJK(translation.Transliteration) {
				return true
			}
		}
	}

	for _, result := range response.Results {
		if containsCJK(result.Title) || containsCJK(result.URL) || containsCJK(result.Content) || containsCJK(result.Engine) {
			return true
		}
	}

	for _, infobox := range response.Infoboxes {
		if containsCJK(infobox.Infobox) || containsCJK(infobox.Content) {
			return true
		}

		for _, attribute := range infobox.Attributes {
			if containsCJK(attribute.Label) || containsCJK(attribute.Value) {
				return true
			}
		}

		for _, itemURL := range infobox.URLs {
			if containsCJK(itemURL.Title) || containsCJK(itemURL.URL) {
				return true
			}
		}
	}

	for _, suggestion := range response.Suggestions {
		if containsCJK(suggestion) {
			return true
		}
	}

	return false
}

func payloadItemCount(response searxng.SearchResponse) int {
	return len(response.Results) + len(response.Infoboxes) + len(response.Answers)
}

func firstResultURL(response searxng.SearchResponse) string {
	if len(response.Results) == 0 {
		return ""
	}

	return response.Results[0].URL
}

func requireMeaningfulPayload(t *testing.T, response searxng.SearchResponse, name string, stderr *bytes.Buffer) {
	t.Helper()

	if payloadItemCount(response) == 0 {
		t.Fatalf("%s payload item count = 0\nresponse: %#v\nstderr:\n%s", name, response, stderr.String())
	}
}

func resultsContainRecentDateReference(results []searxng.SearchResult) bool {
	now := time.Now()
	dateNeedles := []string{
		now.Format("2006"),
		now.Format("2006-01"),
		now.Format("Jan"),
		now.Format("January"),
	}

	for _, result := range results {
		haystack := result.Title + " " + result.URL + " " + result.Content
		for _, needle := range dateNeedles {
			if needle != "" && strings.Contains(haystack, needle) {
				return true
			}
		}
	}

	return false
}

func runConcurrentSearches(ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr *bytes.Buffer) {
	t.Helper()

	type concurrentCase struct {
		name  string
		query string
	}

	cases := []concurrentCase{
		{name: "concurrent golang", query: "golang"},
		{name: "concurrent rust", query: "rust programming"},
		{name: "concurrent python", query: "python programming"},
	}

	var wg sync.WaitGroup
	errCh := make(chan string, len(cases))

	for _, tc := range cases {
		tc := tc
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := callSearchToolRaw(ctx, session, map[string]any{"query": tc.query, "limit": 1})
			if err != nil {
				errCh <- tc.name + ": " + err.Error()

				return
			}
			if result.IsError {
				text, ok := toolTextIfPresent(result)
				if !ok {
					errCh <- tc.name + ": tool result error with no text content"

					return
				}

				errCh <- tc.name + ": " + text

				return
			}

			text, ok := toolTextIfPresent(result)
			if !ok {
				errCh <- tc.name + ": tool result has no text content"

				return
			}

			var response searxng.SearchResponse
			if err := json.Unmarshal([]byte(text), &response); err != nil {
				errCh <- tc.name + ": response is not SearchResponse JSON: " + err.Error()

				return
			}

			if strings.TrimSpace(response.Query) == "" {
				errCh <- tc.name + ": empty response query"
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for errText := range errCh {
		t.Fatalf("concurrent search failed: %s\nstderr:\n%s", errText, stderr.String())
	}
}

func toolTextIfPresent(result *mcp.CallToolResult) (string, bool) {
	if result == nil || len(result.Content) == 0 {
		return "", false
	}

	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return "", false
	}

	return textContent.Text, true
}

func verifyDebugModeIncludesUnresponsiveEngines(ctx context.Context, t *testing.T, binaryPath string, searxngURL string) {
	t.Helper()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = append(os.Environ(), "SEARXNG_URL="+searxngURL, "DEBUG=1")
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-e2e-debug-test",
		Version: version,
	}, nil)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect debug MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close debug MCP session: %v\nstderr:\n%s", closeErr, stderr.String())
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	result := callSearchTool(ctx, t, session, map[string]any{"query": "golang", "limit": 1}, &stderr)
	if result.IsError {
		t.Fatalf("debug mode search returned tool error: %s\nstderr:\n%s", toolText(t, result), stderr.String())
	}

	raw := toolText(t, result)
	if !strings.Contains(raw, `"unresponsive_engines"`) {
		t.Fatalf("debug mode response missing unresponsive_engines\nresponse: %s\nstderr:\n%s", raw, stderr.String())
	}
}

func containsCJK(value string) bool {
	for _, r := range value {
		if (r >= '\u3400' && r <= '\u4dbf') ||
			(r >= '\u4e00' && r <= '\u9fff') ||
			(r >= '\uf900' && r <= '\ufaff') {
			return true
		}
	}

	return false
}
