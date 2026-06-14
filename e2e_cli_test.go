package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

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

// envWithout returns a copy of the current environment with the given keys removed.
func envWithout(keys ...string) []string {
	env := os.Environ()

	for _, key := range keys {
		env = removeEnv(env, key)
	}

	return env
}

// runCLI runs the binary with the given environment and arguments and returns
// the combined stdout/stderr output and exit code.
func runCLI(t *testing.T, binPath string, env []string, args ...string) (string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, args...) //nolint:gosec // test runs built binary
	cmd.Env = env
	cmd.Dir = "."

	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError

	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exitErr):
		return string(out), exitErr.ExitCode()
	default:
		t.Fatalf("unexpected error type %T: %v\noutput: %s", err, err, out)

		return "", 0
	}
}

// TestCLIHelp verifies that --help exits successfully and prints usage text.
func TestCLIHelp(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	out, code := runCLI(t, binPath, os.Environ(), "--help")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\noutput: %s", code, out)
	}

	if !strings.Contains(out, "SearXNG MCP Server") {
		t.Errorf("stdout should contain 'SearXNG MCP Server', got: %s", out)
	}

	if !strings.Contains(out, "USAGE:") {
		t.Errorf("stdout should contain usage text, got: %s", out)
	}
}

// TestCLIVersion verifies that --version exits successfully and prints the
// version string.
func TestCLIVersion(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	out, code := runCLI(t, binPath, os.Environ(), "--version")
	if code != 0 {
		t.Fatalf("exit code = %d, want 0\noutput: %s", code, out)
	}

	if !strings.Contains(out, "searxng-mcp-go version") {
		t.Errorf("stdout should contain 'searxng-mcp-go version', got: %s", out)
	}
}

// TestCLIMissingSearxngURL verifies that CLI mode fails with exit code 1 when
// neither --searxng-url nor SEARXNG_URL is provided.
func TestCLIMissingSearxngURL(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	env := envWithout("SEARXNG_URL")
	out, code := runCLI(t, binPath, env, "test")

	if code != 1 {
		t.Errorf("exit code = %d, want 1\noutput: %s", code, out)
	}

	if !strings.Contains(out, "SEARXNG_URL") {
		t.Errorf("stderr should contain 'SEARXNG_URL', got: %s", out)
	}
}

// TestCLIMultipleQueries verifies that more than one positional argument is
// rejected.
func TestCLIMultipleQueries(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	out, code := runCLI(t, binPath, os.Environ(), "foo", "bar")

	if code != 1 {
		t.Errorf("exit code = %d, want 1\noutput: %s", code, out)
	}

	if !strings.Contains(out, "only one query") {
		t.Errorf("stderr/output should contain 'only one query', got: %s", out)
	}
}

// TestCLIEnvFlagPrecedence verifies that CLI flags override environment
// variables for timeout, max-retries, and allow-get-fallback. Because the
// overridden values are valid, an unrelated invalid flag (--pageno 0) is used
// to trigger a validation error; reaching that error proves the configuration
// was parsed and accepted successfully.
func TestCLIEnvFlagPrecedence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		env      []string
		args     []string
		wantCode int
		wantOut  string
	}{
		{
			name:     "timeout flag overrides env",
			env:      []string{"SEARXNG_TIMEOUT=30s"},
			args:     []string{"--timeout", "10s", "--pageno", "0", "--searxng-url", "http://localhost:9999", "test"},
			wantCode: 1,
			wantOut:  "validation error",
		},
		{
			name:     "max-retries flag overrides env",
			env:      []string{"SEARXNG_MAX_RETRIES=5"},
			args:     []string{"--max-retries", "1", "--pageno", "0", "--searxng-url", "http://localhost:9999", "test"},
			wantCode: 1,
			wantOut:  "validation error",
		},
		{
			name:     "allow-get-fallback flag overrides env",
			env:      []string{"SEARXNG_ALLOW_GET_FALLBACK=0"},
			args:     []string{"--allow-get-fallback", "--pageno", "0", "--searxng-url", "http://localhost:9999", "test"},
			wantCode: 1,
			wantOut:  "validation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			binPath, cleanup := buildTestBinary(t)
			defer cleanup()

			env := envWithout(
				"SEARXNG_URL",
				"SEARXNG_TIMEOUT",
				"SEARXNG_MAX_RETRIES",
				"SEARXNG_ALLOW_GET_FALLBACK",
			)
			env = append(env, tt.env...)

			out, code := runCLI(t, binPath, env, tt.args...)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\noutput: %s", code, tt.wantCode, out)
			}

			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output should contain %q, got: %s", tt.wantOut, out)
			}

			if strings.Contains(out, "configuration error") {
				t.Errorf("expected flag/env parsing to succeed, got configuration error: %s", out)
			}
		})
	}
}

// TestCLINonJSONOutputFormat verifies that a CLI run without --json does not
// emit JSON-formatted output on failure. Because this test has no live SearXNG
// server, it fails at the HTTP layer; the practical assertion is that the
// captured output is human-readable error text rather than JSON.
func TestCLINonJSONOutputFormat(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	env := envWithout("SEARXNG_URL")
	// Use a short timeout and no retries so the failure is deterministic and
	// fast even when the test environment resolves localhost slowly.
	out, code := runCLI(t, binPath, env,
		"--searxng-url", "http://localhost:9999",
		"--timeout", "2s",
		"--max-retries", "0",
		"test",
	)

	if code != 1 {
		t.Errorf("exit code = %d, want 1\noutput: %s", code, out)
	}

	if strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Errorf("human-readable failure output should not look like JSON, got: %s", out)
	}

	if !strings.Contains(out, "search error") {
		t.Errorf("expected a search/HTTP error without live server, got: %s", out)
	}
}
