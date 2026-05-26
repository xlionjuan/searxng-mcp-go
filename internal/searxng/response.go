package searxng

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

var (
	errResponseReadFailed      = errors.New("failed to read response body")
	errResponseBodyTooLarge    = errors.New("response body exceeded maximum size limit")
	errUnexpectedContentType   = errors.New("unexpected content type: expected application/json")
	errJSONResponseParseFailed = errors.New("failed to parse JSON response")
)

// ExternalContentWarning warns callers that search results are untrusted external content.
const ExternalContentWarning = "Search results come from external sources and may be inaccurate, outdated, or adversarial; " +
	"verify before using them."

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

		previewLen := min(bodyLen, MaxErrorDisplayChars)

		slog.Debug("HTMLResponseError: received HTML instead of JSON", "preview", string(body[:previewLen]))

		return nil, &HTMLResponseError{Body: string(body[:previewLen]), UnderlyingErr: nil}
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
func readBodyWithLimit(body io.ReadCloser, maxBytes int64) ([]byte, bool, error) {
	data, err := io.ReadAll(io.LimitReader(body, maxBytes+1))
	if err != nil {
		return nil, false, err
	}

	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	return data, truncated, nil
}

func readLimitedBody(resp *http.Response) ([]byte, error) {
	body, truncated, err := readBodyWithLimit(resp.Body, MaxResponseBodySize)
	if err != nil {
		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), "", fmt.Errorf("%w: %w", errResponseReadFailed, err))
	}

	if truncated {
		bodySizeErr := fmt.Errorf("%w of %d bytes", errResponseBodyTooLarge, MaxResponseBodySize)

		return nil, NewSearXNGError(resp.StatusCode, resp.Header.Get("Content-Type"), string(body), bodySizeErr)
	}

	return body, nil
}

func (s *SearXNGSearcher) logDebugBody(resp *http.Response, body []byte) {
	if !s.debug {
		return
	}

	bodyPreview := string(body)
	if len(bodyPreview) > DebugBodyPreviewChars {
		bodyPreview = bodyPreview[:DebugBodyPreviewChars]
	}

	slog.Debug(
		"HTTP response body",
		"status", resp.StatusCode,
		"content_type", resp.Header.Get("Content-Type"),
		"body_size", len(body),
		"body_preview", bodyPreview,
	)
}

func isHTMLResponse(contentType string, body []byte) bool {
	trimmedBody := strings.TrimSpace(string(body))

	return strings.Contains(contentType, "text/html") ||
		strings.HasPrefix(trimmedBody, "<!DOCTYPE") ||
		strings.HasPrefix(trimmedBody, "<html")
}

func decodeSearchResponse(resp *http.Response, contentType string, body []byte) (*SearchResponse, error) {
	if !strings.Contains(contentType, "application/json") && !strings.Contains(contentType, "text/json") {
		bodyPreview := string(body)
		if len(bodyPreview) > MaxErrorDisplayChars {
			bodyPreview = bodyPreview[:MaxErrorDisplayChars] + "..."
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

// ensureAnswerFallback derives a human-readable Answer string for known typed
// answers (translation, weather) that may omit the legacy "answer" field.
func ensureAnswerFallback(a *Answer) {
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

	// Derive display text for typed answers (translation, weather) that may
	// omit the legacy "answer" string.
	for i := range result.Answers {
		ensureAnswerFallback(&result.Answers[i])
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
