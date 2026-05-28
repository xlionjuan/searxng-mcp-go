package searxng

import "strings"

// deduplicateAnswers filters out answers whose text is a prefix (substring)
// of any infobox content.
func deduplicateAnswers(answers []Answer, infoboxes []Infobox) []Answer {
	if len(answers) == 0 || len(infoboxes) == 0 {
		return answers
	}

	infoboxTexts, lowerInfoboxTexts := collectInfoboxText(infoboxes)
	if len(infoboxTexts) == 0 {
		return answers
	}

	filtered := make([]Answer, 0, len(answers))
	for _, ans := range answers {
		if ans.Answer == "" {
			continue
		}

		if answerPrefixMatch(ans.Answer, infoboxTexts, lowerInfoboxTexts) {
			continue
		}

		filtered = append(filtered, ans)
	}

	return filtered
}

func collectInfoboxText(infoboxes []Infobox) ([]string, []string) {
	infoboxTexts := make([]string, 0, len(infoboxes))
	lowerInfoboxTexts := make([]string, 0, len(infoboxes))

	for _, ib := range infoboxes {
		if ib.Content != "" {
			infoboxTexts = append(infoboxTexts, ib.Content)
			lowerInfoboxTexts = append(lowerInfoboxTexts, strings.ToLower(ib.Content))
		}
	}

	return infoboxTexts, lowerInfoboxTexts
}

// truncateAtRunes returns s truncated to at most maxRunes runes.
// It never splits a multi-byte UTF-8 rune.
func truncateAtRunes(s string, maxRunes int) string {
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

func answerPrefixMatch(answer string, infoboxTexts []string, lowerInfoboxTexts []string) bool {
	const prefixLen = 200

	prefix := strings.TrimSuffix(answer, " More at Wikipedia")
	prefix = truncateAtRunes(prefix, prefixLen)

	for _, text := range infoboxTexts {
		if strings.Contains(text, prefix) {
			return true
		}
	}

	lowerAnswer := strings.ToLower(answer)
	lowerAnswer = strings.TrimSuffix(lowerAnswer, " more at wikipedia")

	lowerPrefix := truncateAtRunes(lowerAnswer, prefixLen)

	for _, text := range lowerInfoboxTexts {
		if strings.Contains(text, lowerPrefix) {
			return true
		}
	}

	return false
}
