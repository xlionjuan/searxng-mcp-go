# ADR-009: Neutralize Terminal Control Sequences in CLI Text Output

**Status:** Accepted
**Date:** 2026-06-04

## Context

CLI text mode (`formatResults` in `format.go`) writes untrusted SearXNG
result fields directly to the terminal after HTML entity unescaping
(`UnescapeIfNeeded`). If an upstream result contains ANSI or OSC control
bytes — including the HTML-entity-encoded form `&#x1b;`, which
`html.UnescapeString` decodes to a literal ESC — the program emits them
as terminal control sequences. This can alter terminal display state
and, in terminals that support OSC 52, may attempt clipboard writes.

The vulnerability was filed as a low-severity security finding
(CAND-33fe0b85-RUNTIME-001) with proof-of-concept ESC bytes at byte
offsets 221, 293, and 301 of formatted output.

The affected surface is the **terminal** — JSON output goes through
`encoding/json`, which already escapes control bytes, and MCP responses
reuse the same JSON path. The data model returned to MCP consumers does
not need to change.

## Decision

`formatResults` will run every user-controlled text field through a new
`sanitizeTerminalControl` helper in `format.go` that rewrites C0
controls (other than `\t`, `\n`, `\r`), DEL, and C1 controls
(U+0080..U+009F) into visible `\xNN` escape sequences. All other
codepoints — including CJK, emoji, and accented Latin — are preserved
unchanged. Invalid UTF-8 bytes are also rewritten byte-by-byte.

The sanitizer is scoped to the text-format layer. The data model and
the JSON path are untouched.

## Rationale

1. **Localized to the affected surface.** The data model is unchanged,
   so MCP consumers and downstream JSON parsers see exactly the same
   bytes they saw before. The fix targets only the rendering path that
   actually writes to a terminal.
2. **Visible encoding > silent stripping.** A user reviewing a captured
   transcript can still see `\x1b[31mred` rather than a silent gap; the
   upstream payload survives in a safe form. JSON encoding already
   uses this same visible-escape convention.
3. **Preserves ordinary Unicode.** CJK, emoji, and Latin diacritics
   round-trip byte-for-byte. The only codepoints rewritten are those
   that can act as terminal control sequences.
4. **Defense in depth.** C0 controls other than ESC (BEL, BS, VT, FF,
   SO, SI, DLE..US) and the C1 control range are also rewritten. The
   cost is trivial and the safety margin against future payloads
   improves.
5. **Fast path keeps the common case cheap.** A `hasUnsafeControlBytes`
   pre-scan returns the input string unchanged when no rewrite is
   needed, so legitimate result text does not allocate a new buffer.

## Consequences

- A new helper `sanitizeTerminalControl` and a pre-scan helper
  `hasUnsafeControlBytes` live in `format.go` next to the formatter
  that uses them. The internal `searxng` package is not modified.
- Golden capture (`golden_capture_test.go`) is unaffected because the
  golden fixture contains no control bytes.
- New unit tests in `format_sanitize_test.go` cover ANSI, OSC 52, DCS,
  CSI, C0, C1, DEL, invalid UTF-8, HTML-entity-decoded ESC, and
  Unicode preservation. New `formatResults`-level tests in
  `format_test.go` lock end-to-end behavior for results, infoboxes,
  answers, suggestions, and query echo.
- `docs/OUTPUT_FORMAT.md` documents the sanitizer and the JSON / MCP
  unchanged-behavior contract.
- If a future requirement needs to render ANSI color (unlikely for
  untrusted upstream content), this decision should be revisited with a
  TTY-only allowlist and an opt-out flag.
