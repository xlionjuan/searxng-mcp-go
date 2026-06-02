#!/usr/bin/env bash
set -euo pipefail

# Background start script for SearXNG test server.
# Sources the venv, starts SearXNG in the background (detached via nohup),
# writes the PID to .bg-pid for later cleanup, and polls for readiness.
#
# Usage:
#   ./01-start-bg.sh          # start and wait for readiness
#   SEARXNG_PORT=9999 ./01-start-bg.sh  # custom port
#
# Companion scripts:
#   01-start-fg.sh  — foreground start (human interactive debug only)
#   02-stop.sh      — stop a background instance started by this script
#   03-status.sh    — show whether a background instance is alive
#
# State files (kept out of git via .gitignore):
#   ${SCRIPT_DIR}/.bg-pid      — PID of the background searx/webapp.py
#   ${SCRIPT_DIR}/searxng.log  — stdout/stderr of the background process

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib-searxng-pid.sh
source "${SCRIPT_DIR}/lib-searxng-pid.sh"
VENV_DIR="${SCRIPT_DIR}/.venv"
SEARXNG_DIR="${SCRIPT_DIR}/searxng"
SETTINGS_FILE="${SCRIPT_DIR}/settings.yml"
SEARXNG_PORT="${SEARXNG_PORT:-8888}"
SEARXNG_PID_FILE="${SCRIPT_DIR}/.bg-pid"
SEARXNG_LOG_FILE="${SCRIPT_DIR}/searxng.log"

cleanup_on_failure() {
    local pid="$1"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        kill "$pid" 2>/dev/null || true
        # Give it a moment to exit cleanly, then SIGKILL if needed
        sleep 1
        if kill -0 "$pid" 2>/dev/null; then
            kill -9 "$pid" 2>/dev/null || true
        fi
    fi
    rm -f "$SEARXNG_PID_FILE"
}

if [ ! -d "$VENV_DIR" ]; then
    echo "Error: venv not found. Run: ./00-setup.sh" >&2
    exit 1
fi

if [ ! -f "${SEARXNG_DIR}/searx/webapp.py" ]; then
    echo "Error: SearXNG not found. Run: git submodule update --init --recursive --depth 1" >&2
    exit 1
fi

# Reject double-start: if .bg-pid points at a live process, refuse to spawn a
# second instance. Stale PID files (process gone or PID recycled by an
# unrelated process) are silently cleared so the caller can retry without
# manual cleanup. The ownership check (is_searxng_pid) prevents a recycled
# PID from blocking startup or being misread as a running SearXNG.
if [ -f "$SEARXNG_PID_FILE" ]; then
    existing_pid="$(cat "$SEARXNG_PID_FILE" 2>/dev/null || true)"
    if [ -n "$existing_pid" ] && is_searxng_pid "$existing_pid"; then
        echo "Error: SearXNG is already running (PID $existing_pid)." >&2
        echo "       Run ./02-stop.sh first, or 'just test-server-restart'." >&2
        exit 1
    fi
    rm -f "$SEARXNG_PID_FILE"
fi

source "${VENV_DIR}/bin/activate"

export SEARXNG_SETTINGS_PATH="$SETTINGS_FILE"
export SEARXNG_DEBUG="${SEARXNG_DEBUG:-0}"

cd "$SEARXNG_DIR"

# Detach from the calling shell so the background process survives the agent's
# tool shell exit (SIGHUP/SIGTTIN). nohup ignores SIGHUP; </dev/null cuts the
# controlling tty; & disown removes it from the shell's job table.
nohup python searx/webapp.py >>"$SEARXNG_LOG_FILE" 2>&1 </dev/null &
SEARXNG_PID=$!
disown "$SEARXNG_PID" 2>/dev/null || true
echo "$SEARXNG_PID" > "$SEARXNG_PID_FILE"

echo "SearXNG starting (PID: $SEARXNG_PID, port: $SEARXNG_PORT)..."
echo "  log: $SEARXNG_LOG_FILE"

# If the child dies before the readiness loop, surface that immediately
# instead of polling a port nothing is bound to.
if ! kill -0 "$SEARXNG_PID" 2>/dev/null; then
    echo "Error: SearXNG process exited before becoming ready. See $SEARXNG_LOG_FILE" >&2
    rm -f "$SEARXNG_PID_FILE"
    exit 1
fi

# Poll for readiness. curl -sf exits 0 only on 2xx/3xx; without -f, curl writes
# the literal '000' to stdout on connect failure and the old `grep -q .` check
# incorrectly matched that placeholder.
ready=0
for i in $(seq 1 30); do
    if curl -sf -o /dev/null "http://127.0.0.1:${SEARXNG_PORT}/" 2>/dev/null; then
        echo "SearXNG ready on http://127.0.0.1:${SEARXNG_PORT} (attempt $i)"
        ready=1
        break
    fi
    # Detect early child exit during the poll so we don't wait the full 30 s
    if ! kill -0 "$SEARXNG_PID" 2>/dev/null; then
        echo "Error: SearXNG process exited during startup. See $SEARXNG_LOG_FILE" >&2
        rm -f "$SEARXNG_PID_FILE"
        exit 1
    fi
    sleep 1
done

if [ "$ready" -ne 1 ]; then
    echo "Error: SearXNG did not become ready within 30 seconds. See $SEARXNG_LOG_FILE" >&2
    cleanup_on_failure "$SEARXNG_PID"
    exit 1
fi
