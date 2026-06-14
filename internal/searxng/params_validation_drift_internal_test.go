package searxng

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// findParam returns the ParamDef with the given Name from SearchParams, or
// fails the test if it cannot be found.
func findParam(t *testing.T, name string) ParamDef {
	t.Helper()

	for _, p := range SearchParams {
		if p.Name == name {
			return p
		}
	}

	t.Fatalf("SearchParams does not contain %q", name)

	return ParamDef{}
}

// TestSearchParamsLimitDefaultMirrorsDefaultResultLimit is the drift guard
// for issue #24: the limit ParamDef's Default string must equal
// strconv.Itoa(DefaultResultLimit). If this fails, the JSON Schema, CLI
// flag default, and runtime defaulting have desynchronized from
// DefaultResultLimit.
func TestSearchParamsLimitDefaultMirrorsDefaultResultLimit(t *testing.T) {
	t.Parallel()

	p := findParam(t, "limit")

	if got, want := p.Default, strconv.Itoa(DefaultResultLimit); got != want {
		t.Errorf("limit Default = %q, want %q (DefaultResultLimit)", got, want)
	}
}

// TestSearchParamsSafeSearchBounds checks that the SafeSearch ParamDef
// schema agrees with the shared bounds (MinSafeSearch, MaxSafeSearch) and
// with what validateSafesearch enforces at runtime.
func TestSearchParamsSafeSearchBounds(t *testing.T) {
	t.Parallel()

	p := findParam(t, "safesearch")

	if p.Default != strconv.Itoa(MinSafeSearch) {
		t.Errorf("safesearch Default = %q, want %d (MinSafeSearch)", p.Default, MinSafeSearch)
	}

	if p.Minimum == nil || *p.Minimum != MinSafeSearch {
		got := "<nil>"
		if p.Minimum != nil {
			got = strconv.Itoa(*p.Minimum)
		}

		t.Errorf("safesearch Minimum = %s, want %d (MinSafeSearch)", got, MinSafeSearch)
	}

	if p.Maximum == nil || *p.Maximum != MaxSafeSearch {
		got := "<nil>"
		if p.Maximum != nil {
			got = strconv.Itoa(*p.Maximum)
		}

		t.Errorf("safesearch Maximum = %s, want %d (MaxSafeSearch)", got, MaxSafeSearch)
	}

	// Runtime validator agreement: validateSafesearch must reject values
	// outside [MinSafeSearch, MaxSafeSearch] and accept values within.
	verifySafeSearchRejects(t, MinSafeSearch-1)
	verifySafeSearchRejects(t, MaxSafeSearch+1)
	verifySafeSearchAccepts(t, MinSafeSearch)
	verifySafeSearchAccepts(t, MaxSafeSearch)
}

// TestSearchParamsPagenoBounds checks that the pageno ParamDef schema
// agrees with the shared bounds (MinPageno) and that validatePagination
// enforces the same minimum.
func TestSearchParamsPagenoBounds(t *testing.T) {
	t.Parallel()

	p := findParam(t, "pageno")

	if p.Default != strconv.Itoa(MinPageno) {
		t.Errorf("pageno Default = %q, want %d (MinPageno)", p.Default, MinPageno)
	}

	if p.Minimum == nil || *p.Minimum != MinPageno {
		got := "<nil>"
		if p.Minimum != nil {
			got = strconv.Itoa(*p.Minimum)
		}

		t.Errorf("pageno Minimum = %s, want %d (MinPageno)", got, MinPageno)
	}

	// validatePagination must reject values below MinPageno and accept
	// values at the boundary.
	verifyPagenoRejects(t, MinPageno-1)
	verifyPagenoAccepts(t, MinPageno)
}

