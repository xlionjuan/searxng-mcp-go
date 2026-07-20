# justfile for searxng-mcp-go
#
# Common commands:
#   just           — run default (test)
#   just build     — build binary
#   just test      — run tests with race detector
#   just verify   — full CI pipeline (fmt-check → vet → lint → tidy → build → test-cover → test-stress)
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

# Run in-process concurrency stress tests (concurrency_test.go, build tag `stress`).
# No live server required; safe to run anywhere `go test ./...` works.
test-stress:
    go test -race -shuffle=on -tags=stress ./...

# Run E2E tests against a live SearXNG instance (build tag `e2e`).
# Requires `just test-server-start` and `SEARXNG_URL` exported.
# Runs both the regular MCP E2E tests and the E2E stress tests in
# e2e_stress_test.go (which also requires the `stress` build tag).
test-e2e:
    go test -race -tags='e2e stress' -count=1 -timeout=900s .

# Run tests in short mode (skip slow tests)
test-short:
    go test -race -shuffle=on -short ./...

# View coverage in browser
cover:
    go tool cover -html={{ coverfile }}

# View coverage as text
cover-text:
    go tool cover -func={{ coverfile }}

# Check formatting (non-mutating diff, matches CI)
fmt-check:
    golangci-lint fmt --diff

# Apply formatting (mutating)
fmt:
    golangci-lint fmt

# Run go vet
vet:
    go vet ./...

# Run golangci-lint (all build-tag variants matching CI).
# The combined e2e,stress variant ensures e2e_stress_test.go is linted.
lint:
    golangci-lint run --timeout 5m
    golangci-lint run --timeout 5m --build-tags=stress
    golangci-lint run --timeout 5m --build-tags=e2e
    golangci-lint run --timeout 5m --build-tags=e2e,stress

# Verify that the lint build-tag matrix matches between justfile and CI workflow.
# A deterministic drift test to keep them in sync (Issue #498).
lint-drift-check:
    #!/usr/bin/env bash
    set -euo pipefail
    just_count=$(grep -c '^[[:space:]]*golangci-lint run --timeout' justfile)
    workflow_count=$(grep -c 'golangci-lint run --timeout' .github/workflows/lint.yml)
    if [ "$just_count" -ne "$workflow_count" ]; then
        echo "ERROR: Lint invocation count mismatch between justfile and CI" >&2
        exit 1
    fi
    just_tags=$(grep '^[[:space:]]*golangci-lint run --timeout' justfile | sed -n 's/.*--build-tags=//p' | sort -u | tr '\n' ' ' | sed 's/ $//')
    workflow_tags=$(grep 'golangci-lint run --timeout' .github/workflows/lint.yml | sed -n 's/.*--build-tags=//p' | sort -u | tr '\n' ' ' | sed 's/ $//')
    if [ "$just_tags" != "$workflow_tags" ]; then
        echo "ERROR: Lint build-tag set mismatch between justfile and CI" >&2
        exit 1
    fi
    echo "lint-drift-check OK: $just_count invocations, tags: [$just_tags]"

# Full CI pipeline (matches the non-E2E verification gate).
# Mirrors test.yml + lint.yml steps in fail-fast order.
verify: mod-verify fmt-check lint-drift-check vet lint
    go mod download
    just mod-tidy
    go build -o {{ binary }} .
    just test-cover
    just test-stress

# Aliases for verify
alias ci := verify
alias check := verify

# Verify and tidy dependencies
mod-tidy:
    go mod tidy
    git diff --exit-code go.mod go.sum

# Verify module checksums
mod-verify:
    go mod verify

# Run govulncheck
vulncheck:
    go tool govulncheck ./...

# Remove build artifacts and coverage files
clean:
    rm -f {{ binary }} {{ coverfile }}

# Quick check (no formatting, just vet + lint + test)
quick:
    just vet lint test

# Real server test related

# Setup SearXNG test server (venv, deps, config)
test-server-setup:
    cd searxng-server-test && bash 00-setup.sh

# Start SearXNG test server in the background (port 8888)
test-server-start:
    cd searxng-server-test && bash 01-start-bg.sh

# Start SearXNG test server in the foreground (human interactive debug only —
# blocks the calling shell; agents and CI must use test-server-start instead)
test-server-start-fg:
    cd searxng-server-test && bash 01-start-fg.sh

# Stop the background SearXNG test server (no-op if not running)
test-server-stop:
    cd searxng-server-test && bash 02-stop.sh

# Show whether the background SearXNG test server is running
test-server-status:
    cd searxng-server-test && bash 03-status.sh

# Tail the background SearXNG test server log
test-server-logs:
    tail -f searxng-server-test/searxng.log

# Restart the background SearXNG test server
test-server-restart: test-server-stop test-server-start

# Unit-test the is_searxng_pid helper (PID ownership contract)
test-server-pid-helper:
    bash searxng-server-test/test-pid-helper.sh

# Sync pyproject.toml dependencies from upstream SearXNG requirements files
# Run after updating the searxng submodule, or when requirements.txt /
# requirements-server.txt change upstream. Clears existing deps, re-imports,
# and refreshes uv.lock.
test-server-deps-sync:
    cd searxng-server-test && bash 50-sync-searxng-requirement-to-pyproject.sh

# Update SearXNG submoudle
update-searxng-submodule:
    git submodule update --remote searxng-server-test/searxng
    git add searxng-server-test/searxng
    git commit -m "Update SearXNG submodule that for test"
    git log -5

# git pull + submodule
pull:
    git pull --recurse-submodules
