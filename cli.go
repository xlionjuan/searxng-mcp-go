package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

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

// printCLIHelp writes the help message to the given writer.
func printCLIHelp(w io.Writer) {
	var b strings.Builder

	b.WriteString(`SearXNG MCP Server - CLI Mode (` + version + `)`)
	b.WriteString(`
A Model Context Protocol server that provides web search via SearXNG.

USAGE:
  searxng-mcp-go [OPTIONS] [QUERY]

OPTIONS:
  --json             Output results as formatted JSON instead of human-readable text
  --searxng-url URL  SearXNG instance URL (required)
                     Can also be set via SEARXNG_URL environment variable
`)

	for _, p := range searxng.SearchParams {
		b.WriteString(p.CLIHelpLine())
		b.WriteString("\n")
	}

	fmt.Fprintf(&b, `  --debug            Enable verbose HTTP request/response logging
                     Can also be enabled via DEBUG=1 environment variable
  --timeout DURATION HTTP client timeout (e.g., 8s) [default: %s]
                     Can also be set via SEARXNG_TIMEOUT environment variable
  --max-retries N    Max retries after initial search attempt [default: %d]
                     Can also be set via SEARXNG_MAX_RETRIES environment variable
  --allow-get-fallback
                     Enable POST→GET fallback for 405/501 responses (CLI mode)
                     Can also be set via SEARXNG_ALLOW_GET_FALLBACK environment variable
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
  When launched without CLI arguments by an MCP client, the server runs in
  MCP stdio mode. The client must send the MCP initialize message on stdin.

EXIT CODES:
  0   Success
  1   Search error or invalid arguments
  2   MCP server error (in MCP mode)

For more information, see: https://github.com/xlionjuan/searxng-mcp-go
`,
		searxng.DefaultTimeout, searxng.DefaultMaxRetries)

	_, err := w.Write([]byte(b.String()))
	if err != nil {
		slog.Warn("failed to write help text", "error", err)
	}
}

// runCLIMode executes the CLI-mode search flow.
//
//nolint:gocyclo // CLI dispatch (help/debug/search/format) is inherently sequential; extracting adds layers
func runCLIMode(debug bool, flags *CLIFlags, positionalArgs []string) error {
	if flags.Help {
		printCLIHelp(os.Stdout)

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

	cfg, err := getConfig(flags, true)
	if err != nil {
		return fmt.Errorf("%w: %w", errConfigurationFailed, err)
	}

	args := searxng.NewSearchArgs(query)
	args.Language = flags.Language
	args.SafeSearch = flags.SafeSearch
	args.TimeRange = flags.TimeRange
	args.Categories = flags.Categories
	args.Engines = flags.Engines
	args.Pageno = flags.Pageno
	// Limit is set by NewSearchArgs to DefaultResultLimit; override if
	// the CLI flag was explicitly provided (flags.Limit is always non-nil
	// after parseArgs because parseArgs fills the default when unset).
	if flags.Limit != nil {
		args.Limit = flags.Limit
	}

	args, err = searxng.ValidateSearchArgs(args)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearchValidation, err)
	}

	searcher, err := searxng.NewSearXNGSearcher(cfg, debug)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearcherCreation, err)
	}

	defer func() { _ = searcher.Close() }() //nolint:errcheck // cleanup in defer; error is non-actionable

	resp, err := searcher.Search(context.Background(), args)
	if err != nil {
		return fmt.Errorf("%w: %w", errSearchFailed, err)
	}

	if flags.JSON {
		if debug {
			fmt.Println()
		}

		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")

		err := enc.Encode(resp)
		if err != nil {
			return fmt.Errorf("%w: %w", errJSONEncodeFailed, err)
		}
	} else {
		logUnresponsiveEngines(slog.Default(), resp)
		fmt.Print(formatResults(resp))
	}

	return nil
}
