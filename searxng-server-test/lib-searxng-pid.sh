#!/usr/bin/env bash
# Shared helpers for the background SearXNG test server scripts.
# This file is meant to be sourced (not executed) by:
#   - 01-start-bg.sh
#   - 02-stop.sh
#   - 03-status.sh
#
# Keeping a single source of truth avoids drift between the three scripts
# when the PID ownership contract changes.

# is_searxng_pid <pid>
#   Return 0 if <pid> is alive AND its argv contains 'searx/webapp.py' or 'searx.webapp:app'.
#   Return 1 otherwise (process gone, recycled, or unrelated).
#
# This guards against a classic Unix pitfall: a PID recorded on disk can be
# recycled by an unrelated process between writes. Treating the recycled PID
# as "our SearXNG" would let stop/status scripts SIGTERM/SIGKILL a stranger
# or report a false live state, and would block a clean re-start.
is_searxng_pid() {
    local pid="$1"
    [ -n "$pid" ] || return 1
    ps -p "$pid" -o args= 2>/dev/null | grep -Fq 'searx/webapp.py' || \
        ps -p "$pid" -o args= 2>/dev/null | grep -Fq 'searx.webapp:app'
}
