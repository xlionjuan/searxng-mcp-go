package searxng

import "searxng-mcp-go/internal/searxng/answer"

// TruncateRunes returns s truncated to at most maxRunes runes. The rune-safe
// implementation lives in the answer subpackage so the answer deduplication
// logic can use it without circular imports.
//
// This function is the single source of truth for rune-safe truncation. It is
// used both by the internal deduplication prefix match (searxng package) and
// by the CLI text formatter (root package) for content-length bounding per
// MaxContentRunes.
func TruncateRunes(s string, maxRunes int) string {
	return answer.TruncateRunes(s, maxRunes)
}
