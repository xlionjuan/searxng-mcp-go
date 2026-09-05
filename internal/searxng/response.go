package searxng

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"unicode"
	"unicode/utf8"

	"searxng-mcp-go/internal/searxng/answer"
)

var (
	errResponseReadFailed      = errors.New("failed to read response body")
	errResponseBodyTooLarge    = errors.New("response body exceeded maximum size limit")
	errUnexpectedContentType   = errors.New("unexpected content type: expected application/json")
	errJSONResponseParseFailed = errors.New("failed to parse JSON response")
)

// ExternalContentWarning warns callers that search results are untrusted external content.
const ExternalContentWarning = "Search results come from external sources" +
	" and may be inaccurate, outdated, or adversarial; verify before using them."

// parseSearchResponse reads and parses the response from a SearXNG search request.
func (s *SearXNGSearcher) parseSearchResponse(resp *http.Response, args *SearchArgs) (*SearchResponse, error) {
	body, err := readLimitedBody(resp)
	if err != nil {
		return nil, err
	}

	s.logDebugBody(resp, body)

	contentType := resp.Header.Get("Content-Type")
	if isHTMLResponse(contentType, body) {
		bodyLen := len(body)
		if bodyLen == 0 {
			return nil, &HTMLResponseError{Body: "", UnderlyingErr: nil}
		}

		previewLen := min(bodyLen, MaxErrorDisplayBytes)
		preview := string(truncateBytesToValidUTF8(body, previewLen))

		s.getLogger().Debug("HTMLResponseError: received HTML instead of JSON", "preview", preview)

		return nil, &HTMLResponseError{Body: preview, UnderlyingErr: nil}
	}

	result, err := decodeSearchResponse(resp, contentType, body, s.getLogger())
	if err != nil {
		return nil, err
	}

	s.normalizeResponse(result, args)

	return result, nil
}

// readBodyWithLimit reads up to maxBytes+1 bytes from body and returns the data,
// a boolean indicating if the body was truncated (exceeded maxBytes), and any error.
// When truncated, it walks back to a valid UTF-8 rune boundary to avoid splitting runes.
func readBodyWithLimit(body io.ReadCloser, maxBytes int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}

	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
		// Walk back to a valid UTF-8 rune boundary to avoid splitting multi-byte runes.
		// Only strip truly invalid trailing bytes (size == 1), not valid runes like
		// U+FFFD which also decode as RuneError but with size == 3.
		for len(data) > 0 {
			r, size := utf8.DecodeLastRune(data)
			if r == utf8.RuneError && size == 1 {
				data = data[:len(data)-1]

				continue
			}

			break
		}
	}

	return data, truncated, nil
}

func readLimitedBody(resp *http.Response) ([]byte, error) {
	body, truncated, err := readBodyWithLimit(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, NewSearXNGError(
			resp.StatusCode, resp.Header.Get("Content-Type"), "",
			fmt.Errorf("%w: %w", errResponseReadFailed, err),
		)
	}

	if truncated {
		bodySizeErr := fmt.Errorf("%w of %d bytes", errResponseBodyTooLarge, MaxResponseBodySize)

		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), buildErrorPreview(body), bodySizeErr)
	}

	return body, nil
}

func (s *SearXNGSearcher) logDebugBody(resp *http.Response, body []byte) {
	if !s.debug {
		return
	}

	bodyPreview := string(truncateBytesToValidUTF8(body, DebugBodyPreviewBytes))

	s.getLogger().Debug(
		"HTTP response body",
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"body_size", len(body),
		"body_preview", bodyPreview,
	)
}

func isHTMLResponse(contentType string, body []byte) bool {
	if isHTMLContentType(contentType) {
		return true
	}

	return hasHTMLPrefix(body)
}

// maxHTMLProbe is the maximum number of bytes examined from the body for
// HTML prefix detection. It covers leading whitespace plus the longest
// prefix we check (<!doctype or <html).
const maxHTMLProbe = 64

// hasHTMLPrefix reports whether body has <!doctype or <html prefix after
// skipping leading Unicode whitespace, examining at most maxHTMLProbe bytes.
// It avoids allocating a lowercase copy of the entire body.
func hasHTMLPrefix(body []byte) bool {
	probe := body[:min(len(body), maxHTMLProbe)]
	i := 0

	for i < len(probe) {
		r, size := utf8.DecodeRune(probe[i:])

		if r == utf8.RuneError {
			break
		}

		if !unicode.IsSpace(r) {
			break
		}

		i += size
	}

	remaining := probe[i:]
	if len(remaining) == 0 {
		return false
	}

	return equalFoldPrefix(remaining, "<!doctype") || equalFoldPrefix(remaining, "<html")
}

