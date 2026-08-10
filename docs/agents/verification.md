# Verification

- Code changes must be verified with the narrowest meaningful build/test
  commands before committing. Broaden verification for shared behavior, public
  interfaces, test infrastructure, or release/CI changes.
- PR agents must run local verification before opening or updating a PR or
  reporting the task complete. **Run `just verify`** for Go code, Go tests,
  dependencies, Go-related scripts, or workflows that execute or configure Go
  build/test/lint/release behavior. This mirrors the non-E2E CI pipeline
  (test.yml + lint.yml).
- Do not run `just verify` for changes that cannot affect Go code,
  dependencies, build, test, lint, or release behavior. For example, pure
  documentation changes and GitHub Actions metadata or prompt-only changes
  should use narrow validation such as diff checks, YAML parsing, or actionlint
  when available.
- If `golangci-lint` is unavailable, run `go vet ./...` as the fallback static
  check, skip the lint variants, and state in the PR body that the linter
  itself could not be run.
- If a subagent made code changes, the coordinating agent must review and verify
  those changes before committing.
- Never trust your own knowledge of version numbers, release dates, or
  specification statuses. Time-sensitive external facts must be verified against
  a current authoritative source before being stated as fact; if verification is
  unavailable, avoid the claim or state the verification gap.

## Individual Recipes

The build and test commands are also available as individual `just` recipes for
targeted use. Root benchmarks live in `bench_test.go`; internal benchmarks live
in `internal/searxng/bench_internal_test.go`.

| Recipe | Command | Scope |
|--------|---------|-------|
| `build` | `go build -o searxng-mcp-go .` | Build the repository binary |
| `deps` | `go mod download` + `go mod verify` | Download and verify module dependencies |
| `test` | `go test -race -shuffle=on ./...` | Unit tests, excluding e2e and stress |
| `test-cover` | `go test -race -shuffle=on -coverprofile=coverage.out ./...` | CI-style test run with race detector |
| `test-stress` | `go test -race -tags=stress -shuffle=on ./...` | Include stress/concurrency tests |
| `test-e2e` | `go test -race -tags='e2e stress' -count=1 -timeout=900s .` | E2E test; requires `SEARXNG_URL` and a running test server. `E2E_MCP_BINARY` skips the MCP E2E package-level build; see `docs/MCP_TESTING.md`. |
| `test-e2e-stress` | `go test -v -tags='e2e stress' -run 'TestMCPStress' -race -count=1 -timeout=900s .` | E2E stress subset used by the manual stress workflow |
| `test-e2e-mcp` | `go test -v -tags=e2e -run 'TestMCP' -race -count=1 -timeout=600s .` | MCP stdio E2E subset used with the CI retry wrapper |
| `test-e2e-cli-smoke` | `go test -v -tags=e2e -run 'TestCLISmoke' -race -count=1 -timeout=600s .` | CLI smoke E2E subset used with the CI retry wrapper |
| `lint` | `golangci-lint run --timeout 5m` (+ `stress` + `e2e` + `e2e,stress` build tags) | Full lint suite (4 tag variants); CI uses v2.12.2 |
| `vet` | `go vet ./...` | Static analysis fallback |

## Completion Gate for AI Agents

Before any AI agent opens a PR, updates a PR, or reports a code-changing task
complete, it must run the gate that matches the touched surface.

**For Go code, Go tests, Go dependencies, Go-related scripts, or workflow
changes that alter Go setup, Go commands, test/lint commands, release builds,
or required environment for Go execution**, run:

```bash
just verify
```

This runs all build/test/lint steps from `.github/workflows/test.yml` and
`.github/workflows/lint.yml` (CI-only steps such as coverage-artifact upload
excluded), matching the CI verification gate.

For workflow-only changes that do not affect Go execution, use targeted
workflow validation instead of `just verify`. Examples include trigger
metadata, concurrency groups, permissions that do not enable code mutation,
manual-dispatch inputs, OpenCode prompts, labels, comments, or job names. At
minimum, inspect the diff and run `git diff --check`; also run `actionlint` or a
YAML parser when available. If the workflow change can change Go commands,
toolchain versions, dependency behavior, test coverage, linting, release
artifacts, or generated files, use `just verify` instead.

Do not treat the E2E workflow as a pre-completion requirement for AI agents.
E2E requires the SearXNG test server and remains a pull-request status check.
CodeQL is also a GitHub Actions status check rather than a repo-local command
that agents should try to reproduce before completion.

If a required tool is unavailable, use the documented fallback when one exists
and state the exact limitation in the PR body or final response. If the missing
tool means `just verify` could not be run, do not claim the task is fully
verified.

E2E retry tweaks and the `e2eMCPEnv` helper live in `docs/MCP_TESTING.md`;
do not duplicate them here. The CLI exit-code tests in
`e2e_exitcode_test.go` are part of the default `go test ./...` set (they
do not require a live server), so they are exercised by `just verify`.
Of those, `TestMCPExitCode_StdinValidation` also matches the E2E workflow's
`-run 'TestMCP'` filter, while `TestValidationExitCode` runs under
`just verify` only.
