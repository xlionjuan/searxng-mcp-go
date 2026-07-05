package searxng

import (
	"net"
	"strings"
)

const (
	ipv4Private10         = 10
	ipv4Loopback127       = 127
	ipv6UniqueLocalPrefix = 0xfc
	classBPrefix          = 192
	classBSecondOctet     = 168
	multicastFirstOctet   = 224
	ipv6MulticastPrefix   = 0xff
)

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
	// The host at this point has been decoded by url.Parse (called during
	// searcher construction), so percent-encoded %25 has already been
	// decoded to bare %. Any remaining % is a genuine zone ID separator.
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

//nolint:gocognit,gocyclo,cyclop // exhaustive CIDR ranges are clearer as explicit blocks than a data-driven loop
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
