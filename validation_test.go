package main

import (
	"fmt"
	"strings"
	"testing"

	searxng "searxng-mcp-go/internal/searxng"
)

func assertValidSearchArgs(t *testing.T, args *searxng.SearchArgs) {
	t.Helper()
	if err := searxng.ValidateSearchArgs(args); err != nil {
		t.Fatalf("expected valid args, got %v", err)
	}
}

func assertValidationError(t *testing.T, args *searxng.SearchArgs, field, contains string) {
	t.Helper()
	err := searxng.ValidateSearchArgs(args)
	if err == nil {
		t.Fatalf("expected validation error for %s", field)
	}
	ve, ok := err.(*searxng.ValidationError)
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
			lang := lang
			t.Run(lang, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Language: lang})
			})
		}
	})

	t.Run("unicode language codes", func(t *testing.T) {
		for _, lang := range []string{"日本語", "中文", "Русский"} {
			lang := lang
			t.Run(lang, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Language: lang})
			})
		}
	})

	t.Run("auto is normalized to empty", func(t *testing.T) {
		t.Parallel()
		args := &searxng.SearchArgs{Query: "test", Language: "auto"}
		if err := searxng.ValidateSearchArgs(args); err != nil {
			t.Fatalf("expected auto to be valid, got %v", err)
		}
		if args.Language != "" {
			t.Fatalf("expected Language to be empty after normalization, got %q", args.Language)
		}
	})

	t.Run("invalid language codes", func(t *testing.T) {
		for _, lang := range []string{"INVALID_LANG", "123", "e", "en123", "en!@#", "en_US", "en-"} {
			lang := lang
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
			tr := tr
			t.Run(tr, func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", TimeRange: tr})
			})
		}
	})

	t.Run("invalid time ranges", func(t *testing.T) {
		for _, tr := range []string{"hour", "week", "all", "123"} {
			tr := tr
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
			assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Categories: "general,news"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general\nnews"}, "categories", "invalid category")
		})
		t.Run("invalid identifier", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Categories: "general!@#"}, "categories", "invalid category")
		})
	})

	t.Run("engines", func(t *testing.T) {
		t.Parallel()
		t.Run("valid", func(t *testing.T) {
			t.Parallel()
			assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Engines: "google,bing"})
		})
		t.Run("invalid control characters", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google\tbing"}, "engines", "invalid engine")
		})
		t.Run("invalid identifier", func(t *testing.T) {
			t.Parallel()
			assertValidationError(t, &searxng.SearchArgs{Query: "test", Engines: "google!@#"}, "engines", "invalid engine")
		})
	})
}

func TestValidateSearchArgs_SafeSearch(t *testing.T) {
	t.Parallel()

	t.Run("valid values", func(t *testing.T) {
		for _, ss := range []int{0, 1, 2} {
			ss := ss
			t.Run(fmt.Sprintf("value_%d", ss), func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", SafeSearch: ss})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		for _, ss := range []int{-1, 3, -999, 999} {
			ss := ss
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
			page := page
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				t.Parallel()
				assertValidSearchArgs(t, &searxng.SearchArgs{Query: "test", Pageno: &page})
			})
		}
	})

	t.Run("invalid values", func(t *testing.T) {
		for _, page := range []int{0, -1, -999} {
			page := page
			t.Run(fmt.Sprintf("value_%d", page), func(t *testing.T) {
				t.Parallel()
				assertValidationError(t, &searxng.SearchArgs{Query: "test", Pageno: &page}, "pageno", "must be >= 1")
			})
		}
	})
}

// --- containsControlCharacters 獨立單元測試 ---

func TestContainsControlCharacters(t *testing.T) {
	t.Parallel()

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("") {
			t.Error("ContainsControlCharacters(\"\") = true, want false")
		}
	})

	t.Run("normal printable ASCII", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("hello world") {
			t.Error("ContainsControlCharacters(hello world) = true, want false")
		}
	})

	t.Run("tab character", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("hello\tworld") {
			t.Error("ContainsControlCharacters(hello\\tworld) = false, want true")
		}
	})

	t.Run("newline character", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("hello\nworld") {
			t.Error("ContainsControlCharacters(hello\\nworld) = false, want true")
		}
	})

	t.Run("carriage return", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("hello\rworld") {
			t.Error("ContainsControlCharacters(hello\\rworld) = false, want true")
		}
	})

	t.Run("null character \\x00", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("test\x00null") {
			t.Error("ContainsControlCharacters(test\\x00null) = false, want true")
		}
	})

	t.Run("boundary \\x1f (last control char)", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("test\x1fctrl") {
			t.Error("ContainsControlCharacters(test\\x1fctrl) = false, want true")
		}
	})

	t.Run("space \\x20 (first printable)", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("hello world") {
			t.Error("ContainsControlCharacters(hello world) = true, want false (space is printable)")
		}
	})

	t.Run("DEL character \\x7f", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("test\x7fdel") {
			t.Error("ContainsControlCharacters(test\\x7fdel) = false, want true")
		}
	})

	t.Run("boundary \\x80 (above DEL)", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("test\x80above") {
			t.Error("ContainsControlCharacters(test\\x80above) = true, want false (\\x80 is not a control char)")
		}
	})

	t.Run("unicode CJK characters", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("你好世界") {
			t.Error("ContainsControlCharacters(你好世界) = true, want false")
		}
	})

	t.Run("unicode emoji", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("🔥🎉😀") {
			t.Error("ContainsControlCharacters(🔥🎉😀) = true, want false")
		}
	})

	t.Run("mixed printable and unicode", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("hello 你好 🔥") {
			t.Error("ContainsControlCharacters(hello 你好 🔥) = true, want false")
		}
	})

	t.Run("only control characters", func(t *testing.T) {
		t.Parallel()
		if !searxng.ContainsControlCharacters("\x00\x01\x02\x1f\x7f") {
			t.Error("ContainsControlCharacters(only controls) = false, want true")
		}
	})

	t.Run("digits and punctuation", func(t *testing.T) {
		t.Parallel()
		if searxng.ContainsControlCharacters("123!@#$%^&*()") {
			t.Error("ContainsControlCharacters(digits+punctuation) = true, want false")
		}
	})

	t.Run("non-breaking space U+00A0", func(t *testing.T) {
		t.Parallel()
		// Non-breaking space is not a control character (0xA0 > 0x7f, but not 127)
		// U+00A0 is 0xC2 0xA0 in UTF-8. As a rune it's 160 which is > 127.
		if searxng.ContainsControlCharacters("hello\u00a0world") {
			t.Error("ContainsControlCharacters(hello\\u00a0world) = true, want false")
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
