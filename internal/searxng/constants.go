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

// MaxContentRunes is the maximum result content length retained after normalization.
const MaxContentRunes = 4000

// MaxErrorBodySize is the maximum response body size read for error handling.
const MaxErrorBodySize = 100 * 1024

// MaxResponseBodySize is the maximum successful search response body size.
const MaxResponseBodySize = 2 * 1024 * 1024

// MaxErrorDisplayChars caps the size of error body previews retained on
// SearXNGError.ResponseBody (and surfaced in debug logs and error messages).
//
// Despite the "Chars" suffix, this is a byte ceiling, not a rune/character
// count: callers truncate with truncateBytesToValidUTF8, which slices at
// most MaxErrorDisplayChars bytes and walks back to a valid UTF-8 rune
// boundary so multi-byte sequences are never split. The "Chars" name
// follows the display-preview convention used by DebugBodyPreviewChars.
const MaxErrorDisplayChars = 200

// ResultSizeEstimate is the approximate JSON result size used for response preallocation.
const ResultSizeEstimate = 200

// DebugBodyPreviewChars is the maximum response body preview length emitted in debug logs.
const DebugBodyPreviewChars = 500
