package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	searxng "searxng-mcp-go/internal/searxng"
)

func assertValidSearchArgs(t *testing.T, args *searxng.SearchArgs) {
	t.Helper()

	err := searxng.ValidateSearchArgs(args)
	if err != nil {
		t.Fatalf("expected valid args, got %v", err)
	}
}

func assertValidationError(t *testing.T, args *searxng.SearchArgs, field, contains string) {
	t.Helper()

	err := searxng.ValidateSearchArgs(args)
	if err == nil {
		t.Fatalf("expected validation error for %s", field)
	}

	var validationErr *searxng.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if validationErr.Field != field {
		t.Fatalf("field = %q, want %q", validationErr.Field, field)
	}

	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("error %q does not contain %q", err.Error(), contains)
	}
}

func TestValidateSearchArgs_NilAndQuery(t *testing.T) {
	t.Parallel()

	t.Run("nil args", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, nil, "args", "search arguments cannot be nil")
	})

	t.Run("empty query", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: ""}, "query", "search query cannot be only whitespace")
	})

	t.Run("whitespace query", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: " \t\n "}, "query", "search query cannot be only whitespace")
	})

	t.Run("long query", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: strings.Repeat("a", searxng.MaxQueryLength+1)}, "query", "must be 500 characters or less")
	})

	t.Run("valid query", func(t *testing.T) {
		t.Parallel()
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "golang search"})
	})

	t.Run("emoji query", func(t *testing.T) {
		t.Parallel()
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "search 🔍 with emoji"})
	})

	t.Run("query with control characters", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: "test\nquery"}, "query", "invalid control characters")
	})
}

func TestValidateSearchArgs_Language(t *testing.T) {
	t.Parallel()

	t.Run("empty language", func(t *testing.T) {
		t.Parallel()
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Language: ""})
	})

	t.Run("valid language codes", func(t *testing.T) {
		for _, lang := range []string{"en", "EN", "zh-tw", "ja", "en-US", "pt-BR", "sr-Latn", "sr-Latn-RS", "es-419", "ZH-hant", "auto", "AUTO", "Auto"} {
			t.Run(lang, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Language: lang})
			})
		}
	})

	t.Run("unicode language codes", func(t *testing.T) {
		for _, lang := range []string{"日本語", "中文", "Русский"} {
			t.Run(lang, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Language: lang})
			})
		}
	})

	t.Run("auto is normalized to empty", func(t *testing.T) {
		t.Parallel()

		args := &searxng.SearchArgs{Query: "test", Language: "auto"}
		err := searxng.ValidateSearchArgs(args)
		if err != nil {
			t.Fatalf("expected auto to be valid, got %v", err)
		}

		if args.Language != "" {
			t.Fatalf("expected Language to be empty after normalization, got %q", args.Language)
		}
	})

	t.Run("invalid language codes", func(t *testing.T) {
		for _, lang := range []string{"INVALID_LANG", "123", "e", "en123", "en!@#", "en_US", "en-"} {
			t.Run(lang, func(t *testing.T) {
				t.Parallel()
				assertValidationError(t, &searxng.SearchArgs{Query: "test", Language: lang}, "language", "valid language code")
			})
		}
	})

	t.Run("too long language tag", func(t *testing.T) {
		t.Parallel()

		longLang := strings.Repeat("a-", 40) + "a"
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Language: longLang}, "language", "35 characters or less")
	})
}

func TestValidateSearchArgs_TimeRange(t *testing.T) {
	t.Parallel()

	t.Run("empty time_range", func(t *testing.T) {
		t.Parallel()
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test"})
	})

	t.Run("valid time ranges", func(t *testing.T) {
		for _, tr := range []string{"day", "month", "year"} {
			t.Run(tr, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", TimeRange: tr})
			})
		}
	})

	t.Run("invalid time ranges", func(t *testing.T) {
		for _, tr := range []string{"hour", "week", "all", "123"} {
			t.Run(tr, func(t *testing.T) {
				t.Parallel()
				assertValidationError(t, &searxng.SearchArgs{Query: "test", TimeRange: tr}, "time_range", "day, month or year")
			})
		}
	})
}

func TestValidateSearchArgs_CategoriesAndEngines(t *testing.T) {
	t.Parallel()

	t.Run("categories", func(t *testing.T) {
		t.Parallel()
		t.Run("valid", func(t *testing.T) {
			t.Parallel()
			assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Categories: "general,news,software wikis"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general\nnews"}, "categories", "invalid category")
		})
		t.Run("invalid path separator", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general/news"}, "categories", "invalid category")
		})
	})

	t.Run("engines", func(t *testing.T) {
		t.Parallel()
		t.Run("valid", func(t *testing.T) {
			t.Parallel()
			assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Engines: "google,bing,docker hub,google news,Torznab EZTV"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google\tbing"}, "engines", "invalid engine")
		})
		t.Run("invalid path separator", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google/bing"}, "engines", "invalid engine")
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google\\bing"}, "engines", "invalid engine")
		})
	})
}

func TestValidateSearchArgs_SafeSearch(t *testing.T) {
	t.Parallel()

	t.Run("valid values", func(t *testing.T) {
		t.Parallel()

		for _, ss := range []int{0, 1, 2} {
			t.Run(fmt.Sprintf("value_%d", ss), func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", SafeSearch: ss})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		t.Parallel()

		for _, ss := range []int{-1, 3, -999, 999} {
			t.Run(fmt.Sprintf("value_%d", ss), func(t *testing.T) {
				t.Parallel()
				assertValidationError(t, &searxng.SearchArgs{Query: "test", SafeSearch: ss}, "safesearch", "0 off, 1 moderate, or 2 strict")
			})
		}
	})
}

func TestValidateSearchArgs_Pageno(t *testing.T) {
	t.Parallel()

	t.Run("nil is valid", func(t *testing.T) {
		t.Parallel()
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Pageno: nil})
	})

	t.Run("valid values", func(t *testing.T) {
		for _, page := range []int{1, 5, 1000000} {
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Pageno: &page})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		for _, page := range []int{0, -1, -999} {
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				t.Parallel()
				assertValidationError(t, &searxng.SearchArgs{Query: "test", Pageno: &page}, "pageno", "must be >= 1")
			})
		}
	})
}

func TestValidateSearchArgs_CategoriesAndEngines_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("max identifier length", func(t *testing.T) {
		t.Parallel()

		longIdentifier := strings.Repeat("a", 51)
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: longIdentifier}, "categories", "invalid category")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: longIdentifier}, "engines", "invalid engine")
	})

	t.Run("exactly max identifier length", func(t *testing.T) {
		t.Parallel()

		validIdentifier := strings.Repeat("a", 50)
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Categories: validIdentifier})
		assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Engines: validIdentifier})
	})

	t.Run("empty comma segments", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google,,bing"}, "engines", "invalid engine")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google,"}, "engines", "invalid engine")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: ",google"}, "engines", "invalid engine")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general,,news"}, "categories", "invalid category")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general,"}, "categories", "invalid category")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: ",news"}, "categories", "invalid category")
	})

	t.Run("whitespace-only segments", func(t *testing.T) {
		t.Parallel()
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "  "}, "engines", "invalid engine")
		assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google,  ,bing"}, "engines", "invalid engine")
	})
}
