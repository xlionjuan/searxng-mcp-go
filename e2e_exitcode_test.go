//go:build e2e

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestBinary compiles the binary for exit code testing.
// Returns the path to the binary and a cleanup function.
func buildTestBinary(t *testing.T) (string, func()) {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "searxng-mcp-go")

	cmd := exec.Command( //nolint:gosec // test runs built binary
		"go", "build", "-o", binPath, ".",
	)

	cmd.Dir = "."

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}

	return binPath, func() { _ = os.Remove(binPath) }
}

// TestValidationExitCode verifies that all validation errors in CLI mode
// produce exit code 1. This is an end-to-end test using exec.Command.
func TestValidationExitCode(t *testing.T) {
	t.Parallel()

	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	cmd := exec.Command( //nolint:gosec // test runs built binary
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
