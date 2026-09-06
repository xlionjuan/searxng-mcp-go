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
	errRedirectMethodChanged   = errors.New("redirect would change POST to GET; only 307/308 preserve POST semantics")
	errTooManyRedirects        = errors.New("stopped after 10 redirects")
)

const maxSearchRedirects = 10

func enforceSearchRedirectPolicy(req *http.Request, via []*http.Request) error {
	// This policy preserves the original scheme for same-host redirects.
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

			if prevScheme == schemeHTTPS && nextScheme == schemeHTTP {
				return fmt.Errorf("%w: %s -> %s", errRedirectSchemeDowngrade, prev.URL.Scheme, req.URL.Scheme)
			}
		}

		// Reject redirects that change the search method (e.g. 301, 302, 303
		// from POST to GET). Go's http.Client drops the body before calling
		// CheckRedirect for these status codes, silently discarding search
		// parameters such as q, format=json, and all search options.
		// via[0] is the original request (chronological order, oldest first);
		// req is the redirect target Go will issue next.
		origMethod := via[0].Method
		nextMethod := req.Method

		// HTTP methods are case-sensitive per RFC 9110 §9.2.1, and
		// Go's http.NewRequestWithContext does not canonicalise non-empty
		// methods. Use exact comparison.
		if origMethod != "" && nextMethod != "" && origMethod != nextMethod {
			return fmt.Errorf("%w: %s -> %s", errRedirectMethodChanged, origMethod, nextMethod)
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
	hostPart, port, err := net.SplitHostPort(host)
	if err != nil {
		// No port present — return as-is.
		return host
	}

	if (port == "443" && scheme == schemeHTTPS) || (port == "80" && scheme == schemeHTTP) {
		if strings.HasPrefix(host, "[") {
			return "[" + hostPart + "]"
		}

		return hostPart
	}

	return host
}
