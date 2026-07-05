package answer_test

import (
	"testing"

	"searxng-mcp-go/internal/searxng/answer"
)

func TestTranslationAnswerFallback_NoTranslations(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{}

	if got := answer.TranslationAnswerFallback(a); got != "" {
		t.Fatalf("TranslationAnswerFallback() = %q, want empty string", got)
	}
}

func TestTranslationAnswerFallback_WithTranslations(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Translations: []answer.TranslationItem{
			{Text: "bonjour"},
			{Text: "salut"},
		},
	}
	got := answer.TranslationAnswerFallback(a)

	if got != "Translation: bonjour; salut" {
		t.Fatalf("TranslationAnswerFallback() = %q, want %q", got, "Translation: bonjour; salut")
	}
}

func TestTranslationAnswerFallback_SkipsEmptyText(t *testing.T) {
	t.Parallel()

	t.Run("some empty", func(t *testing.T) {
		t.Parallel()

		a := &answer.Answer{
			Translations: []answer.TranslationItem{
				{Text: ""},
				{Text: "hello"},
			},
		}
		got := answer.TranslationAnswerFallback(a)

		if got != "Translation: hello" {
			t.Fatalf("TranslationAnswerFallback() = %q, want %q", got, "Translation: hello")
		}
	})

	t.Run("all empty", func(t *testing.T) {
		t.Parallel()

		a := &answer.Answer{
			Translations: []answer.TranslationItem{
				{Text: "   "},
			},
		}

		if got := answer.TranslationAnswerFallback(a); got != "" {
			t.Fatalf("TranslationAnswerFallback() = %q, want empty string", got)
		}
	})
}

func TestWeatherAnswerFallback_NilCurrent(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{}

	if got := answer.WeatherAnswerFallback(a); got != "" {
		t.Fatalf("WeatherAnswerFallback() = %q, want empty string", got)
	}
}

func TestWeatherAnswerFallback_WithSummary(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Current: &answer.WeatherItem{
			Summary: "Partly cloudy throughout the day.",
		},
	}
	got := answer.WeatherAnswerFallback(a)

	if got != "Partly cloudy throughout the day." {
		t.Fatalf("WeatherAnswerFallback() = %q, want summary", got)
	}
}

func TestWeatherAnswerFallback_BuildsFromComponents(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Current: &answer.WeatherItem{
			Location:    answer.WeatherLocation{Name: "Berlin"},
			Temperature: answer.WeatherMeasure{Val: 11.2, Unit: "°C"},
			Condition:   "partly cloudy",
		},
	}
	got := answer.WeatherAnswerFallback(a)

	if got != "Weather: Berlin, 11.2 °C, partly cloudy" {
		t.Fatalf("WeatherAnswerFallback() = %q, want %q", got, "Weather: Berlin, 11.2 °C, partly cloudy")
	}
}

func TestWeatherAnswerFallback_EmptyLocation(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Current: &answer.WeatherItem{
			Temperature: answer.WeatherMeasure{Val: 25.0, Unit: "°C"},
			Condition:   "sunny",
		},
	}
	got := answer.WeatherAnswerFallback(a)

	if got != "Weather: 25 °C, sunny" {
		t.Fatalf("WeatherAnswerFallback() = %q, want %q", got, "Weather: 25 °C, sunny")
	}
}

func TestWeatherAnswerFallback_EmptyComponents(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Current: &answer.WeatherItem{
			Location:    answer.WeatherLocation{Name: ""},
			Temperature: answer.WeatherMeasure{},
			Condition:   "",
		},
	}

	if got := answer.WeatherAnswerFallback(a); got != "" {
		t.Fatalf("WeatherAnswerFallback() = %q, want empty string", got)
	}
}

func TestEnsureAnswerFallback_NonEmptyNotOverwritten(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{Answer: "existing answer"}
	answer.EnsureAnswerFallback(a)

	if a.Answer != "existing answer" {
		t.Fatalf("Answer = %q, want %q", a.Answer, "existing answer")
	}
}

func TestEnsureAnswerFallback_TranslationFallback(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Translations: []answer.TranslationItem{{Text: "bonjour"}},
	}
	answer.EnsureAnswerFallback(a)

	if a.Answer != "Translation: bonjour" {
		t.Fatalf("Answer = %q, want %q", a.Answer, "Translation: bonjour")
	}
}

func TestEnsureAnswerFallback_WeatherFallback(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Current: &answer.WeatherItem{
			Location:    answer.WeatherLocation{Name: "Paris"},
			Temperature: answer.WeatherMeasure{Val: 15.0, Unit: "°C"},
			Condition:   "clear",
		},
	}
	answer.EnsureAnswerFallback(a)

	want := "Weather: Paris, 15 °C, clear"

	if a.Answer != want {
		t.Fatalf("Answer = %q, want %q", a.Answer, want)
	}
}

func TestEnsureAnswerFallback_NoFallbackStaysEmpty(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{}
	answer.EnsureAnswerFallback(a)

	if a.Answer != "" {
		t.Fatalf("Answer = %q, want empty", a.Answer)
	}
}

func TestEnsureAnswerFallback_WhitespaceOnlyTreatedAsEmpty(t *testing.T) {
	t.Parallel()

	a := &answer.Answer{
		Answer:       "   ",
		Translations: []answer.TranslationItem{{Text: "hola"}},
	}
	answer.EnsureAnswerFallback(a)

	if a.Answer != "Translation: hola" {
		t.Fatalf("Answer = %q, want %q", a.Answer, "Translation: hola")
	}
}
