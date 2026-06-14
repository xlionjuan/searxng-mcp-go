package main

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"searxng-mcp-go/internal/searxng"
)

const noResultsFound = "No results found."

// sanitizeTerminalControl replaces C0 control bytes, DEL, and C1 control
// codepoints (other than the common whitespace \t and \n) with visible
// "\xNN" escape sequences. This neutralizes ANSI / OSC / DCS terminal
// control sequences from upstream search content that could otherwise
// alter terminal display state or trigger side effects such as OSC 52
// clipboard writes. All other codepoints, including CJK and emoji, are
// preserved unchanged.
//
// The sanitizer is scoped to CLI text formatting; JSON output is emitted
// through encoding/json, which already escapes control bytes, and MCP
// responses reuse that JSON path. Keeping the sanitizer at the format
// layer preserves the structured data model and avoids changing
// JSON-mode behavior.
func sanitizeTerminalControl(s string) string {
	if !hasUnsafeControlBytes(s) {
		return s
	}

	const rewriteReserve = 4

	var buf strings.Builder

	buf.Grow(len(s) + rewriteReserve)

	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			switch {
			case b == '\t' || b == '\n':
				buf.WriteByte(b)
			case b < 0x20 || b == 0x7F:
				fmt.Fprintf(&buf, `\x%02x`, b)
			default:
				buf.WriteByte(b)
			}

			i++

			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			fmt.Fprintf(&buf, `\x%02x`, b)

			i++

			continue
		}

		if r >= 0x80 && r <= 0x9F {
			fmt.Fprintf(&buf, `\x%02x`, r)
		} else {
			buf.WriteRune(r)
		}

		i += size
	}

	return buf.String()
}

// hasUnsafeControlBytes reports whether s contains any byte or codepoint
// that sanitizeTerminalControl would rewrite. It is a fast-path scan that
// lets the common (clean) case return the original string without
// allocating a new buffer.
func hasUnsafeControlBytes(s string) bool {
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			if (b < 0x20 && b != '\t' && b != '\n') || b == 0x7F {
				return true
			}

			i++

			continue
		}

		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			return true
		}

		if r >= 0x80 && r <= 0x9F {
			return true
		}

		i += size
	}

	return false
}

// ============================================================================
// Formatting
// ============================================================================

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
		buf.WriteString(sanitizeTerminalControl(ans.Answer))
		buf.WriteByte('\n')

		if ans.Engine != "" {
			buf.WriteString("    Engine: ")
			buf.WriteString(sanitizeTerminalControl(ans.Engine))
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
		buf.WriteString(sanitizeTerminalControl(searxng.UnescapeIfNeeded(ib.Infobox)))
		buf.WriteByte('\n')

		if ib.Content != "" {
			content := searxng.UnescapeIfNeeded(ib.Content)
			content = searxng.TruncateRunes(content, searxng.MaxContentRunes)
			content = sanitizeTerminalControl(content)

			buf.WriteString("    ")
			buf.WriteString(content)
			buf.WriteByte('\n')
		}

		if len(ib.Attributes) > 0 {
			buf.WriteString("    Attributes:\n")

			for _, attr := range ib.Attributes {
				buf.WriteString("      - ")
				buf.WriteString(sanitizeTerminalControl(attr.Label))
				buf.WriteString(": ")
				buf.WriteString(sanitizeTerminalControl(attr.Value))
				buf.WriteByte('\n')
			}
		}

		if len(ib.URLs) > 0 {
			buf.WriteString("    URLs:\n")

			for _, u := range ib.URLs {
				buf.WriteString("      - ")
				buf.WriteString(sanitizeTerminalControl(u.Title))
				buf.WriteString(": ")
				buf.WriteString(sanitizeTerminalControl(u.URL))
				buf.WriteByte('\n')
			}
		}

		if idx < len(infoboxes)-1 {
			buf.WriteByte('\n')
		}
	}

	buf.WriteByte('\n')
}

