package searxng

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"searxng-mcp-go/internal/testhelper"
)

// --- validateBaseURL tests ---

func TestValidateBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		wantErr string
	}{
		{name: "empty", url: "", wantErr: "baseurl cannot be empty"},
		{name: "invalid URL", url: "://invalid", wantErr: "invalid URL"},
		{name: "unsupported scheme", url: "ftp://example.com", wantErr: "url must use http or https scheme"},
		{name: "missing host", url: "https:///search", wantErr: "url must include a host"},
		{name: "valid https", url: "https://search.example.com", wantErr: ""},
		{name: "valid http", url: "http://127.0.0.1:8080", wantErr: ""},
		{name: "valid with path", url: "https://example.com/searxng", wantErr: ""},
		{name: "rejects userinfo", url: "https://user:***@example.com/search", //nolint:gosec // password in test URL
			wantErr: "url must not contain userinfo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateBaseURL(tt.url)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateBaseURL(%q) = %v, want nil", tt.url, err)
				}

				return
			}

			if err == nil {
				t.Fatalf("validateBaseURL(%q) = nil, want error containing %q", tt.url, tt.wantErr)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("validateBaseURL(%q) = %q, want error containing %q", tt.url, err.Error(), tt.wantErr)
			}
		})
	}
}

// --- isPrivateHost / isPrivateIPv4 / isPrivateIPv6 tests ---

func TestIsPrivateHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
		want bool
	}{
		// Hostname-based private (RFC 6761 §6.3)
		{name: "localhost", host: "localhost", want: true},
		{name: "subdomain localhost", host: "foo.localhost", want: true},
		{name: "uppercase localhost", host: "LOCALHOST", want: true},
		// Non-RFC suffixes — no longer treated as private (ADR-003).
		{name: "lan suffix is not private", host: "nas.lan", want: false},
		{name: "internal suffix is not private", host: "db.internal", want: false},
		{name: "local suffix is not private", host: "printer.local", want: false},
		{name: "home suffix is not private", host: "router.home", want: false},
		{name: "corp suffix is not private", host: "wiki.corp", want: false},
		{name: "intranet suffix is not private", host: "wiki.intranet", want: false},
		{name: "home.arpa suffix is not private", host: "nas.home.arpa", want: false},
		// IPv4 private (RFC 1918, RFC 1122, RFC 5737, RFC 6598, RFC 6890, RFC 8190)
		{name: "10.x.x.x", host: "10.0.0.1", want: true},
		{name: "172.16.0.0", host: "172.16.0.1", want: true},
		{name: "172.31.255.255", host: "172.31.255.255", want: true},
		{name: "192.168.0.0", host: "192.168.1.1", want: true},
		{name: "127.0.0.1 loopback", host: "127.0.0.1", want: true},
		{name: "127.255.255.255 loopback", host: "127.255.255.255", want: true},
		// IPv6 private (RFC 4291, RFC 4193)
		{name: "IPv6 loopback", host: "::1", want: true},
		{name: "bracketed IPv6 loopback", host: "[::1]", want: true},
		{name: "IPv6 unique-local fc00", host: "fc00::1", want: true},
		{name: "bracketed IPv6 unique-local fc00", host: "[fc00::1]", want: true},
		{name: "IPv6 unique-local fd00", host: "fd12:3456::1", want: true},
		{name: "IPv6 link-local fe80", host: "fe80::1", want: true},
		{name: "bracketed IPv6 link-local with encoded zone", host: "[fe80::1%25eth0]", want: true},
		// Public
		{name: "public IPv4", host: "8.8.8.8", want: false},
		{name: "public domain", host: "example.com", want: false},
		{name: "172.32.x.x is public", host: "172.32.0.1", want: false},
		{name: "192.167.x.x is public", host: "192.167.1.1", want: false},
		{name: "IPv6 global unicast", host: "2001:4860:4860::8888", want: false},
		// With port
		{name: "localhost with port", host: "localhost:8080", want: true},
		{name: "10.0.0.1 with port", host: "10.0.0.1:9090", want: true},
		{name: "bracketed IPv6 loopback with port", host: "[::1]:8080", want: true},
		{name: "public with port", host: "example.com:443", want: false},
		{name: "IPv6 link-local with raw zone id", host: "fe80::1%eth0", want: true},
		{name: "bracketed IPv6 with raw zone id and port", host: "[fe80::1%eth0]:8080", want: true},
		{name: "non-RFC suffix with port", host: "printer.local:8080", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isPrivateHost(tt.host); got != tt.want {
				t.Fatalf("isPrivateHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestIsPrivateIPv4(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "10.0.0.0/8", ip: "10.0.0.1", want: true},
		{name: "10.255.255.255", ip: "10.255.255.255", want: true},
		{name: "172.16.0.0/12 start", ip: "172.16.0.0", want: true},
		{name: "172.31.255.255/12 end", ip: "172.31.255.255", want: true},
		{name: "192.168.0.0/16", ip: "192.168.0.1", want: true},
		{name: "192.168.255.255", ip: "192.168.255.255", want: true},
		{name: "127.0.0.0/8 loopback", ip: "127.0.0.1", want: true},
		{name: "127.255.255.255", ip: "127.255.255.255", want: true},
		// Additional IPv4 private ranges
		{name: "0.0.0.0/8 current network", ip: "0.0.0.1", want: true},
		{name: "100.64.0.0/10 CGNAT start", ip: "100.64.0.1", want: true},
		{name: "100.127.255.255 CGNAT end", ip: "100.127.255.255", want: true},
		{name: "169.254.0.0/16 link-local", ip: "169.254.1.1", want: true},
		{name: "192.0.0.0/24 IETF", ip: "192.0.0.1", want: true},
		{name: "192.0.2.0/24 TEST-NET-1", ip: "192.0.2.1", want: true},
		{name: "192.88.99.0/24 relay anycast", ip: "192.88.99.1", want: true},
		{name: "198.18.0.0/15 benchmarking start", ip: "198.18.0.1", want: true},
		{name: "198.19.255.255 benchmarking end", ip: "198.19.255.255", want: true},
		{name: "198.51.100.0/24 TEST-NET-2", ip: "198.51.100.1", want: true},
		{name: "203.0.113.0/24 TEST-NET-3", ip: "203.0.113.1", want: true},
		{name: "224.0.0.0/4 multicast start", ip: "224.0.0.1", want: true},
		{name: "239.255.255.255 multicast end", ip: "239.255.255.255", want: true},
		{name: "255.255.255.255/32 limited broadcast", ip: "255.255.255.255", want: true},
		// Public IPs that exercise false-return paths
		{name: "172.15.x.x is public", ip: "172.15.0.1", want: false},
		{name: "172.32.x.x is public", ip: "172.32.0.1", want: false},
		{name: "192.167.x.x is public", ip: "192.167.0.1", want: false},
		{name: "8.8.8.8 is public", ip: "8.8.8.8", want: false},
		{name: "1.1.1.1 is public", ip: "1.1.1.1", want: false},
		{name: "192.0.1.1 is public", ip: "192.0.1.1", want: false},
		{name: "192.0.3.1 is public", ip: "192.0.3.1", want: false},
		{name: "192.88.98.1 is public", ip: "192.88.98.1", want: false},
		{name: "192.88.100.1 is public", ip: "192.88.100.1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ip := net.ParseIP(tt.ip).To4()
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tt.ip)
			}

			if got := isPrivateIPv4(ip); got != tt.want {
				t.Fatalf("isPrivateIPv4(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

func TestIsPrivateIPv6(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "loopback ::1", ip: "::1", want: true},
		{name: "unique-local fc00::", ip: "fc00::1", want: true},
		{name: "unique-local fd00::", ip: "fd12:3456:789a::1", want: true},
		{name: "link-local fe80::", ip: "fe80::1", want: true},
		{name: "link-local fe81::", ip: "fe81::1", want: true},
		{name: "unspecified ::", ip: "::", want: true},
		{name: "multicast ff02::1", ip: "ff02::1", want: true},
		{name: "multicast ff00::", ip: "ff00::", want: true},
		{name: "global 2001::", ip: "2001:4860:4860::8888", want: false},
		{name: "global 2607::", ip: "2607:f8b0::1", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ip := net.ParseIP(tt.ip)
			if ip == nil {
				t.Fatalf("net.ParseIP(%q) returned nil", tt.ip)
			}

			if got := isPrivateIPv6(ip); got != tt.want {
				t.Fatalf("isPrivateIPv6(%q) = %v, want %v", tt.ip, got, tt.want)
			}
		})
	}
}

// --- enforceSearchRedirectPolicy tests ---

//nolint:gocognit,cyclop,gocyclo,maintidx // table-driven test covers many redirect scenarios
func TestEnforceSearchRedirectPolicy(t *testing.T) {
	t.Parallel()

	t.Run("no redirect when same host", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/search?q=test")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil", err)
		}
	})

	t.Run("block redirect to different host", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://evil.com/phishing")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		if !strings.Contains(err.Error(), "redirect to different host blocked") {
			t.Fatalf("error = %q, want redirect to different host blocked", err.Error())
		}
	})

	t.Run("block too many redirects", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}

		via := make([]*http.Request, 10)
		for i := range via {
			via[i] = &http.Request{URL: mustParseURL(t, "https://search.example.com/step")}
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		if !errors.Is(err, errTooManyRedirects) {
			t.Fatalf("error = %v, want errTooManyRedirects", err)
		}
	})

	t.Run("no via requests passes", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/search?q=test")}

		err := enforceSearchRedirectPolicy(req, nil)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil", err)
		}
	})

	t.Run("allow same host with different letter case", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.Example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for case-variant host", err)
		}
	})

	t.Run("allow same host differing only in letter case with port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://Search.Example.COM:8443/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:8443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for case-variant host with port", err)
		}
	})

	t.Run("block https to http scheme downgrade on same host", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		if !errors.Is(err, errRedirectSchemeDowngrade) {
			t.Fatalf("error = %v, want errRedirectSchemeDowngrade", err)
		}

		if !strings.Contains(err.Error(), "https -> http") {
			t.Fatalf("error = %q, want it to describe the https -> http downgrade", err.Error())
		}
	})

	t.Run("block https to http scheme downgrade with case-variant scheme", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "HTTP://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "HTTPS://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		if !errors.Is(err, errRedirectSchemeDowngrade) {
			t.Fatalf("error = %v, want errRedirectSchemeDowngrade", err)
		}
	})

	t.Run("block https to http scheme downgrade with port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com:8443/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:8443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		if !errors.Is(err, errRedirectSchemeDowngrade) {
			t.Fatalf("error = %v, want errRedirectSchemeDowngrade", err)
		}
	})

	t.Run("allow http to https scheme upgrade on same host", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "http://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for http -> https upgrade", err)
		}
	})

	t.Run("allow http to http same host redirect", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "http://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for http -> http same host", err)
		}
	})

	t.Run("allow https to https same host redirect", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for https -> https same host", err)
		}
	})

	// --- Default port stripping tests (issue #242) ---

	t.Run("allow :443 to no-port (same host, default port stripped)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for :443 → no-port redirect", err)
		}
	})

	t.Run("allow no-port to :443 (same host, default port added)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com:443/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for no-port → :443 redirect", err)
		}
	})

	t.Run("allow :80 to no-port (same host, default port stripped)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "http://search.example.com:80/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for :80 → no-port redirect", err)
		}
	})

	t.Run("allow no-port to :80 (same host, default port added)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com:80/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "http://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for no-port → :80 redirect", err)
		}
	})

	t.Run("non-default port is still considered different from no-port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:8080/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error for :8080 → no-port redirect")
		}
	})

	t.Run("allow IPv6 :443 to no-port (same host, default port stripped)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://[::1]/search")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://[::1]:443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for IPv6 :443 → no-port redirect", err)
		}
	})

	t.Run("allow IPv6 no-port to :443 (same host, default port added)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://[::1]:443/search")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://[::1]/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for IPv6 no-port → :443 redirect", err)
		}
	})

	t.Run("block IPv6 cross-host redirect", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://[::2]/search")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://[::1]:443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error for IPv6 cross-host redirect")
		}

		if !strings.Contains(err.Error(), "redirect to different host blocked") {
			t.Fatalf("error = %q, want redirect to different host blocked", err.Error())
		}
	})

	// --- Method-change redirect tests ---

	t.Run("block POST to GET redirect (301/302/303)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{Method: http.MethodGet, URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{Method: http.MethodPost, URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error for POST -> GET method change")
		}

		if !errors.Is(err, errRedirectMethodChanged) {
			t.Fatalf("error = %v, want errRedirectMethodChanged", err)
		}

		if !strings.Contains(err.Error(), "POST -> GET") {
			t.Fatalf("error = %q, want it to describe the POST -> GET method change", err.Error())
		}
	})

	t.Run("allow POST to POST redirect (307/308 preserves method)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{Method: http.MethodPost, URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{Method: http.MethodPost, URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for POST -> POST method-preserving redirect", err)
		}
	})

	t.Run("allow GET to GET redirect (GET fallback scenario)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{Method: http.MethodGet, URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{Method: http.MethodGet, URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil for GET -> GET redirect", err)
		}
	})

	t.Run("allow redirect when via[0] method is empty (default GET)", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{Method: http.MethodGet, URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil when via method is unset", err)
		}
	})

	t.Run("block POST to GET redirect preserves across scheme downgrade", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{Method: http.MethodGet, URL: mustParseURL(t, "http://search.example.com/result")}
		via := []*http.Request{
			{Method: http.MethodPost, URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error")
		}

		// Scheme downgrade is checked before method change, so the error
		// is errRedirectSchemeDowngrade — the POST body is still protected.
		if !errors.Is(err, errRedirectSchemeDowngrade) {
			t.Fatalf("error = %v, want errRedirectSchemeDowngrade (checked before method)", err)
		}
	})
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse(%q) = %v", raw, err)
	}

	return parsed
}

