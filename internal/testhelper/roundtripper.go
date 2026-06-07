// Package testhelper provides shared test utilities for searxng-mcp-go tests.
package testhelper

import "net/http"

// RoundTripperFunc adapts a function to the http.RoundTripper interface.
type RoundTripperFunc func(*http.Request) (*http.Response, error)

// RoundTrip implements the http.RoundTripper interface.
func (f RoundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
