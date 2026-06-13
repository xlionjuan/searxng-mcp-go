# Verification

- Code changes must be verified with the narrowest meaningful build/test
  commands before committing. Broaden verification for shared behavior, public
  interfaces, test infrastructure, or release/CI changes.
- PR agents must run local verification before opening or updating a PR or
  reporting the task complete. Choose the gate by the affected surface. For Go
  code, Go tests, dependencies, Go-related scripts, or workflows that execute or
  configure Go build/test/lint/release behavior, the minimum completion gate is
  the non-E2E workflow checks listed below.
- Do not run the Go completion gate for changes that cannot affect Go code,
  dependencies, build, test, lint, or release behavior. For example, pure
  documentation changes and GitHub Actions metadata or prompt-only changes
  should use narrow validation such as diff checks, YAML parsing, or actionlint
  when available.
- If `golangci-lint` is unavailable, run `go vet ./...` as the fallback static
  check and state in the PR body that the linter itself could not be run.
- If a subagent made code changes, the coordinating agent must review and verify
  those changes before committing.
- Never trust your own knowledge of version numbers, release dates, or
  specification statuses. Time-sensitive external facts must be verified against
  a current authoritative source before being stated as fact; if verification is
  unavailable, avoid the claim or state the verification gap.

## Build and Test Commands

Root benchmarks live in `bench_test.go`; internal benchmarks live in
`internal/searxng/bench_test.go`.

| Command | Scope |
|---------|-------|
| `go build ./...` | Build all packages |
| `go test ./...` | Run unit tests, excluding e2e and stress |
| `go test -race -shuffle=on ./...` | CI-style test run with race detector |
| `go test -tags=stress -race ./...` | Include stress/concurrency tests |
| `go test -tags=e2e -run TestMCPStdioE2E -count=1 .` | E2E test; requires `SEARXNG_URL` and a running test server. `E2E_MCP_BINARY` skips per-test `go build`; see `docs/MCP_TESTING.md`. |
| `golangci-lint run ./...` | Lint; CI uses v2.12.2 |
| `go vet ./...` | Static analysis fallback |

## Completion Gate for AI Agents

Before any AI agent opens a PR, updates a PR, or reports a code-changing task
complete, it must run the gate that matches the touched surface.

For Go code, Go tests, Go dependencies, Go-related scripts, or workflow changes
that alter Go setup, Go commands, test/lint commands, release builds, or
required environment for Go execution, run the `.github/workflows/test.yml`
checks except the E2E workflow:

- `go mod verify`
- `go mod download`
- `go mod tidy`
- `git diff --exit-code go.mod go.sum`
- `go build -o searxng-mcp-go .`
- `go test -race -shuffle=on -coverprofile=coverage.out ./...`
- `go test -race -tags=stress -shuffle=on ./...`

Also run the `.github/workflows/lint.yml` checks:

- `golangci-lint run --timeout 5m`
- `golangci-lint fmt --diff`

For workflow-only changes that do not affect Go execution, use targeted
workflow validation instead of the Go completion gate. Examples include trigger
metadata, concurrency groups, permissions that do not enable code mutation,
manual-dispatch inputs, OpenCode prompts, labels, comments, or job names. At
minimum, inspect the diff and run `git diff --check`; also run `actionlint` or a
YAML parser when available. If the workflow change can change Go commands,
toolchain versions, dependency behavior, test coverage, linting, release
artifacts, or generated files, use the Go completion gate above.

Do not treat the E2E workflow as a pre-completion requirement for AI agents.
E2E requires the SearXNG test server and remains a pull-request status check.
CodeQL is also a GitHub Actions status check rather than a repo-local command
that agents should try to reproduce before completion.

If a required tool is unavailable, use the documented fallback when one exists
and state the exact limitation in the PR body or final response. If the missing
tool means the completion gate could not be run, do not claim the task is fully
verified.

E2E retry tweaks and the `e2eMCPEnv` helper live in `docs/MCP_TESTING.md`;
do not duplicate them here. The CLI exit-code tests in
`e2e_exitcode_test.go` are part of the default `go test ./...` set (they
do not require a live server), so they are exercised by the `Run tests`
step in `.github/workflows/test.yml` as well as by the E2E workflow.
