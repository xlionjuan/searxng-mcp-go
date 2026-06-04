# ADR-009: Disable GET Fallback by Default

**Status:** Accepted  
**Date:** 2026-06-04

## Context

SearXNG search requests are sent with POST so query parameters stay in the
request body rather than the URL. The previous implementation retried POST
`/search` as GET when the server returned `405 Method Not Allowed` or
`501 Not Implemented`.

That fallback improved compatibility with broken deployments, but it also moved
the `q` parameter into the URL. URLs are commonly recorded by SearXNG access
logs, reverse proxies, CDNs, corporate proxies, and Go network error strings.
For an MCP server, search queries may contain sensitive user prompts, so silently
switching from POST to GET is a security-relevant behavior change.

Modern SearXNG supports POST `/search`; a 405 or 501 usually indicates a
misconfigured reverse proxy or deployment policy rather than a normal upstream
capability gap.

## Decision

GET fallback is disabled by default.

If POST `/search` returns 405 or 501, the server returns an actionable
`SearXNGError` telling the operator to fix POST handling or explicitly opt in
with:

```bash
SEARXNG_ALLOW_GET_FALLBACK=1
```

When the fallback is enabled, the server logs a warning during searcher
initialization and another warning each time the fallback is used. Errors from
the GET fallback redact the `q` URL parameter before wrapping the transport
error.

The setting is runtime configuration, not a build feature. MCP hosts and CLI
deployments can enable it through the environment without rebuilding the binary.

## Consequences

- The default path no longer copies search queries into URLs.
- Deployments with a proxy that rejects POST now fail loudly instead of silently
  downgrading to GET.
- Operators that need the compatibility path can still opt in per environment.
- Enabling the fallback is an explicit acceptance that search parameters may
  appear in upstream URLs and logs.
- Tests must cover both the default-off behavior and the opt-in fallback path.

## Alternatives Considered

### Keep fallback enabled and only redact errors

Rejected. Redacting Go error strings would address one leak path, but the query
would still be present in upstream request URLs and access logs.

### Remove fallback entirely

Rejected for now. Some deployments may still need a temporary compatibility
escape hatch while fixing reverse proxy POST handling.

### Use a build tag only

Rejected. This is an operational compatibility setting, and requiring a custom
build would make MCP client deployment unnecessarily difficult. A future
compile-out option can be considered separately for hardened distributions.

### Use `SEARXNG_DISABLE_GET_FALLBACK`

Rejected. The security-sensitive path should be a positive opt-in, not a
negative disable flag.
