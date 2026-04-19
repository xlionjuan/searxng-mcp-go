# ADR-006: Expose `unresponsive_engines` Only in Debug Mode

Date: 2026-04-19
Status: Accepted

## Context

SearXNG's JSON response includes an `unresponsive_engines` field listing engines that failed to respond (e.g., rate-limited, CAPTCHA, connection errors). During code review, it was identified that `SearchResponse` does not model this field.

## Decision

**Expose `unresponsive_engines` in the response, but only when debug mode is enabled.**

In normal output (CLI and MCP), the field is omitted. When `--debug` or `DEBUG=1` is set, the field is included in JSON output and logged in CLI output.

## Rationale

1. **Useful for troubleshooting, not for end users**: Regular users and AI agents do not need to know which engines failed. This is diagnostic information.
2. **Common in practice**: Engines like Brave (rate-limited), Startpage (CAPTCHA), and Yahoo frequently appear as unresponsive on public instances. Suppressing this entirely loses diagnostic value.
3. **Debug mode is the right filter**: Debug mode already shows HTTP request/response details. Including engine failures here is consistent with the debug UX.

## Consequences

- `SearchResponse` gains an `UnresponsiveEngines` field with `json:",omitempty"` tag.
- `performSearch()` or `formatResults()` conditionally includes/excludes the field based on debug mode.
- CLI debug output logs unresponsive engines.
- MCP JSON output includes the field only when debug is active.
