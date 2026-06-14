//go:build e2e

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"syscall"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startMCPLifecycleSession starts an MCP stdio session without registering
// automatic cleanup. The caller is responsible for closing the session.
// The subprocess lifecycle is managed by mcp.CommandTransport.
func startMCPLifecycleSession(
	ctx context.Context, t *testing.T, searxngURL string,
) (*mcp.ClientSession, *safeBuffer, *exec.Cmd) {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := newMCPSession(ctx, t, cmd, &stderr, "searxng-mcp-go-lifecycle-test")
	t.Log("MCP stdio lifecycle session connected")

	return session, &stderr, cmd
}

func TestMCPLifecycle_SIGINTGracefulShutdown(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, stderr, cmd := startMCPLifecycleSession(ctx, t, searxngURL)

	defer func() {
		if session != nil {
			_ = session.Close() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("send SIGINT to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	closeErr := session.Close()
	session = nil

	assertCleanSessionClose(t, closeErr, stderr, "SIGINT")
}

func TestMCPLifecycle_SIGTERMGracefulShutdown(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	session, stderr, cmd := startMCPLifecycleSession(ctx, t, searxngURL)

	defer func() {
		if session != nil {
			_ = session.Close() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	closeErr := session.Close()
	session = nil

	assertCleanSessionClose(t, closeErr, stderr, "SIGTERM")
}

func TestMCPLifecycle_InvalidJSONAfterInitialize(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	cmd, stdin, stderr := startRawMCPProcess(ctx, t, searxngURL)

	defer func() {
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // best-effort cleanup
			_, _ = cmd.Process.Wait() //nolint:errcheck // best-effort cleanup
		}
	}()

	sendInvalidJSONInput(t, stdin, stderr)

	waitErr := waitForRawProcessExit(ctx, t, cmd, stderr)
	assertAcceptableExitCode(t, waitErr, stderr, "invalid JSON", 0, 2)
}

// startRawMCPProcess builds and starts the MCP binary with a raw stdin pipe.
// The caller is responsible for process cleanup.
func startRawMCPProcess(
	ctx context.Context, t *testing.T, searxngURL string,
) (*exec.Cmd, io.WriteCloser, *safeBuffer) {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("start MCP process: %v", err)
	}

	return cmd, stdin, &stderr
}

// sendInvalidJSONInput writes a valid initialize message, waits briefly, then
// writes an invalid JSON line and closes stdin.
func sendInvalidJSONInput(t *testing.T, stdin io.WriteCloser, stderr *safeBuffer) {
	t.Helper()

	_, err := fmt.Fprint(stdin, validMCPInitialize)
	if err != nil {
		t.Fatalf("write initialize message: %v\nstderr:\n%s", err, stderr.String())
	}

	// Give the server time to finish startup and begin reading messages.
	time.Sleep(200 * time.Millisecond)

	_, err = fmt.Fprint(stdin, "this is not valid json\n")
	if err != nil {
		t.Fatalf("write invalid JSON: %v\nstderr:\n%s", err, stderr.String())
	}

	err = stdin.Close()
	if err != nil {
		t.Fatalf("close stdin: %v\nstderr:\n%s", err, stderr.String())
	}
}

// waitForRawProcessExit waits for a process started outside of
// mcp.CommandTransport to exit, failing the test on timeout.
func waitForRawProcessExit(
	ctx context.Context, t *testing.T, cmd *exec.Cmd, stderr *safeBuffer,
) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("timeout waiting for process to exit\nstderr:\n%s", stderr.String())

		return nil
	}
}

// assertAcceptableExitCode fails the test unless waitErr represents one of the
// allowed exit codes. An error other than *exec.ExitError is also a failure.
func assertAcceptableExitCode(
	t *testing.T, waitErr error, stderr *safeBuffer, label string, allowed ...int,
) {
	t.Helper()

	if waitErr == nil {
		return
	}

	var exitErr *exec.ExitError

	if !errors.As(waitErr, &exitErr) {
		t.Fatalf("unexpected wait error after %s: %v\nstderr:\n%s", label, waitErr, stderr.String())
	}

	code := exitErr.ExitCode()
	if slices.Contains(allowed, code) {
		return
	}

	t.Fatalf("process exited with code %d after %s, want one of %v\nstderr:\n%s",
		code, label, allowed, stderr.String())
}

// assertCleanSessionClose fails the test if closeErr indicates the subprocess
// did not exit cleanly after a signal. A nil error means exit code 0; an
// exec.ExitError with code 0 is also accepted.
func assertCleanSessionClose(
	t *testing.T, closeErr error, stderr *safeBuffer, label string,
) {
	t.Helper()

	if closeErr == nil {
		return
	}

	var exitErr *exec.ExitError

	if errors.As(closeErr, &exitErr) {
		if exitErr.ExitCode() == 0 {
			return
		}

		t.Fatalf("process exited with code %d after %s, want 0\nstderr:\n%s",
			exitErr.ExitCode(), label, stderr.String())
	}

	t.Fatalf("close MCP session after %s: %v\nstderr:\n%s", label, closeErr, stderr.String())
}
