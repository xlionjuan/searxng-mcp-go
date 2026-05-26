# Output Format

The SearXNG MCP Server provides two output modes: **CLI text mode** (default) and **JSON mode** (`--json`).

```bash
# CLI text mode (default)
./searxng-mcp-go "query"

# JSON mode
./searxng-mcp-go "query" --json
```

---

## Example 1: Full Output (All Fields Populated)

Query: `apple inc` — this query triggers Infoboxes, Results, and Suggestions simultaneously. Answers from DuckDuckGo are deduplicated against infobox content and do not appear.

### CLI Text Mode

```
$ ./searxng-mcp-go "apple inc"

=== Web Search Results ===
Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.

=== Infoboxes ===

[1] Apple Inc.
    Apple Inc. is an American multinational technology company headquartered in Cupertino, California, in Silicon Valley, best known for its consumer electronics, software and online services. ...
    Attributes:
      - Formerly called: Apple Computer Company (1976–1977), Apple Computer, Inc. (1977–2007)
      - Type: Public
      - Traded as: NASDAQ: AAPL, Nasdaq-100 component, DJIA component, S&P 100 component, S&P 500 component
      - Industry: Consumer electronics, Software services, Online services
      - Founded: April 01, 1976, in Los Altos, California, US
      - Founders: Steve Jobs, Steve Wozniak, Ronald Wayne
      - Key people: Arthur Levinson (chairman), Tim Cook (CEO)
      - Products: AirPods, AirTag, Apple TV, Apple Vision Pro, Apple Watch, HomePod, iPad, iPhone, Mac
      ...
    URLs:
      - Wikipedia: https://en.wikipedia.org/wiki/Apple_Inc.
      - Official site: https://www.apple.com/
      ...

=== Results ===

Found 16 results for 'apple inc':

1. Apple Inc.
   URL: https://www.apple.com/
   Summary: Discover the innovative world of Apple and shop everything iPhone, iPad, Apple Watch, Mac, and Apple TV, plus explore accessories, entertainment, and expert device support.
   Engine: ddg definitions

2. Apple Inc. (AAPL) Stock Price, News, Quote & History - Yahoo Finance
   URL: https://finance.yahoo.com/quote/AAPL/
   Summary: Find the latest Apple Inc. (AAPL) stock quote, history, news and other vital information to help you with your stock trading and investing.
   Engine: google

...

=== Search Suggestions ===
  - Apple app
  - Retail companies of the United States
  - Apple Store locations
  - Apple Inc stock
  - Steve Jobs
  - ...
```

### JSON Mode

```json
{
  "query": "apple inc",
  "warning": "Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.",
  "number_of_results": 16,
  "infoboxes": [
    {
      "infobox": "Apple Inc.",
      "content": "Apple Inc. is an American multinational technology company headquartered in Cupertino, California...",
      "attributes": [
        {
          "label": "Type",
          "value": "Public"
        }
      ],
      "urls": [
        {
          "title": "Official site",
          "url": "https://www.apple.com/"
        }
      ]
    }
  ],
  "results": [
    {
      "title": "Apple Inc.",
      "url": "https://www.apple.com/",
      "content": "Discover the innovative world of Apple and shop everything iPhone, iPad, Apple Watch, Mac, and Apple TV...",
      "engine": "ddg definitions"
    },
    {
      "title": "Apple Inc. (AAPL) Stock Price, News, Quote & History - Yahoo Finance",
      "url": "https://finance.yahoo.com/quote/AAPL/",
      "content": "Find the latest Apple Inc. (AAPL) stock quote, history, news...",
      "engine": "google"
    }
  ],
  "suggestions": [
    "Apple app",
    "Retail companies of the United States",
    "Apple Store locations",
    "Apple Inc stock"
  ]
}
```

---

## Example 2: Simple Output (Results and Suggestions Only)

Query: `golang tutorial` — this query returns no Answers or Infoboxes.

### CLI Text Mode

```
$ ./searxng-mcp-go "golang tutorial"

=== Web Search Results ===
Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.

=== Results ===

Found 17 results for 'golang tutorial':

1. Tutorials - The Go Programming Language
   URL: https://go.dev/doc/tutorial/
   Summary: Learn Go with tutorials on various topics, such as modules, databases, APIs, generics, fuzzing, and vulnerabilities.
   Engine: google

2. Go Tutorial - GeeksforGeeks
   URL: https://www.geeksforgeeks.org/go-language/go/
   Summary: Go (or Golang) is a modern programming language developed by Google, designed for building fast and reliable applications...
   Engine: google

3. Go by Example
   URL: https://gobyexample.com/
   Summary: Learn Go by doing with annotated example programs. Go by Example covers topics such as variables, functions, channels...
   Engine: google

...

=== Search Suggestions ===
  - Best Golang tutorial
  - Golang tutorial interactive
  - Golang tutorial w3schools
  - Golang tutorial youtube
  - Golang tutorial for beginners
  - Golang tutorial free
```

### JSON Mode

```json
{
  "query": "golang tutorial",
  "warning": "Search results come from external sources and may be inaccurate, outdated, or adversarial; verify before using them.",
  "number_of_results": 17,
  "results": [
    {
      "title": "Tutorials - The Go Programming Language",
      "url": "https://go.dev/doc/tutorial/",
      "content": "Learn Go with tutorials on various topics, such as modules, databases, APIs, generics, fuzzing, and vulnerabilities.",
      "engine": "google"
    },
    {
      "title": "Go Tutorial - GeeksforGeeks",
      "url": "https://www.geeksforgeeks.org/go-language/go/",
      "content": "Go (or Golang) is a modern programming language developed by Google...",
      "engine": "google"
    }
  ],
  "suggestions": [
    "Best Golang tutorial",
    "Golang tutorial interactive",
    "Golang tutorial w3schools",
    "Golang tutorial for beginners",
    "Golang tutorial free"
  ]
}
```

