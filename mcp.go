package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"searxng-mcp-go/internal/searxng"
)

// buildSearchSchema generates the JSON Schema for the search tool input from
// the centralized SearchParams table.
func buildSearchSchema() (json.RawMessage, error) {
	props := make(map[string]any)

	var required []string

	for _, p := range searxng.SearchParams {
		props[p.Name] = p.JSONSchema()

		if p.Required {
			required = append(required, p.Name)
		}
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
		return nil, fmt.Errorf("marshal search schema: %w", err)
	}

	return json.RawMessage(data), nil
}

// buildToolDescription returns a concise tool-level description for the MCP search tool.
// Detailed parameter descriptions live in the structured inputSchema; this
// description provides context at the tool level only.
func buildToolDescription() string {
	return "Search the web using SearXNG meta-search engine. " +
		"Returns web results with titles, URLs, summaries, published dates, and engine source information."
}

// newSearchTool creates the search tool definition with annotations marking it
// as read-only and open-world, signaling to clients that it does not modify
// state and returns untrusted external content.
func newSearchTool(schema json.RawMessage) *mcp.Tool {
	openWorldHint := true

	return &mcp.Tool{
		Name:        "search",
		Description: buildToolDescription(),
		InputSchema: schema,
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: &openWorldHint,
		},
	}
}

// mcpFirstMessage is the JSON-RPC envelope of the first message an MCP client
// sends over stdin to start a session.
type mcpFirstMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

const (
	// mcpFirstMessageMaxBytes is the fixed transport bound, in bytes, for the
	// first newline-delimited JSON message read from stdin. The trailing
	// newline delimiter is allowed in addition to this bound. It is deliberately
	// a transport limit, independent of any search payload limits.
	mcpFirstMessageMaxBytes = 1 << 20

	// mcpFirstMessageReadChunkSize keeps the incremental preflight reader's
	// buffer small while still allowing ordinary MCP messages to be read with
	// few underlying calls.
	mcpFirstMessageReadChunkSize = 32 << 10

	// mcpFirstMessageMaxDelimiterBytes permits either an LF or CRLF delimiter
	// in addition to the JSON transport bound.
	mcpFirstMessageMaxDelimiterBytes = len("\r\n")

	mcpMethodInitialize = "initialize"
	mcpMethodDiscover   = "server/discover"

	// metaKeyProtocolVersion marks a request that uses the stateless MCP
	// protocol introduced in 2026-07-28. The SDK validates the complete
	// metadata object after this transport gate accepts the message.
	metaKeyProtocolVersion = "io.modelcontextprotocol/protocolVersion"
)

var (
	errInvalidMCPFirstMessage             = errors.New("stdin does not contain a valid MCP first message")
	errMCPFirstMessageTooLong             = errors.New("MCP first message exceeds the transport limit")
	errInvalidMCPFirstMessageReaderResult = errors.New("invalid MCP stdin reader result")
)

// prepareMCPStdin reads the first line of stdin to verify it starts a valid
// MCP session (legacy "initialize", "server/discover", or a stateless request
// carrying protocol metadata), preventing the MCP server from hanging when
// piped non-MCP input.
func prepareMCPStdin(stdin io.Reader) (io.Reader, error) {
	firstLine, leftover, err := readFirstLine(stdin, mcpFirstMessageMaxBytes)
	if err != nil || !isValidMCPFirstMessage(firstLine) {
		return nil, errInvalidMCPFirstMessage
	}

	return io.MultiReader(
		bytes.NewReader(firstLine),
		bytes.NewReader(leftover),
		stdin,
	), nil
}

