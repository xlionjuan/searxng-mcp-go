# MCP Tools Reference

This document describes the MCP tools exposed by the SearXNG MCP server.

## search

Search the web using a SearXNG meta-search engine instance.

### Tool Description

The `search` tool proxies web search requests to a SearXNG instance, which aggregates results from multiple search engines while maintaining user privacy. MCP mode always returns structured JSON for programmatic parsing.

### Parameters

| Parameter    | Type    | Required | Default | Description                                      |
|--------------|---------|----------|---------|--------------------------------------------------|
| `query`      | string  | Yes      | -       | The search query string                          |
| `language`   | string  | No       |         | Language code for results (e.g., en, zh-tw, ja). Empty or `"auto"` = SearXNG decides |
| `safesearch` | integer | No       | 0       | SafeSearch filtering level:                     |
|              |         |          |         | - `0` = Off (show all results)                   |
|              |         |          |         | - `1` = Moderate (filter some adult content)    |
|              |         |          |         | - `2` = Strict (filter adult content)           |
| `time_range` | string  | No       | -       | Restrict results to a time period:              |
|              |         |          |         | - `day` = Last 24 hours                         |
|              |         |          |         | - `month` = Last 30 days                        |
|              |         |          |         | - `year` = Last 365 days                       |
| `categories` | string  | No       | -       | Comma-separated list of SearXNG categories.    |
|              |         |          |         | "general" covers most queries; other values    |
|              |         |          |         | (it, science, news, map, music, files,         |
|              |         |          |         | social media) also work but are rarely needed. |
|              |         |          |         | Max 4096 bytes for the full string.            |
| `engines`    | string  | No       | -       | Comma-separated list of SearXNG engine names.  |
|              |         |          |         | Common engines: google, bing, duckduckgo.      |
|              |         |          |         | Max 4096 bytes for the full string.            |
| `pageno`     | integer, null | No       | 1       | Page number for pagination (SearXNG defaults to page 1 when omitted) |
| `limit`      | integer | No       | 10      | Maximum number of results returned (1-20)      |

### Response Format

The tool returns a JSON text response containing the full `SearchResponse` object. This enables programmatic parsing by MCP clients.

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | The original search query |
| `warning` | string | External-content advisory noting that search results come from untrusted sources and should be verified before use (always present on successful responses) |
| `answers` | array | Direct answers (omitted when empty) |
| `number_of_results` | integer | Total count of results; if SearXNG returns 0 while results exist, normalized to `len(results)` |
| `infoboxes` | array | Knowledge panels with content, attributes, URLs (omitted when empty) |
| `results` | array | Array of search result objects (always present, `[]` when empty) |
| `suggestions` | array | Related search suggestions (always present, `[]` when empty) |
| `unresponsive_engines` | array | Debug-only array of `[engine_name, error_message]` pairs (omitted unless debug mode is enabled) |

**Result Object Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Result title |
| `url` | string | URL of the result |
| `content` | string | Content snippet from the page |
| `engine` | string | Source search engine |
| `publishedDate` | string | Publication date provided by SearXNG (omitted when the backend does not include it; no normalization is applied) |

**Note:** The `number_of_results` field may return 0 from SearXNG even when results are present. The server normalizes this by replacing 0 with `len(results)` when results exist. If `pageno` is omitted, the server does not send the parameter and SearXNG uses its page 1 default.

**Note:** The `warning` field is a static advisory emitted on every successful response to remind AI agents and JSON consumers that search results originate from external, untrusted sources. It is not a per-result provenance marker; the canonical wire-format ordering and JSON-mode behavior are documented in [docs/OUTPUT_FORMAT.md](OUTPUT_FORMAT.md).

### Example Usage

#### Basic search

```json
{
  "query": "golang tutorial"
}
```

#### Search with language filter

```json
{
  "query": "machine learning",
  "language": "en"
}
```

#### Safe search enabled

```json
{
  "query": "programming",
  "safesearch": 1
}
```

#### Time-restricted search (recent news)

```json
{
  "query": "AI breakthroughs",
  "time_range": "day"
}
```

#### Search with specific categories

```json
{
  "query": "technology news",
  "categories": "news"
}
```

#### Search with specific engines

```json
{
  "query": "web development",
  "engines": "google,bing"
}
```

#### Pagination

```json
{
  "query": "recipes",
  "pageno": 2
}
```

#### Combined parameters

```json
{
  "query": "climate change research",
  "language": "en",
  "safesearch": 2,
  "time_range": "month"
}
```

### Example Response

Legacy instant-answer example (query `ip`):

```json
{
  "query": "ip",
  "warning": "Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.",
  "answers": [
    {"answer": "203.0.113.42", "engine": "ip_lookup"}
  ],
  "number_of_results": 0,
  "results": [],
  "suggestions": []
}
```

Typed-answer example (query `translate hello to french`):

```json
{
  "query": "translate hello to french",
  "warning": "Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.",
  "answers": [
    {
      "answer": "bonjour",
      "engine": "lingva",
      "translations": [
        {
          "text": "bonjour",
          "transliteration": "bɔ̃ʒuʁ",
          "examples": ["Bonjour le monde", "Bonjour, comment ça va ?"],
          "definitions": ["a formal greeting"],
          "synonyms": ["salut", "coucou"]
        }
      ]
    }
  ],
  "number_of_results": 0,
  "results": [],
  "suggestions": []
}
```

