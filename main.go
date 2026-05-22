package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"searxng-mcp-go/internal/searxng"
)

var (
	version = "v1.0.4"
	commit  = "none"
	date    = "unknown"
)

const defaultResultLimit = 10

// debugMode is set to true when --debug flag or DEBUG=1 env var is active.
//
// No race condition: assignment happens before any goroutine starts
// (parseArgs → main → runMCPMode/runCLIMode all complete in a single
// goroutine). If runtime modification is needed in the future, use
// atomic.Bool to guarantee concurrent safety.
var debugMode bool

var (
	errArgumentParseFailed = errors.New("failed to parse arguments")
	errSearXNGURLRequired  = errors.New("SearXNG_URL is required: set SEARXNG_URL environment variable or --searxng-url flag")
	errTestConfigRequired  = errors.New("testPerformSearch: cfg cannot be nil")
)

// ============================================================================

// CLIFlags holds parsed CLI flag values.
type CLIFlags struct {
	Query         string
	JSON          bool
	Help          bool
	Version       bool
	SearXNGURL    string
	Language      string
	SafeSearch    int
	TimeRange     string
	Categories    string
	Engines       string
	Pageno        *int
	Limit         *int
	Debug         bool
	Timeout       time.Duration
	TimeoutSet    bool
	MaxRetries    int
	MaxRetriesSet bool
}

type registeredFlags struct {
	query      *string
	jsonOut    *bool
	help       *bool
	version    *bool
	searxngURL *string
	language   *string
	safeSearch *int
	timeRange  *string
	categories *string
	engines    *string
	pageno     *int
	limit      *int
	debug      *bool
	timeout    *time.Duration
	maxRetries *int
}

// parseArgs parses command-line arguments and returns the mode, flags, and positional arguments.
// Any supplied arguments route the process into CLI mode; otherwise the server runs in MCP mode.
// Flags are accepted anywhere before or after positional args, matching the current CLI behavior.
func parseArgs(args []string) (bool, CLIFlags, []string, error) {
	// Build the FlagSet first so we can use Lookup to determine whether a
	// flag takes a value (via the IsBoolFlag interface) during the
	// interleaved scan, avoiding the need to hard-code a parallel map.
	fs, registered := registerFlags()
	flagArgs, positionalArgs := extractPositionalArgs(args, fs)

	err := fs.Parse(flagArgs)
	if err != nil {
		return false, CLIFlags{}, nil, fmt.Errorf("%w: %w", errArgumentParseFailed, err)
	}

	// Use flag.Visit to determine whether optional pointer flags were explicitly set.
	// When --pageno is not set, leave Pageno nil so CLI mode omits pageno from the
	// search request (matching MCP mode behavior and the documented
	// "omitted = backend default/page 1" contract). Limit always has an effective
	// default so response truncation is consistent with the documented CLI default.
	var (
		pagenoPtr     *int
		limitPtr      *int
		timeoutSet    bool
		maxRetriesSet bool
	)

	fs.Visit(func(f *flag.Flag) {
		if f.Name == "pageno" {
			pagenoPtr = registered.pageno
		}

		if f.Name == "limit" {
			limitPtr = registered.limit
		}

		if f.Name == "timeout" {
			timeoutSet = true
		}

		if f.Name == "max-retries" {
			maxRetriesSet = true
		}
	})

	if limitPtr == nil {
		defaultLimit := defaultResultLimit
		limitPtr = &defaultLimit
	}

	flags := CLIFlags{
		Query:         *registered.query,
		JSON:          *registered.jsonOut,
		Help:          *registered.help,
		Version:       *registered.version,
		SearXNGURL:    *registered.searxngURL,
		Language:      *registered.language,
		SafeSearch:    *registered.safeSearch,
		TimeRange:     *registered.timeRange,
		Categories:    *registered.categories,
		Engines:       *registered.engines,
		Pageno:        pagenoPtr,
		Limit:         limitPtr,
		Debug:         *registered.debug,
		Timeout:       *registered.timeout,
		TimeoutSet:    timeoutSet,
		MaxRetries:    *registered.maxRetries,
		MaxRetriesSet: maxRetriesSet,
	}

	isCLIMode := len(args) > 0 || flags.Help || flags.Version || flags.Query != "" || flags.JSON || len(positionalArgs) > 0

	return isCLIMode, flags, positionalArgs, nil
}

