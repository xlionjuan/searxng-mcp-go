package searxng

import "strconv"

// TranslationItem is a translation entry from a typed SearXNG answer.
type TranslationItem struct {
	Text            string   `json:"text"`
	Transliteration string   `json:"transliteration,omitempty"`
	Examples        []string `json:"examples,omitempty"`
	Definitions     []string `json:"definitions,omitempty"`
	Synonyms        []string `json:"synonyms,omitempty"`
}

// WeatherItem is a current or forecast weather entry from a typed SearXNG answer.
type WeatherItem struct {
	Location    WeatherLocation `json:"location"`
	Datetime    *string         `json:"datetime,omitempty"`
	Summary     string          `json:"summary,omitempty"`
	Temperature WeatherMeasure  `json:"temperature"`
	FeelsLike   *WeatherMeasure `json:"feels_like,omitempty"`
	Condition   string          `json:"condition"`
	Pressure    *WeatherMeasure `json:"pressure,omitempty"`
	Humidity    *WeatherMeasure `json:"humidity,omitempty"`
	WindFrom    *WeatherMeasure `json:"wind_from,omitempty"`
	WindSpeed   *WeatherMeasure `json:"wind_speed,omitempty"`
	CloudCover  *int            `json:"cloud_cover,omitempty"`
}

// WeatherLocation describes the location for a weather answer.
type WeatherLocation struct {
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Elevation   float64 `json:"elevation,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
}

// WeatherMeasure is a numeric weather measurement and optional unit.
type WeatherMeasure struct {
	Val  float64 `json:"val"`
	Unit string  `json:"unit,omitempty"`
}

// String returns a compact representation of a weather measurement.
func (m WeatherMeasure) String() string {
	if m.Val == 0 && m.Unit == "" {
		return ""
	}

	value := strconv.FormatFloat(m.Val, 'f', -1, 64)
	if m.Unit == "" {
		return value
	}

	return value + " " + m.Unit
}
