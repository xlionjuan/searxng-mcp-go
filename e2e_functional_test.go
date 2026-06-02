//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

func TestMCPFunctional(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}
	var warnings []string

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
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

	var session *mcp.ClientSession
	t.Cleanup(func() {
		if session != nil {
			if closeErr := session.Close(); closeErr != nil && !strings.Contains(closeErr.Error(), "signal: terminated") {
				t.Logf("close MCP session: %v\nstderr:\n%s", closeErr, stderr.String())
			}
		}

		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "searxng-mcp-go-e2e-test",
		Version: version,
	}, nil)

	var err error
	session, err = client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect MCP stdio session failed: %v\nstderr:\n%s", err, stderr.String())
	}
	t.Log("MCP stdio session connected")

	_ = findSearchTool(ctx, t, session, &stderr)
	t.Log("search tool found")

	t.Run("all safesearch levels", func(t *testing.T) {
		for _, safesearch := range []int{0, 1, 2} {
			safesearch := safesearch
			t.Run(strconv.Itoa(safesearch), func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":      "framework computer inc",
					"safesearch": safesearch,
					"limit":      3,
				}, &stderr, "all safesearch levels")

				if len(response.Results) == 0 {
					warning := "safesearch=" + strconv.Itoa(safesearch) + " results length = 0"
					warnings = append(warnings, warning)
					t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
				}

				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("safesearch=%d result[%d] title is empty\nresponse: %#v\nstderr:\n%s", safesearch, i, response, stderr.String())
					}
				}
			})
		}
	})

	t.Run("all time ranges", func(t *testing.T) {
		for _, timeRange := range []string{"day", "month", "year", ""} {
			timeRange := timeRange
			name := timeRange
			if name == "" {
				name = "all"
			}
			t.Run(name, func(t *testing.T) {
				args := map[string]any{
					"query": "framework computer inc",
					"limit": 3,
				}
				if timeRange != "" {
					args["time_range"] = timeRange
				}
				response := requireSearchResponse(ctx, t, session, args, &stderr, "all time ranges")

				if len(response.Results) == 0 {
					if timeRange != "" {
						t.Logf("time_range=%q results length = 0 (persistent, expected)\nresponse: %#v\nstderr:\n%s", timeRange, response, stderr.String())
					} else {
						warning := "time_range=\"all\" results length = 0"
						warnings = append(warnings, warning)
						t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
					}
				} else {
					for i, result := range response.Results {
						if strings.TrimSpace(result.Title) == "" {
							t.Fatalf("time_range=%q result[%d] title is empty\nresponse: %#v\nstderr:\n%s", timeRange, i, response, stderr.String())
						}
					}
				}
			})
		}
	})

	t.Run("all categories", func(t *testing.T) {
		// NOTE: "files" is intentionally excluded — it consistently returns 0 results
		// for test queries on the CI SearXNG instance. Retry cannot fix this; it is not
		// transient. Do not add it back without ensuring the files category returns results.
		for _, category := range []string{"general", "news", "music", "images", "videos", "science", "it"} {
			category := category
			t.Run(category, func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":      "framework computer inc",
					"categories": category,
					"limit":      3,
				}, &stderr, "all categories")

				if len(response.Results) == 0 {
					warning := "categories=" + strconv.Quote(category) + " results length = 0"
					warnings = append(warnings, warning)
					t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
				}

				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("categories=%q result[%d] title is empty\nresponse: %#v\nstderr:\n%s", category, i, response, stderr.String())
					}
				}
			})
		}
	})

	t.Run("all engines", func(t *testing.T) {
		for _, engine := range []string{"google", "bing", "duckduckgo", "yahoo"} {
			engine := engine
			t.Run(engine, func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":   "framework computer inc",
					"engines": engine,
					"limit":   3,
				}, &stderr, "all engines")

				// Allow empty results — some engines may not return results
				// for certain queries due to rate limiting or content gaps,
				// but non-empty results must have valid titles.
				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("engines=%q result[%d] title is empty\nresponse: %#v\nstderr:\n%s", engine, i, response, stderr.String())
					}
				}
				t.Logf("engines=%q: got %d results", engine, len(response.Results))
			})
		}
	})

	t.Run("paginations", func(t *testing.T) {
		for _, pageno := range []int{1, 2, 3} {
			pageno := pageno
			t.Run(strconv.Itoa(pageno), func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":  "framework computer inc",
					"pageno": pageno,
					"limit":  3,
				}, &stderr, "paginations")

				if len(response.Results) == 0 {
					warning := "pageno=" + strconv.Itoa(pageno) + " results length = 0"
					warnings = append(warnings, warning)
					t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
				}

				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("pageno=%d result[%d] title is empty\nresponse: %#v\nstderr:\n%s", pageno, i, response, stderr.String())
					}
				}
			})
		}
	})

	t.Run("limit boundaries", func(t *testing.T) {
		for _, limit := range []int{1, 5, 10, 20} {
			limit := limit
			t.Run(strconv.Itoa(limit), func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query": "framework computer inc",
					"limit": limit,
				}, &stderr, "limit boundaries")

				if len(response.Results) == 0 {
					warning := "limit=" + strconv.Itoa(limit) + " got 0 results, want 1.." + strconv.Itoa(limit)
					warnings = append(warnings, warning)
					t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
				}
				if len(response.Results) > limit {
					t.Fatalf("limit=%d got %d results, want <= %d\nresponse: %#v\nstderr:\n%s", limit, len(response.Results), limit, response, stderr.String())
				}
				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("limit=%d result[%d] title is empty\nresponse: %#v\nstderr:\n%s", limit, i, response, stderr.String())
					}
				}
			})
		}
	})

	t.Run("parameter combinations", func(t *testing.T) {
		t.Run("language+categories", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query":      "framework computer inc",
				"language":   "en",
				"categories": "general",
				"limit":      3,
			}, &stderr, "parameter combinations language+categories")

			if len(response.Results) == 0 {
				warning := "language+categories results length = 0"
				warnings = append(warnings, warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}

			for i, result := range response.Results {
				if strings.TrimSpace(result.Title) == "" {
					t.Fatalf("language+categories result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
				}
			}
		})

		t.Run("engines+time_range", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query":      "framework computer inc",
				"engines":    "bing",
				"time_range": "month",
				"limit":      3,
			}, &stderr, "parameter combinations engines+time_range")

			// Allow empty results — specific engine + time range combinations
			// may return no results.
			for i, result := range response.Results {
				if strings.TrimSpace(result.Title) == "" {
					t.Fatalf("engines+time_range result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
				}
			}
			t.Logf("engines+time_range: got %d results", len(response.Results))
		})

		t.Run("pageno+limit", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query":  "framework computer inc",
				"pageno": 2,
				"limit":  5,
			}, &stderr, "parameter combinations pageno+limit")

			if len(response.Results) == 0 {
				t.Logf("pageno+limit results length = 0 (persistent, expected)\nresponse: %#v\nstderr:\n%s", response, stderr.String())
			} else if len(response.Results) > 5 {
				t.Fatalf("pageno+limit got %d results, want <= 5\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
			}
			if len(response.Results) > 0 {
				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("pageno+limit result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
					}
				}
			}
		})
	})

	t.Run("unicode and special characters", func(t *testing.T) {
		t.Run("chinese", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "你好世界",
				"limit": 3,
			}, &stderr, "unicode chinese query")

			if !strings.Contains(response.Query, "你好") {
				t.Fatalf("chinese query not preserved in response: query=%q\nstderr:\n%s", response.Query, stderr.String())
			}
			if len(response.Results) == 0 {
				warning := "chinese query results length = 0"
				warnings = append(warnings, warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})

		t.Run("japanese", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "こんにちは",
				"limit": 3,
			}, &stderr, "unicode japanese query")

			if !strings.Contains(response.Query, "こんにちは") {
				t.Fatalf("japanese query not preserved in response: query=%q\nstderr:\n%s", response.Query, stderr.String())
			}
			if len(response.Results) == 0 {
				warning := "japanese query results length = 0"
				warnings = append(warnings, warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})

		t.Run("emoji", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "🔍",
				"limit": 3,
			}, &stderr, "unicode emoji query")

			if len(response.Results) == 0 {
				warning := "emoji query results length = 0"
				warnings = append(warnings, warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})
	})

	t.Run("response structure", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "framework computer inc",
			"limit": 10,
		}, &stderr, "response structure")

		if len(response.Results) == 0 {
			warning := "response structure results length = 0"
			warnings = append(warnings, warning)
			t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
		}

		for i, result := range response.Results {
			if strings.TrimSpace(result.Title) == "" {
				t.Fatalf("result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			if strings.TrimSpace(result.URL) == "" {
				t.Fatalf("result[%d] URL is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			if strings.TrimSpace(result.Content) == "" {
				t.Fatalf("result[%d] content is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
			if strings.TrimSpace(result.Engine) == "" {
				t.Fatalf("result[%d] engine is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
			}
		}
	})

	if len(warnings) > 0 {
		t.Logf("--- WARNING SUMMARY ---")
		for _, warning := range warnings {
			t.Logf("  WARN: %s", warning)
		}
	}
	t.Log("MCP functional tests completed")
}

// TestCLISmoke exercises the binary in flag-driven (non-MCP) CLI mode.
// It is the Go E2E replacement for the two loose shell smoke assertions
// previously hosted in .github/workflows/e2e.yml:
//
//   - `--time_range year` previously used
//     `grep -Eq "Results|No results found" || echo "WARN:..."`, which
//     exits 0 even when neither pattern matches.
//   - `--safesearch 2 "test"` previously used
//     `grep -Eq "Results|No results found"`, which silently accepted
//     "No results found" without routing through the WARNING SUMMARY
//     path used by every other Go E2E test.
//
// Both cases now parse structured `--json` output and route empty
// Results into the WARNING SUMMARY at the end of the test, matching the
// AGENTS.md "E2E Tests" rules. Non-empty results are still asserted to
// have a title and URL.
func TestCLISmoke(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}
	var warnings []string

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using CLI smoke binary: %s", binaryPath)

	// runCLI executes the binary in CLI mode with --json, parses the
	// structured SearchResponse, and surfaces command failures / parse
	// errors as test failures. Each call uses a per-test timeout.
	runCLI := func(t *testing.T, name string, args ...string) searxng.SearchResponse {
		t.Helper()

		subCtx, subCancel := context.WithTimeout(ctx, 60*time.Second)
		defer subCancel()

		cliArgs := append([]string{"--json", "--searxng-url", searxngURL}, args...)

		cmd := exec.CommandContext(subCtx, binaryPath, cliArgs...) //nolint:gosec // test runs built binary
		cmd.Env = append(os.Environ(), "SEARXNG_MAX_RETRIES=2")

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: binary failed: %v\nstdout:\n%s\nstderr:\n%s",
				name, err, stdout.String(), stderr.String())
		}

		var response searxng.SearchResponse
		if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
			t.Fatalf("%s: --json output is not SearchResponse JSON: %v\nstdout:\n%s\nstderr:\n%s",
				name, err, stdout.String(), stderr.String())
		}

		t.Logf("%s parsed: query=%q, results=%d, answers=%d, infoboxes=%d",
			name, response.Query, len(response.Results), len(response.Answers), len(response.Infoboxes))

		return response
	}

	t.Run("time_range year", func(t *testing.T) {
		// Mirrors the previous E2E shell smoke for --time_range year.
		// The original shell step used `grep -Eq "Results|No results found" || echo "WARN:..."`,
		// which exits 0 even when neither pattern matches. This Go version
		// always parses structured output; an empty Results is recorded in
		// the WARNING SUMMARY instead of being silently swallowed.
		response := runCLI(t, "time_range year", "--time_range", "year", "golang")

		if len(response.Results) == 0 {
			warning := "time_range=year results length = 0"
			warnings = append(warnings, warning)
			t.Logf("%s\nresponse: %#v", warning, response)
		}

		for i, result := range response.Results {
			if strings.TrimSpace(result.Title) == "" {
				t.Fatalf("time_range=year result[%d] title is empty\nresponse: %#v", i, response)
			}
			if strings.TrimSpace(result.URL) == "" {
				t.Fatalf("time_range=year result[%d] URL is empty\nresponse: %#v", i, response)
			}
		}
	})

	t.Run("safesearch strict", func(t *testing.T) {
		// Mirrors the previous E2E shell smoke for --safesearch 2.
		// The original shell step accepted "No results found" without
		// routing through the WARNING SUMMARY path. This Go version
		// asserts non-empty Results (with a warning fallback) per
		// AGENTS.md "Core functional tests assert non-zero results".
		response := runCLI(t, "safesearch strict", "--safesearch", "2", "test")

		if len(response.Results) == 0 {
			warning := "safesearch=2 strict results length = 0"
			warnings = append(warnings, warning)
			t.Logf("%s\nresponse: %#v", warning, response)
		}

		for i, result := range response.Results {
			if strings.TrimSpace(result.Title) == "" {
				t.Fatalf("safesearch=2 result[%d] title is empty\nresponse: %#v", i, response)
			}
			if strings.TrimSpace(result.URL) == "" {
				t.Fatalf("safesearch=2 result[%d] URL is empty\nresponse: %#v", i, response)
			}
		}
	})

	if len(warnings) > 0 {
		t.Logf("--- WARNING SUMMARY ---")
		for _, warning := range warnings {
			t.Logf("  WARN: %s", warning)
		}
	}
	t.Log("CLI smoke tests completed")
}
