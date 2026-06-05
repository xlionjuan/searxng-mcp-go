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

#### 1. `Answer` (union answer object)

The Go client represents all answer types as a single `Answer` struct with
optional typed fields. Legacy string answers populate the `answer` field;
typed answers (translations, weather) use template-specific fields.

```json
{
  "answer": "The computed result or information",
  "engine": "engine_name",
  "template": "template_name",
  "url": "https://example.com/result",
  "translations": [
    {
      "text": "translated text",
      "transliteration": "transliteration",
      "examples": ["example 1", "example 2"],
      "definitions": ["definition 1"],
      "synonyms": ["synonym 1"]
    }
  ],
  "current": {
    "location": { "name": "Tokyo", "country_code": "JP" },
    "summary": "Partly cloudy",
    "temperature": { "val": 22.5, "unit": "°C" },
    "condition": "partly_cloudy"
  },
  "forecasts": [...],
  "service": "engine_service_name"
}
```

Fields:
- `answer` (string): Legacy answer text. Typed answers may omit this field; a
  fallback string is derived by the normalization layer.
- `engine` (string): The engine that produced the answer.
- `template` (string, optional): The engine's answer template name.
- `url` (string, optional): A URL associated with the answer.
- `translations` (array, optional): Translation entries, present for translation-type answers.
- `current` (object, optional): Current weather conditions, present for weather-type answers.
- `forecasts` (array, optional): Weather forecast entries, present for weather-type answers.
- `service` (string, optional): Service name or category for the answer.

#### 2. `Translations` (translation results)

A translation-type answer contains one or more `TranslationItem` entries
with the translated text, transliteration, examples, definitions, and synonyms.

The Go struct for each translation entry:

```json
{
  "text": "translated text",
  "transliteration": "transliteration",
  "examples": ["example 1", "example 2"],
  "definitions": ["definition 1"],
  "synonyms": ["synonym 1"]
}
```

```go
type TranslationItem struct {
    Text            string   `json:"text"`
    Transliteration string   `json:"transliteration,omitempty"`
    Examples        []string `json:"examples,omitempty"`
    Definitions     []string `json:"definitions,omitempty"`
    Synonyms        []string `json:"synonyms,omitempty"`
}
```

Fields:
- `text` (string): The translated text.
- `transliteration` (string, optional): Transliteration of the translated text.
- `examples` (array of strings, optional): Usage examples for the translation.
- `definitions` (array of strings, optional): Definitions of the translated term.
- `synonyms` (array of strings, optional): Synonyms for the translated term.

**Fallback display:** When the legacy `answer` string is empty and `Translations` is populated, the server produces `"Translation: <text>; <text>; ..."` using `translationAnswerFallback`.

#### 3. `WeatherAnswer` (weather data)

A weather-type answer contains a `Current` weather observation and/or
`Forecasts` array, each represented as a `WeatherItem`.

The Go struct for a weather observation:

```json
{
  "location": {
    "name": "Tokyo",
    "latitude": 35.6762,
    "longitude": 139.6503,
    "elevation": 40.0,
    "country_code": "JP",
    "timezone": "Asia/Tokyo"
  },
  "datetime": {
    "datetime": "2026-05-28T12:00:00+09:00"
  },
  "summary": "Partly cloudy",
  "temperature": { "val": 22.5, "unit": "°C" },
  "feels_like": { "val": 21.0, "unit": "°C" },
  "condition": "partly_cloudy",
  "pressure": { "val": 1013.25, "unit": "hPa" },
  "humidity": { "val": 65.0, "unit": "%" },
  "wind_from": { "val": 180.0, "unit": "°" },
  "wind_speed": { "val": 3.5, "unit": "m/s" },
  "cloud_cover": 40
}
```

```go
type WeatherItem struct {
    Location    WeatherLocation  `json:"location"`
    Datetime    *WeatherDateTime `json:"datetime,omitempty"`
    Summary     string           `json:"summary,omitempty"`
    Temperature WeatherMeasure   `json:"temperature"`
    FeelsLike   *WeatherMeasure  `json:"feels_like,omitempty"`
    Condition   string           `json:"condition"`
    Pressure    *WeatherMeasure  `json:"pressure,omitempty"`
    Humidity    *WeatherMeasure  `json:"humidity,omitempty"`
    WindFrom    *WeatherMeasure  `json:"wind_from,omitempty"`
    WindSpeed   *WeatherMeasure  `json:"wind_speed,omitempty"`
    CloudCover  *int             `json:"cloud_cover,omitempty"`
}
```

Supporting types:

```go
type WeatherLocation struct {
    Name        string  `json:"name"`
    Latitude    float64 `json:"latitude,omitempty"`
    Longitude   float64 `json:"longitude,omitempty"`
    Elevation   float64 `json:"elevation,omitempty"`
    CountryCode string  `json:"country_code,omitempty"`
    Timezone    string  `json:"timezone,omitempty"`
}

type WeatherDateTime struct {
    Datetime string `json:"datetime"`
}

type WeatherMeasure struct {
    Val  float64 `json:"val"`
    Unit string  `json:"unit,omitempty"`
}
```

Fields (WeatherItem):
- `location` (object): Location information (name, coordinates, country, timezone).
- `datetime` (object, optional): ISO 8601 timestamp for the observation.
- `summary` (string, optional): Human-readable weather summary.
- `temperature` (object): Temperature measurement with value and unit.
- `feels_like` (object, optional): Apparent (feels-like) temperature.
- `condition` (string): Weather condition code (e.g., "partly_cloudy", "clear", "rain").
- `pressure` (object, optional): Atmospheric pressure measurement.
- `humidity` (object, optional): Relative humidity percentage.
- `wind_from` (object, optional): Wind direction in degrees.
- `wind_speed` (object, optional): Wind speed measurement.
- `cloud_cover` (integer, optional): Cloud cover percentage.

**Fallback display:** When the legacy `answer` string is empty and `Current` is populated, the server produces a human-readable summary using `weatherAnswerFallback`, preferring `Current.Summary` first, then falling back to `"Weather: <location>, <temperature>, <condition>"`.

The `WeatherMeasure.String()` method returns a compact representation: `"<value> <unit>"` or just `"<value>"` when unit is empty.

### Answer Types Summary

| Template | Typed Fields | Fallback Derivation |
|----------|-------------|---------------------|
| `simple` or empty | None (uses legacy `answer` string) | N/A — `answer` already populated |
| `translations.html` | `Translations []TranslationItem` | `"Translation: <text>; <text>; ..."` |
| `weather.html` | `Current *WeatherItem`, `Forecasts []WeatherItem` | `Current.Summary` or `"Weather: <location>, <temperature>, <condition>"` |

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
    Answer       string            `json:"answer"`
    Engine       string            `json:"engine"`
    Template     string            `json:"template,omitempty"`
    URL          string            `json:"url,omitempty"`
    Translations []TranslationItem `json:"translations,omitempty"`
    Current      *WeatherItem      `json:"current,omitempty"`
    Forecasts    []WeatherItem     `json:"forecasts,omitempty"`
    Service      string            `json:"service,omitempty"`
}

type SearchResponse struct {
    Query               string         `json:"query"`
    Warning             string         `json:"warning"`
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
infobox content are deduplicated by `DeduplicateAnswers` before the response
is returned.

---

## References

- Source: `searx/results.py` — `ResultContainer` class
- Source: `searx/webutils.py` — `get_json_response()` function
- Source: `searx/answerers/` — Built-in answerer modules
- Source: `searx/result_types/answer.py` — Answer type definitions
- Source: `searx/search/__init__.py` — `search_answerers()` method
