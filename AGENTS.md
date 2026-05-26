# SearXNG MCP Server

A Model Context Protocol (MCP) server providing web search via the SearXNG meta-search engine.

## Purpose

AI agents (like Hermes) call this server to perform web searches without direct internet access. The server proxies requests to a SearXNG instance and returns formatted results.

## Project Structure

```
searxng-mcp-go/
├── main.go              # Entry point: main(), parseArgs(), CLIFlags, getConfig()
├── cli.go               # CLI mode: printCLIHelp(), runCLIMode()
├── mcp.go                # MCP mode: runMCPMode(), prepareMCPStdin(), NewSearchToolHandler(), etc.
├── format.go            # Output formatting
├── main_test.go         # Main tests (CLI/MCP mode, runCLIMode)
├── search_test.go       # Search tests (SearXNGSearcher, DeduplicateAnswers)
├── errors_test.go       # Error type tests
├── format_test.go       # Formatting tests (including pagination)
├── concurrency_test.go  # Concurrency/stress tests (-tags=stress)
├── error_path_test.go   # Error path coverage tests
├── fuzz_test.go         # Fuzz tests
├── bench_test.go        # Benchmark tests
├── mcp_tool_test.go     # MCP tool tests
├── golden_capture_test.go # Golden file/capture tests
├── e2e_mcp_test.go       # End-to-end MCP tests (build tag e2e)
├── testhelpers.go       # Test helper functions
├── README.md            # Project README
├── go.mod               # Go module definition
├── go.sum               # Go dependency checksums
├── .golangci.yml        # Linter configuration
├── codecov.yml          # Code coverage configuration
├── .env.example         # Environment variable template
├── .goreleaser.yaml     # GoReleaser release config
├── justfile             # Common task runner commands
├── testdata/            # Test fixtures (sample JSON responses)
├── .github/workflows/   # CI: test.yml, lint.yml, security.yml, e2e.yml, release.yml
├── .github/renovate.json # Renovate dependency update config
├── internal/
│   └── searxng/         # SearXNG client library
│       ├── client.go    # HTTP client creation, redirect policy, validateBaseURL
│       ├── constants.go # Size limits and configuration constants
│       ├── deduplicate.go # Answer deduplication against infobox content
│       ├── errors.go    # Error types and handling
│       ├── request.go   # buildSearchRequest, setBrowserHeaders
│       ├── response.go  # parseSearchResponse, normalizeResponse
│       ├── retry.go     # Retry logic: backoff, jitter, retryable checks
│       ├── searcher.go  # SearXNGSearcher, Search, performSearch
│       ├── types.go     # SearchArgs, SearchResponse, SearchResult, Answer, Infobox, Config
│       ├── validation.go # Search argument validation
│       ├── bench_test.go            # Internal benchmarks (marshal, validation)
│       ├── deduplicate_internal_test.go # Internal deduplication tests
│       ├── errors_internal_test.go      # Internal error handling tests
│       ├── response_internal_test.go    # Internal response parsing tests
│       ├── retry_internal_test.go       # Internal retry logic tests
│       ├── searcher_test.go             # Searcher tests
│       └── validation_test.go           # Validation tests
└── docs/
    ├── INSTALL.md           # Installation, build, configuration
    ├── MCP_TOOLS.md         # MCP tool documentation
    ├── MCP_TESTING.md       # MCP testing guide
    ├── OUTPUT_FORMAT.md     # Output format specification
    ├── SEARXNG_ANSWER_DEDUP.md     # Answer deduplication design
    ├── SEARXNG_BOT_DETECTION.md    # SearXNG limiter & bot detection
    ├── SEARXNG_RESPONSE_FIELDS.md  # SearXNG response fields reference
    ├── SEARXNG_TEST_QUERIES.md     # Test queries reference
    ├── AI_UX_TEST_GUIDE.md         # AI UX testing guide (archived research)
    ├── LANGUAGE_PARAMETER_RESEARCH.md  # Language parameter research (archived)
    ├── PROMPT_INJECTION_SAFETY.md  # External content warning research (archived)
    ├── REPORT_PERF_2026-04-19.md   # Performance profiling report (archived)
    ├── research-external-content-json-boundary-marking.md  # Boundary marking research (archived)
    └── adr/
        ├── 001-no-pgo.md                      # ADR: No PGO optimization
        ├── 003-http-warning-for-non-private-hosts.md  # ADR: HTTP warning for non-private hosts
        ├── 004-mcp-stdin-env-only.md          # ADR: MCP stdin mode env-only
        ├── 005-no-corrections.md              # ADR: No corrections exposure
        ├── 006-unresponsive-engines-debug-only.md  # ADR: unresponsive_engines debug-only
        ├── 007-no-dns-rebinding.md            # ADR: No DNS rebinding
        └── 008-same-hostname-redirect.md      # ADR: Same-hostname redirect
```

