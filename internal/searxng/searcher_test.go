package searxng

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const testAnswerBody = "answer"

func TestNewSearXNGSearcherErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "empty URL", baseURL: "", want: "baseurl cannot be empty"},
		{name: "invalid scheme", baseURL: "ftp://example.com", want: "url must use http or https scheme"},
		{name: "missing host", baseURL: "https:///search", want: "url must include a host"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			searcher, err := NewSearXNGSearcher(&Config{SearXNGURL: tt.baseURL, Timeout: time.Second}, false)
			if err == nil {
				if searcher != nil {
					_ = searcher.Close()
				}

				t.Fatal("NewSearXNGSearcher() error = nil, want error")
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("NewSearXNGSearcher() error = %q, want error containing %q", err.Error(), tt.want)
			}
		})
	}
}

func TestNewSearXNGSearcherSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
	}{
		{name: "valid https URL", baseURL: "https://search.example.com"},
		{name: "valid http URL with private host", baseURL: "http://127.0.0.1:8080"},
		{name: "URL with path", baseURL: "https://search.example.com/searxng"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			searcher, err := NewSearXNGSearcher(&Config{SearXNGURL: tt.baseURL, Timeout: time.Second}, false)
			if err != nil {
				t.Fatalf("NewSearXNGSearcher() error = %v, want nil", err)
			}

			if searcher == nil {
				t.Fatal("NewSearXNGSearcher() searcher = nil, want non-nil")
			}

			err = searcher.Close()
			if err != nil {
				t.Fatalf("Close() error = %v, want nil", err)
			}
		})
	}
}

func TestConfigAndDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig() = nil, want config")
	}

	if cfg.SearXNGURL != "" {
		t.Fatalf("SearXNGURL = %q, want empty string", cfg.SearXNGURL)
	}

	if cfg.Timeout != 8*time.Second {
		t.Fatalf("Timeout = %v, want 8s", cfg.Timeout)
	}

	if cfg.HTTPClient != nil {
		t.Fatalf("HTTPClient = %v, want nil", cfg.HTTPClient)
	}

	if cfg.MaxRetries != DefaultMaxRetries {
		t.Fatalf("MaxRetries = %d, want %d", cfg.MaxRetries, DefaultMaxRetries)
	}

	if cfg.RetryDelay != DefaultRetryDelay {
		t.Fatalf("RetryDelay = %v, want %v", cfg.RetryDelay, DefaultRetryDelay)
	}

	if cfg.MaxRetryDelay != DefaultMaxRetryDelay {
		t.Fatalf("MaxRetryDelay = %v, want %v", cfg.MaxRetryDelay, DefaultMaxRetryDelay)
	}

	client := &http.Client{Timeout: time.Second}
	cfg.SearXNGURL = "https://example.com/search"
	cfg.Timeout = 5 * time.Second
	cfg.MaxRetries = 3
	cfg.RetryDelay = 2 * time.Millisecond
	cfg.MaxRetryDelay = 10 * time.Millisecond

	cfg.HTTPClient = client
	if cfg.SearXNGURL != "https://example.com/search" ||
		cfg.Timeout != 5*time.Second ||
		cfg.HTTPClient != client ||
		cfg.MaxRetries != 3 ||
		cfg.RetryDelay != 2*time.Millisecond ||
		cfg.MaxRetryDelay != 10*time.Millisecond {
		t.Fatal("Config fields are not writable")
	}
}

func TestSearchResponseMarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("nil slices serialize as empty arrays", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		var got map[string]json.RawMessage

		err = json.Unmarshal(body, &got)
		if err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if string(got["results"]) != "[]" {
			t.Fatalf("results = %s, want []", got["results"])
		}

		if string(got["suggestions"]) != "[]" {
			t.Fatalf("suggestions = %s, want []", got["suggestions"])
		}
	})

	t.Run("Debug false omits unresponsive_engines", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{
			UnresponsiveEngines: [][]string{{"google", "timeout"}},
			Debug:               false,
		})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		if strings.Contains(string(body), "unresponsive_engines") {
			t.Fatalf("json = %s, want unresponsive_engines omitted", body)
		}
	})

	t.Run("Debug true includes empty unresponsive_engines", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{Debug: true})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		if !strings.Contains(string(body), `"unresponsive_engines":[]`) {
			t.Fatalf("json = %s, want empty unresponsive_engines", body)
		}
	})

	t.Run("Debug true includes unresponsive_engines values", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{
			UnresponsiveEngines: [][]string{{"google", "timeout"}},
			Debug:               true,
		})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		if !strings.Contains(string(body), `"unresponsive_engines":[["google","timeout"]]`) {
			t.Fatalf("json = %s, want unresponsive_engines values", body)
		}
	})
}

