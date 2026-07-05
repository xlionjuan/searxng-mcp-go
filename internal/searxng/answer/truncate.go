package answer

// TruncateRunes returns s truncated to at most maxRunes runes in a single pass
// over the underlying bytes. Multi-byte UTF-8 runes are never split: the
// function walks the string with `for i := range s` and slices on the byte
// offset of the rune boundary, which is the standard Go idiom for rune-safe
// truncation without an intermediate []rune conversion.
//
// The function returns the original string unchanged when its rune count is
// at or below maxRunes. A non-positive maxRunes returns the empty string,
// matching the pre-refactor behavior of the two callers it replaces.
//
// This helper is the single source of truth for rune-safe truncation. It is
// used both by the internal deduplication prefix match (answer package) and
// by the CLI text formatter (root package) for content-length bounding per
// MaxContentRunes.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}

	count := 0

	for i := range s {
		count++

		if count > maxRunes {
			return s[:i]
		}
	}

	return s
}