// TestSearchParamsLimitBounds checks that the limit ParamDef schema
// agrees with the shared bounds (MinResultLimit, MaxResultLimit) and that
// validatePagination enforces the same range.
func TestSearchParamsLimitBounds(t *testing.T) {
	t.Parallel()

	p := findParam(t, "limit")

	if p.Minimum == nil || *p.Minimum != MinResultLimit {
		got := "<nil>"
		if p.Minimum != nil {
			got = strconv.Itoa(*p.Minimum)
		}

		t.Errorf("limit Minimum = %s, want %d (MinResultLimit)", got, MinResultLimit)
	}

	if p.Maximum == nil || *p.Maximum != MaxResultLimit {
		got := "<nil>"
		if p.Maximum != nil {
			got = strconv.Itoa(*p.Maximum)
		}

		t.Errorf("limit Maximum = %s, want %d (MaxResultLimit)", got, MaxResultLimit)
	}

	// Runtime validator agreement.
	verifyLimitRejects(t, MinResultLimit-1)
	verifyLimitRejects(t, MaxResultLimit+1)
	verifyLimitAccepts(t, MinResultLimit)
	verifyLimitAccepts(t, MaxResultLimit)
}

// TestSearchParamsTimeRangeEnum is the drift guard for the time_range
// ParamDef: its Enum must equal ["", "day", "month", "year"], which is
// ValidTimeRanges() with the empty "no restriction" sentinel prepended.
// This keeps the JSON Schema enum in sync with the runtime validator.
func TestSearchParamsTimeRangeEnum(t *testing.T) {
	t.Parallel()

	p := findParam(t, "time_range")

	want := append([]string{""}, ValidTimeRanges()...)

	if len(p.Enum) != len(want) {
		t.Fatalf("time_range Enum length = %d, want %d (got=%v, want=%v)",
			len(p.Enum), len(want), p.Enum, want)
	}

	for i, v := range want {
		if p.Enum[i] != v {
			t.Errorf("time_range Enum[%d] = %q, want %q (full got=%v, want=%v)",
				i, p.Enum[i], v, p.Enum, want)
		}
	}

	// Runtime validator agreement: validateTimeRange must accept every
	// non-empty value in ValidTimeRanges() and reject a synthetic value.
	for _, v := range ValidTimeRanges() {
		verifyTimeRangeAccepts(t, v)
	}

	verifyTimeRangeRejects(t, "definitely-not-a-range")

	// Empty string is "no restriction" and must be accepted without
	// consulting the map.
	verifyTimeRangeAccepts(t, "")
}

// TestValidTimeRangesReturnsDefensiveCopy guards the public accessor shape:
// callers must not be able to mutate the canonical time_range list used by
// schema generation and validation.
func TestValidTimeRangesReturnsDefensiveCopy(t *testing.T) {
	t.Parallel()

	ranges := ValidTimeRanges()
	if len(ranges) == 0 {
		t.Fatal("ValidTimeRanges() returned an empty list")
	}

	original := ValidTimeRanges()
	ranges[0] = "mutated"

	afterMutation := ValidTimeRanges()
	if afterMutation[0] != original[0] {
		t.Fatalf("ValidTimeRanges() returned shared state: got first value %q, want %q", afterMutation[0], original[0])
	}
}

// TestValidateTimeRangeErrorMessageUsesValidTimeRanges guards the error
// message in validateTimeRange so it does not silently drift from the
// shared ValidTimeRanges() list. The message is generated from
// strings.Join(ValidTimeRanges(), ", "), so an extra or missing range entry
// shows up here.
func TestValidateTimeRangeErrorMessageUsesValidTimeRanges(t *testing.T) {
	t.Parallel()

	err := validateTimeRange("nope")
	if err == nil {
		t.Fatal("validateTimeRange(\"nope\") = nil, want error")
	}

	wantSubstr := strings.Join(ValidTimeRanges(), ", ")
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q (ValidTimeRanges join)", err.Error(), wantSubstr)
	}
}

// TestValidatePaginationErrorMessageUsesBounds guards the limit error
// message in validatePagination so it does not silently drift from the
// shared MinResultLimit/MaxResultLimit bounds.
func TestValidatePaginationErrorMessageUsesBounds(t *testing.T) {
	t.Parallel()

	over := MaxResultLimit + 1

	err := validatePagination(nil, &over)
	if err == nil {
		t.Fatal("validatePagination(limit=MaxResultLimit+1) = nil, want error")
	}

	wantSubstr := "between " + strconv.Itoa(MinResultLimit) + " and " + strconv.Itoa(MaxResultLimit)
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

// TestValidateSafesearchErrorMessageUsesBounds guards the safesearch
// error message so it does not silently drift from the shared
// MinSafeSearch/MaxSafeSearch bounds.
func TestValidateSafesearchErrorMessageUsesBounds(t *testing.T) {
	t.Parallel()

	err := validateSafesearch(MaxSafeSearch + 1)
	if err == nil {
		t.Fatal("validateSafesearch(MaxSafeSearch+1) = nil, want error")
	}

	wantSubstr := strconv.Itoa(MinSafeSearch) + " off, " +
		strconv.Itoa(MinSafeSearch+1) + " moderate, or " +
		strconv.Itoa(MaxSafeSearch) + " strict"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Errorf("error %q does not contain %q", err.Error(), wantSubstr)
	}
}

