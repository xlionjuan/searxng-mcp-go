#!/usr/bin/env bash
set -euo pipefail

# Report whether a background SearXNG test server is running.
#
# States:
#   live   — .bg-pid points at a running process AND the port responds
#   stale  — .bg-pid points at a dead process (PID file will be cleared)
#   dead   — no PID file, no orphaned searx/webapp.py process
#   orphan — no PID file but pgrep finds searx/webapp.py (likely from a prior
#            crashed agent session; use ./02-stop.sh --force to clean up)
#
# Exit codes:
#   0  — server is live
#   1  — server is not live (stale / dead / orphan)
#
# Usage:
#   ./03-status.sh
#   just test-server-status

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib-searxng-pid.sh
source "${SCRIPT_DIR}/lib-searxng-pid.sh"
SEARXNG_PID_FILE="${SCRIPT_DIR}/.bg-pid"
SEARXNG_PORT="${SEARXNG_PORT:-8888}"

is_port_open() {
    curl -sf -o /dev/null "http://127.0.0.1:${SEARXNG_PORT}/" 2>/dev/null
}

if [ -f "$SEARXNG_PID_FILE" ]; then
    pid="$(cat "$SEARXNG_PID_FILE" 2>/dev/null || true)"
    if [ -n "$pid" ] && is_searxng_pid "$pid"; then
        if is_port_open; then
            echo "live  — PID $pid, port $SEARXNG_PORT responding"
            exit 0
        fi
        echo "degraded — PID $pid alive but port $SEARXNG_PORT not responding"
        echo "           process may still be starting up; tail searxng.log"
        exit 1
    fi
    echo "stale — .bg-pid points at PID ${pid:-?} which is not this SearXNG"
    echo "         (PID file will be cleared on next start; run ./02-stop.sh to clean up)"
    exit 1
fi

# No PID file — look for orphans
orphans="$(pgrep -f 'searx/webapp\\.py|searx\\.webapp:app' 2>/dev/null || true)"
if [ -n "$orphans" ]; then
    echo "orphan — no .bg-pid but a SearXNG process (searx/webapp.py or granian via searx.webapp:app) is running (PIDs: $orphans)"
    echo "          this is leftover from a previous crashed session"
    echo "          run: ./02-stop.sh --force"
    exit 1
fi

echo "dead  — no SearXNG test server is running"
echo "        start with: ./01-start-bg.sh   (or: just test-server-start)"
exit 1
