# ADR-003: HTTP Warning for Non-Private Hosts

## Status
Accepted

## Context
When connecting to a SearXNG instance over HTTP, search queries are transmitted
in clear text without encryption. This is a security concern for public networks
where queries could be intercepted by network observers.

## Decision
Warn (but do not block) when using HTTP for non-private hosts on every invocation.
This warning cannot be disabled - there is no opt-out mechanism.

The following hosts are considered "private" and will NOT trigger a warning:

- Hostname: localhost (exact match) and *.localhost (suffix match)
- TLD suffixes: .lan, .internal, .local, .home
- IPv4 private ranges: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
- IPv4 loopback: 127.0.0.0/8
- IPv6 private: fc00::/7, fe80::/10
- IPv6 loopback: ::1

## Warning Message
When HTTP is used for a non-private host:
"Using HTTP for non-private host. Search queries may be transmitted in clear text.
Search results could be intercepted and modified by a MITM attacker."

## Consequences

### Positive
- Users are informed of the security risk without being blocked
- Works out-of-the-box for internal/private deployments
- No configuration required

### Negative
- Users on private networks with HTTP will see unnecessary warnings
- Warning fatigue may cause users to ignore real security issues

## Alternatives Considered

### Block HTTP entirely
Rejected: Would break valid use cases for internal infrastructure testing.

### Allow opt-out via environment variable
Rejected: This is intentionally a no-opt-out decision. Organizations that need
HTTP-only setups should use private network detection or accept the warning.