// writeSuggestions writes formatted search suggestions to buf.
func writeSuggestions(buf *strings.Builder, suggestions []string) {
	if len(suggestions) == 0 {
		return
	}

	buf.WriteString("=== Search Suggestions ===\n\n")

	for _, sug := range suggestions {
		buf.WriteString("  - ")
		buf.WriteString(sanitizeTerminalControl(sug))
		buf.WriteByte('\n')
	}
}

// writeSingleResult writes a single search result entry to buf.
func writeSingleResult(buf *strings.Builder, idx int, res searxng.SearchResult) {
	title := searxng.UnescapeIfNeeded(res.Title)

	buf.WriteString(strconv.Itoa(idx))
	buf.WriteString(". ")
	buf.WriteString(sanitizeTerminalControl(title))
	buf.WriteByte('\n')
	buf.WriteString("   URL: ")
	buf.WriteString(sanitizeTerminalControl(res.URL))
	buf.WriteByte('\n')

	if res.Content != "" {
		content := searxng.UnescapeIfNeeded(res.Content)
		content = searxng.TruncateRunes(content, searxng.MaxContentRunes)
		content = sanitizeTerminalControl(content)

		buf.WriteString("   Summary: ")
		buf.WriteString(content)
		buf.WriteByte('\n')
	}

	if res.PublishedDate != nil && *res.PublishedDate != "" {
		buf.WriteString("   Published date: ")
		buf.WriteString(sanitizeTerminalControl(*res.PublishedDate))
		buf.WriteByte('\n')
	}

	buf.WriteString("   Engine: ")
	buf.WriteString(sanitizeTerminalControl(res.Engine))
	buf.WriteString("\n\n")
}

// writeResultsSection writes the formatted results header and individual
// result entries to buf. It is a no-op when results is empty.
func writeResultsSection(buf *strings.Builder, results []searxng.SearchResult, query string, numberOfResults int) {
	if len(results) == 0 {
		return
	}

	buf.WriteString("=== Results ===\n\n")

	nResults := len(results)
	total := numberOfResults

	if total == 0 {
		total = nResults
	}

	buf.WriteString("Found ")

	if total != nResults {
		buf.WriteString(strconv.Itoa(total))
		buf.WriteString(" total (showing ")
		buf.WriteString(strconv.Itoa(nResults))
		buf.WriteString(") results for '")
	} else {
		buf.WriteString(strconv.Itoa(total))
		buf.WriteString(" results for '")
	}

	buf.WriteString(sanitizeTerminalControl(searxng.UnescapeIfNeeded(query)))
	buf.WriteString("':\n\n")

	for idx, res := range results {
		writeSingleResult(buf, idx+1, res)
	}
}

func logUnresponsiveEngines(logger *slog.Logger, resp *searxng.SearchResponse) {
	if resp == nil || !resp.Debug || len(resp.UnresponsiveEngines) == 0 {
		return
	}

	const unresponsiveEngineEntryFields = 2

	for _, entry := range resp.UnresponsiveEngines {
		if len(entry) < unresponsiveEngineEntryFields {
			logger.Debug("unresponsive engine", "entry", entry)

			continue
		}

		logger.Debug("unresponsive engine", "engine", entry[0], "error", entry[1])
	}
}

// formatResults formats search results as a readable string.
func formatResults(resp *searxng.SearchResponse) string {
	prefix := "=== Web Search Results ===\nWarning: " + searxng.ExternalContentWarning + "\n\n"

	if resp == nil {
		return prefix + noResultsFound
	}

	if len(resp.Results) == 0 && len(resp.Infoboxes) == 0 && len(resp.Answers) == 0 && len(resp.Suggestions) == 0 {
		return prefix + noResultsFound
	}

	var buf strings.Builder

	estimate := len(resp.Query) + len(resp.Results)*searxng.ResultSizeEstimate
	if estimate > 0 {
		buf.Grow(estimate)
	}

	buf.WriteString(prefix)

	writeAnswers(&buf, resp.Answers)
	writeInfoboxes(&buf, resp.Infoboxes)
	writeResultsSection(&buf, resp.Results, resp.Query, resp.NumberOfResults)
	writeSuggestions(&buf, resp.Suggestions)

	return buf.String()
}
