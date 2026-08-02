//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"
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
		// Kill the subprocess only if it is still running (test failed).
		// Do NOT call session.Close or cmd.Wait here: the SDK's cleanup
		// goroutine calls pipeRWC.Close → cmd.Wait internally when the
		// process exits; we must not race with that.
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGINT)
	if err != nil {
		t.Fatalf("send SIGINT to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	// Wait for the session to close naturally.  The signal causes the server
	// to exit; the SDK picks up EOF on stdout, runs pipeRWC.Close (which
	// calls cmd.Wait), and closes the connection.  session.Wait returns the
	// result — without us calling cmd.Wait or session.Close from the test.
	waitErr := waitForSessionClose(ctx, t, session, stderr)
	assertCleanSessionClose(t, waitErr, stderr, "SIGINT")
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
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
		}
	}()

	err := cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		t.Fatalf("send SIGTERM to MCP process: %v\nstderr:\n%s", err, stderr.String())
	}

	waitErr := waitForSessionClose(ctx, t, session, stderr)
	assertCleanSessionClose(t, waitErr, stderr, "SIGTERM")
}

func TestMCPLifecycle_InvalidJSONAfterInitialize(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	process := startRawMCPProcess(ctx, t, searxngURL)

	defer func() {
		process.cleanup()
	}()

	sendInvalidJSONInput(t, process.stdin, process.stderr)

	waitErr := waitForRawProcessExit(ctx, t, process)
	assertAcceptableExitCode(t, waitErr, process.stderr, "invalid JSON", 0, 2)
}

// rawMCPResponse is the small JSON-RPC envelope needed by the first-message
// tests. Result is kept raw so each test can decode its method-specific shape.
type rawMCPResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

// rawMCPProcess owns the one cmd.Wait call for a raw MCP subprocess. Every
// caller observes waitDone and cleanup only kills an unfinished process before
// waiting for that same goroutine to finish.
type rawMCPProcess struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	stderr   *safeBuffer
	stdout   *safeBuffer
	waitDone chan struct{}
	waitErr  error
}

// runRawFirstMessageScenario starts the MCP binary over raw stdio, writes a
// first MCP request, waits for a complete JSON-RPC response line, closes stdin,
// and asserts a clean exit. The response envelope is returned for
// method-specific result assertions.
func runRawFirstMessageScenario(
	ctx context.Context, t *testing.T, searxngURL, msg, label string,
) rawMCPResponse {
	t.Helper()

	process := startRawMCPProcess(ctx, t, searxngURL)

	defer process.cleanup()

	_, err := fmt.Fprint(process.stdin, msg)
	if err != nil {
		t.Fatalf("write first message: %v\nstderr:\n%s", err, process.stderr.String())
	}

	// Wait for the server to answer before closing stdin; closing stdin too
	// early would race the asynchronous response write and lose the reply.
	response := waitForJSONRPCResponse(ctx, t, process)

	t.Logf("server stdout:\n%s", process.stdout.String())

	// Closing stdin makes the server see EOF after answering the request, so
	// the process exits cleanly with code 0.
	err = process.stdin.Close()
	if err != nil {
		t.Fatalf("close stdin: %v\nstderr:\n%s", err, process.stderr.String())
	}

	waitErr := waitForRawProcessExit(ctx, t, process)
	assertAcceptableExitCode(t, waitErr, process.stderr, label, 0)

	return response
}

func TestMCPFirstMessage_ServerDiscoverAccepted(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// A go-sdk v1.7.0+ client probes server/discover as its first message on
	// stdio (SEP-2575). The stdin gate must accept it.
	response := runRawFirstMessageScenario(
		ctx,
		t,
		searxngURL,
		validMCPDiscover,
		"server/discover first request",
	)

	var result struct {
		SupportedVersions []string `json:"supportedVersions"`
	}

	err := json.Unmarshal(response.Result, &result)
	if err != nil {
		t.Fatalf("server/discover result is invalid: %v\nresult:%s", err, response.Result)
	}

	if len(result.SupportedVersions) == 0 {
		t.Fatalf("server/discover supportedVersions is empty\nresult:%s", response.Result)
	}
}

func TestMCPFirstMessage_StatelessToolsListAccepted(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	// In the stateless protocol (>= 2026-07-28) there is no handshake: a
	// direct tools/list request carrying per-request _meta metadata is a valid
	// first message (SEP-2575).
	response := runRawFirstMessageScenario(
		ctx,
		t,
		searxngURL,
		validMCPToolsList,
		"stateless tools/list first request",
	)

	var result struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}

	err := json.Unmarshal(response.Result, &result)
	if err != nil {
		t.Fatalf("tools/list result is invalid: %v\nresult:%s", err, response.Result)
	}

	for _, tool := range result.Tools {
		if tool.Name == "search" {
			return
		}
	}

	t.Fatalf("tools/list result does not include search tool\nresult:%s", response.Result)
}

