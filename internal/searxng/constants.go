// Package searxng provides a client library for interacting with SearXNG search instances.
package searxng

import "time"

// DefaultTimeout is the default timeout for SearXNG HTTP requests.
const DefaultTimeout = 8 * time.Second

// DefaultMaxRetries is the default number of retries after the initial search attempt.
const DefaultMaxRetries = 5

// DefaultRetryDelay is the default base delay for retry backoff.
const DefaultRetryDelay = time.Second

// DefaultMaxRetryDelay is the maximum default delay between retry attempts.
const DefaultMaxRetryDelay = 30 * time.Second

// MaxContentRunes is the maximum number of UTF-8 runes in content truncation.
const MaxContentRunes = 4000

// MaxErrorBodySize is the maximum number of bytes to read from an error response body.
const MaxErrorBodySize = 100 * 1024

// MaxResponseBodySize is the maximum number of bytes to read from a response body.
const MaxResponseBodySize = 2 * 1024 * 1024

// MaxErrorDisplayChars is the maximum number of characters to display from an error body.
const MaxErrorDisplayChars = 200

// ResultSizeEstimate is an empirical estimate of bytes per result for pre-allocating the response buffer.
const ResultSizeEstimate = 200

// DebugBodyPreviewChars is the maximum number of characters logged for debug body previews.
const DebugBodyPreviewChars = 500
