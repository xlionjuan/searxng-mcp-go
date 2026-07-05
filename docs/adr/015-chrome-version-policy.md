# ADR-015: Chrome Version Extraction and Update Policy

**Status:** Accepted
**Date:** 2026-07-05

## Context

`setBrowserHeaders()` in `internal/searxng/request.go` hardcodes Chrome/147 in
both the User-Agent header and the Sec-Ch-Ua header. Every browser version bump
requires editing two separate string literals, and the version number (147)
appears in the UA twice and in Sec-Ch-Ua three times — they can easily drift on
update. There is no documented policy for when or how to update.

Other headers (Sec-Fetch-*, Accept, Accept-Language, Priority) are
version-independent and should not be extracted.

## Decision

Introduce a package-level `var DefaultChromeVersion = "147"` and derive
`DefaultUserAgent` by concatenating it:

```go
var DefaultChromeVersion = "147"

var DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64)" +
    " AppleWebKit/537.36 (KHTML, like Gecko)" +
    " Chrome/" + DefaultChromeVersion + ".0.0.0 Safari/537.36"
```

`setBrowserHeaders` references `DefaultUserAgent` and `DefaultChromeVersion` so
that changing one number keeps both headers in sync.

**Update policy:** Track the latest stable Chrome major version. Update when
SearXNG's limiter / bot-detection starts blocking the current version. The
following evidence confirms a block:

- Consistent HTTP 429 responses from SearXNG.
- SearXNG logs showing the `limiter` module rejecting requests from this
  server's IP with a reason matching known bot-detection patterns.

Without evidence of blocking, do not bump.

## Consequences

- **Single source of truth.** A version bump changes only `DefaultChromeVersion`.
- **No env var or config field.** The version is a package-level var, easily
  overridable in tests via `init()` or `TestMain`.
- **ADR answers "when to bump".** No guesswork needed; blocking evidence is
  required.
- **Backward compatible.** Behavior is unchanged; only the source-of-truth
  moves.
