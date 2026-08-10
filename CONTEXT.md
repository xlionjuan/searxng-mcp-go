# SearXNG MCP Server (searxng-mcp-go)

A Model Context Protocol (MCP) server and CLI tool that proxies web search requests to a SearXNG meta-search engine instance and returns formatted results.

## Language

### Core Types

**SearXNGSearcher**: The HTTP client that holds a base URL and an `*http.Client` and communicates with a SearXNG instance to execute search queries. Its `Close()` method releases owned idle HTTP connections and cancels in-flight `Search()` calls; see ADR-012.
_Avoid_: Searcher (ambiguous — appears only in docstrings, not as a separate exported type)

**Config**: The connection parameters for a SearXNG instance — a base URL, a per-request HTTP timeout duration, retry configuration (MaxRetries, RetryDelay, MaxRetryDelay), an optional custom HTTP client, and the opt-in GET Fallback flag.

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

**Deduplicate**: The process of filtering out DuckDuckGo `Answer` entries whose text overlaps with `Infobox` content (specifically, DuckDuckGo returning the same Wikipedia summary in both answers and infoboxes), using prefix matching on the first 200 characters with a lowercase fallback. Dedup runs only when at least one non-empty infobox text exists — otherwise the original answers are returned unchanged. When it runs, answers with empty `Answer` text are dropped before the engine gate; the remaining answers are checked only when `Engine == "duckduckgo"`, and non-DuckDuckGo answers with non-empty text pass through. The implementation lives in `internal/searxng/answer`.
_Avoid_: Dedup (internal function name; prefer the full term in docs)

**setBrowserHeaders**: The function that applies Chrome-like HTTP headers (User-Agent, Accept, Sec-* family, Priority) to every search request to bypass SearXNG's limiter / bot-detection mechanism.
_Avoid_: BotHeaders, StealthHeaders

**GET Fallback**: The opt-in compatibility mechanism that re-issues a failed POST search as a GET request when the SearXNG instance returns HTTP 405 (Method Not Allowed) or 501 (Not Implemented). It is disabled by default and enabled only with `SEARXNG_ALLOW_GET_FALLBACK=1` because GET sends search parameters in the URL.
_Avoid_: POSTtoGETFallback (internal test function name)

**Private Host Detection**: The RFC-grounded classification that suppresses the HTTP warning when the configured SearXNG URL points to a private/internal destination. A host is "private" iff the literal name is `localhost` or ends in `.localhost` (RFC 6761 §6.3, Special-Use Domain Names), or the literal address falls inside one of the published private/loopback/link-local/ULA/CGNAT/multicast/broadcast ranges enumerated in `docs/adr/003-http-warning-for-non-private-hosts.md`. No DNS resolution is performed, and the contract intentionally accepts that names like `printer.local` or `nas.lan` will now trigger the warning because no cited RFC reserves those suffixes for "private network" use.

**prepareMCPStdin**: The function that peeks at the first line of stdin and applies a fixed 1 MiB transport bound to the JSON first-message wire bytes, excluding an optional trailing newline delimiter, plus a safe structural MCP gate. It accepts a legacy JSON-RPC 2.0 `initialize` request, a `server/discover` request, or a stateless request carrying `params._meta["io.modelcontextprotocol/protocolVersion"]`; complete protocol metadata validation remains with the MCP SDK. This prevents the MCP server from hanging when piped non-MCP input.

### Constants

**MaxContentRunes**: The truncation limit of 4000 Unicode runes applied by the **CLI text formatter** (`formatResults` in `format.go`) to the `content` field of search results and infoboxes. It is purely a **rendering budget for terminal output** — JSON mode and MCP mode return the full un-truncated normalized response, so downstream JSON/MCP consumers always see the complete upstream text. The rune-safe truncation itself is shared with the searxng deduplication prefix match through the `searxng.TruncateRunes` helper in `internal/searxng/truncate.go`. See `docs/adr/011-max-content-runes-cli-only.md` for the scope decision.

## Relationships

- **SearXNGSearcher** is configured by **Config** (URL + timeout + optional HTTP client) and executes a search via **SearchArgs**, returning a **SearchResponse**.
- **SearchResponse** contains zero or more **SearchResult**s, **Answer**s, **Infobox**es, **Suggestion**s, and **UnresponsiveEngines** entries.
- **Answer**s are **Deduplicate**d against **Infobox** content to remove overlapping DuckDuckGo Wikipedia summaries; answers from non-DuckDuckGo engines are not checked.
- **setBrowserHeaders** is applied by **SearXNGSearcher** to every HTTP request made during search execution.
- **SearXNGSearcher** falls back to a **GET Fallback** only when the SearXNG instance rejects the initial POST with 405 or 501 and **Config** enables `AllowGETFallback`.
- **SearXNGSearcher.Close** cancels active **SearchArgs** executions through an internal shutdown signal, so shutdown can return in-flight searches with `context.Canceled` instead of waiting for upstream completion.
- **CLI Mode** and **MCP Mode** are the two mutually exclusive operation modes — the program runs CLI mode when any arguments are present, MCP mode otherwise.
- **Debug Mode** gates the exposure of **UnresponsiveEngines** in JSON output and enables verbose HTTP logging.
- **CLIFlags** maps to **SearchArgs** fields plus mode-selection flags (--json, --help, --version, --debug, --searxng-url).
- **ValidationError** is returned by `ValidateSearchArgs`, which pre-checks all **SearchArgs** before any HTTP request.
- **SearXNGError** wraps HTTP-level failures from the SearXNG service; **HTMLResponseError** is a specific case returned when HTML is received instead of JSON.
- **Config** reads the SearXNG URL from **CLIFlags** or the `SEARXNG_URL` environment variable, returning an error (`SEARXNG_URL is required`) if neither is set. It also reads `SEARXNG_ALLOW_GET_FALLBACK=1` as an explicit opt-in for **GET Fallback**.

