package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintCLIHelp(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printCLIHelp()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err := buf.ReadFrom(r)
	if err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
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
		"--debug",
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

// buildTestBinary compiles the binary for exit code testing.
// Returns the path to the binary and a cleanup function.
func buildTestBinary(t *testing.T) (string, func()) {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "searxng-mcp-go")
	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = "."
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build binary: %v\n%s", err, out)
	}
	return binPath, func() { os.Remove(binPath) }
}

// TestValidationExitCode verifies that all validation errors in CLI mode
// produce exit code 1. This is an end-to-end test using exec.Command.
func TestValidationExitCode(t *testing.T) {
	binPath, cleanup := buildTestBinary(t)
	defer cleanup()

	cmd := exec.Command(binPath, "--json", "--pageno", "0", "test")
	cmd.Dir = "."
	out, err := cmd.CombinedOutput()

	if err == nil {
		t.Fatal("expected non-zero exit code for validation error, but process exited with 0")
	}

	exitErr, ok := err.(*exec.ExitError)
	if !ok {
		t.Fatalf("expected ExitError, got %T: %v", err, err)
	}

	if exitErr.ExitCode() != 1 {
		t.Errorf("exit code = %d, want 1\noutput: %s", exitErr.ExitCode(), out)
	}

	if !strings.Contains(string(out), "validation error") {
		t.Errorf("output should contain 'validation error', got: %s", out)
	}
}

