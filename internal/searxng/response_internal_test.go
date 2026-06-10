package searxng

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// --- readBodyWithLimit tests ---

//nolint:gocognit // table-driven test covering read limits and edge cases
func TestReadBodyWithLimit(t *testing.T) {
	t.Parallel()

	t.Run("body shorter than limit", func(t *testing.T) {
		t.Parallel()

		body := io.NopCloser(strings.NewReader("hello"))

		data, truncated, err := readBodyWithLimit(body, 100)
		if err != nil {
			t.Fatalf("readBodyWithLimit() error = %v", err)
		}

		if truncated {
			t.Fatal("readBodyWithLimit() truncated = true, want false")
		}

		if string(data) != "hello" {
			t.Fatalf("data = %q, want %q", data, "hello")
		}
	})

	t.Run("body exactly at limit", func(t *testing.T) {
		t.Parallel()

		body := io.NopCloser(strings.NewReader("hello"))

		data, truncated, err := readBodyWithLimit(body, 5)
		if err != nil {
			t.Fatalf("readBodyWithLimit() error = %v", err)
		}

		if truncated {
			t.Fatal("readBodyWithLimit() truncated = true, want false")
		}

		if string(data) != "hello" {
			t.Fatalf("data = %q, want %q", data, "hello")
		}
	})

	t.Run("body exceeds limit", func(t *testing.T) {
		t.Parallel()

		body := io.NopCloser(strings.NewReader("hello world"))

		data, truncated, err := readBodyWithLimit(body, 5)
		if err != nil {
			t.Fatalf("readBodyWithLimit() error = %v", err)
		}

		if !truncated {
			t.Fatal("readBodyWithLimit() truncated = false, want true")
		}

		if string(data) != "hello" {
			t.Fatalf("data = %q, want %q", data, "hello")
		}
	})

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()

		body := io.NopCloser(strings.NewReader(""))

		data, truncated, err := readBodyWithLimit(body, 100)
		if err != nil {
			t.Fatalf("readBodyWithLimit() error = %v", err)
		}

		if truncated {
			t.Fatal("readBodyWithLimit() truncated = true, want false")
		}

		if len(data) != 0 {
			t.Fatalf("data length = %d, want 0", len(data))
		}
	})
}

// --- readLimitedBody tests ---

