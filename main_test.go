package main

import (
	"bytes"
	"os"
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

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		wantCLIMode    bool
		wantFlags      CLIFlags
		wantPositional []string
		wantErr        bool
	}{
		{
			name:        "empty args",
			args:        []string{},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:           "positional query only",
			args:           []string{"test query"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Language: "en", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"test query"},
			wantErr:        false,
		},
		{
			name:        "--help flag",
			args:        []string{"--help"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Help: true, Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--version flag",
			args:        []string{"--version"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Version: true, Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--query flag",
			args:        []string{"--query", "my search"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{Query: "my search", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--json flag",
			args:        []string{"--json"},
			wantCLIMode: true,
			wantFlags:   CLIFlags{JSON: true, Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--searxng-url flag",
			args:        []string{"--searxng-url", "https://example.com"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{SearXNGURL: "https://example.com", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--language flag",
			args:        []string{"--language", "ja"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Language: "ja", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--safesearch flag",
			args:        []string{"--safesearch", "2"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{SafeSearch: 2, Language: "en", Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--time_range flag",
			args:        []string{"--time_range", "month"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{TimeRange: "month", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--categories flag",
			args:        []string{"--categories", "general,news"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Categories: "general,news", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--engines flag",
			args:        []string{"--engines", "google,bing"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Engines: "google,bing", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:     false,
		},
		{
			name:        "--pageno flag",
			args:        []string{"--pageno", "5"},
			wantCLIMode: false,
			wantFlags:   CLIFlags{Pageno: 5, Language: "en", SafeSearch: 0},
			wantErr:     false,
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
			wantFlags:      CLIFlags{JSON: true, Language: "en", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{"positional"},
			wantErr:        false,
		},
		{
			name:           "double dash separator treats subsequent dashes as flags",
			args:           []string{"--", "--help"},
			wantCLIMode:    true,
			wantFlags:      CLIFlags{Help: true, Language: "en", SafeSearch: 0, Pageno: 1},
			wantPositional: []string{},
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
			flags:     CLIFlags{Language: "en", SafeSearch: 0, Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "safesearch out of range",
			flags:     CLIFlags{Query: "test", Language: "en", SafeSearch: -1, Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "invalid time_range",
			flags:     CLIFlags{Query: "test", Language: "en", SafeSearch: 0, TimeRange: "invalid", Pageno: 1},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "pageno zero",
			flags:     CLIFlags{Query: "test", Language: "en", SafeSearch: 0, Pageno: 0},
			query:     []string{},
			wantErr:   true,
			errSubstr: "validation error",
		},
		{
			name:      "query too long",
			flags:     CLIFlags{Query: strings.Repeat("a", 501), Language: "en", SafeSearch: 0, Pageno: 1},
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
	flags := CLIFlags{Help: true, Language: "en", SafeSearch: 0, Pageno: 1}

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
	flags := CLIFlags{Version: true, Language: "en", SafeSearch: 0, Pageno: 1}

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
	flags := CLIFlags{Query: "test", SearXNGURL: "http://localhost:99999", Language: "en", SafeSearch: 0, Pageno: 1}

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
			flags:     CLIFlags{SearXNGURL: "https://example.com", Language: "en", SafeSearch: 0, Pageno: 1},
			wantErr:   true,
			errSubstr: "search query is required",
		},
		{
			name:      "multiple flags only - no query",
			flags:     CLIFlags{Language: "en", SafeSearch: 1, Pageno: 1},
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
