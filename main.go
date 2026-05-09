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
//
// 目前無 race condition：賦值發生在 goroutine 啟動之前（parseArgs → main → runMCPMode
// / runCLIMode 皆在單一 goroutine 中完成）。若未來需要在執行期間動態修改此變數，
// 應改用 atomic.Bool 以確保 concurrent-safe。
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
	Pageno     *int
	Limit      *int
	Debug      bool
}

// parseArgs parses command-line arguments and returns the mode, flags, and positional arguments.
// Any supplied arguments route the process into CLI mode; otherwise the server runs in MCP mode.
// Flags are accepted anywhere before or after positional args, matching the current CLI behavior.
func parseArgs(args []string) (isCLIMode bool, flags CLIFlags, positionalArgs []string, err error) {
	// Build the FlagSet first so we can use Lookup to determine whether a
	// flag takes a value (via the IsBoolFlag interface) during the
	// interleaved scan, avoiding the need to hard-code a parallel map.
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
	limit := fs.Int("limit", 10, "Maximum number of results to return (1-20)")
	debug := fs.Bool("debug", false, "Enable verbose HTTP request/response logging (can also be set via DEBUG=1 env var)")

	// Interleaved scan: extract flag tokens (with their values) while
	// collecting positional args.  Flags are accepted before or after
	// positional arguments and are passed to fs.Parse() afterwards.
	flagArgs := make([]string, 0, len(args))
	positionalArgs = make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// Everything after -- is positional (standard convention).
			positionalArgs = append(positionalArgs, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") {
			positionalArgs = append(positionalArgs, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)

		// Consult the FlagSet to decide whether this flag expects a value.
		// Bool flags (--help, --json, --version, --debug) do not consume
		// the next token; all other flags do.
		name := strings.TrimLeft(arg, "-")
		if fl := fs.Lookup(name); fl != nil {
			if _, isBool := fl.Value.(interface{ IsBoolFlag() bool }); !isBool {
				if i+1 < len(args) {
					i++
					flagArgs = append(flagArgs, args[i])
				}
			}
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return false, CLIFlags{}, nil, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Use flag.Visit to determine whether --pageno was explicitly set.
	// When not set, leave Pageno nil so CLI mode omits pageno from the
	// search request (matching MCP mode behaviour and the documented
	// "omitted = backend default/page 1" contract).
	var pagenoPtr *int
	var limitPtr *int
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "pageno" {
			pagenoPtr = pageno
		}
		if f.Name == "limit" {
			limitPtr = limit
		}
	})

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
		Pageno:     pagenoPtr,
		Limit:      limitPtr,
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
			"description": "Language code for results. Common codes: en, zh-tw, zh, ja, fr, de, es, pt, ru, ar. Leave empty for auto-detect (SearXNG decides based on query)"
		},
		"safesearch": {
			"type": "integer",
			"description": "SafeSearch level. 0=Off (no filtering), 1=Moderate (filter moderate explicit content), 2=Strict (filter all explicit content). Defaults to 0",
			"minimum": 0,
			"maximum": 2
		},
		"time_range": {
			"type": "string",
			"description": "Time range filter. Available values: empty (all time), day, month, year. Defaults to empty (all time)",
			"enum": ["", "day", "month", "year"]
		},
		"categories": {
			"type": "string",
			"description": "Comma-separated list of categories to search. Common categories: general, news, images, videos, music, science, files, it, social_media, map. Leave empty for all categories"
		},
		"engines": {
			"type": "string",
			"description": "Comma-separated list of search engines to use (e.g., google, bing, duckduckgo). Leave empty to use SearXNG default engines"
		},
		"pageno": {
			"type": ["null", "integer"],
			"description": "Page number for pagination. Defaults to 1",
			"minimum": 1
		},
		"limit": {
			"type": "integer",
			"description": "Maximum number of results to return (1-20). Defaults to 10",
			"minimum": 1,
			"maximum": 20
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
		return fmt.Errorf("search query is required; use --help for usage information")
	}

	cfg := getConfig(flags)
	args := &SearchArgs{
		Query:      query,
		Language:   flags.Language,
		SafeSearch: flags.SafeSearch,
		TimeRange:  flags.TimeRange,
		Categories: flags.Categories,
		Engines:    flags.Engines,
		Pageno:     flags.Pageno,
		Limit:      flags.Limit,
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

// NewSearchToolHandler creates an MCP tool handler function that performs SearXNG searches.
// It returns a function suitable for use as an mcp.ToolHandler, which validates the search
// arguments, executes the search, and returns the formatted results.
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

const (
	mcpInitializeMaxBytes    = 1 << 20
	mcpInitializeReadTimeout = 5 * time.Second
)

func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpInitializeReadTimeout)
	defer cancel()

	type result struct {
		reader io.Reader
		err    error
	}

	resultCh := make(chan result, 1)
	go func() {
		reader := bufio.NewReader(io.LimitReader(stdin, mcpInitializeMaxBytes+1))
		firstLine, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			resultCh <- result{err: fmt.Errorf("stdin does not contain a valid MCP initialize message")}
			return
		}
		if len(firstLine) > mcpInitializeMaxBytes || !isValidMCPInitializeMessage(firstLine) {
			resultCh <- result{err: fmt.Errorf("stdin does not contain a valid MCP initialize message")}
			return
		}
		resultCh <- result{reader: io.MultiReader(bytes.NewReader(firstLine), reader)}
	}()

	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("stdin does not contain a valid MCP initialize message")
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}
		return res.reader, nil
	}
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
			// MCP stdio 場景中，pipe 寫入失敗通常發生在 MCP client 關閉 stdin
			// 或行程即將結束之時，屬於預期內的正常關閉行為，僅以 Debug 層級記錄即可，
			// 無需做額外的錯誤處理或重試。
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
		Description: "Search the web using SearXNG meta-search engine. Returns web results with titles, URLs, summaries, published dates, and engine source information. Parameters: query (required) - search query string; language - language code (e.g., en, zh-tw, ja, fr, de, es), auto-detect if empty; safesearch - 0=Off, 1=Moderate, 2=Strict (default 0); time_range - empty (all time), day, month, year; categories - comma-separated (e.g., general, news, images, videos, music, science, files, it, social_media, map); engines - comma-separated (e.g., google, bing, duckduckgo), SearXNG defaults if empty; pageno - page number (default 1); limit - max results 1-20 (default 10).",
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
