package searxng

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
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
		{name: "172.15.x.x is public", ip: "172.15.0.1", want: false},
		{name: "172.32.x.x is public", ip: "172.32.0.1", want: false},
		{name: "192.167.x.x is public", ip: "192.167.0.1", want: false},
		{name: "8.8.8.8 is public", ip: "8.8.8.8", want: false},
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

//nolint:gocognit,gocyclo,cyclop // table-driven test covers many redirect scenarios
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

	t.Run("allow https redirect that drops explicit :443 port", func(t *testing.T) {
		t.Parallel()

		// url.URL.Host is "search.example.com:443" for the first request and
		// "search.example.com" for the redirect; both represent the same host.
		req := &http.Request{URL: mustParseURL(t, "https://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil when :443 is dropped from https host", err)
		}
	})

	t.Run("allow https redirect that adds explicit :443 port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://search.example.com:443/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil when :443 is added to https host", err)
		}
	})

	t.Run("allow http redirect that drops explicit :80 port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "http://search.example.com/result")}
		via := []*http.Request{
			{URL: mustParseURL(t, "http://search.example.com:80/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err != nil {
			t.Fatalf("enforceSearchRedirectPolicy() = %v, want nil when :80 is dropped from http host", err)
		}
	})

	t.Run("block redirect to genuinely different host even with default port", func(t *testing.T) {
		t.Parallel()

		req := &http.Request{URL: mustParseURL(t, "https://evil.com/phishing")}
		via := []*http.Request{
			{URL: mustParseURL(t, "https://search.example.com:443/search")},
		}

		err := enforceSearchRedirectPolicy(req, via)
		if err == nil {
			t.Fatal("enforceSearchRedirectPolicy() = nil, want error for genuinely different host")
		}

		if !errors.Is(err, errRedirectDifferentHost) {
			t.Fatalf("error = %v, want errRedirectDifferentHost", err)
		}
	})
}

// --- hostWithoutDefaultPort tests ---

func TestHostWithoutDefaultPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		scheme string
		host   string
		want   string
	}{
		{name: "https strips :443", scheme: "https", host: "example.com:443", want: "example.com"},
		{name: "https keeps :8443", scheme: "https", host: "example.com:8443", want: "example.com:8443"},
		{name: "https keeps no port", scheme: "https", host: "example.com", want: "example.com"},
		{name: "http strips :80", scheme: "http", host: "example.com:80", want: "example.com"},
		{name: "http keeps :8080", scheme: "http", host: "example.com:8080", want: "example.com:8080"},
		{name: "http keeps no port", scheme: "http", host: "example.com", want: "example.com"},
		{name: "https does not strip :80", scheme: "https", host: "example.com:80", want: "example.com:80"},
		{name: "http does not strip :443", scheme: "http", host: "example.com:443", want: "example.com:443"},
		{name: "scheme case-insensitive HTTPS", scheme: "HTTPS", host: "example.com:443", want: "example.com"},
		{name: "scheme case-insensitive HTTP", scheme: "HTTP", host: "example.com:80", want: "example.com"},
		{name: "unknown scheme leaves host unchanged", scheme: "ftp", host: "example.com:21", want: "example.com:21"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := hostWithoutDefaultPort(tt.scheme, tt.host)
			if got != tt.want {
				t.Fatalf("hostWithoutDefaultPort(%q, %q) = %q, want %q", tt.scheme, tt.host, got, tt.want)
			}
		})
	}
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
		closeResponseBody(nil)
	})

	t.Run("nil body", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{Body: nil}
		// Should not panic
		closeResponseBody(resp)
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
		closeResponseBody(resp)

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
