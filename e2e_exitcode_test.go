package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// buildTestBinary compiles the binary for exit code testing.
// Returns the path to the binary and a cleanup function.
func buildTestBinary(t *testing.T) (string, func()) {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "searxng-mcp-go")

	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binPath, ".") //nolint:gosec // test runs built binary

	cmd.Dir = "."

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	return binPath, func() { _ = os.Remove(binPath) } //nolint:errcheck // best-effort temp cleanup
}

// TestValidationExitCode verifies that all validation errors in CLI mode
// produce exit code 1. This is an end-to-end test using exec.Command.
func TestValidationExitCode(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, //nolint:gosec // test runs built binary
		binPath, "--json", "--searxng-url",
		"http://localhost:9999", "--pageno", "0", "test",
	)
	cmd.Dir = "."

	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("expected non-zero exit code for validation error, but process exited with 0")
	}

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1\noutput: %s", exitErr.ExitCode(), out)
	}

	if !strings.Contains(string(out), "validation error") {
		t.Errorf("output should contain 'validation error', got: %s", out)
	}
}

// TestMCPExitCode_StdinValidation verifies that the MCP server exits with
// the documented exit code 2 when stdin does not start with a valid MCP
// initialize message. CLI mode keeps exit code 1.
func TestMCPExitCode_StdinValidation(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	var stdin bytes.Buffer
	stdin.WriteString("not a valid MCP initialize message\n")

	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, binPath) //nolint:gosec // test runs built binary
	cmd.Stdin = &stdin
	cmd.Stderr = &stderr
	cmd.Env = os.Environ()

	err := cmd.Run()

	stderrStr := stderr.String()
	t.Logf("stderr:\n%s", stderrStr)
	t.Logf("exit error: %v", err)

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}

	if got := exitErr.ExitCode(); got != 2 {
		t.Errorf("exit code = %d, want 2 (MCP server error per documented contract)\nstderr:\n%s", got, stderrStr)
	}

	if !strings.Contains(stderrStr, "stdin does not contain a valid MCP initialize message") {
		t.Errorf("stderr should contain stdin validation error, got: %s", stderrStr)
	}
}
