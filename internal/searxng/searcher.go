package searxng

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"
)

var (
	errSearcherConfigRequired   = errors.New("newSearXNGSearcher: config cannot be nil")
	errSearcherURLParseInternal = errors.New(
		"newSearXNGSearcher: url.Parse failed after validateBaseURL passed (internal error)")
	errRequestCreateFailed = errors.New("failed to create request")
	errSearchRequestFailed = errors.New("failed to execute search request")
	errErrorBodyTooLarge   = errors.New("error response body exceeded maximum size limit")
	errSearchEmptyResults  = errors.New("search returned empty results after all retries")
	errGETFallbackUsed     = errors.New(
		"GET fallback was used; search query parameters may have been sent in the request URL")
	errNilFinishResponse = errors.New("finishResponse: nil http.Response")
)

const getFallbackLogRisk = "Search query parameters may be sent in upstream URLs and recorded by " +
	"SearXNG, proxy, CDN, or access logs"

// SearXNGSearcher performs web searches via a SearXNG instance.
type SearXNGSearcher struct {
	client           *http.Client // Configurable HTTP client
	searchEndpoint   *url.URL     // Precomputed /search endpoint URL; cloned per request
	debug            bool         // When true, enables verbose HTTP request/response logging
	logger           *slog.Logger // Logger for warnings and debug output; nil = slog.Default()
	done             chan struct{}
	closeOnce        sync.Once
	retryStrategy    *exponentialBackoffStrategy
	ownsTransport    bool // true if the searcher created its own transport (safe to close)
	allowGETFallback bool
}

// NewSearXNGSearcher creates a new SearXNGSearcher with the given configuration.
// Returns an error if cfg is nil, cfg.Validate fails, the base URL is empty
// or invalid, or endpoint construction fails internally.
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

	logger := slog.Default()
	if cfg.Logger != nil {
		logger = cfg.Logger
	}

	if searchEndpoint.Scheme == "http" && !isPrivateHost(searchEndpoint.Host) {
		logger.Warn("Using HTTP for non-private host. " +
			"Search queries may be transmitted in clear text. " +
			"Search results could be intercepted and modified by a MITM attacker")
	}

	if cfg.AllowGETFallback {
		logger.Warn("GET fallback is enabled. " + getFallbackLogRisk)
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

	s := &SearXNGSearcher{
		client:           client,
		searchEndpoint:   searchEndpoint,
		debug:            debug,
		logger:           logger,
		retryStrategy:    newExponentialBackoffStrategy(cfg.MaxRetries, cfg.RetryDelay, cfg.MaxRetryDelay),
		ownsTransport:    ownsTransport,
		allowGETFallback: cfg.AllowGETFallback,
	}
	s.done = make(chan struct{})

	return s, nil
}

// Close releases resources held by the searcher and cancels in-flight searches.
// Close is safe to call on searchers that use a shared default client —
// it will skip closing idle connections on transports it does not own.
func (s *SearXNGSearcher) Close() error {
	s.closeOnce.Do(func() {
		if s.done != nil {
			close(s.done)
		}
	})

	if s.ownsTransport && s.client != nil && s.client.Transport != nil {
		if transport, ok := s.client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}

	return nil
}

// Search executes the search query against SearXNG with retry support.
// Returns SearXNGError wrapping the last error if all retries are exhausted.
// Returns ValidationError if args are invalid.
//
//nolint:gocognit,gocyclo,cyclop // orchestrates distinct concerns; extracting adds indirection
func (s *SearXNGSearcher) Search(ctx context.Context, args *SearchArgs) (*SearchResponse, error) {
	normalized, err := ValidateSearchArgs(args)
	if err != nil {
		return nil, err
	}

	args = normalized

	// Tie search lifecycle to searcher close
	searchCtx, searchCancel := s.searchContext(ctx)
	defer searchCancel()

	var lastErr error

	maxRetries := s.retryStrategy.MaxRetries()

	for attempt := 0; attempt <= maxRetries; attempt++ {
		resp, _, err := s.doSearchAttempt(searchCtx, args)

		ar, classifyErr := s.classifyAttempt(searchCtx, attempt, maxRetries, resp, err, args)
		if classifyErr != nil {
			return nil, classifyErr
		}

		if ar.outcome == OutcomeSuccess {
			if maxRetries > 0 && s.isEmptyResponse(ar.result) {
		s.getLogger().Warn(
				"search returned empty after exhausting retries",
				"attempts", attempt+1,
			)
			}

			return ar.result, nil
		}

		// Determine the error to track for this attempt
		trackErr := err
		if trackErr == nil && ar.outcome == OutcomeEmptyRetry {
			trackErr = errSearchEmptyResults
		}

		shouldRetry, delay := s.retryStrategy.ShouldRetry(searchCtx, attempt, ar.outcome)

		if !shouldRetry {
			if trackErr == nil && searchCtx.Err() != nil {
				trackErr = searchCtx.Err()
			}

			if trackErr != nil {
				lastErr = trackErr

				closeResponseBody(resp, s.getLogger())
			} else if resp != nil && resp.StatusCode != http.StatusOK {
				_, finishErr := s.finishResponse(resp, args)

				return nil, finishErr
			}

			break
		}

		// Track last error for final wrapping
		lastErr = trackErr

		s.logDebugRetry(attempt, maxRetries+1, delay, lastErr)

		closeResponseBody(resp, s.getLogger())

		waitErr := retryWait(searchCtx, delay)
		if waitErr != nil {
			lastErr = waitErr

			break
		}
	}

	return nil, wrapSearchError(lastErr)
}

