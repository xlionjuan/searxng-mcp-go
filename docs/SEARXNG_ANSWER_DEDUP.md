# SearXNG Answer Deduplication

## Problem

SearXNG's DuckDuckGo engine returns Wikipedia summaries in **both** `answers` and `infoboxes` fields for entity queries (company names, people, places, etc.). This causes duplicate content in our output — the same text appears under "=== Answers ===" and again under "=== Infoboxes ===".

### Example: Query "apple inc"

SearXNG raw JSON (simplified):

```json
{
  "answers": [
    {
      "answer": "Apple Inc. is an American multinational technology company headquartered in Cupertino, California...",
      "engine": "duckduckgo",
      "template": "wikipedia.html"
    }
  ],
  "infoboxes": [
    {
      "infobox": "Apple Inc.",
      "content": "Apple Inc. is an American multinational technology company headquartered in Cupertino, California...",
      "urls": [...]
    }
  ]
}
```

The `answers[0].answer` text is a verbatim copy of `infoboxes[0].content` (or a prefix of it).

### Queries Affected

This primarily affects **entity queries** — searches that trigger DuckDuckGo's Wikipedia instant answer:

- Company names: "apple inc", "google", "microsoft"
- People: "elon musk", "albert einstein"
- Places: "tokyo", "mount everest"
- General knowledge: "python programming language"

### Queries NOT Affected

Queries where `answers` contains genuinely distinct information:

- IP lookup: "ip" → answer is your public IP, infobox may explain what an IP is
- Calculator: "2+2" → answer is "4"
- Timezone: "time in tokyo"
- Hash lookup: hash values in answers

## Solution

### Deduplication Strategy

The function `deduplicateAnswers()` in `search.go` filters answers at the **search response layer**, before the result reaches JSON serialization or CLI formatting. This means both output modes benefit.

**Algorithm:**

For each answer, check if its text is:
1. A **substring** of any infobox content (case-insensitive), OR
2. **Contains** any infobox content (case-insensitive)

If either condition is true, the answer is considered a duplicate of the infobox and is removed.

This catches:
- Exact matches: answer text == infobox content
- Prefix matches: answer text is a truncated version of infobox content
- Superset matches: answer text contains the full infobox content

### Why Search Layer, Not Format Layer

Deduplication happens in `performSearch()` (search.go) rather than in `formatResults()` (format.go) so that:
- **JSON output** (`--json` flag or MCP mode) also has clean results
- **CLI output** benefits automatically
- Single point of truth — no scattered filtering logic

### Edge Cases Handled

| Scenario | Behavior |
|----------|----------|
| Empty `answers` | Return as-is (no filtering) |
| Empty `infoboxes` | Return as-is (no filtering) |
| Infobox with empty `content` | Skip that infobox (no filtering against it) |
| Empty `answer` text | Skip (filtered out) |
| Case differences | Case-insensitive comparison |
| Multiple answers, some duplicate | Keep non-duplicates, remove duplicates |

## Implementation

- **Function**: `deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer` in `search.go`
- **Called**: In `performSearch()` after JSON unmarshalling, before `inferDates()`
- **Tests**: 8 test cases in `search_test.go` covering empty inputs, exact match, prefix match, case insensitivity, distinct answers (IP), and mixed scenarios
