# Pull Requests

Use this guide when an agent is preparing a branch or pull request for this
repository.

## GitHub Operations

Use the `gh` CLI for all GitHub operations. Do not use browser tools for GitHub.

Common commands:

- **Inspect current PR context**: `gh pr view --json number,title,body,labels,files,reviewDecision,statusCheckRollup`
- **Create a PR**: `gh pr create --title "..." --body "..."`
- **Check CI**: `gh pr checks <number>`
- **Read review comments**: `gh pr view <number> --comments`
- **Comment on a PR**: `gh pr comment <number> --body "..."`

## Local Reports

`REPORT.md` is only for local ad hoc review/report work. PR agents must ignore
it unless the user explicitly asks for a local report. Do not add `REPORT.md` to
a branch or PR.

For PR work, put the useful handoff material in the PR body or GitHub comments:

- What changed
- Why it changed
- Tests and commands run
- Remaining risks or follow-up work
- Documentation updates included, or why none were needed

## Documentation Updates

Update related documentation in the same PR whenever the change affects:

- CLI flags, environment variables, defaults, exit codes, or examples
- MCP tool schema, parameters, response fields, or error behavior
- Output formatting, truncation, warning text, or JSON field presence
- Build, CI, lint, E2E, release, or local test workflows
- Domain terminology in `CONTEXT.md`
- Agent instructions in `AGENTS.md` or `docs/agents/`
- Accepted or challenged decisions in `docs/adr/`

If a code change deliberately does not need documentation updates, state that in
the PR body with a short reason.

## ADR Awareness

Read `CONTEXT.md` and the ADRs relevant to the area being changed before
opening the PR. If the PR contradicts an accepted ADR, either update/supersede
the ADR in the same PR or explicitly frame the PR as an ADR challenge.

Do not silently bypass these accepted decisions:

- ADR-001: no PGO without representative profiling data
- ADR-003: HTTP warning policy for non-private hosts
- ADR-004: MCP stdin mode uses environment variables, not CLI args
- ADR-005: do not expose `corrections`
- ADR-006: expose `unresponsive_engines` only in debug mode
- ADR-007: no DNS rebinding protection unless the ADR is revisited
- ADR-008: redirects are same-hostname-only

## Verification

Run the narrowest meaningful tests for the change, then broaden when the blast
radius is larger.

Common checks:

- `go build ./...`
- `go test ./...`
- `go test -race -shuffle=on ./...`
- `go test -tags=stress -race ./...`
- `go vet ./...`
- `golangci-lint run ./...`

E2E tests require a SearXNG test server and `SEARXNG_URL`; see `AGENTS.md` for
setup details.

## PR Body Checklist

Every PR body should include:

- Summary of code changes
- Documentation changes, or "No documentation changes needed" with a reason
- Tests run
- Linked issue(s)
- Known limitations or follow-up work, if any
