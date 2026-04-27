# SearXNG MCP Server

A Model Context Protocol (MCP) server providing web search via the SearXNG meta-search engine.

## Purpose

AI agents (like Hermes) call this server to perform web searches without direct internet access. The server proxies requests to a SearXNG instance and returns formatted results.

## Quick Start

```bash
# Build
go build -o searxng-mcp-go .

# Run tests
go test ./...

# Execute (stdio transport - invoked by MCP client host)
./searxng-mcp-go

# Test with MCP Inspector
npx @modelcontextprotocol/inspector ./searxng-mcp-go
```

## Project Structure

```
searxng-mcp-go/
├── main.go              # Main server implementation (CLI + MCP entry points)
├── search.go            # Search functionality, HTTP client, SearXNGSearcher
├── errors.go            # Error types and handling
├── format.go            # Output formatting
├── validation.go        # Input validation
├── constants.go         # Size limits and configuration constants
├── main_test.go         # Main tests (CLI/MCP mode, runCLIMode)
├── search_test.go       # Search tests (isPrivateHost, validateBaseURL, getDefaultHTTPClient)
├── errors_test.go       # Error type tests
├── format_test.go       # Formatting tests (including pagination)
├── validation_test.go   # Validation edge case tests
├── concurrency_test.go  # Concurrency and stress tests
├── error_path_test.go   # Error path coverage tests
├── bench_test.go        # Benchmark tests
├── mcp_tool_test.go     # MCP tool integration tests
├── golden_capture_test.go # Golden file/capture tests
├── README.md            # Project README
├── go.mod/go.sum        # Go module/dependencies
├── .golangci.yml        # Linter configuration
├── codecov.yml          # Code coverage configuration
├── .env.example         # Environment variable template
├── .github/workflows/   # CI: lint, security, test
└── docs/
    ├── INSTALL.md           # Installation, build, configuration
    ├── MCP_TOOLS.md         # MCP tool documentation
    ├── MCP_TESTING.md       # MCP testing guide
    ├── OUTPUT_FORMAT.md     # Output format specification
    ├── AI_UX_TEST_GUIDE.md  # AI UX testing guide
    ├── LANGUAGE_PARAMETER_RESEARCH.md  # Language parameter research
    ├── SEARXNG_RESPONSE_FIELDS.md  # SearXNG response fields reference
    ├── SEARXNG_ANSWER_DEDUP.md     # Answer deduplication design
    ├── SEARXNG_TEST_QUERIES.md     # Test queries reference
    ├── REPORT_PERF_2026-04-19.md   # Performance report
    └── adr/
        ├── 001-no-pgo.md                      # ADR: No PGO optimization
        ├── 003-http-warning-for-non-private-hosts.md  # ADR: HTTP warning for non-private hosts
        ├── 004-mcp-stdin-env-only.md          # ADR: MCP stdin mode env-only
        ├── 005-no-corrections.md              # ADR: No corrections exposure
        └── 006-unresponsive-engines-debug-only.md  # ADR: unresponsive_engines debug-only
```

## MCP Tools

### `search`

Search the web via SearXNG.

| Parameter    | Type   | Required | Default | Description                              |
|--------------|--------|----------|---------|------------------------------------------|
| `query`      | string | Yes      | -       | Search query string                      |
| `language`   | string | No       | auto    | Language code (en, zh-tw, ja, etc.); empty = SearXNG decides |
| `safesearch` | int    | No       | 0       | 0=Off, 1=Moderate, 2=Strict              |
| `time_range` | string | No       | -       | day, month, year                         |
| `categories` | string | No       | -       | Comma-separated categories               |
| `engines`    | string | No       | -       | Comma-separated engines                  |
| `pageno`     | int (nullable) | No       | 1       | Page number (≥1)，*int；nil 時不發送參數 |

See [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) for full details.

## Configuration

Default SearXNG URL: `https://search-4.xlion.dev`

**ENV Naming Convention ⚠️**

Environment variable names should be neutral. **Only the SearXNG server URL variable may contain `searxng`** (e.g. `SEARXNG_URL`). All other functional ENV vars must NOT use the `SEARXNG_` prefix.

```bash
# Environment variable
export SEARXNG_URL=https://your-searxng-instance.example.com

# Command-line flag (overrides env var)
./searxng-mcp-go -searxng-url=https://your-searxng-instance.example.com

# JSON output
./searxng-mcp-go "search query" --json
```

Priority: command-line flag > environment variable > default

See [docs/INSTALL.md](docs/INSTALL.md) for full configuration details.

## Debug Mode

Enable verbose HTTP request/response logging for troubleshooting SearXNG communication.

```bash
# CLI flag
./searxng-mcp-go "query" --debug

# Environment variable
export DEBUG=1
./searxng-mcp-go "query"
```

