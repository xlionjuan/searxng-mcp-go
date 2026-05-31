//go:build e2e

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPFunctional(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}

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
					"query":      "golang",
					"safesearch": safesearch,
					"limit":      3,
				}, &stderr, "all safesearch levels")

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
					"query": "golang",
					"limit": 3,
				}
				if timeRange != "" {
					args["time_range"] = timeRange
				}
				response := requireSearchResponse(ctx, t, session, args, &stderr, "all time ranges")

				for i, result := range response.Results {
					if strings.TrimSpace(result.Title) == "" {
						t.Fatalf("time_range=%q result[%d] title is empty\nresponse: %#v\nstderr:\n%s", timeRange, i, response, stderr.String())
					}
				}
			})
		}
	})

	t.Run("all categories", func(t *testing.T) {
		for _, category := range []string{"general", "news", "music", "images", "videos", "science", "files", "it"} {
			category := category
			t.Run(category, func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":      "golang",
					"categories": category,
					"limit":      3,
				}, &stderr, "all categories")

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
					"query":   "golang",
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
					"query":  "golang",
					"pageno": pageno,
					"limit":  3,
				}, &stderr, "paginations")

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
					"query": "golang",
					"limit": limit,
				}, &stderr, "limit boundaries")

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
				"query":      "golang",
				"language":   "en",
				"categories": "general",
				"limit":      3,
			}, &stderr, "parameter combinations language+categories")

			for i, result := range response.Results {
				if strings.TrimSpace(result.Title) == "" {
					t.Fatalf("language+categories result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
				}
			}
		})

		t.Run("engines+time_range", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query":      "golang",
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
				"query":  "golang",
				"pageno": 2,
				"limit":  5,
			}, &stderr, "parameter combinations pageno+limit")

			if len(response.Results) > 5 {
				t.Fatalf("pageno+limit got %d results, want <= 5\nresponse: %#v\nstderr:\n%s", len(response.Results), response, stderr.String())
			}
			for i, result := range response.Results {
				if strings.TrimSpace(result.Title) == "" {
					t.Fatalf("pageno+limit result[%d] title is empty\nresponse: %#v\nstderr:\n%s", i, response, stderr.String())
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
		})

		t.Run("japanese", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "こんにちは",
				"limit": 3,
			}, &stderr, "unicode japanese query")

			if !strings.Contains(response.Query, "こんにちは") {
				t.Fatalf("japanese query not preserved in response: query=%q\nstderr:\n%s", response.Query, stderr.String())
			}
		})

		t.Run("emoji", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "🔍",
				"limit": 3,
			}, &stderr, "unicode emoji query")

			if strings.TrimSpace(response.Query) == "" {
				t.Fatalf("emoji query is empty in response\nresponse: %#v\nstderr:\n%s", response, stderr.String())
			}
		})
	})

	t.Run("response structure", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "golang",
			"limit": 10,
		}, &stderr, "response structure")

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

	t.Log("MCP functional tests completed")
}
