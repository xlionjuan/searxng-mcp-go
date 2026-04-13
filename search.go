package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

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
	SafeSearch int    `json:"safesearch" jsonschema:"SafeSearch level: 0=Off, 1=Moderate, 2=Strict. Defaults to 0"`
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

	safesearch := args.SafeSearch
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
