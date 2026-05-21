package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"searxng-mcp-go/internal/searxng"
)

var (
	errMultipleQueries     = errors.New("only one query is accepted; use quotes for multi-word queries")
	errSearchQueryRequired = errors.New("search query is required; use --help for usage information")
	errConfigurationFailed = errors.New("configuration error")
	errSearchValidation    = errors.New("validation error")
	errSearcherCreation    = errors.New("failed to create searcher")
	errSearchFailed        = errors.New("search error")
	errJSONEncodeFailed    = errors.New("failed to encode json")
)

// printCLIHelp prints the help message for CLI mode.
func printCLIHelp() {
	fmt.Println(`SearXNG MCP Server - CLI Mode (` + version + `)

A Model Context Protocol server that provides web search via SearXNG.

USAGE:
  searxng-mcp-go [OPTIONS] [QUERY]

OPTIONS:
  --query string     Search query string (alternative to positional argument)
  --json             Output results as formatted JSON instead of human-readable text
  --searxng-url URL  SearXNG instance URL (required)
                     Can also be set via SEARXNG_URL environment variable
  --language LANG    Language code for results (e.g., en, zh-tw, ja) [default: auto]
  --safesearch 0-2   SafeSearch level: 0=Off, 1=Moderate, 2=Strict [default: 0]
  --time_range RANGE Time range filter: day, month, year
  --categories CAT   Comma-separated list of categories to search
  --engines ENG      Comma-separated list of search engines to use
  --pageno N         Page number for pagination [default: 1]
  --limit N          Maximum number of results to return (1-20) [default: 10]
  --debug            Enable verbose HTTP request/response logging
                     Can also be enabled via DEBUG=1 environment variable
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

// runCLIMode executes the CLI-mode search flow.
func runCLIMode(flags CLIFlags, positionalArgs []string) error {
	if flags.Help {
		printCLIHelp()

		return nil
	}

	if flags.Version {
		fmt.Printf("searxng-mcp-go version %s (commit: %s, built: %s)\n", version, commit, date)
		fmt.Println("SearXNG MCP Server - CLI + MCP stdio dual-mode")

		return nil
	}

	query := flags.Query
	if query == "" {
		if len(positionalArgs) > 1 {
			return errMultipleQueries
		}

		if len(positionalArgs) > 0 {
			query = positionalArgs[0]
		}
	}

	if query == "" {
		return errSearchQueryRequired
	}

	cfg, err := getConfig(flags)
	if err != nil {
		return fmt.Errorf("%w: %w", errConfigurationFailed, err)
	}

	args := &searxng.SearchArgs{
		Query:      query,
		Language:   flags.Language,
		SafeSearch: flags.SafeSearch,
		TimeRange:  flags.TimeRange,
		Categories: flags.Categories,
		Engines:    flags.Engines,
		Pageno:     flags.Pageno,
		Limit:      flags.Limit,
	}

	err = searxng.ValidateSearchArgs(args)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearchValidation, err)
	}

	searcher, err := searxng.NewSearXNGSearcher(cfg, debugMode)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearcherCreation, err)
	}

	defer func() { _ = searcher.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), searxng.DefaultTimeout)
	defer cancel()

	resp, err := searcher.Search(ctx, args)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearchFailed, err)
	}

	if flags.JSON {
		if debugMode {
			fmt.Println()
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		err := enc.Encode(resp)
		if err != nil {
			return fmt.Errorf("%w: %w", errJSONEncodeFailed, err)
		}
	} else {
		fmt.Print(formatResults(resp))
	}

	return nil
}
