// TEST_OK
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// CLI Flags
// ============================================================================

// CLI-specific flags
var (
	cliQuery      = flag.String("query", "", "Search query string (CLI mode)")
	cliJSON       = flag.Bool("json", false, "Output results as JSON (CLI mode)")
	cliHelp       = flag.Bool("help", false, "Show this help message")
	cliVersion    = flag.Bool("version", false, "Show version information")
	cliSearXNGURL = flag.String("searxng-url", "", "SearXNG URL (can also be set via SEARXNG_URL env var)")
	cliLanguage   = flag.String("language", "en", "Language code for results (e.g., en, zh-tw, ja)")
	cliSafeSearch = flag.Int("safesearch", 0, "SafeSearch level: 0=Off, 1=Moderate, 2=Strict")
	cliTimeRange  = flag.String("time_range", "", "Time range filter: day, month, year")
	cliCategories = flag.String("categories", "", "Comma-separated list of categories to search")
	cliEngines    = flag.String("engines", "", "Comma-separated list of search engines to use")
	cliPageno     = flag.Int("pageno", 1, "Page number for pagination")
)

// ============================================================================
// Help and Usage
// ============================================================================

// usage prints the help message for CLI mode
func printCLIHelp() {
	fmt.Println(`SearXNG MCP Server - CLI Mode

A Model Context Protocol server that provides web search via SearXNG.

USAGE:
  searxng-mcp-go [OPTIONS] [QUERY]

OPTIONS:
  --query string     Search query string (alternative to positional argument)
  --json             Output results as formatted JSON instead of human-readable text
  --searxng-url URL  SearXNG instance URL (default: https://search-4.xlion.dev)
                     Can also be set via SEARXNG_URL environment variable
  --language LANG    Language code for results (e.g., en, zh-tw, ja) [default: en]
  --safesearch 0-2   SafeSearch level: 0=Off, 1=Moderate, 2=Strict [default: 0]
  --time_range RANGE Time range filter: day, month, year
  --categories CAT   Comma-separated list of categories to search
  --engines ENG      Comma-separated list of search engines to use
  --pageno N         Page number for pagination [default: 1]
  --help             Show this help message
  --version          Show version information

ARGUMENTS:
  QUERY              Search query string (if --query not specified)

OUTPUT:
  Results include title, URL, summary, publication date (if available), and engine source.

EXAMPLES:
  searxng-mcp-go "golang programming"
  searxng-mcp-go --query "web development" --json
  searxng-mcp-go --searxng-url https://my-searxng.example.com "search terms"
  searxng-mcp-go "news" --language en --time_range month --categories news

MCP MODE:
  When run without CLI arguments, the server starts in MCP stdio mode
  for communication with MCP clients like Claude Desktop or other AI tools.

EXIT CODES:
  0   Success
  1   Search error or invalid arguments
  2   MCP server error (in MCP mode)

For more information, see: https://github.com/xlionjuan/searxng-mcp-go`)
}

// searchInputSchema defines the JSON schema for the search tool input.
// Only "query" is required; all other parameters are optional.
var searchInputSchema = `{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query string"
		},
		"language": {
			"type": "string",
			"description": "Language code for results (e.g., en, zh-tw, ja). Defaults to en"
		},
		"safesearch": {
			"type": ["null", "integer"],
			"description": "SafeSearch level: 0=Off, 1=Moderate, 2=Strict. Defaults to 0"
		},
		"time_range": {
			"type": "string",
			"description": "Time range filter: day, month, year, or empty for all time"
		},
		"categories": {
			"type": "string",
			"description": "Comma-separated list of categories to search (e.g., general, news, music)"
		},
		"engines": {
			"type": "string",
			"description": "Comma-separated list of search engines to use (e.g., google, bing, duckduckgo)"
		},
		"pageno": {
			"type": ["null", "integer"],
			"description": "Page number for pagination. Defaults to 1"
		}
	},
	"required": ["query"]
}`

