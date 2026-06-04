package searxng

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"
)

var (
	errSearcherConfigRequired   = errors.New("newSearXNGSearcher: config cannot be nil")
	errSearcherURLParseInternal = errors.New("newSearXNGSearcher: url.Parse failed after validateBaseURL passed (internal error)")
	errRequestCreateFailed      = errors.New("failed to create request")
	errSearchRequestFailed      = errors.New("failed to execute search request")
	errErrorBodyTooLarge        = errors.New("error response body exceeded maximum size limit")
	errEmptyResponse            = errors.New("empty response from SearXNG")
	errGETFallbackUsed          = errors.New("GET fallback was used; search query parameters may have been sent in the request URL")
)

const getFallbackLogRisk = "Search query parameters may be sent in upstream URLs and recorded by " +
	"SearXNG, proxy, CDN, or access logs"

// SearXNGSearcher performs web searches via a SearXNG instance.
type SearXNGSearcher struct {
	client           *http.Client // Configurable HTTP client
	searchEndpoint   *url.URL     // Precomputed /search endpoint URL; cloned per request
	debug            bool         // When true, enables verbose HTTP request/response logging
	maxRetries       int
	retryStrategy    *exponentialBackoffStrategy
	ownsTransport    bool // true if the searcher created its own transport (safe to close)
	allowGETFallback bool
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration.
func NewSearXNGSearcher(cfg *Config, debug bool) (*SearXNGSearcher, error) {
	if cfg == nil {
		return nil, errSearcherConfigRequired
	}

	// Validate config first to catch invalid negative values before normalization
	err := cfg.Validate()
	if err != nil {
		return nil, fmt.Errorf("newSearXNGSearcher: %w", err)
	}

	// Normalize config: apply safe defaults for zero values
	cfg = cfg.Normalize()

	baseURL := cfg.SearXNGURL

	err = validateBaseURL(baseURL)
	if err != nil {
		return nil, fmt.Errorf("newSearXNGSearcher: %w", err)
	}

	// Precompute the /search endpoint URL once so per-request construction
	// only has to clone the result. computeSearchEndpoint may still fail
	// after validateBaseURL (defensive); surface that as an internal error.
	searchEndpoint, err := computeSearchEndpoint(baseURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errSearcherURLParseInternal, err)
	}

	if searchEndpoint.Scheme == "http" && !isPrivateHost(searchEndpoint.Host) {
		slog.Warn("Using HTTP for non-private host. " +
			"Search queries may be transmitted in clear text. " +
			"Search results could be intercepted and modified by a MITM attacker")
	}

	if cfg.AllowGETFallback {
		slog.Warn("GET fallback is enabled. " + getFallbackLogRisk)
	}

	client := cfg.HTTPClient

	var ownsTransport bool

	if client != nil {
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
		client = &wrapped
	} else {
		if cfg.Timeout > 0 {
			client = newHTTPClient(cfg.Timeout)
			ownsTransport = true
		} else {
			client = getDefaultHTTPClient()
			// ownsTransport remains false — we do not own the shared singleton
		}
	}

	return &SearXNGSearcher{
		client:           client,
		searchEndpoint:   searchEndpoint,
		debug:            debug,
		maxRetries:       cfg.MaxRetries,
		retryStrategy:    newExponentialBackoffStrategy(cfg.MaxRetries, cfg.RetryDelay, cfg.MaxRetryDelay),
		ownsTransport:    ownsTransport,
		allowGETFallback: cfg.AllowGETFallback,
	}, nil
}

