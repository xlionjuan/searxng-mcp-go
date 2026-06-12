# Code Review

Use this guide when an AI agent is asked to review a pull request, branch,
commit, or working-tree diff. Review means finding defects and review-blocking
risks. It is not a summary task, not a style pass, and not a rubber stamp.

## Review Standard

Start from the assumption that the change can be wrong in ways that compile.
Read the actual diff, then read the adjacent code that the diff calls, mutates,
or depends on. A review that only comments on changed lines is incomplete.

A code review must answer:

- Does the patch preserve the existing behavior that callers rely on?
- Does the new behavior match the PR title, body, issue, tests, and docs?
- Does the patch integrate with nearby types, helpers, tests, CI, docs, and ADRs?
- Does it introduce a correctness, reliability, security, or maintainability
  risk that should block merge?

Do not approve a change because tests are green. Tests are evidence, not proof.
Use CI to calibrate risk, then inspect the code.

## Required Inputs

For GitHub PR review, inspect at least:

- PR title and body
- Changed files and full diff
- Latest commit SHA and CI status
- Existing review comments when the user asks for follow-up
- Linked issue or ADR when the PR claims to close, implement, supersede, or
  challenge one

Use `gh` for GitHub operations. Useful commands:

- `gh pr view <number> --json number,title,body,headRefOid,files,statusCheckRollup,reviewDecision`
- `gh pr diff <number>`
- `gh pr checks <number>`
- `gh pr view <number> --comments`

For local branch or commit review, inspect the merge base and the full patch:

- `git status --short --branch`
- `git diff --stat <base>...HEAD`
- `git diff <base>...HEAD`
- `git show --stat --oneline <commit>`
- `git show <commit>`

## Scope Discipline

Review the patch and the integration surface around it. Read adjacent code until
you can explain how the changed code is reached, what invariants it depends on,
and what tests should fail if it is wrong.

Do not expand into a repository-wide audit unless the user asked for one. If you
notice unrelated problems outside the patch, mention them only when they make the
patch unsafe or misleading.

Do not rewrite the code during review unless the user explicitly asks for fixes.
Review output should be actionable findings, not an unsolicited patch.

## Findings

Lead with findings, ordered by severity. A finding must be specific enough for a
maintainer to act without guessing.

Each finding should include:

- Severity: `[P0]`, `[P1]`, `[P2]`, or `[P3]`
- File and line reference
- The broken invariant or concrete failure mode
- Why this patch introduces or exposes the problem
- The minimal condition that triggers it

Severity:

- `[P0]` blocks everything: data loss, credential exposure, remote code
  execution, or a guaranteed production outage.
- `[P1]` must be fixed before merge: incorrect public behavior, CI/release
  breakage, security boundary weakening, panic on valid input, race, leak, or
  broken compatibility.
- `[P2]` should be fixed before merge: missing edge-case handling, incomplete
  tests for risky logic, unclear API contract, brittle workflow, or misleading
  documentation.
- `[P3]` is non-blocking: cleanup, clarity, naming, or local maintainability.

Do not invent findings. If you are uncertain, state the assumption and the
evidence needed. If no actionable findings exist, say that directly and name the
residual risk or test gap.

## Output Format

Use this order:

1. Findings
2. Open questions or assumptions
3. Brief summary
4. Tests or CI evidence inspected

Keep summaries short. The value of a review is in the findings.

## Go Review Focus

Pay special attention to failures that AI-generated Go code often misses:

- Ignored errors, especially `result, _ := ...`, unchecked `Close`, unchecked
  encoder/decoder writes, and fire-and-forget cleanup.
- Error wrapping that loses the chain. Internal propagation should usually use
  `%w`, not `%v`, and add useful context.
- Log-and-return error handling. An error should normally be logged or returned,
  not both.
- `:=` shadowing of `err`, config values, or normalized arguments.
- Typed nil values returned as interfaces.
- Nil map writes, nil channel waits, and nil slice JSON behavior when API output
  expects `[]`.
- Slice aliasing after `append`, subslice retention, and returned mutable maps or
  slices that expose internal state.
- Numeric conversions that can truncate or wrap.
- `time.Time` comparison with `==` instead of `Equal` when monotonic clock data
  may differ.
- `strings.Trim` used as if it removed a prefix or suffix.
- Context not passed as the first parameter, not propagated to I/O, or ignored
  inside retry/loop/concurrency code.
