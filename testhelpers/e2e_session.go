// Package testhelpers provides shared utilities for E2E and integration tests.
package testhelpers

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	// defaultConnectTimeout is the default timeout for connecting to the MCP server.
	defaultConnectTimeout = 30 * time.Second
	// defaultSessionTimeout is the default timeout for the MCP session.
	defaultSessionTimeout = 180 * time.Second
	// defaultBuildTimeout is the timeout for building the MCP binary.
	defaultBuildTimeout = 60 * time.Second
	// defaultClientName is the default client name for E2E tests.
	defaultClientName = "searxng-mcp-go-e2e-test"
	// defaultVersion is the default version for E2E tests.
	defaultVersion = "test"
	// mcpBinaryName is the name of the MCP binary to build.
	mcpBinaryName = "searxng-mcp-go"
)

// E2EMCPConfig holds configuration for E2E MCP tests.
type E2EMCPConfig struct {
	BinaryPath     string
	SearXNGURL     string
	ExtraEnv       []string
	ConnectTimeout time.Duration
	SessionTimeout time.Duration
	ClientName     string
	Version        string
}

// E2ESession wraps an MCP client session with cleanup.
type E2ESession struct {
	Session *mcp.ClientSession
	Cmd     *exec.Cmd
	Stderr  *bytes.Buffer
	Cleanup func()
}

// ConnectMCPSession starts the MCP server binary and connects a client session.
// Returns the session and a cleanup function. The caller is responsible for
// calling the cleanup function (typically via t.Cleanup or defer).
func ConnectMCPSession(ctx context.Context, t *testing.T, cfg E2EMCPConfig) (*E2ESession, func()) {
	t.Helper()

	if cfg.ConnectTimeout == 0 {
		cfg.ConnectTimeout = defaultConnectTimeout
	}

	if cfg.SessionTimeout == 0 {
		cfg.SessionTimeout = defaultSessionTimeout
	}

	if cfg.ClientName == "" {
		cfg.ClientName = defaultClientName
	}

	if cfg.Version == "" {
		cfg.Version = defaultVersion
	}

	binaryPath := cfg.BinaryPath
	if binaryPath == "" {
		// Build binary in temp dir
		buildCtx, buildCancel := context.WithTimeout(ctx, defaultBuildTimeout)
		defer buildCancel()

		binaryPath = t.TempDir() + "/" + mcpBinaryName
		cmd := exec.CommandContext(buildCtx, "go", "build", "-o", binaryPath, ".") //nolint:gosec

		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build fallback MCP binary failed: %v\noutput:\n%s", err, string(output))
		}
	}

	searxngURL := cfg.SearXNGURL
	if searxngURL == "" {
		searxngURL = os.Getenv("SEARXNG_URL")
		if searxngURL == "" {
			t.Skip("SEARXNG_URL not set")
		}
	}

	var stderr bytes.Buffer

	subCtx, subCancel := context.WithTimeout(ctx, cfg.SessionTimeout)

	cmd := exec.CommandContext(subCtx, binaryPath) //nolint:gosec

	cmd.Env = append(os.Environ(), "SEARXNG_URL="+searxngURL, "SEARXNG_MAX_RETRIES=2")
	cmd.Env = append(cmd.Env, cfg.ExtraEnv...)
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{
		Name:    cfg.ClientName,
		Version: cfg.Version,
	}, nil)

	session, err := client.Connect(subCtx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		subCancel()
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	t.Log("MCP stdio session connected")

	sess := &E2ESession{
		Session: session,
		Cmd:     cmd,
		Stderr:  &stderr,
	}

	cleanup := func() {
		if session != nil {
			closeErr := session.Close()
			if closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
				t.Logf("close MCP session: %v\nstderr:\n%s", closeErr, stderr.String())
			}
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}

		subCancel()
	}

	return sess, cleanup
}

// E2EMCPEnv returns the standard environment variables for E2E MCP tests.
func E2EMCPEnv(searxngURL string, extra ...string) []string {
	env := append(os.Environ(), "SEARXNG_URL="+searxngURL, "SEARXNG_MAX_RETRIES=2")
	env = append(env, extra...)

	return env
}