// --- validator agreement helpers ---
// The helpers below use plain assignment instead of inline if-init so the
// noinlineerr linter is satisfied while keeping the table of expectations
// readable.

func verifySafeSearchRejects(t *testing.T, value int) {
	t.Helper()

	err := validateSafesearch(value)
	if err == nil {
		t.Errorf("validateSafesearch(%d) = nil, want error", value)
	}
}

func verifySafeSearchAccepts(t *testing.T, value int) {
	t.Helper()

	err := validateSafesearch(value)
	if err != nil {
		t.Errorf("validateSafesearch(%d) = %v, want nil", value, err)
	}
}

func verifyPagenoRejects(t *testing.T, value int) {
	t.Helper()

	pageno := value

	err := validatePagination(&pageno, nil)
	if err == nil {
		t.Errorf("validatePagination(pageno=%d) = nil, want error", value)
	}
}

func verifyPagenoAccepts(t *testing.T, value int) {
	t.Helper()

	pageno := value

	err := validatePagination(&pageno, nil)
	if err != nil {
		t.Errorf("validatePagination(pageno=%d) = %v, want nil", value, err)
	}
}

func verifyLimitRejects(t *testing.T, value int) {
	t.Helper()

	limit := value

	err := validatePagination(nil, &limit)
	if err == nil {
		t.Errorf("validatePagination(limit=%d) = nil, want error", value)
	}
}

func verifyLimitAccepts(t *testing.T, value int) {
	t.Helper()

	limit := value

	err := validatePagination(nil, &limit)
	if err != nil {
		t.Errorf("validatePagination(limit=%d) = %v, want nil", value, err)
	}
}

func verifyTimeRangeRejects(t *testing.T, value string) {
	t.Helper()

	err := validateTimeRange(value)
	if err == nil {
		t.Errorf("validateTimeRange(%q) = nil, want error", value)
	}
}

func verifyTimeRangeAccepts(t *testing.T, value string) {
	t.Helper()

	err := validateTimeRange(value)
	if err != nil {
		t.Errorf("validateTimeRange(%q) = %v, want nil", value, err)
	}
}

// TestSearchArgsFieldsMatchSearchParams is the drift guard that ensures every
// ParamDef in SearchParams has a matching json-tagged field in SearchArgs,
// and vice versa. If a new parameter is added to SearchParams without adding
// a corresponding field to SearchArgs (or if a SearchArgs field is renamed
// without updating SearchParams), this test fails.
func TestSearchArgsFieldsMatchSearchParams(t *testing.T) {
	t.Parallel()

	// Build set of json tag names from SearchArgs struct fields.
	argsType := reflect.TypeFor[SearchArgs]()
	argsFieldTags := make(map[string]bool)

	for field := range argsType.Fields() {
		tag := field.Tag.Get("json")
		if tag == "" {
			continue
		}

		// Strip omitempty etc. — take only the name before comma.
		if idx := strings.Index(tag, ","); idx >= 0 {
			tag = tag[:idx]
		}

		argsFieldTags[tag] = true
	}

	// Build set of param names from SearchParams.
	paramNames := make(map[string]bool)

	for _, p := range SearchParams {
		paramNames[p.Name] = true
	}

	// Every SearchParams name must have a matching SearchArgs json tag.
	for _, p := range SearchParams {
		if !argsFieldTags[p.Name] {
			t.Errorf("SearchParams has %q but SearchArgs has no json tag %q", p.Name, p.Name)
		}
	}

	// Every SearchArgs json tag must have a matching SearchParams entry.
	for tag := range argsFieldTags {
		if !paramNames[tag] {
			t.Errorf("SearchArgs has json tag %q but SearchParams has no entry for it", tag)
		}
	}
}

