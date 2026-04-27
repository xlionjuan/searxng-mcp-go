package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const version = "1.0.0"

// debugMode is set to true when --debug flag or DEBUG=1 env var is active.
var debugMode bool

// ============================================================================
// CLIFlags holds parsed CLI flag values
type CLIFlags struct {
	Query      string
	JSON       bool
	Help       bool
	Version    bool
	SearXNGURL string
	Language   string
	SafeSearch int
	TimeRange  string
	Categories string
	Engines    string
	Pageno     int
	Debug      bool
}

// parseArgs parses command-line arguments and returns the mode, flags, and positional arguments.
// Any supplied arguments route the process into CLI mode; otherwise the server runs in MCP mode.
// Flags are accepted anywhere before or after positional args, matching the current CLI behavior.
func parseArgs(args []string) (isCLIMode bool, flags CLIFlags, positionalArgs []string, err error) {
	flagArgs := make([]string, 0, len(args))
	positionalArgs = make([]string, 0, len(args))
	afterDoubleDash := false

	flagsWithValues := map[string]bool{
		"--query": true, "--searxng-url": true, "--language": true,
		"--safesearch": true, "--time_range": true, "--categories": true,
		"--engines": true, "--pageno": true,
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			afterDoubleDash = true
			continue
		}
		if afterDoubleDash || !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
			continue
		}
		flagArgs = append(flagArgs, arg)
		if flagsWithValues[arg] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
			flagArgs = append(flagArgs, args[i])
		}
	}

	fs := flag.NewFlagSet("searxng-mcp-go", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	query := fs.String("query", "", "Search query string (CLI mode)")
	jsonOut := fs.Bool("json", false, "Output results as JSON (CLI mode)")
	help := fs.Bool("help", false, "Show this help message")
	versionFlag := fs.Bool("version", false, "Show version information")
	searxngURL := fs.String("searxng-url", "", "SearXNG URL (can also be set via SEARXNG_URL env var)")
	language := fs.String("language", "", "Language code for results (e.g., en, zh-tw, ja). Leave empty for auto")
	safeSearch := fs.Int("safesearch", 0, "SafeSearch level: 0=Off, 1=Moderate, 2=Strict")
	timeRange := fs.String("time_range", "", "Time range filter: day, month, year")
	categories := fs.String("categories", "", "Comma-separated list of categories to search")
	engines := fs.String("engines", "", "Comma-separated list of search engines to use")
	pageno := fs.Int("pageno", 1, "Page number for pagination")
	debug := fs.Bool("debug", false, "Enable verbose HTTP request/response logging (can also be set via DEBUG=1 env var)")

	if err := fs.Parse(flagArgs); err != nil {
		return false, CLIFlags{}, nil, err
	}

	flags = CLIFlags{
		Query:      *query,
		JSON:       *jsonOut,
		Help:       *help,
		Version:    *versionFlag,
		SearXNGURL: *searxngURL,
		Language:   *language,
		SafeSearch: *safeSearch,
		TimeRange:  *timeRange,
		Categories: *categories,
		Engines:    *engines,
		Pageno:     *pageno,
		Debug:      *debug,
	}

	isCLIMode = len(args) > 0 || flags.Help || flags.Version || flags.Query != "" || flags.JSON || len(positionalArgs) > 0

	return isCLIMode, flags, positionalArgs, nil
}

// ============================================================================
// Help and Usage
// ============================================================================

