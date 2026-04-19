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
| `language`   | string  | No       | auto    | Language code for results (e.g., en, zh-tw, ja). Empty = SearXNG decides |
| `safesearch` | integer | No       | 0       | SafeSearch filtering level:                     |
|              |         |          |         | - `0` = Off (show all results)                   |
|              |         |          |         | - `1` = Moderate (filter some adult content)    |
|              |         |          |         | - `2` = Strict (filter adult content)           |
| `time_range` | string  | No       | -       | Restrict results to a time period:              |
|              |         |          |         | - `day` = Last 24 hours                         |
|              |         |          |         | - `month` = Last 30 days                        |
|              |         |          |         | - `year` = Last 365 days                       |
| `categories` | string  | No       | -       | Comma-separated list of categories to search    |
|              |         |          |         | (e.g., general, news, music)                   |
| `engines`    | string  | No       | -       | Comma-separated list of search engines to use  |
|              |         |          |         | (e.g., google, bing, duckduckgo)               |
| `pageno`     | integer, null | No       | 1       | Page number for pagination (null = backend default/page 1) |

### Response Format

The tool returns a JSON text response containing the full `SearchResponse` object. This enables programmatic parsing by MCP clients.

**Response Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `query` | string | The original search query |
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

**Note:** The `number_of_results` field may return 0 from SearXNG even when results are present. The server normalizes this by replacing 0 with `len(results)` when results exist.

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

```json
{
  "query": "golang tutorial",
  "answers": [
    {"answer": "203.0.113.42", "engine": "ip_lookup"}
  ],
  "number_of_results": 2,
  "results": [
    {
      "title": "Go Language Tutorial",
      "url": "https://example.com/golang-tutorial",
      "content": "Learn Go programming from scratch with this comprehensive tutorial...",
      "engine": "google",
      "publishedDate": "2024-01-15"
    },
    {
      "title": "Building Web Applications with Go",
      "url": "https://example.com/go-web-dev",
      "content": "A practical guide to building modern web applications using Go...",
      "engine": "duckduckgo"
    }
  ],
  "suggestions": [
    "Best Golang tutorial",
    "Golang tutorial interactive"
  ]
}
```

### Error Responses

The server returns the following error types:

| Error Type | Description |
|------------|-------------|
| `ValidationError` | User-provided parameter validation failure (missing/invalid fields) |
| `HTMLResponseError` | `searxng returned html instead of json - json output may not be enabled on the server` |
| `SearXNGError` | Network failures, HTTP errors, or API errors from SearXNG |

Actual error message formats from the server:

| Error Condition               | Response Format                                                |
|-------------------------------|----------------------------------------------------------------|
| Missing `query` parameter     | `validation error on "query": search query cannot be only whitespace` |
| Query too long (>500 chars)   | `validation error on "query": must be 500 characters or less` |
| Invalid `safesearch` value    | `validation error on "safesearch": must be 0 off, 1 moderate, or 2 strict` |
| Invalid `pageno` value        | `validation error on "pageno": must be >= 1` |
| Invalid `time_range` value    | `validation error on "time_range": must be one of day, month or year` |
| Network failure               | `searxng error (status 0): context deadline exceeded` (or similar) |
| SearXNG HTTP error           | `searxng error (status 500): internal server error: the search engine encountered an internal error` |
| HTML response (JSON disabled)| `searxng returned html instead of json - json output may not be enabled on the server` |
| Invalid JSON from SearXNG     | `searxng error (status 200): failed to parse JSON response: <underlying error>` |

### Implementation Details

- **Transport**: Stdio (stdin/stdout)
- **Protocol**: MCP (Model Context Protocol)
- **SearXNG Format**: JSON (`format=json`)
- **Timeout**: 30 seconds (configurable in source code; not adjustable via MCP client parameters)