## Example dialogue

> **Dev:** "Why did the CLI mode search return HTML instead of JSON?"
>
> **Domain expert:** "That's an **HTMLResponseError** — the SearXNG instance at that **Config** URL doesn't have `format=json` enabled. Check the server's settings. The **SearXNGSearcher** tries a **POST** first. If POST is rejected with 405 or 501, fix the reverse proxy or explicitly enable **GET Fallback** with `SEARXNG_ALLOW_GET_FALLBACK=1`; an HTML response is not a method issue — it's the SearXNG config."
>
> **Dev:** "And I see duplicate answers in the output for DuckDuckGo queries. What's that about?"
>
> **Domain expert:** "The **Deduplicate** function handles that. DuckDuckGo puts the same Wikipedia snippet in both the **Answer** and the **Infobox**. When non-empty infobox content exists, empty answers are dropped first; remaining answers are checked only if they come from the `duckduckgo` engine — if the answer text is a prefix of any infobox content, it's filtered out. Non-empty non-DuckDuckGo answers pass through."
>
> **Dev:** "I ran a query with `--debug` and saw my search query logged in plain text. Is that safe?"
>
> **Domain expert:** "**Debug Mode** logs the full HTTP request body, so yes — avoid sensitive queries. Also note that `--debug` gates the **UnresponsiveEngines** field in JSON output; without it you won't see which engines failed."
>
> **Dev:** "The server at `192.168.1.50` uses plain HTTP. There's no warning, but the same instance at `search.example.com` does warn. And I just noticed that `printer.local` also warns now — it used to be quiet. Why?"
>
> **Domain expert:** "That's **Private Host Detection**. The HTTP warning is suppressed only for RFC-grounded private destinations: literal `localhost` / `*.localhost` (RFC 6761) and literal addresses inside the private/loopback/link-local/ULA/CGNAT/multicast/broadcast ranges listed in ADR-003. There is no DNS resolution and no private-TLD allowlist. `192.168.1.50` matches RFC 1918 and is silent; `search.example.com` is a public name and warns. `printer.local` is not considered private by this check — RFC 6762 defines `.local` as a multicast-DNS link-local namespace, not a private-network indicator — so it now triggers the warning too. If you don't want the warning for a LAN-only host, either bind SearXNG to a literal RFC 1918 address or put TLS in front of it."

## Flagged ambiguities

1. **"Answer" ambiguity**: The term `Answer` refers both to the Go struct (with `Answer`, `Engine`, `Template` fields) and to the top-level `answers` array in the SearXNG JSON response. Additionally, SearXNG documentation mentions a legacy `{"answer": "..."}` result format that is a flat string, which is distinct from the typed struct used in this project.

2. **"Searcher" vs "SearXNGSearcher"**: The broader term "Searcher" appears in function-level docstrings and developer discussion but is not a separate exported type — only `SearXNGSearcher` exists in code. `Search` is the public entry point for executing a search.

3. **`NumberOfResults` post-processing**: SearXNG may return `number_of_results = 0` even when results exist. The code silently corrects this by setting it to `len(results)` when the field is zero but results are non-empty. Consumers cannot rely on this field being an unmodified pass-through from SearXNG.

4. **`corrections` explicitly excluded**: The SearXNG response includes a `corrections` string array (spelling corrections from search engines), but this field is intentionally absent from `SearchResponse` per ADR-005 — it is not exposed in any output mode.

5. **`UnresponsiveEngines` conditional exposure**: This field is always captured in the Go struct but is only serialized to JSON when `Debug` is true. In CLI text mode, it is never printed to stdout — it is only logged via `slog.Debug` (visible in stderr with debug logging).

6. **Unknown engines/categories silently ignored by SearXNG**: When `engines` or `categories` contains unknown names, SearXNG silently falls back to defaults (`wikipedia` for engines, `general` for categories) and returns results with `warnings: []` — no error or warning is emitted. The MCP server has no way to detect this, so consumers should not treat the presence of results as confirmation that a requested engine or category was used.
