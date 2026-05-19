package searxng

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

// defaultHTTPClient is the shared client used when callers do not request a custom timeout.
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL != nil && len(via) > 0 {
				prevHost := via[len(via)-1].URL.Host
				if req.URL.Host != prevHost {
					return fmt.Errorf("redirect to different host blocked: %s → %s", prevHost, req.URL.Host)
				}
			}
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			return nil
		},
	}
}

// getDefaultHTTPClient returns the shared default HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = newHTTPClient(30 * time.Second)
	})
	return defaultHTTPClient
}

// SearXNGSearcher performs web searches via a SearXNG instance
type SearXNGSearcher struct {
	client  *http.Client // Configurable HTTP client
	baseURL string
	debug   bool // When true, enables verbose HTTP request/response logging
}

// Close releases resources held by the searcher.
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

func isBodyTruncated(body io.Reader) (bool, error) {
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

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}
	if err := resp.Body.Close(); err != nil {
		slog.Debug("failed to close response body", "error", err)
	}
}

// isPrivateHost checks if the host is a private/internal address
func isPrivateHost(host string) bool {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	if host == "localhost" || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}

	lowerHost := strings.ToLower(host)
	if strings.HasSuffix(lowerHost, ".lan") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".home") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 10 {
			return true
		}
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		if ip4[0] == 127 {
			return true
		}
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		if ip4[0] == 0 {
			return true
		}
		if ip4[0] == 100 && ip4[1]&0xc0 == 0x40 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return true
		}
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return true
		}
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return true
		}
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return true
		}
		if ip4[0] >= 224 {
			return true
		}
		return false
	}

	if ip.Equal(net.IPv6loopback) {
		return true
	}
	if ip[0]&0xfe == 0xfc {
		return true
	}
	if ip[0] == 0xfe && ip[1]&0xc0 == 0x80 {
		return true
	}

	return false
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration
func NewSearXNGSearcher(baseURL string, timeout time.Duration, client *http.Client, debug bool) (*SearXNGSearcher, error) {
	if err := validateBaseURL(baseURL); err != nil {
		return nil, fmt.Errorf("newSearXNGSearcher: %w", err)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("newSearXNGSearcher: url.Parse failed after validateBaseURL passed (internal error): %w", err)
	}

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
		debug:   debug,
	}, nil
}

// Search is the external API entry point that delegates to the internal performSearch method.
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	return s.performSearch(ctx, args)
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

