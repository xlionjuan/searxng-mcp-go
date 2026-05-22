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

const (
	transportDialTimeout           = 10 * time.Second
	transportKeepAlive             = 30 * time.Second
	transportTLSHandshakeTimeout   = 10 * time.Second
	transportResponseHeaderTimeout = 30 * time.Second
	transportIdleConnTimeout       = 90 * time.Second
	transportMaxIdleConns          = 100
	transportMaxIdleConnsPerHost   = 10
	maxSearchRedirects             = 10
)

const (
	ipv4ClassAPrivateOctet       = 10
	ipv4Private172FirstOctet     = 172
	ipv4Private172SecondMin      = 16
	ipv4Private172SecondMax      = 31
	ipv4Private192FirstOctet     = 192
	ipv4Private192SecondOctet    = 168
	ipv4LoopbackFirstOctet       = 127
	ipv4LinkLocalFirstOctet      = 169
	ipv4LinkLocalSecondOctet     = 254
	ipv4CurrentNetworkFirstOctet = 0
	ipv4SharedAddressFirstOctet  = 100
	ipv4SharedAddressMask        = 0xc0
	ipv4SharedAddressValue       = 0x40
	ipv4IETFProtocolSecondOctet  = 0
	ipv4IETFProtocolThirdOctet   = 0
	ipv4DocumentationThirdOctet  = 2
	ipv4BenchmarkFirstOctet      = 198
	ipv4BenchmarkSecondMin       = 18
	ipv4BenchmarkSecondMax       = 19
	ipv4ExampleSecondOctet       = 51
	ipv4ExampleThirdOctet        = 100
	ipv4Documentation203First    = 203
	ipv4Documentation203Second   = 0
	ipv4Documentation203Third    = 113
	ipv4MulticastFirstMin        = 224
	ipv6UniqueLocalMask          = 0xfe
	ipv6UniqueLocalValue         = 0xfc
	ipv6LinkLocalFirstOctet      = 0xfe
	ipv6LinkLocalMask            = 0xc0
	ipv6LinkLocalValue           = 0x80
)

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   transportDialTimeout,
				KeepAlive: transportKeepAlive,
			}).DialContext,
			TLSHandshakeTimeout:   transportTLSHandshakeTimeout,
			ResponseHeaderTimeout: transportResponseHeaderTimeout,
			IdleConnTimeout:       transportIdleConnTimeout,
			MaxIdleConns:          transportMaxIdleConns,
			MaxIdleConnsPerHost:   transportMaxIdleConnsPerHost,
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

	if len(via) >= maxSearchRedirects {
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
		defaultHTTPClient = newHTTPClient(DefaultTimeout)
	})

	return defaultHTTPClient
}

// SearXNGSearcher performs web searches via a SearXNG instance.
type SearXNGSearcher struct {
	client        *http.Client // Configurable HTTP client
	baseURL       string
	debug         bool // When true, enables verbose HTTP request/response logging
	maxRetries    int
	retryDelay    time.Duration
	maxRetryDelay time.Duration
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
		slog.Warn("Using HTTP for non-private host. " +
			"Search queries may be transmitted in clear text. " +
			"Search results could be intercepted and modified by a MITM attacker")
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

	maxRetries, retryDelay, maxRetryDelay := normalizeRetryConfig(cfg)

	return &SearXNGSearcher{
		client:        client,
		baseURL:       baseURL,
		debug:         debug,
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
		maxRetryDelay: maxRetryDelay,
	}, nil
}

func normalizeRetryConfig(cfg *Config) (int, time.Duration, time.Duration) {
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	maxRetryDelay := cfg.MaxRetryDelay
	if maxRetryDelay <= 0 {
		maxRetryDelay = DefaultMaxRetryDelay
	}

	if maxRetryDelay < retryDelay {
		maxRetryDelay = retryDelay
	}

	return maxRetries, retryDelay, maxRetryDelay
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
		return isPrivateIPv4(ip4)
	}

	return isPrivateIPv6(ip)
}

