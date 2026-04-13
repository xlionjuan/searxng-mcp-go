// TEST_OK
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ============================================================================
// Error Types
// ============================================================================

// ValidationError represents a user-provided parameter validation failure.
// These errors are returned when input parameters are invalid or missing.
type ValidationError struct {
	Field   string // Field is the name of the invalid field
	Message string // Message describes the validation failure
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %q: %s", e.Field, e.Message)
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}

// SearXNGError represents an error that occurred during communication with
// the SearXNG service. This includes network errors, HTTP errors, and API errors.
type SearXNGError struct {
	StatusCode    int    // HTTP status code if available
	ContentType   string // Content-Type header from response
	ResponseBody  string // Truncated response body for debugging
	UnderlyingErr error  // The original error that caused this
}

func (e *SearXNGError) Error() string {
	if e.UnderlyingErr != nil {
		return fmt.Sprintf("searxng error (status %d): %v", e.StatusCode, e.UnderlyingErr)
	}
	return fmt.Sprintf("searxng error: status %d, content-type: %s", e.StatusCode, e.ContentType)
}

// Unwrap returns the underlying error for errors.Is/ errors.As support
func (e *SearXNGError) Unwrap() error {
	return e.UnderlyingErr
}

// NewSearXNGError creates a new SearXNGError
func NewSearXNGError(statusCode int, contentType, body string, err error) *SearXNGError {
	// Truncate body for storage
	bodyPreview := body
	if len(bodyPreview) > 200 {
		bodyPreview = bodyPreview[:200] + "..."
	}
	return &SearXNGError{
		StatusCode:    statusCode,
		ContentType:   contentType,
		ResponseBody:  bodyPreview,
		UnderlyingErr: err,
	}
}

// HTTPStatusError creates a SearXNGError from an HTTP status code
func HTTPStatusError(statusCode int, contentType string, body []byte) error {
	var bodyStr string
	if len(body) > 0 {
		if len(body) > 200 {
			bodyStr = string(body[:200]) + "..."
		} else {
			bodyStr = string(body)
		}
	}

	var msg string
	switch statusCode {
	case 400:
		msg = "bad request: the query parameters may be invalid"
	case 401:
		msg = "unauthorized: authentication is required"
	case 403:
		msg = "forbidden: access denied"
	case 404:
		msg = "not found: the search endpoint could not be found"
	case 429:
		msg = "rate limited: too many requests, please wait before making more searches"
	case 500:
		msg = "internal server error: the search engine encountered an internal error"
	case 502:
		msg = "bad gateway: received an invalid response from an upstream server"
	case 503:
		msg = "service unavailable: the search engine is temporarily unavailable"
	case 504:
		msg = "gateway timeout: timed out waiting for an upstream server"
	default:
		msg = fmt.Sprintf("unexpected status code received")
	}

	return NewSearXNGError(statusCode, contentType, bodyStr, errors.New(msg))
}

// httpStatusError is an alias for HTTPStatusError for backward compatibility with tests
func httpStatusError(statusCode int, contentType string, body []byte) error {
	return HTTPStatusError(statusCode, contentType, body)
}

// HTMLResponseError creates a specialized error for HTML responses (JSON not enabled)
type HTMLResponseError struct {
	Body       string // Truncated HTML body
	Underlying error  // The underlying network error if any
}

func (e *HTMLResponseError) Error() string {
	return fmt.Sprintf("searxng returned HTML instead of JSON (JSON output likely not enabled on instance). Response: %s", e.Body)
}

func (e *HTMLResponseError) Unwrap() error {
	return e.Underlying
}

// ============================================================================
// Searcher Interface
// ============================================================================

// Searcher defines the interface for performing web searches.
// This allows for different implementations (real SearXNG, mock, etc.)
type Searcher interface {
	Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error)
}

// SearXNGSearcher implements the Searcher interface using a real SearXNG instance
type SearXNGSearcher struct {
	client  *http.Client // Configurable HTTP client
	baseURL string
	timeout time.Duration
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration
func NewSearXNGSearcher(baseURL string, timeout time.Duration, client *http.Client) *SearXNGSearcher {
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	return &SearXNGSearcher{
		client:  client,
		baseURL: baseURL,
		timeout: timeout,
	}
}

// performSearch is a backward-compatible wrapper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its Search method.
// This function exists for backward compatibility with existing tests and external callers.
func performSearch(ctx context.Context, cfg *Config, args *SearchArgs) (*SearchResponse, error) {
	searcher := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	return searcher.Search(ctx, args)
}

// Search implements the Searcher interface
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	return s.performSearch(ctx, args)
}

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
// Config
// ============================================================================