- Goroutines without a clear owner, cancellation path, wait path, or panic/error
  propagation.
- Channel ownership mistakes: receiver closes, missing close, send after close,
  missing `ok` check when close is meaningful.
- `time.After` in hot or long-running loops.
- `sync.WaitGroup.Add` inside a goroutine.
- Mutex copied by value receiver, mutex held across I/O, or shared map without
  synchronization.
- `http.Get` or default clients without timeout.
- `http.Error` without `return`.
- Resource cleanup deferred inside loops where resources accumulate until
  function exit.
- Reflection or `any` used where generics or concrete types would preserve type
  safety.
- Positional composite literals for non-local structs.
- Public surface area added without a durable reason.

## Tests in Review

Check whether tests constrain behavior, not just coverage.

Flag tests that:

- Assert implementation details instead of observable behavior.
- Add happy-path coverage while missing error, boundary, cancellation, or
  malformed-input cases.
- Use table-driven tests without named cases.
- Depend on execution order, wall-clock sleeps, external network, or shared
  mutable state.
- Add flakiness through real time, unbounded goroutines, or retries without
  deterministic failure evidence.
- Fail to update golden data when output format changes intentionally.
- Change CLI, MCP schema, formatting, warnings, or defaults without regression
  coverage.

For this repository, remember that `golden_capture_test.go` is a byte-for-byte
lock on `formatResults()` output. Any intentional formatting change must update
the inline golden string in that test.

## `//nolint` Review Rules

Treat every new or modified `//nolint` as suspicious until justified.

A `//nolint` is acceptable only when all are true:

- It names the exact linter, for example `//nolint:gocyclo`.
- It includes a specific reason, not a placeholder.
- The reason explains why fixing the root cause would make the code worse or why
  the linter is a false positive.
- The suppression is scoped to the smallest possible line or declaration.
- It does not hide correctness, resource lifecycle, security, or error handling
  issues.

Flag:

- Bare `//nolint`
- `//nolint` without an explanatory comment
- Vague reasons such as "needed", "ignore", "false positive" without evidence,
  "AI generated", or "lint"
- Suppression of `errcheck`, `govet`, `staticcheck`, `bodyclose`,
  `sqlclosecheck`, `rowserrcheck`, or security linters unless the justification
  is exceptional and verifiable
- File-wide or function-wide suppression where a line-level suppression would
  work
- A suppression added next to code that could be made clearer instead

## Documentation and Workflow Review

When a patch changes behavior visible to users, agents, or CI, verify matching
documentation changes. This includes CLI flags, environment variables, defaults,
MCP schema, output formatting, warning text, test commands, release process,
GitHub Actions, and agent instructions.

When a patch changes workflows, check that:

- `uses:` entries are pinned to SHA with a `# vX.Y.Z` comment.
- Go version is fixed, not `stable`.
- Job and step names do not encode version numbers.
- Permissions are minimal and match the operation.
- Agent workflows do not grant write permissions without a documented reason.
- Any workflow that can commit uses an explicit, verified identity.

## Optional Go Skill Corpus

The optional `cc-skills-golang/skills` corpus is useful review context, but it
is not a substitute for repository rules.

GitHub Actions OpenCode workflows install a pinned checkout of
`samber/cc-skills-golang` into `~/.agents/skills/cc-skills-golang`, an OpenCode
auto-discovery path outside the repository working tree. Do not depend on a
maintainer's personal filesystem layout.

On a developer machine, use the corpus only if it is already available or can be
located from the local environment. Do not assume a fixed path and do not require
cloning it into `/tmp`.

When the corpus is available, an AI reviewer must locate and read the relevant
skill files before producing review findings. Start code review with these
review-relevant skills:

- `golang-troubleshooting/references/code-review-flags.md`
- `golang-lint/references/nolint-directives.md`
- `golang-error-handling/SKILL.md`
- `golang-concurrency/SKILL.md`
- `golang-safety/SKILL.md`
- `golang-testing/SKILL.md`
- `golang-code-style/SKILL.md`

Read other `cc-skills-golang` skills when the patch touches their domain, such
as context propagation, CLI behavior, dependency management, security, or
performance.

For coding tasks, do not use this review-focused list as the default. Choose the
skills that match the implementation work, then read them before editing, as
described in `AGENTS.md`.
Repository rules win over generic skill advice, and the PR diff still has
priority over background reading.
