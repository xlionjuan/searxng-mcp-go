package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

	"searxng-mcp-go/internal/searxng"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPrintCLIHelp(t *testing.T) {
	t.Parallel()

	output := captureStdout(t, func() {
		printCLIHelp()
	})

	expectedContent := []string{
		"SearXNG MCP Server - CLI Mode",
		"USAGE:",
		"OPTIONS:",
		"--query",
		"--json",
		"--searxng-url",
		"--language",
		"--safesearch",
		"--time_range",
		"--categories",
		"--engines",
		"--pageno",
		"--limit",
		"--debug",
		"--allow-get-fallback",
		"--help",
		"--version",
		"ARGUMENTS:",
		"QUERY",
		"OUTPUT:",
		"EXAMPLES:",
		"MCP MODE:",
		"EXIT CODES:",
		"https://github.com/xlionjuan/searxng-mcp-go",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(output, expected) {
			t.Errorf("printCLIHelp() output missing expected content %q", expected)
		}
	}
}

func TestParseArgs_InvalidFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		args      []string
		errSubstr string
	}{
		{name: "unknown flag", args: []string{"--unknown"}, errSubstr: "flag provided but not defined"},
		{name: "missing query value", args: []string{"--query"}, errSubstr: "flag needs an argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := parseArgs(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
			}
		})
	}
}

func TestParseArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		args           []string
		wantCLIMode    bool
		wantFlags      CLIFlags
		wantPositional []string
		wantLimit      *int
		wantErr        bool
		errSubstr      string
	}{
		{
			name:        "empty args",
			args:        []string{},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:           "--limit explicitly set",
			args:           []string{"--limit", "7", "test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"test query"},
			wantLimit:      new(7),
			wantErr:        false,
		},
		{
			name:           "positional query only",
			args:           []string{"test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"test query"},
			wantLimit:      new(searxng.DefaultResultLimit),
			wantErr:        false,
		},
		{
			name:        "--help flag",
			args:        []string{"--help"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Help: true, Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:        "--version flag",
			args:        []string{"--version"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Version: true, Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:        "--query flag",
			args:        []string{"--query", "my search"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Query: "my search", Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:        "--json flag",
			args:        []string{"--json"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{JSON: true, Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:        "searxng-url flag without query",
			args:        []string{"--searxng-url", "https://example.com"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{SearXNGURL: "https://example.com", Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:     false,
		},
		{
			name:        "language and safesearch flags without query",
			args:        []string{"--language", "ja", "--safesearch", "2"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Language: "ja", SafeSearch: 2, Pageno: nil},
			wantErr:     false,
		},
		{
			name:           "multiple flags with positional",
			args:           []string{"--language", "fr", "--safesearch", "1", "positional query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "fr", SafeSearch: 1, Pageno: nil},
			wantPositional: []string{"positional query"},
			wantErr:        false,
		},
		{
			name:           "flags after positional",
			args:           []string{"positional", "--json"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{JSON: true, Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"positional"},
			wantErr:        false,
		},
		{
			name:           "double dash separator treats subsequent args as positional",
			args:           []string{"--", "--help"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"--help"},
			wantErr:        false,
		},
		{
			name:           "double dash with leading dash query",
			args:           []string{"--", "-leading-dash-query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"-leading-dash-query"},
			wantErr:        false,
		},
		{
			name:        "server config flags without query",
			args:        []string{"--searxng-url", "https://example.com", "--language", "zh-tw", "--safesearch", "2"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{SearXNGURL: "https://example.com", Language: "zh-tw", SafeSearch: 2, Pageno: nil},
			wantErr:     false,
		},
		{
			name:           "--debug flag with query",
			args:           []string{"--debug", "test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Debug: true, Language: "", SafeSearch: 0, Pageno: nil},
			wantPositional: []string{"test query"},
			wantErr:        false,
		},
		{
			name:           "--pageno explicitly set",
			args:           []string{"--pageno", "3", "test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: new(3)},
			wantPositional: []string{"test query"},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			isCLIMode, flags, positionalArgs, err := parseArgs(tt.args)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if isCLIMode != tt.wantCLIMode {
				t.Errorf("isCLIMode = %v, want %v", isCLIMode, tt.wantCLIMode)
			}

			if flags.Language != tt.wantFlags.Language {
				t.Errorf("flags.Language = %q, want %q", flags.Language, tt.wantFlags.Language)
			}

			if flags.SafeSearch != tt.wantFlags.SafeSearch {
				t.Errorf("flags.SafeSearch = %d, want %d", flags.SafeSearch, tt.wantFlags.SafeSearch)
			}

			if (flags.Pageno == nil) != (tt.wantFlags.Pageno == nil) ||
				(flags.Pageno != nil && tt.wantFlags.Pageno != nil && *flags.Pageno != *tt.wantFlags.Pageno) {
				t.Errorf("flags.Pageno = %v, want %v", flags.Pageno, tt.wantFlags.Pageno)
			}

			if tt.wantLimit != nil {
				if flags.Limit == nil {
					t.Fatal("flags.Limit = nil, want limit pointer")
				}

				if *flags.Limit != *tt.wantLimit {
					t.Errorf("flags.Limit = %d, want %d", *flags.Limit, *tt.wantLimit)
				}
			}

			if flags.Query != tt.wantFlags.Query {
				t.Errorf("flags.Query = %q, want %q", flags.Query, tt.wantFlags.Query)
			}

			if flags.Help != tt.wantFlags.Help {
				t.Errorf("flags.Help = %v, want %v", flags.Help, tt.wantFlags.Help)
			}

			if flags.Version != tt.wantFlags.Version {
				t.Errorf("flags.Version = %v, want %v", flags.Version, tt.wantFlags.Version)
			}

			if flags.JSON != tt.wantFlags.JSON {
				t.Errorf("flags.JSON = %v, want %v", flags.JSON, tt.wantFlags.JSON)
			}

			if flags.SearXNGURL != tt.wantFlags.SearXNGURL {
				t.Errorf("flags.SearXNGURL = %q, want %q", flags.SearXNGURL, tt.wantFlags.SearXNGURL)
			}

			if flags.TimeRange != tt.wantFlags.TimeRange {
				t.Errorf("flags.TimeRange = %q, want %q", flags.TimeRange, tt.wantFlags.TimeRange)
			}

			if flags.Categories != tt.wantFlags.Categories {
				t.Errorf("flags.Categories = %q, want %q", flags.Categories, tt.wantFlags.Categories)
			}

			if flags.Engines != tt.wantFlags.Engines {
				t.Errorf("flags.Engines = %q, want %q", flags.Engines, tt.wantFlags.Engines)
			}

			if flags.Debug != tt.wantFlags.Debug {
				t.Errorf("flags.Debug = %v, want %v", flags.Debug, tt.wantFlags.Debug)
			}

			if len(positionalArgs) != len(tt.wantPositional) {
				t.Errorf("positionalArgs = %v, want %v", positionalArgs, tt.wantPositional)
			}

			for i, pos := range positionalArgs {
				if i >= len(tt.wantPositional) {
					break
				}

				if pos != tt.wantPositional[i] {
					t.Errorf("positionalArgs[%d] = %q, want %q", i, pos, tt.wantPositional[i])
				}
			}
		})
	}
}

func TestIsValidMCPInitializeMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		line string
		want bool
	}{
		{name: "valid initialize", line: `{"jsonrpc":"2.0","method":"initialize","id":1}` + "\n", want: true},
		{name: "wrong method", line: `{"jsonrpc":"2.0","method":"tools/list"}` + "\n", want: false},
		{name: "wrong version", line: `{"jsonrpc":"1.0","method":"initialize"}` + "\n", want: false},
		{name: "not json", line: `hello` + "\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isValidMCPInitializeMessage([]byte(tt.line)); got != tt.want {
				t.Fatalf("isValidMCPInitializeMessage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPrepareMCPStdin(t *testing.T) {
	t.Parallel()

	input := `{"jsonrpc":"2.0","method":"initialize","id":1}` + "\n" + `{"jsonrpc":"2.0","method":"tools/list","id":2}` + "\n"

	stdin, err := prepareMCPStdin(strings.NewReader(input))
	if err != nil {
		t.Fatalf("prepareMCPStdin() returned error: %v", err)
	}

	got, err := io.ReadAll(stdin)
	if err != nil {
		t.Fatalf("failed to read prepared stdin: %v", err)
	}

	if string(got) != input {
		t.Fatalf("prepared stdin mismatch\nwant: %q\ngot:  %q", input, string(got))
	}
}

func TestPrepareMCPStdinAllowsLargePostInitializeTraffic(t *testing.T) {
	t.Parallel()

	initialize := `{"jsonrpc":"2.0","method":"initialize","id":1}` + "\n"
	laterTraffic := `{"jsonrpc":"2.0","method":"tools/call","id":2,"params":{"payload":"` +
		strings.Repeat("a", mcpInitializeMaxBytes+1024) + `"}}` + "\n"
	input := initialize + laterTraffic

	stdin, err := prepareMCPStdin(strings.NewReader(input))
	if err != nil {
		t.Fatalf("prepareMCPStdin() returned error: %v", err)
	}

	got, err := io.ReadAll(stdin)
	if err != nil {
		t.Fatalf("failed to read prepared stdin: %v", err)
	}

	if string(got) != input {
		t.Fatalf("prepared stdin mismatch\nwant length: %d\ngot length:  %d", len(input), len(got))
	}
}

func TestPrepareMCPStdinRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	_, err := prepareMCPStdin(strings.NewReader("not initialize\\n"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "stdin does not contain a valid MCP initialize message" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareMCPStdinRejectsOversizedInitializeLine(t *testing.T) {
	t.Parallel()

	input := `{"jsonrpc":"2.0","method":"initialize","padding":"` + strings.Repeat("a", mcpInitializeMaxBytes) + `"}` + "\n"

	_, err := prepareMCPStdin(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if err.Error() != "stdin does not contain a valid MCP initialize message" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	stdoutMu.Lock()
	defer stdoutMu.Unlock()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	func() {
		defer func() {
			_ = w.Close()
		}()

		fn()
	}()

	var buf bytes.Buffer

	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}

	return buf.String()
}

// stdoutMu serializes access to the process-global os.Stdout variable inside
// captureStdout. Without it, two tests that call captureStdout in parallel can
// race: one test's os.Stdout swap may capture output belonging to another
// test, or one test's own output may be captured by the other. The mutex keeps
// the invariant local to captureStdout so individual tests can keep
// t.Parallel() without coordinating at the call site.
var stdoutMu sync.Mutex

func TestGetConfig(t *testing.T) {
	t.Setenv("SEARXNG_URL", "https://env.example.com")

	t.Run("flag overrides env", func(t *testing.T) {
		cfg, err := getConfig(&CLIFlags{SearXNGURL: "https://flag.example.com"})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.SearXNGURL != "https://flag.example.com" {
			t.Fatalf("SearXNGURL = %q, want flag value", cfg.SearXNGURL)
		}
	})

	t.Run("env used when flag empty", func(t *testing.T) {
		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.SearXNGURL != "https://env.example.com" {
			t.Fatalf("SearXNGURL = %q, want env value", cfg.SearXNGURL)
		}
	})

	t.Run("timeout env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "250ms")

		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != 250*time.Millisecond {
			t.Fatalf("Timeout = %v, want 250ms", cfg.Timeout)
		}
	})

	t.Run("max retries env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_MAX_RETRIES", "3")

		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.MaxRetries != 3 {
			t.Fatalf("MaxRetries = %d, want 3", cfg.MaxRetries)
		}
	})

	t.Run("GET fallback env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "1")

		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = false, want true")
		}
	})

	t.Run("GET fallback env zero disables", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "0")

		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = true, want false")
		}
	})

	t.Run("cli flags override env", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "30s")
		t.Setenv("SEARXNG_MAX_RETRIES", "9")
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "0")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--timeout", "1500ms",
			"--max-retries", "4",
			"--allow-get-fallback",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		cfg, err := getConfig(flags)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != 1500*time.Millisecond {
			t.Fatalf("Timeout = %v, want 1500ms", cfg.Timeout)
		}

		if cfg.MaxRetries != 4 {
			t.Fatalf("MaxRetries = %d, want 4", cfg.MaxRetries)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = false, want true (CLI flag overrides env var)")
		}
	})

	t.Run("--allow-get-fallback flag without env enables fallback", func(t *testing.T) {
		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--allow-get-fallback",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		if !flags.AllowGETFallback {
			t.Fatal("flags.AllowGETFallback = false, want true")
		}

		cfg, err := getConfig(flags)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("cfg.AllowGETFallback = false, want true (CLI flag alone enables fallback)")
		}
	})

	t.Run("--allow-get-fallback absent preserves env value", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "1")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		if flags.AllowGETFallback {
			t.Fatal("flags.AllowGETFallback = true, want false (flag not passed)")
		}

		cfg, err := getConfig(flags)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("cfg.AllowGETFallback = false, want true (env=1, no flag)")
		}
	})

	t.Run("invalid env values fall back to defaults", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "not-a-duration")
		t.Setenv("SEARXNG_MAX_RETRIES", "-1")
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "true")

		cfg, err := getConfig(&CLIFlags{})
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != searxng.DefaultTimeout {
			t.Fatalf("Timeout = %v, want default %v", cfg.Timeout, searxng.DefaultTimeout)
		}

		if cfg.MaxRetries != searxng.DefaultMaxRetries {
			t.Fatalf("MaxRetries = %d, want default %d", cfg.MaxRetries, searxng.DefaultMaxRetries)
		}

		if cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = true, want false for invalid env value")
		}
	})

	t.Run("max retries zero flag disables retries", func(t *testing.T) {
		t.Setenv("SEARXNG_MAX_RETRIES", "7")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--max-retries", "0",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		cfg, err := getConfig(flags)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.MaxRetries != 0 {
			t.Fatalf("MaxRetries = %d, want 0", cfg.MaxRetries)
		}
	})

	t.Run("timeout zero flag overrides env", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "30s")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--timeout", "0",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		cfg, err := getConfig(flags)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != 0 {
			t.Fatalf("Timeout = %v, want 0", cfg.Timeout)
		}
	})

	t.Run("error when neither set", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "")

		_, err := getConfig(&CLIFlags{})
		if err == nil {
			t.Fatal("getConfig() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "SearXNG_URL") || !strings.Contains(err.Error(), "required") {
			t.Fatalf("getConfig() error = %q, want error mentioning 'SearXNG_URL' and 'required'", err.Error())
		}
	})
}