Typed-weather-answer example (query `weather Berlin`):

```json
{
  "query": "weather Berlin",
  "warning": "Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.",
  "answers": [
    {
      "engine": "open-meteo",
      "current": {
        "location": {"name": "Berlin", "latitude": 52.52, "longitude": 13.41, "timezone": "Europe/Berlin"},
        "temperature": {"val": 18.4, "unit": "°C"},
        "condition": "Partly cloudy",
        "humidity": {"val": 62, "unit": "%"}
      },
      "forecasts": [],
      "service": "open-meteo.com"
    }
  ],
  "number_of_results": 0,
  "results": [],
  "suggestions": []
}
```

Typed answers (translations, weather) populate `translations`, `current`,
`forecasts`, and `service` instead of the legacy `answer` string. See
`internal/searxng/types.go` (`Answer` struct) for the full set of typed
fields and their `omitempty` behavior.

### Error Responses

The server returns the following error types:

| Error Type | Description |
|------------|-------------|
| `ValidationError` | User-provided parameter validation failure (missing/invalid fields) |
| `HTMLResponseError` | `searxng returned html instead of json - json output may not be enabled on the server` |
| `SearXNGError` | Network failures, HTTP errors, or API errors from SearXNG |

Validation errors can come from two places. MCP SDK schema validation runs before
the search handler for JSON Schema constraints; handler validation runs after
argument decoding for project-specific checks.

The SDK version is pinned in `go.mod` (`github.com/modelcontextprotocol/go-sdk`)
and the format is verified by `TestMCPErrors_InvalidInputs` against that pinned
version. Treat the response-format strings below as the contract for that
pinned version; a SDK upgrade must update both the table and the guarding test
in the same change.

SDK schema validation error examples:

| Error Condition | Response Format (as received by MCP client) |
|-----------------|---------------------------------------------|
| Invalid `safesearch` value | `validating "arguments": ... "safesearch" ...` |
| Invalid `pageno` value | `validating "arguments": ... "pageno" ...` |
| Invalid `time_range` value | `validating "arguments": ... "time_range" ...` |
| Invalid `limit` value | `validating "arguments": ... "limit" ...` |

Handler validation error examples:

| Error Condition               | Response Format (as received by MCP client)                              |
|-------------------------------|--------------------------------------------------------------------------|
| Missing or whitespace `query` | `Validation error: validation error on "query": search query cannot be only whitespace` |
| Query too long (>500 runes)   | `Validation error: validation error on "query": must be 500 runes or less` |
| Query control characters      | `Validation error: validation error on "query": contains invalid control characters` |
| Invalid `categories` value    | `Validation error: validation error on "categories": contains invalid category` |
| Invalid `engines` value       | `Validation error: validation error on "engines": contains invalid engine` |
| Invalid `language` value      | `Validation error: validation error on "language": must be a valid language code (e.g., en, zh-tw, ja, en-US)` |
| Language too long (>35 runes) | `Validation error: validation error on "language": must be 35 runes or less` |

Search error examples — the response format depends on whether the error is a `SearXNGError` (includes status code, content type, and underlying cause) or another error type:

| Error Condition | Response Format (as received by MCP client) |
|-----------------|---------------------------------------------|
| Network failure (wrapped as `SearXNGError`) | `Search error: searxng error (status 0): <error description>` (full error logged server-side) |
| SearXNG HTTP error (4xx/5xx, wrapped as `SearXNGError`) | `Search error: searxng error (status <N>) - content-type <type>: <error description>` (full error logged server-side) |
| POST `/search` rejected with 405/501 | `Search error: searxng error (status 405/501) - content-type <type>: search method rejected` (full error logged server-side; fix the reverse proxy or opt in with `SEARXNG_ALLOW_GET_FALLBACK=1`) |
| Invalid JSON from SearXNG | `Search error: searxng error (status <N>) - content-type <type>: <error description>` (full error logged server-side) |
| HTML response (JSON disabled) | `Search error: request failed` (full error logged server-side) |
| Other unexpected errors | `Search error: request failed` (full error logged server-side) |
| Response marshal failure | `Search error: failed to format results` (full error logged server-side) |

### Implementation Details

- **Transport**: Stdio (stdin/stdout)
- **Protocol**: MCP (Model Context Protocol)
- **SearXNG Format**: JSON (`format=json`)
- **Timeout**: 8 seconds by default; set `SEARXNG_TIMEOUT` or, in CLI mode, `--timeout`
- **MaxRetries**: 5 retries after the initial search attempt by default; set `SEARXNG_MAX_RETRIES` or, in CLI mode, `--max-retries`
- **POST→GET fallback**: Disabled by default. If POST `/search` returns 405 or 501, the server returns an error so operators can fix the SearXNG or reverse-proxy configuration. Set `SEARXNG_ALLOW_GET_FALLBACK=1` to opt in; this sends search parameters in the URL and may expose queries in upstream logs. In CLI mode, `--allow-get-fallback` overrides the environment variable.
- **Initialize message size limit**: The first line of stdin (the MCP `initialize` JSON-RPC message) is capped at 1 MB; oversized input causes the server to exit instead of hanging
