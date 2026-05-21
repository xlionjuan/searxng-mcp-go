package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"searxng-mcp-go/internal/searxng"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// searchInputSchema defines the JSON schema for the search tool input.
// Only "query" is required; all other parameters are optional.
const searchInputSchema = `{
	"type": "object",
	"properties": {
		"query": {
			"type": "string",
			"description": "Search query string"
		},
		"language": {
			"type": "string",
			"description": "Language code for results. Common codes: en, zh-tw, zh, ja, fr, de, es, pt, ru, ar. Leave empty for auto-detect (SearXNG decides based on query)"
		},
		"safesearch": {
			"type": "integer",
			"description": "SafeSearch level. 0=Off (no filtering), 1=Moderate (filter moderate explicit content), 2=Strict (filter all explicit content). Defaults to 0",
			"minimum": 0,
			"maximum": 2
		},
		"time_range": {
			"type": "string",
			"description": "Time range filter. Available values: empty (all time), day, month, year. Defaults to empty (all time)",
			"enum": ["", "day", "month", "year"]
		},
		"categories": {
			"type": "string",
			"description": "Comma-separated list of categories to search. Common categories: general, news, images, videos, music, science, files, it, social_media, map. Leave empty for all categories"
		},
		"engines": {
			"type": "string",
			"description": "Comma-separated list of search engines to use (e.g., google, bing, duckduckgo). Leave empty to use SearXNG default engines"
		},
		"pageno": {
			"type": ["null", "integer"],
			"description": "Page number for pagination. Defaults to 1",
			"minimum": 1
		},
		"limit": {
			"type": "integer",
			"description": "Maximum number of results to return (1-20). Defaults to 10",
			"minimum": 1,
			"maximum": 20
		}
	},
	"required": ["query"],
	"additionalProperties": false
}`

type mcpInitializeMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
}

const (
	mcpInitializeMaxBytes    = 1 << 20
	mcpInitializeReadTimeout = 5 * time.Second
)

var errInvalidMCPInitializeMessage = errors.New("stdin does not contain a valid MCP initialize message")

// prepareMCPStdin reads the first line of stdin to verify it contains a valid
// MCP initialize message (JSON-RPC 2.0 with method "initialize"), preventing
// the MCP server from hanging when piped non-MCP input.
func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpInitializeReadTimeout)
	defer cancel()

	type result struct {
		reader io.Reader
		err    error
	}

	resultCh := make(chan result, 1)

	go func() {
		reader := bufio.NewReader(stdin)
		firstLine := make([]byte, 0)

		for {
			fragment, err := reader.ReadSlice('\n')

			firstLine = append(firstLine, fragment...)
			if len(firstLine) > mcpInitializeMaxBytes {
				resultCh <- result{reader: nil, err: errInvalidMCPInitializeMessage}

				return
			}

			if err == nil {
				break
			}

			if err == io.EOF {
				break
			}

			if !errors.Is(err, bufio.ErrBufferFull) {
				resultCh <- result{reader: nil, err: errInvalidMCPInitializeMessage}

				return
			}
		}

		if len(firstLine) > mcpInitializeMaxBytes || !isValidMCPInitializeMessage(firstLine) {
			resultCh <- result{reader: nil, err: errInvalidMCPInitializeMessage}

			return
		}

		resultCh <- result{reader: io.MultiReader(bytes.NewReader(firstLine), reader)}
	}()

	select {
	case <-ctx.Done():
		return nil, errInvalidMCPInitializeMessage
	case res := <-resultCh:
		if res.err != nil {
			return nil, res.err
		}

		return res.reader, nil
	}
}

// isValidMCPInitializeMessage checks whether the given byte slice is a valid
// JSON-RPC 2.0 initialize message.
func isValidMCPInitializeMessage(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return false
	}

	var msg mcpInitializeMessage

	err := json.Unmarshal(line, &msg)
	if err != nil {
		return false
	}

	return msg.JSONRPC == "2.0" && msg.Method == "initialize"
}

// attachStdin replaces os.Stdin with a pipe that reads from the given reader,
// allowing the MCP server to consume pre-processed stdin data. It returns a
// restore function to revert os.Stdin to its original value.
func attachStdin(stdin io.Reader) (func(), error) {
	originalStdin := os.Stdin

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	os.Stdin = pr

	go func() {
		_, copyErr := io.Copy(pw, stdin)
		if copyErr != nil {
			slog.Debug("failed to copy MCP stdin", "error", copyErr)
		}

		_ = pw.Close()
	}()

	return func() {
		os.Stdin = originalStdin
		_ = pr.Close()
	}, nil
}

// runMCPMode starts the MCP stdio server, registers the search tool, and
// blocks until a signal (SIGINT/SIGTERM) is received or the server exits.
func runMCPMode(flags CLIFlags, stdin io.Reader) {
	restoreStdin, err := attachStdin(stdin)
	if err != nil {
		slog.Error("failed to prepare MCP stdin", "error", err)
		os.Exit(1)
	}
	defer restoreStdin()

	cfg, err := getConfig(flags)
	if err != nil {
		slog.Error("configuration error", "error", err)
		//nolint:gocritic // defer restoreStdin handles normal shutdown path
		os.Exit(1)
	}

	searcher, err := searxng.NewSearXNGSearcher(cfg, debugMode)
	if err != nil {
		slog.Error("failed to create searcher", "error", err)
		os.Exit(1)
	}

	defer func() { _ = searcher.Close() }()

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: "Search the web using SearXNG meta-search engine. Returns web results with titles, URLs, summaries, published dates, and engine source information. Parameters: query (required) - search query string; language - language code (e.g., en, zh-tw, ja, fr, de, es), auto-detect if empty; safesearch - 0=Off, 1=Moderate, 2=Strict (default 0); time_range - empty (all time), day, month, year; categories - comma-separated (e.g., general, news, images, videos, music, science, files, it, social_media, map); engines - comma-separated (e.g., google, bing, duckduckgo), SearXNG defaults if empty; pageno - page number (default 1); limit - max results 1-20 (default 10).",
		InputSchema: json.RawMessage(searchInputSchema),
	}, NewSearchToolHandler(searcher))

	slog.Info("starting SearXNG MCP server")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = server.Run(ctx, &mcp.StdioTransport{})
	if err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}

// searchTool is the minimal interface that a search backend must implement.
type searchTool interface {
	Search(ctx context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error)
}

// NewSearchToolHandler creates an MCP tool handler function that performs SearXNG searches.
// It returns a function suitable for use as an mcp.ToolHandler, which validates the search
// arguments, executes the search, and returns the formatted results.
func NewSearchToolHandler(searcher searchTool) func(context.Context, *mcp.CallToolRequest, searxng.SearchArgs) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args searxng.SearchArgs) (*mcp.CallToolResult, any, error) {
		if args.Limit == nil {
			defaultLimit := defaultResultLimit
			args.Limit = &defaultLimit
		}

		err := searxng.ValidateSearchArgs(&args)
		if err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: fmt.Sprintf("validation error: %v", err)},
				},
				IsError: true,
			}, nil, nil
		}

		resp, err := searcher.Search(ctx, &args)
		if err != nil {
			//nolint:nilerr // MCP handler packs error into tool result
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "Search error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		jsonBytes, err := json.Marshal(resp)
		if err != nil {
			//nolint:nilerr // MCP handler packs error into tool result
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "json marshal error: " + err.Error()},
				},
				IsError: true,
			}, nil, nil
		}

		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(jsonBytes)},
			},
		}, nil, nil
	}
}
