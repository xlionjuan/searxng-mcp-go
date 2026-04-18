# SearXNG Test Queries Reference

This document defines which queries to use when inspecting raw SearXNG JSON responses via `curl`, for the purpose of writing tests or verifying JSON parsing behavior.

Use this command pattern:

```bash
curl -s "https://search-4.xlion.dev/search?q=<query>&format=json" | jq '.<field>'
```

---

## Response Fields Overview

| Field | Expected Availability | Notes |
|-------|----------------------|-------|
| `query` | Always present | Echo of the search query |
| `number_of_results` | Always present (but often `0`) | Value is frequently 0 even when results exist; the field itself is always in the response |
| `answers` | Only for special queries | See [Answer Triggers](#answer-triggers) below |
| `infoboxes` | Entity queries | Search for company names, famous people, products (e.g., "apple inc", "nvidia", "elon musk") |
| `results` | Almost always present | Standard search results; empty only on no-match queries |
| `suggestions` | Usually present | Search suggestions; may be empty for very specific queries |
| `corrections` | Rarely present | Only when a search engine detects a typo; most engines don't implement this |
| `unresponsive_engines` | Sometimes present | List of engines that failed to respond |

---

## Answer Triggers

Answers are computed **locally** by SearXNG's built-in answerers and plugins, not fetched from external search engines. They trigger when the query matches specific keywords.

### Answerers (built-in, run before engines)

| Trigger Keyword | Name | Description | Example |
|----------------|------|-------------|---------|
| `random` | Random value generator | Generates various random values | `random string`, `random int`, `random float`, `random sha256`, `random uuid`, `random color` |
| `min`, `max`, `avg`, `sum`, `prod`, `range` | Statistics functions | Compute min/max/avg/sum/prod/range of arguments | `avg 123 548 2.04 24.2` |

### Plugins (run as post-search hooks)

| Trigger Keyword | Name | Description | Example |
|----------------|------|-------------|---------|
| *(any math expression)* | Calculator | Parses and evaluates mathematical expressions | `2+2*3`, `(10+5)/3` |
| `md5`, `sha1`, `sha224`, `sha256`, `sha384`, `sha512` | Hash Plugin | Converts a string to hash digests | `sha512 The quick brown fox jumps over the lazy dog` |
| `ip` | Self Info | Shows the requester's IP address | `ip` |
| `user-agent` | Self Info | Shows the requester's User-Agent string | `user-agent` |
| `time`, `clock` | Timezone Plugin | Shows time in different timezones | `time Berlin`, `clock Los Angeles` |

---

## Test Query Recipes

### One query per field

| Field to Test | Query | Expected Result |
|--------------|-------|-----------------|
| `answers` (hash) | `sha512 hello` | `answers[0].answer` = SHA512 digest |
| `answers` (self info) | `ip` | `answers[0].answer` = IP address |
| `answers` (timezone) | `time Berlin` | `answers[0].answer` = current time in Berlin |
| `answers` (statistics) | `avg 1 2 3 4 5` | `answers[0].answer` = average |
| `answers` (random) | `random uuid` | `answers[0].answer` = UUID |
| `infoboxes` | `apple inc` | `infoboxes[0].content` = company description |
| `results` | `golang tutorial` | `results[]` = array of search results |
| `suggestions` | `golang` | `suggestions[]` = array of search suggestions |
| `number_of_results` | Any query | Field always present (value often 0) |
| `corrections` | *(rare)* | Almost never populated in practice |

### Multi-field query (all fields at once)

```
apple inc
```

Expected: `answers` (filtered if overlapping with infobox) + `infoboxes` + `results` + `suggestions`.

### Dedup behavior

DuckDuckGo's engine often puts the same Wikipedia summary in both `answers` and `infoboxes`. Our client deduplicates this: if an answer's text is a prefix of any infobox content, the answer is filtered out. Use `apple inc` to verify this behavior.

### Empty / edge case queries

| Query | Expected |
|-------|----------|
| `xyzzyqqq1234567890xyz` | No results, possibly no suggestions |
| Empty string | Validation error from client |
| Very long query (>500 chars) | Should handle gracefully |

---

## Quick Reference

```bash
# Test answers (hash)
curl -s "https://search-4.xlion.dev/search?q=sha512+hello&format=json" | jq '.answers'

# Test answers (IP)
curl -s "https://search-4.xlion.dev/search?q=ip&format=json" | jq '.answers'

# Test infoboxes
curl -s "https://search-4.xlion.dev/search?q=apple+inc&format=json" | jq '.infoboxes'

# Test suggestions
curl -s "https://search-4.xlion.dev/search?q=golang&format=json" | jq '.suggestions'

# Test results
curl -s "https://search-4.xlion.dev/search?q=golang+tutorial&format=json" | jq '.results | length'

# Full response keys
curl -s "https://search-4.xlion.dev/search?q=apple+inc&format=json" | jq 'keys'
```