// Config holds the SearXNG configuration
type Config struct {
	SearXNGURL string
	Timeout    time.Duration
	HTTPClient *http.Client // Optional custom HTTP client
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		SearXNGURL: "https://search-4.xlion.dev",
		Timeout:    30 * time.Second,
	}
}

// getEnv returns the value of an environment variable or a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// ============================================================================
// Data Types
// ============================================================================

// SearchArgs defines the arguments for the search tool
type SearchArgs struct {
	Query      string `json:"query" jsonschema:"Search query string"`
	Language   string `json:"language" jsonschema:"Language code for results (e.g., en, zh-tw, ja). Defaults to en"`
	SafeSearch *int   `json:"safesearch" jsonschema:"SafeSearch level: 0=Off, 1=Moderate, 2=Strict. Defaults to 0"`
	TimeRange  string `json:"time_range" jsonschema:"Time range filter: day, month, year, or empty for all time"`
	Categories string `json:"categories" jsonschema:"Comma-separated list of categories to search (e.g., general, news, music)"`
	Engines    string `json:"engines" jsonschema:"Comma-separated list of search engines to use (e.g., google, bing, duckduckgo)"`
	Pageno     *int   `json:"pageno" jsonschema:"Page number for pagination. Defaults to 1"`
}

// DateSource represents the date's source
type DateSource string

const (
	DateSourceAPI      DateSource = "api"      // From SearXNG API
	DateSourceInferred DateSource = "inferred" // Inferred from content
	DateSourceNone     DateSource = ""         // Unable to determine
)

// SearchResult represents a single search result
type SearchResult struct {
	Title         string     `json:"title"`
	URL           string     `json:"url"`
	Content       string     `json:"content"`
	Engine        string     `json:"engine"`
	PublishedDate *string    `json:"publishedDate,omitempty"`
	DateSource    DateSource `json:"dateSource,omitempty"`
}

// SearchResponse represents the full search response from SearXNG
type SearchResponse struct {
	Results         []SearchResult `json:"results"`
	NumberOfResults int            `json:"number_of_results"`
	Query           string         `json:"query"`
}

// ============================================================================
// Centralized Validation
// ============================================================================

// ValidTimeRanges contains the set of valid time range values
var ValidTimeRanges = map[string]bool{"day": true, "month": true, "year": true}

// ValidateSearchArgs validates the search arguments and returns a ValidationError if invalid
func ValidateSearchArgs(args *SearchArgs) error {
	if args == nil {
		return NewValidationError("args", "search arguments cannot be nil")
	}

	if args.Query == "" {
		return NewValidationError("query", "search query is required")
	}

	if args.TimeRange != "" && !ValidTimeRanges[args.TimeRange] {
		return NewValidationError("time_range", "must be one of: day, month, year")
	}

	if args.SafeSearch != nil {
		ss := *args.SafeSearch
		if ss < 0 || ss > 2 {
			return NewValidationError("safesearch", "must be 0 (Off), 1 (Moderate), or 2 (Strict)")
		}
	}

	return nil
}

// ============================================================================
// Search Implementation
// ============================================================================

// performSearch executes the search query against SearXNG
func (s *SearXNGSearcher) performSearch(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("invalid SearXNG URL: %w", err))
	}

	params := url.Values{}
	params.Set("q", args.Query)
	params.Set("format", "json")

	language := args.Language
	if language == "" {
		language = "en"
	}
	params.Set("language", language)

	safesearch := 0
	if args.SafeSearch != nil {
		safesearch = *args.SafeSearch
	}
	params.Set("safesearch", fmt.Sprintf("%d", safesearch))

	if args.TimeRange != "" {
		params.Set("time_range", args.TimeRange)
	}

	if args.Categories != "" {
		params.Set("categories", args.Categories)
	}

	if args.Engines != "" {
		params.Set("engines", args.Engines)
	}

	if args.Pageno != nil {
		params.Set("pageno", fmt.Sprintf("%d", *args.Pageno))
	}

	baseURL.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL.String(), nil)
	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to create request: %w", err))
	}

	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to execute search request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read response body: %w", err))
	}

	// Check Content-Type to provide better error messages for non-JSON responses
	contentType := resp.Header.Get("Content-Type")
	isHTMLResponse := strings.Contains(contentType, "text/html") || strings.HasPrefix(strings.TrimSpace(string(body)), "<!DOCTYPE") || strings.HasPrefix(strings.TrimSpace(string(body)), "<html")

	if isHTMLResponse {
		return nil, &HTMLResponseError{Body: string(body)[:min(200, len(string(body)))], Underlying: nil}
	}

	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "text/json") {
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return nil, NewSearXNGError(resp.StatusCode, contentType, bodyPreview, errors.New("unexpected content type (expected application/json)"))
	}

	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		bodyPreview := string(body)
		if len(bodyPreview) > 200 {
			bodyPreview = bodyPreview[:200] + "..."
		}
		return nil, NewSearXNGError(resp.StatusCode, contentType, bodyPreview, fmt.Errorf("failed to parse JSON response: %w", err))
	}

	// SearXNG may return number_of_results=0 even when results exist
	if result.NumberOfResults == 0 && len(result.Results) > 0 {
		result.NumberOfResults = len(result.Results)
	}

	return &result, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// ============================================================================
