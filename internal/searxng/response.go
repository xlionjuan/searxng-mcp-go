package searxng

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
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

		slog.Debug("HTMLResponseError: received HTML instead of JSON", "preview", preview)

		return nil, &HTMLResponseError{Body: preview, UnderlyingErr: nil}
	}

	result, err := decodeSearchResponse(resp, contentType, body)
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
			fmt.Errorf("%w: %w", errResponseReadFailed, err))
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

	slog.Debug(
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

	trimmedBody := bytes.TrimSpace(body)
	lowerBody := bytes.ToLower(trimmedBody)

	return bytes.HasPrefix(lowerBody, []byte("<!doctype")) ||
		bytes.HasPrefix(lowerBody, []byte("<html"))
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

func decodeSearchResponse(resp *http.Response, contentType string, body []byte) (*SearchResponse, error) {
	if !isJSONContentType(contentType) {
		bodyPreview := string(truncateBytesToValidUTF8(body, MaxErrorDisplayBytes))
		if len(body) > MaxErrorDisplayBytes {
			bodyPreview += "..."
		}

		slog.Debug("UnexpectedContentTypeError", "content_type", contentType, "body_preview", bodyPreview)

		return nil, NewSearXNGError(resp.StatusCode, contentType, "", errUnexpectedContentType)
	}

	var result SearchResponse

	err := json.Unmarshal(body, &result)
	if err != nil {
		slog.Debug("JSONParseError: failed to parse JSON response", "error", err)

		return nil, NewSearXNGError(resp.StatusCode, contentType, "", fmt.Errorf("%w: %w", errJSONResponseParseFailed, err))
	}

	return &result, nil
}

const maxWeatherSummaryParts = 3

// EnsureAnswerFallback derives a human-readable Answer string for known typed
// answers (translation, weather) that may omit the legacy "answer" field.
func EnsureAnswerFallback(a *Answer) {
	if strings.TrimSpace(a.Answer) != "" {
		return
	}

	if fallback := translationAnswerFallback(a); fallback != "" {
		a.Answer = fallback

		return
	}

	if fallback := weatherAnswerFallback(a); fallback != "" {
		a.Answer = fallback
	}
}

func translationAnswerFallback(a *Answer) string {
	if len(a.Translations) == 0 {
		return ""
	}

	parts := make([]string, 0, len(a.Translations))
	for _, item := range a.Translations {
		text := strings.TrimSpace(item.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "Translation: " + strings.Join(parts, "; ")
}

func weatherAnswerFallback(a *Answer) string {
	if a.Current == nil {
		return ""
	}

	current := a.Current
	if summary := strings.TrimSpace(current.Summary); summary != "" {
		return summary
	}

	parts := make([]string, 0, maxWeatherSummaryParts)
	if location := strings.TrimSpace(current.Location.Name); location != "" {
		parts = append(parts, location)
	}

	if temperature := current.Temperature.String(); temperature != "" {
		parts = append(parts, temperature)
	}

	if condition := strings.TrimSpace(current.Condition); condition != "" {
		parts = append(parts, condition)
	}

	if len(parts) == 0 {
		return ""
	}

	return "Weather: " + strings.Join(parts, ", ")
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
		slog.Warn("truncating answers before deduplication",
			"count", len(result.Answers),
			"max", MaxAnswers)
		result.Answers = result.Answers[:MaxAnswers]
	}

	if len(result.Infoboxes) > MaxInfoboxes {
		slog.Warn("truncating infoboxes before deduplication",
			"count", len(result.Infoboxes),
			"max", MaxInfoboxes)
		result.Infoboxes = result.Infoboxes[:MaxInfoboxes]
	}

	// Derive display text for typed answers (translation, weather) that may
	// omit the legacy "answer" string.
	for i := range result.Answers {
		EnsureAnswerFallback(&result.Answers[i])
	}

	result.Answers = deduplicateAnswers(result.Answers, result.Infoboxes)

	if args.Limit != nil && *args.Limit >= 0 && len(result.Results) > *args.Limit {
		result.Results = result.Results[:*args.Limit]
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