// DeduplicateAnswers filters out answers whose text is a prefix (substring)
// of any infobox content.
//
// DeduplicateAnswers is exported for testing. External consumers should use
// Search/SearchResponse instead.
func DeduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer {
	if len(answers) == 0 || len(infoboxes) == 0 {
		return answers
	}

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

	var infoboxTexts []string

	filtered := make([]Answer, 0, len(answers))
	for _, a := range answers {
		if a.Answer == "" {
			continue
		}

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

// buildSearchRequest constructs an HTTP request for searching SearXNG.
func (s *SearXNGSearcher) buildSearchRequest(ctx context.Context, args *SearchArgs) (*http.Request, string, error) {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, "", NewSearXNGError(0, "", "", fmt.Errorf("invalid SearXNG URL: %w", err))
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

	postBodyStr := params.Encode()
	postReq, err := http.NewRequestWithContext(ctx, "POST", searchURL.String(), strings.NewReader(postBodyStr))
	if err != nil {
		return nil, "", NewSearXNGError(0, "", "", fmt.Errorf("failed to create request: %w", err))
	}
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setBrowserHeaders(postReq)

	return postReq, postBodyStr, nil
}

// parseSearchResponse reads and parses the response from a SearXNG search request.
func (s *SearXNGSearcher) parseSearchResponse(resp *http.Response, args *SearchArgs) (*SearchResponse, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBodySize))
	if err != nil {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read response body: %w", err))
	}
	truncated, truncErr := isBodyTruncated(resp.Body)
	if truncErr != nil {
		slog.Debug("isBodyTruncated read error", "error", truncErr)
	}
	if truncated {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), fmt.Errorf("response body exceeded maximum size limit of %d bytes", MaxResponseBodySize))
	}

	if s.debug {
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

	contentType := resp.Header.Get("Content-Type")
	isHTMLResponse := strings.Contains(contentType, "text/html") || strings.HasPrefix(strings.TrimSpace(string(body)), "<!DOCTYPE") || strings.HasPrefix(strings.TrimSpace(string(body)), "<html")

	if isHTMLResponse {
		bodyLen := len(body)
		if bodyLen == 0 {
			return nil, &HTMLResponseError{Body: "", UnderlyingErr: nil}
		}
		previewLen := bodyLen
		if previewLen > MaxErrorDisplayChars {
			previewLen = MaxErrorDisplayChars
		}
		slog.Debug("HTMLResponseError: received HTML instead of JSON", "preview", string(body[:previewLen]))
		return nil, &HTMLResponseError{Body: string(body[:previewLen]), UnderlyingErr: nil}
	}

	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "text/json") {
		bodyPreview := string(body)
		if len(bodyPreview) > MaxErrorDisplayChars {
			bodyPreview = bodyPreview[:MaxErrorDisplayChars] + "..."
		}
		slog.Debug("UnexpectedContentTypeError", "content_type", contentType, "body_preview", bodyPreview)
		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("unexpected content type: expected application/json"))
	}

	var result SearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		slog.Debug("JSONParseError: failed to parse JSON response", "error", err)
		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("failed to parse JSON response: %w", err))
	}

	if result.NumberOfResults == 0 && len(result.Results) > 0 {
		result.NumberOfResults = len(result.Results)
	}

	result.Answers = DeduplicateAnswers(result.Answers, result.Infoboxes)

	if args.Limit != nil && *args.Limit >= 0 && len(result.Results) > *args.Limit {
		result.Results = result.Results[:*args.Limit]
	}

	result.Debug = s.debug

	return &result, nil
}

// performSearch executes the search query against SearXNG
func (s *SearXNGSearcher) performSearch(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	if err := ValidateSearchArgs(args); err != nil {
		return nil, err
	}
	postReq, postBodyStr, err := s.buildSearchRequest(ctx, args)
	if err != nil {
		return nil, err
	}

	if s.debug {
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

	resp, err := s.client.Do(postReq)

	if s.debug && err == nil && resp != nil {
		slog.Debug("HTTP response",
			"status", resp.StatusCode,
			"content_type", resp.Header.Get("Content-Type"),
		)
	}

	if err == nil && resp != nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		if s.debug {
			slog.Debug("Redirecting to GET fallback", "status", resp.StatusCode, "reason", "POST not supported by server")
		}
		closeResponseBody(resp)
		getURL := *postReq.URL
		getURL.RawQuery = postBodyStr
		getReq, reqErr := http.NewRequestWithContext(ctx, "GET", getURL.String(), nil)
		if reqErr != nil {
			return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to create request: %w", reqErr))
		}
		setBrowserHeaders(getReq)

		if s.debug {
			slog.Debug("HTTP request",
				"method", getReq.Method,
				"url", getReq.URL.String(),
				"Accept", getReq.Header.Get("Accept"),
			)
		}

		resp, err = s.client.Do(getReq)

		if s.debug && err == nil && resp != nil {
			slog.Debug("HTTP response",
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
			)
		}
	}

	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("failed to execute search request: %w", err))
	}
	defer func() { closeResponseBody(resp) }()

	if resp.StatusCode != http.StatusOK {
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodySize))
		if readErr != nil {
			return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("failed to read error response body: %w", readErr))
		}
		if s.debug {
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
		truncated, truncErr := isBodyTruncated(resp.Body)
		if truncErr != nil {
			slog.Debug("isBodyTruncated read error", "error", truncErr)
		}
		if truncated {
			return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), fmt.Errorf("error response body exceeded maximum size limit of %d bytes", MaxErrorBodySize))
		}
		return nil, HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	return s.parseSearchResponse(resp, args)
}
