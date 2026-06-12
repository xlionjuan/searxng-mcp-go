// Package main implements a SearXNG MCP server and CLI tool
// that provides web search capabilities via the SearXNG meta-search engine.
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
	version = "v1.4.1"
	commit  = "none"
	date    = "unknown"
)

var (
	errArgumentParseFailed = errors.New("failed to parse arguments")
	errSearXNGURLRequired  = errors.New(
		"SearXNG_URL is required: set SEARXNG_URL environment variable or --searxng-url flag")
	errUnexpectedFlagType = errors.New("registered search flag has unexpected type")
)

// Process exit codes. CLI failures exit with exitCodeCLIError; MCP mode
// failures (stdin validation, server startup/run) exit with exitCodeMCPError
// to match the documented contract in cli.go's "EXIT CODES" section.
const (
	exitCodeCLIError = 1
	exitCodeMCPError = 2
)

// ============================================================================

// errParamDefaultNotInt is the sentinel error wrapped into the panic value
// raised when a ParamDef declares GoType "int" with a Default string that
// does not parse as an integer. Defaults are compile-time constants today,
// so a non-int default indicates a programming error; failing fast at
// flag-registration time is preferable to silently using 0 (which would be
// a surprise at runtime). Wrapping keeps the panic value errors.Is-checkable.
var errParamDefaultNotInt = errors.New("registerFlags: ParamDef has unparseable int default")

// CLIFlags holds parsed CLI flag values.
type CLIFlags struct {
	Query               string
	JSON                bool
	Help                bool
	Version             bool
	SearXNGURL          string
	Language            string
	SafeSearch          int
	TimeRange           string
	Categories          string
	Engines             string
	Pageno              *int
	Limit               *int
	Debug               bool
	Timeout             *time.Duration
	MaxRetries          *int
	AllowGETFallback    bool
	AllowGETFallbackExplicit bool
}

type registeredFlags struct {
	jsonOut          *bool
	help             *bool
	version          *bool
	searxngURL       *string
	debug            *bool
	timeout          *time.Duration
	maxRetries       *int
	allowGETFallback *bool
	// Search parameters are registered generically from the shared
	// searxng.SearchParams table and stored in searchFlags. Each value is
	// either *string or *int matching the ParamDef.GoType.
	searchFlags map[string]any
}

// parseArgs parses command-line arguments and returns the mode, flags, and positional arguments.
// Any supplied arguments route the process into CLI mode; otherwise the server runs in MCP mode.
// Flags are accepted anywhere before or after positional args, matching the current CLI behavior.
//
//nolint:gocyclo,cyclop // interleaved positional/flag processing inherent to CLI; linear scan is standard
func parseArgs(args []string) (bool, *CLIFlags, []string, error) {
	// Build the FlagSet first so we can use Lookup to determine whether a
	// flag takes a value (via the IsBoolFlag interface) during the
	// interleaved scan, avoiding the need to hard-code a parallel map.
	fs, registered := registerFlags()
	flagArgs, positionalArgs := extractPositionalArgs(args, fs)

	err := fs.Parse(flagArgs)
	if err != nil {
		return false, nil, nil, fmt.Errorf("%w: %w", errArgumentParseFailed, err)
	}

	// Use flag.Visit with a symmetric data-driven pattern to populate *T
	// optional fields. Only flags that were explicitly set get a non-nil
	// pointer; unset flags remain nil so downstream code (getConfig) can
	// distinguish "not provided" from "provided-but-equal-to-default".
	var (
		pagenoPtr           *int
		limitPtr            *int
		timeoutPtr          *time.Duration
		maxRetriesPtr       *int
		allowGETFallbackExplicit bool
	)

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "pageno":
			if ptr, ok := registered.searchFlags["pageno"].(*int); ok {
				pagenoPtr = ptr
			}
		case "limit":
			if ptr, ok := registered.searchFlags["limit"].(*int); ok {
				limitPtr = ptr
			}
		case "timeout":
			val := *registered.timeout
			timeoutPtr = &val
		case "max-retries":
			val := *registered.maxRetries
			maxRetriesPtr = &val
		case "allow-get-fallback":
			allowGETFallbackExplicit = true
		}
	})

	if limitPtr == nil {
		defaultLimit := searxng.DefaultResultLimit
		limitPtr = &defaultLimit
	}

	queryPtr, err := searchFlagPtr[string](registered.searchFlags, "query")
	if err != nil {
		return false, nil, nil, err
	}

	languagePtr, err := searchFlagPtr[string](registered.searchFlags, "language")
	if err != nil {
		return false, nil, nil, err
	}

	safeSearchPtr, err := searchFlagPtr[int](registered.searchFlags, "safesearch")
	if err != nil {
		return false, nil, nil, err
	}

	timeRangePtr, err := searchFlagPtr[string](registered.searchFlags, "time_range")
	if err != nil {
		return false, nil, nil, err
	}

	categoriesPtr, err := searchFlagPtr[string](registered.searchFlags, "categories")
	if err != nil {
		return false, nil, nil, err
	}

	enginesPtr, err := searchFlagPtr[string](registered.searchFlags, "engines")
	if err != nil {
		return false, nil, nil, err
	}

	flags := CLIFlags{
		Query:               *queryPtr,
		JSON:                *registered.jsonOut,
		Help:                *registered.help,
		Version:             *registered.version,
		SearXNGURL:          *registered.searxngURL,
		Language:            *languagePtr,
		SafeSearch:          *safeSearchPtr,
		TimeRange:           *timeRangePtr,
		Categories:          *categoriesPtr,
		Engines:             *enginesPtr,
		Pageno:              pagenoPtr,
		Limit:               limitPtr,
		Debug:               *registered.debug,
		Timeout:             timeoutPtr,
		MaxRetries:          maxRetriesPtr,
		AllowGETFallback:    *registered.allowGETFallback,
		AllowGETFallbackExplicit: allowGETFallbackExplicit,
	}

	isCLIMode := len(args) > 0 || flags.Help || flags.Version || flags.Query != "" || flags.JSON || len(positionalArgs) > 0

	return isCLIMode, &flags, positionalArgs, nil
}

