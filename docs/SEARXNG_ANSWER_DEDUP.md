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

The function `deduplicateAnswers` in `internal/searxng/deduplicate.go` filters answers at the **search response layer**, before the result reaches JSON serialization or CLI formatting. This means both output modes benefit.

**Algorithm (exact-case fast path + lazy lowercase fallback):**

For each answer, the following steps are applied:

*Fast path (no allocation):*
1. Strip the known suffix `" More at Wikipedia"` (exact case) that DuckDuckGo appends.
2. Take the first 200 characters of the stripped text as a prefix.
3. Check if any infobox content **contains** this prefix using exact-case substring matching.

*Lazy fallback (lowercase, built on first use):*
If the fast path did not match, a lowercased copy of all infobox contents is built once and reused for subsequent answers:
1. Lowercase the answer text.
2. Strip the known suffix `" more at wikipedia"` (lowercase).
3. Take the first 200 characters of the stripped text as a prefix.
4. Check if any lowercased infobox content **contains** this prefix (case-insensitive substring match).

If either path finds a match, the answer is considered a duplicate and is removed. The lazy fallback handles edge cases where the answer and infobox differ only in casing (e.g. `"Apple"` vs `"apple"`).

### Why Search Layer, Not Format Layer

Deduplication happens while normalizing the SearXNG response, using `deduplicateAnswers` from `internal/searxng/deduplicate.go`, rather than in `formatResults()` (format.go) so that:
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

## Bounded Work Cap (CAND-33fe0b85-RUNTIME-003)

The dedup loop is O(len(answers) * len(infoboxes)) substring work, and for
each answer it scans every infobox twice (exact case, then lowercase) with
`strings.Contains`. A successful JSON body is already capped at
`MaxResponseBodySize` (2 MiB), but compact answer/infobox arrays can still
pack tens of thousands of entries into that envelope and force very large CPU
work before result limiting.

To bound that work, response normalization trims `Answers` and `Infoboxes`
to `MaxAnswers` and `MaxInfoboxes` (each 100) **before** the dedup call:

- Real SearXNG responses contain at most a handful of each, so 100 is a
  generous cap.
- Worst-case dedup work is bounded at
  `100 * 100 * 2 = 20,000` `Contains` calls.
- Truncation is logged with `slog.Warn` (`truncating answers/infoboxes
  before deduplication`) and is visible in debug logs.

The cap is enforced in `normalizeResponse` (see
`internal/searxng/response.go`), not in `deduplicateAnswers` itself, so the
dedup function remains a pure data transform and tests can exercise it
without a searcher.

## Implementation

- **Function**: `deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer` in `internal/searxng/deduplicate.go`
- **Called**: During response normalization after JSON unmarshalling, after the `MaxAnswers` / `MaxInfoboxes` cap is applied, before the response is returned to formatting/output code
- **Tests**:
  - 11 cases in `internal/searxng/deduplicate_internal_test.go` covering empty inputs, exact match, prefix match, DDG "More at Wikipedia" suffix stripping, case insensitivity, distinct answers (IP), mixed scenarios, empty answer skipping, typed answers with fallback text, and fixture survival
  - 5 new subtests in `TestNormalizeResponse` (`internal/searxng/response_internal_test.go`) covering cap truncation for answers and infoboxes, no-op when at the cap, dedup behavior after truncation, and a pathological-input time budget assertion
