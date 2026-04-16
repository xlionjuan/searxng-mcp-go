# ADR-004: TLS Verification Bypass

## Status
Accepted

## Context
Internal infrastructure (private networks, development environments) often uses
self-signed or internally-issued TLS certificates. Requiring valid public CA
certificates for all connections would break legitimate internal use cases.

## Decision
Implement a two-tier TLS bypass mechanism:

### Tier 1: Explicit Bypass (Environment Variable)
- Variable: INSECURE_SKIP_VERIFY=true
- Behavior: Always bypass TLS verification, always warn
- Warning: "TLS certificate verification is disabled - connections are
  susceptible to man-in-the-middle attacks and data may be intercepted
  or modified."

### Tier 2: Implicit Private Network Bypass
- Trigger: HTTPS + destination is a private host (same definition as ADR-003)
- Behavior: Automatically bypass TLS verification, show weak warning
- Warning: "TLS certificate verification skipped for private network host -
  this is expected for internal infrastructure but results may be intercepted
  by local attackers."

## Private Host Definition
Same as ADR-003:
- TLD suffixes: .lan, .internal, .local, .home
- IPv4: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, 127.0.0.0/8, 169.254.0.0/16
- IPv6: fc00::/7, fe80::/10, ::1

## Consequences

### Positive
- Works seamlessly with internal infrastructure using self-signed certs
- Explicit bypass for development/testing with strong warning
- Implicit bypass for private networks reduces noise

### Negative
- Dual bypass mechanism may be confusing
- Implicit bypass assumes private network is trustworthy

## Alternatives Considered

### Single global bypass via environment variable
Rejected: Would not automatically handle private networks and would require
all users to explicitly set the variable.

### Require valid certificates everywhere
Rejected: Would break internal infrastructure use cases with self-signed certs.