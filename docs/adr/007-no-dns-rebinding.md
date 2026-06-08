# ADR-007: No DNS Rebinding Protection in SSRF Defense

**Status:** Accepted (redirect policy superseded by ADR-008)  
**Date:** 2026-04-28

> **Note:** The redirect SSRF defense described in the Context and Rationale
> below was further refined by
> [ADR-008: Same-Hostname-Only Redirect Policy](008-same-hostname-redirect.md),
> which replaced the `isPrivateHost()`-based redirect check with a
> hostname-and-scheme comparison. `isPrivateHost()` is no longer used for
> redirect decisions; it is retained for the HTTP warning described in
> ADR-003. The DNS-rebinding decision itself is unchanged and remains in
> force.

## Context

The redirect SSRF defense in `newHTTPClient` (`internal/searxng/client.go`)
controls which redirects are followed at the HTTP transport layer.
One potential attack vector is DNS rebinding: a hostname initially resolves
to a public IP, then after the host check passes, rebinds to a private IP
during the actual TCP dial.

## Decision

**We will NOT implement DNS rebinding protection** (i.e., a custom `DialContext` that re-checks the resolved IP before connecting).

## Rationale

1. **Threat model mismatch:** The primary threat is a compromised or misconfigured SearXNG instance redirecting to internal hosts. DNS rebinding requires an attacker to control both the DNS and the redirect target simultaneously — a significantly higher bar.
2. **Over-engineering:** Adding a `DialContext` IP check introduces complexity (custom dialer, dual resolution, TOCTOU edge cases) for a scenario that is both unlikely and already mitigated at the application layer (SearXNG runs on trusted infrastructure).
3. **Existing mitigations suffice:** The redirect policy (see ADR-008 for the
   current implementation) and the `isPrivateHost()` check that drives the
   HTTP non-private-host warning (ADR-003) cover the realistic attack surface
   without adding custom DNS resolution logic.

## Consequences

- Redirect defense remains host-based only.
- If a future deployment model exposes the MCP server to untrusted SearXNG instances with attacker-controlled DNS, this decision should be revisited.