// --- closeResponseBody tests ---

func TestCloseResponseBody(t *testing.T) {
	t.Parallel()

	t.Run("nil response", func(t *testing.T) {
		t.Parallel()
		// Should not panic
		closeResponseBody(nil, slog.Default())
	})

	t.Run("nil body", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{Body: nil}
		// Should not panic
		closeResponseBody(resp, slog.Default())
	})

	t.Run("closes body", func(t *testing.T) {
		t.Parallel()

		closeCalled := false
		body := &closeTrackingReader{
			Reader: strings.NewReader("hello"),
			onClose: func() {
				closeCalled = true
			},
		}
		resp := &http.Response{Body: body}
		closeResponseBody(resp, slog.Default())

		if !closeCalled {
			t.Fatal("closeResponseBody() did not close the response body")
		}
	})
}

// closeTrackingReader wraps a reader and calls onClose when closed.
type closeTrackingReader struct {
	Reader  *strings.Reader
	onClose func()
}

func (r *closeTrackingReader) Read(p []byte) (int, error) {
	return r.Reader.Read(p)
}

func (r *closeTrackingReader) Close() error {
	r.onClose()

	return nil
}

// --- newHTTPClient tests ---

func TestNewHTTPClient(t *testing.T) {
	t.Parallel()

	timeout := 5 * time.Second
	client := newHTTPClient(timeout)

	if client.Timeout != timeout {
		t.Fatalf("Timeout = %v, want %v", client.Timeout, timeout)
	}

	if client.CheckRedirect == nil {
		t.Fatal("CheckRedirect = nil, want enforceSearchRedirectPolicy")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	if transport.MaxIdleConns != transportMaxIdleConns {
		t.Fatalf("MaxIdleConns = %d, want %d", transport.MaxIdleConns, transportMaxIdleConns)
	}

	if transport.MaxIdleConnsPerHost != transportMaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, transportMaxIdleConnsPerHost)
	}

	if transport.TLSHandshakeTimeout != transportTLSHandshakeTimeout {
		t.Fatalf("TLSHandshakeTimeout = %v, want %v", transport.TLSHandshakeTimeout, transportTLSHandshakeTimeout)
	}

	if transport.IdleConnTimeout != transportIdleConnTimeout {
		t.Fatalf("IdleConnTimeout = %v, want %v", transport.IdleConnTimeout, transportIdleConnTimeout)
	}
}

