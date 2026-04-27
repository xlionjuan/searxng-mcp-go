# SearXNG Language Parameter Research

## Current Behavior

The implementation uses the `language` parameter end-to-end.

- MCP schema exposes `language`
- `SearchArgs.Language` maps to `language`
- `performSearch()` sends `language` to SearXNG when the field is non-empty
- `ValidateSearchArgs()` rejects invalid language codes

## Semantics

- Empty `language` means auto mode: the parameter is omitted and SearXNG chooses its default
- Valid values include codes like `en`, `zh-tw`, and `ja`
- Invalid values are rejected with a validation error
- There is no fallback that rewrites invalid input to English

## Historical Note

Earlier versions of this project and this document discussed a `lang` parameter and English fallback behavior. That no longer reflects the codebase.

## Reference Points

- `search.go`
- `validation.go`
- `main.go`

> **注意：** 以下 curl 範例同時使用 `lang` 與 `language` 參數進行對比測試，屬於研究用途。實際程式碼（`search.go` / `performSearch()`）僅發送 `language` 參數，不使用 `lang`。

### Query: python programming

```bash
curl -s "https://search-4.xlion.dev/search?q=python+programming&format=json&lang=en" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=python+programming&format=json&language=en" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=python+programming&format=json&lang=ja" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=python+programming&format=json&language=ja" | jq '[.results[] | {engine, title}]'
```

### Query: artificial intelligence

```bash
curl -s "https://search-4.xlion.dev/search?q=artificial+intelligence&format=json&lang=en" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=artificial+intelligence&format=json&lang=de" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=artificial+intelligence&format=json&lang=fr" | jq '[.results[] | {engine, title}]'
curl -s "https://search-4.xlion.dev/search?q=artificial+intelligence&format=json" | jq '[.results[] | {engine, title}]'
```

---

## Reference Points

- `search.go` — `SearchArgs` struct, `performSearch()` API call construction
- `validation.go` — `ValidateSearchArgs()` language validation
- `main.go` — MCP tool schema definition
