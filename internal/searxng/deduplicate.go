package searxng

import "searxng-mcp-go/internal/searxng/answer"

// deduplicateAnswers filters out DuckDuckGo answers whose text is a prefix
// (substring) of any infobox content. This is a thin wrapper that extracts
// infobox content strings and delegates to the answer subpackage.
//
// Only answers with Engine "duckduckgo" are checked. When dedup runs (at
// least one non-empty infobox exists), answers with empty text are dropped
// before the engine gate; otherwise the original answers are returned
// unchanged. Non-empty non-DuckDuckGo answers, including those with unknown
// Engine, pass through.
func deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer {
	contents := make([]string, 0, len(infoboxes))
	for _, ib := range infoboxes {
		if ib.Content != "" {
			contents = append(contents, ib.Content)
		}
	}

	return answer.DeduplicateAnswers(answers, contents)
}