// TestRegisterFlagsDefaultPinning guards against the bug reported in
// xlionjuan/searxng-mcp-go issue: the Go flag package reports the literal
// flag default (flag.Lookup(...).DefValue), not the effective default. If
// the flag default diverges from searxng.DefaultTimeout / DefaultMaxRetries,
// programmatic introspection of the FlagSet contradicts the help text and
// any consumer relying on DefValue will see a stale value.
func TestRegisterFlagsDefaultPinning(t *testing.T) {
	t.Parallel()

	fs, _ := registerFlags()

	timeoutFlag := fs.Lookup("timeout")
	if timeoutFlag == nil {
		t.Fatal("timeout flag not registered")
	}

	if got, want := timeoutFlag.DefValue, searxng.DefaultTimeout.String(); got != want {
		t.Errorf("timeout DefValue = %q, want %q (effective default)", got, want)
	}

	maxRetriesFlag := fs.Lookup("max-retries")
	if maxRetriesFlag == nil {
		t.Fatal("max-retries flag not registered")
	}

	if got, want := maxRetriesFlag.DefValue, strconv.Itoa(searxng.DefaultMaxRetries); got != want {
		t.Errorf("max-retries DefValue = %q, want %q (effective default)", got, want)
	}
}

func TestRunCLIMode_Success(t *testing.T) {
	successResp := searxng.SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results:         []searxng.SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
	}

	tests := []struct {
		name       string
		resp       searxng.SearchResponse
		rawResp    string
		flags      *CLIFlags
		positional []string
		debug      bool
		check      func(t *testing.T, output string)
	}{
		{
			name:  "text output",
			resp:  successResp,
			flags: &CLIFlags{Query: "golang", Pageno: nil},
			check: func(t *testing.T, output string) {
				t.Helper()

				if !strings.Contains(output, "=== Results ===") || !strings.Contains(output, "Go") {
					t.Fatalf("unexpected text output: %q", output)
				}
			},
		},
		{
			name:  "json output",
			resp:  successResp,
			flags: &CLIFlags{Query: "golang", JSON: true, Pageno: nil},
			check: func(t *testing.T, output string) {
				t.Helper()

				var resp searxng.SearchResponse

				err := json.Unmarshal([]byte(output), &resp)
				if err != nil {
					t.Fatalf("output is not valid JSON: %v\n%s", err, output)
				}

				if resp.Query != "golang" || len(resp.Results) != 1 || resp.Results[0].Title != "Go" {
					t.Fatalf("unexpected JSON output: %+v", resp)
				}
			},
		},
		{
			name: "debug json includes unresponsive engines",
			rawResp: `{"query":"golang","number_of_results":1,` +
				`"results":[{"title":"Go","url":"https://go.dev","content":"Go language","engine":"google"}],` +
				`"suggestions":[],` +
				`"unresponsive_engines":[["brave","Suspended:\" too many \"requests"]]}`,
			flags: &CLIFlags{Query: "golang", JSON: true, Pageno: nil},
			debug: true,
			check: func(t *testing.T, output string) {
				t.Helper()

				if !strings.Contains(output, "unresponsive_engines") {
					t.Fatalf("expected debug JSON to include unresponsive_engines, got %q", output)
				}
			},
		},
		{
			name: "query flag wins over positional query",
			resp: searxng.SearchResponse{
				Query:           "flag query",
				NumberOfResults: 1,
				Results:         []searxng.SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
			},
			flags:      &CLIFlags{Query: "flag query", Pageno: nil},
			positional: []string{"positional query"},
			check: func(t *testing.T, output string) {
				t.Helper()

				if !strings.Contains(output, "flag query") {
					t.Fatalf("expected flag query to win, got %q", output)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.rawResp != "" {
				server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tt.rawResp))
				}))
			} else {
				server = newJSONTestServer(t, tt.resp)
			}
			defer server.Close()

			flags := *tt.flags
			flags.SearXNGURL = server.URL

			var err error

			output := captureStdout(t, func() {
				err = runCLIMode(tt.debug, &flags, tt.positional)
			})
			if err != nil {
				t.Fatalf("runCLIMode() error = %v", err)
			}

			tt.check(t, output)
		})
	}
}

