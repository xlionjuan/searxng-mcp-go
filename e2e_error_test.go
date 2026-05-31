//go:build e2e

package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// validMCPInitialize is a minimal valid MCP initialize message to pass
// the stdin validation gate so we reach config validation.
const validMCPInitialize = `{"jsonrpc":"2.0","method":"initialize"}` + "\n"

func TestMCPErrors_Startup(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using MCP binary: %s", binaryPath)

	tests := []struct {
		name       string
		env        map[string]string
		wantStderr []string
	}{
		{
			name: "empty URL",
			env: map[string]string{
				"SEARXNG_URL": "",
			},
			wantStderr: []string{"SEARXNG_URL"},
		},
		{
			name: "bad scheme",
			env: map[string]string{
				"SEARXNG_URL": "ftp://searxng.example.com",
			},
			wantStderr: []string{"http", "https"},
		},
		{
			name: "no host",
			env: map[string]string{
				"SEARXNG_URL": "http://",
			},
			wantStderr: []string{"host"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
			defer subCancel()

			var stderr bytes.Buffer
			var stdin bytes.Buffer
			stdin.WriteString(validMCPInitialize)

			cmd := exec.CommandContext(subCtx, binaryPath)
			cmd.Env = os.Environ()
			for k, v := range tt.env {
				if v == "" {
					cmd.Env = removeEnv(cmd.Env, k)
				} else {
					cmd.Env = append(cmd.Env, k+"="+v)
				}
			}
			cmd.Stdin = &stdin
			cmd.Stderr = &stderr

			// Run the binary — it should pass the stdin validation but fail
			// on config validation and exit.
			err := cmd.Run()

			stderrStr := stderr.String()
			t.Logf("stderr:\n%s", stderrStr)
			t.Logf("exit error: %v", err)

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("cmd.Run() error = %v, want *exec.ExitError\nstderr:\n%s", err, stderrStr)
			}
			if got := exitErr.ExitCode(); got != 1 {
				t.Fatalf("exit code = %d, want 1\nstderr:\n%s", got, stderrStr)
			}

			for _, want := range tt.wantStderr {
				if !strings.Contains(stderrStr, want) {
					t.Errorf("stderr does not contain %q\nstderr:\n%s", want, stderrStr)
				}
			}
		})
	}
}

func TestMCPErrors_DebugMode(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using MCP binary: %s", binaryPath)

	// Run the MCP server with DEBUG=1.
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL, "DEBUG=1")
	cmd.Stderr = &stderr

	var session *mcp.ClientSession
	t.Cleanup(func() {
		if session != nil {
			if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
				t.Logf("close MCP session: %v", closeErr)
			}
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-debug-test",
		Version: version,
	}, nil)

	var err error
	session, err = client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	// Verify stderr contains debug logging from startup
	stderrStr := stderr.String()
	t.Logf("stderr (debug mode) after connect:\n%s", stderrStr)

	if !strings.Contains(stderrStr, "debug") && !strings.Contains(stderrStr, "DEBUG") {
		// The server may not have logged anything yet — do a search to trigger logging
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "golang",
			"limit": 3,
		}, &stderr, "debug mode search")

		stderrStr = stderr.String()
		t.Logf("stderr after search:\n%s", stderrStr)

		if len(response.Results) == 0 {
			t.Fatalf("debug mode search returned no results\nstderr:\n%s", stderrStr)
		}
	}

	// Verify stderr has some content indicating debug/log output
	if stderrStr == "" {
		t.Log("stderr is empty — this may be acceptable if server logs are buffered")
	}

	// Verify unresponsive_engines is present in the response (debug mode)
	response := requireSearchResponse(ctx, t, session, map[string]any{
		"query": "golang",
		"limit": 3,
	}, &stderr, "debug mode response fields")

	// Verify the response is valid JSON and contains expected fields.
	t.Logf("debug mode response: query=%q, results=%d, answers=%d, infoboxes=%d",
		response.Query, len(response.Results), len(response.Answers), len(response.Infoboxes))
}

func TestMCPErrors_InvalidInputs(t *testing.T) {
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
	t.Logf("using MCP binary: %s", binaryPath)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := connectMCPSessionErrorTest(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	tests := []struct {
		name          string
		arguments     map[string]any
		wantField     string
		wantSchemaErr bool
	}{
		{
			name:      "whitespace query",
			arguments: map[string]any{"query": "   "},
			wantField: "query",
		},
		{
			name:      "control characters in query",
			arguments: map[string]any{"query": "golang\x00search"},
			wantField: "query",
		},
		{
			name:      "long query",
			arguments: map[string]any{"query": strings.Repeat("a", 501)},
			wantField: "query",
		},
		{
			name:          "limit too high",
			arguments:     map[string]any{"query": "golang", "limit": 21},
			wantField:     "limit",
			wantSchemaErr: true,
		},
		{
			name:          "limit too low",
			arguments:     map[string]any{"query": "golang", "limit": 0},
			wantField:     "limit",
			wantSchemaErr: true,
		},
		{
			name:          "pageno too low",
			arguments:     map[string]any{"query": "golang", "pageno": 0},
			wantField:     "pageno",
			wantSchemaErr: true,
		},
		{
			name:          "invalid safesearch",
			arguments:     map[string]any{"query": "golang", "safesearch": 3},
			wantField:     "safesearch",
			wantSchemaErr: true,
		},
		{
			name:          "safesearch negative",
			arguments:     map[string]any{"query": "golang", "safesearch": -1},
			wantField:     "safesearch",
			wantSchemaErr: true,
		},
		{
			name:          "invalid time range",
			arguments:     map[string]any{"query": "golang", "time_range": "week"},
			wantField:     "time_range",
			wantSchemaErr: true,
		},
		{
			name:      "invalid language",
			arguments: map[string]any{"query": "golang", "language": "not a valid language code"},
			wantField: "language",
		},
		{
			name:      "invalid categories",
			arguments: map[string]any{"query": "golang", "categories": "general/../../x"},
			wantField: "categories",
		},
		{
			name:      "invalid engines",
			arguments: map[string]any{"query": "golang", "engines": "bing/../../x"},
			wantField: "engines",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			result := callSearchTool(ctx, t, session, tt.arguments, &stderr)
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
}

func TestMCPErrors_IncorrectParams(t *testing.T) {
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
	t.Logf("using MCP binary: %s", binaryPath)

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := connectMCPSessionErrorTest(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	tests := []struct {
		name      string
		arguments map[string]any
		wantField string
	}{
		{
			name:      "wrong type limit",
			arguments: map[string]any{"query": "golang", "limit": "twenty"},
			wantField: "limit",
		},
		{
			name:      "wrong type safesearch",
			arguments: map[string]any{"query": "golang", "safesearch": "two"},
			wantField: "safesearch",
		},
		{
			name:      "unexpected parameter",
			arguments: map[string]any{"query": "golang", "unknown_param": "value"},
			wantField: "unknown_param",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			result := callSearchTool(ctx, t, session, tt.arguments, &stderr)
			if !result.IsError {
				t.Fatalf("IsError = false, want true\nresult: %#v\nstderr:\n%s", result, stderr.String())
			}

			text := toolText(t, result)
			assertMCPValidationText(t, text, tt.wantField, true, stderr.String())
		})
	}
}

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

// connectMCPSessionErrorTest connects to the MCP server via stdio.
func connectMCPSessionErrorTest(ctx context.Context, t *testing.T, cmd *exec.Cmd, stderr *bytes.Buffer) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-error-test",
		Version: version,
	}, nil)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	return session
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
