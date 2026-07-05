package searxng

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
)

var (
	errRedirectDifferentHost   = errors.New("redirect to different host blocked")
	errRedirectSchemeDowngrade = errors.New("redirect from https to http blocked")
	errTooManyRedirects        = errors.New("stopped after 10 redirects")
)

const maxSearchRedirects = 10

func enforceSearchRedirectPolicy(req *http.Request, via []*http.Request) error {
	// Note: This policy preserves the original scheme for same-host redirects.
	// An https → http downgrade is rejected so that configuring an HTTPS
	// SearXNG URL keeps subsequent request and response traffic on TLS.
	// An http → https upgrade is allowed because it strengthens, rather
	// than weakens, transport security.
	if req.URL != nil && len(via) > 0 {
		prev := via[len(via)-1]
		if prev.URL != nil {
			prevHost := hostNoDefaultPort(prev.URL.Host, prev.URL.Scheme)
			nextHost := hostNoDefaultPort(req.URL.Host, req.URL.Scheme)

			if !strings.EqualFold(nextHost, prevHost) {
				return fmt.Errorf("%w: %s -> %s", errRedirectDifferentHost, prevHost, nextHost)
			}

			prevScheme := strings.ToLower(prev.URL.Scheme)

			nextScheme := strings.ToLower(req.URL.Scheme)

			if prevScheme == "https" && nextScheme == "http" {
				return fmt.Errorf("%w: %s -> %s", errRedirectSchemeDowngrade, prev.URL.Scheme, req.URL.Scheme)
			}
		}
	}

	if len(via) >= maxSearchRedirects {
		return errTooManyRedirects
	}

	return nil
}

// hostNoDefaultPort strips default port numbers (443 for https, 80 for http)
// from a host:port string. This allows same-host detection when a reverse proxy
// such as NGINX strips the default port during a redirect (e.g. host:443 → host).
func hostNoDefaultPort(host, scheme string) string {
	h, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port present — return as-is.
		return host
	}

	if (port == "443" && scheme == "https") || (port == "80" && scheme == "http") {
		if strings.HasPrefix(host, "[") {
			return "[" + h + "]"
		}

		return h
	}

	return host
}