func TestRunCLIMode_MultiplePositionalArgsError(t *testing.T) {
	t.Parallel()

	err := runCLIMode(false, &CLIFlags{Pageno: nil}, []string{"golang", "programming"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if !strings.Contains(err.Error(), "only one query is accepted") {
		t.Fatalf("error %q does not contain %q", err.Error(), "only one query is accepted")
	}

	if !strings.Contains(err.Error(), "use quotes") {
		t.Fatalf("error %q does not contain %q", err.Error(), "use quotes")
	}
}

func TestRunCLIMode_ValidationErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		flags     *CLIFlags
		query     []string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "missing query",
			flags:     &CLIFlags{Language: "", SafeSearch: 0, Pageno: nil},
			query:     []string{},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "safesearch out of range",
			flags:     &CLIFlags{Query: "test", SearXNGURL: "http://localhost:9999", Language: "", SafeSearch: -1, Pageno: nil},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "invalid time_range",
			flags:     &CLIFlags{Query: "test", SearXNGURL: "http://localhost:9999", Language: "", SafeSearch: 0, TimeRange: "invalid", Pageno: nil},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "pageno zero",
			flags:     &CLIFlags{Query: "test", SearXNGURL: "http://localhost:9999", Language: "", SafeSearch: 0, Pageno: new(0)},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "query too long",
			flags:     &CLIFlags{Query: strings.Repeat("a", 501), SearXNGURL: "http://localhost:9999", Language: "", SafeSearch: 0, Pageno: nil},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := runCLIMode(false, tt.flags, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRunCLIMode_HelpFlag(t *testing.T) {
	flags := &CLIFlags{Help: true, Language: "", SafeSearch: 0, Pageno: nil}

	output := captureStdout(t, func() {
		err := runCLIMode(false, flags, []string{})
		if err != nil {
			t.Errorf("runCLIMode() with --help should return nil, got: %v", err)
		}
	})

	if !strings.Contains(output, "SearXNG") {
		t.Errorf("expected help output to contain 'SearXNG', got output: %q", output)
	}

	if !strings.Contains(output, "USAGE:") {
		t.Errorf("expected help output to contain 'USAGE:', got output: %q", output)
	}
}

func TestRunCLIMode_VersionFlag(t *testing.T) {
	flags := &CLIFlags{Version: true, Language: "", SafeSearch: 0, Pageno: nil}

	output := captureStdout(t, func() {
		err := runCLIMode(false, flags, []string{})
		if err != nil {
			t.Errorf("runCLIMode() with --version should return nil, got: %v", err)
		}
	})

	if !strings.Contains(output, "version") {
		t.Errorf("expected version output to contain 'version', got output: %q", output)
	}

	if !strings.Contains(output, "searxng-mcp-go") {
		t.Errorf("expected version output to contain 'searxng-mcp-go', got output: %q", output)
	}
}

func TestRunCLIMode_SearchErrorReturnsError(t *testing.T) {
	t.Parallel()

	flags := &CLIFlags{Query: "test", SearXNGURL: "http://localhost:99999", Language: "", SafeSearch: 0, Pageno: nil}

	err := runCLIMode(false, flags, []string{})
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}

	if !strings.Contains(err.Error(), "search error") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected search error with 'invalid', got: %v", err)
	}
}

func TestRunCLIMode_FlagOnlyInvocations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		flags     *CLIFlags
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "language flag only - no query",
			flags:     &CLIFlags{Language: "ja", SafeSearch: 0, Pageno: nil},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "searxng-url flag only - no query",
			flags:     &CLIFlags{SearXNGURL: "https://example.com", Language: "", SafeSearch: 0, Pageno: nil},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "multiple flags only - no query",
			flags:     &CLIFlags{Language: "", SafeSearch: 1, Pageno: nil},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "all optional flags without query",
			flags:     &CLIFlags{Language: "zh-tw", SafeSearch: 2, TimeRange: "month", Categories: "general", Engines: "google", Pageno: nil},
			wantErr:   true,
			errSubstr: "search query is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := runCLIMode(false, tt.flags, []string{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}

				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
