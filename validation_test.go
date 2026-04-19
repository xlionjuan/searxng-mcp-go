package main

import (
	"fmt"
	"strings"
	"testing"
)

func assertValidSearchArgs(t *testing.T, args *SearchArgs) {
	t.Helper()
	if err := ValidateSearchArgs(args); err != nil {
		t.Fatalf("expected valid args, got %v", err)
	}
}

func assertValidationError(t *testing.T, args *SearchArgs, field, contains string) {
	t.Helper()
	err := ValidateSearchArgs(args)
	if err == nil {
		t.Fatalf("expected validation error for %s", field)
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.Field != field {
		t.Fatalf("field = %q, want %q", ve.Field, field)
	}
	if contains != "" && !strings.Contains(err.Error(), contains) {
		t.Fatalf("error %q does not contain %q", err.Error(), contains)
	}
}

func TestValidateSearchArgs_NilAndQuery(t *testing.T) {
	t.Run("nil args", func(t *testing.T) {
		assertValidationError(t, nil, "args", "search arguments cannot be nil")
	})

	t.Run("empty query", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: ""}, "query", "search query cannot be only whitespace")
	})

	t.Run("whitespace query", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: " \t\n "}, "query", "search query cannot be only whitespace")
	})

	t.Run("long query", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: strings.Repeat("a", MaxQueryLength+1)}, "query", "must be 500 characters or less")
	})

	t.Run("valid query", func(t *testing.T) {
		assertValidSearchArgs(t, &SearchArgs{Query: "golang search"})
	})

	t.Run("query with control characters", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: "test\nquery"}, "query", "invalid control characters")
	})
}

func TestValidateSearchArgs_Language(t *testing.T) {
	t.Run("empty language", func(t *testing.T) {
		assertValidSearchArgs(t, &SearchArgs{Query: "test", Language: ""})
	})

	t.Run("valid language codes", func(t *testing.T) {
		for _, lang := range []string{"en", "zh-tw", "ja", "en-US", "pt-BR", "sr-Latn", "es-419", "ZH-hant"} {
			t.Run(lang, func(t *testing.T) {
				assertValidSearchArgs(t, &SearchArgs{Query: "test", Language: lang})
			})
		}
	})

	t.Run("invalid language codes", func(t *testing.T) {
		for _, lang := range []string{"INVALID_LANG", "123", "e", "en123", "en!@#", "en_US", "en-", "auto"} {
			t.Run(lang, func(t *testing.T) {
				assertValidationError(t, &SearchArgs{Query: "test", Language: lang}, "language", "valid language code")
			})
		}
	})
}

func TestValidateSearchArgs_TimeRange(t *testing.T) {
	t.Run("empty time_range", func(t *testing.T) {
		assertValidSearchArgs(t, &SearchArgs{Query: "test"})
	})

	t.Run("valid time ranges", func(t *testing.T) {
		for _, tr := range []string{"day", "month", "year"} {
			t.Run(tr, func(t *testing.T) {
				assertValidSearchArgs(t, &SearchArgs{Query: "test", TimeRange: tr})
			})
		}
	})

	t.Run("invalid time ranges", func(t *testing.T) {
		for _, tr := range []string{"hour", "week", "all", "123"} {
			t.Run(tr, func(t *testing.T) {
				assertValidationError(t, &SearchArgs{Query: "test", TimeRange: tr}, "time_range", "day, month or year")
			})
		}
	})
}

func TestValidateSearchArgs_CategoriesAndEngines(t *testing.T) {
	t.Run("categories", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			assertValidSearchArgs(t, &SearchArgs{Query: "test", Categories: "general,news"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			assertValidationError(t, &SearchArgs{Query: "test", Categories: "general\nnews"}, "categories", "invalid control characters")
		})
		t.Run("invalid identifier", func(t *testing.T) {
			assertValidationError(t, &SearchArgs{Query: "test", Categories: "general!@#"}, "categories", "invalid category")
		})
	})

	t.Run("engines", func(t *testing.T) {
		t.Run("valid", func(t *testing.T) {
			assertValidSearchArgs(t, &SearchArgs{Query: "test", Engines: "google,bing"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			assertValidationError(t, &SearchArgs{Query: "test", Engines: "google\tbing"}, "engines", "invalid control characters")
		})
		t.Run("invalid identifier", func(t *testing.T) {
			assertValidationError(t, &SearchArgs{Query: "test", Engines: "google!@#"}, "engines", "invalid engine")
		})
	})
}

func TestValidateSearchArgs_SafeSearch(t *testing.T) {
	t.Run("valid values", func(t *testing.T) {
		for _, ss := range []int{0, 1, 2} {
			ss := ss
			t.Run(fmt.Sprintf("value_%d", ss), func(t *testing.T) {
				assertValidSearchArgs(t, &SearchArgs{Query: "test", SafeSearch: ss})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		for _, ss := range []int{-1, 3, -999, 999} {
			ss := ss
			t.Run(fmt.Sprintf("value_%d", ss), func(t *testing.T) {
				assertValidationError(t, &SearchArgs{Query: "test", SafeSearch: ss}, "safesearch", "0 off, 1 moderate, or 2 strict")
			})
		}
	})
}

func TestValidateSearchArgs_Pageno(t *testing.T) {
	t.Run("nil is valid", func(t *testing.T) {
		assertValidSearchArgs(t, &SearchArgs{Query: "test", Pageno: nil})
	})

	t.Run("valid values", func(t *testing.T) {
		for _, page := range []int{1, 5, 1000000} {
			page := page
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				assertValidSearchArgs(t, &SearchArgs{Query: "test", Pageno: &page})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		for _, page := range []int{0, -1, -999} {
			page := page
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				assertValidationError(t, &SearchArgs{Query: "test", Pageno: &page}, "pageno", "must be >= 1")
			})
		}
	})
}

func TestValidateSearchArgs_CategoriesAndEngines_EdgeCases(t *testing.T) {
	t.Run("max identifier length", func(t *testing.T) {
		longIdentifier := strings.Repeat("a", 51)
		assertValidationError(t, &SearchArgs{Query: "test", Categories: longIdentifier}, "categories", "invalid category")
		assertValidationError(t, &SearchArgs{Query: "test", Engines: longIdentifier}, "engines", "invalid engine")
	})

	t.Run("exactly max identifier length", func(t *testing.T) {
		validIdentifier := strings.Repeat("a", 50)
		assertValidSearchArgs(t, &SearchArgs{Query: "test", Categories: validIdentifier})
		assertValidSearchArgs(t, &SearchArgs{Query: "test", Engines: validIdentifier})
	})

	t.Run("empty comma segments", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: "test", Engines: "google,,bing"}, "engines", "invalid engine")
		assertValidationError(t, &SearchArgs{Query: "test", Engines: "google,"}, "engines", "invalid engine")
		assertValidationError(t, &SearchArgs{Query: "test", Engines: ",google"}, "engines", "invalid engine")
		assertValidationError(t, &SearchArgs{Query: "test", Categories: "general,,news"}, "categories", "invalid category")
	})

	t.Run("whitespace-only segments", func(t *testing.T) {
		assertValidationError(t, &SearchArgs{Query: "test", Engines: "  "}, "engines", "invalid engine")
		assertValidationError(t, &SearchArgs{Query: "test", Engines: "google,  ,bing"}, "engines", "invalid engine")
	})
}