## Documentation Index

Most detailed topic docs live in `docs/`; root docs (README.md, CONTEXT.md, AGENTS.md) cover project overview, domain context, and agent instructions.

| Topic | Document |
|-------|----------|
| Build, install, configuration, debug mode | [docs/INSTALL.md](docs/INSTALL.md) |
| MCP tool parameters & error reference | [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) |
| MCP testing guide | [docs/MCP_TESTING.md](docs/MCP_TESTING.md) |
| Output format (CLI + JSON) & truncation limits | [docs/OUTPUT_FORMAT.md](docs/OUTPUT_FORMAT.md) |
| SearXNG bot detection / limiter internals | [docs/SEARXNG_BOT_DETECTION.md](docs/SEARXNG_BOT_DETECTION.md) |
| SearXNG response fields reference | [docs/SEARXNG_RESPONSE_FIELDS.md](docs/SEARXNG_RESPONSE_FIELDS.md) |
| Answer deduplication design | [docs/SEARXNG_ANSWER_DEDUP.md](docs/SEARXNG_ANSWER_DEDUP.md) |
| Test queries reference | [docs/SEARXNG_TEST_QUERIES.md](docs/SEARXNG_TEST_QUERIES.md) |
| Architecture Decision Records | [docs/adr/](docs/adr/) |

## Code Cleanliness

- No junk files: `.bak`, `.test` (compiled binaries), `*~`, `.swp`, `.swo`, and temp/backup files **must** be deleted before committing
- `.gitignore` already covers `*.bak`, `*.test`, `*.swp`, `*.swo`, `*.out`, `REPORT.md`, `.env`, and `searxng-server-test/.venv/`

## Review & QA Workflow

- All code reviews, AGENTS.md reviews, test coverage analysis, etc. must be written to `REPORT.md` (project root)
- `REPORT.md` is permanently in `.gitignore` — **never commit**
- Append or update a task-specific section for each new review; do not overwrite previous reports
- TODO list must be concise and contain only unfinished or handoff-relevant work

## Project Rules

### Do Not Change
- Do not modify `.gitignore` unless explicitly asked
- Do not change MCP handler's User-Agent header

### CI & Release
- GitHub Actions `uses:` must pin to SHA with `# vX.Y.Z` version comment
- CI `go-version` must use a fixed version, not `stable`; step/job names must not contain version numbers
- MCP stdin mode does not accept CLI args — use env vars only (see `docs/adr/004-mcp-stdin-env-only.md`)

### Documentation
- All docs (`docs/*.md`, README, CONTEXT, AGENTS) must be in English

### Editing
- Use patch-style edits for existing files; avoid `sed -i` / ad hoc rewrites
- Use the agent's file-write primitive for new files

### Verification
- Subagent code changes must be verified by compiling and running tests before committing
- **🔴 Critical: Never trust your own knowledge of version numbers, release dates, or specification statuses.** Time-sensitive information (language versions, dependency versions, API stability, RFC status, etc.) MUST be verified via web search before being stated as fact.

### GitHub Operations
- **🔴 GitHub API operations MUST use `gh` CLI** — via terminal, always. **Absolutely NO browser tools** (browser_navigate, browser_vision, etc.) for GitHub — not for Actions, not for PRs, not for anything on github.com.

## Build & Test Commands

Root benchmarks (format/search) live in `bench_test.go`; internal benchmarks (marshal/validation) live in `internal/searxng/bench_test.go`.

| Command | Scope |
|---------|-------|
| `go build ./...` | Build all packages |
| `go test ./...` | Run unit tests (excludes e2e, stress) |
| `go test -race -shuffle=on ./...` | CI-style test run with race detector |
| `go test -tags=stress -race ./...` | Include stress/concurrency tests |
| `go test -tags=e2e -run TestMCPStdioE2E -count=1 .` | E2E test (requires `SEARXNG_URL` + test server) |
| `golangci-lint run ./...` | Lint (CI uses v2.12.2) |
| `go vet ./...` | Static analysis |

## Agent skills

### Issue tracker

Issues are tracked on GitHub. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context — one `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.
