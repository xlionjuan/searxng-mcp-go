package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestNewSearXNGSearcherErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		want    string
	}{
		{name: "empty URL", baseURL: "", want: "SearXNGURL cannot be empty"},
		{name: "invalid scheme", baseURL: "ftp://example.com", want: "url must use http or https scheme"},
		{name: "missing host", baseURL: "https:///search", want: "url must include a host"},
		{name: "URL parse error", baseURL: "://invalid", want: "invalid URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			searcher, err := NewSearXNGSearcher(&Config{SearXNGURL: tt.baseURL, Timeout: time.Second}, false)
			if err == nil {
				if searcher != nil {
					_ = searcher.Close() //nolint:errcheck // test cleanup; error is non-actionable
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
		wantURL string
	}{
		{name: "valid https URL", baseURL: "https://search.example.com", wantURL: "https://search.example.com/search"},
		{name: "valid http URL with private host", baseURL: "http://127.0.0.1:8080", wantURL: "http://127.0.0.1:8080/search"},
		{
			name:    "URL with path",
			baseURL: "https://search.example.com/searxng",
			wantURL: "https://search.example.com/searxng/search",
		},
		{
			name:    "URL with /search path",
			baseURL: "https://search.example.com/search",
			wantURL: "https://search.example.com/search",
		},
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

			if searcher.searchEndpoint == nil {
				t.Fatal("searcher.searchEndpoint = nil, want non-nil precomputed endpoint")
			}

			if got := searcher.searchEndpoint.String(); got != tt.wantURL {
				t.Fatalf("searchEndpoint = %q, want %q", got, tt.wantURL)
			}

			err = searcher.Close()
			if err != nil {
				t.Fatalf("Close() error = %v, want nil", err)
			}
		})
	}
}

//nolint:gocognit,gocyclo // subtest-based config verification test
func TestConfigAndDefaultConfig(t *testing.T) {
	t.Parallel()

	t.Run("default fields are set correctly", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		if cfg == nil {
			t.Fatal("DefaultConfig() = nil, want config")

			return
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
	})

	t.Run("fields are writable", func(t *testing.T) {
		t.Parallel()

		cfg := DefaultConfig()
		client := &http.Client{Timeout: time.Second}
		cfg.SearXNGURL = "https://example.com/search"
		cfg.Timeout = 5 * time.Second
		cfg.MaxRetries = 3
		cfg.RetryDelay = 2 * time.Millisecond
		cfg.MaxRetryDelay = 10 * time.Millisecond
		cfg.HTTPClient = client

		if cfg.SearXNGURL != "https://example.com/search" {
			t.Fatal("Config fields are not writable")
		}

		if cfg.Timeout != 5*time.Second {
			t.Fatal("Config fields are not writable")
		}

		if cfg.HTTPClient != client {
			t.Fatal("Config fields are not writable")
		}

		if cfg.MaxRetries != 3 {
			t.Fatal("Config fields are not writable")
		}

		if cfg.RetryDelay != 2*time.Millisecond {
			t.Fatal("Config fields are not writable")
		}

		if cfg.MaxRetryDelay != 10*time.Millisecond {
			t.Fatal("Config fields are not writable")
		}
	})
}

//nolint:gocognit,gocyclo,cyclop // table-driven test covering JSON marshal cases for SearchResponse
func TestSearchResponseMarshalJSON(t *testing.T) {
	t.Parallel()

	t.Run("nil slices serialize as null", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{Warning: ExternalContentWarning})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		var got map[string]json.RawMessage

		err = json.Unmarshal(body, &got)
		if err != nil {
			t.Fatalf("json.Unmarshal() error = %v", err)
		}

		if string(got["results"]) != "null" {
			t.Fatalf("results = %s, want null", got["results"])
		}

		if string(got["suggestions"]) != "null" {
			t.Fatalf("suggestions = %s, want null", got["suggestions"])
		}

		if string(got["warning"]) != `"`+ExternalContentWarning+`"` {
			t.Fatalf("warning = %s, want %q", got["warning"], ExternalContentWarning)
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

	t.Run("Debug true includes null unresponsive_engines", func(t *testing.T) {
		t.Parallel()

		body, err := json.Marshal(SearchResponse{Debug: true})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}

		if !strings.Contains(string(body), `"unresponsive_engines":null`) {
			t.Fatalf("json = %s, want null unresponsive_engines", body)
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

	t.Run("Empty warning is always serialized", func(t *testing.T) {
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

		raw, ok := got["warning"]
		if !ok {
			t.Fatalf("json = %s, want warning field always present", body)
		}

		if string(raw) != `""` {
			t.Fatalf("warning = %s, want empty string", raw)
		}
	})
}
