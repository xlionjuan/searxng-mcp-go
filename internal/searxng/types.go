package searxng

import (
	"encoding/json"
	"errors"
	"html"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const errTimeoutNegativeMessage = "Timeout cannot be negative"

var (

	errURLRequired           = errors.New("SearXNGURL cannot be empty")
	errTimeoutNegative       = errors.New(errTimeoutNegativeMessage)
	errMaxRetriesNegative    = errors.New("MaxRetries cannot be negative")
	errMaxRetriesTooLarge    = errors.New("MaxRetries cannot exceed 20")
	maxRetryCap              = 20
	errRetryDelayNegative    = errors.New("RetryDelay cannot be negative")
	errMaxRetryDelayNegative = errors.New("MaxRetryDelay cannot be negative")
)

// Config controls SearXNG client behavior.
type Config struct {
	SearXNGURL       string
	Timeout          time.Duration
	HTTPClient       *http.Client // Optional custom HTTP client. When set, Timeout is ignored.
	MaxRetries       int
	RetryDelay       time.Duration
	MaxRetryDelay    time.Duration
	AllowGETFallback bool
}

// DefaultConfig returns a Config populated with default timeout and retry settings.
func DefaultConfig() *Config {
	return &Config{
		SearXNGURL:       "",
		Timeout:          DefaultTimeout,
		MaxRetries:       DefaultMaxRetries,
		RetryDelay:       DefaultRetryDelay,
		MaxRetryDelay:    DefaultMaxRetryDelay,
		AllowGETFallback: false,
	}
}

// Validate checks the configuration for valid values.
// No side effects: no HTTP calls, no logging.
func (c *Config) Validate() error {
	if c.SearXNGURL == "" {
		return errURLRequired
	}

	if c.Timeout < 0 {
		return errTimeoutNegative
	}

	if c.MaxRetries < 0 {
		return errMaxRetriesNegative
	}

	if c.MaxRetries > maxRetryCap {
		return errMaxRetriesTooLarge
	}

	if c.RetryDelay < 0 {
		return errRetryDelayNegative
	}

	if c.MaxRetryDelay < 0 {
		return errMaxRetryDelayNegative
	}

	return nil
}

// Normalize returns a copy of the Config with safe defaults applied.
// Zero or negative retry delays are replaced with defaults.
// MaxRetryDelay is clamped to be at least RetryDelay.
func (c *Config) Normalize() *Config {
	cfg := *c // Copy

	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}

	if cfg.MaxRetryDelay <= 0 {
		cfg.MaxRetryDelay = DefaultMaxRetryDelay
	}

	if cfg.MaxRetryDelay < cfg.RetryDelay {
		cfg.MaxRetryDelay = cfg.RetryDelay
	}

	return &cfg
}

// SearchArgs contains normalized search request parameters.
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

// UnescapeIfNeeded calls html.UnescapeString only when the string contains
// HTML entities, avoiding unnecessary allocations.
func UnescapeIfNeeded(s string) string {
	if !strings.ContainsAny(s, "&<>\"") {
		return s
	}

	return html.UnescapeString(s)
}

// SearchResult is a single web result returned by SearXNG.
type SearchResult struct {
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Engine        string  `json:"engine"`
	PublishedDate *string `json:"publishedDate,omitempty"`
}

// InfoboxURL is a link associated with a SearXNG infobox.
type InfoboxURL struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// InfoboxAttribute is a label-value pair associated with a SearXNG infobox.
type InfoboxAttribute struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Infobox is a structured summary returned by SearXNG.
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

// WeatherLocation describes the location for a weather answer.
type WeatherLocation struct {
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	Elevation   float64 `json:"elevation,omitempty"`
	CountryCode string  `json:"country_code,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
}

// WeatherDateTime contains the timestamp for a weather answer.
type WeatherDateTime struct {
	Datetime string `json:"datetime"`
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

// SearchResponse is the normalized response returned to CLI and MCP callers.
type SearchResponse struct {
	Query               string         `json:"query"`
	Warning             string         `json:"warning"`
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
	Warning             string         `json:"warning"`
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
		Warning:         r.Warning,
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
