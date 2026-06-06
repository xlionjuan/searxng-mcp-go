//go:build e2e

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"

	"searxng-mcp-go/internal/searxng"
)

func TestMCPFunctional(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}
	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 180*time.Second)
	defer cancel()

	session, stderr, _ := startMCPSession(ctx, t, searxngURL)

	_ = findSearchTool(ctx, t, session, stderr)
	t.Log("search tool found")

	t.Run("all safesearch levels", func(t *testing.T) {
		for _, safesearch := range []int{0, 1, 2} {
			safesearch := safesearch
			t.Run(strconv.Itoa(safesearch), func(t *testing.T) {
				response := requireSearchResponse(ctx, t, session, map[string]any{
					"query":      "framework computer inc",
					"safesearch": safesearch,
					"limit":      3,
				}, stderr, "all safesearch levels")

				if len(response.Results) == 0 {
					warning := "safesearch=" + strconv.Itoa(safesearch) + " results length = 0"
					warnings.Add("%s", warning)
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
				response := requireSearchResponse(ctx, t, session, args, stderr, "all time ranges")

				if len(response.Results) == 0 {
					if timeRange != "" {
						t.Logf("time_range=%q results length = 0 (persistent, expected)\nresponse: %#v\nstderr:\n%s", timeRange, response, stderr.String())
					} else {
						warning := "time_range=\"all\" results length = 0"
						warnings.Add("%s", warning)
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
				}, stderr, "all categories")

				if len(response.Results) == 0 {
					warning := "categories=" + strconv.Quote(category) + " results length = 0"
					warnings.Add("%s", warning)
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
				}, stderr, "all engines")

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
				}, stderr, "paginations")

				if len(response.Results) == 0 {
					warning := "pageno=" + strconv.Itoa(pageno) + " results length = 0"
					warnings.Add("%s", warning)
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
				}, stderr, "limit boundaries")

				if len(response.Results) == 0 {
					warning := "limit=" + strconv.Itoa(limit) + " got 0 results, want 1.." + strconv.Itoa(limit)
					warnings.Add("%s", warning)
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
			}, stderr, "parameter combinations language+categories")

			if len(response.Results) == 0 {
				warning := "language+categories results length = 0"
				warnings.Add("%s", warning)
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
			}, stderr, "parameter combinations engines+time_range")

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
			}, stderr, "parameter combinations pageno+limit")

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
			}, stderr, "unicode chinese query")

			if !strings.Contains(response.Query, "你好") {
				t.Fatalf("chinese query not preserved in response: query=%q\nstderr:\n%s", response.Query, stderr.String())
			}
			if len(response.Results) == 0 {
				warning := "chinese query results length = 0"
				warnings.Add("%s", warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})

		t.Run("japanese", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "こんにちは",
				"limit": 3,
			}, stderr, "unicode japanese query")

			if !strings.Contains(response.Query, "こんにちは") {
				t.Fatalf("japanese query not preserved in response: query=%q\nstderr:\n%s", response.Query, stderr.String())
			}
			if len(response.Results) == 0 {
				warning := "japanese query results length = 0"
				warnings.Add("%s", warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})

		t.Run("emoji", func(t *testing.T) {
			response := requireSearchResponse(ctx, t, session, map[string]any{
				"query": "🔍",
				"limit": 3,
			}, stderr, "unicode emoji query")

			if len(response.Results) == 0 {
				warning := "emoji query results length = 0"
				warnings.Add("%s", warning)
				t.Logf("%s\nresponse: %#v\nstderr:\n%s", warning, response, stderr.String())
			}
		})
	})

	t.Run("response structure", func(t *testing.T) {
		response := requireSearchResponse(ctx, t, session, map[string]any{
			"query": "framework computer inc",
			"limit": 10,
		}, stderr, "response structure")

		if len(response.Results) == 0 {
			warning := "response structure results length = 0"
			warnings.Add("%s", warning)
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

	warnings.Report(t)
	t.Log("MCP functional tests completed")
}

// TestCLISmoke exercises the binary in flag-driven (non-MCP) CLI mode.
// It is the Go E2E replacement for the shell smoke steps previously hosted
// in .github/workflows/e2e.yml.
//
// The previous shell steps asserted live-server behavior with `grep`,
// `python3 -c` heredocs, and `|| echo "WARN:..."` fallbacks. None of those
// patterns survive AGENTS.md "E2E Tests" rules: grep / python3 assertions
// cannot route through the WARNING SUMMARY path, and `|| echo` makes the
// whole step exit 0 even when the expected evidence is missing.
//
// Every subtest here parses a structured `--json` SearchResponse so that
// "expected content missing" (zero Results, zero Answers, zero Infoboxes,
// or — for the language test — the CJK query not being preserved) routes
// into a single WARNING SUMMARY printed at the end of the test. Non-empty
// evidence is still asserted strictly (title/URL/answer shape, infobox
// presence, CJK preservation, etc.), so this test does not silently pass
// when the live server is broken in a way the WARNING SUMMARY cannot
// express — e.g. malformed `--json` output, or `--json "..."` returning a
// non-SearchResponse payload.
func TestCLISmoke(t *testing.T) {
	searxngURL := os.Getenv("SEARXNG_URL")
	if searxngURL == "" {
		t.Skip("SEARXNG_URL not set")
	}
	var warnings e2eWarnings

	ctx, cancel := context.WithTimeout(t.Context(), 600*time.Second)
	defer cancel()

	binaryPath := os.Getenv("E2E_MCP_BINARY")
	if binaryPath == "" {
		binaryPath = buildE2EMCPBinary(ctx, t)
	}
	t.Logf("using CLI smoke binary: %s", binaryPath)

	// runCLI executes the binary in CLI mode with --json, parses the
	// structured SearchResponse, and surfaces command failures / parse
	// errors as test failures. Each call uses a per-call timeout so a
	// stuck subprocess cannot exceed its budget.
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

		t.Logf("%s parsed: query=%q, results=%d, answers=%d, infoboxes=%d, suggestions=%d",
			name, response.Query, len(response.Results), len(response.Answers), len(response.Infoboxes), len(response.Suggestions))

		return response
	}

	// assertResultsNotEmpty records a WARNING SUMMARY entry when results
	// are empty and otherwise asserts that every result has a title and
	// URL. This mirrors the "core functional tests assert non-zero
	// results" rule from AGENTS.md.
	//
	// Per-result title/URL quality checks are routed to WARNING SUMMARY
	// rather than fatal: individual engines (e.g. Google News) can
	// occasionally return results with empty titles, which is an
	// upstream data-quality issue, not a binary bug. The shell smoke
	// steps this replaces only checked `grep -q "Results"`, so the
	// strict "every result has a title and URL" assertion would be a
	// new strictness level. The WARNING SUMMARY path keeps the signal
	// visible without turning a meta-search flakiness mode into a hard
	// CI failure.
	assertResultsNotEmpty := func(t *testing.T, name string, response searxng.SearchResponse) {
		t.Helper()
		if len(response.Results) == 0 {
			warnings.Add("%s results length = 0", name)
			t.Logf("%s results length = 0\nresponse: %#v", name, response)

			return
		}
		for i, result := range response.Results {
			if strings.TrimSpace(result.Title) == "" {
				warnings.Add("%s result[%d] title is empty", name, i)
				t.Logf("%s result[%d] title is empty\nresponse: %#v", name, i, response)
			}
			if strings.TrimSpace(result.URL) == "" {
				warnings.Add("%s result[%d] URL is empty", name, i)
				t.Logf("%s result[%d] URL is empty\nresponse: %#v", name, i, response)
			}
		}
	}

	// assertAnswerMatches records a WARNING SUMMARY entry when no answers
	// are returned and otherwise asserts that at least one answer matches
	// the provided regex. The regex is checked against the legacy
	// Answer.Answer field, which is the human-readable text SearXNG
	// answerers populate (e.g. "avg = 3", an IPv4 string, a UUID, a
	// timezone report). Typed-answer fallback (e.g. Weather with no
	// legacy text) is intentionally out of scope here — these subtests
	// are direct migrations of the previous shell smoke regex checks.
	assertAnswerMatches := func(t *testing.T, name string, response searxng.SearchResponse, pattern *regexp.Regexp) {
		t.Helper()
		if len(response.Answers) == 0 {
			warnings.Add("%s answers length = 0", name)
			t.Logf("%s answers length = 0\nresponse: %#v", name, response)

			return
		}
		for i, ans := range response.Answers {
			if pattern.MatchString(ans.Answer) {
				return
			}
			t.Logf("%s answer[%d]=%q did not match %s", name, i, ans.Answer, pattern)
		}
		t.Fatalf("%s: no answer matched %s\nresponse: %#v", name, pattern, response)
	}

	// query/format subtests — these were previously strict grep / python3
	// assertions in shell and are now WARNING-tolerated 0 results plus
	// strict non-empty shape checks.

	t.Run("basic query", func(t *testing.T) {
		// Mirrors: ./searxng-mcp-go "golang tutorial" → grep -q "Results"
		// The "Results" header is only present when the response has at
		// least one result, so the shell assertion is effectively a
		// "results non-empty" check. We make that intent explicit here
		// and route zero-result outcomes through WARNING SUMMARY.
		response := runCLI(t, "basic query", "golang tutorial")
		assertResultsNotEmpty(t, "basic query", response)
	})

	t.Run("json output", func(t *testing.T) {
		// Mirrors: --json "golang" → python3 assert len(d['results']) > 0
		// runCLI's JSON parse enforces "--json output is SearchResponse
		// JSON" structurally; we add the non-zero result check here.
		response := runCLI(t, "json output", "golang")
		assertResultsNotEmpty(t, "json output", response)
	})

	t.Run("infobox json", func(t *testing.T) {
		// Mirrors: --json "apple inc" →
		//   python3 assert isinstance(d.get('infoboxes'), list) and d['infoboxes']
		// Empty infoboxes are the typical flakiness case (engine may
		// rotate and drop the Wikipedia infobox), so we route them to
		// WARNING SUMMARY and otherwise require at least one infobox
		// with non-empty content.
		response := runCLI(t, "infobox json", "apple inc")

		if len(response.Infoboxes) == 0 {
			warnings.Add("infobox json infoboxes length = 0")
			t.Logf("infobox json infoboxes length = 0\nresponse: %#v", response)
		} else {
			for i, ib := range response.Infoboxes {
				if strings.TrimSpace(ib.Infobox) == "" && strings.TrimSpace(ib.Content) == "" {
					t.Fatalf("infobox json infobox[%d] has empty infobox and content\nresponse: %#v", i, response)
				}
			}
		}
	})

	t.Run("language parameter", func(t *testing.T) {
		// Mirrors: --language zh-tw "測試" →
		//   python3 assert any('\u4e00' <= c <= '\u9fff' for c in s)
		// The original check verified CJK characters appear in the
		// output. With --json the equivalent is: the response.Query
		// preserves the CJK input (and any CJK content in results).
		// If the response is empty (a known flakiness mode) we still
		// want to know the CJK query was preserved, which is itself
		// non-trivial because some pipelines normalize the query string
		// before sending it on the wire.
		response := runCLI(t, "language parameter", "--language", "zh-tw", "測試")

		if !containsCJK(response.Query) {
			t.Fatalf("language parameter response.Query=%q does not contain CJK characters\nresponse: %#v", response.Query, response)
		}

		assertResultsNotEmpty(t, "language parameter", response)
	})

	t.Run("time range year", func(t *testing.T) {
		// Mirrors: --time_range year "golang" →
		//   grep -Eq "Results|No results found" || echo "WARN:..."
		// The original `|| echo` made the step exit 0 even when neither
		// pattern matched; this Go version always parses structured
		// output and routes zero-result outcomes through WARNING
		// SUMMARY.
		response := runCLI(t, "time range year", "--time_range", "year", "golang")
		assertResultsNotEmpty(t, "time range year", response)
	})

	t.Run("categories news", func(t *testing.T) {
		// Mirrors: --categories news "news" → grep -q "Results"
		response := runCLI(t, "categories news", "--categories", "news", "news")
		assertResultsNotEmpty(t, "categories news", response)
	})

	t.Run("long query", func(t *testing.T) {
		// Mirrors: long query (golang + 35 "search" words) → grep "Results"
		// The shell step's only assertion was "no error and the output
		// mentions Results". The Go equivalent is: the binary must
		// accept the long query (validation cap is 500 runes), parse
		// the JSON, and either return results or surface the empty
		// case via WARNING SUMMARY. A validation error or non-JSON
		// output is a hard fail.
		words := []string{"golang"}
		for range 35 {
			words = append(words, "search")
		}

		response := runCLI(t, "long query", strings.Join(words, " "))

		if response.Query == "" {
			t.Fatalf("long query response.Query is empty\nresponse: %#v", response)
		}
		assertResultsNotEmpty(t, "long query", response)
	})

	t.Run("safesearch strict", func(t *testing.T) {
		// Mirrors: --safesearch 2 "test" →
		//   grep -Eq "Results|No results found"
		// The original silently accepted "No results found" without
		// WARNING SUMMARY. This Go version still tolerates 0 results
		// for the strict-safesearch case (it's the most likely to
		// return 0 on a meta-search backend) but routes it through the
		// shared WARNING SUMMARY path.
		response := runCLI(t, "safesearch strict", "--safesearch", "2", "test")
		assertResultsNotEmpty(t, "safesearch strict", response)
	})

	// answer regex subtests — these were previously `grep -q "Answers" +
	// grep -Eq <pattern>` pairs in shell. We replace both with a single
	// WARNING-tolerated 0-answers check plus a strict regex match
	// against the legacy Answer.Answer field.

	t.Run("sha512 answer", func(t *testing.T) {
		// Mirrors: "sha512 hello" → grep "Answers" + grep -Eq "[[:xdigit:]]{64,}"
		response := runCLI(t, "sha512 answer", "sha512 hello")
		assertAnswerMatches(t, "sha512 answer", response, regexp.MustCompile(`[[:xdigit:]]{64,}`))
	})

	t.Run("ip answer", func(t *testing.T) {
		// Mirrors: "ip" → grep "Answers" + grep -Eq "([0-9]{1,3}\.){3}[0-9]{1,3}"
		response := runCLI(t, "ip answer", "ip")
		assertAnswerMatches(t, "ip answer", response, regexp.MustCompile(`([0-9]{1,3}\.){3}[0-9]{1,3}`))
	})

	t.Run("random uuid answer", func(t *testing.T) {
		// Mirrors: "random uuid" →
		//   grep "Answers" + grep -Eiq "[0-9a-f]{8}-[0-9a-f]{4}-..."
		response := runCLI(t, "random uuid answer", "random uuid")
		assertAnswerMatches(t, "random uuid answer", response, regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`))
	})

	t.Run("berlin time answer", func(t *testing.T) {
		// Mirrors: "time Berlin" →
		//   grep "Answers" + grep -Eiq "Berlin|[0-9]{1,2}:[0-9]{2}"
		response := runCLI(t, "berlin time answer", "time Berlin")
		assertAnswerMatches(t, "berlin time answer", response, regexp.MustCompile(`(?i)Berlin|[0-9]{1,2}:[0-9]{2}`))
	})

	t.Run("average answer", func(t *testing.T) {
		// Mirrors: "avg 1 2 3 4 5" → grep "Answers" + grep -Eq "avg.*= 3"
		response := runCLI(t, "average answer", "avg 1 2 3 4 5")
		assertAnswerMatches(t, "average answer", response, regexp.MustCompile(`avg.*= 3`))
	})

	warnings.Report(t)
	t.Log("CLI smoke tests completed")
}

// containsCJK reports whether s contains any CJK Unified Ideograph.
// This is the Go equivalent of the shell-side
// `any('\u4e00' <= c <= '\u9fff' for c in s)` check used by the
// previous --language zh-tw "測試" smoke step.
func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}

	return false
}
