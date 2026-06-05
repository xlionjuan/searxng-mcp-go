# ADR-011: Share `TruncateRunes` Helper; `MaxContentRunes` Is CLI-Only

**Status:** Accepted
**Date:** 2026-06-05

## Context

Two rune-safe truncation helpers had drifted into the codebase, and the
`MaxContentRunes` constant's documented scope did not match the
implementation.

1. `format.go:113` (root package) defined an unexported `truncateRunes`
   helper used by the CLI text formatter to bound result and infobox
   `content` fields.
2. `internal/searxng/deduplicate.go:47` defined a separate unexported
   `truncateAtRunes` helper used by the answer/infobox deduplication
   prefix match.

Both helpers walked the input with `for i := range s`, sliced on the
rune boundary when the limit was reached, and returned the original
string unchanged for inputs that already fit. The behavior was
functionally equivalent; the duplication was structural and risked
subtle drift (e.g. one of them short-circuited on `str == ""`, the
other let the empty-string case fall through the loop naturally).

Separately, `MaxContentRunes` was documented in
`internal/searxng/constants.go:23` as "the maximum result content
length retained after normalization." That wording implied the limit
applies to the normalized `SearchResponse` that flows into both the
CLI formatter and the JSON / MCP path. In practice, the constant was
only consulted in `format.go`. The typed response surfaced to JSON
mode (`--json`) and MCP consumers was the full un-truncated
`SearchResponse`. `CONTEXT.md:72` repeated the same wording without
flagging the rendering-vs-data-model distinction.

## Decision

1. **One shared helper.** Rune-safe truncation moves to a new exported
   helper `searxng.TruncateRunes` in
   `internal/searxng/truncate.go`. The searxng deduplication pass and
   the root CLI formatter both call it. The two prior local helpers
   (`truncateRunes` in `format.go`, `truncateAtRunes` in
   `deduplicate.go`) are removed.

2. **`MaxContentRunes` is a CLI formatting limit, not a normalization
   limit.** The constant stays where it is (`internal/searxng/constants.go`)
   because the helper's primary consumer is the formatter, and the
   searxng package is the natural home for a low-level string
   utility. The docstring is rewritten to spell out:
   - it bounds `content` rendering in `formatResults`;
   - the typed `SearchResponse` returned to JSON and MCP consumers is
     NOT truncated;
   - the truncation itself uses `TruncateRunes`.

   `CONTEXT.md` and `docs/OUTPUT_FORMAT.md` are updated for
   consistency. The `docs/OUTPUT_FORMAT.md` limits table already
   stated "CLI text mode only" and is left as the single place that
   documents the per-mode behavior; the constant's docstring now
   matches that table.

## Rationale

1. **Single source of truth.** Two byte-identical helpers invited
   drift. `TruncateRunes` is exercised by both call sites and by
   `TestTruncateRunes` in `internal/searxng/truncate_internal_test.go`.
2. **Helper lives with the lower-level package.** The searxng package
   has no dependencies on the root package, so the import direction
   is clean. The root package already imports searxng for
   `MaxContentRunes`, `UnescapeIfNeeded`, `ExternalContentWarning`,
   etc.; calling `searxng.TruncateRunes` adds no new import edge.
3. **Render-time limit, not data-model limit.** Truncating before
   formatting preserves the contract that JSON and MCP consumers
   always see the full upstream text. If a future change needs
   output-size bounding for JSON, that should be a separate
   `MaxJSONContentRunes` (or similar) constant applied at JSON
   marshal time — not a silent broadening of `MaxContentRunes`.
4. **Docstring matters.** The previous "retained after normalization"
   wording was wrong enough to mislead a future contributor into
   thinking normalization itself trims content. Explicitly naming
   the formatter and the modes avoids that trap.

## Consequences

- New file `internal/searxng/truncate.go` defines the exported
  `TruncateRunes(s string, maxRunes int) string` helper. The package
  gains a trivial public surface, but the helper is well within the
  package's existing responsibility (string utilities for result
  processing).
- New file `internal/searxng/truncate_internal_test.go` carries the
  consolidated unit tests (empty / zero / negative limit, shorter /
  exact / longer, ASCII, CJK, emoji, mixed, no-truncation). The
  previously duplicated `TestTruncateRunes` in `format_test.go` is
  removed; behavior is locked in one place.
- `format.go` no longer defines `truncateRunes`. Both call sites in
  `formatResults` (infobox content and result content) call
  `searxng.TruncateRunes`.
- `internal/searxng/deduplicate.go` no longer defines
  `truncateAtRunes`. `answerPrefixMatch` calls `TruncateRunes` for
  the 200-rune prefix and the lowercased 200-rune prefix.
- `MaxContentRunes` is unchanged in value (4000). Its docstring is
  rewritten to clarify the CLI-only scope. `CONTEXT.md` is updated
  to match.
- No changes to `formatResults` output, the golden fixture
  (`golden_capture_test.go`), or to the typed `SearchResponse`
  produced by `normalizeResponse`. The CLI behavior and the
  JSON / MCP behavior are both unchanged.
- If a future ADR decides to bound JSON / MCP output, it should
  introduce a new constant rather than widen `MaxContentRunes`.
