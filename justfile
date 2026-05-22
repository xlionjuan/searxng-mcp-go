# justfile for searxng-mcp-go
#
# Common commands:
#   just           — run default (test)
#   just build     — build binary
#   just test      — run tests with race detector
#   just check     — full pre-PR gate (fmt → vet → lint → test)
#   just clean     — remove build artifacts

binary := "searxng-mcp-go"
coverfile := "coverage.out"

# Run tests (default)
default: test

# Build the binary
build:
    go build -o {{ binary }} .

# Run tests with race detector
test:
    go test -race -shuffle=on ./...

# Run tests with coverage
test-cover:
    go test -race -shuffle=on -coverprofile={{ coverfile }} ./...

# Run tests with verbose output
test-verbose:
    go test -race -shuffle=on -v ./...

# Run integration-tagged tests
test-integration:
    go test -race -shuffle=on -tags=integration ./...

# Run tests in short mode (skip slow tests)
test-short:
    go test -race -shuffle=on -short ./...

# View coverage in browser
cover:
    go tool cover -html={{ coverfile }}

# View coverage as text
cover-text:
    go tool cover -func={{ coverfile }}

# Format code (gofumpt + goimports)
fmt:
    gofumpt -w .
    goimports -w .

# Run go vet
vet:
    go vet ./...

# Run golangci-lint
lint:
    golangci-lint run --timeout 5m

# Run all code quality checks
check: fmt vet lint
    just test

# Verify and tidy dependencies
mod-tidy:
    go mod tidy
    git diff --exit-code go.mod go.sum

# Verify module checksums
mod-verify:
    go mod verify

# Run govulncheck
vulncheck:
    go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Remove build artifacts and coverage files
clean:
    rm -f {{ binary }} {{ coverfile }}

# Full CI-like pipeline: fmt → vet → lint → test
ci: fmt vet lint
    just test-cover

# Quick check (no formatting, just vet + lint + test)
quick:
    just vet lint test

# Real server test related

# Setup SearXNG test server (venv, deps, config)
test-server-setup:
    cd searxng-server-test && bash 00-setup.sh

# Start SearXNG test server (port 8888)
test-server-start:
    cd searxng-server-test && bash 01-start.sh

# Update SearXNG submoudle
update-searxng-submodule:
    git submodule update --remote searxng-server-test/searxng
    git add searxng-server-test/searxng
    git commit -m "Update SearXNG submodule that for test"
    git log -5
