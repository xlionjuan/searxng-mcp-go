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
├── golden_capture_test.go  # Golden file/capture tests
├── e2e_error_test.go       # End-to-end error handling tests (build tag e2e)
├── e2e_exitcode_test.go    # End-to-end exit code tests (build tag e2e)
├── e2e_functional_test.go  # End-to-end functional tests (build tag e2e)
├── e2e_mcp_test.go         # End-to-end MCP tests (build tag e2e)
├── e2e_stress_test.go      # End-to-end stress tests (build tag e2e)
├── testhelpers_test.go   # Test helper functions
├── README.md             # Project README
├── go.mod                # Go module definition
├── go.sum                # Go dependency checksums
├── .golangci.yml         # Linter configuration
├── codecov.yml           # Code coverage configuration
├── .env.example          # Environment variable template
├── .goreleaser.yaml      # GoReleaser release config
├── justfile              # Common task runner commands
├── testdata/             # Test fixtures (sample JSON responses)
├── .github/workflows/    # CI: test.yml, lint.yml, security.yml, e2e.yml, release.yml
├── .github/renovate.json # Renovate dependency update config
├── internal/
│   └── searxng/          # SearXNG client library
│       ├── client.go     # HTTP client creation, redirect policy, validateBaseURL
│       ├── constants.go  # Size limits and configuration constants
│       ├── deduplicate.go # Answer deduplication against infobox content
│       ├── errors.go     # Error types and handling
│       ├── params.go     # Shared search parameter definitions for CLI/MCP
│       ├── request.go    # buildSearchRequest, setBrowserHeaders
│       ├── response.go   # parseSearchResponse, normalizeResponse
│       ├── retry.go      # Retry logic: backoff, jitter, retryable checks
│       ├── searcher.go   # SearXNGSearcher, Search, search execution and retry flow
│       ├── searcher_internal_test.go    # Internal searcher tests
│       ├── searcher_test.go             # Searcher tests (white-box)
│       ├── search_execution_test.go     # doSearchAttempt / GETfallback / finishResponse tests
│       ├── search_http_internal_test.go # HTTP search request tests
│       ├── search_test_helpers_test.go  # Internal test helpers
│       ├── types.go      # SearchArgs, SearchResponse, SearchResult, Answer, Infobox, Config
│       ├── validation.go # Search argument validation
│       ├── bench_test.go            # Internal benchmarks (marshal, validation)
│       ├── client_internal_test.go  # Internal HTTP client tests
│       ├── deduplicate_internal_test.go # Internal deduplication tests
│       ├── errors_internal_test.go      # Internal error handling tests
│       ├── handle_nonok_test.go         # handleNonOKResponse tests
│       ├── request_internal_test.go     # Internal request tests
│       ├── response_internal_test.go    # Internal response parsing tests
│       ├── retry_internal_test.go       # Internal retry logic tests
│       ├── types_internal_test.go       # Internal type tests
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
| Pull-request agent workflow | [docs/agents/pull-requests.md](docs/agents/pull-requests.md) |

## Code Cleanliness

- Remove junk files generated by your own work before committing: `.bak`, `.test` (compiled binaries), `*~`, `.swp`, `.swo`, and temp/backup files. If an untracked temp/backup file may belong to a user, ask before deleting it.
- `.gitignore` already covers `*.bak`, `*.test`, `*.swp`, `*.swo`, `*.out`, `REPORT.md`, `.env`, and `searxng-server-test/.venv/`; `*~` files are not ignored and must still be removed manually

## Local Review & QA Reports

- `REPORT.md` is a local-only scratch report for ad hoc repository reviews, AGENTS.md reviews, test coverage analysis, and similar local QA work.
- `REPORT.md` is permanently in `.gitignore` — **never commit**.
- Do not create or update `REPORT.md` for normal issue-fix, CI-fix, implementation, or PR-only work unless the user explicitly asks for a local review/report.
- Independent PR agents should ignore `REPORT.md` and put their summary, test evidence, and open questions in the PR body or GitHub comments instead.
- When using `REPORT.md`, append or update a task-specific section for each new review; do not overwrite previous reports.
- TODO lists in reports must be concise and contain only unfinished or handoff-relevant work.

## Project Rules

### Do Not Change
- Do not modify `.gitignore` unless explicitly asked
- Do not change MCP handler's User-Agent header

### CI & Release
- GitHub Actions `uses:` must pin to SHA with `# vX.Y.Z` version comment
- CI `go-version` must use a fixed version, not `stable`; step/job names must not contain version numbers
- MCP stdin mode does not accept CLI args — use env vars only (see `docs/adr/004-mcp-stdin-env-only.md`)