// searchContext creates a context tied to the searcher lifecycle.
// The returned context is canceled when the searcher is closed.
func (s *SearXNGSearcher) searchContext(ctx context.Context) (context.Context, context.CancelFunc) {
	searchCtx, cancel := context.WithCancel(ctx)

	go func() {
		select {
		case <-s.done:
			cancel()
		case <-searchCtx.Done():
		}
	}()

	return searchCtx, cancel
}

// wrapSearchError wraps the last error for the Search return value.
// Preserves SearXNGError unwrapping to avoid hiding the real status code.
// Returns the fallback error for the "should never reach here" case when err is nil.
func wrapSearchError(err error) error {
	var se *SearXNGError
	if errors.As(err, &se) {
		return fmt.Errorf("%w: %w", errSearchRequestFailed, err)
	}

	if err != nil {
		return NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errSearchRequestFailed, err))
	}

	// Should never reach here
	return NewSearXNGError(0, "", "", errSearchRequestFailed)
}

// attemptResult holds the result of classifying a single search attempt.
type attemptResult struct {
	outcome Outcome
	result  *SearchResponse
}

// classifyAttempt processes the response from a single search attempt and
// determines its Outcome. Returns an error only if finishResponse itself
// fails (parse error, non-retryable HTTP status). For all other cases, it
// returns an attemptResult whose Outcome guides the retry decision.
func (s *SearXNGSearcher) classifyAttempt(
	ctx context.Context,
	attempt, maxRetries int,
	resp *http.Response,
	err error,
	args *SearchArgs,
) (*attemptResult, error) {
	if err != nil {
		return &attemptResult{outcome: classifyOutcome(ctx, attempt, maxRetries, resp, err, false)}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &attemptResult{outcome: classifyOutcome(ctx, attempt, maxRetries, resp, nil, false)}, nil
	}

	result, finishErr := s.finishResponse(resp, args)
	if finishErr != nil {
		return nil, finishErr
	}

	// finishResponse closed resp.Body via defer; nil it so the retry
	// loop does not close the same body a second time.
	resp.Body = nil

	isEmpty := s.isEmptyResponse(result)
	outcome := classifyOutcome(ctx, attempt, maxRetries, resp, nil, isEmpty)

	return &attemptResult{outcome: outcome, result: result}, nil
}

// finishResponse handles non-OK status, JSON parsing, and body closure for a response.
//
// resp is required: passing nil triggers errNilFinishResponse. The retry
// path inside Search never reaches here with a nil response, but guarding
// the field makes the function robust against future refactors and
// self-documents the precondition.
func (s *SearXNGSearcher) finishResponse(resp *http.Response, args *SearchArgs) (*SearchResponse, error) {
	if resp == nil {
		return nil, errNilFinishResponse
	}

	defer closeResponseBody(resp, s.getLogger())

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

	shouldFallbackToGET := err == nil && resp != nil && s.allowGETFallback &&
		(resp.StatusCode == http.StatusMethodNotAllowed || resp.StatusCode == http.StatusNotImplemented)
	if shouldFallbackToGET {
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
		s.getLogger().Debug(
			"HTTP request",
			"method", req.Method,
			"url", redactSearchURLParams(req.URL.String()),
			"Accept", req.Header.Get("Accept"),
		)

		return
	}

	bodyPreview := string(truncateBytesToValidUTF8([]byte(body), DebugBodyPreviewBytes))

	s.getLogger().Debug(
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

	s.getLogger().Debug(
		"HTTP response",
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
	)
}

func (s *SearXNGSearcher) logDebugRetry(attempt, maxAttempts int, delay time.Duration, err error) {
	if !s.debug {
		return
	}

	s.getLogger().Debug(
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
	s.getLogger().Warn(
		"GET fallback used after POST search was rejected. "+getFallbackLogRisk,
		"status", resp.StatusCode,
	)

	if s.debug {
		s.getLogger().Debug("Redirecting to GET fallback",
			"status", resp.StatusCode,
			"reason", "POST not supported by server")
	}

	// Capture the original 405/501 error before closing the response body so the
	// hint (including the "set SEARXNG_ALLOW_GET_FALLBACK=1" message) is preserved
	// when GET fallback also fails.
	origErr := HTTPStatusError(resp.StatusCode, resp.Header.Get("Content-Type"), nil)

	closeResponseBody(resp, s.getLogger())

	getURL := *postReq.URL
	getURL.RawQuery = postBodyStr

	//nolint:gosec // GET fallback reuses the validated SearXNG base URL and only changes query parameters.
	getReq, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, getURL.String(), http.NoBody)
	if reqErr != nil {
		return nil, errors.Join(origErr, NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errRequestCreateFailed, reqErr)))
	}

	setBrowserHeaders(getReq)
	s.logDebugRequest(getReq, "")

	//nolint:gosec // The client redirect policy blocks fallback redirects to a different host.
	getResp, err := s.client.Do(getReq)
	if err != nil {
		err = fmt.Errorf("%w: %w", errGETFallbackUsed, redactSearchURLParamsFromError(err))
		err = errors.Join(origErr, err)
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

	if parsed.RawQuery != "" {
		parsed.RawQuery = "[REDACTED]"
	}

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
		errBodyPreview := string(truncateBytesToValidUTF8(body, DebugBodyPreviewBytes))

		s.getLogger().Debug(
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

// getLogger returns the searcher's logger, falling back to slog.Default() if nil.
func (s *SearXNGSearcher) getLogger() *slog.Logger {
	if s.logger != nil {
		return s.logger
	}

	return slog.Default()
}
