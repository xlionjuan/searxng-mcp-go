package searxng

import (
	"encoding/json"
	"net/http"
	"time"
)

// DefaultSearXNGURL is the default SearXNG instance URL.
// WARNING: This is a default value for convenience only. For production use,
// you should set your own instance via the SEARXNG_URL environment variable.
const DefaultSearXNGURL = "https://search-4.xlion.dev"

// Config holds the SearXNG configuration
type Config struct {
	SearXNGURL string
	Timeout    time.Duration
	HTTPClient *http.Client // Optional custom HTTP client
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		SearXNGURL: DefaultSearXNGURL,
		Timeout:    30 * time.Second,
	}
}

// SearchArgs defines the arguments for the search tool
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

// SearchResult represents a single search result
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

// Answer represents a direct answer from SearXNG (e.g., IP, hash, timezone, calculator).
type Answer struct {
	Answer   string `json:"answer"`
	Engine   string `json:"engine"`
	Template string `json:"template,omitempty"`
}

// SearchResponse represents the full search response from SearXNG
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
	UnresponsiveEngines [][]string     `json:"unresponsive_engines,omitempty"`
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
		base.UnresponsiveEngines = r.UnresponsiveEngines
	}
	return json.Marshal(base)
}
