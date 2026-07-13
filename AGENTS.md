# SearXNG MCP Server

A Model Context Protocol (MCP) server providing web search via the SearXNG
meta-search engine. Agents call this server to perform web searches through a
configured SearXNG instance.

## Orientation

- Entry points: `main.go`, `cli.go`, `mcp.go`, `format.go`
- Core package: `internal/searxng/`
- Answer types and deduplication: `internal/searxng/answer/`
- Shared test helpers: `internal/testhelper/`
- Unit and integration tests: root `*_test.go` plus `internal/searxng/*_test.go`
- E2E and stress tests: `e2e_*_test.go`, build tags `e2e` and `stress`
- Golden regression test: `golden_capture_test.go` is a byte-for-byte lock on
  `formatResults()` output. Any intentional formatting change requires updating
  the inline golden string in that test.
- Fixtures: `testdata/`
- CI and release workflows: `.github/workflows/`
- GitHub Actions OpenCode runtime: `docs/agents/opencode-github-actions.md`
- Domain context and terminology: `CONTEXT.md`
- Architecture decisions: `docs/adr/`

## Documentation Index

| Topic | Document |
|-------|----------|
| Local SearXNG test server | [docs/agents/test-server.md](docs/agents/test-server.md) |
| Pull-request agent workflow | [docs/agents/pull-requests.md](docs/agents/pull-requests.md) |
| Code review workflow | [docs/agents/code-review.md](docs/agents/code-review.md) |
| GitHub Actions OpenCode runtime | [docs/agents/opencode-github-actions.md](docs/agents/opencode-github-actions.md) |
| Release agent workflow | [docs/agents/release.md](docs/agents/release.md) |
| Issue tracker workflow | [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md) |
| Domain and ADR workflow | [docs/agents/domain.md](docs/agents/domain.md) |
| Architecture decisions | [docs/adr/](docs/adr/) |

## Code Cleanliness

- Remove junk files (`.bak`, `.test`, `*~`, `.swp`, `.swo`) before committing.
- `.gitignore` already covers `*.bak`, `*.test`, `*.swp`, `*.swo`, `*.out`,
  `REPORT.md`, `.env`; `*~` is not.

## Local Review Reports

- `REPORT.md` is local-only scratch (`.gitignore`d). See
  [docs/agents/local-reports.md](docs/agents/local-reports.md).

## Project Rules

### Do Not Change

- Do not modify `.gitignore` unless explicitly asked.
- Do not change the MCP handler's User-Agent header.

### CI and Release

- Run the release workflow only when the user explicitly asks for a release and
  the target version is known or confirmed. See
  [docs/agents/release.md](docs/agents/release.md).
- CI workflow rules: [docs/agents/ci.md](docs/agents/ci.md).
- In GitHub Actions OpenCode, any dirty worktree may be committed and pushed by
  the action infrastructure. For read-only requests, do not leave local changes;
  see [docs/agents/opencode-github-actions.md](docs/agents/opencode-github-actions.md).

### E2E Tests

- Core functional E2E tests assert non-zero results, with a WARNING SUMMARY
  path for live-server edge cases.
- Owner-approved exceptions must not be broadened without approval. See
  [docs/agents/e2e-tests.md](docs/agents/e2e-tests.md).
- For local server setup, use `just test-server-start`; never run
  `searxng-server-test/01-start-fg.sh` from agents or CI. See
  [docs/agents/test-server.md](docs/agents/test-server.md).

### Documentation

- All docs (`docs/*.md`, README, CONTEXT, AGENTS) must be in English.
- Pull requests must update related documentation; see
  [docs/agents/pull-requests.md](docs/agents/pull-requests.md).

### Editing

- Use patch-style edits for existing files; avoid `sed -i` and ad hoc rewrites.
- For new files, use an available file-edit/write primitive rather than shell
  heredocs or command-output redirection.

### Go Skill Corpus

- When the optional `cc-skills-golang` corpus is available, every AI agent
  working on Go code must locate and read the relevant skill files before
  editing, reviewing, or claiming completion.
- For coding tasks, first identify the implementation domains involved, such as
  error handling, concurrency, context propagation, CLI behavior, testing,
  security, dependencies, or performance. Then read the matching
  `cc-skills-golang` skill files. Do not limit coding work to the
  review-focused skill subset.
- For code review tasks, follow
  [docs/agents/code-review.md](docs/agents/code-review.md) and read the
  review-relevant skills named there before producing findings.
- Repository rules and local project context override generic skill advice.

### Go Toolchain

Never manually install Go. Run `which go` before any Go command.
If `which go` fails, report and stop — do not install, download, or work around
it.

### Verification

- **Before every commit that touches Go code, tests, dependencies, or Go-related
  scripts/workflows, the committing agent must run `just verify` locally.** This
  is the canonical verification gate defined in
  [docs/agents/verification.md](docs/agents/verification.md). Do not guess or
  compose a custom verification command — use `just verify`.
- `just verify` covers build, vet, lint, test (with race detector), stress
  tests, mod tidy, and coverage. It mirrors the non-E2E CI pipeline.
- Pure documentation changes (`.md` files only) do not require `just verify`.

## GitHub and PR Work

- Use `gh` CLI for all GitHub operations (not browser tools).
- For code review requests, follow
  [docs/agents/code-review.md](docs/agents/code-review.md).
- PR title and body must be in English and follow the repository PR title
  policy whenever an agent creates or updates PR metadata. See
  [docs/agents/pull-requests.md](docs/agents/pull-requests.md) for the full PR
  workflow, title policy, and body checklist.
- PR agents must create requested PRs with `gh pr create`; do not stop after
  pushing a branch. For OpenCode running inside GitHub Actions, see
  [docs/agents/opencode-github-actions.md](docs/agents/opencode-github-actions.md).

## Git Configuration

- Treat all Git configuration as read-only. Agents may inspect Git config, but
  must not write, unset, or override any Git config at repo, worktree, global,
  system, file, submodule, or per-command (`git -c`) scope unless the current
  human explicitly names the config key to change. If Git fails because config
  or identity is missing, unsafe, or wrong, stop and report; do not repair,
  guess, or work around it.

## Git Identity

- Use the existing git identity as-is. Inspect with
  `git config --get user.name` / `git config --get user.email`. Do not set,
  override, or hard-code author/committer identity.
- A commit with known wrong author metadata is tainted: do not make it the tip
  of any branch. See
  [docs/agents/pull-requests.md#git-identity](docs/agents/pull-requests.md#git-identity).

## Agent Workflows

- Issues: [docs/agents/issue-tracker.md](docs/agents/issue-tracker.md)
- Pull requests: [docs/agents/pull-requests.md](docs/agents/pull-requests.md)
- Code review: [docs/agents/code-review.md](docs/agents/code-review.md)
- GitHub Actions OpenCode runtime:
  [docs/agents/opencode-github-actions.md](docs/agents/opencode-github-actions.md)
- Release: [docs/agents/release.md](docs/agents/release.md)
- Domain and ADR: [docs/agents/domain.md](docs/agents/domain.md)
