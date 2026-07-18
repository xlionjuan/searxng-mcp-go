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
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"searxng-mcp-go/internal/searxng"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestPrintParseError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "unknown flag", args: []string{"--unknown"}},
		{name: "missing query value", args: []string{"--query"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, _, _, err := parseArgs(tt.args)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			var buf bytes.Buffer
			printParseError(err, &buf)

			output := buf.String()

			if !strings.Contains(output, "ERROR:") {
				t.Error("output should contain error message")
			}

			if !strings.Contains(output, "SearXNG MCP Server") {
				t.Error("output should contain help text")
			}
		})
	}
}

func TestMainRoutesParseErrorToStderr(t *testing.T) {
	t.Parallel()

	// Capture os.Stderr via pipe.
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	oldStderr := os.Stderr
	os.Stderr = stderrW

	readCh := make(chan string)

	go func() {
		var buf bytes.Buffer

		_, readErr := buf.ReadFrom(stderrR)
		if readErr != nil {
			t.Errorf("failed to read from stderr pipe: %v", readErr)
		}

		readCh <- buf.String()
	}()

	// Call printParseError with os.Stderr — same routing as main().
	//nolint:dogsled // parseArgs returns 4 values; only error is needed
	_, _, _, parseErr := parseArgs([]string{"--unknown"})
	if parseErr == nil {
		t.Fatal("expected parse error, got nil")
	}

	// Capture stdout to verify nothing leaks there.
	stdout := captureStdout(t, func() {
		printParseError(parseErr, os.Stderr)
	})

	err = stderrW.Close()
	if err != nil {
		t.Errorf("failed to close stderr pipe: %v", err)
	}

	os.Stderr = oldStderr

	stderrOutput := <-readCh

	if stdout != "" {
		t.Error("printParseError should not write to stdout")
	}

	if !strings.Contains(stderrOutput, "ERROR:") {
		t.Error("stderr should contain error message")
	}

	if !strings.Contains(stderrOutput, "SearXNG MCP Server") {
		t.Error("stderr should contain help text")
	}
}

