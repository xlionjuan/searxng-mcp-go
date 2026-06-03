# ADR-003: HTTP Warning for Non-Private Hosts

## Status

Accepted

## Context

When connecting to a SearXNG instance over HTTP, search queries are transmitted
in clear text without encryption. This is a security concern for public networks
where queries could be intercepted by network observers.

The earlier version of this decision classified a host as "private" using a
hand-maintained TLD suffix list (`.lan`, `.internal`, `.local`, `.home`,
`.corp`, `.intranet`) on top of literal IP range checks. As of 2026 the IANA
root zone lists over 1,500 gTLDs and ccTLDs, and the private/internal TLD
landscape is similarly fragmented: `.home.arpa` is IETF-standardized by
RFC 7788 / RFC 8375, `.localhost` is reserved by RFC 6761, `.local` is the
mDNS namespace defined by RFC 6762 (link-local name resolution, not
"private network"), and the remaining suffixes on the old list are de-facto
conventions that vary by organization. A hardcoded suffix list is impossible
to keep complete, easy to bypass with an unlisted suffix, and cannot be
audited against any external standard.

## Decision

Warn (but do not block) when using HTTP for non-private hosts on every invocation.
This warning cannot be disabled - there is no opt-out mechanism.

`isPrivateHost()` returns `true` if and only if the host matches one of the
RFC-grounded criteria below. The check is purely syntactic — it does not
perform DNS resolution, does not consult any local suffix allowlist, and
returns `false` for any hostname that is not RFC 6761-special or a literal
address in one of the listed ranges.

### Hostname (RFC 6761 — Special-Use Domain Names)

| Form | Reference |
|------|-----------|
| `localhost` (exact match, ASCII lower-case) | RFC 6761 §6.3 |
| `*.localhost` (suffix match) | RFC 6761 §6.3 |

### IPv4 address ranges

| Range | Reference |
|-------|-----------|
| `0.0.0.0/8` (current network) | RFC 1122 §3.2.1.3, RFC 6890 §2.1 |
| `10.0.0.0/8` (private) | RFC 1918 §3 |
| `100.64.0.0/10` (CGNAT / shared address space) | RFC 6598 |
| `127.0.0.0/8` (loopback) | RFC 1122 §3.2.1.3, RFC 6890 §2.2 |
| `169.254.0.0/16` (link-local) | RFC 3927, RFC 6890 §2.4 |
| `172.16.0.0/12` (private) | RFC 1918 §3 |
| `192.0.0.0/24` (IETF protocol assignments) | RFC 6890 §2.5 |
| `192.0.2.0/24` (TEST-NET-1) | RFC 5737 |
| `192.88.99.0/24` (6to4 anycast, RFC 7526 deprecated) | RFC 7526 |
| `192.168.0.0/16` (private) | RFC 1918 §3 |
| `198.18.0.0/15` (benchmarking) | RFC 2544, RFC 6890 §2.6 |
| `198.51.100.0/24` (TEST-NET-2) | RFC 5737 |
| `203.0.113.0/24` (TEST-NET-3) | RFC 5737 |
| `224.0.0.0/4` (multicast) | RFC 5771, RFC 6890 §2.7 |
| `255.255.255.255/32` (limited broadcast) | RFC 8190, RFC 6890 §2.8 |

### IPv6 address ranges

| Range | Reference |
|-------|-----------|
| `::` (unspecified) | RFC 4291 §2.5.2 |
| `::1` (loopback) | RFC 4291 §2.5.3 |
| `fc00::/7` (unique local) | RFC 4193 |
| `fe80::/10` (link-local) | RFC 4291 §2.5.6 |
| `ff00::/8` (multicast) | RFC 4291 §2.7 |

### Explicitly NOT considered private

The following suffixes were on the earlier hand-maintained list and are
**removed** because none of them are reserved by an IETF Standards-Track
RFC for "private network" use. Hosts matching these suffixes will now
trigger the HTTP warning:

- `*.lan` — no RFC reservation
- `*.internal` — no RFC reservation
- `*.local` — RFC 6762 (mDNS link-local name resolution); the suffix is for
  local name resolution, not a "private network" indicator
- `*.home` — RFC 8375 §1 explicitly deprecates `.home` in favor of
  `.home.arpa`; not reserved for arbitrary private use
- `*.corp` — no RFC reservation
- `*.intranet` — no RFC reservation

The check is intentionally literal — there is no DNS resolution and no
suffix allowlist. Users who need a private-network exception for a custom
suffix must use a literal RFC 1918/4193 address, an `localhost` hostname,
or terminate TLS in front of SearXNG.

## Warning Message

When HTTP is used for a non-private host:
"Using HTTP for non-private host. Search queries may be transmitted in clear text.
Search results could be intercepted and modified by a MITM attacker."

## Consequences

### Positive

- The "private" predicate is fully auditable: every accepted hostname
  matches RFC 6761, and every accepted IP range is named in the
  Standards-Track RFCs above. There is no internal suffix list to drift
  out of date.
- Removing the speculative suffix list eliminates false negatives caused
  by unlisted TLDs (`.home.arpa`, organization-specific suffixes) and
  false positives caused by public TLDs that happen to match
  (`*.corp`).
- Users running a SearXNG instance on `*.lan` / `*.local` will see the
  HTTP warning and are nudged to either terminate TLS or bind the
  instance to a literal RFC 1918/4193 address.

### Negative

- Users on private networks with HTTP and a non-RFC 6761 hostname
  (e.g. `printer.local`) will see the warning. This is an intentional,
  accepted behavior change — see "Accepted trade-offs" below.
- Warning fatigue may cause users to ignore real security issues.
- The HTTP warning is not a security control on its own; the real
  defenses are redirect safety (ADR-008) and the user's choice of
  URL.

### Accepted trade-offs

- Hosts matching the removed suffixes (`.lan`, `.local`, `.internal`,
  `.home`, `.corp`, `.intranet`) are no longer classified as private.
  This is accepted because the previous classification was ungrounded
  and the user has agreed (in the issue thread) to surface the warning
  for these hosts in exchange for a contract that is auditable
  against published RFCs.

## Alternatives Considered

### Block HTTP entirely

Rejected: Would break valid use cases for internal infrastructure testing.

### Allow opt-out via environment variable

Rejected: This is intentionally a no-opt-out decision. Organizations that need
HTTP-only setups should use private network detection or accept the warning.

### Keep the speculative TLD list and add `.home.arpa`

Rejected: any non-RFC suffix list inherits the maintenance and bypass
problems described in Context. The RFC 6761-only contract is the smallest
auditable set.

### Resolve the hostname and check the resulting IP

Rejected: introduces DNS resolution, TOCTOU windows, and resolver
behavior variance across environments. ADR-007 also decides that
DNS-rebinding-class defenses are out of scope for this codebase.
