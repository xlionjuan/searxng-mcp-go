package searxng

import "searxng-mcp-go/internal/searxng/answer"

// deduplicateAnswers filters out answers whose text is a prefix (substring)
// of any infobox content. This is a thin wrapper that extracts infobox content
// strings and delegates to the answer subpackage.
//
// This function is DuckDuckGo-scoped: it strips " More at Wikipedia" suffixes
// from answer text and matches against infobox content, a pattern specific to
// DuckDuckGo response formatting.
func deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer {
	contents := make([]string, 0, len(infoboxes))
	for _, ib := range infoboxes {
		if ib.Content != "" {
			contents = append(contents, ib.Content)
		}
	}

	return answer.DeduplicateAnswers(answers, contents)
}
