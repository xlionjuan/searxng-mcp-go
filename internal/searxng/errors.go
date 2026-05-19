package searxng

import (
	"errors"
	"fmt"
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

func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation error on %q: %s", e.Field, e.Message)
}

// Is implements the errors.Is interface for ValidationError
func (e *ValidationError) Is(target error) bool {
	ve, ok := target.(*ValidationError)
	if !ok {
		return false
	}
	return e.Field == ve.Field && e.Message == ve.Message
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{Field: field, Message: message}
}

// TruncateBody returns a truncated preview of body for error messages
func TruncateBody(body []byte, maxLen int) string {
	if len(body) == 0 || maxLen <= 0 {
		return ""
	}
	previewLen := len(body)
	if previewLen > maxLen {
		previewLen = maxLen
	}
	return string(body[:previewLen])
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) bool {
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

func (e *SearXNGError) Error() string {
	if e.UnderlyingErr != nil {
		if e.RespContentType != "" {
			return fmt.Sprintf("searxng error (status %d) - content-type %s: %v", e.StatusCode, e.RespContentType, e.UnderlyingErr)
		}
		return fmt.Sprintf("searxng error (status %d): %v", e.StatusCode, e.UnderlyingErr)
	}
	return fmt.Sprintf("searxng error: status %d, content-type: %s", e.StatusCode, e.RespContentType)
}

// Unwrap returns the underlying error for errors.Is/ errors.As support
func (e *SearXNGError) Unwrap() error {
	return e.UnderlyingErr
}

// NewSearXNGError creates a new SearXNGError
func NewSearXNGError(statusCode int, contentType, body string, err error) *SearXNGError {
	return &SearXNGError{
		StatusCode:      statusCode,
		RespContentType: contentType,
		ResponseBody:    TruncateBody([]byte(body), MaxErrorDisplayChars),
		UnderlyingErr:   err,
	}
}

// HTTPStatusError creates a SearXNGError from an HTTP status code
func HTTPStatusError(statusCode int, contentType string, body []byte) error {
	bodyStr := TruncateBody(body, MaxErrorDisplayChars)

	var err error
	switch statusCode {
	case 400:
		err = errBadRequest
	case 401:
		err = errUnauthorized
	case 403:
		err = errForbidden
	case 404:
		err = errNotFound
	case 429:
		err = errRateLimited
	case 500:
		err = errInternalServerError
	case 502:
		err = errBadGateway
	case 503:
		err = errServiceUnavailable
	case 504:
		err = errGatewayTimeout
	default:
		err = errUnexpectedStatus
	}

	return NewSearXNGError(statusCode, contentType, bodyStr, err)
}

// HTMLResponseError creates a specialized error for HTML responses (JSON not enabled)
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