// TestNewSearchArgsDefaultLimit verifies that NewSearchArgs sets Limit to
// DefaultResultLimit. This guards against accidental drift if the defaulting
// policy moves out of the constructor into callers (which would re-introduce
// the duplication that NewSearchArgs was designed to eliminate).
func TestNewSearchArgsDefaultLimit(t *testing.T) {
	t.Parallel()

	args := NewSearchArgs("test")

	if args.Limit == nil {
		t.Fatal("NewSearchArgs().Limit = nil, want non-nil")
	}

	if *args.Limit != DefaultResultLimit {
		t.Fatalf("NewSearchArgs().Limit = %d, want %d", *args.Limit, DefaultResultLimit)
	}

	if args.Pageno != nil {
		t.Fatal("NewSearchArgs().Pageno = non-nil, want nil")
	}
}

// TestApplyDefaultsFillsNilLimit verifies that ApplyDefaults fills a nil
// Limit with DefaultResultLimit and is a no-op when Limit is already set.
// This is the MCP path's entry point for defaulting.
func TestApplyDefaultsFillsNilLimit(t *testing.T) {
	t.Parallel()

	t.Run("fills nil limit", func(t *testing.T) {
		t.Parallel()

		args := &SearchArgs{Query: "test"}

		if args.Limit != nil {
			t.Fatal("test setup: expected nil limit")
		}

		args.ApplyDefaults()

		if args.Limit == nil {
			t.Fatal("ApplyDefaults() left Limit = nil, want non-nil")
		}

		if *args.Limit != DefaultResultLimit {
			t.Fatalf("ApplyDefaults() Limit = %d, want %d", *args.Limit, DefaultResultLimit)
		}
	})

	t.Run("no-op when limit already set", func(t *testing.T) {
		t.Parallel()

		limit := 7
		args := &SearchArgs{Query: "test", Limit: &limit}

		args.ApplyDefaults()

		if args.Limit != &limit {
			t.Fatalf("ApplyDefaults() changed Limit from %v to %v", &limit, args.Limit)
		}
	})
}

// TestParamDefJSONSchema locks the translation contract for ParamDef.JSONSchema.
func TestParamDefJSONSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		param    ParamDef
		wantType any
		wantKeys []string
	}{
		{
			name: "query", param: findParam(t, "query"),
			wantType: "string", wantKeys: []string{"type", "description"},
		},
		{
			name: "language", param: findParam(t, "language"),
			wantType: "string", wantKeys: []string{"type", "description", "default"},
		},
		{
			name: "safesearch", param: findParam(t, "safesearch"),
			wantType: "integer", wantKeys: []string{"type", "description", "default", "minimum", "maximum"},
		},
		{
			name: "time_range", param: findParam(t, "time_range"),
			wantType: "string", wantKeys: []string{"type", "description", "enum"},
		},
		{
			name: "categories", param: findParam(t, "categories"),
			wantType: "string", wantKeys: []string{"type", "description", "examples"},
		},
		{
			name: "pageno", param: findParam(t, "pageno"),
			wantType: []string{"null", "integer"}, wantKeys: []string{"type", "description", "default", "minimum"},
		},
		{
			name: "limit", param: findParam(t, "limit"),
			wantType: "integer", wantKeys: []string{"type", "description", "default", "minimum", "maximum"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.param.JSONSchema()
			if got == nil {
				t.Fatal("ParamDef.JSONSchema returned nil")
			}

			if !reflect.DeepEqual(got["type"], tt.wantType) {
				t.Errorf("type = %v (%T), want %v (%T)", got["type"], got["type"], tt.wantType, tt.wantType)
			}

			for _, key := range tt.wantKeys {
				if _, ok := got[key]; !ok {
					t.Errorf("missing key %q", key)
				}
			}
		})
	}
}

