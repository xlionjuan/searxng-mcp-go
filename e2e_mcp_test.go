//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
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

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

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

	searchTool := findSearchTool(ctx, t, session, &stderr)
	verifySearchToolSchema(t, searchTool.InputSchema)

	golangResult := callSearchTool(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 3,
	}, &stderr)
	if golangResult.IsError {
		t.Fatalf("golang search returned tool error: %s\nstderr:\n%s", toolText(t, golangResult), stderr.String())
	}

	golangResponse := parseSearchResponse(t, golangResult, &stderr)
	if len(golangResponse.Results) == 0 {
		t.Fatalf("golang search results length = 0\nresponse: %#v\nstderr:\n%s", golangResponse, stderr.String())
	}
	if strings.TrimSpace(golangResponse.Query) == "" {
		t.Fatalf("golang search query is empty\nresponse: %#v\nstderr:\n%s", golangResponse, stderr.String())
	}

	emptyQueryResult := callSearchTool(ctx, t, session, map[string]any{"query": ""}, &stderr)
	if !emptyQueryResult.IsError {
		t.Fatalf("empty query IsError = false, want true; content: %s\nstderr:\n%s", toolText(t, emptyQueryResult), stderr.String())
	}
	if text := toolText(t, emptyQueryResult); !strings.Contains(text, "validation error") {
		t.Fatalf("empty query error text = %q, want validation error\nstderr:\n%s", text, stderr.String())
	}

	zhTWResult := callSearchTool(ctx, t, session, map[string]any{
		"query":    "測試",
		"language": "zh-tw",
		"limit":    5,
	}, &stderr)
	if zhTWResult.IsError {
		t.Fatalf("zh-tw search returned tool error: %s\nstderr:\n%s", toolText(t, zhTWResult), stderr.String())
	}

	zhTWResponse := parseSearchResponse(t, zhTWResult, &stderr)
	if !searchResponseContainsCJK(zhTWResponse) {
		t.Fatalf("zh-tw search response does not contain CJK characters\nresponse: %#v\nstderr:\n%s", zhTWResponse, stderr.String())
	}
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
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if got, ok := value.(string); ok && got == want {
			return true
		}
	}

	return false
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
	if containsCJK(response.Query) {
		return true
	}

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