// errFakeTransport is a sentinel error used by TestNewHTTPClient_DefaultTransportFallback
// to satisfy the err113 linter (no dynamic errors in tests).
var errFakeTransport = errors.New("not a real transport")

// TestNewHTTPClient_DefaultTransportFallback verifies that newHTTPClient works
// even when http.DefaultTransport has been replaced with a non-*http.Transport
// implementation. The fallback path constructs a transport from scratch.
func TestNewHTTPClient_DefaultTransportFallback(t *testing.T) {
	// This test modifies http.DefaultTransport — a package-level global —
	// so it cannot use t.Parallel().

	// Replace DefaultTransport with a non-*http.Transport implementation.
	orig := http.DefaultTransport

	http.DefaultTransport = testhelper.RoundTripperFunc(func(_ *http.Request) (*http.Response, error) {
		return nil, errFakeTransport
	})
	defer func() { http.DefaultTransport = orig }()

	client := newHTTPClient(5 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport (fallback should construct one)")
	}

	if transport.MaxIdleConnsPerHost != transportMaxIdleConnsPerHost {
		t.Fatalf("MaxIdleConnsPerHost = %d, want %d", transport.MaxIdleConnsPerHost, transportMaxIdleConnsPerHost)
	}

	if transport.Proxy == nil {
		t.Fatal("Transport.Proxy is nil (fallback should set ProxyFromEnvironment)")
	}
}

