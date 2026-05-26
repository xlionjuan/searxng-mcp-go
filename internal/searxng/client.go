package searxng

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// defaultHTTPClient is the shared client used when callers do not request a custom timeout.
var (
	defaultHTTPClient     *http.Client
	defaultHTTPClientOnce sync.Once
)

var (
	errRedirectDifferentHost = errors.New("redirect to different host blocked")
	errTooManyRedirects      = errors.New("stopped after 10 redirects")
	errBaseURLEmpty          = errors.New("baseurl cannot be empty")
	errInvalidURL            = errors.New("invalid URL")
	errUnsupportedURLScheme  = errors.New("url must use http or https scheme")
	errURLMissingHost        = errors.New("url must include a host (e.g., search.example.com)")
)

const (
	transportDialTimeout           = 10 * time.Second
	transportKeepAlive             = 30 * time.Second
	transportTLSHandshakeTimeout   = 10 * time.Second
	transportResponseHeaderTimeout = 30 * time.Second
	transportIdleConnTimeout       = 90 * time.Second
	transportMaxIdleConns          = 100
	transportMaxIdleConnsPerHost   = 10
	maxSearchRedirects             = 10
	ipv4Private10                  = 10
	ipv4Loopback127                = 127
	ipv6UniqueLocalPrefix          = 0xfc
)

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
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

func enforceSearchRedirectPolicy(req *http.Request, via []*http.Request) error {
	if req.URL != nil && len(via) > 0 {
		prevHost := via[len(via)-1].URL.Host
		if req.URL.Host != prevHost {
			return fmt.Errorf("%w: %s -> %s", errRedirectDifferentHost, prevHost, req.URL.Host)
		}
	}

	if len(via) >= maxSearchRedirects {
		return errTooManyRedirects
	}

	return nil
}

// getDefaultHTTPClient returns the shared default HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = newHTTPClient(DefaultTimeout)
	})

	return defaultHTTPClient
}

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

	return nil
}

func closeResponseBody(resp *http.Response) {
	if resp == nil || resp.Body == nil {
		return
	}

	err := resp.Body.Close()
	if err != nil {
		slog.Debug("failed to close response body", "error", err)
	}
}

// isPrivateHost checks if the host is a private/internal address.
func isPrivateHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		host = h
	}

	if host == "localhost" || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return true
	}

	lowerHost := strings.ToLower(host)
	if strings.HasSuffix(lowerHost, ".lan") ||
		strings.HasSuffix(lowerHost, ".internal") ||
		strings.HasSuffix(lowerHost, ".local") ||
		strings.HasSuffix(lowerHost, ".home") {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip4 := ip.To4(); ip4 != nil {
		return isPrivateIPv4(ip4)
	}

	return isPrivateIPv6(ip)
}

func isPrivateIPv4(ip4 net.IP) bool {
	// 10.0.0.0/8
	if ip4[0] == ipv4Private10 {
		return true
	}
	// 172.16.0.0/12
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	// 192.168.0.0/16
	if ip4[0] == 192 && ip4[1] == 168 {
		return true
	}
	// 127.0.0.0/8 (loopback)
	if ip4[0] == ipv4Loopback127 {
		return true
	}

	return false
}

func isPrivateIPv6(ip net.IP) bool {
	// ::1 (loopback)
	if ip.Equal(net.IPv6loopback) {
		return true
	}
	// fc00::/7 (unique-local)
	if ip[0]&0xfe == ipv6UniqueLocalPrefix {
		return true
	}
	// fe80::/10 (link-local)
	if ip[0] == 0xfe && ip[1]&0xc0 == 0x80 {
		return true
	}

	return false
}