// TestJSONSchemaDefaultDerivesFromConstant verifies that every int-typed
// parameter with a DefaultInt carries a JSON Schema "default" that matches
// its source constant. This guards against drift between the schema and the
// canonical default value.
func TestJSONSchemaDefaultDerivesFromConstant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		paramName string
		want      int
	}{
		{name: "limit", paramName: "limit", want: DefaultResultLimit},
		{name: "safesearch", paramName: "safesearch", want: MinSafeSearch},
		{name: "pageno", paramName: "pageno", want: MinPageno},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := findParam(t, tt.paramName)
			schema := p.JSONSchema()

			raw, ok := schema["default"]
			if !ok {
				t.Fatal("JSONSchema() missing \"default\" key")
			}

			got, ok := raw.(int)
			if !ok {
				t.Fatalf("JSONSchema() default = %v (%T), want int", raw, raw)
			}

			if got != tt.want {
				t.Errorf("JSONSchema() default = %d, want %d", got, tt.want)
			}
		})
	}
}

// TestParamDefFlagDefault locks the translation contract for ParamDef.FlagDefault.
func TestParamDefFlagDefault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		param   ParamDef
		wantVal any
		wantErr bool
	}{
		{
			name:    "string query",
			param:   findParam(t, "query"),
			wantVal: "",
			wantErr: false,
		},
		{
			name:    "string language",
			param:   findParam(t, "language"),
			wantVal: "",
			wantErr: false,
		},
		{
			name:    "int safesearch",
			param:   findParam(t, "safesearch"),
			wantVal: MinSafeSearch,
			wantErr: false,
		},
		{
			name:    "int pageno",
			param:   findParam(t, "pageno"),
			wantVal: MinPageno,
			wantErr: false,
		},
		{
			name:    "int limit",
			param:   findParam(t, "limit"),
			wantVal: DefaultResultLimit,
			wantErr: false,
		},
		{
			name: "unknown go type",
			param: ParamDef{
				Name: "bad", GoType: "float64", Default: "1.5",
			},
			wantVal: nil,
			wantErr: true,
		},
		{
			name: "int with unparseable default",
			param: ParamDef{
				Name: "badint", GoType: "int", Default: "not-a-number",
			},
			wantVal: nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := tt.param.FlagDefault()

			if tt.wantErr {
				if err == nil {
					t.Fatal("FlagDefault() = nil, want error")
				}

				return
			}

			if err != nil {
				t.Fatalf("FlagDefault() = %v, %v, want %v, nil", got, err, tt.wantVal)
			}

			if got != tt.wantVal {
				t.Errorf("FlagDefault() = %v (%T), want %v (%T)", got, got, tt.wantVal, tt.wantVal)
			}
		})
	}
}

// TestParamDefCLIHelpLine locks the output format for ParamDef.CLIHelpLine.
func TestParamDefCLIHelpLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		param      ParamDef
		wantPrefix string
		wantSuffix string
	}{
		{
			name:       "query",
			param:      findParam(t, "query"),
			wantPrefix: "  --query string",
			wantSuffix: findParam(t, "query").CLIHelp,
		},
		{
			name:       "safesearch",
			param:      findParam(t, "safesearch"),
			wantPrefix: "  --safesearch 0-2",
			wantSuffix: findParam(t, "safesearch").CLIHelp,
		},
		{
			name:       "pageno",
			param:      findParam(t, "pageno"),
			wantPrefix: "  --pageno N",
			wantSuffix: findParam(t, "pageno").CLIHelp,
		},
		{
			name:       "limit",
			param:      findParam(t, "limit"),
			wantPrefix: "  --limit N",
			wantSuffix: findParam(t, "limit").CLIHelp,
		},
		{
			name:       "categories",
			param:      findParam(t, "categories"),
			wantPrefix: "  --categories CAT",
			wantSuffix: findParam(t, "categories").CLIHelp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.param.CLIHelpLine()

			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("CLIHelpLine() = %q, want prefix %q", got, tt.wantPrefix)
			}

			if !strings.HasSuffix(got, tt.wantSuffix) {
				t.Errorf("CLIHelpLine() = %q, want suffix %q", got, tt.wantSuffix)
			}

			// Verify the padding is at least 1 space and lines up to 18-char column.
			flagExpr := "--" + tt.param.Name + " " + tt.param.CLIType

			if len(got) < len("  "+flagExpr+" ") {
				t.Errorf("CLIHelpLine() length = %d, want at least %d", len(got), len("  "+flagExpr+" "))
			}
		})
	}
}
