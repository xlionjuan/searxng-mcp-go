package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "1.0.0"

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
const searchInputSchema = `{
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
			"type": "integer",
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
	// Parse flags manually to handle flags appearing after positional arguments
	// Go's flag.Parse() stops at the first non-flag argument, so we need to
	// collect all flag arguments (starting with -) before the first positional arg
	// But flags that appear AFTER positional args should still be parsed if they start with -
	args := os.Args[1:]
	flagArgs := []string{}
	positionalArgs := []string{}
	seenPositional := false

	// Map of flags that require a value
	flagsWithValues := map[string]bool{
		"--query": true, "--searxng-url": true, "--language": true,
		"--safesearch": true, "--time_range": true, "--categories": true,
		"--engines": true, "--pageno": true,
	}

	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			// Explicit separator - treat everything after as positional
			seenPositional = true
			i++
			continue
		}
		if len(arg) > 0 && arg[0] == '-' {
			// Flag argument (starts with -)
			needsValue := flagsWithValues[arg]
			flagArgs = append(flagArgs, arg)
			if needsValue {
				// This flag needs a value - get the next arg if available
				if i+1 < len(args) && len(args[i+1]) > 0 && args[i+1][0] != '-' {
					// Next arg is a value (doesn't start with -)
					flagArgs = append(flagArgs, args[i+1])
					i += 2
				} else {
					// No value provided - let flag parser handle the error
					i++
				}
			} else {
				i++
			}
		} else if !seenPositional {
			// First positional argument
			seenPositional = true
			positionalArgs = append(positionalArgs, arg)
			i++
		} else {
			// After we've seen positional(s), treat non-flag args as positional too
			positionalArgs = append(positionalArgs, arg)
			i++
		}
	}

	// Parse only the flag arguments
	flag.CommandLine.Parse(flagArgs)

	// Check if we're in CLI mode (CLI-specific flags or positional args)
	isCLIMode := *cliHelp || *cliVersion || *cliQuery != "" || *cliJSON || len(positionalArgs) > 0

	if isCLIMode {
		if err := runCLIMode(positionalArgs); err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}
		return
	}

	// MCP stdio mode
	runMCPMode()
}

func getConfig() *Config {
	// Priority: flag > environment variable > default
	cfg := DefaultConfig()

	if *cliSearXNGURL != "" {
		cfg.SearXNGURL = *cliSearXNGURL
	} else {
		envURL := os.Getenv("SEARXNG_URL")
		if envURL != "" {
			cfg.SearXNGURL = envURL
		}
	}

	// Check if URL was explicitly provided (via env var or CLI flag)
	// If not, warn the user that they're using the default
	if *cliSearXNGURL == "" && os.Getenv("SEARXNG_URL") == "" {
		fmt.Fprintln(os.Stdout, "WARNING: No SearXNG server specified, using default server (https://search-4.xlion.dev). To use a different server, set the SEARXNG_URL environment variable or use the --searxng-url command line flag.")
	}

	return cfg
}

func runCLIMode(positionalArgs []string) error {
	if *cliHelp {
		printCLIHelp()
		return nil
	}

	if *cliVersion {
		fmt.Println("searxng-mcp-go version " + version)
		fmt.Println("SearXNG MCP Server - CLI + MCP stdio dual-mode")
		return nil
	}

	// Get query from flag or positional argument
	query := *cliQuery
	if query == "" && len(positionalArgs) > 0 {
		query = positionalArgs[0]
	}

	if query == "" {
		return fmt.Errorf("search query is required use --help for usage information")
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
		return fmt.Errorf("validation error: %v", err)
	}

	// Create searcher with configurable HTTP client
	searcher, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		return fmt.Errorf("failed to create searcher: %v", err)
	}

	ctx := context.Background()
	resp, err := searcher.Search(ctx, args)
	if err != nil {
		return fmt.Errorf("search error: %v", err)
	}

	if *cliJSON {
		// Output as JSON
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("failed to encode json: %v", err)
		}
	} else {
		// Output as human-readable text
		fmt.Print(formatResults(resp))
	}

	return nil
}

func runMCPMode() {
	cfg := getConfig()

	// Create searcher with configurable HTTP client
	searcher, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		log.Fatalf("Failed to create searcher: %v", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
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
					&mcp.TextContent{Text: fmt.Sprintf("validation error: %v", err)},
				},
				IsError: true,
			}, nil, nil
		}

		resp, err := searcher.Search(ctx, &args)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("Search error: %s", err.Error())},
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

	// Create context that listens for SIGINT/SIGTERM for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