### E2E Tests
- **Core functional tests assert non-zero results.** Tests that validate search results (`basic_search`, `response_structure`, `all_safesearch_levels`, `paginations`, `limit_boundaries`, `parameter_combinations`, `unicode_and_special_characters`) use strict `len(Results) > 0` checks. If the live server returns only an infobox with 0 web results, those tests log a `t.Logf` warning instead of failing, and the warnings are collected in a `WARNING SUMMARY` block at the end of each test group (`TestMCPFunctional`, `TestMCPStdioE2E`).
- **Exceptions are owner-approved and documented.** Known limitations of the CI SearXNG instance (e.g. `files` category removed, `all_time_ranges` day/month/year relaxed, `pageno+limit` warning-only) have explicit owner approval. Do not add new leniency without approval.
- **Adding new strict assertions is preferred.** When a test currently tolerates empty results but could meaningfully assert non-zero results (e.g. with a different query or parameter combination), prefer tightening over leaving it lenient.

### Version Bump & Release Workflow
**Tag style:** `v{major}.{minor}.{patch}` (e.g. `v1.0.4`, `v1.1.0`). No `-beta`, `-rc`, or other suffixes.

Run this workflow only when the user explicitly asks for a release and the target version is known or confirmed.

**To release a new version:**
1. Patch the `version` constant in `main.go` to the new version
2. Commit: `git commit -m "chore: bump version to vX.Y.Z"` (note: use `-m` flag, do not invoke interactive editor)
3. Create annotated tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
4. Push both: `git push origin main` and `git push origin vX.Y.Z` — this triggers the `release.yml` GoReleaser workflow

**GoReleaser outputs (`.goreleaser.yaml`):**
- Archives: `tar.zst` format, `linux/amd64` + `linux/arm64` (static, `CGO_ENABLED=0`)
- Name template: `searxng-mcp-go_v{Version}_{Os}_{Arch}`
- Contents: binary + README.md + LICENSE + checksums file
- GitHub Release (auto prerelease detection, `make_latest: true`)
- Homebrew tap auto-update to `xlionjuan/homebrew-tap`
- Changelog auto-generated from commits (excludes `docs:`, `test:`, `chore:` prefixes)

**Version injection:** GoReleaser's ldflags (`-X main.version=v{{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}`) override the hardcoded `main.go` value at build time. The hardcoded value is for `--version` flag when not built by GoReleaser.

### Documentation
- All docs (`docs/*.md`, README, CONTEXT, AGENTS) must be in English
- Pull requests must update related documentation in the same branch when behavior, configuration, CLI/MCP parameters, output format, test workflow, release workflow, domain terminology, or ADR-governed decisions change. If no docs need updates, state that explicitly in the PR body.

### Editing
- Use patch-style edits for existing files; avoid `sed -i` / ad hoc rewrites
- For new files, use an available file-edit/write primitive rather than shell heredocs or command-output redirection.

### Verification
- Code changes must be verified with the narrowest meaningful build/test commands before committing. Broaden verification for shared behavior, public interfaces, test infrastructure, or release/CI changes.
- PR agents must run local verification themselves before opening or updating a PR; do not rely on CI or reviewers as the first validation pass. For Go code, test, CI, or script changes, the minimum local gate is `go test ./...` plus `golangci-lint run ./...`.
- If `golangci-lint` is unavailable in the local environment, run `go vet ./...` as the fallback static check and state in the PR body that the linter itself could not be run.
- If a subagent made code changes, the coordinating agent must review and verify those changes before committing.
- Never trust your own knowledge of version numbers, release dates, or specification statuses. Time-sensitive external facts (language versions, dependency versions, API stability, RFC status, etc.) must be verified against a current authoritative source before being stated as fact; if that verification is unavailable, avoid making the claim or state the verification gap.

### GitHub Operations
- **🔴 GitHub API operations MUST use `gh` CLI** — via terminal, always. **Absolutely NO browser tools** (browser_navigate, browser_vision, etc.) for GitHub — not for Actions, not for PRs, not for anything on github.com.

### PR Title and Body Language
- **PR title and body must be in English**, even if the user originally discussed the change in another language (e.g. Chinese, Japanese). This applies to all PR agents in this repo, including OpenCode, Aider, Codex, and any other coding-agent that opens or updates a PR here.
- The English requirement covers the PR **title** and **PR body** (the persistent, searchable record of the change). When opening or updating a PR, translate the relevant discussion material into English for the body — do not copy non-English prose verbatim.
- GitHub **PR comments, issue comments, and review replies are exempt** from this rule. Reply in whatever language the user is using. The English-only rule applies only to the PR title and body.
- This rule does not change the `### Documentation` rule above — repo docs (`docs/*.md`, README, CONTEXT, AGENTS) remain English-only, and PR body English is in addition to that.

