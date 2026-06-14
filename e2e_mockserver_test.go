package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// mockServerResponse is a minimal valid SearXNG JSON response body used by
// mock-server network boundary tests.
const mockServerResponse = `{
	"query": "mock query",
	"number_of_results": 1,
	"results": [
		{
			"title": "Mock Result",
			"url": "https://example.com/mock",
			"content": "This is a mock result from the local test server.",
			"engine": "mock"
		}
	],
	"answers": [],
	"infoboxes": [],
	"suggestions": [],
	"unresponsive_engines": []
}`

// removeEnvVar returns a copy of env with the variable named key removed.
func removeEnvVar(env []string, key string) []string {
	var result []string

	prefix := key + "="
	for _, e := range env {
		if !strings.HasPrefix(e, prefix) {
			result = append(result, e)
		}
	}

	return result
}

// mockEnvWithout returns a copy of the current environment with the given
// keys removed.
func mockEnvWithout(keys ...string) []string {
	env := os.Environ()

	for _, key := range keys {
		env = removeEnvVar(env, key)
	}

	return env
}

// mockRunCLI runs the binary with the given environment and arguments and
// returns the combined stdout/stderr output and exit code.
func mockRunCLI(t *testing.T, binPath string, env []string, args ...string) (string, int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, args...) //nolint:gosec // test runs built binary
	cmd.Env = env
	cmd.Dir = "."

	out, err := cmd.CombinedOutput()

	var exitErr *exec.ExitError

	switch {
	case err == nil:
		return string(out), 0
	case errors.As(err, &exitErr):
		return string(out), exitErr.ExitCode()
	default:
		t.Fatalf("unexpected error type %T: %v\noutput: %s", err, err, out)

		return "", 0
	}
}

// TestMockServerNetworkBoundaries exercises network edge cases against a
// local httptest mock SearXNG server. No live external instance is required.
//
//nolint:gocognit,gocyclo,cyclop // table-driven test covering deterministic mock-server network boundary scenarios
func TestMockServerNetworkBoundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		args          func(url string) []string
		handler       func(t *testing.T) http.Handler
		wantCode      int
		wantOutput    string
		wantReqCount  int
		checkReqCount bool
	}{
		{
			name: "POST to GET fallback",
			args: func(url string) []string {
				return []string{
					"--allow-get-fallback",
					"--searxng-url", url,
					"--timeout", "5s",
					"--max-retries", "0",
					"test",
				}
			},
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				var count atomic.Int32

				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := count.Add(1)

					switch {
					case n == 1 && r.Method == http.MethodPost:
						w.WriteHeader(http.StatusMethodNotAllowed)

						return
					case n == 2 && r.Method == http.MethodGet:
						w.Header().Set("Content-Type", "application/json")

						_, err := w.Write([]byte(mockServerResponse))
						if err != nil {
							t.Logf("mock server write error: %v", err)
						}

						return
					default:
						t.Logf("unexpected request #%d method %s", n, r.Method)
						w.WriteHeader(http.StatusTeapot)
					}
				})
			},
			wantCode:      0,
			wantOutput:    "Mock Result",
			wantReqCount:  2,
			checkReqCount: true,
		},
		{
			name: "HTTP timeout with slow server",
			args: func(url string) []string {
				return []string{
					"--searxng-url", url,
					"--timeout", "500ms",
					"--max-retries", "0",
					"test",
				}
			},
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					// Sleep longer than the configured 500ms timeout. The value
					// is kept short so the test remains fast while still
					// reliably triggering a client-side timeout.
					time.Sleep(1200 * time.Millisecond)
					w.Header().Set("Content-Type", "application/json")

					_, err := w.Write([]byte(mockServerResponse))
					if err != nil {
						t.Logf("mock server write error: %v", err)
					}
				})
			},
			wantCode:   1,
			wantOutput: "search error",
		},
		{
			name: "retry succeeds after transient 500 errors",
			args: func(url string) []string {
				return []string{
					"--searxng-url", url,
					"--timeout", "5s",
					"--max-retries", "2",
					"test",
				}
			},
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				var count atomic.Int32

				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					n := count.Add(1)

					if r.Method != http.MethodPost {
						t.Logf("unexpected request method %s", r.Method)
						w.WriteHeader(http.StatusTeapot)

						return
					}

					if n <= 2 {
						w.WriteHeader(http.StatusInternalServerError)

						return
					}

					w.Header().Set("Content-Type", "application/json")

					_, err := w.Write([]byte(mockServerResponse))
					if err != nil {
						t.Logf("mock server write error: %v", err)
					}
				})
			},
			wantCode:      0,
			wantOutput:    "Mock Result",
			wantReqCount:  3,
			checkReqCount: true,
		},
		{
			name: "malformed JSON response returns parse error",
			args: func(url string) []string {
				return []string{
					"--searxng-url", url,
					"--timeout", "5s",
					"--max-retries", "0",
					"test",
				}
			},
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")

					_, err := w.Write([]byte("{invalid json"))
					if err != nil {
						t.Logf("mock server write error: %v", err)
					}
				})
			},
			wantCode:   1,
			wantOutput: "search error",
		},
		{
			name: "5xx server error with no retries",
			args: func(url string) []string {
				return []string{
					"--searxng-url", url,
					"--timeout", "5s",
					"--max-retries", "0",
					"test",
				}
			},
			handler: func(t *testing.T) http.Handler {
				t.Helper()

				return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusServiceUnavailable)

					_, err := w.Write([]byte(`{"error":"upstream unavailable"}`))
					if err != nil {
						t.Logf("mock server write error: %v", err)
					}
				})
			},
			wantCode:   1,
			wantOutput: "search error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var reqCount atomic.Int32

			baseHandler := tt.handler(t)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				reqCount.Add(1)
				baseHandler.ServeHTTP(w, r)
			}))
			defer server.Close()

			binPath, cleanup := buildTestBinary(t)
			defer cleanup()

			env := mockEnvWithout(
				"SEARXNG_URL",
				"SEARXNG_TIMEOUT",
				"SEARXNG_MAX_RETRIES",
				"SEARXNG_ALLOW_GET_FALLBACK",
			)

			out, code := mockRunCLI(t, binPath, env, tt.args(server.URL)...)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d\noutput: %s", code, tt.wantCode, out)
			}

			if !strings.Contains(out, tt.wantOutput) {
				t.Errorf("output should contain %q, got: %s", tt.wantOutput, out)
			}

			if tt.checkReqCount && int(reqCount.Load()) != tt.wantReqCount {
				t.Errorf("request count = %d, want %d", reqCount.Load(), tt.wantReqCount)
			}
		})
	}
}
