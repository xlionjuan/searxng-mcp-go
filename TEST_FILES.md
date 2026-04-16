# Test Files Structure

Test suite for `searxng-mcp-go`, organized by functional area.

## Files

| File | Test Functions | Area |
|------|---------------|------|
| `main_test.go` | `TestPrintCLIHelp`, `TestRunCLIMode_MissingQuery`, `TestRunCLIMode_ValidationError`, `TestRunCLIMode_InvalidTimeRange`, `TestRunCLIMode_InvalidPageno`, `TestRunCLIMode_QueryTooLong`, `TestRunCLIMode_HelpFlag`, `TestRunCLIMode_VersionFlag`, `TestRunCLIMode_SearchErrorReturnsError` | CLI mode and help output |
| `search_test.go` | `TestPerformSearch_Success`, `TestPerformSearch_NetworkError`, `TestPerformSearch_NonOKStatus`, `TestPerformSearch_InvalidJSON`, `TestPerformSearch_InvalidURL`, `TestPerformSearch_TimeRangeParam`, `TestPerformSearch_DefaultLanguage`, `TestPerformSearch_Categories`, `TestPerformSearch_Engines`, `TestPerformSearch_Pageno`, `TestPerformSearch_HTMLResponseError`, `TestPerformSearch_ContextCancellation`, `TestPerformSearch_HTTPStatusCodes`, `TestPerformSearch_NonJSONResponse`, `TestPerformSearch_JSONParseError`, `TestPerformSearch_QueryEncoding` | HTTP search, request building, error handling |
| `format_test.go` | `TestFormatResults` | Output formatting, truncation, HTML unescaping |
| `errors_test.go` | `TestValidationError`, `TestHTTPStatusError` | Error types: `ValidationError` and `HTTPStatusError` |
| `date_test.go` | `TestParseRelativeDate`, `TestParseRelativeDate_ZeroHours`, `TestParseRelativeDate_UpperBoundaries`, `TestParseRelativeDate_FutureDate`, `TestParseRelativeDate_TooOld`, `TestInferDates` | Date parsing, relative date inference, German date patterns |
| `validation_test.go` | `TestValidateSearchArgs` | Input validation: query, language, safesearch, time_range, pageno |

## Shared Helpers

- `intPtr(i int) *int` — defined in `search_test.go`, used by `search_test.go` and `validation_test.go`

## Running Tests

```bash
go test ./...                # Run all tests
go test -cover ./...         # Run with coverage
go test -v ./...             # Verbose output
go test -run TestFormatResults ./...   # Run specific test
```