Note: `answers` and `infoboxes` are omitted from this output because they are empty (uses `omitempty`). In contrast, `results` and `suggestions` always appear — they are forced to `[]` (empty array) rather than omitted.

---

## Empty Field Handling

When a field in the query results has no value, the behavior is as follows:

| Mode | Behavior |
|------|----------|
| **CLI text mode** | The entire section is omitted — nothing is printed. For example, if there are no Answers, the `=== Answers ===` heading will not appear. |
| **JSON mode** | `warning`, `results`, and `suggestions` are always present for successful searches. `answers` and `infoboxes` use `omitempty` and are omitted when empty. `results` and `suggestions` are forced to `[]` (empty array) when empty, never omitted or `null`. `unresponsive_engines` is omitted unless debug mode is enabled. |

`corrections` is intentionally excluded from all output modes per ADR-005.

### Specific Rules

**CLI text mode:**
- successful output includes the `=== Web Search Results ===` warning header
- `answers` is empty → omit the entire `=== Answers ===` section
- `infoboxes` is empty → omit the entire `=== Infoboxes ===` section
- `infobox.attributes` is empty → omit the `Attributes:` subsection for that infobox
- `infobox.urls` is empty → omit the `URLs:` subsection for that infobox
- `results` is empty → omit the entire `=== Results ===` section
- `result.content` is empty → omit the `Summary:` line for that result
- `result.publishedDate` is empty → omit the `Published date:` line for that result
- `suggestions` is empty → omit the entire `=== Search Suggestions ===` section

**JSON mode:**
- `warning` is present for successful search responses
- `answers` is empty → no `answers` key in the JSON (omitempty)
- `infoboxes` is empty → no `infoboxes` key in the JSON (omitempty)
- `results` is empty → `"results": []` (always present, forced to empty array)
- `suggestions` is empty → `"suggestions": []` (always present, forced to empty array)
- `unresponsive_engines` is empty and debug mode is on → `"unresponsive_engines": []` (always present in debug JSON)
- debug mode is off → no `unresponsive_engines` key in the JSON
- `result.publishedDate` is empty → no `publishedDate` key in that result object
- `infobox.attributes` is empty → no `attributes` key in that infobox object
- `infobox.urls` is empty → no `urls` key in that infobox object
- `answer.template` is empty → no `template` key in that answer object

---

## Field Ordering

### JSON Mode Field Order

1. **`query`** — Search query echoed in the response
2. **`warning`** — External-content warning for AI agents and JSON consumers
3. **`answers`** — Direct answers (e.g., IP, hash, timezone, etc.)
4. **`number_of_results`** — Total result count reported in JSON output
5. **`infoboxes`** — Knowledge panels
6. **`results`** — Search result list
7. **`suggestions`** — Related search suggestions

`answers` and `infoboxes` may be omitted when empty. `results` and `suggestions` remain present as arrays.

#### Output Order Diagram

```
query
  ↓
warning
  ↓
answers
  ↓
number_of_results
  ↓
infoboxes
  ↓
results
  ↓
suggestions
```

```json
{
  "query": "string",
  "warning": "string",
  "answers": [],
  "number_of_results": "int",
  "infoboxes": [],
  "results": [],
  "suggestions": [],
  "unresponsive_engines": []  // debug mode only
}
```

Note: JSON does not guarantee field order, but Go's `encoding/json.Marshal` serializes struct fields in declaration order. The order above is enforced by the Go struct definition and its `MarshalJSON` override. `warning`, `results`, and `suggestions` are always present for successful searches; `answers` and `infoboxes` use `omitempty` (omitted when empty). `corrections` is intentionally excluded per ADR-005. `unresponsive_engines` is debug-only and omitted when debug mode is off.

---

## Field Trigger Conditions

Whether each field is populated depends on the results returned by the SearXNG backend engines:

| Field | Trigger Condition |
|-------|-------------------|
| **`answers`** | SearXNG's answerer module is triggered (e.g., DuckDuckGo Instant Answer, calculator, IP lookup, hash lookup, timezone conversion, etc.). Entity queries (e.g., company names, celebrity names) tend to trigger this. |
| **`infoboxes`** | The query targets a well-known entity (person, company, location, concept) and SearXNG engines (e.g., Wikipedia, Wikidata) have corresponding knowledge panel data. |
| **`results`** | Nearly all queries return results. The count depends on the number of SearXNG engines configured and their responses. |
| **`suggestions`** | SearXNG engines return related search suggestions. Most queries will have suggestions, but not 100%. |

### Result Sub-field Trigger Conditions

| Sub-field | Trigger Condition |
|-----------|-------------------|
| `title` | Always populated (required field) |
| `url` | Always populated (required field) |
| `content` | The engine returned a summary/description; some results may lack this field |
| `engine` | Always populated; indicates which search engine produced this result |
| `publishedDate` | SearXNG provided a publication date for the result |

**CLI Text Content Truncation:** In CLI text mode only, `formatResults` truncates
result summaries (`content` field) and infobox content to **4000 Unicode
characters (runes)** to keep terminal output manageable for LLM context windows.
JSON mode and MCP mode return the full normalized response without this
formatting truncation.

---

## Quick Reference

```bash
# CLI text mode (default)
./searxng-mcp-go "apple inc"

# JSON mode
./searxng-mcp-go "apple inc" --json

# With additional parameters
./searxng-mcp-go "apple inc" --json --language=en --safesearch=1 --time_range=month

# Custom SearXNG server
./searxng-mcp-go "query" --searxng-url=https://your-searxng.example.com

# Debug mode
./searxng-mcp-go "query" --debug
```
