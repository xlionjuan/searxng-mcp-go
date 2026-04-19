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

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			IdleConnTimeout:       90 * time.Second,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   10,
		},
	}
}

// getDefaultHTTPClient returns the singleton HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = newHTTPClient(30 * time.Second)
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

func checkBodyTruncated(body io.Reader) (bool, error) {
	buf := make([]byte, 1)
	_, err := body.Read(buf)
	if err == nil {
		return true, nil
	}
	if err == io.EOF {
		return false, nil
	}
	return false, err
}

// isPrivateHost checks if the host is a private/internal address
func isPrivateHost(host string) bool {
	// Remove port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	// Check localhost explicitly
	if host == "localhost" || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
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
		if timeout > 0 {
			client = newHTTPClient(timeout)
		} else {
			client = getDefaultHTTPClient()
		}
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

// SearchArgs defines the arguments for the search tool
type SearchArgs struct {
	Query      string `json:"query" jsonschema:"Search query string"`
	Language   string `json:"language" jsonschema:"Language code for results (e.g., en, zh-tw, ja). Defaults to auto (SearXNG decides)"`
	SafeSearch int    `json:"safesearch" jsonschema:"SafeSearch level: 0=Off, 1=Moderate, 2=Strict. Defaults to 0"`
	TimeRange  string `json:"time_range" jsonschema:"Time range filter: day, month, year, or empty for all time"`
	Categories string `json:"categories" jsonschema:"Comma-separated list of categories to search (e.g., general, news, music)"`
	Engines    string `json:"engines" jsonschema:"Comma-separated list of search engines to use (e.g., google, bing, duckduckgo)"`
	Pageno     *int   `json:"pageno" jsonschema:"Page number for pagination. Defaults to 1"`
}

// SearchResult represents a single search result
type SearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	PublishedDate *string `json:"publishedDate,omitempty"`
}

// InfoboxURL represents a URL entry in an infobox.
type InfoboxURL struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// InfoboxAttribute represents a key-value attribute in an infobox.
type InfoboxAttribute struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Infobox represents a knowledge panel / infobox from SearXNG.
type Infobox struct {
	Infobox    string             `json:"infobox"`
	Content    string             `json:"content"`
	Attributes []InfoboxAttribute `json:"attributes,omitempty"`
	URLs       []InfoboxURL       `json:"urls,omitempty"`
}

// Answer represents a direct answer from SearXNG (e.g., IP, hash, timezone, calculator).
type Answer struct {
	Answer   string `json:"answer"`
	Engine   string `json:"engine"`
	Template string `json:"template,omitempty"`
}

// SearchResponse represents the full search response from SearXNG
type SearchResponse struct {
	Query               string         `json:"query"`
	Answers             []Answer       `json:"answers,omitempty"`
	NumberOfResults     int            `json:"number_of_results"`
	Infoboxes           []Infobox      `json:"infoboxes,omitempty"`
	Results             []SearchResult `json:"results"`
	Suggestions         []string       `json:"suggestions"`
	UnresponsiveEngines [][]string     `json:"unresponsive_engines,omitempty"`
	Debug               bool           `json:"-"`
}

// MarshalJSON ensures JSON field ordering and only exposes debug-only fields when requested.
func (r SearchResponse) MarshalJSON() ([]byte, error) {
	// Ensure slices are empty (not nil) so JSON serializes as [] instead of null
	if r.Results == nil {
		r.Results = []SearchResult{}
	}
	if r.Suggestions == nil {
		r.Suggestions = []string{}
	}
	if r.Debug {
		if r.UnresponsiveEngines == nil {
			r.UnresponsiveEngines = [][]string{}
		}
		return json.Marshal(struct {
			Query               string         `json:"query"`
			Answers             []Answer       `json:"answers,omitempty"`
			NumberOfResults     int            `json:"number_of_results"`
			Infoboxes           []Infobox      `json:"infoboxes,omitempty"`
			Results             []SearchResult `json:"results"`
			Suggestions         []string       `json:"suggestions"`
			UnresponsiveEngines [][]string     `json:"unresponsive_engines"`
		}{
			Query:               r.Query,
			Answers:             r.Answers,
			NumberOfResults:     r.NumberOfResults,
			Infoboxes:           r.Infoboxes,
			Results:             r.Results,
			Suggestions:         r.Suggestions,
			UnresponsiveEngines: r.UnresponsiveEngines,
		})
	}
	return json.Marshal(struct {
		Query           string         `json:"query"`
		Answers         []Answer       `json:"answers,omitempty"`
		NumberOfResults int            `json:"number_of_results"`
		Infoboxes       []Infobox      `json:"infoboxes,omitempty"`
		Results         []SearchResult `json:"results"`
		Suggestions     []string       `json:"suggestions"`
	}{
		Query:           r.Query,
		Answers:         r.Answers,
		NumberOfResults: r.NumberOfResults,
		Infoboxes:       r.Infoboxes,
		Results:         r.Results,
		Suggestions:     r.Suggestions,
	})
}

// ============================================================================
// Search Implementation
// ============================================================================

// setBrowserHeaders sets browser-like HTTP headers to bypass SearXNG bot detection.
func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Priority", "u=0, i")
}

