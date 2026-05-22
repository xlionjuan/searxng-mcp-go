package searxng

import (
	"errors"
	"fmt"
	"io"
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
)

const (
	ipv4ClassAPrivateOctet       = 10
	ipv4Private172FirstOctet     = 172
	ipv4Private172SecondMin      = 16
	ipv4Private172SecondMax      = 31
	ipv4Private192FirstOctet     = 192
	ipv4Private192SecondOctet    = 168
	ipv4LoopbackFirstOctet       = 127
	ipv4LinkLocalFirstOctet      = 169
	ipv4LinkLocalSecondOctet     = 254
	ipv4CurrentNetworkFirstOctet = 0
	ipv4SharedAddressFirstOctet  = 100
	ipv4SharedAddressMask        = 0xc0
	ipv4SharedAddressValue       = 0x40
	ipv4IETFProtocolSecondOctet  = 0
	ipv4IETFProtocolThirdOctet   = 0
	ipv4DocumentationThirdOctet  = 2
	ipv4BenchmarkFirstOctet      = 198
	ipv4BenchmarkSecondMin       = 18
	ipv4BenchmarkSecondMax       = 19
	ipv4ExampleSecondOctet       = 51
	ipv4ExampleThirdOctet        = 100
	ipv4Documentation203First    = 203
	ipv4Documentation203Second   = 0
	ipv4Documentation203Third    = 113
	ipv4MulticastFirstMin        = 224
	ipv6UniqueLocalMask          = 0xfe
	ipv6UniqueLocalValue         = 0xfc
	ipv6LinkLocalFirstOctet      = 0xfe
	ipv6LinkLocalMask            = 0xc0
	ipv6LinkLocalValue           = 0x80
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

func withSearchRedirectPolicy(client *http.Client) *http.Client {
	if client == nil {
		return nil
	}

	originalCheckRedirect := client.CheckRedirect
	wrapped := *client
	wrapped.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			return err
		}

		if originalCheckRedirect != nil {
			return originalCheckRedirect(req, via)
		}

		return nil
	}

	return &wrapped
}

// getDefaultHTTPClient returns the shared default HTTP client.
func getDefaultHTTPClient() *http.Client {
	defaultHTTPClientOnce.Do(func() {
		defaultHTTPClient = newHTTPClient(DefaultTimeout)
	})

	return defaultHTTPClient
}

func normalizeRetryConfig(cfg *Config) (int, time.Duration, time.Duration) {
	maxRetries := cfg.MaxRetries
	if maxRetries < 0 {
		maxRetries = 0
	}

	retryDelay := cfg.RetryDelay
	if retryDelay <= 0 {
		retryDelay = DefaultRetryDelay
	}

	maxRetryDelay := cfg.MaxRetryDelay
	if maxRetryDelay <= 0 {
		maxRetryDelay = DefaultMaxRetryDelay
	}

	if maxRetryDelay < retryDelay {
		maxRetryDelay = retryDelay
	}

	return maxRetries, retryDelay, maxRetryDelay
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

func isBodyTruncated(body io.Reader) (bool, error) {
	buf := make([]byte, 1)

	_, err := body.Read(buf)
	if err == nil {
		return true, nil
	}

	if err == io.EOF {
		return false, nil
	}

	return false, err
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
	if ip4[0] == ipv4ClassAPrivateOctet {
		return true
	}

	if ip4[0] == ipv4Private172FirstOctet &&
		ip4[1] >= ipv4Private172SecondMin &&
		ip4[1] <= ipv4Private172SecondMax {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet && ip4[1] == ipv4Private192SecondOctet {
		return true
	}

	if ip4[0] == ipv4LoopbackFirstOctet {
		return true
	}

	if ip4[0] == ipv4LinkLocalFirstOctet && ip4[1] == ipv4LinkLocalSecondOctet {
		return true
	}

	if ip4[0] == ipv4CurrentNetworkFirstOctet {
		return true
	}

	if ip4[0] == ipv4SharedAddressFirstOctet &&
		ip4[1]&ipv4SharedAddressMask == ipv4SharedAddressValue {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet &&
		ip4[1] == ipv4IETFProtocolSecondOctet &&
		ip4[2] == ipv4IETFProtocolThirdOctet {
		return true
	}

	if ip4[0] == ipv4Private192FirstOctet &&
		ip4[1] == ipv4IETFProtocolSecondOctet &&
		ip4[2] == ipv4DocumentationThirdOctet {
		return true
	}

	if ip4[0] == ipv4BenchmarkFirstOctet &&
		(ip4[1] == ipv4BenchmarkSecondMin || ip4[1] == ipv4BenchmarkSecondMax) {
		return true
	}

	if ip4[0] == ipv4BenchmarkFirstOctet &&
		ip4[1] == ipv4ExampleSecondOctet &&
		ip4[2] == ipv4ExampleThirdOctet {
		return true
	}

	if ip4[0] == ipv4Documentation203First &&
		ip4[1] == ipv4Documentation203Second &&
		ip4[2] == ipv4Documentation203Third {
		return true
	}

	if ip4[0] >= ipv4MulticastFirstMin {
		return true
	}

	return false
}

func isPrivateIPv6(ip net.IP) bool {
	if ip.Equal(net.IPv6loopback) {
		return true
	}

	if ip[0]&ipv6UniqueLocalMask == ipv6UniqueLocalValue {
		return true
	}

	if ip[0] == ipv6LinkLocalFirstOctet && ip[1]&ipv6LinkLocalMask == ipv6LinkLocalValue {
		return true
	}

	return false
}