func TestDeduplicateAnswers(t *testing.T) {
	t.Parallel()

	t.Run("nil answers returns nil", func(t *testing.T) {
		t.Parallel()

		if got := deduplicateAnswers(nil, []Infobox{{Content: "content"}}); got != nil {
			t.Fatalf("deduplicateAnswers() = %#v, want nil", got)
		}
	})

	t.Run("empty answers returns empty", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{}

		got := deduplicateAnswers(answers, []Infobox{{Content: "content"}})
		if len(got) != 0 {
			t.Fatalf("deduplicateAnswers() length = %d, want 0", len(got))
		}
	})

	t.Run("nil infoboxes returns as-is", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: testAnswerBody}}

		got := deduplicateAnswers(answers, nil)
		if len(got) != 1 || got[0].Answer != testAnswerBody {
			t.Fatalf("deduplicateAnswers() = %#v, want original answers", got)
		}
	})

	t.Run("empty infoboxes returns as-is", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: testAnswerBody}}

		got := deduplicateAnswers(answers, []Infobox{})
		if len(got) != 1 || got[0].Answer != testAnswerBody {
			t.Fatalf("deduplicateAnswers() = %#v, want original answers", got)
		}
	})

	t.Run("infoboxes without content returns as-is", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: testAnswerBody}}

		got := deduplicateAnswers(answers, []Infobox{{Infobox: "empty"}})
		if len(got) != 1 || got[0].Answer != testAnswerBody {
			t.Fatalf("deduplicateAnswers() = %#v, want original answers", got)
		}
	})

	t.Run("answer prefix of infobox is filtered", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: "Albert Einstein was a theoretical physicist"}}

		infoboxes := []Infobox{{Content: "Albert Einstein was a theoretical physicist who developed relativity."}}
		if got := deduplicateAnswers(answers, infoboxes); len(got) != 0 {
			t.Fatalf("deduplicateAnswers() = %#v, want answer filtered", got)
		}
	})

	t.Run("answer that is not prefix is kept", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: "Marie Curie discovered polonium"}}
		infoboxes := []Infobox{{Content: "Albert Einstein was a theoretical physicist."}}

		got := deduplicateAnswers(answers, infoboxes)
		if len(got) != 1 || got[0].Answer != answers[0].Answer {
			t.Fatalf("deduplicateAnswers() = %#v, want original answer", got)
		}
	})

	t.Run("More at Wikipedia suffix is trimmed before checking", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: "Ada Lovelace was an English mathematician More at Wikipedia"}}

		infoboxes := []Infobox{{Content: "Ada Lovelace was an English mathematician and writer."}}
		if got := deduplicateAnswers(answers, infoboxes); len(got) != 0 {
			t.Fatalf("deduplicateAnswers() = %#v, want answer filtered", got)
		}
	})

	t.Run("lowercase fallback when exact case fails", func(t *testing.T) {
		t.Parallel()

		answers := []Answer{{Answer: "Grace Hopper was a computer scientist"}}

		infoboxes := []Infobox{{Content: "grace hopper was a computer scientist and naval officer."}}
		if got := deduplicateAnswers(answers, infoboxes); len(got) != 0 {
			t.Fatalf("deduplicateAnswers() = %#v, want answer filtered by lowercase fallback", got)
		}
	})
}

func TestHTTPStatusError(t *testing.T) {
	t.Parallel()

	err := HTTPStatusError(http.StatusTooManyRequests, "text/plain", []byte("slow down"))

	var searxErr *SearXNGError
	if !errors.As(err, &searxErr) {
		t.Fatalf("HTTPStatusError() = %T, want SearXNGError", err)
	}

	if searxErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("StatusCode = %d, want %d", searxErr.StatusCode, http.StatusTooManyRequests)
	}

	if !strings.Contains(err.Error(), "rate limited") {
		t.Fatalf("Error() = %q, want rate limited message", err.Error())
	}
}
