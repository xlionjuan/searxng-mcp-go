package searxng

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"
)

var (
	errPlainTestError   = errors.New("plain error")
	errNetworkTestError = errors.New("network failed")
)

func requireValidationError(t *testing.T, err error, field string) {
	t.Helper()

	if err == nil {
		t.Fatalf("expected validation error for %s, got nil", field)
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}

	if ve.Field != field {
		t.Fatalf("ValidationError.Field = %q, want %q", ve.Field, field)
	}
}

// requireValidationErrorMsg asserts that err is a ValidationError for the given
// field and that its error message contains wantMsg.
func requireValidationErrorMsg(t *testing.T, err error, field, wantMsg string) {
	t.Helper()

	requireValidationError(t, err, field)

	if !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("ValidationError message = %q, want containing %q", err.Error(), wantMsg)
	}
}

func validateSearchArgsErr(args *SearchArgs) error {
	_, err := ValidateSearchArgs(args)

	return err
}

//nolint:gocognit // test exercises many branches across subtests
func TestValidateSearchArgs(t *testing.T) {
	t.Parallel()

	t.Run("nil args", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(nil), "args")
	})

	t.Run("empty query", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{}), "query")
	})

	t.Run("whitespace-only query", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: " \t "}), "query")
	})

	t.Run("query with control characters", func(t *testing.T) {
		t.Parallel()

		for _, query := range []string{"test\x00query", "test\x1fquery", "test\x7fquery", "test\nquery"} {
			t.Run(fmt.Sprintf("%q", query), func(t *testing.T) {
				t.Parallel()
				requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: query}), "query")
			})
		}
	})

	t.Run("query exceeding MaxQueryLength", func(t *testing.T) {
		t.Parallel()

		args := &SearchArgs{Query: strings.Repeat("a", MaxQueryLength+1)}
		requireValidationError(t, validateSearchArgsErr(args), "query")
	})

	t.Run("valid query", func(t *testing.T) {
		t.Parallel()

		_, err := ValidateSearchArgs(&SearchArgs{Query: "golang search"})
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("invalid time_range", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: "test", TimeRange: "week"}), "time_range")
	})

	t.Run("valid time_range", func(t *testing.T) {
		t.Parallel()

		_, err := ValidateSearchArgs(&SearchArgs{Query: "test", TimeRange: "day"})
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("SafeSearch out of range", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: "test", SafeSearch: 3}), "safesearch")
	})

	t.Run("SafeSearch valid", func(t *testing.T) {
		t.Parallel()

		_, err := ValidateSearchArgs(&SearchArgs{Query: "test", SafeSearch: 2})
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})

	t.Run("Pageno less than one", func(t *testing.T) {
		t.Parallel()

		pageno := 0
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: "test", Pageno: &pageno}), "pageno")
	})

	t.Run("Limit out of range", func(t *testing.T) {
		t.Parallel()

		limit := 21
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: "test", Limit: &limit}), "limit")
	})

	t.Run("deep copy prevents pointer aliasing for Pageno and Limit", func(t *testing.T) {
		t.Parallel()

		pageno := 2
		limit := 10
		origPageno := pageno
		origLimit := limit

		args := &SearchArgs{Query: "test", Pageno: &pageno, Limit: &limit}

		got, err := ValidateSearchArgs(args)
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}

		if got.Pageno == args.Pageno {
			t.Fatal("Pageno pointer was not deep-copied: got.Pageno == args.Pageno")
		}

		if got.Limit == args.Limit {
			t.Fatal("Limit pointer was not deep-copied: got.Limit == args.Limit")
		}

		*got.Pageno = 99
		*got.Limit = 99

		if *args.Pageno != origPageno {
			t.Fatalf("original Pageno mutated by result: got %d, want %d", *args.Pageno, origPageno)
		}

		if *args.Limit != origLimit {
			t.Fatalf("original Limit mutated by result: got %d, want %d", *args.Limit, origLimit)
		}
	})

	t.Run("Language auto normalized to empty", func(t *testing.T) {
		t.Parallel()

		args := &SearchArgs{Query: "test", Language: "auto"}

		got, err := ValidateSearchArgs(args)
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}

		if got.Language != "" {
			t.Fatalf("Language = %q, want empty string", got.Language)
		}
	})

	t.Run("Language invalid format", func(t *testing.T) {
		t.Parallel()
		requireValidationError(t, validateSearchArgsErr(&SearchArgs{Query: "test", Language: "en_US"}), "language")
	})

	t.Run("Language valid", func(t *testing.T) {
		t.Parallel()

		_, err := ValidateSearchArgs(&SearchArgs{Query: "test", Language: "en-US"})
		if err != nil {
			t.Fatalf("ValidateSearchArgs() error = %v, want nil", err)
		}
	})
}

