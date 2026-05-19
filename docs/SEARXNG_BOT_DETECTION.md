# SearXNG Bot Detection & Limiter

This document describes how SearXNG's `limiter` module detects and blocks non-browser HTTP clients, and how the MCP server handles these protections.

## Limiter Filters

SearXNG's `limiter` is enabled when `server.limiter: true` or `public_instance: true`. It validates incoming HTTP headers and returns **429 Too Many Requests** on failure.

| Filter | Condition |
|--------|-----------|
| `http_user_agent` | UA must not match bot regex (curl, wget, Go-http-client, Python, etc.) |
| `http_accept` | Must contain `text/html` |
| `http_accept_language` | Must be non-empty |
| `http_accept_encoding` | Must contain `gzip` or `deflate` |
| `http_sec_fetch` | `Sec-Fetch-Mode` must be `navigate` or `cors` (HTTPS only) |
| `ip_limit` | `format=json` in URL query triggers 4 requests/hour limit |

## Link Token Protection

`link_token` is forced on for `public_instance` and requires a browser CSS challenge:

- Non-browser clients accumulate in a suspicious IP counter
- After **3 requests within 30 days**, the server starts returning **302 redirects**
- **This cannot be bypassed with headers alone** — it requires JavaScript execution

## MCP Server Implementation

Our HTTP headers are set via `setBrowserHeaders()` in `internal/searxng/searcher.go`. Both POST and GET fallback share the same function to emulate browser-like headers and pass the limiter filters.
