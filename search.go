package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultSearXNGURL is the default SearXNG instance URL.
// WARNING: This is a default value for convenience only. For production use,
// you should set your own instance via the SEARXNG_URL environment variable.
const DefaultSearXNGURL = "https://search-4.xlion.dev"

// defaultHTTPClient is a singleton HTTP client shared across all searchers.
// This avoids creating new transport goroutines for each search while preventing
// unbounded memory growth.
var defaultHTTPClient *http.Client
var defaultHTTPClientOnce sync.Once

// getDefaultHTTPClient returns the singleton HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
				IdleConnTimeout:       90 * time.Second,
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   10,
			},
		}
	})
	return defaultHTTPClient
}

// ============================================================================
// SearXNG Searcher
// ============================================================================

// SearXNGSearcher performs web searches via a SearXNG instance
type SearXNGSearcher struct {
	client  *http.Client // Configurable HTTP client
	baseURL string
}

// Close releases resources held by the searcher.
//
// OWNERSHIP SEMANTICS:
//
//   - The HTTP client may be shared (cached globally). Close() only closes idle
//     connections on the cached client, it does NOT evict the client from the cache.
//     Subsequent calls to NewSearXNGSearcher with the same URL/timeout may return
//     the same cached client.
//
//   - Calling Close() multiple times is SAFE (idempotent). It only closes idle
//     connections, and calling it on an already-drained transport is a no-op.
//
//   - The cache itself lives for the lifetime of the process and is not affected
//     by Close(). If you need to fully release resources, close the underlying
//     transport and let it be garbage collected; the cache entry will be replaced
//     on the next call with a fresh client.
func (s *SearXNGSearcher) Close() error {
	if s.client != nil && s.client.Transport != nil {
		if transport, ok := s.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
	return nil
}

// validateBaseURL checks that the baseURL is valid and returns an error if not
func validateBaseURL(baseURL string) error {
	if baseURL == "" {
		return errors.New("baseurl cannot be empty")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https scheme")
	}
	if parsed.Host == "" {
		return errors.New("url must include a host (e.g., search.example.com)")
	}
	return nil
}

// isPrivateHost checks if the host is a private/internal address
func isPrivateHost(host string) bool {
	// Remove port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Check TLD-based private domains (case-insensitive)
	lowerHost := strings.ToLower(host)
	if strings.HasSuffix(lowerHost, ".lan") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".home") {
		return true
	}

	// Check if it's an IP address
	ip := net.ParseIP(host)
	if ip == nil {
		// Not an IP address, not private
		return false
	}

	// Check IPv4 private ranges
	// 10.0.0.0/8
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 127.0.0.0/8 (loopback)
		if ip4[0] == 127 {
			return true
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		return false
	}

	// Check IPv6
	// ::1 (loopback)
	if ip.Equal(net.IPv6loopback) {
		return true
	}
	// fc00::/7 (unique local)
	if ip[0]&0xfe == 0xfc {
		return true
	}
	// fe80::/10 (link-local)
	if ip[0] == 0xfe && ip[1]&0xc0 == 0x80 {
		return true
	}

	return false
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration
func NewSearXNGSearcher(baseURL string, timeout time.Duration, client *http.Client) (*SearXNGSearcher, error) {
	if err := validateBaseURL(baseURL); err != nil {
		return nil, fmt.Errorf("NewSearXNGSearcher: %w", err)
	}

	// Parse URL to check scheme and host
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("NewSearXNGSearcher: url.Parse failed after validateBaseURL passed (internal error): %w", err)
	}

	// Warn if using HTTP with non-private host
	if parsed.Scheme == "http" && !isPrivateHost(parsed.Host) {
		slog.Warn("Using HTTP for non-private host. Search queries may be transmitted in clear text. Search results could be intercepted and modified by a MITM attacker")
	}

	if client == nil {
		client = getDefaultHTTPClient()
	}

	return &SearXNGSearcher{
		client:  client,
		baseURL: baseURL,
	}, nil
}