func TestNewHTTPClient_ProxyFromEnvironment(t *testing.T) {
	t.Parallel()

	client := newHTTPClient(DefaultTimeout)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	if transport.Proxy == nil {
		t.Fatal("Transport.Proxy is nil, want http.ProxyFromEnvironment")
	}

	// Verify the proxy function returns nil for localhost and loopback
	// addresses. These are guaranteed by ProxyFromEnvironment's useProxy
	// regardless of proxy env var state.
	tests := []struct {
		name string
		url  string
	}{
		{name: "localhost", url: "http://localhost:8080/search"},
		{name: "loopback IPv4", url: "http://127.0.0.1:8080/search"},
		{name: "loopback IPv6", url: "http://[::1]:8080/search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := &http.Request{
				Method: http.MethodGet,
				URL:    mustParseURL(t, tt.url),
			}

			proxyURL, err := transport.Proxy(req)
			if err != nil {
				t.Fatalf("Proxy(%q) returned error: %v", tt.url, err)
			}

			if proxyURL != nil {
				t.Fatalf("Proxy(%q) = %v, want nil (loopback bypass)", tt.url, proxyURL)
			}
		})
	}
}

// proxySubprocessEnv is the sentinel env var that indicates the test is running
// inside a subprocess with proxy env vars pre-configured. Using a subprocess
// isolates the test from http.ProxyFromEnvironment's global sync.Once cache.
const proxySubprocessEnv = "_SEARXNG_MCP_TEST_PROXY"

// proxySubprocessCmd returns an exec.Cmd that re-runs the named test function
// in a subprocess. It uses t.Context() for cancellation (with a 30s timeout)
// and filters out proxy-related environment variables from the parent process
// so the child starts with a clean proxy environment.
func proxySubprocessCmd(t *testing.T, testName string, extraEnv ...string) *exec.Cmd {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)

	// Filter out proxy-related env vars from the parent to avoid
	// interference (e.g. NO_PROXY=* from the test runner's shell).
	env := make([]string, 0, len(os.Environ())+len(extraEnv))

	for _, e := range os.Environ() {
		key, _, _ := strings.Cut(e, "=")

		switch strings.ToLower(key) {
		case "http_proxy", "https_proxy", "no_proxy", "all_proxy":
			continue
		}

		env = append(env, e)
	}

	env = append(env, extraEnv...)

	//nolint:gosec // G204: test binary path is controlled by the test runner
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run", "^"+testName+"$")
	cmd.Env = env

	return cmd
}

// TestNewHTTPClient_ProxyEnvHonored verifies that setting HTTP_PROXY makes the
// transport return the configured proxy URL for non-loopback requests.
//
// The test re-executes itself in a subprocess with HTTP_PROXY set, because
// http.ProxyFromEnvironment caches its configuration on first call via a
// global sync.Once that cannot be reset from outside net/http.
func TestNewHTTPClient_ProxyEnvHonored(t *testing.T) {
	if os.Getenv(proxySubprocessEnv) == "" {
		cmd := proxySubprocessCmd(t, "TestNewHTTPClient_ProxyEnvHonored",
			proxySubprocessEnv+"=1",
			"HTTP_PROXY=http://test-proxy:8080",
			"HTTPS_PROXY=http://test-proxy:8080",
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("subprocess test failed: %v\n%s", err, out)
		}

		return
	}

	// Child process: HTTP_PROXY is set and the sync.Once cache is fresh.
	client := newHTTPClient(DefaultTimeout)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	req := &http.Request{
		Method: http.MethodGet,
		URL:    mustParseURL(t, "http://example.com/search"),
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy(req) returned error: %v", err)
	}

	if proxyURL == nil {
		t.Fatal("Proxy(req) = nil, want http://test-proxy:8080")
	}

	if proxyURL.String() != "http://test-proxy:8080" {
		t.Fatalf("Proxy(req) = %q, want http://test-proxy:8080", proxyURL.String())
	}

	// Also verify the HTTPS request uses the same proxy.
	req2 := &http.Request{
		Method: http.MethodGet,
		URL:    mustParseURL(t, "https://example.com/search"),
	}

	proxyURL2, err := transport.Proxy(req2)
	if err != nil {
		t.Fatalf("Proxy(req) for HTTPS returned error: %v", err)
	}

	if proxyURL2 == nil {
		t.Fatal("Proxy(req) for HTTPS = nil, want http://test-proxy:8080")
	}

	if proxyURL2.String() != "http://test-proxy:8080" {
		t.Fatalf("Proxy(req) for HTTPS = %q, want http://test-proxy:8080", proxyURL2.String())
	}
}

