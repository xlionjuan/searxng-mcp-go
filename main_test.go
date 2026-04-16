package main

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"
)

// --- printCLIHelp tests ---

func TestPrintCLIHelp(t *testing.T) {
	// Capture stdout
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

	// Verify help content includes expected sections
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

// --- runCLIMode integration test notes ---
// runCLIMode() is challenging to unit test directly because:
// 1. It calls os.Exit() on errors, which terminates the test process
// 2. It uses global flag variables (*cliHelp, *cliVersion, *cliQuery, etc.)
// 3. It makes HTTP calls to SearXNG
//
// Integration test approach (as a separate binary or script):
// - Build the binary: go build -o searxng-mcp-go .
// - Test help: ./searxng-mcp-go --help (check exit code 0 and help output)
// - Test version: ./searxng-mcp-go --version (check exit code 0 and version output)
// - Test query with mock server:
//   - Start a mock SearXNG server on localhost
//   - Run: ./searxng-mcp-go --searxng-url=http://localhost:port "test query"
//   - Verify search results output or JSON format
// - Test missing query: ./searxng-mcp-go (check exit code 1 and error message)
//
// Example integration test using exec.Command in Go:
//
// func TestRunCLIModeIntegration(t *testing.T) {
//     // Test --help flag
//     cmd := exec.Command("./searxng-mcp-go", "--help")
//     output, err := cmd.Output()
//     if err != nil {
//         t.Fatalf("expected no error for --help, got: %v", err)
//     }
//     if !strings.Contains(string(output), "SearXNG MCP Server") {
//         t.Errorf("expected help header in output")
//     }
// }

func resetCLIFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	*cliQuery = ""
	*cliHelp = false
	*cliVersion = false
	*cliJSON = false
	*cliLanguage = "en"
	*cliSafeSearch = 0
	*cliTimeRange = ""
	*cliCategories = ""
	*cliEngines = ""
	*cliPageno = 1
	*cliSearXNGURL = ""
}

// --- runCLIMode tests ---

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_MissingQuery tests that runCLIMode returns an error when no query is provided
// instead of calling os.Exit
func TestRunCLIMode_MissingQuery(t *testing.T) {
	resetCLIFlags()

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected error for missing query, got nil")
	}
	if !strings.Contains(err.Error(), "search query is required") {
		t.Errorf("expected 'search query is required' error, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_ValidationError tests that runCLIMode returns validation errors
// instead of calling os.Exit
func TestRunCLIMode_ValidationError(t *testing.T) {
	resetCLIFlags()
	*cliSafeSearch = -1

	// Test with safesearch = -1 (invalid)
	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for safesearch=-1, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "safesearch") {
		t.Errorf("expected safesearch in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_InvalidTimeRange tests that invalid time_range returns validation error
func TestRunCLIMode_InvalidTimeRange(t *testing.T) {
	resetCLIFlags()
	*cliTimeRange = "invalid"

	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for invalid time_range, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "time_range") {
		t.Errorf("expected time_range in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_InvalidPageno tests that invalid pageno returns validation error
func TestRunCLIMode_InvalidPageno(t *testing.T) {
	resetCLIFlags()
	*cliPageno = 0

	err := runCLIMode([]string{"test query"})
	if err == nil {
		t.Fatal("expected validation error for pageno=0, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "pageno") {
		t.Errorf("expected pageno in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_QueryTooLong tests that query > 500 chars returns validation error
func TestRunCLIMode_QueryTooLong(t *testing.T) {
	resetCLIFlags()
	*cliQuery = strings.Repeat("a", 501)

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected validation error for query too long, got nil")
	}
	if !strings.Contains(err.Error(), "validation error") {
		t.Errorf("expected validation error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "500 characters") {
		t.Errorf("expected '500 characters' in error message, got: %v", err)
	}
}

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_HelpFlag tests that --help returns without error
func TestRunCLIMode_HelpFlag(t *testing.T) {
	resetCLIFlags()
	*cliHelp = true

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCLIMode([]string{})

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

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_VersionFlag tests that --version returns without error
func TestRunCLIMode_VersionFlag(t *testing.T) {
	resetCLIFlags()
	*cliVersion = true

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runCLIMode([]string{})

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

// NOTE: This test modifies global flag state and should not run in parallel
// with other tests that depend on flag state.
// TestRunCLIMode_SearchErrorReturnsError tests that search errors are returned as errors
// and not cause os.Exit to be called
func TestRunCLIMode_SearchErrorReturnsError(t *testing.T) {
	resetCLIFlags()
	*cliQuery = "test query"
	*cliSearXNGURL = "http://localhost:99999"
	*cliPageno = 1

	err := runCLIMode([]string{})
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	// Should NOT be a validation error - it should be a search error about invalid URL
	if !strings.Contains(err.Error(), "search error") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected search error with 'invalid', got: %v", err)
	}
}
