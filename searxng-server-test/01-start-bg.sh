#!/usr/bin/env bash
set -euo pipefail

# Background start script for SearXNG test server.
# Sources the venv, starts SearXNG in background, and waits for port 8888.
# Writes PID to SEARXNG_PID_FILE for later cleanup.
#
# Usage:
#   ./01-start-bg.sh          # start and wait for readiness
#   SEARXNG_PORT=9999 ./01-start-bg.sh  # custom port

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
VENV_DIR="${SCRIPT_DIR}/.venv"
SEARXNG_DIR="${SCRIPT_DIR}/searxng"
SETTINGS_FILE="${SCRIPT_DIR}/settings.yml"
SEARXNG_PORT="${SEARXNG_PORT:-8888}"
SEARXNG_PID_FILE="${SCRIPT_DIR}/.bg-pid"

if [ ! -d "$VENV_DIR" ]; then
    echo "Error: venv not found. Run: ./00-setup.sh" >&2
    exit 1
fi

if [ ! -f "${SEARXNG_DIR}/searx/webapp.py" ]; then
    echo "Error: SearXNG not found. Run: git submodule update --init --recursive --depth 1" >&2
    exit 1
fi

source "${VENV_DIR}/bin/activate"

export SEARXNG_SETTINGS_PATH="$SETTINGS_FILE"
export SEARXNG_DEBUG="${SEARXNG_DEBUG:-0}"

cd "$SEARXNG_DIR"
python searx/webapp.py &
SEARXNG_PID=$!
echo "$SEARXNG_PID" > "$SEARXNG_PID_FILE"

echo "SearXNG starting (PID: $SEARXNG_PID, port: $SEARXNG_PORT)..."

# Poll for readiness
for i in $(seq 1 30); do
    if curl -s -o /dev/null -w '%{http_code}' "http://127.0.0.1:${SEARXNG_PORT}/" 2>/dev/null | grep -q .; then
        echo "SearXNG ready on http://127.0.0.1:${SEARXNG_PORT} (attempt $i)"
        exit 0
    fi
    sleep 1
done

echo "Error: SearXNG did not start within 30 seconds" >&2
exit 1
