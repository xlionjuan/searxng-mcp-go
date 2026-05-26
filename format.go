package main

import (
	"log/slog"
	"strconv"
	"strings"

	"searxng-mcp-go/internal/searxng"
)

const noResultsFound = "No results found."

// ============================================================================
// Formatting
// ============================================================================

// truncateRunes truncates s to at most limit runes in a single pass.
// It returns the original string unchanged if already within the limit.
func truncateRunes(str string, limit int) string {
	if limit <= 0 || str == "" {
		return ""
	}

	runeCount := 0
	for i := range str {
		if runeCount == limit {
			return str[:i]
		}

		runeCount++
	}

	return str
}

// writeAnswers writes formatted direct answers to b.
func writeAnswers(buf *strings.Builder, answers []searxng.Answer) {
	if len(answers) == 0 {
		return
	}

	buf.WriteString("=== Answers ===\n\n")

	for i, ans := range answers {
		buf.WriteByte('[')
		buf.WriteString(strconv.Itoa(i + 1))
		buf.WriteString("] ")
		buf.WriteString(ans.Answer)
		buf.WriteByte('\n')

		if ans.Engine != "" {
			buf.WriteString("    Engine: ")
			buf.WriteString(ans.Engine)
			buf.WriteByte('\n')
		}
	}

	buf.WriteByte('\n')
}

// writeInfoboxes writes formatted infoboxes to b.
func writeInfoboxes(buf *strings.Builder, infoboxes []searxng.Infobox) {
	if len(infoboxes) == 0 {
		return
	}

	buf.WriteString("=== Infoboxes ===\n\n")

	for idx, ib := range infoboxes {
		buf.WriteByte('[')
		buf.WriteString(strconv.Itoa(idx + 1))
		buf.WriteString("] ")
		buf.WriteString(searxng.UnescapeIfNeeded(ib.Infobox))
		buf.WriteByte('\n')

		if ib.Content != "" {
			content := searxng.UnescapeIfNeeded(ib.Content)
			content = truncateRunes(content, searxng.MaxContentRunes)

			buf.WriteString("    ")
			buf.WriteString(content)
			buf.WriteByte('\n')
		}

		if len(ib.Attributes) > 0 {
			buf.WriteString("    Attributes:\n")

			for _, attr := range ib.Attributes {
				buf.WriteString("      - ")
				buf.WriteString(attr.Label)
				buf.WriteString(": ")
				buf.WriteString(attr.Value)
				buf.WriteByte('\n')
			}
		}

		if len(ib.URLs) > 0 {
			buf.WriteString("    URLs:\n")

			for _, u := range ib.URLs {
				buf.WriteString("      - ")
				buf.WriteString(u.Title)
				buf.WriteString(": ")
				buf.WriteString(u.URL)
				buf.WriteByte('\n')
			}
		}

		if idx < len(infoboxes)-1 {
			buf.WriteByte('\n')
		}
	}

	buf.WriteByte('\n')
}

func logUnresponsiveEngines(resp *searxng.SearchResponse) {
	if resp == nil || !resp.Debug || len(resp.UnresponsiveEngines) == 0 {
		return
	}

	const unresponsiveEngineEntryFields = 2

	for _, entry := range resp.UnresponsiveEngines {
		if len(entry) < unresponsiveEngineEntryFields {
			slog.Debug("unresponsive engine", "entry", entry)

			continue
		}

		slog.Debug("unresponsive engine", "engine", entry[0], "error", entry[1])
	}
}

// formatResults formats search results as a readable string.
func formatResults(resp *searxng.SearchResponse) string {
	logUnresponsiveEngines(resp)

	if resp == nil {
		return noResultsFound
	}

	if len(resp.Results) == 0 && len(resp.Infoboxes) == 0 && len(resp.Answers) == 0 && len(resp.Suggestions) == 0 {
		return noResultsFound
	}

	var buf strings.Builder

	estimate := len(resp.Query) + len(resp.Results)*searxng.ResultSizeEstimate
	if estimate > 0 {
		buf.Grow(estimate)
	}

	// Answers first (direct answers like IP, hash, timezone)
	writeAnswers(&buf, resp.Answers)

	// Infoboxes
	writeInfoboxes(&buf, resp.Infoboxes)

	// Results
	if len(resp.Results) > 0 {
		buf.WriteString("=== Results ===\n\n")

		total := resp.NumberOfResults
		if total == 0 {
			total = len(resp.Results)
		}

		buf.WriteString("Found ")
		buf.WriteString(strconv.Itoa(total))
		buf.WriteString(" results for '")
		buf.WriteString(searxng.UnescapeIfNeeded(resp.Query))
		buf.WriteString("':\n\n")

		for idx, res := range resp.Results {
			title := searxng.UnescapeIfNeeded(res.Title)

			buf.WriteString(strconv.Itoa(idx + 1))
			buf.WriteString(". ")
			buf.WriteString(title)
			buf.WriteByte('\n')
			buf.WriteString("   URL: ")
			buf.WriteString(res.URL)
			buf.WriteByte('\n')

			if res.Content != "" {
				content := searxng.UnescapeIfNeeded(res.Content)
				content = truncateRunes(content, searxng.MaxContentRunes)

				buf.WriteString("   Summary: ")
				buf.WriteString(content)
				buf.WriteByte('\n')
			}

			if res.PublishedDate != nil && *res.PublishedDate != "" {
				buf.WriteString("   Published date: ")
				buf.WriteString(*res.PublishedDate)
				buf.WriteByte('\n')
			}

			buf.WriteString("   Engine: ")
			buf.WriteString(res.Engine)
			buf.WriteString("\n\n")
		}
	}

	// Suggestions last
	if len(resp.Suggestions) > 0 {
		buf.WriteString("=== Search Suggestions ===\n\n")

		for _, sug := range resp.Suggestions {
			buf.WriteString("  - ")
			buf.WriteString(sug)
			buf.WriteByte('\n')
		}
	}

	return buf.String()
}
