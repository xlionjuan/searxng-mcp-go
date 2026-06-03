package searxng

import (
	"errors"
	"fmt"
	"net/http"
	"unicode/utf8"
)

var (
	errBadRequest          = errors.New("bad request: the query parameters may be invalid")
	errUnauthorized        = errors.New("unauthorized: authentication is required")
	errForbidden           = errors.New("forbidden: access denied")
	errNotFound            = errors.New("not found: the search endpoint could not be found")
	errRateLimited         = errors.New("rate limited: too many requests, please wait before making more searches")
	errInternalServerError = errors.New("internal server error: the search engine encountered an internal error")
	errBadGateway          = errors.New("bad gateway: received an invalid response from an upstream server")
	errServiceUnavailable  = errors.New("service unavailable: the search engine is temporarily unavailable")
	errGatewayTimeout      = errors.New("gateway timeout: timed out waiting for an upstream server")
	errUnexpectedStatus    = errors.New("unexpected status code received")
)

// ValidationError represents a user-provided parameter validation failure.
type ValidationError struct {
	Field   string // Field is the name of the invalid field
	Message string // Message describes the validation failure
}

// NewValidationError creates a new ValidationError.
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %q: %s", e.Field, e.Message)
}

// Is implements the errors.Is interface for ValidationError.
func (e *ValidationError) Is(target error) bool {
	ve, ok := target.(*ValidationError)
	if !ok {
		return false
	}

	return e.Field == ve.Field && e.Message == ve.Message
}

// truncateBody returns a truncated preview of body for error messages.
// The result is guaranteed to end on a valid UTF-8 rune boundary.
func truncateBody(body []byte, maxLen int) string {
	if len(body) == 0 || maxLen <= 0 {
		return ""
	}

	truncated := truncateBytesToValidUTF8(body, maxLen)

	return string(truncated)
}

// buildErrorPreview returns a preview of body suitable for storage in
// SearXNGError.ResponseBody. The size and UTF-8 safety contract are
// defined by MaxErrorDisplayChars; see its doc comment for details.
func buildErrorPreview(body []byte) string {
	return truncateBody(body, MaxErrorDisplayChars)
}

// truncateBytesToValidUTF8 returns data truncated to at most maxBytes bytes,
// walking back to a valid UTF-8 rune boundary to avoid splitting multi-byte sequences.
func truncateBytesToValidUTF8(data []byte, maxBytes int) []byte {
	if len(data) <= maxBytes {
		return data
	}

	data = data[:maxBytes]

	// Walk back to a valid UTF-8 rune boundary.
	for len(data) > 0 {
		r, size := utf8.DecodeLastRune(data)
		if r == utf8.RuneError && size == 1 {
			data = data[:len(data)-1]

			continue
		}

		break
	}

	return data
}

// isValidationError checks if an error is a ValidationError.
func isValidationError(err error) bool {
	var ve *ValidationError

	return errors.As(err, &ve)
}

// SearXNGError represents an error that occurred during communication with
// the SearXNG service.
type SearXNGError struct {
	StatusCode      int    // HTTP status code if available
	RespContentType string // Content-Type header from response
	ResponseBody    string // Truncated response body for debugging
	UnderlyingErr   error  // The original error that caused this
}

// NewSearXNGError creates a new SearXNGError.
func NewSearXNGError(statusCode int, contentType, body string, err error) *SearXNGError {
	return &SearXNGError{
		StatusCode:      statusCode,
		RespContentType: contentType,
		ResponseBody:    truncateBody([]byte(body), MaxErrorDisplayChars),
		UnderlyingErr:   err,
	}
}

func (e *SearXNGError) Error() string {
	if e.UnderlyingErr != nil {
		if e.RespContentType != "" {
			return fmt.Sprintf("searxng error (status %d) - content-type %s: %v", e.StatusCode, e.RespContentType, e.UnderlyingErr)
		}

		return fmt.Sprintf("searxng error (status %d): %v", e.StatusCode, e.UnderlyingErr)
	}

	return fmt.Sprintf("searxng error: status %d, content-type: %s", e.StatusCode, e.RespContentType)
}

// Unwrap returns the underlying error for errors.Is/ errors.As support.
func (e *SearXNGError) Unwrap() error {
	return e.UnderlyingErr
}

// HTTPStatusError creates a SearXNGError from an HTTP status code.
func HTTPStatusError(statusCode int, contentType string, body []byte) error {
	bodyStr := buildErrorPreview(body)

	var err error

	switch statusCode {
	case http.StatusBadRequest:
		err = errBadRequest
	case http.StatusUnauthorized:
		err = errUnauthorized
	case http.StatusForbidden:
		err = errForbidden
	case http.StatusNotFound:
		err = errNotFound
	case http.StatusTooManyRequests:
		err = errRateLimited
	case http.StatusInternalServerError:
		err = errInternalServerError
	case http.StatusBadGateway:
		err = errBadGateway
	case http.StatusServiceUnavailable:
		err = errServiceUnavailable
	case http.StatusGatewayTimeout:
		err = errGatewayTimeout
	default:
		err = errUnexpectedStatus
	}

	return NewSearXNGError(statusCode, contentType, bodyStr, err)
}

// HTMLResponseError creates a specialized error for HTML responses (JSON not enabled).
type HTMLResponseError struct {
	Body          string // Truncated HTML body
	UnderlyingErr error  // The underlying network error if any
}

func (e *HTMLResponseError) Error() string {
	return "searxng returned html instead of json - json output may not be enabled on the server"
}

func (e *HTMLResponseError) Unwrap() error {
	return e.UnderlyingErr
}
