# SearXNG MCP Server

A Model Context Protocol (MCP) server providing web search via the SearXNG meta-search engine.

## Purpose

AI agents (like Hermes) call this server to perform web searches without direct internet access. The server proxies requests to a SearXNG instance and returns formatted results.

## Quick Start

```bash
# Build
go build -o searxng-mcp-go .

# Run tests
go test ./...

# Execute (stdio transport - invoked by MCP client host)
./searxng-mcp-go

# Test with MCP Inspector
npx @modelcontextprotocol/inspector ./searxng-mcp-go
```

## Project Structure

```
searxng-mcp-go/
├── main.go              # Main server implementation (CLI + MCP entry points)
├── search.go            # Search functionality, HTTP client, SearXNGSearcher
├── errors.go            # Error types and handling
├── format.go            # Output formatting
├── validation.go        # Input validation
├── date.go              # Date/time utilities
├── constants.go         # Size limits and configuration constants
├── main_test.go         # Main tests (CLI/MCP mode, runCLIMode)
├── search_test.go       # Search tests (isPrivateHost, validateBaseURL, getCachedHTTPClient)
├── errors_test.go       # Error type tests
├── format_test.go       # Formatting tests (including pagination)
├── validation_test.go   # Validation edge case tests
├── date_test.go         # Date parsing and inference tests
├── concurrency_test.go   # Concurrency and stress tests
├── error_path_test.go   # Error path coverage tests
├── go.mod/go.sum        # Go module/dependencies
├── .golangci.yml        # Linter configuration
├── codecov.yml          # Code coverage configuration
├── .env.example         # Environment variable template
├── .github/workflows/   # CI: lint, security, test
└── docs/
    ├── INSTALL.md           # Installation, build, configuration
    ├── MCP_TOOLS.md         # MCP tool documentation
    ├── AI_UX_TEST_GUIDE.md  # AI UX testing guide
    └── LANGUAGE_PARAMETER_RESEARCH.md  # Language parameter research
```

## MCP Tools

### `search`

Search the web via SearXNG.

| Parameter    | Type   | Required | Default | Description                              |
|--------------|--------|----------|---------|------------------------------------------|
| `query`      | string | Yes      | -       | Search query string                      |
| `language`   | string | No       | en      | Language code (en, zh-tw, ja, etc.)      |
| `safesearch` | int    | No       | 0       | 0=Off, 1=Moderate, 2=Strict              |
| `time_range` | string | No       | -       | day, month, year                         |
| `categories` | string | No       | -       | Comma-separated categories               |
| `engines`    | string | No       | -       | Comma-separated engines                  |
| `pageno`     | int    | No       | 1       | Page number (≥1)                         |

See [docs/MCP_TOOLS.md](docs/MCP_TOOLS.md) for full details.

## Configuration

Default SearXNG URL: `https://search-4.xlion.dev`

```bash
# Environment variable
export SEARXNG_URL=https://your-searxng-instance.example.com

# Command-line flag (overrides env var)
./searxng-mcp-go -searxng-url=https://your-searxng-instance.example.com

# JSON output
./searxng-mcp-go "search query" --json
```

Priority: command-line flag > environment variable > default

See [docs/INSTALL.md](docs/INSTALL.md) for full configuration details.

## Error Handling

- Empty or whitespace-only `query` rejected
- Invalid `time_range` values (must be: day, month, year)
- Network/connectivity failures
- SearXNG API errors (non-200, malformed JSON, HTML responses)

## Known Limitations

1. **Pagination**: SearXNG API starts at page 1 (server validates `pageno >= 1`)
2. **Date Inference**: Dates inferred from content when not provided by API
3. **HTML Detection**: Returns `HTMLResponseError` if SearXNG returns HTML instead of JSON
4. **Y2K Threshold**: 2-digit year parsing uses Y2K_THRESHOLD=2000 (needs update before 2038)
5. **Content Length**: Summaries truncated to 4000 UTF-8 runes

## Development

```bash
go build -o searxng-mcp-go .  # Build
go mod tidy                   # Tidy dependencies
go fmt ./...                  # Format code
go test ./...                 # Run tests
```

See [docs/INSTALL.md](docs/INSTALL.md) for Docker and other build options.

## Code Cleanliness

**絕對禁止留下任何垃圾檔案** ⚠️

- 禁止：`.bak`、`.test`（編譯產出的 binary）、`*~`、`.swp`、`.swo` 等任何臨時/備份檔案
- 這些檔案 **必須** 在 commit 前刪除，並永久排除於 Git 歷史之外（已用 `--force --invert-paths` 清除）
- `.gitignore` 已設定 `*.bak`、`.test`、`.swp`、`.swo` 等規則，確保不會再被追蹤

## Review & QA Workflow

**審查報告規範** ⚠️

- 所有程式碼審查、AGENTS.md 審查、test 覆蓋率分析等報告，**一律優先寫入 `REPORT.md`**（置於專案根目錄）
- `REPORT.md` 永久列入 `.gitignore`，**嚴禁 commit**
- 每個 Task 完成後，隨即產生對應的 Todo list，一併寫入 `REPORT.md`
- Todo list 需包含「掌門」（Hermes Agent）待會兒派子代理時所需的所有規劃資訊