Debug output includes: HTTP method, URL, Content-Type, Accept header, request body, response status, content-type, and response body preview (first 500 chars). On error responses, debug mode also logs `body_size` and a `body_preview`. Additionally, the `unresponsive_engines` field (listing engines that failed to respond, e.g., rate-limited or CAPTCHA) is **only included in the JSON response when debug mode is enabled**; it is omitted entirely in non-debug mode (see ADR-006).

## HTTP Headers & Bot Detection

SearXNG's `limiter` (enabled when `server.limiter: true` or `public_instance: true`) blocks non-browser requests via header validation. Each filter returns 429 on failure:

| Filter | Condition |
|--------|-----------|
| `http_user_agent` | UA must not match bot regex (curl, wget, Go-http-client, Python, etc.) |
| `http_accept` | Must contain `text/html` |
| `http_accept_language` | Must be non-empty |
| `http_accept_encoding` | Must contain `gzip` or `deflate` |
| `http_sec_fetch` | Sec-Fetch-Mode must be `navigate` or `cors` (HTTPS only) |
| `ip_limit` | `format=json` in URL query triggers 4 requests/hour limit |

**Additional protections**: `link_token` (forced on for `public_instance`) requires browser CSS challenge — non-browser clients accumulate in a suspicious IP counter (3 requests/30 days → 302 redirect). This cannot be bypassed with headers alone.

Our headers are set via `setBrowserHeaders()` in `search.go`. POST and GET fallback share the same function.

## Error Handling

Validation errors (all returned as `validation error on "<field>": <message>`):

- **query**: empty or whitespace-only → `search query cannot be only whitespace`
- **query**: exceeds 500 characters → `must be 500 characters or less`
- **query**: contains control characters (U+0000–U+001F, U+007F) → `contains invalid control characters`
- **time_range**: not one of day/month/year → `must be one of day, month or year`
- **safesearch**: not in 0–2 range → `must be 0 off, 1 moderate, or 2 strict`
- **categories**: contains invalid identifier (only `[a-z0-9_-]`, max 50 chars each) → `contains invalid category`
- **engines**: contains invalid identifier (only `[a-z0-9_-]`, max 50 chars each) → `contains invalid engine`
- **language**: set to `"auto"` → `must be a valid language code (e.g., en, zh-tw, ja, en-US)` (use empty string to let SearXNG decide)
- **language**: non-BCP47 pattern or >35 chars → `must be a valid language code (e.g., en, zh-tw, ja, en-US)`
- **pageno**: < 1 → `must be >= 1`

Runtime errors:

- Network/connectivity failures
- SearXNG API errors (non-200, malformed JSON, HTML responses)
- HTML responses use the fixed message `searxng returned html instead of json - json output may not be enabled on the server`
- Malformed JSON is reported as `searxng error (status N): failed to parse JSON response: ...`

## Known Limitations

1. **Pagination**: SearXNG API starts at page 1 (server validates `pageno >= 1`)
2. **HTML Detection**: Returns `HTMLResponseError` if SearXNG returns HTML instead of JSON
3. **Content Length**: Summaries truncated to 4000 UTF-8 runes

## Development

```bash
go build -o searxng-mcp-go .  # Build
go mod tidy                   # Tidy dependencies
go fmt ./...                  # Format code
go test ./...                 # Run tests
```

See [docs/INSTALL.md](docs/INSTALL.md) for Docker and other build options.

## Code Cleanliness

**No junk files allowed** ⚠️

- Forbidden: `.bak`, `.test` (compiled binaries), `*~`, `.swp`, `.swo`, and any temp/backup files
- These files **must** be deleted before committing, and permanently excluded from git history (already cleaned with `--force --invert-paths`)
- `.gitignore` already has rules for `*.bak`, `*.test`, `*.swp`, `*.swo` to prevent tracking

## Review & QA Workflow

**Report Guidelines** ⚠️

- All code reviews, AGENTS.md reviews, test coverage analysis, etc. must be written to `REPORT.md` (project root)
- `REPORT.md` is permanently in `.gitignore` — **never commit**
- After each task, generate a corresponding TODO list in `REPORT.md`
- TODO list must contain all planning information needed by the orchestrating agent

## Project Rules

- Do not modify `.gitignore` unless explicitly asked
- Do not change MCP handler's User-Agent header
- GitHub Actions `uses:` must pin to SHA with `# vX.Y.Z` version comment
- CI `go-version` must use a fixed version, not `stable`; step/job names must not contain version numbers
- MCP stdin mode does not accept CLI args — use env vars only (see `docs/adr/004-mcp-stdin-env-only.md`)
- All documentation (`docs/*.md`) must be in English
- Edit files with `patch` (find-and-replace), not `sed`; new files with `write_file`
- Subagent code changes must be verified by compiling and running tests before committing
- **Critical: Never trust your own knowledge of version numbers, release dates, or specification statuses.** Any information that is time-sensitive (language versions, dependency versions, API stability, RFC status, etc.) MUST be verified via web search before being stated as fact. Training data is frozen at a cutoff date; asserting version facts without verification has repeatedly caused serious errors.
