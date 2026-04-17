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

## Appendix: Files Reviewed

| File | Purpose | Key Lines |
|------|---------|-----------|
| `search.go` | SearchArgs struct, API call construction | 93-100 (struct), 147-148 (params) |
| `validation.go` | Language validation logic | Full file |
| `main.go` | MCP tool schema definition | 99-102 (searchInputSchema) |

---

---

## 7. Bugs Found and Fixes Applied

### Bug 1: Incorrect API Parameter Name

**Severity:** HIGH

**Description:**  
The implementation was sending `lang` as the API parameter name instead of `language`. This caused suboptimal search results because the SearXNG API treats `lang` and `language` differently - the `language` parameter has a significantly stronger effect on result ordering and content language filtering.

**Evidence:**  
Live testing showed that `language=ja` for query "python programming" returned nearly all Japanese-language results, while `lang=ja` returned results similar to the default (no language specified).

**Location:**  
`search.go` line ~148

**Original Code:**
```go
params.Set("lang", language)
```

**Fixed Code:**
```go
params.Set("language", language)
```

**Fix Applied:**  
Changed `params.Set("lang", language)` to `params.Set("language", language)` to use the correct parameter name as specified in the official SearXNG API documentation.

**Verification:**  
After the fix, confirmed that:
- `language=en` prioritizes English/Wikipedia results
- `language=ja` returns Japanese-language results
- Invalid language codes correctly fall back to `"en"`

---

### Bug 2: Schema/Implementation Mismatch (Pre-existing)

**Severity:** LOW (no functional impact)

**Description:**  
The MCP tool schema exposed `language` as the parameter name, and the internal Go struct also used `Language` with JSON tag `language`. However, the API call was sending `lang`. This mismatch could have caused confusion during debugging.

**Status:**  
Fixed automatically by Bug 1 fix - the API parameter now matches the schema and struct field names.

---

## 8. Summary of Changes

| File | Change | Reason |
|------|--------|--------|
| `search.go` | Changed `params.Set("lang", ...)` to `params.Set("language", ...)` | Use correct API parameter name per official docs |
| `main.go` | Rewrote argument parsing to handle flags after positional args | Fix --json flag being ignored |
| `main.go` | Track which flags require values | Fix --language handling |
| `validation.go` | Added language validation to `ValidateSearchArgs()` | Return error for invalid language codes |

**Result:**
The MCP server now correctly uses `language` as the API parameter, produces expected language-specific search results, and properly handles all command-line flags.

---

### Bug 3: --json Flag Was Ignored (CRITICAL)

**Severity:** CRITICAL

**Description:**  
When running `searxng-mcp-go "test" --json`, the JSON flag was ignored and human-readable text was returned instead of JSON output.

**Root Cause:**  
Go's `flag.Parse()` stops at the first positional argument. When running with query as a positional argument first (e.g., `searxng-mcp-go "test" --json`), the `--json` flag appearing after the positional argument was never parsed.

**Location:**  
`main.go` around line 134

**Original Code:**
```go
flag.CommandLine.Parse(os.Args[1:])
```

**Fix Applied:**  
Rewrote the argument parsing logic to handle flags appearing after positional arguments. The new logic collects all flags before the first positional argument AND recognizes flags appearing AFTER positional args.

**Verification:**  
`searxng-mcp-go "test" --json | jq '.'` now outputs valid JSON.

---

### Bug 4: Invalid Language Silently Accepted (LOW)

**Severity:** LOW

**Description:**  
When providing an invalid language code like `--language INVALID_LANG_123`, it was silently accepted and defaulted to English without any error or warning message.

**Root Cause:**  
The `ValidateSearchArgs()` function in `validation.go` did not validate the language field - it only validated `safesearch`, `time_range`, and `pageno`.

**Location:**  
`validation.go` - `ValidateSearchArgs()` function

**Fix Applied:**  
Added language validation to `ValidateSearchArgs()` that returns a `ValidationError` for invalid language codes.

**Verification:**  
`searxng-mcp-go "test" --language INVALID_LANG_123` now returns a validation error.

---

### Bug 5: Argument Parsing Didn't Handle Flags Requiring Values

**Severity:** MEDIUM

**Description:**  
After fixing Bug 3, the rewritten argument parser didn't know which flags require values. When running `--language INVALID_LANG`, the parser treated `INVALID_LANG` as a separate positional argument (search query) instead of the value for `--language`.

**Root Cause:**  
The custom argument parser collected flags but didn't track which flags require values and which are boolean flags.

**Location:**  
`main.go` - argument parsing logic

**Fix Applied:**  
Rewrote the argument parsing logic to properly track which flags require values and consume the following argument as the flag's value.

**Verification:**  
`searxng-mcp-go "test" --language en --json` now works correctly, and `--language INVALID_LANG` is properly recognized as an invalid language value.

---

*Report generated: April 13, 2026*
*Project: searxng-mcp-go*  
*Test server: https://search-4.xlion.dev/*
