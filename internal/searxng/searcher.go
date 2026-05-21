package searxng

import (
	"context"
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
var (
	defaultHTTPClient     *http.Client
	defaultHTTPClientOnce sync.Once
)

var (
	errRedirectDifferentHost    = errors.New("redirect to different host blocked")
	errTooManyRedirects         = errors.New("stopped after 10 redirects")
	errBaseURLEmpty             = errors.New("baseurl cannot be empty")
	errInvalidURL               = errors.New("invalid URL")
	errUnsupportedURLScheme     = errors.New("url must use http or https scheme")
	errURLMissingHost           = errors.New("url must include a host (e.g., search.example.com)")
	errSearcherConfigRequired   = errors.New("newSearXNGSearcher: config cannot be nil")
	errSearcherURLParseInternal = errors.New("newSearXNGSearcher: url.Parse failed after validateBaseURL passed (internal error)")
	errRequestCreateFailed      = errors.New("failed to create request")
	errSearchRequestFailed      = errors.New("failed to execute search request")
	errErrorBodyTooLarge        = errors.New("error response body exceeded maximum size limit")
)

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
		CheckRedirect: enforceSearchRedirectPolicy,
	}
}

func enforceSearchRedirectPolicy(req *http.Request, via []*http.Request) error {
	if req.URL != nil && len(via) > 0 {
		prevHost := via[len(via)-1].URL.Host
		if req.URL.Host != prevHost {
			return fmt.Errorf("%w: %s -> %s", errRedirectDifferentHost, prevHost, req.URL.Host)
		}
	}

	if len(via) >= 10 {
		return errTooManyRedirects
	}

	return nil
}

func withSearchRedirectPolicy(client *http.Client) *http.Client {
	if client == nil {
		return nil
	}

	originalCheckRedirect := client.CheckRedirect
	wrapped := *client
	wrapped.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			return err
		}

		if originalCheckRedirect != nil {
			return originalCheckRedirect(req, via)
		}

		return nil
	}

	return &wrapped
}

// getDefaultHTTPClient returns the shared default HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = newHTTPClient(30 * time.Second)
	})

	return defaultHTTPClient
}

// SearXNGSearcher performs web searches via a SearXNG instance.
type SearXNGSearcher struct {
	client  *http.Client // Configurable HTTP client
	baseURL string
	debug   bool // When true, enables verbose HTTP request/response logging
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration.
func NewSearXNGSearcher(cfg *Config, debug bool) (*SearXNGSearcher, error) {
	if cfg == nil {
		return nil, errSearcherConfigRequired
	}

	baseURL := cfg.SearXNGURL

	err := validateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("newSearXNGSearcher: %w", err)
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSearcherURLParseInternal, err)
	}

	if parsed.Scheme == "http" && !isPrivateHost(parsed.Host) {
		slog.Warn("Using HTTP for non-private host. Search queries may be transmitted in clear text. Search results could be intercepted and modified by a MITM attacker")
	}

	client := cfg.HTTPClient
	if client != nil {
		client = withSearchRedirectPolicy(client)
	} else {
		if cfg.Timeout > 0 {
			client = newHTTPClient(cfg.Timeout)
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

// Close releases resources held by the searcher.
func (s *SearXNGSearcher) Close() error {
	if s.client != nil && s.client.Transport != nil {
		if transport, ok := s.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	return nil
}

// validateBaseURL checks that the baseURL is valid and returns an error if not.
func validateBaseURL(baseURL string) error {
	if baseURL == "" {
		return errBaseURLEmpty
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("%w: %w", errInvalidURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errUnsupportedURLScheme
	}

	if parsed.Host == "" {
		return errURLMissingHost
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

	err := resp.Body.Close()
	if err != nil {
		slog.Debug("failed to close response body", "error", err)
	}
}

// isPrivateHost checks if the host is a private/internal address.
func isPrivateHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
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

// Search is the external API entry point that delegates to the internal performSearch method.
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	return s.performSearch(ctx, args)
}

// ============================================================================
// Search Implementation
// ============================================================================

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
	for _, ans := range answers {
		ans.EnsureFallback()

		if ans.Answer == "" {
			continue
		}

		prefix := ans.Answer

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

		lowerAnswer := strings.ToLower(ans.Answer)
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
			filtered = append(filtered, ans)
		}
	}

	return filtered
}

// performSearch executes the search query against SearXNG.
func (s *SearXNGSearcher) performSearch(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	err := ValidateSearchArgs(args)
	if err != nil {
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

		slog.Debug(
			"HTTP request",
			"method", postReq.Method,
			"url", postReq.URL.String(),
			"Content-Type", postReq.Header.Get("Content-Type"),
			"Accept", postReq.Header.Get("Accept"),
			"body", bodyPreview,
		)
	}

	resp, err := s.client.Do(postReq)

	if s.debug && err == nil && resp != nil {
		slog.Debug(
			"HTTP response",
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

		getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, getURL.String(), nil)
		if reqErr != nil {
			return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errRequestCreateFailed, reqErr))
		}

		setBrowserHeaders(getReq)

		if s.debug {
			slog.Debug(
				"HTTP request",
				"method", getReq.Method,
				"url", getReq.URL.String(),
				"Accept", getReq.Header.Get("Accept"),
			)
		}

		resp, err = s.client.Do(getReq)

		if s.debug && err == nil && resp != nil {
			slog.Debug(
				"HTTP response",
				"status", resp.StatusCode,
				"content_type", resp.Header.Get("Content-Type"),
			)
		}
	}

	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errSearchRequestFailed, err))
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

			slog.Debug(
				"HTTP error response body",
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
			err := fmt.Errorf("%w of %d bytes", errErrorBodyTooLarge, MaxErrorBodySize)

			return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), err)
		}

		return nil, HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
	}

	return s.parseSearchResponse(resp, args)
}
