//go:build e2e && stress

// This file requires BOTH the `e2e` and `stress` build tags.
//
// The `stress` tag groups the in-process concurrency tests
// (concurrency_test.go) with the live-server E2E stress tests below.
// The `e2e` tag gates the live-server tests in this repo.
//
// The combined tag is intentional: it keeps the live-server stress suite
// out of the default `./...` runs and the in-process-only `go test
// -tags=stress ./...` invocations documented in AGENTS.md. Run with
// `-tags='e2e stress'` (or via the `e2e-stress` workflow / `just test-e2e`
// recipe) to include it.

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//nolint:gocognit // test: concurrent stress; complexity inherent in pattern
func TestMCPStress_Concurrent(t *testing.T) {
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

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test: binary from env/build is trusted
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := newMCPSession(ctx, t, cmd, &stderr, "searxng-mcp-go-stress-test")
	defer func() {
		closeErr := session.Close()
		if closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // test cleanup; fire-and-forget kill
			_, _ = cmd.Process.Wait() //nolint:errcheck // test cleanup; fire-and-forget wait
		}
	}()

	queries := []string{
		"framework computer inc",
		"rust programming",
		"python async",
		"kubernetes",
		"machine learning",
	}

	var wg sync.WaitGroup

	errs := make(chan string, len(queries))

	for _, query := range queries {
		wg.Go(func() {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": query,
					"limit": 3,
				},
			})
			if err != nil {
				errs <- fmt.Sprintf("tools/call search failed for %q: %v", query, err)

				return
			}

			if result.IsError {
				text, ok := toolTextFromResult(result)
				if !ok {
					errs <- fmt.Sprintf("search returned tool error with malformed content for %q", query)

					return
				}

				errs <- fmt.Sprintf("search returned tool error for %q: %s", query, text)

				return
			}
		})
	}

	wg.Wait()
	close(errs)

	failCount := 0

	for errText := range errs {
		t.Errorf("%s\nstderr:\n%s", errText, stderr.String())

		failCount++
	}

	if failCount > 0 {
		t.Fatalf("%d concurrent searches failed", failCount)
	}

	t.Logf("all %d concurrent searches completed successfully", len(queries))
}

func TestMCPStress_SequentialSessions(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}

	t.Logf("using MCP binary: %s", binaryPath)

	for i := range 10 {
		t.Run(fmt.Sprintf("session_%d", i), func(t *testing.T) {
			subCtx, subCancel := context.WithTimeout(ctx, 30*time.Second)
			defer subCancel()

			var stderr bytes.Buffer

			cmd := exec.CommandContext(subCtx, binaryPath) //nolint:gosec // test: binary from env/build is trusted
			cmd.Env = e2eMCPEnv(searxngURL)
			cmd.Stderr = &stderr

			session := newMCPSession(subCtx, t, cmd, &stderr, "searxng-mcp-go-stress-test")
			defer func() {
				closeErr := session.Close()
				if closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
					t.Logf("session %d close: %v", i, closeErr)
				}

				if cmd.Process != nil && cmd.ProcessState == nil {
					_ = cmd.Process.Kill()    //nolint:errcheck // test cleanup; fire-and-forget kill
					_, _ = cmd.Process.Wait() //nolint:errcheck // test cleanup; fire-and-forget wait
				}
			}()

			response := requireSearchResponse(subCtx, t, session, map[string]any{
				"query": "test",
				"limit": 3,
			}, &stderr, fmt.Sprintf("sequential session %d", i))

			t.Logf("session %d: got %d results", i, len(response.Results))
		})
	}
}

//nolint:gocognit // test: rapid-fire stress; complexity inherent in pattern
func TestMCPStress_RapidFire(t *testing.T) {
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

	cmd := exec.CommandContext(ctx, binaryPath) //nolint:gosec // test: binary from env/build is trusted
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := newMCPSession(ctx, t, cmd, &stderr, "searxng-mcp-go-stress-test")
	defer func() {
		closeErr := session.Close()
		if closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()    //nolint:errcheck // test cleanup; fire-and-forget kill
			_, _ = cmd.Process.Wait() //nolint:errcheck // test cleanup; fire-and-forget wait
		}
	}()

	queries := []string{
		"framework computer inc", "python", "rust", "javascript", "typescript",
		"kubernetes", "docker", "linux", "nginx", "redis",
	}

	start := time.Now()

	var wg sync.WaitGroup

	errs := make(chan string, len(queries))

	for _, query := range queries {
		wg.Go(func() {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name: "search",
				Arguments: map[string]any{
					"query": query,
					"limit": 3,
				},
			})
			if err != nil {
				errs <- fmt.Sprintf("tools/call search failed for %q: %v", query, err)

				return
			}

			if result.IsError {
				text, ok := toolTextFromResult(result)
				if !ok {
					errs <- fmt.Sprintf("search returned tool error with malformed content for %q", query)

					return
				}

				errs <- fmt.Sprintf("search returned tool error for %q: %s", query, text)

				return
			}
		})
	}

	wg.Wait()
	close(errs)

	elapsed := time.Since(start)

	failCount := 0

	for errText := range errs {
		t.Errorf("%s\nstderr:\n%s", errText, stderr.String())

		failCount++
	}

	throughput := float64(len(queries)) / elapsed.Seconds()
	t.Logf("RapidFire: %d requests in %v (%.1f req/s), %d failures",
		len(queries), elapsed.Round(time.Millisecond), throughput, failCount)

	if failCount > 0 {
		t.Fatalf("%d rapid fire requests failed", failCount)
	}
}
