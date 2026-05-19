package searxng

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

var errInvalidSearXNGURL = errors.New("invalid SearXNG URL")

// buildSearchRequest constructs an HTTP request for searching SearXNG.
func (s *SearXNGSearcher) buildSearchRequest(ctx context.Context, args *SearchArgs) (*http.Request, string, error) {
	baseURL, err := url.Parse(s.baseURL)
	if err != nil {
		return nil, "", NewSearXNGError(0, "", "", fmt.Errorf("%w: %w", errInvalidSearXNGURL, err))
	}

	params := url.Values{}
	params.Set("q", args.Query)
	params.Set("format", "json")

	if args.Language != "" {
		params.Set("language", args.Language)
	}

	safesearch := args.SafeSearch
	params.Set("safesearch", fmt.Sprintf("%d", safesearch))

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
		params.Set("pageno", fmt.Sprintf("%d", *args.Pageno))
	}

	searchURL := *baseURL
	searchURL.RawQuery = ""
	trimmedPath := strings.TrimRight(searchURL.Path, "/")
	lastSegment := trimmedPath
	if idx := strings.LastIndex(trimmedPath, "/"); idx >= 0 {
		lastSegment = trimmedPath[idx+1:]
	}
	if lastSegment != "search" {
		if trimmedPath == "" {
			searchURL.Path = "/search"
		} else {
			searchURL.Path = trimmedPath + "/search"
		}
	} else {
		searchURL.Path = trimmedPath
	}

	postBodyStr := params.Encode()
	postReq, err := http.NewRequestWithContext(ctx, "POST", searchURL.String(), strings.NewReader(postBodyStr))
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
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
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
