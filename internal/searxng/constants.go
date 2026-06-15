// Package searxng provides a client library for interacting with SearXNG search instances.
package searxng

import "time"

// DefaultTimeout is the fallback HTTP client timeout for SearXNG requests.
const DefaultTimeout = 8 * time.Second

// DefaultMaxRetries is the fallback number of retries after the initial request.
const DefaultMaxRetries = 5

// DefaultRetryDelay is the fallback base delay for exponential backoff.
const DefaultRetryDelay = time.Second

// DefaultMaxRetryDelay is the fallback upper bound for retry backoff delays.
const DefaultMaxRetryDelay = 30 * time.Second

// DefaultResultLimit is the default value applied when callers omit a result
// limit. It is the single source of truth used by CLI flag defaulting, MCP
// handler defaulting, and the canonical ParamDef metadata in params.go.
var DefaultResultLimit = 10

// MaxContentRunes is the CLI text-mode formatting limit, in Unicode runes,
// applied to result `content` and infobox `content` fields by
// `formatResults` (root package). It is purely a rendering budget for
// terminal output: the typed `SearchResponse` returned by normalization
// and surfaced to JSON and MCP consumers is NOT truncated, so downstream
// consumers always see the full upstream text.
//
// The rune-safe truncation itself is implemented by `TruncateRunes`
// (truncate.go) so the searxng deduplication prefix match and the CLI
// formatter share a single helper. See docs/adr/011-max-content-runes-cli-only.md
// for the scope decision.
const MaxContentRunes = 4000

// MaxErrorBodySize is the maximum response body size read for error handling.
const MaxErrorBodySize = 100 * 1024

// MaxResponseBodySize is the maximum successful search response body size.
const MaxResponseBodySize = 2 * 1024 * 1024

// MaxAnswers caps the number of answers retained after JSON unmarshalling.
//
// The answer/infobox deduplication pass runs in O(len(answers)*len(infoboxes))
// and, for each answer, scans every infobox twice with strings.Contains.
// Real SearXNG responses contain at most a handful of each, so a generous cap
// keeps deduplication work bounded and prevents an adversarial or
// mis-configured upstream from forcing O(n*m) substring scans inside the
// MaxResponseBodySize envelope. Truncation is performed before
// deduplication and a warning is logged when it triggers.
const MaxAnswers = 100

// MaxInfoboxes caps the number of infoboxes retained after JSON
// unmarshalling. See MaxAnswers for the rationale; the cap exists to bound
// the answer/infobox deduplication work the same way.
const MaxInfoboxes = 100

// MaxErrorDisplayBytes caps the size of error body previews retained on
// SearXNGError.ResponseBody (and surfaced in debug logs and error messages).
//
// This is a byte ceiling, not a rune/character count: callers truncate with
// truncateBytesToValidUTF8, which slices at most MaxErrorDisplayBytes bytes
// and walks back to a valid UTF-8 rune boundary so multi-byte sequences are
// never split.
const MaxErrorDisplayBytes = 200

// ResultSizeEstimate is the approximate JSON result size used for response preallocation.
const ResultSizeEstimate = 200

// DebugBodyPreviewBytes is the maximum response body preview length emitted in debug logs.
const DebugBodyPreviewBytes = 500
