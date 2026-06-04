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
	errRedirectDifferentHost   = errors.New("redirect to different host blocked")
	errRedirectSchemeDowngrade = errors.New("redirect from https to http blocked")
	errTooManyRedirects        = errors.New("stopped after 10 redirects")
	errBaseURLEmpty            = errors.New("baseurl cannot be empty")
	errInvalidURL              = errors.New("invalid URL")
	errUnsupportedURLScheme    = errors.New("url must use http or https scheme")
	errURLMissingHost          = errors.New("url must include a host (e.g., search.example.com)")
	errURLHasUserInfo          = errors.New("url must not contain userinfo (user:password@host)")
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
	classBPrefix                   = 192
	classBSecondOctet              = 168
	multicastFirstOctet            = 224
	ipv6MulticastPrefix            = 0xff
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
	// Note: This policy preserves the original scheme for same-host redirects.
	// An https → http downgrade is rejected so that configuring an HTTPS
	// SearXNG URL keeps subsequent request and response traffic on TLS.
	// An http → https upgrade is allowed because it strengthens, rather
	// than weakens, transport security.
	if req.URL != nil && len(via) > 0 {
		prev := via[len(via)-1]
		if prev.URL != nil {
			prevHost := prev.URL.Host
			if !strings.EqualFold(req.URL.Host, prevHost) {
				return fmt.Errorf("%w: %s -> %s", errRedirectDifferentHost, prevHost, req.URL.Host)
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

	if parsed.User != nil {
		return errURLHasUserInfo
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

// isPrivateHost reports whether the given host is RFC-grounded as a
// private/internal destination and therefore exempt from the HTTP warning.
//
// The contract is fully auditable against published RFCs and performs no DNS
// resolution:
//   - Hostname match: the exact name "localhost" or any name ending in
//     ".localhost" (RFC 6761 §6.3, Special-Use Domain Names).
//   - Literal IP match: the IPv4 and IPv6 ranges enumerated by
//     isPrivateIPv4 / isPrivateIPv6, each backed by a published RFC.
//
// Any other hostname — including ".lan", ".local", ".internal", ".home",
// ".corp", ".intranet", and ".home.arpa" — is NOT considered private and
// will trigger the HTTP warning. See docs/adr/003-http-warning-for-non-private-hosts.md.
func isPrivateHost(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err == nil {
		host = h
	}

	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}

	// Strip IPv6 zone ID (e.g., "fe80::1%eth0" → "fe80::1") before parsing.
	if i := strings.Index(host, "%"); i >= 0 {
		host = host[:i]
	}

	lowerHost := strings.ToLower(host)

	if lowerHost == "localhost" || strings.HasSuffix(lowerHost, ".localhost") {
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
	// 0.0.0.0/8 (current network)
	if ip4[0] == 0 {
		return true
	}
	// 10.0.0.0/8
	if ip4[0] == ipv4Private10 {
		return true
	}
	// 100.64.0.0/10 (CGNAT / shared address space)
	if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		return true
	}
	// 127.0.0.0/8 (loopback)
	if ip4[0] == ipv4Loopback127 {
		return true
	}
	// 169.254.0.0/16 (link-local)
	if ip4[0] == 169 && ip4[1] == 254 {
		return true
	}
	// 172.16.0.0/12
	if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
		return true
	}
	// 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 198.51.100.0/24, 203.0.113.0/24 (IETF / TEST-NET)
	if ip4[0] == classBPrefix {
		if ip4[1] == 0 && (ip4[2] == 0 || ip4[2] == 2) {
			return true
		}

		if ip4[1] == classBSecondOctet {
			return true // 192.168.0.0/16
		}

		if ip4[1] == 88 && ip4[2] == 99 {
			return true
		}
	}
	// 198.18.0.0/15 (benchmarking)
	if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
		return true
	}
	// 198.51.100.0/24
	if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
		return true
	}
	// 203.0.113.0/24
	if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
		return true
	}
	// 224.0.0.0/4 (multicast)
	if ip4[0]&0xf0 == multicastFirstOctet {
		return true
	}
	// 255.255.255.255/32 (limited broadcast)
	if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
		return true
	}

	return false
}

func isPrivateIPv6(ip net.IP) bool {
	// :: (unspecified)
	if ip.Equal(net.IPv6zero) {
		return true
	}
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
	// ff00::/8 (multicast)
	if ip[0] == ipv6MulticastPrefix {
		return true
	}

	return false
}
