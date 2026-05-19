package searxng

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSearchResponse_TypedAnswersGetFallbackText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		fixture    string
		wantAnswer string
		wantTmpl   string
	}{
		{
			name:       "translation",
			fixture:    "typed_translation_answer.json",
			wantAnswer: "Translation: bonjour",
			wantTmpl:   "answer/translations.html",
		},
		{
			name:       "weather",
			fixture:    "typed_weather_answer.json",
			wantAnswer: "Weather: Berlin, 11.2 °C, partly cloudy",
			wantTmpl:   "answer/weather.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("..", "..", "testdata", tt.fixture))
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			searcher := &SearXNGSearcher{}
			resp := &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(string(body))),
			}

			result, err := searcher.parseSearchResponse(resp, &SearchArgs{})
			if err != nil {
				t.Fatalf("parseSearchResponse() error = %v", err)
			}

			if len(result.Answers) != 1 {
				t.Fatalf("len(Answers) = %d, want 1", len(result.Answers))
			}

			if result.Answers[0].Answer != tt.wantAnswer {
				t.Fatalf("Answer = %q, want %q", result.Answers[0].Answer, tt.wantAnswer)
			}

			if result.Answers[0].Template != tt.wantTmpl {
				t.Fatalf("Template = %q, want %q", result.Answers[0].Template, tt.wantTmpl)
			}
		})
	}
}
