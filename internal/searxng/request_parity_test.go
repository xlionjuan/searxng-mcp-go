package searxng //nolint:testpackage // white-box tests need access to unexported types/functions

import (
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// captureCLIBody calls buildSearchRequest directly, which is the same path
// the CLI-mode search uses to construct the POST body. The args are used
// as-is, with no defaulting.
func captureCLIBody(t *testing.T, searcher *SearXNGSearcher, args *SearchArgs) string {
	t.Helper()

	_, bodyStr, err := searcher.buildSearchRequest(t.Context(), args)
	if err != nil {
		t.Fatalf("captureCLIBody: buildSearchRequest() error = %v", err)
	}

	return bodyStr
}

// captureMCPBody simulates the preprocessing that the MCP handler performs
// (defaulting Limit to DefaultResultLimit when nil) and then calls
// buildSearchRequest to capture the wire body.
//
// Note: Limit is client-side only (truncation in normalizeResponse) and is
// never serialized into the wire body. The Limit defaulting here is applied
// for completeness but does not affect the returned body string.
func captureMCPBody(t *testing.T, searcher *SearXNGSearcher, args *SearchArgs) string {
	t.Helper()

	// Shallow copy so we don't mutate the caller's args.
	processed := *args

	// MCP handler defaults Limit when nil (see NewSearchToolHandler in mcp.go).
	if processed.Limit == nil {
		defaultLimit := DefaultResultLimit
		processed.Limit = &defaultLimit
	}

	_, bodyStr, err := searcher.buildSearchRequest(t.Context(), &processed)
	if err != nil {
		t.Fatalf("captureMCPBody: buildSearchRequest() error = %v", err)
	}

	return bodyStr
}

// validateSearchArgs is a thin wrapper around ValidateSearchArgs that fails the
// test when the args are invalid.
func validateSearchArgs(t *testing.T, args *SearchArgs) {
	t.Helper()

	err := ValidateSearchArgs(args)
	if err != nil {
		t.Fatalf("ValidateSearchArgs() error = %v", err)
	}
}

// bodyHasKey reports whether the URL-encoded form body contains the given key.
func bodyHasKey(body, key string) bool {
	vals, err := url.ParseQuery(body)
	if err != nil {
		return false
	}

	return vals.Has(key)
}

// ---------------------------------------------------------------------------
// #260 — wire-body parity drift test between CLI and MCP paths
// ---------------------------------------------------------------------------

// TestRequestBodyParity verifies that the CLI and MCP code paths produce
// byte-for-byte identical HTTP request bodies for the same logical search
// parameters.
//
// The two paths converge on buildSearchRequest, but differ in how they
// preprocess args before calling it:
//   - CLI path uses the args as provided by the user (no Limit defaulting).
//   - MCP path defaults Limit to DefaultResultLimit when the caller omits it.
//
// Since Limit is client-side only (truncation in normalizeResponse) and is
// never serialized into the wire body, this difference does NOT affect the
// returned body. Both paths should always produce byte-for-byte identical
// bodies when starting from the same logical SearchArgs.
func TestRequestBodyParity(t *testing.T) {
	t.Parallel()

	pageno := 2
	limit := 10
	safesearch := 1

	tests := []struct {
		name string
		args SearchArgs
	}{
		{
			name: "bare query",
			args: SearchArgs{
				Query:      "hello world",
				SafeSearch: safesearch,
			},
		},
		{
			name: "query with language",
			args: SearchArgs{
				Query:      "golang testing",
				Language:   "en",
				SafeSearch: safesearch,
			},
		},
		{
			name: "language auto (normalized to empty)",
			args: SearchArgs{
				Query:      "news today",
				Language:   "auto",
				SafeSearch: safesearch,
			},
		},
		{
			name: "with safesearch 2",
			args: SearchArgs{
				Query:      "safe search example",
				SafeSearch: 2,
			},
		},
		{
			name: "with time_range",
			args: SearchArgs{
				Query:      "monthly report",
				TimeRange:  "month",
				SafeSearch: safesearch,
			},
		},
		{
			name: "with pageno",
			args: SearchArgs{
				Query:      "paginated results",
				SafeSearch: safesearch,
				Pageno:     &pageno,
			},
		},
		{
			name: "with categories",
			args: SearchArgs{
				Query:      "science news",
				Categories: "science",
				SafeSearch: safesearch,
			},
		},
		{
			name: "with engines",
			args: SearchArgs{
				Query:      "custom engine search",
				Engines:    "google,bing,duckduckgo",
				SafeSearch: safesearch,
			},
		},
		{
			name: "all optional parameters",
			args: SearchArgs{
				Query:      "comprehensive search",
				Language:   "zh-tw",
				SafeSearch: 2,
				TimeRange:  "year",
				Categories: "general,news",
				Engines:    "google,bing",
				Pageno:     &pageno,
			},
		},
		{
			name: "with explicit limit (client-side only, not in body)",
			args: SearchArgs{
				Query:      "limited results",
				SafeSearch: safesearch,
				Limit:      &limit,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			searcher := newRequestTestSearcher(t, "https://search.example.com")

			// Validate the args (both paths validate before building the request).
			validateSearchArgs(t, &tt.args)

			cliBody := captureCLIBody(t, searcher, &tt.args)

			// captureMCPBody handles Limit defaulting internally.
			mcpBody := captureMCPBody(t, searcher, &tt.args)

			if cliBody != mcpBody {
				t.Fatalf("CLI body != MCP body\nCLI: %q\nMCP: %q", cliBody, mcpBody)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// #261 — lock 'bare query body has no limit/pageno key' contract
// ---------------------------------------------------------------------------

// TestBareQueryBodyOmitsLimitAndPagenoKeys locks the contract that:
//   - When Pageno is nil in SearchArgs, the resulting wire body does not
//     contain a pageno= key.
//   - When Pageno is set, the wire body contains pageno= with the correct
//     value.
//   - Limit is never present in the request body regardless of its value;
//     it is client-side only (applied in normalizeResponse after receiving
//     the server response).
func TestBareQueryBodyOmitsLimitAndPagenoKeys(t *testing.T) {
	t.Parallel()

	t.Run("nil limit and nil pageno", func(t *testing.T) {
		t.Parallel()

		searcher := newRequestTestSearcher(t, "https://search.example.com")

		args := &SearchArgs{
			Query: "bare query",
		}
		validateSearchArgs(t, args)

		body := captureCLIBody(t, searcher, args)

		// Pageno is nil — must not appear in the wire body.
		if bodyHasKey(body, "pageno") {
			t.Errorf("body contains unexpected 'pageno' key when Pageno is nil\nbody: %q", body)
		}

		// Limit is never sent to SearXNG; it is client-side only.
		// Verify it is absent regardless of the struct value.
		if bodyHasKey(body, "limit") {
			t.Errorf("body contains unexpected 'limit' key (Limit is client-side only)\nbody: %q", body)
		}
	})

	t.Run("nil limit with explicit pageno", func(t *testing.T) {
		t.Parallel()

		searcher := newRequestTestSearcher(t, "https://search.example.com")

		pageno := 3
		args := &SearchArgs{
			Query:  "page navigation",
			Pageno: &pageno,
		}
		validateSearchArgs(t, args)

		body := captureCLIBody(t, searcher, args)

		// Pageno is set — must appear in the wire body.
		if !bodyHasKey(body, "pageno") {
			t.Errorf("body should contain 'pageno' key when Pageno is set\nbody: %q", body)
		}

		if !strings.Contains(body, "pageno=3") {
			t.Errorf("body = %q, want to contain 'pageno=3'", body)
		}

		// Limit is client-side only, never in the wire body.
		if bodyHasKey(body, "limit") {
			t.Errorf("body contains unexpected 'limit' key (Limit is client-side only)\nbody: %q", body)
		}
	})

	t.Run("nil pageno with explicit limit", func(t *testing.T) {
		t.Parallel()

		searcher := newRequestTestSearcher(t, "https://search.example.com")

		limit := 10
		args := &SearchArgs{
			Query: "limited results",
			Limit: &limit,
		}
		validateSearchArgs(t, args)

		body := captureCLIBody(t, searcher, args)

		// Pageno is nil — must not appear in the wire body.
		if bodyHasKey(body, "pageno") {
			t.Errorf("body contains unexpected 'pageno' key when Pageno is nil\nbody: %q", body)
		}

		// Limit is client-side only. Even when explicitly set, it never
		// appears in the wire body — it is consumed by normalizeResponse
		// after the HTTP round-trip completes.
		if bodyHasKey(body, "limit") {
			t.Errorf("body contains unexpected 'limit' key (Limit is client-side only)\nbody: %q", body)
		}
	})

	t.Run("both limit and pageno set explicitly", func(t *testing.T) {
		t.Parallel()

		searcher := newRequestTestSearcher(t, "https://search.example.com")

		pageno := 1
		limit := 5
		args := &SearchArgs{
			Query:  "both set",
			Pageno: &pageno,
			Limit:  &limit,
		}
		validateSearchArgs(t, args)

		body := captureCLIBody(t, searcher, args)

		// Pageno is set — must appear in the wire body.
		if !bodyHasKey(body, "pageno") {
			t.Errorf("body should contain 'pageno' key when Pageno is set\nbody: %q", body)
		}

		if !strings.Contains(body, "pageno=1") {
			t.Errorf("body = %q, want to contain 'pageno=1'", body)
		}

		// Limit is client-side only, never in the wire body.
		if bodyHasKey(body, "limit") {
			t.Errorf("body contains unexpected 'limit' key (Limit is client-side only)\nbody: %q", body)
		}
	})
}

// TestAlwaysPresentKeysInBareQueryBody verifies that certain keys (q, format,
// safesearch) are always present even in the most minimal query body.
func TestAlwaysPresentKeysInBareQueryBody(t *testing.T) {
	t.Parallel()

	searcher := newRequestTestSearcher(t, "https://search.example.com")

	args := &SearchArgs{
		Query: "minimal",
	}
	validateSearchArgs(t, args)

	body := captureCLIBody(t, searcher, args)

	if !bodyHasKey(body, "q") {
		t.Errorf("body missing 'q' key\nbody: %q", body)
	}

	if !bodyHasKey(body, "format") {
		t.Errorf("body missing 'format' key\nbody: %q", body)
	}

	if !bodyHasKey(body, "safesearch") {
		t.Errorf("body missing 'safesearch' key\nbody: %q", body)
	}

	if !strings.Contains(body, "format=json") {
		t.Errorf("body = %q, want to contain 'format=json'", body)
	}
}

// TestMCPBodyIsUnchangedByLimitDefaulting uses the MCP capture helper to verify
// that the MCP path applies its Limit defaulting without affecting the wire
// body (since Limit is client-side only). This test documents the contract:
// the MCP body must match the CLI body for the same logical search,
// even when the MCP handler defaults Limit to DefaultResultLimit.
func TestMCPBodyIsUnchangedByLimitDefaulting(t *testing.T) {
	t.Parallel()

	searcher := newRequestTestSearcher(t, "https://search.example.com")

	// Same input for both paths.
	args := &SearchArgs{
		Query: "test limit defaulting",
	}
	validateSearchArgs(t, args)

	// CLI path: Limit stays nil.
	cliBody := captureCLIBody(t, searcher, args)

	// MCP path: Limit defaults to DefaultResultLimit.
	mcpBody := captureMCPBody(t, searcher, args)

	// Both bodies must be identical because Limit is not serialized.
	if cliBody != mcpBody {
		t.Fatalf("CLI body != MCP body (Limit defaulting should not affect wire body)\nCLI: %q\nMCP: %q", cliBody, mcpBody)
	}

	// Verify that limit= never appears in either body.
	if bodyHasKey(cliBody, "limit") {
		t.Errorf("CLI body contains 'limit' key (client-side only)\nbody: %q", cliBody)
	}

	if bodyHasKey(mcpBody, "limit") {
		t.Errorf("MCP body contains 'limit' key (client-side only)\nbody: %q", mcpBody)
	}
}
