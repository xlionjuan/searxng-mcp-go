# ADR-007: No DNS Rebinding Protection in SSRF Defense

**Status:** Accepted  
**Date:** 2026-04-28

## Context

The redirect SSRF defense in `newHTTPClient` (`internal/searxng/searcher.go`) blocks redirects to private/internal hosts using `isPrivateHost()`. One potential attack vector is DNS rebinding: a hostname initially resolves to a public IP, then after the host check passes, rebinds to a private IP during the actual TCP dial.

## Decision

**We will NOT implement DNS rebinding protection** (i.e., a custom `DialContext` that re-checks the resolved IP before connecting).

## Rationale

1. **Threat model mismatch:** The primary threat is a compromised or misconfigured SearXNG instance redirecting to internal hosts. DNS rebinding requires an attacker to control both the DNS and the redirect target simultaneously — a significantly higher bar.
2. **Over-engineering:** Adding a `DialContext` IP check introduces complexity (custom dialer, dual resolution, TOCTOU edge cases) for a scenario that is both unlikely and already mitigated at the application layer (SearXNG runs on trusted infrastructure).
3. **Existing mitigations suffice:** The host-level `isPrivateHost()` check and proper host normalization in the redirect path cover the realistic attack surface without adding custom DNS resolution logic.

## Consequences

- Redirect defense remains host-based only.
- If a future deployment model exposes the MCP server to untrusted SearXNG instances with attacker-controlled DNS, this decision should be revisited.