func registerFlags() (*flag.FlagSet, registeredFlags) {
	fs := flag.NewFlagSet("searxng-mcp-go", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	r := registeredFlags{
		jsonOut:    fs.Bool("json", false, "Output results as JSON (CLI mode)"),
		help:       fs.Bool("help", false, "Show this help message"),
		version:    fs.Bool("version", false, "Show version information"),
		searxngURL: fs.String("searxng-url", "", "SearXNG URL (can also be set via SEARXNG_URL env var)"),
		debug: fs.Bool("debug", false,
			"Enable verbose HTTP request/response logging (can also be set via DEBUG=1 env var)"),
		timeout: fs.Duration(
			"timeout",
			searxng.DefaultTimeout,
			"HTTP client timeout (e.g., 8s); overrides SEARXNG_TIMEOUT env var",
		),
		maxRetries: fs.Int(
			"max-retries",
			searxng.DefaultMaxRetries,
			"Max retries after initial search attempt; overrides SEARXNG_MAX_RETRIES env var",
		),
		allowGETFallback: fs.Bool(
			"allow-get-fallback",
			false,
			"Enable POST→GET fallback for 405/501 responses (CLI mode); overrides SEARXNG_ALLOW_GET_FALLBACK env var",
		),
		searchFlags: make(map[string]any),
	}

	// Register search parameters from the shared table.
	// Note: "query" is also used as a positional argument in CLI mode.
	for _, p := range searxng.SearchParams {
		defaultVal, err := p.FlagDefault()
		if err != nil {
			panic(fmt.Errorf("%w (name=%q goType=%q default=%q): %w",
				errParamDefaultNotInt, p.Name, p.GoType, p.Default, err))
		}

		switch v := defaultVal.(type) {
		case string:
			r.searchFlags[p.Name] = fs.String(p.Name, v, p.Description)
		case int:
			r.searchFlags[p.Name] = fs.Int(p.Name, v, p.Description)
		default:
			panic(fmt.Errorf("%w: %s has unexpected default type %T",
				errUnexpectedFlagType, p.Name, v))
		}
	}

	return fs, r
}

func searchFlagPtr[T any](searchFlags map[string]any, name string) (*T, error) {
	ptr, ok := searchFlags[name].(*T)
	if !ok {
		return nil, fmt.Errorf("%w: %s", errUnexpectedFlagType, name)
	}

	return ptr, nil
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
		os.Exit(exitCodeCLIError)
	}

	// Enable debug mode via --debug flag or DEBUG=1 env var
	debug := flags.Debug || os.Getenv("DEBUG") == "1"
	if debug {
		slog.SetLogLoggerLevel(slog.LevelDebug)
		slog.Debug("debug mode enabled", "version", version)
		slog.Warn("debug mode logs search queries and HTTP requests in plain text; avoid sensitive searches")
	}

	if isCLIMode {
		err = runCLIMode(debug, flags, positionalArgs)
		if err != nil {
			slog.Error("CLI error", "error", err)
			os.Exit(exitCodeCLIError)
		}

		return
	}

	mcpStdin, err := prepareMCPStdin(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\033[31mERROR: %v\033[0m\n", err)
		os.Exit(exitCodeMCPError)
	}

	err = runMCPMode(debug, flags, mcpStdin)
	if err != nil {
		slog.Error("MCP server error", "error", err)
		os.Exit(exitCodeMCPError)
	}
}

// configSource represents one configurable setting with env-var fallback and
// CLI-flag override. Each source is a row in a single loop inside getConfig;
// adding a new configurable field means one new row, not branches in three
// places. The shared warn-and-ignore handler is warnInvalidConfigEntry.
type configSource[T any] struct {
	envVar   string
	getFlag  func(*CLIFlags) *T
	parseEnv func(string) (T, error)
	setValue func(*searxng.Config, T) error
}

func (cs configSource[T]) apply(cfg *searxng.Config, flags *CLIFlags) {
	var err error

	// Phase 1: environment variable.
	if envStr, ok := os.LookupEnv(cs.envVar); ok {
		var val T

		val, err = cs.parseEnv(envStr)
		if err != nil {
			warnInvalidConfigEntry(cs.envVar, envStr, err)

			return
		}

		err = cs.setValue(cfg, val)
		if err != nil {
			warnInvalidConfigEntry(cs.envVar, envStr, err)
		}
	}

	// Phase 2: CLI flag override (takes precedence over env var).
	if ptr := cs.getFlag(flags); ptr != nil {
		err = cs.setValue(cfg, *ptr)
		if err != nil {
			warnInvalidConfigEntry("--"+cs.envVar, *ptr, err)
		}
	}
}

func warnInvalidConfigEntry[T any](source string, value T, err error) {
	slog.Warn("invalid configuration value from "+source+", ignoring",
		"value", value, "error", err)
}

var (
	errMustBeNonNegative = errors.New("must be non-negative")
	errMustBe01          = errors.New("must be 0 or 1")
)

// intFromString parses an integer from an environment variable string,
// rejecting negative values. Zero is accepted (disables retries).
func intFromString(s string) (int, error) {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, err
	}

	if n < 0 {
		return 0, errMustBeNonNegative
	}

	return n, nil
}

// boolFromString parses a boolean from "1"/"0" environment variable string.
func boolFromString(s string) (bool, error) {
	switch s {
	case "1":
		return true, nil
	case "0":
		return false, nil
	default:
		return false, errMustBe01
	}
}

// configSources is the single loop table for getConfig. Each row wires one
// configurable setting: env var → parse → apply → flag override.
var configSources = []func(*searxng.Config, *CLIFlags){
	configSource[time.Duration]{
		envVar:   "SEARXNG_TIMEOUT",
		getFlag:  func(f *CLIFlags) *time.Duration { return f.Timeout },
		parseEnv: time.ParseDuration,
		setValue: (*searxng.Config).SetTimeout,
	}.apply,
	configSource[int]{
		envVar:   "SEARXNG_MAX_RETRIES",
		getFlag:  func(f *CLIFlags) *int { return f.MaxRetries },
		parseEnv: intFromString,
		setValue: (*searxng.Config).SetMaxRetries,
	}.apply,
	configSource[bool]{
		envVar:   "SEARXNG_ALLOW_GET_FALLBACK",
		getFlag:  allowGetFallbackFlagPtr,
		parseEnv: boolFromString,
		setValue: func(cfg *searxng.Config, v bool) error {
			cfg.AllowGETFallback = v

			return nil
		},
	}.apply,
}

func allowGetFallbackFlagPtr(flags *CLIFlags) *bool {
	if flags.AllowGETFallbackExplicit {
		return &flags.AllowGETFallback
	}

	return nil
}

// getConfig builds a Config from defaults, env vars, and CLI flags.
// Precedence: default → env var → CLI flag override.
func getConfig(flags *CLIFlags) (*searxng.Config, error) {
	cfg := searxng.DefaultConfig()

	// URL is handled separately because it is required and has no
	// per-env-var setter (it is a plain string assignment).
	searxngURL := flags.SearXNGURL
	if searxngURL == "" {
		searxngURL = os.Getenv("SEARXNG_URL")
	}

	if searxngURL == "" {
		return nil, errSearXNGURLRequired
	}

	cfg.SearXNGURL = searxngURL

	// Apply the data-driven config sources (env → flag override).
	for _, apply := range configSources {
		apply(cfg, flags)
	}

	// Lightweight final validation. By this point each setter has already
	// rejected invalid individual values, so Validate primarily serves as a
	// cross-field consistency check (none today) and a safety net.
	err := cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}