// TestValidateLanguagePure verifies the pure-helper contract: validateLanguage
// is a pure function that does not mutate any caller-visible state. It
// returns the normalized value (empty string for "auto", case-insensitive)
// alongside any validation error. This guards against a regression of the
// defensive-programming issue where the helper used to mutate *SearchArgs
// in place, which was a data-race hazard for callers that share the struct
// across goroutines.
//
//nolint:gocognit,gocyclo,cyclop // subtests cover each normalization/validation branch explicitly
func TestValidateLanguagePure(t *testing.T) {
	t.Parallel()

	t.Run("empty stays empty", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("")
		if err != nil {
			t.Fatalf("validateLanguage(\"\") error = %v, want nil", err)
		}

		if got != "" {
			t.Fatalf("validateLanguage(\"\") = %q, want %q", got, "")
		}
	})

	t.Run("auto normalized to empty (lowercase)", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("auto")
		if err != nil {
			t.Fatalf("validateLanguage(\"auto\") error = %v, want nil", err)
		}

		if got != "" {
			t.Fatalf("validateLanguage(\"auto\") = %q, want %q", got, "")
		}
	})

	t.Run("AUTO normalized to empty (uppercase, case-insensitive)", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("AUTO")
		if err != nil {
			t.Fatalf("validateLanguage(\"AUTO\") error = %v, want nil", err)
		}

		if got != "" {
			t.Fatalf("validateLanguage(\"AUTO\") = %q, want %q", got, "")
		}
	})

	t.Run("valid language code returned unchanged", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("en-US")
		if err != nil {
			t.Fatalf("validateLanguage(\"en-US\") error = %v, want nil", err)
		}

		if got != "en-US" {
			t.Fatalf("validateLanguage(\"en-US\") = %q, want %q", got, "en-US")
		}
	})

	t.Run("grandfathered i-klingon accepted", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("i-klingon")
		if err != nil {
			t.Fatalf("validateLanguage(\"i-klingon\") error = %v, want nil", err)
		}

		if got != "i-klingon" {
			t.Fatalf("validateLanguage(\"i-klingon\") = %q, want %q", got, "i-klingon")
		}
	})

	t.Run("private-use x-private accepted", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("x-private")
		if err != nil {
			t.Fatalf("validateLanguage(\"x-private\") error = %v, want nil", err)
		}

		if got != "x-private" {
			t.Fatalf("validateLanguage(\"x-private\") = %q, want %q", got, "x-private")
		}
	})

	t.Run("invalid language code returns ValidationError and original value", func(t *testing.T) {
		t.Parallel()

		got, err := validateLanguage("en_US")
		if err == nil {
			t.Fatal("validateLanguage(\"en_US\") error = nil, want ValidationError")
		}

		requireValidationError(t, err, "language")

		if got != "en_US" {
			t.Fatalf("validateLanguage(\"en_US\") = %q, want original %q on error", got, "en_US")
		}
	})

	t.Run("language exceeding max length returns ValidationError", func(t *testing.T) {
		t.Parallel()

		longLang := strings.Repeat("a", maxLanguageLength+1)

		_, err := validateLanguage(longLang)
		if err == nil {
			t.Fatal("validateLanguage(long) error = nil, want ValidationError")
		}

		requireValidationError(t, err, "language")
	})
}

func TestValidateCategories(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		categories string
		wantErr    bool
		wantMsg    string // expected substring in error message
	}{
		{name: "empty", categories: ""},
		{name: "valid categories", categories: "general,news,science-technology,software wikis"},
		{name: "path separator", categories: "general/news", wantErr: true, wantMsg: "slash not allowed in category name"},
		{name: "control characters", categories: "general\nnews", wantErr: true, wantMsg: "control characters"},
		{name: "empty segment", categories: "general,,news", wantErr: true, wantMsg: "empty item in comma-separated list"},
		{
			name:       "long category",
			categories: "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz",
			wantErr:    true, wantMsg: "exceeds 50 runes",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateCSVIdentifiers(tt.categories, "categories", "category")
			if tt.wantErr {
				requireValidationErrorMsg(t, err, "categories", tt.wantMsg)

				return
			}

			if err != nil {
				t.Fatalf("validateCSVIdentifiers() error = %v, want nil", err)
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
		wantMsg string
	}{
		{name: "empty", engines: ""},
		{name: "valid engines", engines: "google,bing,duckduckgo-lite,docker hub,google news,Torznab EZTV"},
		{name: "path separator", engines: "google/bing", wantErr: true, wantMsg: "slash not allowed in engine name"},
		{name: "control characters", engines: "google	bing", wantErr: true, wantMsg: "control characters"},
		{name: "empty segment", engines: "google,,bing", wantErr: true, wantMsg: "empty item in comma-separated list"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateCSVIdentifiers(tt.engines, "engines", "engine")
			if tt.wantErr {
				requireValidationErrorMsg(t, err, "engines", tt.wantMsg)

				return
			}

			if err != nil {
				t.Fatalf("validateCSVIdentifiers() error = %v, want nil", err)
			}
		})
	}
}

