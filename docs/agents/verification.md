# Verification

- Code changes must be verified with the narrowest meaningful build/test
  commands before committing. Broaden verification for shared behavior, public
  interfaces, test infrastructure, or release/CI changes.
- PR agents must run local verification before opening or updating a PR. For Go
  code, test, CI, or script changes, the minimum local gate is `go test ./...`
  plus `golangci-lint run ./...`.
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

E2E retry tweaks and the `e2eMCPEnv` helper live in `docs/MCP_TESTING.md`;
do not duplicate them here. The CLI exit-code tests in
`e2e_exitcode_test.go` are part of the default `go test ./...` set (they
do not require a live server), so they are exercised by the `Run tests`
step in `.github/workflows/test.yml` as well as by the E2E workflow.
