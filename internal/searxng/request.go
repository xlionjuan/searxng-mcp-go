package searxng

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

var errSearchEndpointNotPrecomputed = errors.New("search endpoint not precomputed")

// computeSearchEndpoint parses the baseURL and normalizes its path so the
// returned URL points at the SearXNG /search endpoint. The result has no
// query string and is intended to be cloned per request rather than re-parsed.
// This is called once at searcher construction time.
func computeSearchEndpoint(baseURL string) (*url.URL, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	parsed.RawQuery = ""
	trimmedPath := strings.TrimRight(parsed.Path, "/")

	lastSegment := trimmedPath
	if idx := strings.LastIndex(trimmedPath, "/"); idx >= 0 {
		lastSegment = trimmedPath[idx+1:]
	}

	if lastSegment != "search" {
		if trimmedPath == "" {
			parsed.Path = "/search"
		} else {
			parsed.Path = trimmedPath + "/search"
		}
	} else {
		parsed.Path = trimmedPath
	}

	return parsed, nil
}

// buildSearchRequest constructs an HTTP request for searching SearXNG.
// It clones the precomputed search endpoint URL and only sets the per-request
// body; the endpoint path is never re-derived here.
func (s *SearXNGSearcher) buildSearchRequest(ctx context.Context, args *SearchArgs) (*http.Request, string, error) {
	if s.searchEndpoint == nil {
		return nil, "", NewSearXNGError(0, "", "", errSearchEndpointNotPrecomputed)
	}

	params := url.Values{}
	params.Set("q", args.Query)
	params.Set("format", "json")

	if args.Language != "" {
		params.Set("language", args.Language)
	}

	safesearch := args.SafeSearch
	params.Set("safesearch", strconv.Itoa(safesearch))

	if args.TimeRange != "" {
		params.Set("time_range", args.TimeRange)
	}

	if args.Categories != "" {
		params.Set("categories", args.Categories)
	}

	if args.Engines != "" {
		params.Set("engines", args.Engines)
	}

	if args.Pageno != nil {
		params.Set("pageno", strconv.Itoa(*args.Pageno))
	}

	// Clone the immutable endpoint; this is a shallow copy of the *url.URL
	// struct so per-request mutation of RawQuery (via the opt-in GET fallback)
	// does not leak back into the precomputed value.
	searchURL := *s.searchEndpoint

	postBodyStr := params.Encode()

	postReq, err := http.NewRequestWithContext(ctx, http.MethodPost, searchURL.String(), strings.NewReader(postBodyStr))
	if err != nil {
		return nil, "", NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errRequestCreateFailed, err))
	}

	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setBrowserHeaders(postReq)

	return postReq, postBodyStr, nil
}

// setBrowserHeaders sets browser-like HTTP headers to bypass SearXNG bot detection.
func setBrowserHeaders(req *http.Request) {
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/147.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,"+
		"image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Sec-Ch-Ua", `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Linux"`)
	req.Header.Set("Priority", "u=0, i")
}
