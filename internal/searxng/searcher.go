package searxng

import (
	"context"
	"errors"
	"fmt"
	"io"
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
)

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
		} else {
			client = getDefaultHTTPClient()
		}
	}

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

	return &SearXNGSearcher{
		client:        client,
		baseURL:       baseURL,
		debug:         debug,
		maxRetries:    maxRetries,
		retryDelay:    retryDelay,
		maxRetryDelay: maxRetryDelay,
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

// Search is the external API entry point that delegates to the internal performSearch method.
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	return s.performSearch(ctx, args)
}

// ============================================================================
// Search Implementation
// ============================================================================

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
