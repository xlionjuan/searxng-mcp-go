package searxng

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

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
