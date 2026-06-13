# E2E Tests

- Core functional tests assert non-zero results. If the live server returns only
  an infobox with 0 web results, tests log a warning instead of failing, and
  warnings are collected in a `WARNING SUMMARY` block at the end of the test
  group (`TestMCPFunctional`, `TestMCPStdioE2E`, `TestCLISmoke`).
- Exceptions are owner-approved and documented. Known limitations of the CI
  SearXNG instance — for example, the `files` category being excluded from
  `TestMCPFunctional`'s category loop, or the `time_range=""` subtest
  downgrading zero-result outcomes to a `WARNING SUMMARY` entry — must not be
  broadened without approval. (The `time_range=""` subtest is named `"all"` in
  `TestMCPFunctional`'s `"all time ranges"` subtest.) The `time_range="day"`,
  `"month"`, and `"year"` subtests are expected to potentially return zero
  results and log the outcome as `(persistent, expected)`; they are not
  downgraded to warnings and are not part of the exception list.
- Adding strict assertions is preferred when a test can meaningfully assert
  non-zero results with a better query or parameter combination.
- Live-server assertions belong in Go E2E tests (`TestMCPFunctional`,
  `TestMCPStdioE2E`, `TestCLISmoke`) so they can use the shared warning-summary
  path. Shell smoke in `.github/workflows/e2e.yml` must stay limited to
  deterministic exit-code or structural checks and must not use `|| echo`,
  `grep -Eq "A|B"`, swallowed `python3 -c` assertions, or similar patterns that
  mask missing live-server evidence.
- For local server setup, use `just test-server-start`; never run
  `searxng-server-test/01-start-fg.sh` from agents or CI. See
  `docs/agents/test-server.md`.

## CI Retry Wrapper

`.github/workflows/e2e.yml` wraps all E2E test suites — those matching
`-run 'TestMCP'` (which includes `TestMCPFunctional`, `TestMCPStdioE2E`,
and `TestMCPErrors_InvalidInputs`) and `-run 'TestCLISmoke'` — in
`Wandalen/wretry.action` with `attempt_limit=10`.
This is because upstream search engines probabilistically rate-limit or ban
the CI SearXNG instance during automated runs — a characteristic inherent to
meta-search against live third-party engines, not a code or SearXNG bug.

Library-level retry (`SEARXNG_MAX_RETRIES=2`) covers single-request flakes;
the CI retry wrapper recovers from whole-instance throttling by retrying the
entire test run to land on a clean window. The job-level
`timeout-minutes: 300` in `e2e.yml` is the explicit worst-case budget; the
retry parameters documented in the inline comment must stay within it.