func TestReadLimitedBody(t *testing.T) {
	t.Parallel()

	t.Run("successful read", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"key": "value"}`)),
		}

		data, err := readLimitedBody(resp)
		if err != nil {
			t.Fatalf("readLimitedBody() error = %v", err)
		}

		if string(data) != `{"key": "value"}` {
			t.Fatalf("data = %q, want %q", data, `{"key": "value"}`)
		}
	})

	t.Run("body exceeds MaxResponseBodySize", func(t *testing.T) {
		t.Parallel()

		largeBody := bytes.Repeat([]byte("x"), int(MaxResponseBodySize)+100)
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(bytes.NewReader(largeBody)),
		}

		data, err := readLimitedBody(resp)
		if err == nil {
			t.Fatal("readLimitedBody() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "response body exceeded maximum size limit") {
			t.Fatalf("error = %q, want body exceeded maximum size limit", err.Error())
		}

		if data != nil {
			t.Fatalf("data = %v, want nil on error", data)
		}
	})
}

// --- isHTMLResponse tests ---

func TestIsHTMLResponse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		body        string
		want        bool
	}{
		{name: "content-type text/html", contentType: "text/html", body: "<html></html>", want: true},
		{name: "content-type text/html; charset=utf-8", contentType: "text/html; charset=utf-8", body: "hello", want: true},
		{name: "content-type TEXT/HTML (uppercase)", contentType: "TEXT/HTML", body: "hello", want: true},
		{name: "content-type Text/Html (mixed case)", contentType: "Text/Html", body: "hello", want: true},
		{
			name:        "content-type text/html with extra params",
			contentType: "text/html; charset=utf-8; boundary=xyz",
			body:        "hello",
			want:        true,
		},
		{name: "body starts with <!DOCTYPE", contentType: "application/json", body: "<!DOCTYPE html>", want: true},
		{
			name:        "body starts with <!doctype (lowercase)",
			contentType: "application/json",
			body:        "<!doctype html>",
			want:        true,
		},
		{name: "body starts with <html", contentType: "application/json", body: `<html lang="en">`, want: true},
		{name: "body starts with <HTML (uppercase)", contentType: "application/json", body: "<HTML>", want: true},
		{
			name:        "body starts with <Html (mixed case)",
			contentType: "application/json",
			body:        `<Html lang="en">`,
			want:        true,
		},
		{name: "json response", contentType: "application/json", body: `{"key": "value"}`, want: false},
		{name: "empty body", contentType: "application/json", body: "", want: false},
		{name: "trimmed body with spaces then DOCTYPE", contentType: "text/plain", body: "  <!DOCTYPE html>", want: true},
		{
			name:        "trimmed body with spaces then lowercase doctype",
			contentType: "text/plain",
			body:        "  <!doctype html>",
			want:        true,
		},
		{name: "near-match text/htmlish with json body", contentType: "text/htmlish", body: `{"key": "value"}`, want: false},
		{name: "malformed content-type with html body", contentType: "not a mime type", body: "<!DOCTYPE html>", want: true},
		{name: "empty content-type with json body", contentType: "", body: `{"key": "value"}`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isHTMLResponse(tt.contentType, []byte(tt.body)); got != tt.want {
				t.Fatalf("isHTMLResponse(%q, %q) = %v, want %v", tt.contentType, tt.body, got, tt.want)
			}
		})
	}
}

// --- isJSONContentType tests ---

func TestIsJSONContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "application/json", contentType: "application/json", want: true},
		{name: "application/json; charset=utf-8", contentType: "application/json; charset=utf-8", want: true},
		{name: "application/json with extra params", contentType: "application/json; charset=utf-8; foo=bar", want: true},
		{name: "APPLICATION/JSON (uppercase)", contentType: "APPLICATION/JSON", want: true},
		{name: "Application/Json (mixed case)", contentType: "Application/Json", want: true},
		{name: "text/json", contentType: "text/json", want: true},
		{name: "text/json; charset=utf-8", contentType: "text/json; charset=utf-8", want: true},
		{name: "TEXT/JSON (uppercase)", contentType: "TEXT/JSON", want: true},
		{name: "Text/Json (mixed case)", contentType: "Text/Json", want: true},
		{name: "application/jsonish (near-match rejected)", contentType: "application/jsonish", want: false},
		{name: "application/json+ld (structured extension rejected)", contentType: "application/json+ld", want: false},
		{name: "text/jsonish (near-match rejected)", contentType: "text/jsonish", want: false},
		{name: "text/html is not json", contentType: "text/html", want: false},
		{name: "empty content-type", contentType: "", want: false},
		{name: "malformed content-type", contentType: "not a mime type", want: false},
		{name: "missing subtype", contentType: "application/", want: false},
		{name: "missing type", contentType: "/json", want: false},
		{name: "whitespace only", contentType: "   ", want: false},
		{name: "application/json with surrounding whitespace", contentType: "  application/json  ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isJSONContentType(tt.contentType); got != tt.want {
				t.Fatalf("isJSONContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

// --- isHTMLContentType tests ---

func TestIsHTMLContentType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		contentType string
		want        bool
	}{
		{name: "text/html", contentType: "text/html", want: true},
		{name: "text/html; charset=utf-8", contentType: "text/html; charset=utf-8", want: true},
		{name: "text/html with extra params", contentType: "text/html; charset=utf-8; boundary=xyz", want: true},
		{name: "TEXT/HTML (uppercase)", contentType: "TEXT/HTML", want: true},
		{name: "Text/Html (mixed case)", contentType: "Text/Html", want: true},
		{name: "text/htmlish (near-match rejected)", contentType: "text/htmlish", want: false},
		{name: "application/xhtml+xml is not text/html", contentType: "application/xhtml+xml", want: false},
		{name: "application/json is not html", contentType: "application/json", want: false},
		{name: "empty content-type", contentType: "", want: false},
		{name: "malformed content-type", contentType: "not a mime type", want: false},
		{name: "missing subtype", contentType: "text/", want: false},
		{name: "whitespace only", contentType: "   ", want: false},
		{name: "text/html with surrounding whitespace", contentType: "  text/html  ", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := isHTMLContentType(tt.contentType); got != tt.want {
				t.Fatalf("isHTMLContentType(%q) = %v, want %v", tt.contentType, got, tt.want)
			}
		})
	}
}

// --- decodeSearchResponse tests ---

//nolint:gocognit,gocyclo,cyclop,maintidx // table-driven test covering many response decode scenarios
func TestDecodeSearchResponse(t *testing.T) {
	t.Parallel()

	t.Run("valid JSON response", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test", "results": [` +
			`{"title": "Result 1", "url": "https://example.com", ` +
			`"content": "Content", "engine": "google"}], ` +
			`"suggestions": []}`
		resp := &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}

		result, err := decodeSearchResponse(resp, "application/json", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}

		if len(result.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(result.Results))
		}

		if result.Results[0].Title != "Result 1" {
			t.Fatalf("Results[0].Title = %q, want %q", result.Results[0].Title, "Result 1")
		}
	})

	t.Run("text/json content type", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "text/json", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("application/json with charset parameter", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "application/json; charset=utf-8", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("text/json with charset parameter", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "text/json; charset=utf-8", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("APPLICATION/JSON (uppercase)", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "APPLICATION/JSON", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("Application/Json (mixed case)", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "Application/Json", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("TEXT/JSON (uppercase)", func(t *testing.T) {
		t.Parallel()

		body := `{"query": "test"}`

		result, err := decodeSearchResponse(&http.Response{}, "TEXT/JSON", []byte(body))
		if err != nil {
			t.Fatalf("decodeSearchResponse() error = %v", err)
		}

		if result.Query != "test" {
			t.Fatalf("Query = %q, want %q", result.Query, "test")
		}
	})

	t.Run("application/jsonish is rejected as near-match", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusOK}

		_, err := decodeSearchResponse(resp, "application/jsonish", []byte(`{"query": "test"}`))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error for application/jsonish")
		}

		if !strings.Contains(err.Error(), "unexpected content type: expected application/json") {
			t.Fatalf("error = %q, want unexpected content type", err.Error())
		}
	})

	t.Run("text/jsonish is rejected as near-match", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusOK}

		_, err := decodeSearchResponse(resp, "text/jsonish", []byte(`{"query": "test"}`))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error for text/jsonish")
		}

		if !strings.Contains(err.Error(), "unexpected content type: expected application/json") {
			t.Fatalf("error = %q, want unexpected content type", err.Error())
		}
	})

	t.Run("malformed content-type falls back to unexpected error", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusOK}

		_, err := decodeSearchResponse(resp, "not a mime type", []byte(`{"query": "test"}`))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error for malformed content-type")
		}

		if !strings.Contains(err.Error(), "unexpected content type: expected application/json") {
			t.Fatalf("error = %q, want unexpected content type", err.Error())
		}
	})

	t.Run("empty content-type is rejected", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusOK}

		_, err := decodeSearchResponse(resp, "", []byte(`{"query": "test"}`))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error for empty content-type")
		}

		if !strings.Contains(err.Error(), "unexpected content type: expected application/json") {
			t.Fatalf("error = %q, want unexpected content type", err.Error())
		}
	})

	t.Run("unexpected content type", func(t *testing.T) {
		t.Parallel()

		resp := &http.Response{StatusCode: http.StatusOK}

		_, err := decodeSearchResponse(resp, "text/plain", []byte("not json"))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error")
		}

		if !strings.Contains(err.Error(), "unexpected content type: expected application/json") {
			t.Fatalf("error = %q, want unexpected content type", err.Error())
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		_, err := decodeSearchResponse(&http.Response{StatusCode: http.StatusOK}, "application/json", []byte("{invalid}"))
		if err == nil {
			t.Fatal("decodeSearchResponse() error = nil, want error")
		}

		var searxErr *SearXNGError
		if !errors.As(err, &searxErr) {
			t.Fatalf("error type = %T, want *SearXNGError", err)
		}
	})
}

