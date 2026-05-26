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
	"strings"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

// buildSearchSchema generates the JSON Schema for the search tool input from
// the centralized SearchParams table.
func buildSearchSchema() json.RawMessage {
	props := make(map[string]any)

	var required []string

	for _, p := range searxng.SearchParams {
		prop := map[string]any{
			"type": p.MCPType,
		}
		if p.Description != "" {
			prop["description"] = p.Description
		}

		if p.Enum != nil {
			prop["enum"] = p.Enum
		}

		if p.Minimum != nil {
			prop["minimum"] = *p.Minimum
		}

		if p.Maximum != nil {
			prop["maximum"] = *p.Maximum
		}

		if p.Nullable {
			// Union type: ["null", "<type>"]
			prop["type"] = []string{"null", p.MCPType}
		}

		if p.Required {
			required = append(required, p.Name)
		}

		props[p.Name] = prop
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           props,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}

	data, err := json.Marshal(schema)
	if err != nil {
		panic(fmt.Sprintf("marshal search schema: %v", err))
	}

	return json.RawMessage(data)
}

// buildToolDescription generates the MCP search tool description from the
// centralized SearchParams table.
func buildToolDescription() string {
	var sb strings.Builder
	sb.WriteString("Search the web using SearXNG meta-search engine. ")
	sb.WriteString("Returns web results with titles, URLs, summaries, published dates, and engine source information. ")
	sb.WriteString("Parameters: ")

	for i, p := range searxng.SearchParams {
		if i > 0 {
			sb.WriteString("; ")
		}

		sb.WriteString(p.Name)

		if p.Required {
			sb.WriteString(" (required)")
		}

		sb.WriteString(" - ")
		sb.WriteString(p.Description)
	}

	sb.WriteString(".")

	return sb.String()
}

type mcpInitializeMessage struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
}

const mcpInitializeMaxBytes = 1 << 20

var errInvalidMCPInitializeMessage = errors.New("stdin does not contain a valid MCP initialize message")

// prepareMCPStdin reads the first line of stdin to verify it contains a valid
// MCP initialize message (JSON-RPC 2.0 with method "initialize"), preventing
// the MCP server from hanging when piped non-MCP input.
func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	reader := bufio.NewReaderSize(stdin, mcpInitializeMaxBytes+1)

	firstLine, err := reader.ReadSlice('\n')
	if errors.Is(err, bufio.ErrBufferFull) {
		return nil, errInvalidMCPInitializeMessage
	}

	if err != nil && !errors.Is(err, io.EOF) {
		return nil, errInvalidMCPInitializeMessage
	}

	if len(firstLine) > mcpInitializeMaxBytes || !isValidMCPInitializeMessage(firstLine) {
		return nil, errInvalidMCPInitializeMessage
	}

	return io.MultiReader(bytes.NewReader(firstLine), reader), nil
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

// runMCPMode starts the MCP stdio server, registers the search tool, and
// blocks until a signal (SIGINT/SIGTERM) is received or the server exits.
func runMCPMode(debug bool, flags CLIFlags, stdin io.Reader) {
	cfg, err := getConfig(flags)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(1)
	}

	searcher, err := searxng.NewSearXNGSearcher(cfg, debug)
	if err != nil {
		slog.Error("failed to create searcher", "error", err)
		os.Exit(1)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "search",
		Description: buildToolDescription(),
		InputSchema: buildSearchSchema(),
	}, NewSearchToolHandler(searcher))

	slog.Info("starting SearXNG MCP server")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)

	err = server.Run(ctx, &mcp.IOTransport{
		Reader: io.NopCloser(stdin),
		Writer: os.Stdout,
	})
	if err != nil {
		slog.Error("server failed", "error", err)

		_ = searcher.Close()

		stop()
		os.Exit(1)
	}

	stop()
}

// searcher is the minimal interface the MCP handler needs from a search provider.
type searcher interface {
	Search(ctx context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error)
}

// NewSearchToolHandler creates an MCP tool handler function that performs SearXNG searches.
// It returns a function suitable for use as an mcp.ToolHandler, which validates the search
// arguments, executes the search, and returns the formatted results.
func NewSearchToolHandler(searcher searcher) func(
	context.Context, *mcp.CallToolRequest, searxng.SearchArgs,
) (*mcp.CallToolResult, any, error) {
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
