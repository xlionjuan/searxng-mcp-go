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
)

func TestMCPErrors_Startup(t *testing.T) { //nolint:gocognit // test table, acceptable complexity
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
		t.Run(tt.name, func(t *testing.T) {
			subCtx, subCancel := context.WithTimeout(ctx, 10*time.Second)
			defer subCancel()

			var stderr, stdin bytes.Buffer
			stdin.WriteString(validMCPInitialize)

			cmd := exec.CommandContext(subCtx, binaryPath) //nolint:gosec // test runs built binary
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

			if got := exitErr.ExitCode(); got != 2 {
				t.Fatalf("exit code = %d, want 2 (MCP server error per documented contract)\nstderr:\n%s", got, stderrStr)
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

	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL, "DEBUG=1")

	// Verify stderr contains debug logging from startup
	stderrStr := stderr.String()
	t.Logf("stderr (debug mode) after connect:\n%s", stderrStr)

	if !strings.Contains(stderrStr, "debug") && !strings.Contains(stderrStr, "DEBUG") {
		// The server may not have logged anything yet — do a search to trigger logging
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "framework computer inc",
			"limit": 3,
		}, stderr, "debug mode search")

		stderrStr = stderr.String()
		t.Logf("stderr after search:\n%s", stderrStr)

		// Route zero-result outcomes through WARNING SUMMARY instead of
		// fatal, consistent with the rest of the E2E test suite.
		if len(response.Results) == 0 {
			warnings.Addf("debug mode search returned no results")
			t.Logf("debug mode search returned no results\nstderr:\n%s", stderrStr)
		}
	}

	if !strings.Contains(stderrStr, "debug mode enabled") {
		t.Fatalf("stderr does not contain debug startup log\nstderr:\n%s", stderrStr)
	}

	// Verify unresponsive_engines is present in the response (debug mode)
	result := callSearchTool(ctx, t, session, map[string]any{
		"query": "framework computer inc",
		"limit": 3,
	}, stderr)
	if result.IsError {
		t.Fatalf("debug mode response fields returned tool error: %s\nstderr:\n%s", toolText(t, result), stderr.String())
	}

	text := toolText(t, result)
	if !strings.Contains(text, `"unresponsive_engines"`) {
		t.Fatalf("debug response JSON does not contain unresponsive_engines\ntext:\n%s\nstderr:\n%s", text, stderr.String())
	}

	response := parseSearchResponse(t, result, stderr)
	if len(response.Results) == 0 {
		t.Logf("debug mode response fields returned no results\nresponse: %#v\nstderr:\n%s", response, stderr.String())
	}

	if response.UnresponsiveEngines == nil {
		t.Fatalf("response.UnresponsiveEngines is nil, "+
			"want debug JSON field to unmarshal as empty or populated slice\ntext:\n%s\nstderr:\n%s",
			text, stderr.String())
	}

	// Verify the response is valid JSON and contains expected fields.
	t.Logf("debug mode response: query=%q, results=%d, answers=%d, infoboxes=%d",
		response.Query, len(response.Results), len(response.Answers), len(response.Infoboxes))

	warnings.Report(t)
}

// TestMCPErrors_InvalidInputs is the exhaustive validation coverage test for
// the MCP handler: it runs every handler-level error path including ones that
// the SDK schema does not catch (whitespace and control characters in the
// query, 501-rune query, negative safesearch, pageno zero, etc.).
//
// It overlaps with the "validation errors" subtest inside TestMCPStdioE2E in
// e2e_mcp_test.go on purpose — that subtest is the integration check through
// the full live MCP stdio session; this one is the coverage test. Do not
// consolidate the two.
func TestMCPErrors_InvalidInputs(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	// Start with the shared overlapping cases and append coverage-specific extra cases.
	tests := make([]invalidInputCase, 0, len(SharedInvalidInputCases)+4)
	tests = append(tests, SharedInvalidInputCases...)
	tests = append(tests,
		// Extra cases that only the exhaustive coverage test exercises
		// (control characters, long query, limit=0, negative safesearch).
		invalidInputCase{
			Name:      "control characters in query",
			Arguments: map[string]any{"query": "golang\x00search"},
			WantField: "query",
		},
		invalidInputCase{
			Name:      "long query",
			Arguments: map[string]any{"query": strings.Repeat("a", 501)},
			WantField: "query",
		},
		invalidInputCase{
			Name:          "limit too low",
			Arguments:     map[string]any{"query": "framework computer inc", "limit": 0},
			WantField:     "limit",
			WantSchemaErr: true,
		},
		invalidInputCase{
			Name:          "safesearch negative",
			Arguments:     map[string]any{"query": "framework computer inc", "safesearch": -1},
			WantField:     "safesearch",
			WantSchemaErr: true,
		},
	)

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			result := callSearchTool(ctx, t, session, tt.Arguments, stderr)
			if !result.IsError {
				t.Fatalf("IsError = false, want true\nresult: %#v\nstderr:\n%s", result, stderr.String())
			}

			if len(result.Content) != 1 {
				t.Fatalf("content length = %d, want 1\nresult: %#v\nstderr:\n%s", len(result.Content), result, stderr.String())
			}

			text := toolText(t, result)
			assertMCPValidationText(t, text, tt.WantField, tt.WantSchemaErr, stderr.String())
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

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	tests := []struct {
		name      string
		arguments map[string]any
		wantField string
	}{
		{
			name:      "wrong type limit",
			arguments: map[string]any{"query": "framework computer inc", "limit": "twenty"},
			wantField: "limit",
		},
		{
			name:      "wrong type safesearch",
			arguments: map[string]any{"query": "framework computer inc", "safesearch": "two"},
			wantField: "safesearch",
		},
		{
			name:      "unexpected parameter",
			arguments: map[string]any{"query": "framework computer inc", "unknown_param": "value"},
			wantField: "unknown_param",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := callSearchTool(ctx, t, session, tt.arguments, stderr)
			if !result.IsError {
				t.Fatalf("IsError = false, want true\nresult: %#v\nstderr:\n%s", result, stderr.String())
			}

			text := toolText(t, result)
			assertMCPValidationText(t, text, tt.wantField, true, stderr.String())
		})
	}
}