// deduplicateAnswers filters out answers whose text is a prefix (substring)
// of any infobox content. DuckDuckGo's engine often puts the same Wikipedia
// summary in both answers and infoboxes, causing duplicate display.
//
// The DDG answer may have "More at Wikipedia" appended, so we use prefix
// matching: take the first 200 characters of the answer and check if that
// prefix appears within the infobox content.
func deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer {
	if len(answers) == 0 || len(infoboxes) == 0 {
		return answers
	}

	// Check that at least one infobox has content (avoids empty-slice edge case).
	hasContent := false
	for _, ib := range infoboxes {
		if ib.Content != "" {
			hasContent = true
			break
		}
	}
	if !hasContent {
		return answers
	}

	const prefixLen = 200

	// infoboxTexts is built lazily — only if an answer needs the lowercase fallback.
	var infoboxTexts []string

	filtered := make([]Answer, 0, len(answers))
	for _, a := range answers {
		if a.Answer == "" {
			continue
		}

		// Fast path: exact-case prefix matching (no lowercase allocation).
		prefix := a.Answer
		prefix = strings.TrimSuffix(prefix, " More at Wikipedia")
		if len(prefix) > prefixLen {
			prefix = prefix[:prefixLen]
		}

		duplicated := false
		for _, ib := range infoboxes {
			if ib.Content != "" && strings.Contains(ib.Content, prefix) {
				duplicated = true
				break
			}
		}
		if duplicated {
			continue
		}

		// Slow path: lowercase fallback — build infoboxTexts on first use.
		if infoboxTexts == nil {
			infoboxTexts = make([]string, 0, len(infoboxes))
			for _, ib := range infoboxes {
				if ib.Content != "" {
					infoboxTexts = append(infoboxTexts, strings.ToLower(ib.Content))
				}
			}
		}

		lowerAnswer := strings.ToLower(a.Answer)
		lowerAnswer = strings.TrimSuffix(lowerAnswer, " more at wikipedia")
		lowerPrefix := lowerAnswer
		if len(lowerAnswer) > prefixLen {
			lowerPrefix = lowerAnswer[:prefixLen]
		}

		for _, ic := range infoboxTexts {
			if strings.Contains(ic, lowerPrefix) {
				duplicated = true
				break
			}
		}
		if !duplicated {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// performSearch executes the search query against SearXNG
func (s *SearXNGSearcher) performSearch(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	if err := ValidateSearchArgs(args); err != nil {
		return nil, err
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

	if args.Language != "" {
		params.Set("language", args.Language)
	}

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

	// Append /search path to avoid SearXNG redirect that drops POST body
	searchURL := *baseURL
	searchURL.RawQuery = ""
	trimmedPath := strings.TrimRight(searchURL.Path, "/")
	lastSegment := trimmedPath
	if idx := strings.LastIndex(trimmedPath, "/"); idx >= 0 {
		lastSegment = trimmedPath[idx+1:]
	}
	if lastSegment != "search" {
		if trimmedPath == "" {
			searchURL.Path = "/search"
		} else {
			searchURL.Path = trimmedPath + "/search"
		}
	} else {
		searchURL.Path = trimmedPath
	}

	var resp *http.Response

	// Save the raw body string so debug logging shows exactly what was sent
	postBodyStr := params.Encode()
	postReq, err := http.NewRequestWithContext(ctx, "POST", searchURL.String(), strings.NewReader(postBodyStr))
	if err == nil {
		postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		setBrowserHeaders(postReq)

		if debugMode {
			bodyPreview := postBodyStr
			if len(bodyPreview) > 500 {
				bodyPreview = bodyPreview[:500]
			}
			slog.Debug("HTTP request",
				"method", postReq.Method,
				"url", postReq.URL.String(),
				"Content-Type", postReq.Header.Get("Content-Type"),
				"Accept", postReq.Header.Get("Accept"),
				"body", bodyPreview,
			)
		}

		resp, err = s.client.Do(postReq)

		if debugMode && err == nil && resp != nil {
			slog.Debug("HTTP response",
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
			)
		}
	}

	if err == nil && resp != nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		if debugMode {
			slog.Debug("Redirecting to GET fallback", "status", resp.StatusCode, "reason", "POST not supported by server")
		}
		resp.Body.Close()
		getURL := searchURL
		getURL.RawQuery = params.Encode()
		getReq, reqErr := http.NewRequestWithContext(ctx, "GET", getURL.String(), nil)
		if reqErr != nil {
			return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to create request: %w", reqErr))
		}
		setBrowserHeaders(getReq)

		if debugMode {
			slog.Debug("HTTP request",
				"method", getReq.Method,
				"url", getReq.URL.String(),
				"Accept", getReq.Header.Get("Accept"),
			)
		}

		resp, err = s.client.Do(getReq)

		if debugMode && err == nil && resp != nil {
			slog.Debug("HTTP response",
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
			)
		}
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
		if debugMode {
			errBodyPreview := string(body)
			if len(errBodyPreview) > 500 {
				errBodyPreview = errBodyPreview[:500]
			}
			slog.Debug("HTTP error response body",
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
				"body_size", len(body),
				"body_preview", errBodyPreview,
			)
		}
		truncated, _ := checkBodyTruncated(resp.Body)
		if truncated {
			return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), fmt.Errorf("error response body exceeded maximum size limit of %d bytes", MaxErrorBodySize))
		}
		return nil, HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read response body: %w", err))
	}
	truncated, _ := checkBodyTruncated(resp.Body)
	if truncated {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), fmt.Errorf("response body exceeded maximum size limit of %d bytes", MaxResponseBodySize))
	}

	if debugMode {
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500]
		}
		slog.Debug("HTTP response body",
			"status", resp.StatusCode,
			"content_type", resp.Header.Get("Content-Type"),
			"body_size", len(body),
			"body_preview", bodyPreview,
		)
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

	// Deduplicate answers that overlap with infobox content.
	// DuckDuckGo engine puts Wikipedia summaries in both answers and infoboxes.
	result.Answers = deduplicateAnswers(result.Answers, result.Infoboxes)

	result.Debug = debugMode

	return &result, nil
}
