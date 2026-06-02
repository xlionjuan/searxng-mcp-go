#!/usr/bin/env bash
set -euo pipefail

# Stop a background SearXNG test server that was started by 01-start-bg.sh.
#
# Strategy:
#   1. Read PID from .bg-pid and SIGTERM it.
#   2. If still alive after 5 s, SIGKILL.
#   3. If .bg-pid is missing/stale (process already gone), fall back to
#      pgrep -f 'searx/webapp.py' to find any orphaned searx process and
#      offer to stop it. This is a safety net for cases where the PID file
#      was lost (e.g. agent shell crash) but SearXNG is still running.
#
# Usage:
#   ./02-stop.sh           # stop, exit 0 even if nothing was running
#   ./02-stop.sh --force   # kill orphaned searx/webapp.py if .bg-pid is stale

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SEARXNG_PID_FILE="${SCRIPT_DIR}/.bg-pid"
# SEARXNG_LOG_FILE is intentionally kept on disk for post-mortem debugging
# (see 01-start-bg.sh). It is not deleted by this script.

force=0
if [ "${1:-}" = "--force" ] || [ "${1:-}" = "-f" ]; then
    force=1
fi

stop_pid() {
    local pid="$1"
    if ! kill -0 "$pid" 2>/dev/null; then
        return 0
    fi
    kill "$pid" 2>/dev/null || true
    for _ in $(seq 1 5); do
        kill -0 "$pid" 2>/dev/null || return 0
        sleep 1
    done
    if kill -0 "$pid" 2>/dev/null; then
        echo "  process did not exit on SIGTERM, sending SIGKILL..." >&2
        kill -9 "$pid" 2>/dev/null || true
    fi
}

stopped=0

# Primary path: read .bg-pid
if [ -f "$SEARXNG_PID_FILE" ]; then
    pid="$(cat "$SEARXNG_PID_FILE" 2>/dev/null || true)"
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
        echo "Stopping SearXNG (PID $pid)..."
        stop_pid "$pid"
        stopped=1
    else
        echo "Stale PID file (PID ${pid:-?} not running); removing."
    fi
    rm -f "$SEARXNG_PID_FILE"
fi

# Fallback: look for any orphaned searx/webapp.py process
if [ "$stopped" -eq 0 ]; then
    orphans="$(pgrep -f 'searx/webapp\.py' 2>/dev/null || true)"
    if [ -n "$orphans" ]; then
        if [ "$force" -eq 1 ]; then
            echo "Found orphaned searx/webapp.py (PIDs: $orphans); --force killing."
            for opid in $orphans; do
                stop_pid "$opid"
            done
            stopped=1
        else
            echo "Found orphaned searx/webapp.py (PIDs: $orphans)." >&2
            echo "These are not tracked by .bg-pid. Re-run with --force to kill," >&2
            echo "or use 'pgrep -f searx/webapp.py' to inspect first." >&2
        fi
    fi
fi

if [ "$stopped" -eq 0 ]; then
    echo "SearXNG is not running."
fi

# Log file is intentionally NOT removed — it is useful post-mortem evidence.
# Use 'rm $SEARXNG_LOG_FILE' manually if you want a clean slate.
exit 0
