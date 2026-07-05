package answer_test

import (
	"testing"

	"searxng-mcp-go/internal/searxng/answer"
)

func TestWeatherMeasure_String(t *testing.T) {
	t.Run("zero value returns empty", func(t *testing.T) {
		m := answer.WeatherMeasure{}

		if got := m.String(); got != "" {
			t.Errorf("WeatherMeasure{}.String() = %q, want empty", got)
		}
	})

	t.Run("value only", func(t *testing.T) {
		m := answer.WeatherMeasure{Val: 42.5}

		if got := m.String(); got != "42.5" {
			t.Errorf("WeatherMeasure{42.5}.String() = %q, want %q", got, "42.5")
		}
	})

	t.Run("value with unit", func(t *testing.T) {
		m := answer.WeatherMeasure{Val: 11.2, Unit: "°C"}

		if got := m.String(); got != "11.2 °C" {
			t.Errorf("WeatherMeasure{11.2, °C}.String() = %q, want %q", got, "11.2 °C")
		}
	})

	t.Run("value zero with unit", func(t *testing.T) {
		m := answer.WeatherMeasure{Val: 0, Unit: "°C"}

		if got := m.String(); got != "0 °C" {
			t.Errorf("WeatherMeasure{0, °C}.String() = %q, want %q", got, "0 °C")
		}
	})

	t.Run("negative value", func(t *testing.T) {
		m := answer.WeatherMeasure{Val: -5.0, Unit: "°C"}

		if got := m.String(); got != "-5 °C" {
			t.Errorf("WeatherMeasure{-5, °C}.String() = %q, want %q", got, "-5 °C")
		}
	})

	t.Run("integer value", func(t *testing.T) {
		m := answer.WeatherMeasure{Val: 25, Unit: "°C"}

		if got := m.String(); got != "25 °C" {
			t.Errorf("WeatherMeasure{25, °C}.String() = %q, want %q", got, "25 °C")
		}
	})
}
