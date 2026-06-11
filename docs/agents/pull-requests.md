# Pull Requests

Use this guide when an agent is preparing a branch or pull request for this
repository.

## GitHub Operations

Use the `gh` CLI for all GitHub operations. Do not use browser tools for GitHub.

Common commands:

- **Inspect current PR context**: `gh pr view --json number,title,body,labels,files,reviewDecision,statusCheckRollup`
- **Create a PR**: `gh pr create --title "..." --body "..."`
- **Update PR metadata**: `gh pr edit <number> --title "..." --body "..."`
- **Check CI**: `gh pr checks <number>`
- **Read review comments**: `gh pr view <number> --comments`
- **Comment on a PR**: `gh pr comment <number> --body "..."`

When the user asks an agent to create a PR, creating the PR is part of the task.
Do not stop after pushing a branch, and do not hand the user a
`https://github.com/.../pull/new/...` URL as a substitute for `gh pr create`.
If `gh pr create` fails, report the exact failure and leave the task blocked
instead of implying that a PR exists.

## Git Identity

Use the git author and committer identity that already exists in the execution
environment. It is safe to inspect it with `git config --get user.name` and
`git config --get user.email`, but do not change it as part of normal PR work.
In particular, do not run:

- `git config user.name ...`
- `git config user.email ...`
- `git config --global user.name ...`
- `git config --global user.email ...`
- `git -c user.name=... -c user.email=... commit`
- `git commit --author=...`

Do not set `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`,
`GIT_COMMITTER_EMAIL`, or `EMAIL` to influence commits. A generic instruction
such as "use the default git identity", "recreate the commit", or "fix the
author" means to use the currently configured identity with a plain
`git commit ...`; it is not permission to hard-code values such as
`opencode-agent`, `opencode@anomaly.co`, or guessed `*@users.noreply.github.com`
addresses.

If a commit fails because git does not know the author identity, stop and report
the error instead of inventing an identity. If the user explicitly asks you to
investigate missing git identity behavior, use an isolated temporary repository;
do not change global config, this repo's local config, or the commit environment
for this project.

### Tainted Commits

A commit is tainted for publishing when any user message, review comment, PR
metadata, repository instruction, or local evidence says its author or committer
metadata may be wrong. Treat disputed identity as a stop condition, even if the
commit's code changes are otherwise correct.

Do not make a tainted commit the tip of any local or remote branch. Do not
restore, reset to, cherry-pick, revert back to, rebase onto, force-push, or
otherwise republish that commit. This applies even when the request says
"restore the original commit", "try again", "preserve the author", "use the
default identity", or "fix the author".

Do not copy author or committer values from a tainted commit into a new commit,
command-line config, environment variable, workflow setting, or PR explanation as
the intended identity. Those values are evidence of the dispute, not a source of
truth.

To keep the code changes from a tainted commit, first stop and state the metadata
conflict. After the user confirms that the code changes should be recreated,
reapply the patch onto a clean base and create a new commit with a plain
`git commit` using the already configured identity. If the configured identity is
missing or appears to be the disputed identity, stop and ask for human guidance
before committing or pushing.

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
- ADR-009: GET fallback is disabled by default and must be explicitly opted in
- ADR-010: neutralize terminal control sequences in CLI text output
- ADR-011: share `TruncateRunes`; `MaxContentRunes` is CLI-only

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
- `go test -tags=stress -race ./...` (in-process concurrency stress; no live server)
- `go test -tags='e2e stress' -race -run 'TestMCPStress' -count=1 -timeout=900s .`
  (E2E stress; requires `SEARXNG_URL` and the local test server)
- `go vet ./...`
- `golangci-lint run ./...`

E2E tests require a SearXNG test server and `SEARXNG_URL`; see
`docs/agents/test-server.md` for setup details.

## Reviewing Existing PRs

When asked to review or follow up on a PR that the agent did not create or
modify, do not re-run the local test suite by default. CI is the source of
truth for the existing change.

Start by checking CI status before doing any local work:

- `gh pr view <number> --json statusCheckRollup,reviewDecision`
- `gh pr checks <number>`
- `gh run view <run-id> --log-failed` to read a failing job's log

Default policy:

- If CI is green on the latest commit, trust it. Do not re-run `go test ./...`,
  `golangci-lint run ./...`, or any other verification locally — the
  verification has already been done. Skipping the rerun is the correct
  behavior, not a shortcut.