func TestParseArgs_InvalidFlags(t *testing.T) {
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
	tests := []struct {
		name           string
		args           []string
		wantCLIMode    bool
		wantFlags      CLIFlags
		wantPositional []string
		wantErr        bool
		errSubstr      string
	}{
		{
			name:        "empty args",
			args:        []string{},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:           "positional query only",
			args:           []string{"test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"test query"},
			wantErr:        false,
		},
		{
			name:        "--help flag",
			args:        []string{"--help"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Help: true, Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--version flag",
			args:        []string{"--version"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Version: true, Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--query flag",
			args:        []string{"--query", "my search"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Query: "my search", Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--json flag",
			args:        []string{"--json"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{JSON: true, Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--searxng-url flag alone is MCP mode with args - error",
			args:        []string{"--searxng-url", "https://example.com"},
			wantCLIMode: false,
			wantErr:     true,
			errSubstr:   "MCP stdin mode does not accept command-line arguments",
		},
		{
			name:        "--language and --safesearch flags in MCP mode",
			args:        []string{"--language", "ja", "--safesearch", "2"},
			wantCLIMode: false,
			wantErr:     true,
			errSubstr:   "MCP stdin mode does not accept command-line arguments",
		},
		{
			name:        "--debug flag in MCP mode",
			args:        []string{"--debug"},
			wantCLIMode: false,
			wantErr:     true,
			errSubstr:   "MCP stdin mode does not accept command-line arguments",
		},
		{
			name:           "multiple flags with positional",
			args:           []string{"--language", "fr", "--safesearch", "1", "positional query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "fr", SafeSearch: 1, Pageno: 1},
			wantPositional: []string{"positional query"},
			wantErr:        false,
		},
		{
			name:           "flags after positional",
			args:           []string{"positional", "--json"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{JSON: true, Language: "", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"positional"},
			wantErr:        false,
		},
		{
			name:           "double dash separator treats subsequent args as positional",
			args:           []string{"--", "--help"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"--help"},
			wantErr:        false,
		},
		{
			name:           "double dash with leading dash query",
			args:           []string{"--", "-leading-dash-query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"-leading-dash-query"},
			wantErr:        false,
		},
		{
			name:        "server config flags without query - MCP mode - error",
			args:        []string{"--searxng-url", "https://example.com", "--language", "zh-tw", "--safesearch", "2"},
			wantCLIMode: false,
			wantErr:     true,
			errSubstr:   "MCP stdin mode does not accept command-line arguments",
		},
		{
			name:           "--debug flag with query",
			args:           []string{"--debug", "test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Debug: true, Language: "", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"test query"},
			wantErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			if flags.Pageno != tt.wantFlags.Pageno {
				t.Errorf("flags.Pageno = %d, want %d", flags.Pageno, tt.wantFlags.Pageno)
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	fn()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatalf("failed to read stdout: %v", err)
	}
	return buf.String()
}

func TestGetConfig_PrecedenceAndDefaultWarning(t *testing.T) {
	t.Setenv("SEARXNG_URL", "https://env.example.com")

	t.Run("flag overrides env", func(t *testing.T) {
		cfg := getConfig(CLIFlags{SearXNGURL: "https://flag.example.com"})
		if cfg.SearXNGURL != "https://flag.example.com" {
			t.Fatalf("SearXNGURL = %q, want flag value", cfg.SearXNGURL)
		}
	})

	t.Run("env used when flag empty", func(t *testing.T) {
		cfg := getConfig(CLIFlags{})
		if cfg.SearXNGURL != "https://env.example.com" {
			t.Fatalf("SearXNGURL = %q, want env value", cfg.SearXNGURL)
		}
	})

	t.Run("default warning when neither set", func(t *testing.T) {
		t.Setenv("SEARXNG_URL", "")
		oldStderr := os.Stderr
		r, w, _ := os.Pipe()
		os.Stderr = w

		cfg := getConfig(CLIFlags{})

		w.Close()
		os.Stderr = oldStderr

		var buf bytes.Buffer
		if _, err := buf.ReadFrom(r); err != nil {
			t.Fatalf("failed to read stderr: %v", err)
		}
		if cfg.SearXNGURL != DefaultSearXNGURL {
			t.Fatalf("SearXNGURL = %q, want default %q", cfg.SearXNGURL, DefaultSearXNGURL)
		}
		if !strings.Contains(buf.String(), "WARN: No SearXNG server specified") {
			t.Fatalf("warning output missing, got %q", buf.String())
		}
	})
}

func TestRunCLIMode_SuccessTextOutput(t *testing.T) {
	server := newTestSearchServer(t, SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results:         []SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
	})

	output := captureStdout(t, func() {
		err := runCLIMode(CLIFlags{Query: "golang", SearXNGURL: server.URL, Pageno: 1}, nil)
		if err != nil {
			t.Fatalf("runCLIMode() error = %v", err)
		}
	})

	if !strings.Contains(output, "=== Results ===") || !strings.Contains(output, "Go") {
		t.Fatalf("unexpected text output: %q", output)
	}
}

func TestRunCLIMode_SuccessJSONOutput(t *testing.T) {
	server := newTestSearchServer(t, SearchResponse{
		Query:           "golang",
		NumberOfResults: 1,
		Results:         []SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
	})

	output := captureStdout(t, func() {
		err := runCLIMode(CLIFlags{Query: "golang", JSON: true, SearXNGURL: server.URL, Pageno: 1}, nil)
		if err != nil {
			t.Fatalf("runCLIMode() error = %v", err)
		}
	})

	var resp SearchResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, output)
	}
	if resp.Query != "golang" || len(resp.Results) != 1 || resp.Results[0].Title != "Go" {
		t.Fatalf("unexpected JSON output: %+v", resp)
	}
}

func TestRunCLIMode_QueryPrecedence(t *testing.T) {
	server := newTestSearchServer(t, SearchResponse{
		Query:           "flag query",
		NumberOfResults: 1,
		Results:         []SearchResult{{Title: "Go", URL: "https://go.dev", Content: "Go language", Engine: "google"}},
	})

	output := captureStdout(t, func() {
		err := runCLIMode(CLIFlags{Query: "flag query", SearXNGURL: server.URL, Pageno: 1}, []string{"positional query"})
		if err != nil {
			t.Fatalf("runCLIMode() error = %v", err)
		}
	})

	if !strings.Contains(output, "flag query") {
		t.Fatalf("expected flag query to win, got %q", output)
	}
}

func newTestSearchServer(t *testing.T, resp SearchResponse) *httptest.Server {
	t.Helper()
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	}))
}

func TestRunCLIMode_ValidationErrors(t *testing.T) {
	tests := []struct {
		name      string
		flags     CLIFlags
		query     []string
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "missing query",
			flags:     CLIFlags{Language: "", SafeSearch: 0, Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "safesearch out of range",
			flags:     CLIFlags{Query: "test", Language: "", SafeSearch: -1, Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "invalid time_range",
			flags:     CLIFlags{Query: "test", Language: "", SafeSearch: 0, TimeRange: "invalid", Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "pageno zero",
			flags:     CLIFlags{Query: "test", Language: "", SafeSearch: 0, Pageno: 0},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "query too long",
			flags:     CLIFlags{Query: strings.Repeat("a", 501), Language: "", SafeSearch: 0, Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCLIMode(tt.flags, tt.query)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestRunCLIMode_HelpFlag(t *testing.T) {
	flags := CLIFlags{Help: true, Language: "", SafeSearch: 0, Pageno: 1}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCLIMode(flags, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("runCLIMode() with --help should return nil, got: %v", err)
	}
	if !strings.Contains(output, "SearXNG") {
		t.Errorf("expected help output to contain 'SearXNG', got output: %q", output)
	}
	if !strings.Contains(output, "USAGE:") {
		t.Errorf("expected help output to contain 'USAGE:', got output: %q", output)
	}
}

func TestRunCLIMode_VersionFlag(t *testing.T) {
	flags := CLIFlags{Version: true, Language: "", SafeSearch: 0, Pageno: 1}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCLIMode(flags, []string{})

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	if err != nil {
		t.Errorf("runCLIMode() with --version should return nil, got: %v", err)
	}
	if !strings.Contains(output, "version") {
		t.Errorf("expected version output to contain 'version', got output: %q", output)
	}
	if !strings.Contains(output, "searxng-mcp-go") {
		t.Errorf("expected version output to contain 'searxng-mcp-go', got output: %q", output)
	}
}

func TestRunCLIMode_SearchErrorReturnsError(t *testing.T) {
	flags := CLIFlags{Query: "test", SearXNGURL: "http://localhost:99999", Language: "", SafeSearch: 0, Pageno: 1}

	err := runCLIMode(flags, []string{})
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "search error") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected search error with 'invalid', got: %v", err)
	}
}

func TestRunCLIMode_FlagOnlyInvocations(t *testing.T) {
	tests := []struct {
		name      string
		flags     CLIFlags
		wantErr   bool
		errSubstr string
	}{
		{
			name:      "language flag only - no query",
			flags:     CLIFlags{Language: "ja", SafeSearch: 0, Pageno: 1},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "searxng-url flag only - no query",
			flags:     CLIFlags{SearXNGURL: "https://example.com", Language: "", SafeSearch: 0, Pageno: 1},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "multiple flags only - no query",
			flags:     CLIFlags{Language: "", SafeSearch: 1, Pageno: 1},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "all optional flags without query",
			flags:     CLIFlags{Language: "zh-tw", SafeSearch: 2, TimeRange: "month", Categories: "general", Engines: "google", Pageno: 1},
			wantErr:   true,
			errSubstr: "search query is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runCLIMode(tt.flags, []string{})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}