// usage prints the help message for CLI mode
func printCLIHelp() {
	fmt.Println(`SearXNG MCP Server - CLI Mode (v` + version + `)

A Model Context Protocol server that provides web search via SearXNG.

USAGE:
  searxng-mcp-go [OPTIONS] [QUERY]

OPTIONS:
  --query string     Search query string (alternative to positional argument)
  --json             Output results as formatted JSON instead of human-readable text
  --searxng-url URL  SearXNG instance URL (default: https://search-4.xlion.dev)
                     Can also be set via SEARXNG_URL environment variable
  --language LANG    Language code for results (e.g., en, zh-tw, ja) [default: auto]
  --safesearch 0-2   SafeSearch level: 0=Off, 1=Moderate, 2=Strict [default: 0]
  --time_range RANGE Time range filter: day, month, year
  --categories CAT   Comma-separated list of categories to search
  --engines ENG      Comma-separated list of search engines to use
  --pageno N         Page number for pagination [default: 1]
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
			"description": "Language code for results (e.g., en, zh-tw, ja). Defaults to auto (SearXNG decides)"
		},
		"safesearch": {
			"type": "integer",
			"description": "SafeSearch level: 0=Off, 1=Moderate, 2=Strict. Defaults to 0",
			"minimum": 0,
			"maximum": 2
		},
		"time_range": {
			"type": "string",
			"description": "Time range filter: day, month, year, or empty for all time",
			"enum": ["", "day", "month", "year"]
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
			"description": "Page number for pagination. Defaults to 1",
			"minimum": 1
		}
	},
	"required": ["query"],
	"additionalProperties": false
}`

// ============================================================================
// Main Entry Point
// ============================================================================

func main() {
	isCLIMode, flags, positionalArgs, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mERROR: %v\033[0m\n", err)
		fmt.Fprintln(os.Stderr, "")
		printCLIHelp()
		os.Exit(1)
	}

	// Enable debug mode via --debug flag or DEBUG=1 env var
	debugMode = flags.Debug || os.Getenv("DEBUG") == "1"
	if debugMode {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("debug mode enabled", "version", version)
		slog.Warn("debug mode logs search queries and HTTP requests in plain text; avoid sensitive searches")
	}

	if isCLIMode {
		if err := runCLIMode(flags, positionalArgs); err != nil {
			slog.Error("CLI error", "error", err)
			os.Exit(1)
		}
		return
	}

	mcpStdin, err := prepareMCPStdin(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	runMCPMode(flags, mcpStdin)
}

func getConfig(flags CLIFlags) *Config {
	cfg := DefaultConfig()

	if flags.SearXNGURL != "" {
		cfg.SearXNGURL = flags.SearXNGURL
	} else {
		envURL := os.Getenv("SEARXNG_URL")
		if envURL != "" {
			cfg.SearXNGURL = envURL
		}
	}

	if flags.SearXNGURL == "" && os.Getenv("SEARXNG_URL") == "" {
		fmt.Fprintf(os.Stderr, "\033[33mWARN: No SearXNG server specified, using default server (https://search-4.xlion.dev). To use a different server, set the SEARXNG_URL environment variable or use the --searxng-url command line flag.\033[0m\n")
	}

	return cfg
}

func runCLIMode(flags CLIFlags, positionalArgs []string) error {
	if flags.Help {
		printCLIHelp()
		return nil
	}

	if flags.Version {
		fmt.Println("searxng-mcp-go version " + version)
		fmt.Println("SearXNG MCP Server - CLI + MCP stdio dual-mode")
		return nil
	}

	query := flags.Query
	if query == "" && len(positionalArgs) > 0 {
		query = positionalArgs[0]
	}

	if query == "" {
		return fmt.Errorf("search query is required use --help for usage information")
	}

	cfg := getConfig(flags)
	pageno := flags.Pageno
	args := &SearchArgs{
		Query:      query,
		Language:   flags.Language,
		SafeSearch: flags.SafeSearch,
		TimeRange:  flags.TimeRange,
		Categories: flags.Categories,
		Engines:    flags.Engines,
		Pageno:     &pageno,
	}

	if err := ValidateSearchArgs(args); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	searcher, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		return fmt.Errorf("failed to create searcher: %w", err)
	}
	defer searcher.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := searcher.Search(ctx, args)
	if err != nil {
		return fmt.Errorf("search error: %w", err)
	}

	if flags.JSON {
		if debugMode {
			fmt.Println()
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("failed to encode json: %w", err)
		}
	} else {
		fmt.Print(formatResults(resp))
	}

	return nil
}

func NewSearchToolHandler(searcher *SearXNGSearcher) func(context.Context, *mcp.CallToolRequest, SearchArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, args SearchArgs) (*mcp.CallToolResult, any, error) {
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

		jsonBytes, err := json.Marshal(resp)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("json marshal error: %s", err.Error())},
				},
				IsError: true,
			}, nil, nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(jsonBytes)},
			},
		}, nil, nil
	}
}

type mcpInitializeMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
}

func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	reader := bufio.NewReader(stdin)
	firstLine, err := reader.ReadBytes('\n')
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("stdin does not contain a valid MCP initialize message")
	}
	if !isValidMCPInitializeMessage(firstLine) {
		return nil, fmt.Errorf("stdin does not contain a valid MCP initialize message")
	}

	return io.MultiReader(bytes.NewReader(firstLine), reader), nil
}

func isValidMCPInitializeMessage(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return false
	}

	var msg mcpInitializeMessage
	if err := json.Unmarshal(line, &msg); err != nil {
		return false
	}

	return msg.JSONRPC == "2.0" && msg.Method == "initialize"
}

func attachStdin(stdin io.Reader) (restore func(), err error) {
	originalStdin := os.Stdin
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	os.Stdin = pr

	go func() {
		_, copyErr := io.Copy(pw, stdin)
		if copyErr != nil {
			slog.Debug("failed to copy MCP stdin", "error", copyErr)
		}
		_ = pw.Close()
	}()

	return func() {
		os.Stdin = originalStdin
		_ = pr.Close()
	}, nil
}

func runMCPMode(flags CLIFlags, stdin io.Reader) {
	restoreStdin, err := attachStdin(stdin)
	if err != nil {
		slog.Error("failed to prepare MCP stdin", "error", err)
		os.Exit(1)
	}
	defer restoreStdin()

	cfg := getConfig(flags)

	searcher, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		slog.Error("failed to create searcher", "error", err)
		os.Exit(1)
	}
	defer searcher.Close()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG meta-search engine. Returns web results with titles, URLs, and summaries. Supports language filtering, category selection, engine selection, SafeSearch, time range restrictions, and pagination.",
		InputSchema: json.RawMessage(searchInputSchema),
	}, NewSearchToolHandler(searcher))

	slog.Info("starting SearXNG MCP server")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
