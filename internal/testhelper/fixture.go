package testhelper

import (
	"encoding/json"
	"os"
	"testing"
)

// ReadFixture reads a test fixture file and fails the test on error.
func ReadFixture(tb testing.TB, path string) []byte {
	tb.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}

	return data
}

// LoadJSONFixture reads a JSON fixture and unmarshals it into dst.
// dst must be a pointer to the target type (e.g. &searxng.SearchResponse{}).
func LoadJSONFixture(tb testing.TB, path string, dst interface{}) {
	tb.Helper()

	data := ReadFixture(tb, path)

	if err := json.Unmarshal(data, dst); err != nil {
		tb.Fatal(err)
	}
}
