# SearXNG MCP Server (searxng-mcp-go)

A Model Context Protocol (MCP) server and CLI tool that proxies web search requests to a SearXNG meta-search engine instance and returns formatted results.

## Language

### Core Types

**SearXNGSearcher**: The HTTP client that holds a base URL and an `*http.Client` and communicates with a SearXNG instance to execute search queries.
_Avoid_: Searcher (ambiguous — appears only in docstrings, not as a separate exported type)

**Config**: The connection parameters for a SearXNG instance — a base URL, a timeout duration, and an optional custom HTTP client.

**SearchArgs**: All input parameters for a search operation — the search query, language code, SafeSearch level, time range, categories, engines, page number, and result limit.
_Avoid_: SearchParams

**SearchResponse**: The complete, normalized response from SearXNG, containing the query string, direct answers, result count, infoboxes, web results, suggestions, and unresponsive-engine diagnostics.

**SearchResult**: A single web result returned by a search engine — title, URL, content summary, engine name, and optional publication date.

### Response Components

**Answer**: A direct instant answer from SearXNG (e.g., IP address, hash digest, calculator result, timezone), carrying an answer string, engine name, and optional template name.
_Avoid_: Instant Answer (SearXNG internal term; not used in Go types)

**Infobox**: A structured knowledge panel from SearXNG containing a title, body content, key-value attribute pairs, and related URLs — typically derived from Wikipedia or Wikidata for entity queries (people, companies, locations).

**InfoboxAttribute**: A single key–value attribute pair within an infobox (e.g., `Founded: April 01, 1976`).

**InfoboxURL**: A titled URL entry within an infobox (e.g., `Wikipedia: https://en.wikipedia.org/wiki/Apple_Inc.`).

**Suggestion**: An alternative search query suggested by the upstream search engines when the original query could be refined.

**UnresponsiveEngines**: A list of `[engine_name, error_message]` pairs for search engines that failed during a query — only exposed in JSON output when Debug Mode is active.
_Avoid_: DeadEngines, FailedEngines

### Operation Modes

**CLI Mode**: An operation mode where the program accepts command-line arguments, executes a one-shot search, and prints formatted text (or JSON with `--json`) to stdout, then exits.

**MCP Mode**: An operation mode where the program starts a stdio JSON-RPC server implementing the Model Context Protocol, listening for incoming `search` tool-call requests and returning JSON responses; uses no CLI args, only environment variables.

**Debug Mode**: An operational mode activated by `--debug` or `DEBUG=1` that logs HTTP request/response details and raw JSON body previews to stderr and gates exposure of `UnresponsiveEngines` in JSON output.

**CLIFlags**: The parsed command-line flag values that include search parameters (query, language, safesearch, etc.) plus meta-flags for output format (`--json`), help, version, debug, and custom SearXNG URL.

### Error Types

**ValidationError**: An error raised when user-supplied search arguments fail validation rules (e.g., empty query, excessive length, invalid time range, control characters).

**SearXNGError**: An error originating from the SearXNG service layer, carrying the HTTP status code, response content type, a truncated body preview, and the underlying cause.

**HTMLResponseError**: A specialized error for when the SearXNG server returns an HTML page instead of the expected JSON — signals that `format=json` is likely not enabled on the instance.

### Search Behavior

**Deduplicate**: The process of filtering out `Answer` entries whose text overlaps with `Infobox` content (specifically, DuckDuckGo returning the same Wikipedia summary in both answers and infoboxes), using prefix matching on the first 200 characters with a lowercase fallback.
_Avoid_: Dedup (internal function name; prefer the full term in docs)

**setBrowserHeaders**: The function that applies Chrome-like HTTP headers (User-Agent, Accept, Sec-* family, Priority) to every search request to bypass SearXNG's limiter / bot-detection mechanism.
_Avoid_: BotHeaders, StealthHeaders

**GET Fallback**: The automatic retry mechanism that re-issues a failed POST search as a GET request when the SearXNG instance returns HTTP 405 (Method Not Allowed) or 501 (Not Implemented).
_Avoid_: POSTtoGETFallback (internal test function name)

**Private Host Detection**: The validation that classifies literal IP addresses and known private hostname suffixes (10.x, 172.16-31, 192.168, localhost, `*.local`, `*.internal`, IPv6 unique-local, link-local, etc.) without DNS resolution — used to warn when HTTP is used to reach non-private hosts.

**prepareMCPStdin**: The function that peeks at the first line of stdin to verify it contains a valid MCP initialize message (JSON-RPC 2.0 with method `initialize`), preventing the MCP server from hanging when piped non-MCP input.

### Constants

**MaxContentRunes**: The truncation limit of 4000 Unicode runes applied to content fields in infobox summaries and result excerpts to keep output within LLM context-window budgets.

