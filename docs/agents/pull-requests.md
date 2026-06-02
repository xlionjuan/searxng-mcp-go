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

## Git Identity

Do not change git author or committer identity as part of normal PR work. In
particular, do not run:

- `git config user.name ...`
- `git config user.email ...`
- `git config --global user.name ...`
- `git config --global user.email ...`

If a commit fails because git does not know the author identity, stop and report
the error instead of inventing an identity. If the user explicitly asks you to
investigate missing git identity behavior, use an isolated temporary repository;
do not change global config or this repo's local config.

When a PR adds or changes a GitHub Actions workflow or agent workflow that can
commit, verify the exact author and committer identity used by the workflow or
action. Do not enable workflows that commit as generic or unverified GitHub
noreply identities such as `<tool>@users.noreply.github.com`. Never derive a
commit email from a tool name, action name, repository name, or package name
unless that exact identity is verified. The identity must be an explicit,
reviewed bot/app identity owned by the workflow provider or this repo owner. If
the action config does not make that clear, document the risk and leave the
workflow disabled or blocked for human review.

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

PR agents must run local verification themselves before opening or updating a
PR. Do not rely on CI or reviewers as the first validation pass.

For Go code, test, CI, or script changes, the minimum local gate is:

- `go test ./...`
- `golangci-lint run ./...`

If `golangci-lint` is unavailable, run `go vet ./...` as the fallback static
check and state in the PR body that the linter itself could not be run. If any
minimum check fails because of the PR's changes, fix the failure before opening
or updating the PR. If a failure is pre-existing or environment-specific, record
the exact command, failure summary, and why it is not caused by the PR.

After the minimum gate, broaden verification when the blast radius is larger.

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

## PR Title and Body Language

The PR title and body must be in English even if the user originally
discussed the change in another language. PR comments and review replies
should reply in whatever language the user is using. See the
`### PR Title and Body Language` section in the root `AGENTS.md` for the
canonical rule.