// --- translationAnswerFallback tests ---

func TestTranslationAnswerFallback(t *testing.T) {
	t.Parallel()

	t.Run("no translations returns empty", func(t *testing.T) {
		t.Parallel()

		a := &Answer{}

		if got := translationAnswerFallback(a); got != "" {
			t.Fatalf("translationAnswerFallback() = %q, want empty string", got)
		}
	})

	t.Run("with translations returns formatted string", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Translations: []TranslationItem{
				{Text: "bonjour"},
				{Text: "salut"},
			},
		}

		got := translationAnswerFallback(a)
		if got != "Translation: bonjour; salut" {
			t.Fatalf("translationAnswerFallback() = %q, want %q", got, "Translation: bonjour; salut")
		}
	})

	t.Run("empty text entries are skipped", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Translations: []TranslationItem{
				{Text: ""},
				{Text: "hello"},
			},
		}

		got := translationAnswerFallback(a)
		if got != "Translation: hello" {
			t.Fatalf("translationAnswerFallback() = %q, want %q", got, "Translation: hello")
		}
	})

	t.Run("all empty texts return empty", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Translations: []TranslationItem{
				{Text: "   "},
			},
		}

		if got := translationAnswerFallback(a); got != "" {
			t.Fatalf("translationAnswerFallback() = %q, want empty string", got)
		}
	})
}

