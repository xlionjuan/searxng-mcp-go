package searxng_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"searxng-mcp-go/internal/searxng"
)

func requireValidationError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected validation error for %s, got nil", field)
	}
	var ve *searxng.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if ve.Field != field {
		t.Fatalf("ValidationError.Field = %q, want %q", ve.Field, field)
	}
}

func TestValidateSearchArgs(t *testing.T) {
	t.Parallel()

	t.Run("nil args", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(nil), "args")
	})

	t.Run("empty query", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{}), "query")
	})

	t.Run("whitespace-only query", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: " \t "}), "query")
	})

	t.Run("query with control characters", func(t *testing.T) {
		t.Parallel()
		for _, query := range []string{"test\x00query", "test\x1fquery", "test\x7fquery", "test\nquery"} {
			t.Run(fmt.Sprintf("%q", query), func(t *testing.T) {
				t.Parallel()
				requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: query}), "query")
			})
		}
	})

	t.Run("query exceeding MaxQueryLength", func(t *testing.T) {
		t.Parallel()
		args := &searxng.SearchArgs{Query: strings.Repeat("a", searxng.MaxQueryLength+1)}
		requireValidationError(t, searxng.ValidateSearchArgs(args), "query")
	})

	t.Run("valid query", func(t *testing.T) {
		t.Parallel()
		if err := searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "golang search"}); err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("invalid time_range", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", TimeRange: "week"}), "time_range")
	})

	t.Run("valid time_range", func(t *testing.T) {
		t.Parallel()
		if err := searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", TimeRange: "day"}); err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("SafeSearch out of range", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", SafeSearch: 3}), "safesearch")
	})

	t.Run("SafeSearch valid", func(t *testing.T) {
		t.Parallel()
		if err := searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", SafeSearch: 2}); err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("Pageno less than one", func(t *testing.T) {
		t.Parallel()
		pageno := 0
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", Pageno: &pageno}), "pageno")
	})

	t.Run("Limit out of range", func(t *testing.T) {
		t.Parallel()
		limit := 21
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", Limit: &limit}), "limit")
	})

	t.Run("Language auto normalized to empty", func(t *testing.T) {
		t.Parallel()
		args := &searxng.SearchArgs{Query: "test", Language: "auto"}
		if err := searxng.ValidateSearchArgs(args); err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
		if args.Language != "" {
			t.Fatalf("Language = %q, want empty string", args.Language)
		}
	})

	t.Run("Language invalid format", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", Language: "en_US"}), "language")
	})

	t.Run("Language valid", func(t *testing.T) {
		t.Parallel()
		if err := searxng.ValidateSearchArgs(&searxng.SearchArgs{Query: "test", Language: "en-US"}); err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})
}

func TestValidateCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		categories string
		wantErr    bool
	}{
		{name: "empty", categories: ""},
		{name: "valid categories", categories: "general,news,science-technology,software wikis"},
		{name: "path separator", categories: "general/news", wantErr: true},
		{name: "control characters", categories: "general\nnews", wantErr: true},
		{name: "empty segment", categories: "general,,news", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := searxng.ValidateCategories(tt.categories)
			if tt.wantErr {
				requireValidationError(t, err, "categories")
				return
			}
			if err != nil {
				t.Fatalf("ValidateCategories() error = %v, want nil", err)
			}
		})
	}
}

func TestValidateEngines(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		engines string
		wantErr bool
	}{
		{name: "empty", engines: ""},
		{name: "valid engines", engines: "google,bing,duckduckgo-lite,docker hub,google news,Torznab EZTV"},
		{name: "path separator", engines: "google/bing", wantErr: true},
		{name: "control characters", engines: "google\tbing", wantErr: true},
		{name: "empty segment", engines: "google,,bing", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := searxng.ValidateEngines(tt.engines)
			if tt.wantErr {
				requireValidationError(t, err, "engines")
				return
			}
			if err != nil {
				t.Fatalf("ValidateEngines() error = %v, want nil", err)
			}
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	err := searxng.NewValidationError("query", "search query cannot be only whitespace")
	if got, want := err.Error(), `validation error on "query": search query cannot be only whitespace`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, searxng.NewValidationError("query", "search query cannot be only whitespace")) {
		t.Fatal("errors.Is() = false, want true for matching ValidationError")
	}
	if errors.Is(err, searxng.NewValidationError("query", "different")) {
		t.Fatal("errors.Is() = true, want false for non-matching ValidationError")
	}
	if !searxng.IsValidationError(fmt.Errorf("wrapped: %w", err)) {
		t.Fatal("IsValidationError() = false, want true for wrapped ValidationError")
	}
	if searxng.IsValidationError(errors.New("plain error")) {
		t.Fatal("IsValidationError() = true, want false for non-validation error")
	}
}

func TestSearXNGErrorAndHTMLResponseError(t *testing.T) {
	t.Parallel()

	underlying := errors.New("network failed")
	searxErr := searxng.NewSearXNGError(503, "text/plain", strings.Repeat("x", searxng.MaxErrorDisplayChars+1), underlying)
	if !errors.Is(searxErr, underlying) {
		t.Fatal("errors.Is() = false, want true for SearXNGError underlying error")
	}
	if searxErr.StatusCode != 503 {
		t.Fatalf("StatusCode = %d, want 503", searxErr.StatusCode)
	}
	if searxErr.RespContentType != "text/plain" {
		t.Fatalf("RespContentType = %q, want text/plain", searxErr.RespContentType)
	}
	if len(searxErr.ResponseBody) != searxng.MaxErrorDisplayChars {
		t.Fatalf("ResponseBody length = %d, want %d", len(searxErr.ResponseBody), searxng.MaxErrorDisplayChars)
	}
	if !strings.Contains(searxErr.Error(), "searxng error (status 503) - content-type text/plain") {
		t.Fatalf("SearXNGError.Error() = %q, want status and content-type", searxErr.Error())
	}

	htmlErr := &searxng.HTMLResponseError{Body: "<html></html>", UnderlyingErr: underlying}
	if got, want := htmlErr.Error(), "searxng returned html instead of json - json output may not be enabled on the server"; got != want {
		t.Fatalf("HTMLResponseError.Error() = %q, want %q", got, want)
	}
	if !errors.Is(htmlErr, underlying) {
		t.Fatal("errors.Is() = false, want true for HTMLResponseError underlying error")
	}
}