// ============================================================================
// Main Entry Point
// ============================================================================

func main() {
	// Reorder args: move all flags to the front, positional args to the back
	// This allows mixing flags and positional args in any order
	// e.g., searxng-mcp-go "test" --json will work correctly
	reorderedArgs := reorderArgs(os.Args[1:])
	flag.CommandLine.Parse(reorderedArgs)

	// Check if we're in CLI mode (any CLI-specific flag is set or non-flag args exist)
	isCLIMode := *cliHelp || *cliVersion || *cliQuery != "" || *cliJSON || flag.NFlag() > 0 || flag.NArg() > 0

	if isCLIMode {
		runCLIMode()
		return
	}

	// MCP stdio mode
	runMCPMode()
}

// reorderArgs reorders arguments so all flags come before positional arguments
// This allows commands like: searxng-mcp-go "test" --json
func reorderArgs(args []string) []string {
	var flags []string
	var positional []string

	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			flags = append(flags, arg)
		} else {
			positional = append(positional, arg)
		}
	}

	result := append(flags, positional...)
	return result
}

func getConfig() *Config {
	// Priority: flag > environment variable > default
	url := getEnv("SEARXNG_URL", "https://search-4.xlion.dev")
	if *cliSearXNGURL != "" {
		url = *cliSearXNGURL
	}

	return &Config{
		SearXNGURL: url,
		Timeout:    30 * time.Second,
	}
}

func runCLIMode() {
	if *cliHelp {
		printCLIHelp()
		return
	}

	if *cliVersion {
		fmt.Println("searxng-mcp-go version 1.0.0")
		fmt.Println("SearXNG MCP Server - CLI + MCP stdio dual-mode")
		return
	}

	// Get query from flag or positional argument
	query := *cliQuery
	if query == "" && flag.NArg() > 0 {
		query = flag.Arg(0)
	}

	if query == "" {
		fmt.Fprintln(os.Stderr, "Error: search query is required")
		fmt.Fprintln(os.Stderr, "Use --help for usage information")
		os.Exit(1)
	}

	cfg := getConfig()
	args := &SearchArgs{
		Query:      query,
		Language:   *cliLanguage,
		SafeSearch: *cliSafeSearch,
		TimeRange:  *cliTimeRange,
		Categories: *cliCategories,
		Engines:    *cliEngines,
		Pageno:     cliPageno,
	}

	// Validate arguments
	if err := ValidateSearchArgs(args); err != nil {
		fmt.Fprintf(os.Stderr, "Validation error: %v\n", err)
		os.Exit(1)
	}

	// Create searcher with configurable HTTP client
	searcher := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)

	ctx := context.Background()
	resp, err := searcher.Search(ctx, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Search error: %v\n", err)
		os.Exit(1)
	}

	if *cliJSON {
		// Output as JSON
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to encode JSON: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Output as human-readable text
		fmt.Print(formatResults(resp))
	}
}

func runMCPMode() {
	cfg := getConfig()

	// Create searcher with configurable HTTP client
	searcher := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: "1.0.0",
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG meta-search engine. Returns web results with titles, URLs, and summaries. Supports language filtering, category selection, engine selection, SafeSearch, time range restrictions, and pagination.",
		InputSchema: json.RawMessage(searchInputSchema),
	}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
		// Centralized validation
		if err := ValidateSearchArgs(&args); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Validation error: %v", err)},
				},
				IsError: true,
			}, nil, nil
		}

		resp, err := searcher.Search(ctx, &args)
		if err != nil {
			errMsg := err.Error()
			// Check for specific error types for better user feedback
			var ve *ValidationError
			if errors.As(err, &ve) {
				errMsg = fmt.Sprintf("Validation error: %v", err)
			}
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Search error: %s", errMsg)},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: formatResults(resp)},
			},
		}, nil, nil
	})

	log.Printf("Starting SearXNG MCP server...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