- If CI is red or pending, prefer reading the CI log with `gh` over re-running
  locally. Re-run locally only when the log is not enough to diagnose the
  failure, the failure looks environment-specific, or the user explicitly
  asks for a local reproduction.
- Re-run local tests if the agent is about to make code changes on the PR,
  because the new code needs verification before pushing. The "Verification"
  section above covers that case.

Record the CI evidence in the PR comment or body the agent posts — for example
"CI: green on `abcdef` (test, lint)" or a short summary of the failing job
plus a link to the run — so reviewers do not have to re-check CI themselves.

This rule does not apply to PRs the agent is creating or updating with new
commits; the "Verification" section above governs those.

## PR Title Policy

PR titles are persistent repository records, not agent progress messages.

Use a concise English title with a semantic prefix:

```text
<type>: <concise change summary>
```

Allowed `type` values:

- `fix`: user-visible bug fixes, CI failures, flaky tests, or broken behavior
- `feat`: new user-visible functionality
- `docs`: documentation-only changes
- `test`: test-only changes that do not change runtime behavior
- `ci`: GitHub Actions, release workflows, or other CI configuration
- `refactor`: behavior-preserving code restructuring
- `chore`: maintenance work that does not fit the categories above

Write the summary in sentence case without a trailing period. Name the
user-visible or reviewer-visible change:

- Prefer titles such as `ci: move CLI smoke checks into Go E2E`
- Use `docs: document PR title requirements for agents` for documentation-only
  guidance changes
- Use `fix: route news E2E zero results through warning summary` for bug or
  flake fixes
- Keep titles specific enough to distinguish the PR in history

Do not include:

- Agent execution state such as `PR pushed`, `branch pushed`, `created PR`, or `ready`
- Prefixes that describe the agent instead of the change, such as `agent:`,
  `opencode:`, `codex:`, or `bot:`
- Session names, run IDs, timestamps, model names, or tool names unless the PR
  changes that tool directly
- Decorative arrows or symbols used as status shorthand
- Raw issue discussion language copied from a non-English conversation
- Vague titles such as `fix: issue`, `docs: update docs`, or `chore: changes`

Before creating or updating a PR, read the final title once as a reviewer would.
If it describes what the agent did operationally instead of what the code or
docs change, rewrite it.

## PR Body Checklist

Every PR body should include:

- Summary of code changes
- Documentation changes, or "No documentation changes needed" with a reason
- Tests run
- Linked issue(s)
- Known limitations or follow-up work, if any

Use this structure unless the user provides a stricter repository-specific
template:

```markdown
## Summary
- ...

## Documentation
- ...

## Tests
- ...

## Risks / Follow-up
- ...

Closes #...
```

Keep the body focused on durable review context. Automatically-appended agent
session cards, social-card images, HTML embeds, links to transient agent
sessions, and GitHub Actions run links are allowed, but they are supplemental
metadata and must not replace the normal summary, documentation, tests, issue
link, and risk sections.

Do not include:

- `https://github.com/.../pull/new/...` links after the PR has been created
- Multiple issues combined under a single closing keyword (e.g. `Closes #22 and #23`, `Closes #22, #23`) — only the first issue is closed. Each issue must have its own `Closes #N` line.
- Duplicated closing keywords such as two separate `Closes #22` lines
- Long pasted chat transcripts or hidden reasoning
- Claims that tests passed without the exact commands run

If the PR supersedes another branch or PR, state that plainly in the summary,
for example `Supersedes #42 by moving all live-server CLI smoke checks into Go
E2E.` Do not use supersession as a reason to omit the normal summary,
documentation, tests, issue link, or risk sections.

## Metadata Self-Check

Before running `gh pr create` or `gh pr edit`, verify:

- The title is English, uses an allowed semantic prefix, and describes the
  change rather than the agent workflow
- The body is English and contains summary, documentation impact, tests, linked
  issues, and risks or follow-up
- The body has no `/pull/new/` URL, duplicated closing keywords, or branch-only
  handoff language
- Any non-English user instructions have been translated or summarized in
  English rather than copied verbatim
- If the user explicitly asked to create the PR, the final result is an actual
  PR URL from `gh pr create`, not only a pushed branch URL

## PR Title and Body Language

The PR title and body must be in English even if the user originally discussed
the change in another language. PR comments and review replies should reply in
whatever language the user is using. See the `GitHub and PR Work` section in the
root `AGENTS.md` for the canonical summary.