// readFirstLine incrementally reads through the first newline, retaining any
// bytes read after that newline as leftover. The reader never consumes more
// than maxBytes plus the longest supported delimiter (CRLF), so an oversized
// first line is rejected without buffering attacker-controlled input or
// allocating the entire transport bound up front. A line ending at EOF is
// accepted for JSON validation by the caller.
func readFirstLine(reader io.Reader, maxBytes int) ([]byte, []byte, error) {
	if reader == nil || maxBytes < 1 {
		return nil, nil, errMCPFirstMessageTooLong
	}

	// The fixed transport bound used by prepareMCPStdin makes this addition
	// safe. Keep the helper defensive for direct tests and future callers.
	maxInt := int(^uint(0) >> 1)
	if maxBytes > maxInt-mcpFirstMessageMaxDelimiterBytes {
		return nil, nil, errMCPFirstMessageTooLong
	}

	readLimit := maxBytes + mcpFirstMessageMaxDelimiterBytes
	chunk := make([]byte, mcpFirstMessageReadChunkSize)
	lineCapacity := min(maxBytes, len(chunk))
	line := make([]byte, 0, lineCapacity)

	consumed := 0
	for consumed < readLimit {
		readSize := min(len(chunk), readLimit-consumed)

		nRead, readErr := reader.Read(chunk[:readSize])
		if nRead < 0 || nRead > readSize {
			return nil, nil, errInvalidMCPFirstMessageReaderResult
		}

		if nRead == 0 {
			if readErr == nil {
				return nil, nil, io.ErrNoProgress
			}

			return mcpFirstMessageReadError(line, readErr, maxBytes)
		}

		consumed += nRead

		chunkLine, leftover, complete := consumeMCPFirstLineChunk(line, chunk[:nRead])
		line = chunkLine

		if complete {
			return mcpFirstMessageLineResult(line, leftover, maxBytes)
		}

		if readErr != nil {
			return mcpFirstMessageReadError(line, readErr, maxBytes)
		}
	}

	return nil, nil, errMCPFirstMessageTooLong
}

func mcpFirstMessageReadError(line []byte, err error, maxBytes int) ([]byte, []byte, error) {
	if errors.Is(err, io.EOF) {
		return mcpFirstMessageLineResult(line, nil, maxBytes)
	}

	return nil, nil, err
}

func mcpFirstMessageLineResult(line, leftover []byte, maxBytes int) ([]byte, []byte, error) {
	if mcpFirstMessagePayloadLength(line) > maxBytes {
		return nil, nil, errMCPFirstMessageTooLong
	}

	return line, leftover, nil
}

func mcpFirstMessagePayloadLength(line []byte) int {
	payloadLength := len(line)
	if payloadLength == 0 || line[payloadLength-1] != '\n' {
		return payloadLength
	}

	payloadLength--
	if payloadLength > 0 && line[payloadLength-1] == '\r' {
		payloadLength--
	}

	return payloadLength
}

func consumeMCPFirstLineChunk(line, data []byte) ([]byte, []byte, bool) {
	newline := bytes.IndexByte(data, '\n')
	if newline < 0 {
		return append(line, data...), nil, false
	}

	line = append(line, data[:newline+1]...)
	leftover := append([]byte(nil), data[newline+1:]...)

	return line, leftover, true
}

// isValidMCPFirstMessage checks whether the given byte slice is a valid JSON-RPC
// 2.0 message that can start an MCP session: the legacy "initialize" handshake,
// the stateless "server/discover" RPC, or any request carrying the per-request
// protocol metadata. Full protocol validation remains the SDK's responsibility.
func isValidMCPFirstMessage(line []byte) bool {
	if len(bytes.TrimSpace(line)) == 0 {
		return false
	}

	var msg mcpFirstMessage

	err := json.Unmarshal(line, &msg)
	if err != nil {
		return false
	}

	if msg.JSONRPC != "2.0" {
		return false
	}

	if msg.Method == mcpMethodInitialize || msg.Method == mcpMethodDiscover {
		return true
	}

	return msg.hasStatelessProtocolMeta()
}

// hasStatelessProtocolMeta reports whether the first message carries the
// stateless protocol-version metadata key. The SDK performs all further
// validation, including value type, supported version, and required peer
// capability metadata.
func (m mcpFirstMessage) hasStatelessProtocolMeta() bool {
	if len(m.Params) == 0 {
		return false
	}

	var params struct {
		//nolint:tagliatelle // matches the wire key "_meta"
		Meta map[string]json.RawMessage `json:"_meta"`
	}

	err := json.Unmarshal(m.Params, &params)
	if err != nil {
		return false
	}

	_, ok := params.Meta[metaKeyProtocolVersion]

	return ok
}