### Git Identity
- Agents must not run `git config user.name`, `git config user.email`, `git config --global user.name`, or `git config --global user.email` in this repo unless the user explicitly asks for that exact operation.
- If `git commit` fails because author identity is missing, stop and report the failure. Do not invent or set a fallback identity.
- Do not use generic or unverified GitHub noreply identities such as `<tool>@users.noreply.github.com` for commits. Never derive a commit email from a tool name, action name, repository name, or package name unless that exact identity is verified. Automated workflows that create commits must use an explicit, reviewed bot/app identity owned by the workflow provider or this repo owner.
- When editing GitHub Actions or agent workflows that can commit, review the workflow/action documentation or source for the exact commit author and committer identity. Treat unknown, generic, or user-name-derived noreply identities as a bug to fix before enabling the workflow.
- If testing Git behavior around missing identity, use an isolated temporary repository and do not change global or repo-local config for this project.

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

### Local SearXNG Test Server

For E2E and integration testing, a local SearXNG dev server is set up under `searxng-server-test/`. The `just` recipes in the root `justfile` are the primary user-facing interface; the underlying shell scripts are implementation detail.

```bash
# Ensure submodule is initialized (one-time)
git submodule update --init --depth 1 searxng-server-test/searxng

# One-time setup (creates venv, installs deps, generates settings.yml)
just test-server-setup

# Day-to-day
just test-server-start     # background, waits for readiness on :8888
just test-server-status    # live / degraded / stale / dead / orphan
just test-server-logs      # tail -f searxng-server-test/searxng.log
just test-server-stop      # SIGTERM (5s) → SIGKILL fallback
just test-server-restart   # stop + start
just test-server-start-fg  # FOREGROUND — blocks the calling shell; humans only
```

Underlying scripts (rarely needed directly):

| Script | Purpose |
|---|---|
| `searxng-server-test/00-setup.sh` | One-time: venv + deps + `settings.yml` (re-run to reset) |
| `searxng-server-test/01-start-bg.sh` | Background start (detached, PID-tracked, readiness polled) |
| `searxng-server-test/01-start-fg.sh` | Foreground start — **do not use from agents/CI** |
| `searxng-server-test/02-stop.sh` | Stop background instance; `--force` kills orphans |
| `searxng-server-test/03-status.sh` | Report live/stale/dead/orphan state |

**Key details:**
- Server runs on `http://127.0.0.1:8888` — set `SEARXNG_URL=http://127.0.0.1:8888` before running E2E tests
- `01-start-bg.sh` polls until the server responds (up to 30s), then exits; the server continues in the background, detached from the calling shell via `nohup`/`disown`
- `settings.yml` is auto-generated from SearXNG repo defaults — enables JSON format, yahoo/bing/ddg-definitions engines, and generates a random secret key
- Background state is tracked in two files (kept out of git via `.gitignore`):
  - `searxng-server-test/.bg-pid` — PID of `searx/webapp.py`
  - `searxng-server-test/searxng.log` — stdout/stderr of the background process
- The log file is the first place to look when an E2E test fails (`just test-server-logs`)
- If `.bg-pid` is missing or stale, `pgrep -f 'searx/webapp.py'` finds the real PID
- `00-setup.sh` cold-run on a clean machine takes ~2–3 min (uv venv + pip install + searxng editable install). Subsequent runs are ~30s only if the venv already exists and is intact
- Starting the server a second time without stopping it first is rejected with a clear error message — use `just test-server-restart` instead

**Pitfalls:**
- `searxng-server-test/searxng/` is a **git submodule** registered in the root `.gitmodules` — do NOT `git init` or `git submodule add` inside `searxng-server-test/`, its state is managed by the parent repo
- Do NOT use `searx/limiter.toml` from production template — the default settings.yml has `limiter: false`
- The repo default settings.yml has `valkey.url: false` (disabled); do not use `utils/templates/etc/searxng/settings.yml` which enables valkey
- When testing with granian (not needed for dev), must pass `--interface wsgi`
- **Never run `01-start-fg.sh` from an agent or CI context** — it runs SearXNG in the foreground and blocks the calling shell until killed. Use `just test-server-start` (background) instead
- If E2E tests fail with a connection/timeout error, check `searxng-server-test/searxng.log` for SearXNG-side stack traces before assuming the test is broken
- `.bg-pid` can go stale (e.g. agent shell killed → PID file lost, but searx process lives on). Recovery: `pgrep -f 'searx/webapp.py'` to inspect, then `just test-server-stop` (with `--force` if the PID file is gone)

## Agent skills

### Issue tracker

Issues are tracked on GitHub. See `docs/agents/issue-tracker.md`.

### Pull requests

Pull-request agents must use `gh` for GitHub operations, must not touch `REPORT.md` unless explicitly asked, and must update related docs with code changes. See `docs/agents/pull-requests.md`.

### Triage labels

Issue labels are defined in GitHub and documented in `docs/agents/triage-labels.md`. Always read the current label list with `gh label list` before label-sensitive work. Labels `accepted`, `needs-explain`, and `rejected` are human-only decision labels; agents may recommend them but must not apply, remove, or change them unless explicitly instructed.

### Domain docs

Single-context — one `CONTEXT.md` + `docs/adr/` at repo root. See `docs/agents/domain.md`.