func registerFlags() (*flag.FlagSet, registeredFlags) {
	fs := flag.NewFlagSet("searxng-mcp-go", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	registered := registeredFlags{
		query:      fs.String("query", "", "Search query string (CLI mode)"),
		jsonOut:    fs.Bool("json", false, "Output results as JSON (CLI mode)"),
		help:       fs.Bool("help", false, "Show this help message"),
		version:    fs.Bool("version", false, "Show version information"),
		searxngURL: fs.String("searxng-url", "", "SearXNG URL (can also be set via SEARXNG_URL env var)"),
		language:   fs.String("language", "", "Language code for results (e.g., en, zh-tw, ja). Leave empty for auto"),
		safeSearch: fs.Int("safesearch", 0, "SafeSearch level: 0=Off, 1=Moderate, 2=Strict"),
		timeRange:  fs.String("time_range", "", "Time range filter: day, month, year"),
		categories: fs.String("categories", "", "Comma-separated list of categories to search"),
		engines:    fs.String("engines", "", "Comma-separated list of search engines to use"),
		pageno:     fs.Int("pageno", 1, "Page number for pagination"),
		limit:      fs.Int("limit", defaultResultLimit, "Maximum number of results to return (1-20)"),
		debug:      fs.Bool("debug", false, "Enable verbose HTTP request/response logging (can also be set via DEBUG=1 env var)"),
		timeout:    fs.Duration("timeout", 0, "HTTP client timeout (e.g., 8s, 30s); overrides SEARXNG_TIMEOUT env var"),
		maxRetries: fs.Int("max-retries", 0, "Max retries after initial search attempt; overrides SEARXNG_MAX_RETRIES env var"),
	}

	return fs, registered
}

func extractPositionalArgs(args []string, fs *flag.FlagSet) ([]string, []string) {
	// Interleaved scan: extract flag tokens (with their values) while
	// collecting positional args. Flags are accepted before or after
	// positional arguments and are passed to fs.Parse() afterwards.
	flagArgs := make([]string, 0, len(args))
	positionalArgs := make([]string, 0, len(args))

	for idx := 0; idx < len(args); idx++ {
		arg := args[idx]
		if arg == "--" {
			// Everything after -- is positional (standard convention).
			positionalArgs = append(positionalArgs, args[idx+1:]...)

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
				if idx+1 < len(args) {
					idx++
					flagArgs = append(flagArgs, args[idx])
				}
			}
		}
	}

	return flagArgs, positionalArgs
}

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
		err = runCLIMode(flags, positionalArgs)
		if err != nil {
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

func getConfig(flags CLIFlags) (*searxng.Config, error) {
	cfg := searxng.DefaultConfig()

	searxngURL := flags.SearXNGURL
	if searxngURL == "" {
		searxngURL = os.Getenv("SEARXNG_URL")
	}

	if searxngURL == "" {
		return nil, errSearXNGURLRequired
	}

	cfg.SearXNGURL = searxngURL

	// Apply SEARXNG_TIMEOUT env var (parsed as Go duration, e.g. "8s", "30s")
	if timeoutStr := os.Getenv("SEARXNG_TIMEOUT"); timeoutStr != "" {
		d, err := time.ParseDuration(timeoutStr)
		if err != nil {
			slog.Warn("invalid SEARXNG_TIMEOUT, ignoring", "value", timeoutStr, "error", err)
		} else {
			cfg.Timeout = d
		}
	}

	// Apply SEARXNG_MAX_RETRIES env var (parsed as integer)
	if maxRetriesStr := os.Getenv("SEARXNG_MAX_RETRIES"); maxRetriesStr != "" {
		n, err := strconv.Atoi(maxRetriesStr)
		if err != nil || n < 0 {
			slog.Warn("invalid SEARXNG_MAX_RETRIES, ignoring", "value", maxRetriesStr, "error", err)
		} else {
			cfg.MaxRetries = n
		}
	}

	// CLI flag overrides take precedence over env vars (and defaults).
	if flags.TimeoutSet {
		cfg.Timeout = flags.Timeout
	}

	if flags.MaxRetriesSet {
		cfg.MaxRetries = flags.MaxRetries
	}

	return cfg, nil
}
