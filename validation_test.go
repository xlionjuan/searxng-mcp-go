package main

import (
	"strings"
	"testing"
)

// --- ValidateSearchArgs tests ---

func TestValidateSearchArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        *SearchArgs
		wantErr     bool
		errField    string
		errContains string // Optional substring that should be in error message
	}{
		// nil args
		{
			name:     "nil args",
			args:     nil,
			wantErr:  true,
			errField: "args",
		},
		// empty query
		{
			name:        "empty query",
			args:        &SearchArgs{Query: ""},
			wantErr:     true,
			errField:    "query",
			errContains: "search query cannot be only whitespace",
		},
		// query too long (> 500 characters)
		{
			name:        "query too long",
			args:        &SearchArgs{Query: strings.Repeat("a", 501)},
			wantErr:     true,
			errField:    "query",
			errContains: "must be 500 characters or less",
		},
		// query exactly 500 characters (should pass)
		{
			name:    "query exactly 500 characters",
			args:    &SearchArgs{Query: strings.Repeat("a", 500)},
			wantErr: false,
		},
		// invalid time_range
		{
			name:     "invalid time_range hour",
			args:     &SearchArgs{Query: "test", TimeRange: "hour"},
			wantErr:  true,
			errField: "time_range",
		},
		{
			name:     "invalid time_range week",
			args:     &SearchArgs{Query: "test", TimeRange: "week"},
			wantErr:  true,
			errField: "time_range",
		},
		{
			name:     "invalid time_range invalid",
			args:     &SearchArgs{Query: "test", TimeRange: "invalid"},
			wantErr:  true,
			errField: "time_range",
		},
		// valid time_range values
		{
			name:    "valid time_range day",
			args:    &SearchArgs{Query: "test", TimeRange: "day"},
			wantErr: false,
		},
		{
			name:    "valid time_range month",
			args:    &SearchArgs{Query: "test", TimeRange: "month"},
			wantErr: false,
		},
		{
			name:    "valid time_range year",
			args:    &SearchArgs{Query: "test", TimeRange: "year"},
			wantErr: false,
		},
		// empty time_range is valid (optional)
		{
			name:    "empty time_range is valid",
			args:    &SearchArgs{Query: "test", TimeRange: ""},
			wantErr: false,
		},
		// safesearch out of range
		{
			name:     "safesearch negative",
			args:     &SearchArgs{Query: "test", SafeSearch: -1},
			wantErr:  true,
			errField: "safesearch",
		},
		{
			name:     "safesearch too large",
			args:     &SearchArgs{Query: "test", SafeSearch: 3},
			wantErr:  true,
			errField: "safesearch",
		},
		// safesearch valid values
		{
			name:    "safesearch 0",
			args:    &SearchArgs{Query: "test", SafeSearch: 0},
			wantErr: false,
		},
		{
			name:    "safesearch 1",
			args:    &SearchArgs{Query: "test", SafeSearch: 1},
			wantErr: false,
		},
		{
			name:    "safesearch 2",
			args:    &SearchArgs{Query: "test", SafeSearch: 2},
			wantErr: false,
		},
		// normal valid args
		{
			name:    "valid args with all fields",
			args:    &SearchArgs{Query: "golang mcp", Language: "en", SafeSearch: 1, TimeRange: "month"},
			wantErr: false,
		},
		{
			name:    "valid args with only query",
			args:    &SearchArgs{Query: "test search"},
			wantErr: false,
		},
		// pageno validation
		{
			name:     "pageno zero",
			args:     &SearchArgs{Query: "test", Pageno: intPtr(0)},
			wantErr:  true,
			errField: "pageno",
		},
		{
			name:     "pageno negative",
			args:     &SearchArgs{Query: "test", Pageno: intPtr(-1)},
			wantErr:  true,
			errField: "pageno",
		},
		{
			name:    "pageno valid 1",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(1)},
			wantErr: false,
		},
		{
			name:    "pageno valid 5",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(5)},
			wantErr: false,
		},
		{
			name:    "pageno nil is valid",
			args:    &SearchArgs{Query: "test", Pageno: nil},
			wantErr: false,
		},
		// language validation
		{
			name:    "language empty is valid",
			args:    &SearchArgs{Query: "test", Language: ""},
			wantErr: false,
		},
		{
			name:    "language en is valid",
			args:    &SearchArgs{Query: "test", Language: "en"},
			wantErr: false,
		},
		{
			name:    "language zh-tw is valid",
			args:    &SearchArgs{Query: "test", Language: "zh-tw"},
			wantErr: false,
		},
		{
			name:    "language ja is valid",
			args:    &SearchArgs{Query: "test", Language: "ja"},
			wantErr: false,
		},
		{
			name:     "language invalid",
			args:     &SearchArgs{Query: "test", Language: "INVALID_LANG"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language invalid2",
			args:     &SearchArgs{Query: "test", Language: "xyz"},
			wantErr:  true,
			errField: "language",
		},
		// Control characters in query
		{
			name:        "query with newline",
			args:        &SearchArgs{Query: "test\nquery"},
			wantErr:     true,
			errField:    "query",
			errContains: "invalid control characters",
		},
		{
			name:        "query with tab",
			args:        &SearchArgs{Query: "test\tquery"},
			wantErr:     true,
			errField:    "query",
			errContains: "invalid control characters",
		},
		{
			name:        "query with null byte",
			args:        &SearchArgs{Query: "test\x00query"},
			wantErr:     true,
			errField:    "query",
			errContains: "invalid control characters",
		},
		// Control characters in categories
		{
			name:        "categories with newline",
			args:        &SearchArgs{Query: "test", Categories: "general\nnews"},
			wantErr:     true,
			errField:    "categories",
			errContains: "invalid control characters",
		},
		{
			name:        "categories with tab",
			args:        &SearchArgs{Query: "test", Categories: "general\tnews"},
			wantErr:     true,
			errField:    "categories",
			errContains: "invalid control characters",
		},
		// Control characters in engines
		{
			name:        "engines with newline",
			args:        &SearchArgs{Query: "test", Engines: "google\nbing"},
			wantErr:     true,
			errField:    "engines",
			errContains: "invalid control characters",
		},
		{
			name:        "engines with null byte",
			args:        &SearchArgs{Query: "test", Engines: "google\x00bing"},
			wantErr:     true,
			errField:    "engines",
			errContains: "invalid control characters",
		},
		// =============================================
		// EXTREME VALIDATION TESTS - Edge Cases & Bugs
		// =============================================

		// --- Query: whitespace-only (BUG: should fail) ---
		{
			name:     "query whitespace only spaces",
			args:     &SearchArgs{Query: "   "},
			wantErr:  true,
			errField: "query",
		},
		{
			name:     "query whitespace only tabs",
			args:     &SearchArgs{Query: "\t\t"},
			wantErr:  true,
			errField: "query",
		},
		{
			name:     "query whitespace only mixed",
			args:     &SearchArgs{Query: " \t \n "},
			wantErr:  true,
			errField: "query",
		},

		// --- Query: very long strings (>10KB) ---
		{
			name:     "query 11KB of ASCII",
			args:     &SearchArgs{Query: strings.Repeat("a", 11*1024)},
			wantErr:  true,
			errField: "query",
		},
		{
			name:     "query 100KB of ASCII",
			args:     &SearchArgs{Query: strings.Repeat("x", 100*1024)},
			wantErr:  true,
			errField: "query",
		},

		// --- Query: unicode edge cases (removed exploratory tests) ---
		{
			name:    "query unicode devanagari Zah",
			args:    &SearchArgs{Query: "हिन्दी"},
			wantErr: false, // Valid unicode in script
		},
		{
			name:    "query unicode mixed valid",
			args:    &SearchArgs{Query: "Hello 世界 🌍"},
			wantErr: false,
		},

		// --- Query: unicode homoglyphs (homograph attacks) ---
		{
			name:    "query unicode Cyrillic homoglyph a",
			args:    &SearchArgs{Query: "аптека"}, // Cyrillic 'а' instead of Latin 'a'
			wantErr: false,
		},
		{
			name:    "query unicode Greek homoglyph o",
			args:    &SearchArgs{Query: "οκρινοκ"}, // Greek 'ο' instead of Latin 'o'
			wantErr: false,
		},

		// --- Query: null bytes (various positions) ---
		{
			name:     "query null byte at start",
			args:     &SearchArgs{Query: "\x00hello"},
			wantErr:  true,
			errField: "query",
		},
		{
			name:     "query null byte at end",
			args:     &SearchArgs{Query: "hello\x00"},
			wantErr:  true,
			errField: "query",
		},
		{
			name:     "query multiple null bytes",
			args:     &SearchArgs{Query: "hel\x00lo\x00"},
			wantErr:  true,
			errField: "query",
		},

		// --- Language codes: invalid/edge cases ---
		{
			name:    "language empty is valid",
			args:    &SearchArgs{Query: "test", Language: ""},
			wantErr: false,
		},
		{
			name:     "language number",
			args:     &SearchArgs{Query: "test", Language: "123"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language special chars",
			args:     &SearchArgs{Query: "test", Language: "en!@#"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language with numbers in code",
			args:     &SearchArgs{Query: "test", Language: "en123"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language single char",
			args:     &SearchArgs{Query: "test", Language: "e"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language three chars",
			args:     &SearchArgs{Query: "test", Language: "xyz"},
			wantErr:  true,
			errField: "language",
		},
		{
			name:     "language uppercase",
			args:     &SearchArgs{Query: "test", Language: "EN"},
			wantErr:  true,
			errField: "language",
		},

		// --- SafeSearch: invalid values ---
		{
			name:     "safesearch negative one",
			args:     &SearchArgs{Query: "test", SafeSearch: -1},
			wantErr:  true,
			errField: "safesearch",
		},
		{
			name:     "safesearch three",
			args:     &SearchArgs{Query: "test", SafeSearch: 3},
			wantErr:  true,
			errField: "safesearch",
		},
		{
			name:     "safesearch large negative",
			args:     &SearchArgs{Query: "test", SafeSearch: -999},
			wantErr:  true,
			errField: "safesearch",
		},
		{
			name:     "safesearch large positive",
			args:     &SearchArgs{Query: "test", SafeSearch: 999},
			wantErr:  true,
			errField: "safesearch",
		},
		// NOTE: SafeSearch is int type, so floats and strings won't compile
		// The MCP layer would catch these before they reach validation

		// --- TimeRange: invalid values ---
		{
			name:     "time_range invalid value",
			args:     &SearchArgs{Query: "test", TimeRange: "hour"},
			wantErr:  true,
			errField: "time_range",
		},
		{
			name:     "time_range week",
			args:     &SearchArgs{Query: "test", TimeRange: "week"},
			wantErr:  true,
			errField: "time_range",
		},
		{
			name:    "time_range empty is valid",
			args:    &SearchArgs{Query: "test", TimeRange: ""},
			wantErr: false,
		},
		{
			name:     "time_range all",
			args:     &SearchArgs{Query: "test", TimeRange: "all"},
			wantErr:  true,
			errField: "time_range",
		},
		{
			name:     "time_range numbers",
			args:     &SearchArgs{Query: "test", TimeRange: "123"},
			wantErr:  true,
			errField: "time_range",
		},

		// --- Categories: invalid/edge cases ---
		{
			name:    "categories empty is valid",
			args:    &SearchArgs{Query: "test", Categories: ""},
			wantErr: false,
		},
		{
			name:     "categories whitespace only",
			args:     &SearchArgs{Query: "test", Categories: "   "},
			wantErr:  true,
			errField: "categories",
		},
		{
			name:    "categories arbitrary name is valid",
			args:    &SearchArgs{Query: "test", Categories: "nonexistent_category"},
			wantErr: false,
		},
		{
			name:     "categories special chars",
			args:     &SearchArgs{Query: "test", Categories: "general!@#"},
			wantErr:  true,
			errField: "categories",
		},

		// --- Engines: invalid/edge cases ---
		{
			name:    "engines empty is valid",
			args:    &SearchArgs{Query: "test", Engines: ""},
			wantErr: false,
		},
		{
			name:     "engines whitespace only",
			args:     &SearchArgs{Query: "test", Engines: "   "},
			wantErr:  true,
			errField: "engines",
		},
		{
			name:    "engines arbitrary name is valid",
			args:    &SearchArgs{Query: "test", Engines: "nonexistent_engine"},
			wantErr: false,
		},
		{
			name:     "engines special chars",
			args:     &SearchArgs{Query: "test", Engines: "google!@#"},
			wantErr:  true,
			errField: "engines",
		},

		// --- Pageno: edge cases ---
		{
			name:     "pageno zero",
			args:     &SearchArgs{Query: "test", Pageno: intPtr(0)},
			wantErr:  true,
			errField: "pageno",
		},
		{
			name:     "pageno negative one",
			args:     &SearchArgs{Query: "test", Pageno: intPtr(-1)},
			wantErr:  true,
			errField: "pageno",
		},
		{
			name:     "pageno negative large",
			args:     &SearchArgs{Query: "test", Pageno: intPtr(-999)},
			wantErr:  true,
			errField: "pageno",
		},
		{
			name:    "pageno valid one",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(1)},
			wantErr: false,
		},
		{
			name:    "pageno valid large",
			args:    &SearchArgs{Query: "test", Pageno: intPtr(1000000)},
			wantErr: false, // Currently allowed
		},
		{
			name:    "pageno nil is valid",
			args:    &SearchArgs{Query: "test", Pageno: nil},
			wantErr: false,
		},
		// NOTE: Pageno is *int type, so floats and strings won't compile
		// The MCP layer would catch these before they reach validation
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSearchArgs(tt.args)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ValidateSearchArgs() expected error, got nil")
					return
				}
				ve, ok := err.(*ValidationError)
				if !ok {
					t.Errorf("ValidateSearchArgs() expected ValidationError, got %T", err)
					return
				}
				if ve.Field != tt.errField {
					t.Errorf("ValidateSearchArgs() error field = %q, want %q", ve.Field, tt.errField)
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("ValidateSearchArgs() error message = %q, want to contain %q", err.Error(), tt.errContains)
				}
			} else {
				if err != nil {
					t.Errorf("ValidateSearchArgs() unexpected error: %v", err)
				}
			}
		})
	}
}