// --- weatherAnswerFallback tests ---

func TestWeatherAnswerFallback(t *testing.T) {
	t.Parallel()

	t.Run("nil current returns empty", func(t *testing.T) {
		t.Parallel()

		a := &Answer{}

		if got := weatherAnswerFallback(a); got != "" {
			t.Fatalf("weatherAnswerFallback() = %q, want empty string", got)
		}
	})

	t.Run("summary is used when present", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Current: &WeatherItem{
				Summary: "Partly cloudy throughout the day.",
			},
		}

		got := weatherAnswerFallback(a)
		if got != "Partly cloudy throughout the day." {
			t.Fatalf("weatherAnswerFallback() = %q, want summary", got)
		}
	})

	t.Run("no summary builds from components", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Current: &WeatherItem{
				Location:    WeatherLocation{Name: "Berlin"},
				Temperature: WeatherMeasure{Val: 11.2, Unit: "°C"},
				Condition:   "partly cloudy",
			},
		}

		got := weatherAnswerFallback(a)
		if got != "Weather: Berlin, 11.2 °C, partly cloudy" {
			t.Fatalf("weatherAnswerFallback() = %q, want %q", got, "Weather: Berlin, 11.2 °C, partly cloudy")
		}
	})

	t.Run("empty location still returns components", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Current: &WeatherItem{
				Temperature: WeatherMeasure{Val: 25.0, Unit: "°C"},
				Condition:   "sunny",
			},
		}

		got := weatherAnswerFallback(a)
		if got != "Weather: 25 °C, sunny" {
			t.Fatalf("weatherAnswerFallback() = %q, want %q", got, "Weather: 25 °C, sunny")
		}
	})

	t.Run("empty components return empty", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Current: &WeatherItem{
				Location:    WeatherLocation{Name: ""},
				Temperature: WeatherMeasure{},
				Condition:   "",
			},
		}

		if got := weatherAnswerFallback(a); got != "" {
			t.Fatalf("weatherAnswerFallback() = %q, want empty string", got)
		}
	})
}

// --- EnsureAnswerFallback tests ---

func TestEnsureAnswerFallback(t *testing.T) {
	t.Parallel()

	t.Run("non-empty answer not overwritten", func(t *testing.T) {
		t.Parallel()

		a := &Answer{Answer: "existing answer"}
		EnsureAnswerFallback(a)

		if a.Answer != "existing answer" {
			t.Fatalf("Answer = %q, want %q", a.Answer, "existing answer")
		}
	})

	t.Run("empty answer with translation gets fallback", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Translations: []TranslationItem{{Text: "bonjour"}},
		}
		EnsureAnswerFallback(a)

		if a.Answer != "Translation: bonjour" {
			t.Fatalf("Answer = %q, want %q", a.Answer, "Translation: bonjour")
		}
	})

	t.Run("empty answer with weather gets fallback", func(t *testing.T) {
		t.Parallel()

		a := &Answer{
			Current: &WeatherItem{
				Location:    WeatherLocation{Name: "Paris"},
				Temperature: WeatherMeasure{Val: 15.0, Unit: "°C"},
				Condition:   "clear",
			},
		}
		EnsureAnswerFallback(a)

		want := "Weather: Paris, 15 °C, clear"
		if a.Answer != want {
			t.Fatalf("Answer = %q, want %q", a.Answer, want)
		}
	})

	t.Run("no fallback available stays empty", func(t *testing.T) {
		t.Parallel()

		a := &Answer{}
		EnsureAnswerFallback(a)

		if a.Answer != "" {
			t.Fatalf("Answer = %q, want empty", a.Answer)
		}
	})
}