func isPrivateIPv4(ip4 net.IP) bool {
	if ip4[0] == ipv4ClassAPrivateOctet {
		return true
	}

	if ip4[0] == ipv4Private172FirstOctet &&
		ip4[1] >= ipv4Private172SecondMin &&
		ip4[1] <= ipv4Private172SecondMax {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet && ip4[1] == ipv4Private192SecondOctet {
		return true
	}

	if ip4[0] == ipv4LoopbackFirstOctet {
		return true
	}

	if ip4[0] == ipv4LinkLocalFirstOctet && ip4[1] == ipv4LinkLocalSecondOctet {
		return true
	}

	if ip4[0] == ipv4CurrentNetworkFirstOctet {
		return true
	}

	if ip4[0] == ipv4SharedAddressFirstOctet &&
		ip4[1]&ipv4SharedAddressMask == ipv4SharedAddressValue {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet &&
		ip4[1] == ipv4IETFProtocolSecondOctet &&
		ip4[2] == ipv4IETFProtocolThirdOctet {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet &&
		ip4[1] == ipv4IETFProtocolSecondOctet &&
		ip4[2] == ipv4DocumentationThirdOctet {
		return true
	}

	if ip4[0] == ipv4BenchmarkFirstOctet &&
		(ip4[1] == ipv4BenchmarkSecondMin || ip4[1] == ipv4BenchmarkSecondMax) {
		return true
	}

	if ip4[0] == ipv4BenchmarkFirstOctet &&
		ip4[1] == ipv4ExampleSecondOctet &&
		ip4[2] == ipv4ExampleThirdOctet {
		return true
	}

	if ip4[0] == ipv4Documentation203First &&
		ip4[1] == ipv4Documentation203Second &&
		ip4[2] == ipv4Documentation203Third {
		return true
	}

	if ip4[0] >= ipv4MulticastFirstMin {
		return true
	}

	return false
}

func isPrivateIPv6(ip net.IP) bool {
	if ip.Equal(net.IPv6loopback) {
		return true
	}

	if ip[0]&ipv6UniqueLocalMask == ipv6UniqueLocalValue {
		return true
	}

	if ip[0] == ipv6LinkLocalFirstOctet && ip[1]&ipv6LinkLocalMask == ipv6LinkLocalValue {
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

	infoboxTexts, lowerInfoboxTexts := collectInfoboxText(infoboxes)
	if len(infoboxTexts) == 0 {
		return answers
	}

	filtered := make([]Answer, 0, len(answers))
	for _, ans := range answers {
		ans.EnsureFallback()

		if ans.Answer == "" {
			continue
		}

		if answerPrefixMatch(ans.Answer, infoboxTexts, lowerInfoboxTexts) {
			continue
		}

		filtered = append(filtered, ans)
	}

	return filtered
}

func collectInfoboxText(infoboxes []Infobox) ([]string, []string) {
	infoboxTexts := make([]string, 0, len(infoboxes))
	lowerInfoboxTexts := make([]string, 0, len(infoboxes))

	for _, ib := range infoboxes {
		if ib.Content != "" {
			infoboxTexts = append(infoboxTexts, ib.Content)
			lowerInfoboxTexts = append(lowerInfoboxTexts, strings.ToLower(ib.Content))
		}
	}

	return infoboxTexts, lowerInfoboxTexts
}

func answerPrefixMatch(answer string, infoboxTexts []string, lowerInfoboxTexts []string) bool {
	const prefixLen = 200

	prefix := strings.TrimSuffix(answer, " More at Wikipedia")
	if len(prefix) > prefixLen {
		prefix = prefix[:prefixLen]
	}

	for _, text := range infoboxTexts {
		if strings.Contains(text, prefix) {
			return true
		}
	}

	lowerAnswer := strings.ToLower(answer)
	lowerAnswer = strings.TrimSuffix(lowerAnswer, " more at wikipedia")

	lowerPrefix := lowerAnswer
	if len(lowerAnswer) > prefixLen {
		lowerPrefix = lowerAnswer[:prefixLen]
	}

	for _, text := range lowerInfoboxTexts {
		if strings.Contains(text, lowerPrefix) {
			return true
		}
	}

	return false
}

// performSearch executes the search query against SearXNG.
func (s *SearXNGSearcher) performSearch(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	err := ValidateSearchArgs(args)
	if err != nil {
		return nil, err
	}

	if s.maxRetries == 0 {
		return s.executeSingleAttempt(ctx, args)
	}

	attempts := s.maxRetries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		resp, err := s.executeSearchAttempt(ctx, args)
		if err != nil {
			wrappedErr := fmt.Errorf("%w: %w", errSearchRequestFailed, err)
			if isRetryableError(ctx, err) && attempt+1 < attempts {
				delay := retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
				s.logDebugRetry(attempt, attempts, delay, wrappedErr)

				if waitErr := retryWait(ctx, delay); waitErr != nil {
					return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errSearchRequestFailed, waitErr))
				}

				continue
			}

			return nil, NewSearXNGError(0, "", "", wrappedErr)
		}

		if resp.StatusCode != http.StatusOK {
			statusErr := s.handleNonOKResponse(resp)
			statusCode := resp.StatusCode
			closeResponseBody(resp)

			if attempt+1 < attempts && isRetryableStatusCode(statusCode) {
				delay := retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
				s.logDebugRetry(attempt, attempts, delay, statusErr)

				if waitErr := retryWait(ctx, delay); waitErr != nil {
					return nil, statusErr
				}

				continue
			}

			return nil, statusErr
		}

		result, parseErr := s.parseSearchResponse(resp, args)
		closeResponseBody(resp)
		if parseErr != nil {
			return nil, parseErr
		}

		if s.isEmptyResponse(result) && attempt+1 < attempts {
			delay := retryBackoff(attempt, s.retryDelay, s.maxRetryDelay)
			s.logDebugRetry(attempt, attempts, delay, nil)

			if waitErr := retryWait(ctx, delay); waitErr != nil {
				return result, nil
			}

			continue
		}

		return result, nil
	}

	return nil, NewSearXNGError(0, "", "", errSearchRequestFailed)
}

func (s *SearXNGSearcher) executeSingleAttempt(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	postReq, postBodyStr, err := s.buildSearchRequest(ctx, args)
	if err != nil {
		return nil, err
	}

	s.logDebugRequest(postReq, postBodyStr)

	resp, err := s.client.Do(postReq)

	s.logDebugResponse(resp, err)

	if err == nil && resp != nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp, err = s.executeGETfallback(ctx, resp, postReq, postBodyStr)

		var searchErr *SearXNGError
		if errors.As(err, &searchErr) {
			return nil, searchErr
		}
	}

	if err != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errSearchRequestFailed, err))
	}

	defer func() { closeResponseBody(resp) }()

	if resp.StatusCode != http.StatusOK {
		return nil, s.handleNonOKResponse(resp)
	}

	return s.parseSearchResponse(resp, args)
}

