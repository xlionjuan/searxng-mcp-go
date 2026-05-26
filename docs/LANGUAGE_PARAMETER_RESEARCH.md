# SearXNG Language Parameter Research

## Current Behavior

The implementation uses the `language` parameter end-to-end.

- MCP schema exposes `language`
- `SearchArgs.Language` maps to `language`
- Search execution sends `language` to SearXNG when the field is non-empty
- `ValidateSearchArgs()` rejects invalid language codes

## Semantics

- Empty `language` means auto mode: the parameter is omitted and SearXNG chooses its default
- Valid values include codes like `en`, `zh-tw`, and `ja`
- Invalid values are rejected with a validation error
- There is no fallback that rewrites invalid input to English

## Historical Note

Earlier versions of this project and this document discussed a `lang` parameter and English fallback behavior. That no longer reflects the codebase.

## Reference Points

- `internal/searxng/searcher.go`
- `internal/searxng/validation.go`
- `mcp.go`

> **Note:** The curl examples below use both the `lang` and `language` parameters for comparison testing and are for research purposes. The actual code only sends the `language` parameter and does not use `lang`.

### Query: python programming

```bash
curl -s "http://localhost:8888/search?q=python+programming&format=json&lang=en" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=python+programming&format=json&language=en" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=python+programming&format=json&lang=ja" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=python+programming&format=json&language=ja" | jq '[.results[] | {engine, title}]'
```

### Query: artificial intelligence

```bash
curl -s "http://localhost:8888/search?q=artificial+intelligence&format=json&lang=en" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=artificial+intelligence&format=json&lang=de" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=artificial+intelligence&format=json&lang=fr" | jq '[.results[] | {engine, title}]'
curl -s "http://localhost:8888/search?q=artificial+intelligence&format=json" | jq '[.results[] | {engine, title}]'
```

---

## Reference Points

- `internal/searxng/types.go` — `SearchArgs` struct
- `internal/searxng/searcher.go` — search request construction and execution
- `internal/searxng/validation.go` — `ValidateSearchArgs()` language validation
- `mcp.go` — MCP tool schema definition
