package main

import (
	"errors"
	"strings"
	"testing"

	searxng "searxng-mcp-go/internal/searxng"
)

// ============================================================================
// FuzzValidateSearchArgs
// ============================================================================

func FuzzValidateSearchArgs(f *testing.F) {
	// Seed corpus: combinations of query, language, safesearch, time_range, categories, engines, pageno
	// Format: query, language, safesearch, time_range, categories, engines, pageno
	f.Add("golang search", "en", int64(0), "", "", "", int64(1))                                  // valid minimal
	f.Add("test", "", int64(0), "", "", "", int64(1))                                             // valid no language
	f.Add("", "", int64(0), "", "", "", int64(1))                                                 // empty query → invalid
	f.Add("test", "auto", int64(0), "", "", "", int64(1))                                         // auto lang → normalized
	f.Add("test", "INVALID_LANG!", int64(0), "", "", "", int64(1))                                // invalid language
	f.Add("test", "en", int64(2), "month", "general,news", "google,bing", int64(1))               // all valid
	f.Add("test\nquery", "en", int64(0), "", "", "", int64(1))                                    // control char in query
	f.Add("test", "en", int64(-1), "", "", "", int64(1))                                          // invalid safesearch
	f.Add("test", "en", int64(3), "", "", "", int64(1))                                           // invalid safesearch
	f.Add("test", "en", int64(0), "hour", "", "", int64(1))                                       // invalid time_range
	f.Add("test", "en", int64(0), "", "invalid!cat", "", int64(1))                                // invalid category
	f.Add("test", "en", int64(0), "", "", "invalid!eng", int64(1))                                // invalid engine
	f.Add("test", "en", int64(0), "", "", "", int64(0))                                           // invalid pageno
	f.Add("test", "en", int64(0), "", "", "", int64(-1))                                          // invalid pageno
	f.Add(strings.Repeat("a", searxng.MaxQueryLength+1), "en", int64(0), "", "", "", int64(1))    // too long query
	f.Add("test", strings.Repeat("a", 36), int64(0), "", "", "", int64(1))                        // too long language
	f.Add("test", "en", int64(0), "", strings.Repeat("a", 51), "", int64(1))                      // too long category
	f.Add("test", "en", int64(0), "", "", strings.Repeat("a", 51), int64(1))                      // too long engine
	f.Add("test", "zh-tw", int64(1), "day", "general", "google", int64(5))                        // valid complex
	f.Add("search 🔍 emoji", "ja", int64(0), "year", "news,general", "bing,duckduckgo", int64(10)) // unicode + complex
	f.Add("   ", "en", int64(0), "", "", "", int64(1))                                            // whitespace-only query

	f.Fuzz(func(t *testing.T, query, language string, safeSearch int64,
		timeRange, categories, engines string, pageno int64,
	) {
		// Clamp safesearch to a reasonable range for the seed — fuzz can still overflow
		ss := int(safeSearch)
		if safeSearch < -1000000 {
			ss = -1
		} else if safeSearch > 1000000 {
			ss = 3
		}

		var pn *int
		// pageno == 0 is treated as "not set" (nil pointer)
		// negative values are treated as explicitly invalid
		pnVal := int(pageno)
		if pageno != 0 {
			pn = &pnVal
		}

		args := &searxng.SearchArgs{
			Query:      query,
			Language:   language,
			SafeSearch: ss,
			TimeRange:  timeRange,
			Categories: categories,
			Engines:    engines,
			Pageno:     pn,
		}

		// ValidateSearchArgs must not panic on any input
		err := searxng.ValidateSearchArgs(args)
		// Invariant: nil args must return error
		// (args is always non-nil here, but we verify the nil path separately)
		// Invariant: if err is non-nil, it should be a *ValidationError
		if err != nil {
			var validationErr *searxng.ValidationError
			if !errors.As(err, &validationErr) {
				t.Errorf("ValidateSearchArgs returned non-ValidationError: %T: %v", err, err)
			}
		}

		// Invariant: if Query is empty/whitespace only, must return error
		if strings.TrimSpace(query) == "" && err == nil {
			t.Errorf("ValidateSearchArgs succeeded with whitespace-only query %q", query)
		}

		// Invariant: after validation, "auto" language should be normalized to empty
		if strings.EqualFold(language, "auto") && err == nil {
			if args.Language != "" {
				t.Errorf("ValidateSearchArgs did not normalize 'auto' language; got %q", args.Language)
			}
		}

		_ = err
	})
}

// ============================================================================
// FuzzUnescapeIfNeeded
// ============================================================================

func FuzzUnescapeIfNeeded(f *testing.F) {
	// Seed corpus: strings with/without HTML entities, edge cases
	seeds := []string{
		"",                                    // empty
		"plain text no entities",              // no entities
		"hello &amp; goodbye",                 // with &amp;
		"<script>alert(1)</script>",           // with < and >
		"&lt;div&gt;content&lt;/div&gt;",      // with &lt; &gt;
		"quote: &quot;hello&quot;",            // with &quot;
		"&#39;single quote&#39;",              // numeric entity
		"&#x27;hex entity&#x27;",              // hex entity
		"&amp;&lt;&gt;&quot;",                 // all four triggers
		"mixed &amp; plain <text> \"quoted\"", // mixed entities + triggers
		"a & b",                               // lone &
		"a < b",                               // lone <
		"a > b",                               // lone >
		"a \" b",                              // lone "
		"a &amp; b &lt; c &gt; d &quot; e",    // all entities
		"日本語 &amp; 中文",                        // unicode + entities
		"🔥 &lt;emoji&gt; 🎉",                   // emoji + entities
		"no special chars here!",              // regular sentence
		"a &lt; b &amp;&amp; c",               // consecutive entities
		"&&&&&",                               // only ampersands
		"<<<>>>\"\"\"",                        // only triggers, no entities
		"&notanentity;",                       // invalid entity-like
		"&",                                   // just an ampersand
		"hello&world",                         // ampersand in middle of word (no entity)
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, s string) {
		result := searxng.UnescapeIfNeeded(s)

		// Invariant: if input has no '&', '<', '>', '"', result must be identical to input
		if !strings.ContainsAny(s, "&<>\"") {
			if result != s {
				t.Errorf("searxng.UnescapeIfNeeded(%q)=%q, expected same string (no trigger chars)", s, result)
			}
		}

		// Invariant: applying unescapeIfNeeded multiple times eventually stabilizes
		// (html.UnescapeString is not strictly idempotent in Go, so we iterate to fixed point)
		current := result
		for range 10 {
			next := searxng.UnescapeIfNeeded(current)
			if next == current {
				break
			}

			current = next
		}
		// After stabilization, one more pass should be identity
		final := searxng.UnescapeIfNeeded(current)
		if final != current {
			t.Errorf("unescapeIfNeeded did not stabilize after 10 iterations: input=%q, stable=%q, final=%q", s, current, final)
		}

		// Invariant: result must not be longer than input * some reasonable factor
		// (unescaping can make strings shorter but never dramatically longer)
		if len(result) > len(s)*10+100 {
			t.Errorf("searxng.UnescapeIfNeeded(%q) returned suspiciously long result: len=%d", s, len(result))
		}

		// Invariant: empty input yields empty output
		if len(s) == 0 && len(result) != 0 {
			t.Errorf("searxng.UnescapeIfNeeded(\"\")=%q, want empty", result)
		}
	})
}
