// Package searxng provides a client library for interacting with SearXNG search instances.
package searxng

import "time"

const DefaultTimeout = 8 * time.Second

const DefaultMaxRetries = 5

const DefaultRetryDelay = time.Second

const DefaultMaxRetryDelay = 30 * time.Second

const MaxContentRunes = 4000

const MaxErrorBodySize = 100 * 1024

const MaxResponseBodySize = 2 * 1024 * 1024

const MaxErrorDisplayChars = 200

const ResultSizeEstimate = 200

const DebugBodyPreviewChars = 500
