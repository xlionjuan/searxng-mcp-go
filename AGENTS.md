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
├── validation_test.go   # Validation edge case tests
├── concurrency_test.go  # Concurrency and stress tests
├── error_path_test.go   # Error path coverage tests
├── fuzz_test.go         # Fuzz tests
├── bench_test.go        # Benchmark tests
├── mcp_tool_integration_test.go # MCP tool integration tests
├── mcp_tool_test.go     # MCP tool tests
├── golden_capture_test.go # Golden file/capture tests
├── testhelpers.go       # Test helper functions
├── README.md            # Project README
├── go.mod               # Go module definition
├── go.sum               # Go dependency checksums
├── .golangci.yml        # Linter configuration
├── codecov.yml          # Code coverage configuration
├── .env.example         # Environment variable template
├── .github/workflows/   # CI: lint, security, test
├── internal/
│   └── searxng/         # SearXNG client library
│       ├── searcher.go  # SearXNGSearcher, NewSearXNGSearcher, Search, performSearch method, DeduplicateAnswers, HTTP client/host checks
│       ├── types.go     # SearchArgs, SearchResponse, SearchResult, Answer, Infobox, InfoboxAttribute, InfoboxURL
│       ├── errors.go    # Error types and handling
│       ├── validation.go # Search argument validation
│       └── constants.go # Size limits and configuration constants
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
    ├── SEARXNG_BOT_DETECTION.md    # SearXNG limiter & bot detection
    ├── REPORT_PERF_2026-04-19.md   # Performance report
    ├── PROMPT_INJECTION_SAFETY.md  # External content warning research
    └── adr/
        ├── 001-no-pgo.md                      # ADR: No PGO optimization
        ├── 002-missing.md                    # ADR: Skipped / never created
        ├── 003-http-warning-for-non-private-hosts.md  # ADR: HTTP warning for non-private hosts
        ├── 004-mcp-stdin-env-only.md          # ADR: MCP stdin mode env-only
        ├── 005-no-corrections.md              # ADR: No corrections exposure
        └── 006-unresponsive-engines-debug-only.md  # ADR: unresponsive_engines debug-only
```

## Documentation Index

All detailed documentation lives in `docs/`. Here's where to find what:

| Topic | Document |
|-------|----------|
| Build, install, configuration, debug mode | [docs/INSTALL.md](docs/INSTALL.md) |
| MCP tool parameters & error reference | [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) |
| MCP testing guide | [docs/MCP_TESTING.md](docs/MCP_TESTING.md) |
| Output format (CLI + JSON) & truncation limits | [docs/OUTPUT_FORMAT.md](docs/OUTPUT_FORMAT.md) |
| SearXNG bot detection / limiter internals | [docs/SEARXNG_BOT_DETECTION.md](docs/SEARXNG_BOT_DETECTION.md) |
| AI UX testing guide | [docs/AI_UX_TEST_GUIDE.md](docs/AI_UX_TEST_GUIDE.md) |
| Language parameter research | [docs/LANGUAGE_PARAMETER_RESEARCH.md](docs/LANGUAGE_PARAMETER_RESEARCH.md) |
| SearXNG response fields reference | [docs/SEARXNG_RESPONSE_FIELDS.md](docs/SEARXNG_RESPONSE_FIELDS.md) |
| Answer deduplication design | [docs/SEARXNG_ANSWER_DEDUP.md](docs/SEARXNG_ANSWER_DEDUP.md) |
| Test queries reference | [docs/SEARXNG_TEST_QUERIES.md](docs/SEARXNG_TEST_QUERIES.md) |
| Performance report | [docs/REPORT_PERF_2026-04-19.md](docs/REPORT_PERF_2026-04-19.md) |
| Prompt injection & external content safety research | [docs/PROMPT_INJECTION_SAFETY.md](docs/PROMPT_INJECTION_SAFETY.md) |
| Architecture Decision Records | [docs/adr/](docs/adr/) |

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

## Agent skills

### Issue tracker

Issues are tracked on GitHub. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-label vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context — one `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.
