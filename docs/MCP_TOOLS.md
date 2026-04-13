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
| `language`   | string  | No       | en      | Language code for results (e.g., en, zh-tw, ja) |
| `safesearch` | integer | No       | 0       | SafeSearch filtering level:                     |
|              |         |          |         | - `0` = Off (show all results)                   |
|              |         |          |         | - `1` = Moderate (filter some adult content)    |
|              |         |          |         | - `2` = Strict (filter adult content)           |
| `time_range` | string  | No       | -       | Restrict results to a time period:              |
|              |         |          |         | - `day` = Last 24 hours                         |
|              |         |          |         | - `month` = Last 30 days                        |
|              |         |          |         | - `year` = Last 365 days                       |
| `categories` | string  | No       | -       | Comma-separated list of categories to search    |
|              |         |          |         | (e.g., general, news, music, videos, it)       |
| `engines`    | string  | No       | -       | Comma-separated list of search engines to use  |
|              |         |          |         | (e.g., google, bing, duckduckgo)               |
| `pageno`     | integer | No       | 1       | Page number for pagination                       |

### Response Format

The tool returns a text response containing:

- Total number of results found
- For each result:
  - Sequential number
  - Title (clickable link text)
  - URL
  - Content/summary snippet
  - Source search engine

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
   Engine: google

2. Building Web Applications with Go
   URL: https://example.com/go-web-dev
   Summary: A practical guide to building modern web applications using Go...
   Engine: duckduckgo
```

### Error Responses

| Error Condition               | Response                                                |
|-------------------------------|--------------------------------------------------------|
| Missing `query` parameter     | "Error: query parameter is required"                   |
| Invalid `time_range` value    | "Error: time_range must be one of: day, month, year"   |
| Network failure               | "Search error: failed to execute search request: ..."  |
| SearXNG API error            | "Search error: SearXNG returned non-OK status: 500"   |
| Invalid JSON from SearXNG     | "Search error: failed to parse JSON response: ..."     |

### Implementation Details

- **Transport**: Stdio (stdin/stdout)
- **Protocol**: MCP (Model Context Protocol)
- **SearXNG Format**: JSON (`format=json`)
- **Timeout**: 30 seconds (configurable in source)
