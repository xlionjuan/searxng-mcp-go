package searxng

import (
	"net/url"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Request-body helpers
// ---------------------------------------------------------------------------

// captureRequestBody calls buildSearchRequest directly and returns the encoded
// POST body. It exercises only the request-builder seam.
func captureRequestBody(t *testing.T, searcher *SearXNGSearcher, args *SearchArgs) string {
	t.Helper()

	_, bodyStr, err := searcher.buildSearchRequest(t.Context(), args)
	if err != nil {
		t.Fatalf("captureRequestBody: buildSearchRequest() error = %v", err)
	}

	return bodyStr
}

// validateSearchArgs is a thin wrapper around ValidateSearchArgs that fails the
// test when the args are invalid.
func validateSearchArgs(t *testing.T, args *SearchArgs) {
	t.Helper()

	_, err := ValidateSearchArgs(args)
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

		body := captureRequestBody(t, searcher, args)

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

		body := captureRequestBody(t, searcher, args)

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

		body := captureRequestBody(t, searcher, args)

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

		body := captureRequestBody(t, searcher, args)

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

	body := captureRequestBody(t, searcher, args)

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
