package searxng

import (
	"errors"
	"fmt"
	"net/url"
)

var (
	errBaseURLEmpty         = errors.New("baseurl cannot be empty")
	errInvalidURL           = errors.New("invalid URL")
	errUnsupportedURLScheme = errors.New("url must use http or https scheme")
	errURLMissingHost       = errors.New("url must include a host (e.g., search.example.com)")
	errURLHasUserInfo       = errors.New("url must not contain userinfo (user:password@host)")
)

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

	if parsed.User != nil {
		return errURLHasUserInfo
	}

	return nil
}