// csvIdentifiersWithByteLength returns a valid CSV value with the requested
// UTF-8 byte length. Each generated item is one rune, so the fixture makes the
// byte-versus-rune distinction explicit without tripping the per-item limit.
func csvIdentifiersWithByteLength(length int) string {
	if length <= 0 {
		return ""
	}

	const item = "界"

	const itemWithSeparatorBytes = len(item) + 1

	itemCount := (length - 1) / itemWithSeparatorBytes
	prefix := strings.Repeat(item+",", itemCount)
	suffix := strings.Repeat("a", length-len(prefix))

	return prefix + suffix
}

func TestValidateCSVIdentifiersAggregateByteLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		noun  string
	}{
		{name: "categories", field: "categories", noun: "category"},
		{name: "engines", field: "engines", noun: "engine"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			atLimit := csvIdentifiersWithByteLength(MaxCSVInputBytes)
			if got := len(atLimit); got != MaxCSVInputBytes {
				t.Fatalf("fixture byte length = %d, want %d", got, MaxCSVInputBytes)
			}

			if got := utf8.RuneCountInString(atLimit); got >= MaxCSVInputBytes {
				t.Fatalf("fixture rune length = %d, want less than byte limit %d", got, MaxCSVInputBytes)
			}

			err := validateCSVIdentifiers(atLimit, tt.field, tt.noun)
			if err != nil {
				t.Fatalf("validateCSVIdentifiers() at byte limit error = %v, want nil", err)
			}

			overLimit := atLimit + "a"
			requireValidationErrorMsg(
				t,
				validateCSVIdentifiers(overLimit, tt.field, tt.noun),
				tt.field,
				tt.noun+" input too long",
			)
		})
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	err := NewValidationError("query", "search query cannot be only whitespace")
	if got, want := err.Error(), `validation error on "query": search query cannot be only whitespace`; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}

	if !errors.Is(err, NewValidationError("query", "search query cannot be only whitespace")) {
		t.Fatal("errors.Is() = false, want true for matching ValidationError")
	}

	if errors.Is(err, NewValidationError("query", "different")) {
		t.Fatal("errors.Is() = true, want false for non-matching ValidationError")
	}

	if !isValidationError(fmt.Errorf("wrapped: %w", err)) {
		t.Fatal("isValidationError() = false, want true for wrapped ValidationError")
	}

	if isValidationError(errPlainTestError) {
		t.Fatal("isValidationError() = true, want false for non-validation error")
	}
}

func TestSearXNGErrorAndHTMLResponseError(t *testing.T) {
	t.Parallel()

	underlying := errNetworkTestError

	searxErr := NewSearXNGError(503, "text/plain", strings.Repeat("x", MaxErrorDisplayBytes+1), underlying)
	if !errors.Is(searxErr, underlying) {
		t.Fatal("errors.Is() = false, want true for SearXNGError underlying error")
	}

	if searxErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("StatusCode = %d, want 503", searxErr.StatusCode)
	}

	if searxErr.RespContentType != "text/plain" {
		t.Fatalf("RespContentType = %q, want text/plain", searxErr.RespContentType)
	}

	if len(searxErr.ResponseBody) != MaxErrorDisplayBytes {
		t.Fatalf("ResponseBody length = %d, want %d", len(searxErr.ResponseBody), MaxErrorDisplayBytes)
	}

	if !strings.Contains(searxErr.Error(), "searxng error (status 503) - content-type text/plain") {
		t.Fatalf("SearXNGError.Error() = %q, want status and content-type", searxErr.Error())
	}

	htmlErr := &HTMLResponseError{Body: "<html></html>", UnderlyingErr: underlying}
	want := "searxng returned html instead of json" +
		" - json output may not be enabled on the server"

	if got := htmlErr.Error(); got != want {
		t.Fatalf("HTMLResponseError.Error() = %q, want %q", got, want)
	}

	if !errors.Is(htmlErr, underlying) {
		t.Fatal("errors.Is() = false, want true for HTMLResponseError underlying error")
	}
}
