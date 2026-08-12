package testhelper

import (
	"encoding/json"
	"os"
	"testing"
)

// ReadFixture reads a test fixture file and fails the test on error.
func ReadFixture(tb testing.TB, path string) []byte {
	tb.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // test helper; path comes from callers, not user input
	if err != nil {
		tb.Fatal(err)
	}

	return data
}

// LoadJSONFixture reads a JSON fixture and unmarshals it into dst.
// dst must be a pointer to the target type (e.g. &searxng.SearchResponse{}).
func LoadJSONFixture(tb testing.TB, path string, dst any) {
	tb.Helper()

	data := ReadFixture(tb, path)

	err := json.Unmarshal(data, dst)
	if err != nil {
		tb.Fatal(err)
	}
}

// RequireSearchResponseFixture checks the result count and number_of_results
// field presence that define a search-response benchmark fixture's workload.
func RequireSearchResponseFixture(
	tb testing.TB,
	path string,
	wantResultCount int,
	wantNumberOfResults bool,
) {
	tb.Helper()

	var fields map[string]json.RawMessage

	data := ReadFixture(tb, path)

	err := json.Unmarshal(data, &fields)
	if err != nil {
		tb.Fatal(err)
	}

	resultsJSON, ok := fields["results"]
	if !ok {
		tb.Fatalf("fixture %q has no results field", path)
	}

	var results []json.RawMessage

	err = json.Unmarshal(resultsJSON, &results)
	if err != nil {
		tb.Fatal(err)
	}

	if got := len(results); got != wantResultCount {
		tb.Fatalf("fixture %q has %d results, want %d", path, got, wantResultCount)
	}

	_, gotNumberOfResults := fields["number_of_results"]
	if gotNumberOfResults != wantNumberOfResults {
		tb.Fatalf("fixture %q number_of_results present = %t, want %t", path, gotNumberOfResults, wantNumberOfResults)
	}
}
