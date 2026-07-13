# ADR-014: Retry Outcome Enum

**Status:** Accepted
**Date:** 2026-06-12

## Context

The `Search()` retry loop had two separate `ShouldRetry` calls with different
semantics: one for errors and HTTP status codes (passing the real error), and
one for empty responses that passed a bare `errEmptyResponse` sentinel to
`ShouldRetry`. This worked only because `isRetryableError` returned `true` for
any non-`*SearXNGError` error — a load-bearing property that was undocumented,
untested, and fragile.

The retry strategy's `ShouldRetry` inspected `resp` and `err` directly, making
it impossible to express "empty response, should retry" without faking an error.
Issue #240 proposed replacing this with an explicit `Outcome` enum.

Issue #247 documented the fragility: `errEmptyResponse` **must** remain a bare
`errors.New` or the retry silently breaks. This is a maintenance trap.

## Decision

### Original decision (2026-06-12)

Introduce an `Outcome` enum type with four values — `OutcomeSuccess`,
`OutcomeRetry`, `OutcomeEmptyRetry`, `OutcomeAbort` — and a pure
`classifyOutcome` function that determines the outcome from
(ctx, attempt, maxRetries, resp, err, isEmpty).

Changes:
- `ShouldRetry` now accepts `outcome Outcome` instead of `resp *http.Response`
  and `err error`. The strategy is purely outcome-driven.
- `isRetryableError` is removed; `classifyOutcome` replaces its logic.
- `errEmptyResponse` is renamed to `errSearchEmptyResults` (last-error path
  only; no longer used for retry decisions).
- The `//nolint:gocognit,cyclop,gocyclo` suppression on `Search()` is removed
  — the new loop is simple enough to pass without it.
- Three helpers (`searchContext`, `wrapSearchError`, `classifyAttempt`) are
  extracted to keep `Search()` focused on orchestration.

### Extension (2026-07-13): `OutcomeEmptyExhausted`

The original design treated empty responses on the final attempt as
`OutcomeSuccess`, returning a successful `SearchResponse` with zero results
and only logging a `slog.Warn`. This was a vestige of the pre-ADR-014 era
where `errEmptyResponse` could not be the final return value.

Issue #475 identified the contradiction between the documented error path
("search returned empty results after all retries") and the actual success
response. The behaviour was corrected by adding a fifth outcome value,
`OutcomeEmptyExhausted`, that causes the retry loop to return a wrapped
`errSearchEmptyResults` error instead of silently succeeding.

Changes:
- Adds `OutcomeEmptyExhausted` to the `Outcome` enum.
- `classifyOutcome` returns `OutcomeEmptyExhausted` when `isEmpty && attempt >= maxRetries` (empty response with no retries remaining).
- `ShouldRetry` treats `OutcomeEmptyExhausted` as non-retryable (returns
  `false`), consistent with its final-attempt semantics.
- `Search()` handles `OutcomeEmptyExhausted` after `OutcomeSuccess`. The two
  values are mutually exclusive (a response cannot be both exhausted and
  successful), so the check order is just a convention.
- `errSearchEmptyResults` is promoted from internal tracking error to the
  actual terminal error returned to callers.
- The `slog.Warn("search returned empty after exhausting retries")` log is
  removed; the error itself communicates the exhausted-retries condition.

## Consequences

- **Explicit contract.** The empty-response retry path no longer relies on an
    accidental property of `errors.As`. The retry decision is visible in the
    `Outcome` switch.
- **Simpler Search loop.** Single `ShouldRetry` call instead of two. No fake
    error, no duplicated wait/continue logic.
- **New exported type.** `Outcome` is exported from `searxng` package. Five
    values: `OutcomeSuccess`, `OutcomeRetry`, `OutcomeEmptyRetry`,
    `OutcomeAbort`, `OutcomeEmptyExhausted`.
- **Backward compatible behavior.** Retry behaviour is preserved for all
    existing scenarios: plain network errors retry, `SearXNGError` does not,
    retryable status codes (429, 5xx) retry, empty responses retry on
    non-final attempts, non-retryable status codes abort.
- **Non-backward compatible terminal behaviour.** The original decision
    returned an empty success response when all retries were exhausted on an
    empty SearXNG reply. This extension changes that to a terminal error. MCP
    callers now receive an error instead of a zero-result success. E2E tests
    that relied on the old success-with-empty contract have been updated to
    expect the error or to tolerate empty results via the warning-summary
    path.
- **Resolves #247.** The fragile `errEmptyResponse` sentinel is no longer used
    for retry decisions.
