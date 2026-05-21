package searxng

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Config holds the SearXNG configuration.
type Config struct {
	SearXNGURL string
	Timeout    time.Duration
	HTTPClient *http.Client // Optional custom HTTP client
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		SearXNGURL: "",
		Timeout:    30 * time.Second,
	}
}

// SearchArgs defines the arguments for the search tool.
type SearchArgs struct {
	Query      string `json:"query"`
	Language   string `json:"language"`
	SafeSearch int    `json:"safesearch"`
	TimeRange  string `json:"time_range"`
	Categories string `json:"categories"`
	Engines    string `json:"engines"`
	Pageno     *int   `json:"pageno"`
	Limit      *int   `json:"limit"`
}

// SearchResult represents a single search result.
type SearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	PublishedDate *string `json:"publishedDate,omitempty"`
}

// InfoboxURL represents a URL entry in an infobox.
type InfoboxURL struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// InfoboxAttribute represents a key-value attribute in an infobox.
type InfoboxAttribute struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Infobox represents a knowledge panel / infobox from SearXNG.
type Infobox struct {
	Infobox    string             `json:"infobox"`
	Content    string             `json:"content"`
	Attributes []InfoboxAttribute `json:"attributes,omitempty"`
	URLs       []InfoboxURL       `json:"urls,omitempty"`
}

// Answer represents a direct answer from SearXNG.
//
// SearXNG has legacy string answers and typed answers. Typed answers such as
// translations and weather use template-specific fields and may omit the
// legacy "answer" string entirely.
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

// TranslationItem represents one SearXNG typed translation answer item.
type TranslationItem struct {
	Text            string   `json:"text"`
	Transliteration string   `json:"transliteration,omitempty"`
	Examples        []string `json:"examples,omitempty"`
	Definitions     []string `json:"definitions,omitempty"`
	Synonyms        []string `json:"synonyms,omitempty"`
}

// WeatherItem represents one SearXNG typed weather answer item.
type WeatherItem struct {
	Location    WeatherLocation  `json:"location"`
	Datetime    *WeatherDateTime `json:"datetime,omitempty"`
	Summary     string           `json:"summary,omitempty"`
	Temperature WeatherMeasure   `json:"temperature"`
	FeelsLike   *WeatherMeasure  `json:"feels_like,omitempty"`
	Condition   string           `json:"condition"`
	Pressure    *WeatherMeasure  `json:"pressure,omitempty"`
	Humidity    *WeatherMeasure  `json:"humidity,omitempty"`
	WindFrom    *WeatherMeasure  `json:"wind_from,omitempty"`
	WindSpeed   *WeatherMeasure  `json:"wind_speed,omitempty"`
	CloudCover  *int             `json:"cloud_cover,omitempty"`
}

// WeatherLocation represents the location object embedded in SearXNG weather answers.
type WeatherLocation struct {
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Elevation   float64 `json:"elevation,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
}

// WeatherDateTime represents SearXNG's wrapped weather datetime value.
type WeatherDateTime struct {
	Datetime string `json:"datetime"`
}

// WeatherMeasure represents a numeric weather measurement with an optional unit.
type WeatherMeasure struct {
	Val  float64 `json:"val"`
	Unit string  `json:"unit,omitempty"`
}

// UnmarshalJSON preserves SearXNG typed answer fields and derives Answer when
// typed answers do not include the legacy answer string.
func (a *Answer) UnmarshalJSON(data []byte) error {
	type answerAlias Answer

	var parsed answerAlias

	err := json.Unmarshal(data, &parsed)
	if err != nil {
		return err
	}

	*a = Answer(parsed)
	a.EnsureFallback()

	return nil
}

// EnsureFallback derives a human-readable Answer value for known typed answers.
func (a *Answer) EnsureFallback() {
	if strings.TrimSpace(a.Answer) != "" {
		return
	}

	if fallback := a.translationFallback(); fallback != "" {
		a.Answer = fallback

		return
	}

	if fallback := a.weatherFallback(); fallback != "" {
		a.Answer = fallback
	}
}

func (a *Answer) translationFallback() string {
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

func (a *Answer) weatherFallback() string {
	if a.Current == nil {
		return ""
	}

	current := a.Current
	if summary := strings.TrimSpace(current.Summary); summary != "" {
		return summary
	}

	parts := make([]string, 0, 3)
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

// SearchResponse represents the full search response from SearXNG.
type SearchResponse struct {
	Query               string         `json:"query"`
	Answers             []Answer       `json:"answers,omitempty"`
	NumberOfResults     int            `json:"number_of_results"`
	Infoboxes           []Infobox      `json:"infoboxes,omitempty"`
	Results             []SearchResult `json:"results"`
	Suggestions         []string       `json:"suggestions"`
	UnresponsiveEngines [][]string     `json:"unresponsive_engines,omitempty"`
	Debug               bool           `json:"-"`
}

// searchResponseJSON is an intermediate type used by MarshalJSON to avoid
// anonymous struct definitions.
type searchResponseJSON struct {
	Query               string         `json:"query"`
	Answers             []Answer       `json:"answers,omitempty"`
	NumberOfResults     int            `json:"number_of_results"`
	Infoboxes           []Infobox      `json:"infoboxes,omitempty"`
	Results             []SearchResult `json:"results"`
	Suggestions         []string       `json:"suggestions"`
	UnresponsiveEngines *[][]string    `json:"unresponsive_engines,omitempty"`
}

// MarshalJSON uses a value receiver (not pointer) to avoid concurrent
// modification of the SearchResponse during serialization.
func (r SearchResponse) MarshalJSON() ([]byte, error) {
	if r.Results == nil {
		r.Results = []SearchResult{}
	}

	if r.Suggestions == nil {
		r.Suggestions = []string{}
	}

	base := searchResponseJSON{
		Query:           r.Query,
		Answers:         r.Answers,
		NumberOfResults: r.NumberOfResults,
		Infoboxes:       r.Infoboxes,
		Results:         r.Results,
		Suggestions:     r.Suggestions,
	}
	if r.Debug {
		if r.UnresponsiveEngines == nil {
			r.UnresponsiveEngines = [][]string{}
		}

		base.UnresponsiveEngines = &r.UnresponsiveEngines
	}

	return json.Marshal(base)
}
