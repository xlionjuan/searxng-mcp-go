#!/usr/bin/env bash
# Guard: every ADR file in docs/adr/ must appear in the docs/agents/domain.md
# directory diagram and vice versa.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIAGRAM_FILE="$ROOT/docs/agents/domain.md"
ADR_DIR="$ROOT/docs/adr"

# Extract ADR filenames from the diagram (between the docs/adr/ and src/ lines).
mapfile -t diagram_adrs < <(
  sed -n '/├── docs\/adr\//,/└── src\//{
    /│   [├└]── [0-9]/{
      s/.*│   [├└]── //p
    }
  }' "$DIAGRAM_FILE"
)

# Get the actual list of ADR files.
mapfile -t actual_adrs < <(ls "$ADR_DIR")

# Sort both for comparison.
IFS=$'\n' sorted_diagram=($(sort <<<"${diagram_adrs[*]}")); unset IFS
IFS=$'\n' sorted_actual=($(sort <<<"${actual_adrs[*]}")); unset IFS

diff_out=$(diff <(printf "%s\n" "${sorted_diagram[@]}") <(printf "%s\n" "${sorted_actual[@]}")) || {
  echo "ERROR: ADR listing mismatch between docs/agents/domain.md diagram and docs/adr/"
  echo ""
  echo "Differences (left = diagram, right = actual files):"
  echo "$diff_out"
  exit 1
}

echo "OK: ADR listing is up to date."
