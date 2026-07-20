package searxng

import (
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"
)

// getDefaultHTTPClient returns the shared default HTTP client.
var getDefaultHTTPClient = sync.OnceValue(func() *http.Client {
	return newHTTPClient(DefaultTimeout)
})

const (
	transportDialTimeout           = 10 * time.Second
	transportKeepAlive             = 30 * time.Second
	transportTLSHandshakeTimeout   = 10 * time.Second
	transportResponseHeaderTimeout = 30 * time.Second
	transportIdleConnTimeout       = 90 * time.Second
	transportMaxIdleConns          = 100
	transportMaxIdleConnsPerHost   = 10
)

func newHTTPClient(timeout time.Duration) *http.Client {
	// Clone DefaultTransport to inherit ForceAttemptHTTP2, ProxyFromEnvironment,
	// and other standard settings, then override project-specific fields.
	// DefaultTransport is guaranteed to be *http.Transport in all current
	// and reasonably future Go versions; a failed assertion is a fatal
	// internal error that must not be silently papered over.
	//nolint:errcheck,forcetypeassert // Clone returns *Transport; no error.
	clone := http.DefaultTransport.(*http.Transport).Clone()
	clone.DialContext = (&net.Dialer{
		Timeout:   transportDialTimeout,
		KeepAlive: transportKeepAlive,
	}).DialContext
	clone.ResponseHeaderTimeout = transportResponseHeaderTimeout
	clone.MaxIdleConns = transportMaxIdleConns
	clone.MaxIdleConnsPerHost = transportMaxIdleConnsPerHost

	return &http.Client{
		Timeout:       timeout,
		Transport:     clone,
		CheckRedirect: enforceSearchRedirectPolicy,
	}
}

func closeResponseBody(resp *http.Response, logger *slog.Logger) {
	if resp == nil || resp.Body == nil {
		return
	}

	// Drain any unread data before closing to allow HTTP keep-alive
	// connection reuse. When readBodyWithLimit truncates at
	// MaxResponseBodySize, the unconsumed tail prevents the Go HTTP
	// transport from recycling the connection. A LimitReader bounds
	// the drain to defend against a slow or malicious server.
	//nolint:errcheck // drain is intentionally discarded for keep-alive
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, int64(MaxResponseBodySize)))

	err := resp.Body.Close()
	if err != nil {
		logger.Debug("failed to close response body", "error", err)
	}
}
