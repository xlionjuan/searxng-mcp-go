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
			t.Fatalf("basic search results length = 0\nresponse: %#v\nstderr:\n%s", response, stderr.String())
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
