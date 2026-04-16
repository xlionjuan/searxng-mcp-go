# ADR-004: TLS Verification Bypass

## Status
Accepted

## Context
Internal infrastructure (private networks, development environments) often uses
self-signed or internally-issued TLS certificates. Requiring valid public CA
certificates for all connections would break legitimate internal use cases.

## Decision
Implement explicit TLS bypass via environment variable:

### Explicit Bypass (Environment Variable)
- Variable: INSECURE_SKIP_VERIFY=true
- Behavior: Always bypass TLS verification, always warn
- Warning: "TLS certificate verification is disabled - connections are
  susceptible to man-in-the-middle attacks and data may be intercepted
  or modified."

## Implementation Notes
The code only implements explicit bypass via the INSECURE_SKIP_VERIFY environment
variable. There is NO implicit bypass for private HTTPS hosts - private hosts
still require valid certificates unless INSECURE_SKIP_VERIFY is explicitly set.

## Consequences

### Positive
- Explicit bypass for development/testing with strong warning
- Clear opt-in mechanism for internal infrastructure

### Negative
- Users with self-signed certs on private hosts must explicitly set the env var
- No automatic handling of private networks

## Alternatives Considered

### Implicit private network bypass
Rejected: The code does not implement implicit bypass for private hosts.
Adding this would require careful consideration of the security implications.

### Require valid certificates everywhere
Rejected: Would break internal infrastructure use cases with self-signed certs.