// runMCPMode starts the MCP stdio server, registers the search tool, and
// blocks until a signal (SIGINT/SIGTERM) is received or the server exits.
// It returns an error on failure so that main can handle exit codes and
// deferred cleanup (searcher.Close) runs correctly.
func runMCPMode(debug bool, flags *CLIFlags, stdin io.Reader) error {
	cfg, err := getConfig(flags, false)
	if err != nil {
		return fmt.Errorf("configuration error: %w", err)
	}

	searcher, err := searxng.NewSearXNGSearcher(cfg, debug)
	if err != nil {
		return fmt.Errorf("failed to create searcher: %w", err)
	}

	defer func() { _ = searcher.Close() }() //nolint:errcheck // cleanup in defer; error is non-actionable

	schema, err := buildSearchSchema()
	if err != nil {
		return fmt.Errorf("failed to build search schema: %w", err)
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "searxng-mcp-go",
		Version: version,
	}, nil)

	mcp.AddTool(server, newSearchTool(schema), NewSearchToolHandler(searcher))

	slog.Info("starting SearXNG MCP server")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	err = server.Run(ctx, &mcp.IOTransport{
		Reader: io.NopCloser(stdin),
		Writer: os.Stdout,
	})
	if err != nil {
		// SIGINT/SIGTERM cancels the context; a normal shutdown should
		// not be reported as a server failure.
		if errors.Is(err, context.Canceled) {
			return nil
		}

		return fmt.Errorf("server failed: %w", err)
	}

	return nil
}

// searcher is the minimal interface the MCP handler needs from a search provider.
type searcher interface {
	Search(ctx context.Context, args *searxng.SearchArgs) (*searxng.SearchResponse, error)
}

var _ searcher = (*searxng.SearXNGSearcher)(nil)

// NewSearchToolHandler creates an MCP tool handler function that performs SearXNG searches.
// It returns a function suitable for use as an mcp.ToolHandler, which validates the search
// arguments, executes the search, and returns the formatted results.
func NewSearchToolHandler(searcher searcher) func(
	context.Context, *mcp.CallToolRequest, searxng.SearchArgs,
) (*mcp.CallToolResult, any, error) {
	return func(ctx context.Context, _ *mcp.CallToolRequest, args searxng.SearchArgs) (*mcp.CallToolResult, any, error) {
		normalized, err := prepareAndValidate(args)
		if err != nil {
			return mcpErrorResult("Validation error: " + err.Error()), nil, nil //nolint:nilerr // error in CallToolResult
		}

		return searchAndBuildResult(ctx, searcher, normalized)
	}
}

// mcpErrorResult builds an MCP tool error result with the given text.
func mcpErrorResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
		IsError: true,
	}
}

// mcpSuccessResult builds an MCP tool success result from marshaled JSON bytes.
func mcpSuccessResult(data []byte) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: string(data)},
		},
	}
}

// prepareAndValidate applies defaults and validates search arguments.
func prepareAndValidate(args searxng.SearchArgs) (*searxng.SearchArgs, error) {
	args.ApplyDefaults()

	normalized, err := searxng.ValidateSearchArgs(&args)
	if err != nil {
		return nil, err
	}

	return normalized, nil
}

// searchAndBuildResult performs the search and builds the MCP result.
func searchAndBuildResult(ctx context.Context, s searcher, args *searxng.SearchArgs) (*mcp.CallToolResult, any, error) {
	resp, err := s.Search(ctx, args)
	if err != nil {
		slog.Error("search failed", "error", err)

		if searxngErr, ok := errors.AsType[*searxng.SearXNGError](err); ok {
			return mcpErrorResult("Search error: " + searxngErr.Error()), nil, nil
		}

		return mcpErrorResult("Search error: request failed"), nil, nil
	}

	jsonBytes, _ := json.Marshal(resp) //nolint:errcheck,errchkjson // all concrete types; cannot fail

	return mcpSuccessResult(jsonBytes), nil, nil
}