// waitForJSONRPCResponse waits for a complete newline-delimited JSON-RPC
// response. It fails immediately if the process exits before writing a line.
func waitForJSONRPCResponse(
	ctx context.Context, t *testing.T, process *rawMCPProcess,
) rawMCPResponse {
	t.Helper()

	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		out := process.stdout.String()

		line, _, found := strings.Cut(out, "\n")
		if found {
			return decodeJSONRPCResponse(t, strings.TrimSuffix(line, "\r"), process.stderr)
		}

		select {
		case <-process.waitDone:
			t.Fatalf("process exited before JSON-RPC response\nstdout:\n%s\nstderr:\n%s\nwait error: %v",
				process.stdout.String(), process.stderr.String(), process.waitErr)
		case <-ctx.Done():
			process.cleanup()
			t.Fatalf("timeout waiting for JSON-RPC response\nstdout:\n%s\nstderr:\n%s",
				process.stdout.String(), process.stderr.String())
		case <-deadline.C:
			process.cleanup()
			t.Fatalf("timeout waiting for JSON-RPC response\nstdout:\n%s\nstderr:\n%s",
				process.stdout.String(), process.stderr.String())
		case <-ticker.C:
		}
	}
}

// decodeJSONRPCResponse unmarshals and validates one complete JSON-RPC
// response line before method-specific result assertions run.
func decodeJSONRPCResponse(t *testing.T, line string, stderr *safeBuffer) rawMCPResponse {
	t.Helper()

	var response rawMCPResponse

	err := json.Unmarshal([]byte(line), &response)
	if err != nil {
		t.Fatalf("invalid JSON-RPC response: %v\nline:%s\nstderr:\n%s",
			err, line, stderr.String())
	}

	if response.JSONRPC != "2.0" {
		t.Fatalf("JSON-RPC version = %q, want %q\nline:%s",
			response.JSONRPC, "2.0", line)
	}

	if !bytes.Equal(bytes.TrimSpace(response.ID), []byte("1")) {
		t.Fatalf("JSON-RPC id = %s, want 1\nline:%s", response.ID, line)
	}

	if len(response.Error) > 0 {
		t.Fatalf("JSON-RPC response contains error: %s\nline:%s", response.Error, line)
	}

	if len(response.Result) == 0 || bytes.Equal(bytes.TrimSpace(response.Result), []byte("null")) {
		t.Fatalf("JSON-RPC response has no result\nline:%s", line)
	}

	return response
}

// startRawMCPProcess builds and starts the MCP binary with a raw stdin pipe,
// capturing stdout and stderr. It starts the sole cmd.Wait goroutine before
// returning; callers must observe waitDone instead of calling cmd.Wait.
func startRawMCPProcess(
	ctx context.Context, t *testing.T, searxngURL string,
) *rawMCPProcess {
	t.Helper()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	var stderr, stdout safeBuffer

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test runs built binary
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr
	cmd.Stdout = &stdout

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatalf("start MCP process: %v", err)
	}

	process := &rawMCPProcess{
		cmd:      cmd,
		stdin:    stdin,
		stderr:   &stderr,
		stdout:   &stdout,
		waitDone: make(chan struct{}),
	}
	go func() {
		process.waitErr = cmd.Wait()
		close(process.waitDone)
	}()

	return process
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

// waitForRawProcessExit waits for the sole cmd.Wait goroutine to finish,
// failing the test on timeout. It never calls cmd.Wait itself.
func waitForRawProcessExit(
	ctx context.Context, t *testing.T, process *rawMCPProcess,
) error {
	t.Helper()

	select {
	case <-process.waitDone:
		return process.waitErr
	case <-ctx.Done():
		process.cleanup()
		t.Fatalf("timeout waiting for process to exit\nstderr:\n%s", process.stderr.String())

		return nil
	}
}

// cleanup kills a still-running process, then waits for the sole cmd.Wait
// goroutine. It is safe to call after a normal wait or from a defer.
func (p *rawMCPProcess) cleanup() {
	select {
	case <-p.waitDone:
		return
	default:
	}

	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill() //nolint:errcheck // best-effort cleanup
	}

	<-p.waitDone
}

// waitForSessionClose waits for the MCP session to close naturally — the
// server exits (e.g. due to a signal), the SDK picks up EOF on stdout and
// cleans up the connection, then session.Wait returns.  Unlike
// waitForRawProcessExit this does NOT call cmd.Wait, avoiding a data race
// with the SDK's internal pipeRWC.Close → cmd.Wait path.
func waitForSessionClose(
	ctx context.Context, t *testing.T, session *mcp.ClientSession, stderr *safeBuffer,
) error {
	t.Helper()

	done := make(chan error, 1)
	go func() { done <- session.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		t.Fatalf("timeout waiting for session close after signal\nstderr:\n%s", stderr.String())

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
