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
| `http_sec_fetch` | HTTPS-only and gated by `is_browser_supported(User-Agent)`: `Sec-Fetch-Mode` must be `navigate` or `cors`; `Sec-Fetch-Site` must be `same-origin`, `same-site`, or `none`; `Sec-Fetch-Dest` must be `document` or `empty` |
| `ip_limit` | `format=json` in URL query triggers 4 requests/hour limit |

Source for each filter lives under `searxng-server-test/searxng/searx/botdetection/` (vendored submodule). The limiter configuration file path is `/etc/searxng/limiter.toml` (see `searxng-server-test/searxng/searx/limiter.py`).

## Link Token Protection

`link_token` is forced on for `public_instance` and requires a browser CSS challenge:

- Non-browser clients accumulate in a suspicious IP counter
- After **3 requests within 30 days**, the server starts returning **302 redirects**
- **This cannot be bypassed with headers alone** — it requires JavaScript execution

Our local test server explicitly disables `link_token` (see `docs/agents/test-server.md`), which is why E2E tests against it succeed without browser-side execution.

## MCP Server Implementation

Our HTTP headers are set via `setBrowserHeaders()` in `internal/searxng/request.go`. Both POST and GET fallback share the same function to emulate browser-like headers and pass the limiter filters.

### Browser-mimicry headers we set

| Header | Value (constant) | Purpose |
|--------|------------------|---------|
| `User-Agent` | Chrome 147 on Linux | Satisfies `http_user_agent`; also flips `http_sec_fetch` into "supported browser" mode so the Sec-Fetch checks are enforced and our values are accepted |
| `Accept` | `text/html,...` | Satisfies `http_accept` (must contain `text/html`) |
| `Accept-Language` | `en-US,en;q=0.9` | Satisfies `http_accept_language` (must be non-empty) |
| `Sec-Fetch-Mode` | `navigate` | Satisfies `http_sec_fetch` |
| `Sec-Fetch-Dest` | `document` | Satisfies `http_sec_fetch` |
| `Sec-Fetch-Site` | `none` | Satisfies `http_sec_fetch` |
| `Sec-Fetch-User` | `?1` | Camouflage; not validated by any current limiter filter |
| `Sec-Ch-Ua` | Chrome 147 brand list | Camouflage; not validated by any current limiter filter |
| `Sec-Ch-Ua-Mobile` | `?0` | Camouflage; not validated by any current limiter filter |
| `Sec-Ch-Ua-Platform` | `"Linux"` | Camouflage; not validated by any current limiter filter |
| `Priority` | `u=0, i` | Camouflage; not validated by any current limiter filter |

The Sec-CH-UA family and `Priority` are not checked by any file under `searxng-server-test/searxng/searx/botdetection/`. They are sent only to look like a real Chrome request and reduce the chance that future filters or downstream proxies flag the request. Keep them as defensive padding unless there is a concrete reason to drop them.

### Accept-Encoding handling

`setBrowserHeaders()` does **not** set `Accept-Encoding`. Go's `net/http.Transport` automatically adds `Accept-Encoding: gzip` when the caller has not set the header and `DisableCompression` is `false`, which satisfies `http_accept_encoding` (accepts `gzip` or `deflate`).

This is a silent dependency on Go's transport default. Any future change that:

- replaces the default `http.Client` with one whose `Transport` has `DisableCompression: true`, or
- sets `Accept-Encoding` on the request to something other than `gzip` or `deflate`,

will silently re-introduce a limiter trip with no test coverage to catch it. Treat `Accept-Encoding` as part of the limiter contract when changing transport configuration.