// performSearch is a backward-compatible wrapper that creates a temporary SearXNGSearcher
// from the provided Config and delegates to its performSearch method.
func performSearch(ctx context.Context, cfg *Config, args *SearchArgs) (*SearchResponse, error) {
	if cfg == nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("performSearch: cfg cannot be nil"))
	}
	searcher, err := NewSearXNGSearcher(cfg.SearXNGURL, cfg.Timeout, cfg.HTTPClient)
	if err != nil {
		return nil, err
	}
	return searcher.performSearch(ctx, args)
}

// Search is the external API entry point that delegates to performSearch.
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
		SearXNGURL: DefaultSearXNGURL,
		Timeout:    30 * time.Second,
	}
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
	if args == nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("args cannot be nil"))
	}
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("invalid SearXNG URL: %w", err))
	}

	// Validate URL scheme
	if baseURL.Scheme != "http" && baseURL.Scheme != "https" {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("searxng url must use http or https scheme"))
	}

	// Validate URL has a host
	if baseURL.Host == "" {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("searxng url must include a host (e.g., search.example.com)"))
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

	postURL := *baseURL
	postURL.RawQuery = ""

	var resp *http.Response

	postReq, err := http.NewRequestWithContext(ctx, "POST", postURL.String(), io.LimitReader(strings.NewReader(params.Encode()), MaxRequestBodySize))
	if err == nil {
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		postReq.Header.Set("Accept", "application/json")
		resp, err = s.client.Do(postReq)
	}

	if err == nil && resp != nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp.Body.Close()
		getURL := *baseURL
		getURL.RawQuery = params.Encode()
		getReq, reqErr := http.NewRequestWithContext(ctx, "GET", getURL.String(), nil)
		if reqErr != nil {
			return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to create request: %w", reqErr))
		}
		getReq.Header.Set("Accept", "application/json")
		resp, err = s.client.Do(getReq)
	}

	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to execute search request: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodySize))
		if readErr != nil {
			return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read error response body: %w", readErr))
		}
		return nil, HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read response body: %w", err))
	}

	// Check Content-Type to provide better error messages for non-JSON responses
	contentType := resp.Header.Get("Content-Type")
	isHTMLResponse := strings.Contains(contentType, "text/html") || strings.HasPrefix(strings.TrimSpace(string(body)), "<!DOCTYPE") || strings.HasPrefix(strings.TrimSpace(string(body)), "<html")

	if isHTMLResponse {
		// Log the HTML response body for debugging, but don't expose it to clients
		bodyLen := len(body)
		if bodyLen == 0 {
			return nil, &HTMLResponseError{Body: "", UnderlyingErr: nil}
		}
		// Log preview internally for debugging
		previewLen := bodyLen
		if previewLen > MaxErrorDisplayChars {
			previewLen = MaxErrorDisplayChars
		}
		slog.Debug("HTMLResponseError: received HTML instead of JSON", "preview", string(body[:previewLen]))
		return nil, &HTMLResponseError{Body: string(body[:previewLen]), UnderlyingErr: nil}
	}

	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "text/json") {
		// Log the unexpected content for debugging, but don't expose it to clients
		bodyPreview := string(body)
		if len(bodyPreview) > MaxErrorDisplayChars {
			bodyPreview = bodyPreview[:MaxErrorDisplayChars] + "..."
		}
		slog.Debug("UnexpectedContentTypeError", "content_type", contentType, "body_preview", bodyPreview)
		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("unexpected content type: expected application/json"))
	}

	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// Log the JSON parse error for debugging, but don't expose the body to clients
		slog.Debug("JSONParseError: failed to parse JSON response", "error", err)
		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("failed to parse JSON response: %w", err))
	}

	// SearXNG may return number_of_results=0 even when results exist
	if result.NumberOfResults == 0 && len(result.Results) > 0 {
		result.NumberOfResults = len(result.Results)
	}

	// Infer dates before returning to avoid mutation side effects in formatResults
	inferDates(&result, nil)

	return &result, nil
}