// equalFoldPrefix reports whether b starts with prefix s under ASCII
// case-insensitive comparison. Both b and s are assumed to contain only
// ASCII bytes. The caller is responsible for bounding how much of b is
// examined.
func equalFoldPrefix(b []byte, s string) bool {
	if len(b) < len(s) {
		return false
	}

	for i := range len(s) {
		if b[i]|0x20 != s[i]|0x20 {
			return false
		}
	}

	return true
}

// isJSONContentType reports whether the Content-Type header value, when parsed
// as a media type, identifies a JSON payload (application/json or text/json).
// The comparison is case-insensitive and parameters such as charset are
// ignored. An empty or unparseable Content-Type returns false.
func isJSONContentType(contentType string) bool {
	return mediaTypeIs(contentType, "application/json", "text/json")
}

// isHTMLContentType reports whether the Content-Type header value, when parsed
// as a media type, identifies an HTML payload (text/html). The comparison is
// case-insensitive and parameters such as charset are ignored. An empty or
// unparseable Content-Type returns false.
func isHTMLContentType(contentType string) bool {
	return mediaTypeIs(contentType, "text/html")
}

// mediaTypeIs reports whether contentType, when parsed as a media type,
// matches any of want. mime.ParseMediaType lowercases the type and parameter
// names, so a successful parse yields a normalized base type. If parsing
// fails, the content type is treated as not matching, which lets body-based
// fallbacks (e.g., HTML body detection) still apply.
func mediaTypeIs(contentType string, want ...string) bool {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return slices.Contains(want, mt)
}

func decodeSearchResponse(
	resp *http.Response, contentType string, body []byte, logger *slog.Logger,
) (*SearchResponse, error) {
	if !isJSONContentType(contentType) {
		bodyPreview := string(truncateBytesToValidUTF8(body, MaxErrorDisplayBytes))
		if len(body) > MaxErrorDisplayBytes {
			bodyPreview += "..."
		}

		logger.Debug("UnexpectedContentTypeError", "content_type", contentType, "body_preview", bodyPreview)

		return nil, NewSearXNGError(resp.StatusCode, contentType, "", errUnexpectedContentType)
	}

	var result SearchResponse

	err := json.Unmarshal(body, &result)
	if err != nil {
		logger.Debug("JSONParseError: failed to parse JSON response", "error", err)

		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("%w: %w", errJSONResponseParseFailed, err))
	}

	return &result, nil
}

func (s *SearXNGSearcher) normalizeResponse(result *SearchResponse, args *SearchArgs) {
	result.Warning = ExternalContentWarning

	if result.NumberOfResults == 0 && len(result.Results) > 0 {
		result.NumberOfResults = len(result.Results)
	}

	// Cap answers and infoboxes before deduplication. The dedup pass is
	// O(n*m) substring work; a configured upstream that returns a body
	// below MaxResponseBodySize could still pack tens of thousands of
	// compact entries into these arrays, forcing very large CPU work
	// before result limiting. See MaxAnswers / MaxInfoboxes for details.
	if len(result.Answers) > MaxAnswers {
		s.getLogger().Warn("truncating answers before deduplication",
			"count", len(result.Answers),
			"max", MaxAnswers)
		result.Answers = slices.Clone(result.Answers[:MaxAnswers])
	}

	if len(result.Infoboxes) > MaxInfoboxes {
		s.getLogger().Warn("truncating infoboxes before deduplication",
			"count", len(result.Infoboxes),
			"max", MaxInfoboxes)
		result.Infoboxes = slices.Clone(result.Infoboxes[:MaxInfoboxes])
	}

	// Derive display text for typed answers (translation, weather) that may
	// omit the legacy "answer" string.
	for i := range result.Answers {
		EnsureAnswerFallback(&result.Answers[i])
	}

	result.Answers = deduplicateAnswers(result.Answers, result.Infoboxes)

	if args.Limit != nil && *args.Limit >= 0 && len(result.Results) > *args.Limit {
		result.Results = slices.Clone(result.Results[:*args.Limit])
	}

	// Normalize nil slices to empty slices for consistent JSON output.
	if result.Results == nil {
		result.Results = []SearchResult{}
	}

	if result.Suggestions == nil {
		result.Suggestions = []string{}
	}

	result.Debug = s.debug

	if s.debug && result.UnresponsiveEngines == nil {
		result.UnresponsiveEngines = [][]string{}
	}
}

// EnsureAnswerFallback derives a human-readable Answer string for known typed
// answers (translation, weather) that may omit the legacy "answer" field.
func EnsureAnswerFallback(a *Answer) {
	answer.EnsureAnswerFallback(a)
}
