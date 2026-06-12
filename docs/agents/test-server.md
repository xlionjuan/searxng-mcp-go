# Local SearXNG Test Server

Use this guide for E2E and integration tests that require a live local SearXNG
instance. The `just` recipes in the root `justfile` are the primary interface;
the shell scripts under `searxng-server-test/` are implementation details.

## Quick Start

```bash
# One-time submodule initialization
git submodule update --init --depth 1 searxng-server-test/searxng

# One-time setup: creates venv, installs deps, generates settings.yml
just test-server-setup

# Day-to-day
just test-server-start
export SEARXNG_URL=http://127.0.0.1:8888
go test -tags=e2e -run TestMCPStdioE2E -count=1 .
just test-server-stop
```

## Recipes

| Command | Purpose |
|---|---|
| `just test-server-setup` | One-time setup or reset |
| `just test-server-deps-sync` | Sync pyproject.toml deps from upstream SearXNG requirements; run after submodule update |
| `just test-server-start` | Start background server and wait for readiness |
| `just test-server-status` | Report live / degraded / stale / dead / orphan |
| `just test-server-logs` | Tail `searxng-server-test/searxng.log` |
| `just test-server-stop` | Stop background instance; uses SIGTERM then SIGKILL fallback |
| `just test-server-restart` | Stop then start |
| `just test-server-start-fg` | Foreground mode for humans only; do not use from agents or CI |

## Underlying Scripts

Agents should prefer `just` recipes. These scripts are listed for debugging
script changes or ownership checks.

| Script | Purpose |
|---|---|
| `searxng-server-test/00-setup.sh` | One-time venv, deps, and `settings.yml` setup |
| `searxng-server-test/01-start-bg.sh` | Background start with PID tracking and readiness polling |
| `searxng-server-test/01-start-fg.sh` | Foreground start; do not use from agents or CI |
| `searxng-server-test/02-stop.sh` | Stop background instance; `--force` kills orphans |
| `searxng-server-test/03-status.sh` | Report live / stale / dead / orphan state |
| `searxng-server-test/50-sync-searxng-requirement-to-pyproject.sh` | Re-import upstream requirements into pyproject.toml and refresh uv.lock |
| `searxng-server-test/lib-searxng-pid.sh` | Shared `is_searxng_pid` ownership check |
| `searxng-server-test/test-pid-helper.sh` | Shell unit tests for `is_searxng_pid` |

## Key Details

- The server runs on `http://127.0.0.1:8888`.
- Set `SEARXNG_URL=http://127.0.0.1:8888` before running E2E tests.
- `01-start-bg.sh` polls readiness for up to 30 seconds, then exits while the
  server remains detached in the background.
- `settings.yml` is generated from SearXNG defaults, enables JSON format,
  enables the yahoo / bing / ddg-definitions engines, and generates a random
  secret key.
- Background state is tracked in `searxng-server-test/.bg-pid` and
  `searxng-server-test/searxng.log`; both are kept out of git.
- Check `searxng-server-test/searxng.log` first when E2E tests fail with
  connection or timeout errors.
- Starting the server a second time without stopping it first is rejected; use
  `just test-server-restart`.

## Stress Testing

The stress and concurrency tests use the same test server setup. The
`stress` build tag groups in-process concurrency tests with the live-server
E2E stress tests; the combined `e2e stress` tag is required for the latter.

| Command | Scope |
|---|---|
| `go test -tags=stress -race ./...` | Internal concurrency tests; no SearXNG required |
| `SEARXNG_URL=http://127.0.0.1:8888 go test -tags='e2e stress' -run 'TestMCPStress' -race -count=1 -timeout=900s .` | Live-server E2E stress suite; requires a running test server |
| `.github/workflows/e2e-stress.yml` | Manual CI workflow dispatch |

Use the same setup and lifecycle as the regular E2E tests: run
`just test-server-setup` once, then `just test-server-start` and
`just test-server-stop` around the stress run. The CI workflow uses
`E2E_MCP_BINARY=./searxng-mcp-go` to skip the per-test `go build`.

## Pitfalls

- `searxng-server-test/searxng/` is a git submodule registered in the root
  `.gitmodules`. Do not run `git init` or `git submodule add` inside
  `searxng-server-test/`.
- Do not use `searx/limiter.toml` from the production template. The generated
  test settings use `limiter: false`.
- The repo's default settings have `valkey.url: false`; do not use
  `utils/templates/etc/searxng/settings.yml`, which enables valkey.
- When testing with granian, pass `--interface wsgi`.
- Never run `01-start-fg.sh` from agents or CI. It blocks the calling shell
  until killed.
- `.bg-pid` can go stale if an agent shell dies. Inspect with
  `pgrep -f 'searx/webapp\\.py|searx\\.webapp:app'`, then use `just test-server-stop` or the stop
  script with `--force` if needed.
- A recorded PID can be recycled by an unrelated process. The start, stop, and
  status scripts verify that argv contains `searx/webapp.py` or `searx.webapp:app`; do not bypass that
  check.