// Date Parsing
// ============================================================================

func parseRelativeDate(content string, currentTime time.Time) *time.Time {
	if content == "" {
		return nil
	}

	lower := strings.ToLower(content)

	if strings.Contains(lower, "vorgestern") {
		t := currentTime.AddDate(0, 0, -2)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "yesterday") {
		t := currentTime.AddDate(0, 0, -1)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	if strings.Contains(lower, "vor woche") || strings.Contains(lower, "last week") {
		t := currentTime.AddDate(0, 0, -7)
		if t.Year() < 2000 {
			return nil
		}
		return &t
	}

	hourPattern := regexp.MustCompile(`(\d+)\s*(hour|hours|h|stunde|stunden)\s*(ago|vor)?`)
	if matches := hourPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		hours := 0
		fmt.Sscanf(matches[1], "%d", &hours)
		if hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	dayPattern := regexp.MustCompile(`(\d+)\s*(day|days|d|tag|tagen)\s*(ago|vor)?`)
	if matches := dayPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		days := 0
		fmt.Sscanf(matches[1], "%d", &days)
		if days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	vorHoursPattern := regexp.MustCompile(`vor\s+(\d+)\s*(stunde|stunden|stunden)\b`)
	if matches := vorHoursPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		hours := 0
		fmt.Sscanf(matches[1], "%d", &hours)
		if hours > 0 && hours <= 48 {
			t := currentTime.Add(-time.Duration(hours) * time.Hour)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	vorDaysPattern := regexp.MustCompile(`vor\s+(\d+)\s*(tag|tagen)\b`)
	if matches := vorDaysPattern.FindStringSubmatch(lower); len(matches) >= 2 {
		days := 0
		fmt.Sscanf(matches[1], "%d", &days)
		if days > 0 && days <= 365 {
			t := currentTime.AddDate(0, 0, -days)
			if t.After(currentTime) || t.Year() < 2000 {
				return nil
			}
			return &t
		}
	}

	return nil
}

// ============================================================================
// Formatting
// ============================================================================

func inferDates(resp *SearchResponse) {
	now := time.Now()
	for i := range resp.Results {
		r := &resp.Results[i]
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			r.DateSource = DateSourceAPI
		} else {
			parsed := parseRelativeDate(r.Content, now)
			if parsed != nil {
				formatted := parsed.Format("2006-01-02")
				r.PublishedDate = &formatted
				r.DateSource = DateSourceInferred
			} else {
				r.DateSource = DateSourceNone
			}
		}
	}
}

// formatResults formats search results as a readable string
func formatResults(resp *SearchResponse) string {
	inferDates(resp)
	if len(resp.Results) == 0 {
		return "No results found."
	}

	output := fmt.Sprintf("Found %d results for '%s':\n\n", len(resp.Results), resp.Query)
	for i, r := range resp.Results {
		output += fmt.Sprintf("%d. %s\n", i+1, r.Title)
		output += fmt.Sprintf("   URL: %s\n", r.URL)
		if r.Content != "" {
			output += fmt.Sprintf("   Summary: %s\n", r.Content)
		}
		if r.PublishedDate != nil && *r.PublishedDate != "" {
			output += fmt.Sprintf("   Date: %s\n", *r.PublishedDate)
		}
		output += fmt.Sprintf("   Engine: %s\n\n", r.Engine)
	}
	return output
}

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
		SafeSearch: cliSafeSearch,
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
