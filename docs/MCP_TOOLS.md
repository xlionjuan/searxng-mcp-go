# MCP Tools Reference

This document describes the MCP tools exposed by the SearXNG MCP server.

## search

Search the web using a SearXNG meta-search engine instance.

### Tool Description

The `search` tool proxies web search requests to a SearXNG instance, which aggregates results from multiple search engines while maintaining user privacy. The server formats results into a human-readable structure.

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

The tool returns a text response containing:

- Total number of results found
- For each result:
  - Sequential number
  - Title
  - URL (plain text)
  - Summary (content snippet from the page)
  - Date (if available, e.g., publication date)
  - Source search engine

Note: `dateSource` is a JSON-only metadata field indicating whether the date came from the SearXNG API ("api") or was inferred from page content ("inferred"). It is not displayed in text output.

**JSON Response Fields:**

When using the `--json` CLI flag, the response includes additional fields:

| Field | Type | Description |
|-------|------|-------------|
| `results` | array | Array of search result objects |
| `number_of_results` | integer | Total count of results (may be 0 even when results exist due to SearXNG API behavior) |
| `query` | string | The original search query |

**Result Object Fields:**

| Field | Type | Description |
|-------|------|-------------|
| `title` | string | Result title |
| `url` | string | URL of the result |
| `content` | string | Content snippet from the page |
| `engine` | string | Source search engine |
| `publishedDate` | string | Publication date if available (ISO 8601 format) |
| `dateSource` | string | Source of the date: "api" (from SearXNG), "inferred" (calculated from content), or "" (not available); only in JSON output |

**Note:** The `number_of_results` field may return 0 even when results are present in the `results` array. This is a known behavior of the SearXNG API, and the code handles this by using the actual array length when this occurs.

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

```
Found 5 results for 'golang tutorial':

1. Go Language Tutorial
   URL: https://example.com/golang-tutorial
   Summary: Learn Go programming from scratch with this comprehensive tutorial...
   Date: 2024-01-15
   Engine: google

2. Building Web Applications with Go
   URL: https://example.com/go-web-dev
   Summary: A practical guide to building modern web applications using Go...
   Engine: duckduckgo
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
