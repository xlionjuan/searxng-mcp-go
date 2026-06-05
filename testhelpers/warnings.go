package testhelpers

import (
	"testing"
)

// WarningSummary collects and prints warnings at the end of a test.
type WarningSummary struct {
	t        *testing.T
	warnings []string
	header   string
}

// NewWarningSummary creates a new warning summary collector.
// The header is printed before the warnings (default: "--- WARNING SUMMARY ---").
func NewWarningSummary(t *testing.T, header string) *WarningSummary {
	t.Helper()

	if header == "" {
		header = "--- WARNING SUMMARY ---"
	}

	return &WarningSummary{
		t:        t,
		warnings: make([]string, 0),
		header:   header,
	}
}

// Add adds a warning message to the summary.
func (ws *WarningSummary) Add(warning string) {
	ws.warnings = append(ws.warnings, warning)
}

// Print prints all collected warnings if any exist.
func (ws *WarningSummary) Print() {
	if len(ws.warnings) > 0 {
		ws.t.Log(ws.header)

		for _, warning := range ws.warnings {
			ws.t.Logf("  WARN: %s", warning)
		}
	}
}

// Warnings returns the collected warnings.
func (ws *WarningSummary) Warnings() []string {
	return ws.warnings
}

// Len returns the number of collected warnings.
func (ws *WarningSummary) Len() int {
	return len(ws.warnings)
}