func (s *SearXNGSearcher) executeSearchAttempt(ctx context.Context, args *SearchArgs) (*http.Response, error) {
	postReq, postBodyStr, err := s.buildSearchRequest(ctx, args)
	if err != nil {
		return nil, err
	}

	s.logDebugRequest(postReq, postBodyStr)

	resp, err := s.client.Do(postReq)

	s.logDebugResponse(resp, err)

	if err == nil && resp != nil && (resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp, err = s.executeGETfallback(ctx, resp, postReq, postBodyStr)
	}

	return resp, err
}

func (s *SearXNGSearcher) isEmptyResponse(resp *SearchResponse) bool {
	return len(resp.Results) == 0 &&
		len(resp.Infoboxes) == 0 &&
		len(resp.Answers) == 0 &&
		len(resp.Suggestions) == 0
}

func (s *SearXNGSearcher) logDebugRequest(req *http.Request, body string) {
	if !s.debug {
		return
	}

	if req.Method == http.MethodGet {
		slog.Debug(
			"HTTP request",
			"method", req.Method,
			"url", req.URL.String(),
			"Accept", req.Header.Get("Accept"),
		)

		return
	}

	bodyPreview := body
	if len(bodyPreview) > DebugBodyPreviewChars {
		bodyPreview = bodyPreview[:DebugBodyPreviewChars]
	}

	slog.Debug(
		"HTTP request",
		"method", req.Method,
		"url", req.URL.String(),
		"Content-Type", req.Header.Get("Content-Type"),
		"Accept", req.Header.Get("Accept"),
		"body", bodyPreview,
	)
}

func (s *SearXNGSearcher) logDebugResponse(resp *http.Response, err error) {
	if !s.debug || err != nil || resp == nil {
		return
	}

	slog.Debug(
		"HTTP response",
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
	)
}

func (s *SearXNGSearcher) logDebugRetry(attempt, maxAttempts int, delay time.Duration, err error) {
	if !s.debug {
		return
	}

	slog.Debug("retrying search request",
		"attempt", attempt+1,
		"max_attempts", maxAttempts,
		"delay", delay,
		"error", err,
	)
}

func (s *SearXNGSearcher) executeGETfallback(
	ctx context.Context,
	resp *http.Response,
	postReq *http.Request,
	postBodyStr string,
) (*http.Response, error) {
	if s.debug {
		slog.Debug("Redirecting to GET fallback", "status", resp.StatusCode, "reason", "POST not supported by server")
	}

	closeResponseBody(resp)

	getURL := *postReq.URL
	getURL.RawQuery = postBodyStr

	//nolint:gosec // GET fallback reuses the validated SearXNG base URL and only changes query parameters.
	getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, getURL.String(), nil)
	if reqErr != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errRequestCreateFailed, reqErr))
	}

	setBrowserHeaders(getReq)
	s.logDebugRequest(getReq, "")

	//nolint:gosec // The client redirect policy blocks fallback redirects to a different host.
	getResp, err := s.client.Do(getReq)
	s.logDebugResponse(getResp, err)

	return getResp, err
}

func (s *SearXNGSearcher) handleNonOKResponse(resp *http.Response) error {
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBodySize))
	if readErr != nil {
		return NewSearXNGError(
			resp.StatusCode, resp.Header.Get("Content-Type"), "",
			fmt.Errorf("failed to read error response body: %w", readErr),
		)
	}

	if s.debug {
		errBodyPreview := string(body)
		if len(errBodyPreview) > DebugBodyPreviewChars {
			errBodyPreview = errBodyPreview[:DebugBodyPreviewChars]
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

		return NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), err)
	}

	return HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