## Relationships

- **SearXNGSearcher** is configured by **Config** (URL + timeout + optional HTTP client) and executes a search via **SearchArgs**, returning a **SearchResponse**.
- **SearchResponse** contains zero or more **SearchResult**s, **Answer**s, **Infobox**es, **Suggestion**s, and **UnresponsiveEngines** entries.
- **Answer**s are **Deduplicate**d against **Infobox** content to remove overlapping DuckDuckGo Wikipedia summaries.
- **setBrowserHeaders** is applied by **SearXNGSearcher** to every HTTP request made during `performSearch`.
- **SearXNGSearcher**'s `performSearch` falls back to a **GET Fallback** when the SearXNG instance rejects the initial POST with 405 or 501.
- **CLI Mode** and **MCP Mode** are the two mutually exclusive operation modes — the program runs CLI mode when any arguments are present, MCP mode otherwise.
- **Debug Mode** gates the exposure of **UnresponsiveEngines** in JSON output and enables verbose HTTP logging.
- **CLIFlags** maps to **SearchArgs** fields plus mode-selection flags (--json, --help, --version, --debug, --searxng-url).
- **ValidationError** is returned by `ValidateSearchArgs`, which pre-checks all **SearchArgs** before any HTTP request.
- **SearXNGError** wraps HTTP-level failures from the SearXNG service; **HTMLResponseError** is a specific case returned when HTML is received instead of JSON.
- **Config** reads the SearXNG URL from **CLIFlags** or the `SEARXNG_URL` environment variable, falling back to the default URL constant.

## Example dialogue

> **Dev:** "Why did the CLI mode search return HTML instead of JSON?"
>
> **Domain expert:** "That's an **HTMLResponseError** — the SearXNG instance at that **Config** URL doesn't have `format=json` enabled. Check the server's settings. The **SearXNGSearcher** tries a **POST** first, and only falls back to a **GET** if it gets a 405 **GET Fallback**, but if the response comes back as HTML, it's not a method issue — it's the SearXNG config."
>
> **Dev:** "And I see duplicate answers in the output for DuckDuckGo queries. What's that about?"
>
> **Domain expert:** "The **Deduplicate** function handles that. DuckDuckGo puts the same Wikipedia snippet in both the **Answer** and the **Infobox**. The dedup checks if the answer text is a prefix of any infobox content and filters it out."
>
> **Dev:** "I ran a query with `--debug` and saw my search query logged in plain text. Is that safe?"
>
> **Domain expert:** "**Debug Mode** logs the full HTTP request body, so yes — avoid sensitive queries. Also note that `--debug` gates the **UnresponsiveEngines** field in JSON output; without it you won't see which engines failed."
>
> **Dev:** "The server at `search.internal` uses plain HTTP. I got a warning about non-private hosts, but `192.168.1.50` doesn't get one. Why?"
>
> **Domain expert:** "That's **Private Host Detection**. The code classifies literal IP addresses and known private hostname suffixes (10.x, 172.16-31, 192.168, localhost, `*.local`, `*.internal`, IPv6 unique-local, etc.); it does not perform DNS resolution. If the host is private, the HTTP warning is suppressed because local networks are trusted. For a hostname like `search.internal`, it's recognized by its TLD suffix."

## Flagged ambiguities

1. **"Answer" ambiguity**: The term `Answer` refers both to the Go struct (with `Answer`, `Engine`, `Template` fields) and to the top-level `answers` array in the SearXNG JSON response. Additionally, SearXNG documentation mentions a legacy `{"answer": "..."}` result format that is a flat string, which is distinct from the typed struct used in this project.

2. **"Searcher" vs "SearXNGSearcher"**: The broader term "Searcher" appears in function-level docstrings and developer discussion but is not a separate exported type — only `SearXNGSearcher` exists in code. Its `performSearch` helper is an unexported method on `SearXNGSearcher`, with `Search` as the public entry point.

3. **`NumberOfResults` post-processing**: SearXNG may return `number_of_results = 0` even when results exist. The code silently corrects this by setting it to `len(results)` when the field is zero but results are non-empty. Consumers cannot rely on this field being an unmodified pass-through from SearXNG.

4. **`corrections` explicitly excluded**: The SearXNG response includes a `corrections` string array (spelling corrections from search engines), but this field is intentionally absent from `SearchResponse` per ADR-005 — it is not exposed in any output mode.

5. **`UnresponsiveEngines` conditional exposure**: This field is always captured in the Go struct but is only serialized to JSON when `Debug` is true. In CLI text mode, it is never printed to stdout — it is only logged via `slog.Debug` (visible in stderr with debug logging).
