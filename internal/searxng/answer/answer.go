// Package answer provides the Answer type, typed answer types, and
// DuckDuckGo-specific answer/infobox deduplication for SearXNG responses.
//
// The package is internal to searxng-mcp-go and imported by the searxng
// parent package. No behavioral changes; all moved code preserves the
// parent package's public API surface.
package answer

import "strings"

const maxWeatherSummaryParts = 3

// Answer represents a direct answer from SearXNG.
//
// SearXNG has legacy string answers and typed answers. Typed answers such as
// translations and weather use template-specific fields and may omit the
// legacy "answer" string entirely. Display text derivation for typed answers
// is handled by the normalization layer.
type Answer struct {
	Answer       string            `json:"answer"`
	Engine       string            `json:"engine"`
	Template     string            `json:"template,omitempty"`
	URL          string            `json:"url,omitempty"`
	Translations []TranslationItem `json:"translations,omitempty"`
	Current      *WeatherItem      `json:"current,omitempty"`
	Forecasts    []WeatherItem     `json:"forecasts,omitempty"`
	Service      string            `json:"service,omitempty"`
}

// EnsureAnswerFallback derives a human-readable Answer string for known typed
// answers (translation, weather) that may omit the legacy "answer" field.
func EnsureAnswerFallback(a *Answer) {
	if strings.TrimSpace(a.Answer) != "" {
		return
	}

	if fallback := TranslationAnswerFallback(a); fallback != "" {
		a.Answer = fallback

		return
	}

	if fallback := WeatherAnswerFallback(a); fallback != "" {
		a.Answer = fallback
	}
}

// TranslationAnswerFallback returns a formatted translation string when the
// answer has translation items, or an empty string otherwise.
func TranslationAnswerFallback(a *Answer) string {
	if len(a.Translations) == 0 {
		return ""
	}

	parts := make([]string, 0, len(a.Translations))
	for _, item := range a.Translations {
		text := strings.TrimSpace(item.Text)
		if text != "" {
			parts = append(parts, text)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	return "Translation: " + strings.Join(parts, "; ")
}

// WeatherAnswerFallback returns a formatted weather string when the answer has
// a weather item with sufficient data, or an empty string otherwise.
func WeatherAnswerFallback(a *Answer) string {
	if a.Current == nil {
		return ""
	}

	current := a.Current
	if summary := strings.TrimSpace(current.Summary); summary != "" {
		return summary
	}

	parts := make([]string, 0, maxWeatherSummaryParts)
	if location := strings.TrimSpace(current.Location.Name); location != "" {
		parts = append(parts, location)
	}

	if temperature := current.Temperature.String(); temperature != "" {
		parts = append(parts, temperature)
	}

	if condition := strings.TrimSpace(current.Condition); condition != "" {
		parts = append(parts, condition)
	}

	if len(parts) == 0 {
		return ""
	}

	return "Weather: " + strings.Join(parts, ", ")
}
