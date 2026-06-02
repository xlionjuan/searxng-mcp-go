//go:build e2e && stress

package main

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

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
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := connectMCPSession(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
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
		query := query
		wg.Add(1)
		go func() {
			defer wg.Done()

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
		}()
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
		i := i
		t.Run(fmt.Sprintf("session_%d", i), func(t *testing.T) {
			subCtx, subCancel := context.WithTimeout(ctx, 30*time.Second)
			defer subCancel()

			var stderr bytes.Buffer
			cmd := exec.CommandContext(subCtx, binaryPath)
			cmd.Env = e2eMCPEnv(searxngURL)
			cmd.Stderr = &stderr

			session := connectMCPSession(subCtx, t, cmd, &stderr)
			defer func() {
				if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
					t.Logf("session %d close: %v", i, closeErr)
				}
				if cmd.Process != nil && cmd.ProcessState == nil {
					_ = cmd.Process.Kill()
					_, _ = cmd.Process.Wait()
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

func TestMCPStress_RapidFire(t *testing.T) {
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

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binaryPath)
	cmd.Env = e2eMCPEnv(searxngURL)
	cmd.Stderr = &stderr

	session := connectMCPSession(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	queries := []string{
		"framework computer inc", "python", "rust", "javascript", "typescript",
		"kubernetes", "docker", "linux", "nginx", "redis",
		"postgresql", "mongodb", "elasticsearch", "kafka", "rabbitmq",
		"react", "vue", "angular", "svelte", "nextjs",
		"tensorflow", "pytorch", "jax", "scikit-learn", "pandas",
	}

	start := time.Now()
	var wg sync.WaitGroup
	errs := make(chan string, len(queries))

	for _, query := range queries {
		query := query
		wg.Add(1)
		go func() {
			defer wg.Done()

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
		}()
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

func TestMCPStress_Stability(t *testing.T) {
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

	session := connectMCPSession(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	queries := []string{
		"framework computer inc",
		"rust programming",
		"python async",
		"kubernetes",
		"machine learning",
	}

	for i, query := range queries {
		i := i
		query := query
		t.Run(fmt.Sprintf("search_%d", i), func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": query,
				"limit": 3,
			}, &stderr, fmt.Sprintf("stability search %d", i))

			if len(response.Results) == 0 {
				t.Fatalf("search %d (%q) returned 0 results\nstderr:\n%s", i, query, stderr.String())
			}

			for j, result := range response.Results {
				if strings.TrimSpace(result.Title) == "" {
					t.Fatalf("search %d result[%d] title is empty\nstderr:\n%s", i, j, stderr.String())
				}
			}

			// Wait 3 seconds between searches (but not after the last one)
			if i < len(queries)-1 {
				select {
				case <-ctx.Done():
					t.Fatalf("context cancelled during inter-search delay")
				case <-time.After(3 * time.Second):
				}
			}
		})
	}
}

func TestMCPStress_Randomized(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
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

	session := connectMCPSession(ctx, t, cmd, &stderr)
	defer func() {
		if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
			t.Logf("close MCP session: %v", closeErr)
		}
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	baseQueries := []string{
		"framework computer inc", "python", "rust", "java", "ruby",
		"kubernetes", "docker", "terraform", "ansible", "prometheus",
	}

	seed := resolveRandomSeed(t)
	t.Logf("randomized seed: %d (set E2E_RANDOM_SEED to replay)", seed)

	rng := rand.New(rand.NewSource(seed)) //nolint:gosec // Test randomization only.
	numSearches := 10
	searchArgs := make([]map[string]any, numSearches)
	searchQueries := make([]string, numSearches)
	for i := range numSearches {
		query := baseQueries[rng.Intn(len(baseQueries))]
		args := map[string]any{
			"query": query,
			"limit": 3,
		}

		// Randomly add optional parameters before launching goroutines.
		if rng.Intn(2) == 0 {
			args["safesearch"] = rng.Intn(3)
		}
		if rng.Intn(2) == 0 {
			timeRanges := []string{"day", "month", "year"}
			args["time_range"] = timeRanges[rng.Intn(len(timeRanges))]
		}

		searchArgs[i] = args
		searchQueries[i] = query
		t.Logf("randomized search %d: query=%q args=%#v", i, query, args)
	}

	var wg sync.WaitGroup
	errs := make(chan string, numSearches)

	for i := range numSearches {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()

			result, err := session.CallTool(ctx, &mcp.CallToolParams{
				Name:      "search",
				Arguments: searchArgs[i],
			})
			if err != nil {
				errs <- fmt.Sprintf("tools/call search failed for %q (args=%#v): %v", searchQueries[i], searchArgs[i], err)
				return
			}
			if result.IsError {
				text, ok := toolTextFromResult(result)
				if !ok {
					errs <- fmt.Sprintf("search returned tool error with malformed content for %q (args=%#v)", searchQueries[i], searchArgs[i])
					return
				}
				errs <- fmt.Sprintf("search returned tool error for %q (args=%#v): %s", searchQueries[i], searchArgs[i], text)
				return
			}
		}()
	}

	wg.Wait()
	close(errs)

	failCount := 0
	for errText := range errs {
		t.Errorf("%s\nstderr:\n%s", errText, stderr.String())
		failCount++
	}

	if failCount > 0 {
		t.Fatalf("%d randomized searches failed (seed=%d)", failCount, seed)
	}
	t.Logf("all %d randomized searches completed successfully (seed=%d)", numSearches, seed)
}

// resolveRandomSeed returns the random seed for stress tests. It honors the
// E2E_RANDOM_SEED env var so a failed run can be reproduced by re-running the
// test with the same value. An invalid value fails the test; when the env var
// is unset, a fresh nanosecond seed is generated automatically.
func resolveRandomSeed(t *testing.T) int64 {
	t.Helper()

	if raw := os.Getenv("E2E_RANDOM_SEED"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			t.Fatalf("E2E_RANDOM_SEED=%q is not a valid int64: %v", raw, err)
		}

		return parsed
	}

	return time.Now().UnixNano()
}

// connectMCPSession builds the MCP client and connects to a stdio-based MCP server.
func connectMCPSession(ctx context.Context, t *testing.T, cmd *exec.Cmd, stderr *bytes.Buffer) *mcp.ClientSession {
	t.Helper()

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-stress-test",
		Version: version,
	}, nil)

	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}

	return session
}