// --- normalizeResponse tests ---

//nolint:gocognit,gocyclo,cyclop,maintidx // table-driven test covering many normalize response scenarios
func TestNormalizeResponse(t *testing.T) {
	t.Parallel()

	t.Run("sets warning and normalizes nil slices", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{}

		s.normalizeResponse(result, &SearchArgs{})

		if result.Warning != ExternalContentWarning {
			t.Fatalf("Warning = %q, want %q", result.Warning, ExternalContentWarning)
		}

		if result.Results == nil {
			t.Fatal("Results is nil, want empty slice")
		}

		if result.Suggestions == nil {
			t.Fatal("Suggestions is nil, want empty slice")
		}

		if result.NumberOfResults != 0 {
			t.Fatalf("NumberOfResults = %d, want 0", result.NumberOfResults)
		}
	})

	t.Run("sets NumberOfResults from results length", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{
			Results: []SearchResult{
				{Title: "A"},
				{Title: "B"},
			},
		}

		s.normalizeResponse(result, &SearchArgs{})

		if result.NumberOfResults != 2 {
			t.Fatalf("NumberOfResults = %d, want 2", result.NumberOfResults)
		}
	})

	t.Run("keeps existing NumberOfResults", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{
			NumberOfResults: 42,
			Results:         []SearchResult{{Title: "A"}},
		}

		s.normalizeResponse(result, &SearchArgs{})

		if result.NumberOfResults != 42 {
			t.Fatalf("NumberOfResults = %d, want 42", result.NumberOfResults)
		}
	})

	t.Run("applies limit", func(t *testing.T) {
		t.Parallel()

		limit := 1
		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{
			Results: []SearchResult{
				{Title: "A"},
				{Title: "B"},
				{Title: "C"},
			},
		}

		s.normalizeResponse(result, &SearchArgs{Limit: &limit})

		if len(result.Results) != 1 {
			t.Fatalf("len(Results) = %d, want 1", len(result.Results))
		}

		if result.Results[0].Title != "A" {
			t.Fatalf("Results[0].Title = %q, want %q", result.Results[0].Title, "A")
		}
	})

	t.Run("limit larger than results does not truncate", func(t *testing.T) {
		t.Parallel()

		limit := 10
		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{
			Results: []SearchResult{
				{Title: "A"},
				{Title: "B"},
			},
		}

		s.normalizeResponse(result, &SearchArgs{Limit: &limit})

		if len(result.Results) != 2 {
			t.Fatalf("len(Results) = %d, want 2", len(result.Results))
		}
	})

	t.Run("debug mode includes unresponsive engines", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: true}
		result := &SearchResponse{}

		s.normalizeResponse(result, &SearchArgs{})

		if !result.Debug {
			t.Fatal("Debug = false, want true")
		}

		if result.UnresponsiveEngines == nil {
			t.Fatal("UnresponsiveEngines is nil, want empty slice")
		}
	})

	t.Run("non-debug mode excludes unresponsive engines", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}
		result := &SearchResponse{}

		s.normalizeResponse(result, &SearchArgs{})

		if result.UnresponsiveEngines != nil {
			t.Fatal("UnresponsiveEngines is not nil, want nil in non-debug mode")
		}
	})

	t.Run("truncates answers to MaxAnswers", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		answers := make([]Answer, MaxAnswers+25)
		for i := range answers {
			answers[i] = Answer{Answer: "answer " + strings.Repeat("x", i%16), Engine: "e"}
		}

		result := &SearchResponse{Answers: answers}

		s.normalizeResponse(result, &SearchArgs{})

		if len(result.Answers) != MaxAnswers {
			t.Fatalf("len(Answers) = %d, want %d", len(result.Answers), MaxAnswers)
		}
	})

	t.Run("truncates infoboxes to MaxInfoboxes", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		infoboxes := make([]Infobox, MaxInfoboxes+25)
		for i := range infoboxes {
			infoboxes[i] = Infobox{Infobox: "topic", Content: "content"}
		}

		result := &SearchResponse{Infoboxes: infoboxes}

		s.normalizeResponse(result, &SearchArgs{})

		if len(result.Infoboxes) != MaxInfoboxes {
			t.Fatalf("len(Infoboxes) = %d, want %d", len(result.Infoboxes), MaxInfoboxes)
		}
	})

	t.Run("does not truncate when at or below cap", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		answers := make([]Answer, MaxAnswers)
		for i := range answers {
			answers[i] = Answer{Answer: "a"}
		}

		infoboxes := make([]Infobox, MaxInfoboxes)
		for i := range infoboxes {
			infoboxes[i] = Infobox{Infobox: "t", Content: "c"}
		}

		result := &SearchResponse{Answers: answers, Infoboxes: infoboxes}

		s.normalizeResponse(result, &SearchArgs{})

		if len(result.Answers) != MaxAnswers {
			t.Fatalf("len(Answers) = %d, want %d", len(result.Answers), MaxAnswers)
		}

		if len(result.Infoboxes) != MaxInfoboxes {
			t.Fatalf("len(Infoboxes) = %d, want %d", len(result.Infoboxes), MaxInfoboxes)
		}
	})

	t.Run("truncation preserves dedup behavior", func(t *testing.T) {
		t.Parallel()

		s := &SearXNGSearcher{debug: false}

		// Mix duplicate and non-duplicate answers past the cap. The kept
		// prefix is all duplicates, so the result must be empty.
		wiki := "Apple Inc. is an American multinational technology company headquartered in Cupertino, California."

		answers := make([]Answer, MaxAnswers+5)
		for i := range answers {
			answers[i] = Answer{Answer: wiki, Engine: "duckduckgo"}
		}

		infoboxes := []Infobox{{Infobox: "Apple Inc.", Content: wiki}}

		result := &SearchResponse{Answers: answers, Infoboxes: infoboxes}

		s.normalizeResponse(result, &SearchArgs{})

		if len(result.Answers) != 0 {
			t.Fatalf("len(Answers) = %d, want 0 (all kept answers were duplicates of the infobox)", len(result.Answers))
		}
	})

	t.Run("pathological answers/infoboxes count terminates quickly", func(t *testing.T) {
		t.Parallel()

		// Build a response with many more answers and infoboxes than the
		// cap. The model artifact in CAND-33fe0b85-RUNTIME-003 shows
		// ~47k entries each fitting under MaxResponseBodySize. The cap
		// must keep the dedup work bounded; we assert a generous time
		// budget to catch unbounded regressions without flaking under
		// load.
		const oversized = 10 * MaxAnswers

		const timeBudget = 2 * time.Second

		answers := make([]Answer, oversized)
		for i := range answers {
			answers[i] = Answer{Answer: "answer text", Engine: "e"}
		}

		infoboxes := make([]Infobox, oversized)
		for i := range infoboxes {
			infoboxes[i] = Infobox{Infobox: "t", Content: "content"}
		}

		result := &SearchResponse{Answers: answers, Infoboxes: infoboxes}

		s := &SearXNGSearcher{debug: false}

		start := time.Now()

		s.normalizeResponse(result, &SearchArgs{})

		elapsed := time.Since(start)

		if elapsed > timeBudget {
			t.Fatalf("normalizeResponse took %v, want < %v (pathological input must stay bounded)", elapsed, timeBudget)
		}

		if len(result.Answers) > MaxAnswers {
			t.Fatalf("len(Answers) = %d, want <= %d", len(result.Answers), MaxAnswers)
		}

		if len(result.Infoboxes) > MaxInfoboxes {
			t.Fatalf("len(Infoboxes) = %d, want <= %d", len(result.Infoboxes), MaxInfoboxes)
		}
	})
}
