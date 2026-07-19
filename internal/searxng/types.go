package searxng

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"searxng-mcp-go/internal/searxng/answer"
)

// Answer is a type alias for the answer subpackage type kept for backward
// compatibility. The canonical definition lives in the answer subpackage.
type Answer = answer.Answer

// TranslationItem is a type alias kept for backward compatibility.
type TranslationItem = answer.TranslationItem

// WeatherItem is a type alias kept for backward compatibility.
type WeatherItem = answer.WeatherItem

// WeatherLocation is a type alias kept for backward compatibility.
type WeatherLocation = answer.WeatherLocation

// WeatherMeasure is a type alias kept for backward compatibility.
type WeatherMeasure = answer.WeatherMeasure

var (
	errURLRequired           = errors.New("SearXNG URL cannot be empty")
	errTimeoutNegative       = errors.New("timeout cannot be negative")
	errTimeoutZero           = errors.New("timeout must be greater than zero; omit it to use the 8s default")
	errMaxRetriesNegative    = errors.New("max retries cannot be negative")
	errMaxRetriesTooLarge    = errors.New("max retries cannot exceed 20")
	maxRetryCap              = 20
	errRetryDelayNegative    = errors.New("retry delay cannot be negative")
	errRetryDelayTooSmall    = errors.New("retry delay must be at least 1s when set explicitly")
	errMaxRetryDelayNegative = errors.New("max retry delay cannot be negative")
	errMaxRetryDelayTooSmall = errors.New("max retry delay must be at least 1s when set explicitly")
)

// Config controls SearXNG client behavior.
type Config struct {
	SearXNGURL string
	// Timeout is the per-request HTTP timeout for each individual SearXNG
	// request attempt. It does not bound the overall Search operation, which
	// is governed by the caller-provided context. When HTTPClient is set,
	// Timeout is ignored and the custom client controls HTTP timing.
	//
	// A zero value in a struct literal means "unset" and is normalized to
	// DefaultTimeout (8s) by Normalize. An explicit zero through SetTimeout
	// (the CLI/env path) is rejected — use a positive duration or omit the
	// setting to keep the default.
	Timeout    time.Duration
	HTTPClient *http.Client
	MaxRetries int
	// RetryDelay is the base delay for exponential backoff. A zero value
	// normalizes to DefaultRetryDelay (1s). An explicit positive value
	// below 1s is rejected at configuration validation time. The final
	// jittered wait is always at least 1s regardless of the configured delay.
	RetryDelay time.Duration
	// MaxRetryDelay is the upper bound for retry backoff delays. A zero
	// value normalizes to DefaultMaxRetryDelay (30s). An explicit positive
	// value below 1s is rejected at configuration validation time. When
	// set below RetryDelay, Normalize clamps it to RetryDelay.
	MaxRetryDelay    time.Duration
	AllowGETFallback bool
	Logger           *slog.Logger // Optional logger; nil = slog.Default()
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

// SetTimeout validates and sets the Timeout field. Returns an error for
// negative values or zero (which would be ambiguous: "unset" vs "explicit"
// are distinguished by whether SetTimeout was called). Both env-var parsing
// and flag overrides go through this setter so zero is rejected for explicit
// CLI/env paths. Programmatic callers using a struct literal zero can rely
// on Normalize to apply DefaultTimeout.
func (c *Config) SetTimeout(d time.Duration) error {
	if d < 0 {
		return errTimeoutNegative
	}

	if d == 0 {
		return errTimeoutZero
	}

	c.Timeout = d

	return nil
}

// SetMaxRetries validates and sets the MaxRetries field. Returns an error
// for negative values or values exceeding maxRetryCap. Both env-var parsing
// and flag overrides go through this setter.
func (c *Config) SetMaxRetries(n int) error {
	if n < 0 {
		return errMaxRetriesNegative
	}

	if n > maxRetryCap {
		return errMaxRetriesTooLarge
	}

	c.MaxRetries = n

	return nil
}

// Validate checks the configuration for valid values.
// No side effects: no HTTP calls, no logging.
// Zero values for Timeout, RetryDelay, and MaxRetryDelay are accepted here
// (they are normalized by Normalize). Only negative and explicitly invalid
// positive values are rejected.
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

	if c.RetryDelay > 0 && c.RetryDelay < time.Second {
		return errRetryDelayTooSmall
	}

	if c.MaxRetryDelay > 0 && c.MaxRetryDelay < time.Second {
		return errMaxRetryDelayTooSmall
	}

	return nil
}

// Normalize returns a copy of the Config with safe defaults applied.
// Zero timeout values are set to DefaultTimeout. Zero retry delay values are
// replaced with their respective defaults. MaxRetryDelay is clamped to be at
// least RetryDelay. Negative values must be rejected by Validate before
// Normalize is called.
func (c *Config) Normalize() *Config {
	cfg := *c // Copy

	if cfg.Timeout == 0 {
		cfg.Timeout = DefaultTimeout
	}

	if cfg.RetryDelay == 0 {
		cfg.RetryDelay = DefaultRetryDelay
	}

	if cfg.MaxRetryDelay == 0 {
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

// NewSearchArgs creates a SearchArgs with the given query and default Limit.
// Pageno is left nil (omitted = backend default / page 1). Both CLI and MCP
// callers should use this constructor so the defaulting policy lives in one
// place instead of being duplicated across layers.
func NewSearchArgs(query string) *SearchArgs {
	limit := DefaultResultLimit

	return &SearchArgs{
		Query: query,
		Limit: &limit,
	}
}

// ApplyDefaults fills nil Limit with DefaultResultLimit. It is a no-op when
// Limit is already set. Callers that construct SearchArgs manually (e.g. from
// MCP deserialization) should call this before ValidateSearchArgs.
func (args *SearchArgs) ApplyDefaults() {
	if args.Limit == nil {
		limit := DefaultResultLimit
		args.Limit = &limit
	}
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
	base := searchResponseJSON{
		Query:           r.Query,
		Warning:         r.Warning,
		Answers:         r.Answers,
		NumberOfResults: r.NumberOfResults,
		Infoboxes:       r.Infoboxes,
		Results:         r.Results,
		Suggestions:     r.Suggestions,
	}
	if r.Results == nil {
		base.Results = []SearchResult{}
	}

	if r.Suggestions == nil {
		base.Suggestions = []string{}
	}

	if r.Debug {
		if r.UnresponsiveEngines == nil {
			empty := [][]string{}
			base.UnresponsiveEngines = &empty
		} else {
			base.UnresponsiveEngines = &r.UnresponsiveEngines
		}
	}

	return json.Marshal(base)
}
