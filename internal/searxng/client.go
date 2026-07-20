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
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   transportDialTimeout,
				KeepAlive: transportKeepAlive,
			}).DialContext,
			TLSHandshakeTimeout:   transportTLSHandshakeTimeout,
			ResponseHeaderTimeout: transportResponseHeaderTimeout,
			IdleConnTimeout:       transportIdleConnTimeout,
			MaxIdleConns:          transportMaxIdleConns,
			MaxIdleConnsPerHost:   transportMaxIdleConnsPerHost,
		},
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
