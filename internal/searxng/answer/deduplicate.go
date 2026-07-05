package answer

import "strings"

const dedupPrefixRunes = 200

// DeduplicateAnswers filters out answers whose text is a prefix (substring)
// of any infobox content. For each answer, the exact-case match runs first;
// the lowercase index is built lazily on the first miss and reused for
// subsequent answers that also miss the exact-case check.
//
// This function is DuckDuckGo-scoped: it strips " More at Wikipedia" suffixes
// from answer text and matches against infobox content, a pattern specific to
// DuckDuckGo response formatting.
func DeduplicateAnswers(answers []Answer, infoboxContents []string) []Answer {
	// Move collectInfoboxText (content filtering) inline.
	filteredContents := make([]string, 0, len(infoboxContents))
	for _, text := range infoboxContents {
		if text != "" {
			filteredContents = append(filteredContents, text)
		}
	}

	if len(answers) == 0 || len(filteredContents) == 0 {
		return answers
	}

	var lowerInfoboxTexts []string

	filtered := make([]Answer, 0, len(answers))
	for _, ans := range answers {
		if ans.Answer == "" {
			continue
		}

		if answerPrefixMatch(ans.Answer, filteredContents, false) {
			continue
		}

		if lowerInfoboxTexts == nil {
			lowerInfoboxTexts = buildLowerInfoboxTexts(filteredContents)
		}

		if answerPrefixMatch(ans.Answer, lowerInfoboxTexts, true) {
			continue
		}

		filtered = append(filtered, ans)
	}

	return filtered
}

func buildLowerInfoboxTexts(infoboxTexts []string) []string {
	lower := make([]string, 0, len(infoboxTexts))
	for _, text := range infoboxTexts {
		lower = append(lower, strings.ToLower(text))
	}

	return lower
}

// answerPrefixMatch reports whether the answer (or its lowercased form when
// lower is true), with the " More at Wikipedia" suffix stripped and
// truncated to dedupPrefixRunes characters, is a substring of any element
// of haystack.
func answerPrefixMatch(answer string, haystack []string, lower bool) bool {
	needle := answer
	suffix := " More at Wikipedia"

	if lower {
		needle = strings.ToLower(needle)
		suffix = " more at wikipedia"
	}

	needle = strings.TrimSuffix(needle, suffix)
	needle = TruncateRunes(needle, dedupPrefixRunes)

	for _, text := range haystack {
		if strings.Contains(text, needle) {
			return true
		}
	}

	return false
}