// Close releases resources held by the searcher.
// Close is safe to call on searchers that use a shared default client —
// it will skip closing idle connections on transports it does not own.
func (s *SearXNGSearcher) Close() error {
	if s.ownsTransport && s.client != nil && s.client.Transport != nil {
		if transport, ok := s.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	return nil
}

// Search executes the search query against SearXNG with retry support.
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	err := ValidateSearchArgs(args)
	if err != nil {
		return nil, err
	}

	var lastErr error

	for attempt := 0; attempt <= s.maxRetries; attempt++ {
		resp, _, err := s.doSearchAttempt(ctx, args)

		shouldRetry, delay := s.retryStrategy.ShouldRetry(ctx, attempt, resp, err)

		if shouldRetry {
			// Retry with backoff
			s.logDebugRetry(attempt, s.maxRetries+1, delay, err)
			lastErr = err

			if resp != nil {
				closeResponseBody(resp)
			}

			waitErr := retryWait(ctx, delay)
			if waitErr != nil {
				lastErr = waitErr

				break
			}

			continue
		}

		// No more retries — handle final result
		if err != nil {
			lastErr = err

			break
		}

		result, finishErr := s.finishResponse(resp, args)
		if finishErr != nil {
			return nil, finishErr
		}

		// Retry empty responses if retries remain
		if attempt < s.maxRetries && s.isEmptyResponse(result) {
			shouldRetry, delay = s.retryStrategy.ShouldRetry(ctx, attempt, resp, errEmptyResponse)
			if shouldRetry {
				s.logDebugRetry(attempt, s.maxRetries+1, delay, nil)

				lastErr = errEmptyResponse

				waitErr := retryWait(ctx, delay)
				if waitErr != nil {
					lastErr = waitErr

					break
				}

				continue
			}
		}

		return result, nil
	}

	if lastErr != nil {
		return nil, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errSearchRequestFailed, lastErr))
	}

	// Should never reach here
	return nil, NewSearXNGError(0, "", "", errSearchRequestFailed)
}

// finishResponse handles non-OK status, JSON parsing, and body closure for a response.
func (s *SearXNGSearcher) finishResponse(resp *http.Response, args *SearchArgs) (*SearchResponse, error) {
	defer closeResponseBody(resp)

	if resp.StatusCode != http.StatusOK {
		return nil, s.handleNonOKResponse(resp)
	}

	return s.parseSearchResponse(resp, args)
}

func (s *SearXNGSearcher) doSearchAttempt(ctx context.Context, args *SearchArgs) (*http.Response, string, error) {
	postReq, postBodyStr, err := s.buildSearchRequest(ctx, args)
	if err != nil {
		return nil, postBodyStr, err
	}

	s.logDebugRequest(postReq, postBodyStr)

	resp, err := s.client.Do(postReq)

	s.logDebugResponse(resp, err)

	if err == nil && resp != nil && s.allowGETFallback &&
		(resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented) {
		resp, err = s.executeGETfallback(ctx, resp, postReq, postBodyStr)
	}

	return resp, postBodyStr, err
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

	bodyPreview := string(truncateBytesToValidUTF8([]byte(body), DebugBodyPreviewChars))

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

	slog.Debug(
		"retrying search request",
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
	slog.Warn(
		"GET fallback used after POST search was rejected. "+getFallbackLogRisk,
		"status", resp.StatusCode,
	)

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
	if err != nil {
		err = fmt.Errorf("%w: %w", errGETFallbackUsed, redactSearchURLParamsFromError(err))
	}

	s.logDebugResponse(getResp, err)

	return getResp, err
}

func redactSearchURLParamsFromError(err error) error {
	if err == nil {
		return nil
	}

	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return err
	}

	redacted := *urlErr
	redacted.URL = redactSearchURLParams(urlErr.URL)

	return &redacted
}

func redactSearchURLParams(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}

	query := parsed.Query()
	if _, ok := query["q"]; ok {
		query.Set("q", "[REDACTED]")
	}

	parsed.RawQuery = query.Encode()

	return parsed.String()
}

func (s *SearXNGSearcher) handleNonOKResponse(resp *http.Response) error {
	body, truncated, readErr := readBodyWithLimit(resp.Body, MaxErrorBodySize)
	if readErr != nil {
		return NewSearXNGError(
			resp.StatusCode, resp.Header.Get("Content-Type"), "",
			fmt.Errorf("failed to read error response body: %w", readErr),
		)
	}

	if s.debug {
		errBodyPreview := string(truncateBytesToValidUTF8(body, DebugBodyPreviewChars))

		slog.Debug(
			"HTTP error response body",
			"status", resp.StatusCode,
			"content_type", resp.Header.Get("Content-Type"),
			"body_size", len(body),
			"body_preview", errBodyPreview,
		)
	}

	if truncated {
		return NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), buildErrorPreview(body),
			fmt.Errorf("%w of %d bytes", errErrorBodyTooLarge, MaxErrorBodySize))
	}

	return HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}
