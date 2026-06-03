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
| `http_sec_fetch` | HTTPS-only and gated by `is_browser_supported(User-Agent)`. Only `Sec-Fetch-Mode` is actually enforced today — it must be `navigate` or `cors`. `Sec-Fetch-Site` and `Sec-Fetch-Dest` are written as if they filter (must be `same-origin`/`same-site`/`none` and `document`/`empty` respectively), but the vendored source builds the redirect response and forgets to `return` it, so those branches fall through to `return None` and never block the request. See the dead-branch note below. |
| `ip_limit` | `format=json` in URL query triggers 4 requests/hour limit |

Source for each filter lives under `searxng-server-test/searxng/searx/botdetection/` (vendored submodule). The limiter configuration file path is `/etc/searxng/limiter.toml` (see `searxng-server-test/searxng/searx/limiter.py`).

### `http_sec_fetch` dead-branch caveat

`searxng-server-test/searxng/searx/botdetection/http_sec_fetch.py` only `return`s the 302 redirect on the `Sec-Fetch-Mode` branch (line 95). The `Sec-Fetch-Site` (line 100) and `Sec-Fetch-Dest` (line 105) branches build the same redirect but never `return` it, so the function falls through to `return None` and the request is allowed. In other words, in the vendored source only `Sec-Fetch-Mode` is actually enforced for browser-like User-Agents on HTTPS. We still send `Sec-Fetch-Site: none` and `Sec-Fetch-Dest: document` so that:

- the moment the vendored bug is fixed (or the submodule is updated), we will satisfy the check without further code changes, and
- downstream proxies or WAFs that read these headers see a coherent Chrome fingerprint.

If the submodule is bumped and a changelog or commit message claims `http_sec_fetch` now rejects on `Sec-Fetch-Site`/`Sec-Fetch-Dest`, the doc above should be tightened to match the live enforcement and the `sec_fetch` rows in the header table should drop the "dead code" qualifier.

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
| `Sec-Fetch-Mode` | `navigate` | Satisfies `http_sec_fetch` (the only Sec-Fetch header the vendored source actually enforces) |
| `Sec-Fetch-Dest` | `document` | Would satisfy `http_sec_fetch` if the vendored source returned the redirect; today the branch is dead code, so this is sent defensively against future fixes |
| `Sec-Fetch-Site` | `none` | Would satisfy `http_sec_fetch` if the vendored source returned the redirect; today the branch is dead code, so this is sent defensively against future fixes |
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
