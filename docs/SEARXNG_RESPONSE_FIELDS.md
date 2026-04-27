# SearXNG JSON Response: `corrections` and `answers` Fields

Analysis of two optional fields in the SearXNG JSON search API response.

## Overview

SearXNG's JSON response (`GET /search?format=json`) returns these top-level fields:

```json
{
  "query": "original query",
  "number_of_results": 1234,
  "results": [...],
  "suggestions": ["suggested query 1", "suggested query 2"],
  "answers": [...],
  "corrections": ["corrected spelling 1", "corrected spelling 2"],
  "infoboxes": [...],
  "unresponsive_engines": [...]
}
```

`unresponsive_engines` is included only in debug output and contains `[engine_name, error_message]` pairs.

---

## `corrections`

### Definition

Spelling corrections detected by search engines for the user's query. Engines like
Google, Bing, or DuckDuckGo may identify typos and return corrected alternatives.

### Type

```
corrections: string[]
```

An array of strings. Each string is a corrected query variant.

### Source

- **Engine results**: When an engine detects a typo, it returns a result object with
  a `"correction"` key. SearXNG collects these into a `set[str]` in `ResultContainer.corrections`.
- Duplicate corrections across engines are deduplicated (set semantics).

### When Non-Empty

- The user's query contains a likely misspelling.
- At least one selected engine provides spelling correction data.
- Common with Google, Bing, DuckDuckGo for typo queries.

### JSON Example

```json
{
  "corrections": ["corrected query", "another correction"]
}
```

---

## `answers`

### Definition

Instant answers computed from the query. These are direct responses (not web links)
provided by SearXNG's built-in **answerers** and **plugins**.

### Type

```
answers: object[]
```

An array of typed answer objects. Each object's structure depends on the answer type.

### Common Answer Types

#### 1. `Answer` (simple string answer)

```json
{
  "answer": "The computed result or information",
  "engine": "engine_name",
  "template": "template_name"
}
```

Fields:
- `answer` (string): The answer text.
- `engine` (string): The engine that produced the answer.
- `template` (string, optional): The engine's answer template name.

#### 2. `Translations` (translation results)

Contains translation items with definitions, examples, and synonyms.

#### 3. `WeatherAnswer` (weather data)

Contains structured weather information.

### Source

1. **Answerers** (`searx.answerers`): Registered modules that pattern-match the query
   and return instant answers. Built-in answerers include:
   - **Random**: `random number`, `random string`, `random sha256`
   - **Statistics**: `10 + 5`, `sqrt(144)`, unit conversions

2. **Plugins** (`searx.plugins`): Plugins with `post_search` hooks can inject
   answers into the result container.

### Processing Flow

```
Query received
  -> answerers.ask(query) called first
     -> If match: answer(s) added to AnswerSet, skip engine search
  -> If no answerer match: standard engine search proceeds
  -> plugins post_search hooks run on results
     -> Can inject additional answers
```

Key detail: If an answerer matches the query, SearXNG returns the answer
**without** performing a standard engine search (the `search_answerers` short-circuits
`search_standard`).

### When Non-Empty

- Query matches a built-in answerer pattern (e.g., `random sha256`, `10 usd to eur`).
- A plugin's `post_search` hook produces an answer.
- An engine returns a legacy `{"answer": "..."}` result (deprecated).

### Deduplication

Answers are deduplicated by hash. The `AnswerSet` class ignores duplicates before
sorting and iteration.

### JSON Example

```json
{
  "answers": [
    {"answer": "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"}
  ]
}
```

---

## Key Differences: `corrections` vs `answers`

| Aspect | `corrections` | `answers` |
|--------|---------------|-----------|
| **Purpose** | Suggest query fixes | Provide direct answers |
| **Type** | `string[]` | `object[]` (typed) |
| **Content** | Corrected spellings | Computed/instant results |
| **Source** | Search engines | Answerers + plugins |
| **Effect on search** | None (suggestion only) | May skip engine search |
| **Typical use** | "Did you mean X?" | Calculations, facts, translations |

---

## Current Implementation Note

The `searxng-mcp-go` codebase now exposes `answers`, `infoboxes`, and
`unresponsive_engines` in the `SearchResponse` struct. The Go struct maps:

```go
type Answer struct {
    Answer   string `json:"answer"`
    Engine   string `json:"engine"`
    Template string `json:"template,omitempty"`
}

type SearchResponse struct {
    Query               string         `json:"query"`
    Answers             []Answer       `json:"answers,omitempty"`
    NumberOfResults     int            `json:"number_of_results"`
    Infoboxes           []Infobox      `json:"infoboxes,omitempty"`
    Results             []SearchResult `json:"results"`
    Suggestions         []string       `json:"suggestions"`
    UnresponsiveEngines [][]string     `json:"unresponsive_engines,omitempty"`
    Debug               bool           `json:"-"`
}
```

Note: `corrections` is **not** currently exposed. `unresponsive_engines` is
omitted unless debug mode is enabled. DuckDuckGo answers that overlap with
infobox content are deduplicated by `deduplicateAnswers()` before the response
is returned.

---

## References

- Source: `searx/results.py` — `ResultContainer` class
- Source: `searx/webutils.py` — `get_json_response()` function
- Source: `searx/answerers/` — Built-in answerer modules
- Source: `searx/result_types/answer.py` — Answer type definitions
- Source: `searx/search/__init__.py` — `search_answerers()` method
