#!/usr/bin/env bash
# Shell tests for the shared is_searxng_pid helper.
#
# Verifies the PID ownership contract used by 01-start-bg.sh, 02-stop.sh,
# and 03-status.sh: a live PID whose argv does NOT contain
# "searx/webapp.py" must be treated as not-our-SearXNG (stale or recycled).
#
# Run:
#   bash searxng-server-test/test-pid-helper.sh
#   just test-pid-helper
#
# Each test prints PASS/FAIL with a short label. Non-zero exit on any FAIL.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=./lib-searxng-pid.sh
source "${SCRIPT_DIR}/lib-searxng-pid.sh"

PASS=0
FAIL=0

assert_eq() {
    local label="$1"
    local expected="$2"
    local actual="$3"
    if [ "$expected" = "$actual" ]; then
        echo "  PASS  $label"
        PASS=$((PASS + 1))
    else
        echo "  FAIL  $label  (expected=$expected actual=$actual)"
        FAIL=$((FAIL + 1))
    fi
}

echo "== is_searxng_pid =="

# Case 1: empty PID → false
if is_searxng_pid ""; then actual=0; else actual=1; fi
assert_eq "empty PID is rejected"     "1" "$actual"

# Case 2: non-existent PID → false (use a clearly impossible value)
if is_searxng_pid "999999999"; then actual=0; else actual=1; fi
assert_eq "non-existent PID is rejected" "1" "$actual"

# Case 3: live unrelated process (sleep) → false
sleep 30 &
unrelated_pid=$!
disown "$unrelated_pid" 2>/dev/null || true
sleep 0.1
# Confirm it really is alive
if kill -0 "$unrelated_pid" 2>/dev/null; then
    if is_searxng_pid "$unrelated_pid"; then actual=0; else actual=1; fi
    assert_eq "live 'sleep' process is rejected" "1" "$actual"
else
    echo "  SKIP  live 'sleep' process (could not spawn)"
fi
kill "$unrelated_pid" 2>/dev/null || true
wait "$unrelated_pid" 2>/dev/null || true

# Case 4: live process whose argv contains "searx/webapp.py" → true
# Use bash 'exec -a' to spoof argv[0] for a sleep process. This is the
# closest safe stand-in for a real SearXNG process for testing.
bash -c 'exec -a "searx/webapp.py" sleep 30' &
fake_pid=$!
disown "$fake_pid" 2>/dev/null || true
sleep 0.1
if kill -0 "$fake_pid" 2>/dev/null; then
    if is_searxng_pid "$fake_pid"; then actual=0; else actual=1; fi
    assert_eq "argv-matching process is accepted" "0" "$actual"
else
    echo "  FAIL  argv-matching process could not be spawned"
    FAIL=$((FAIL + 1))
fi
kill "$fake_pid" 2>/dev/null || true
wait "$fake_pid" 2>/dev/null || true

# Case 5: PID that no longer exists (just-killed child) → false
sleep 0.1 &
dead_pid=$!
wait "$dead_pid" 2>/dev/null || true
if is_searxng_pid "$dead_pid"; then actual=0; else actual=1; fi
assert_eq "just-exited PID is rejected" "1" "$actual"

# Case 6: PID is non-numeric → false (defense in depth)
if is_searxng_pid "not-a-pid"; then actual=0; else actual=1; fi
# 'ps -p not-a-pid' returns non-zero → false. Document observed behavior.
assert_eq "non-numeric PID is rejected" "1" "$actual"

echo
echo "Summary: $PASS passed, $FAIL failed"
if [ "$FAIL" -gt 0 ]; then
    exit 1
fi
