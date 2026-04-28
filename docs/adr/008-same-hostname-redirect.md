# ADR-008: Same-Hostname-Only Redirect Policy

**Status:** Accepted  
**Date:** 2026-04-28

## Context

The HTTP client's `CheckRedirect` currently uses `isPrivateHost()` to block redirects to private/internal IPs. However, the function has known bypass vectors (IPv6 brackets, trailing dots, DNS resolution edge cases), and the entire approach is fragile.

A simpler alternative was proposed: **only follow redirects that stay on the same hostname.**

## Decision

**Redirects are only allowed within the same hostname.** If a SearXNG instance at `search.example.com` redirects to `search.example.com/some-path`, it is followed. If it redirects to any other host (including `127.0.0.1`, `192.168.x.x`, `localhost`, or any other domain), the redirect is rejected.

## Rationale

1. **Eliminates the SSRF attack surface entirely** — no private IP detection, no DNS resolution, no normalization edge cases.
2. **Removes `isPrivateHost()` and all its complexity** — no need to maintain a blocklist of dangerous IP ranges.
3. **SearXNG instances do not need cross-host redirects** — if an instance redirects to a different host, it is either misconfigured or malicious; neither case should be followed.
4. **Trivially testable** — the security test becomes: "redirect to different host → rejected."

## Consequences

- `CheckRedirect` is replaced with a simple hostname comparison.
- `isPrivateHost()` and related normalization logic are removed.
- The `getDefaultHTTPClient` TLS tests remain unchanged.
- If a legitimate SearXNG deployment requires cross-host redirects (unlikely), this decision should be revisited.