// TestNewHTTPClient_NoProxyBypass verifies that NO_PROXY bypasses the proxy
// for matching destinations.
//
// The test re-executes itself in a subprocess with HTTP_PROXY and NO_PROXY
// set, for the same sync.Once isolation reason as ProxyEnvHonored.
func TestNewHTTPClient_NoProxyBypass(t *testing.T) {
	if os.Getenv(proxySubprocessEnv) == "" {
		cmd := proxySubprocessCmd(t, "TestNewHTTPClient_NoProxyBypass",
			proxySubprocessEnv+"=1",
			"HTTP_PROXY=http://test-proxy:8080",
			"HTTPS_PROXY=http://test-proxy:8080",
			"NO_PROXY=example.com",
		)

		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("subprocess test failed: %v\n%s", err, out)
		}

		return
	}

	// Child process: HTTP_PROXY and NO_PROXY are set.
	client := newHTTPClient(DefaultTimeout)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	// Matching NO_PROXY — should bypass proxy.
	req := &http.Request{
		Method: http.MethodGet,
		URL:    mustParseURL(t, "http://example.com/search"),
	}

	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy(req) for NO_PROXY match returned error: %v", err)
	}

	if proxyURL != nil {
		t.Fatalf("Proxy(req) for NO_PROXY match = %v, want nil", proxyURL)
	}

	// Non-matching host — should use proxy.
	req2 := &http.Request{
		Method: http.MethodGet,
		URL:    mustParseURL(t, "http://other.com/search"),
	}

	proxyURL2, err := transport.Proxy(req2)
	if err != nil {
		t.Fatalf("Proxy(req) for non-matching host returned error: %v", err)
	}

	if proxyURL2 == nil {
		t.Fatal("Proxy(req) for non-matching host = nil, want http://test-proxy:8080")
	}

	if proxyURL2.String() != "http://test-proxy:8080" {
		t.Fatalf("Proxy(req) for non-matching host = %q, want http://test-proxy:8080", proxyURL2.String())
	}
}

// TestNewHTTPClient_ProxyServerRouting verifies that when a Proxy function is
// configured on the transport, requests are actually routed through the proxy
// server rather than going direct.
func TestNewHTTPClient_ProxyServerRouting(t *testing.T) {
	t.Parallel()

	proxyCalled := make(chan struct{}, 1)

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		proxyCalled <- struct{}{}

		w.WriteHeader(http.StatusOK)
	}))
	defer proxy.Close()

	proxyURL, err := url.Parse(proxy.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q) = %v", proxy.URL, err)
	}

	client := newHTTPClient(5 * time.Second)

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("Transport is not *http.Transport")
	}

	// Replace the proxy function with one that always points to our test proxy.
	// The target is a non-loopback address so the transport routes through
	// the proxy rather than going direct.
	transport.Proxy = http.ProxyURL(proxyURL)

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://198.51.100.1/search", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequestWithContext failed: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	//nolint:errcheck,gosec // drain + close for keep-alive; error is intentionally discarded
	resp.Body.Close()

	select {
	case <-proxyCalled:
		// Proxy was used — success.
	default:
		t.Fatal("Request was not routed through the proxy server")
	}
}

// --- getDefaultHTTPClient tests ---

func TestGetDefaultHTTPClient(t *testing.T) {
	t.Parallel()

	c1 := getDefaultHTTPClient()
	c2 := getDefaultHTTPClient()

	if c1 != c2 {
		t.Fatal("getDefaultHTTPClient() should return the same singleton instance")
	}

	if c1.Timeout != DefaultTimeout {
		t.Fatalf("Timeout = %v, want %v", c1.Timeout, DefaultTimeout)
	}
}
