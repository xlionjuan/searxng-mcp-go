# ADR-008: Same-Hostname-Only Redirect Policy

**Status:** Accepted  
**Date:** 2026-04-28

## Context

The HTTP client's `CheckRedirect` currently uses `isPrivateHost()` to block redirects to private/internal IPs. However, the function has known bypass vectors (IPv6 brackets, trailing dots, DNS resolution edge cases), and the entire approach is fragile.

A simpler alternative was proposed: **only follow redirects that stay on the same hostname.**

A subsequent security review (issue #97) observed that the original implementation also allowed a same-host scheme downgrade from `https://` to `http://`. That preserved compatibility with mixed reverse-proxy deployments, but it weakened the expectation that configuring an HTTPS SearXNG URL keeps subsequent request and response traffic on TLS. The review recommended deciding whether same-host redirects must preserve scheme.

## Decision

**Redirects are only allowed within the same hostname, must preserve or upgrade the original scheme, and must preserve the search method.** If a SearXNG instance at `https://search.example.com/search` redirects to `https://search.example.com/some-path`, it is followed only if the method remains unchanged — only 307 and 308 preserve POST semantics. If it redirects to any other host (including `127.0.0.1`, `192.168.x.x`, `localhost`, or any other domain), the redirect is rejected. If a same-host redirect changes the scheme from `https://` to `http://`, the redirect is also rejected. A same-host upgrade from `http://` to `https://` is allowed because it strengthens, rather than weakens, transport security.

Method-changing redirects (301, 302, 303) are rejected even for same-host targets because Go's `http.Client` rewrites the method from POST to GET and drops the request body before calling `CheckRedirect`. Following such a redirect would send a bodyless GET request that silently discards `q`, `format=json`, and every search option — violating the POST-default contract established in ADR-009.

## Rationale

1. **Eliminates the SSRF attack surface entirely** — no private IP detection, no DNS resolution, no normalization edge cases.
2. **No longer uses `isPrivateHost()` for redirect decisions** — redirect safety is determined by hostname comparison rather than private IP blocklists.
3. **SearXNG instances do not need cross-host redirects** — if an instance redirects to a different host, it is either misconfigured or malicious; neither case should be followed.
4. **Trivially testable** — the security test becomes: "redirect to different host → rejected" and "https → http on the same host → rejected".
5. **Preserves the operator's TLS choice** — if an HTTPS SearXNG URL is configured, the client will not silently fall back to cleartext for the redirect target or the eventual response, so query traffic stays on TLS end-to-end.

## Consequences

- `CheckRedirect` is replaced with a hostname-and-scheme comparison.
- `isPrivateHost()` is no longer used for redirect decisions, but is retained for the HTTP warning that fires when a non-private SearXNG URL uses `http://` instead of `https://`.
- The `getDefaultHTTPClient` TLS tests remain unchanged.
- Mixed-environment SearXNG deployments that intentionally terminate TLS at the proxy and then redirect to a cleartext backend on the same host will no longer be reached via that downgrade. Operators in that situation should either keep the redirect on `https://` or point the SearXNG base URL at the cleartext endpoint directly (and rely on the existing `http://` non-private-host warning for visibility).
- If a legitimate SearXNG deployment requires cross-host redirects (unlikely), or an `https` → `http` downgrade is genuinely required, this decision should be revisited.
- Same-host method-changing redirects (301, 302, 303) from POST are now rejected even when same-host and same-scheme. SearXNG instances that issue these for canonicalisation must be reconfigured to use 307 or 308 instead to preserve POST semantics.