func TestPrintCLIHelp(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	printCLIHelp(&buf)
	output := buf.String()

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

//nolint:gocognit,gocyclo,cyclop,maintidx // comprehensive table-driven test covering all CLI flag parsing scenarios
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

func TestPrintParseErrorNoColor(t *testing.T) {
	// Not t.Parallel(): modifies global env.
	t.Setenv("NO_COLOR", "1")
	t.Setenv("TERM", "") // clear TERM so NO_COLOR is tested independently

	//nolint:dogsled // parseArgs returns 4 values; only error is needed
	_, _, _, err := parseArgs([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var buf bytes.Buffer
	printParseError(err, &buf)

	output := buf.String()

	if !strings.Contains(output, "ERROR:") {
		t.Error("output should contain error message")
	}

	if strings.Contains(output, "\033[31m") {
		t.Error("output should not contain ANSI escape codes when NO_COLOR is set")
	}
}

func TestPrintParseErrorTermDumb(t *testing.T) {
	// Not t.Parallel(): modifies global env.
	t.Setenv("TERM", "dumb")

	//nolint:dogsled // parseArgs returns 4 values; only error is needed
	_, _, _, err := parseArgs([]string{"--unknown"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var buf bytes.Buffer
	printParseError(err, &buf)

	output := buf.String()

	if !strings.Contains(output, "ERROR:") {
		t.Error("output should contain error message")
	}

	if strings.Contains(output, "\033[31m") {
		t.Error("output should not contain ANSI escape codes when TERM=dumb")
	}
}

func TestPrepareMCPStdin(t *testing.T) {
	t.Parallel()

	input := `{"jsonrpc":"2.0","method":"initialize","id":1}` + "\n" +
		`{"jsonrpc":"2.0","method":"tools/list","id":2}` + "\n"

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

	input := `{"jsonrpc":"2.0","method":"initialize","padding":"` +
		strings.Repeat("a", mcpInitializeMaxBytes) + `"}` + "\n"

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

	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("failed to create pipe for stdout capture: %v", pipeErr)
	}

	os.Stdout = w

	defer func() {
		os.Stdout = oldStdout
	}()

	fn()

	err := w.Close()
	if err != nil {
		t.Logf("warning: failed to close stdout pipe writer: %v", err)
	}

	var buf bytes.Buffer

	_, err = buf.ReadFrom(r)
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

//nolint:gocognit,gocyclo,cyclop,maintidx // comprehensive table-driven test covering all config resolution scenarios
func TestGetConfig(t *testing.T) {
	t.Setenv("SEARXNG_URL", "https://env.example.com")

	t.Run("flag overrides env", func(t *testing.T) {
		cfg, err := getConfig(&CLIFlags{SearXNGURL: "https://flag.example.com"}, false)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.SearXNGURL != "https://flag.example.com" {
			t.Fatalf("SearXNGURL = %q, want flag value", cfg.SearXNGURL)
		}
	})

	t.Run("env used when flag empty", func(t *testing.T) {
		cfg, err := getConfig(&CLIFlags{}, false)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.SearXNGURL != "https://env.example.com" {
			t.Fatalf("SearXNGURL = %q, want env value", cfg.SearXNGURL)
		}
	})

	t.Run("timeout env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "250ms")

		cfg, err := getConfig(&CLIFlags{}, false)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != 250*time.Millisecond {
			t.Fatalf("Timeout = %v, want 250ms", cfg.Timeout)
		}
	})

	t.Run("max retries env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_MAX_RETRIES", "3")

		cfg, err := getConfig(&CLIFlags{}, false)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.MaxRetries != 3 {
			t.Fatalf("MaxRetries = %d, want 3", cfg.MaxRetries)
		}
	})

	t.Run("GET fallback env parsed", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "1")

		cfg, err := getConfig(&CLIFlags{}, false)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = false, want true")
		}
	})

	t.Run("GET fallback env zero disables", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "0")

		cfg, err := getConfig(&CLIFlags{}, false)
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

		cfg, err := getConfig(flags, true)
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

		cfg, err := getConfig(flags, true)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("cfg.AllowGETFallback = false, want true (CLI flag alone enables fallback)")
		}
	})

	t.Run("--allow-get-fallback=false overrides env true", func(t *testing.T) {
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "1")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--allow-get-fallback=false",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		if !flags.AllowGETFallbackExplicit {
			t.Fatal("flags.AllowGETFallbackExplicit = false, want true (flag was explicitly set)")
		}

		if flags.AllowGETFallback {
			t.Fatal("flags.AllowGETFallback = true, want false (flag explicitly set to false)")
		}

		cfg, err := getConfig(flags, true)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.AllowGETFallback {
			t.Fatal("cfg.AllowGETFallback = true, want false (CLI false overrides env true)")
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

		cfg, err := getConfig(flags, true)
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

		cfg, err := getConfig(&CLIFlags{}, false)
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

	t.Run("env parse error still allows CLI flag override", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "not-a-duration")
		t.Setenv("SEARXNG_MAX_RETRIES", "abc")
		t.Setenv("SEARXNG_ALLOW_GET_FALLBACK", "true")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--timeout", "30s",
			"--max-retries", "10",
			"--allow-get-fallback",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		cfg, err := getConfig(flags, true)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.Timeout != 30*time.Second {
			t.Fatalf("Timeout = %v, want 30s", cfg.Timeout)
		}

		if cfg.MaxRetries != 10 {
			t.Fatalf("MaxRetries = %d, want 10", cfg.MaxRetries)
		}

		if !cfg.AllowGETFallback {
			t.Fatal("AllowGETFallback = false, want true (CLI flag overrides unparseable env)")
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

		cfg, err := getConfig(flags, true)
		if err != nil {
			t.Fatalf("getConfig() error = %v, want nil", err)
		}

		if cfg.MaxRetries != 0 {
			t.Fatalf("MaxRetries = %d, want 0", cfg.MaxRetries)
		}
	})

	t.Run("timeout zero flag rejected by SetTimeout", func(t *testing.T) {
		t.Setenv("SEARXNG_TIMEOUT", "30s")

		_, flags, _, err := parseArgs([]string{
			"--searxng-url", "https://flag.example.com",
			"--timeout", "0",
			"test query",
		})
		if err != nil {
			t.Fatalf("parseArgs() error = %v, want nil", err)
		}

		cfg, err := getConfig(flags, true)
		if err == nil {
			t.Fatal("getConfig() error = nil, want error (timeout=0 should be rejected)")
		}

		if cfg != nil {
			t.Fatal("getConfig() returned non-nil config on error")
		}
	})

	t.Run("error when neither set", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "")

		_, err := getConfig(&CLIFlags{}, false)
		if err == nil {
			t.Fatal("getConfig() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "SearXNG_URL") || !strings.Contains(err.Error(), "required") {
			t.Fatalf("getConfig() error = %q, want error mentioning 'SearXNG_URL' and 'required'", err.Error())
		}
	})

	t.Run("cli max-retries exceeds cap", func(t *testing.T) {
		_, err := getConfig(&CLIFlags{
			SearXNGURL: "https://example.com",
			MaxRetries: new(21),
		}, true)
		if err == nil {
			t.Fatal("getConfig() error = nil, want error for --max-retries=21")
		}

		if !strings.Contains(err.Error(), "max retries cannot exceed 20") {
			t.Fatalf("getConfig() error = %q, want mention of max retry cap", err.Error())
		}
	})

	t.Run("cli timeout negative", func(t *testing.T) {
		_, err := getConfig(&CLIFlags{
			SearXNGURL: "https://example.com",
			Timeout:    new(-1 * time.Second),
		}, true)
		if err == nil {
			t.Fatal("getConfig() error = nil, want error for --timeout=-1s")
		}

		if !strings.Contains(err.Error(), "timeout cannot be negative") {
			t.Fatalf("getConfig() error = %q, want mention of negative timeout", err.Error())
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

// TestRegisterFlagsIntDefaultsParse guards against the defensive-programming
// issue where a future ParamDef with GoType "int" and a non-integer Default
// would be silently coerced to 0 by registerFlags. Every int-typed ParamDef
// in searxng.SearchParams must have a Default that strconv.Atoi can parse
// without error; if a new parameter is added with a malformed default, this
// test fails before registerFlags ever runs (and, in production,
// registerFlags would panic with errParamDefaultNotInt).
func TestRegisterFlagsIntDefaultsParse(t *testing.T) {
	t.Parallel()

	for _, p := range searxng.SearchParams {
		if p.GoType != "int" {
			continue
		}

		_, err := strconv.Atoi(p.Default)
		if err != nil {
			t.Errorf("SearchParams[%q] has GoType %q but Default %q does not parse as int: %v",
				p.Name, p.GoType, p.Default, err)
		}
	}

	// Smoke-check the end-to-end registration path: calling registerFlags
	// must not panic. Today this is a no-op because all int defaults are
	// valid, but it is the path that would panic on a regression.
	_, _ = registerFlags()
}

func TestRunCLIMode_Success(t *testing.T) {
	successResp := searxng.SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results: []searxng.SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
		},
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
				Results: []searxng.SearchResult{
					{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"},
				},
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
					_, _ = w.Write([]byte(tt.rawResp)) //nolint:errcheck // test fixture write; failure does not affect test outcome
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
			name: "invalid time_range",
			flags: &CLIFlags{
				Query: "test", SearXNGURL: "http://localhost:9999",
				Language: "", SafeSearch: 0, TimeRange: "invalid", Pageno: nil,
			},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name: "pageno zero",
			flags: &CLIFlags{
				Query: "test", SearXNGURL: "http://localhost:9999",
				Language: "", SafeSearch: 0, Pageno: new(0),
			},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name: "query too long",
			flags: &CLIFlags{
				Query: strings.Repeat("a", 501), SearXNGURL: "http://localhost:9999",
				Language: "", SafeSearch: 0, Pageno: nil,
			},
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

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	server.Close()

	flags := &CLIFlags{Query: "test", SearXNGURL: server.URL, Language: "", SafeSearch: 0, Pageno: nil}

	err := runCLIMode(false, flags, []string{})
	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}

	if !strings.Contains(err.Error(), "search error") {
		t.Fatalf("expected error containing 'search error', got: %v", err)
	}
}

func TestRunCLIMode_TimeoutZeroRejected(t *testing.T) {
	t.Parallel()

	resp := searxng.SearchResponse{
		Query:           "test",
		NumberOfResults: 1,
		Results: []searxng.SearchResult{
			{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "test"},
		},
	}

	server := newJSONTestServer(t, resp)
	defer server.Close()

	timeout := time.Duration(0)
	flags := &CLIFlags{
		Query:   "test",
		Pageno:  nil,
		Timeout: &timeout,
	}
	flags.SearXNGURL = server.URL

	err := runCLIMode(false, flags, []string{})
	if err == nil {
		t.Fatal("runCLIMode() error = nil, want error (timeout=0 should be rejected)")
	}
}

func TestRunCLIMode_TimeoutExceeded(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer server.Close()

	timeout := 1 * time.Millisecond
	maxRetries := 0
	flags := &CLIFlags{
		Query:      "test",
		SearXNGURL: server.URL,
		Pageno:     nil,
		Timeout:    &timeout,
		MaxRetries: &maxRetries,
	}

	err := runCLIMode(false, flags, []string{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "search error") {
		t.Fatalf("expected 'search error' in error, got: %v", err)
	}
}

func TestRunCLIMode_TimeoutIsPerRequestNotOverall(t *testing.T) {
	t.Parallel()

	var attempts atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n == 1 {
			// Exceed the per-request timeout so the first attempt is
			// canceled. Under the old CLI behavior this would also have
			// exhausted the overall search deadline, preventing retries.
			time.Sleep(100 * time.Millisecond)

			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		//nolint:errcheck // test fixture write best-effort
		_, _ = w.Write([]byte(
			`{"query":"test","number_of_results":1,` +
				`"results":[{"title":"OK","url":"https://example.com","content":"ok","engine":"test"}],` +
				`"suggestions":[]}`))
	}))
	defer server.Close()

	// Per-request timeout is shorter than the first request sleep, but the
	// overall search is bounded by context.Background() (no deadline), so
	// retries can still succeed.
	timeout := 50 * time.Millisecond
	maxRetries := 1
	flags := &CLIFlags{
		Query:      "test",
		SearXNGURL: server.URL,
		Pageno:     nil,
		Timeout:    &timeout,
		MaxRetries: &maxRetries,
	}

	output := captureStdout(t, func() {
		err := runCLIMode(false, flags, []string{})
		if err != nil {
			t.Fatalf("runCLIMode() error = %v, want nil (timeout is per-request, retries should not be preempted)", err)
		}
	})

	if got := attempts.Load(); got != 2 {
		t.Fatalf("attempts = %d, want 2 (1 request timeout + 1 success)", got)
	}

	if !strings.Contains(output, "=== Results ===") {
		t.Fatalf("expected results output, got: %q", output)
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
			name: "all optional flags without query",
			flags: &CLIFlags{
				Language: "zh-tw", SafeSearch: 2, TimeRange: "month",
				Categories: "general", Engines: "google", Pageno: nil,
			},
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
