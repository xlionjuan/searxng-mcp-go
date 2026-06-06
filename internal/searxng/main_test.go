package searxng //nolint:testpackage // TestMain must be in internal package to cover all test goroutines

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
