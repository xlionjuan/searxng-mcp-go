# Local Review Reports

- `REPORT.md` is a local-only scratch report for ad hoc repository reviews,
  AGENTS.md reviews, test coverage analysis, and similar local QA work.
- `REPORT.md` is permanently in `.gitignore`; never commit it.
- Do not create or update `REPORT.md` for normal issue-fix, CI-fix,
  implementation, or PR-only work unless the user explicitly asks for a local
  review/report.
- Independent PR agents should ignore `REPORT.md` and put summaries, test
  evidence, and open questions in the PR body or GitHub comments instead.
- When using `REPORT.md`, append or update a task-specific section; do not
  overwrite previous reports.

## Issue and PR References

- Issue tracker work that becomes branch or PR work must follow
  `docs/agents/pull-requests.md`; PR agents must ignore `REPORT.md` unless
  explicitly asked for a local report.
- For PR work, put useful handoff material in the PR body or GitHub comments:
  what changed, why it changed, tests and commands run, remaining risks or
  follow-up work, and documentation updates included or why none were needed.
