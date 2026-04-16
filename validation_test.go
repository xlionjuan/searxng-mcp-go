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
			errContains: "search query is required",
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
