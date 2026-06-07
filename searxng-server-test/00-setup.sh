#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SEARXNG_DIR="${SCRIPT_DIR}/searxng"
VENV_DIR="${SCRIPT_DIR}/.venv"
SETTINGS_FILE="${SCRIPT_DIR}/settings.yml"

echo "=== SearXNG Server Setup ==="

# Check uv
if ! command -v uv &>/dev/null; then
    echo "Error: uv not found. Install: curl -LsSf https://astral.sh/uv/install.sh | sh"
    exit 1
fi
echo "uv: $(uv --version) ✓"

# Check searxng submodule
if [ ! -f "${SEARXNG_DIR}/setup.py" ]; then
    echo "Error: SearXNG submodule not found. Run: git submodule update --init --recursive --depth 1"
    exit 1
fi
echo "SearXNG: found ✓"

# Clean up previous installation
echo ""
echo "--- Cleaning up ---"
if [ -d "$VENV_DIR" ]; then
    echo "  Removing old .venv..."
    rm -rf "$VENV_DIR"
fi
if [ -f "$SETTINGS_FILE" ]; then
    echo "  Removing old settings.yml..."
    rm -f "$SETTINGS_FILE"
fi

# Create venv (official: python3 -m venv)
echo ""
echo "--- Creating venv at ${VENV_DIR} ---"
uv venv "$VENV_DIR" --python python3
source "${VENV_DIR}/bin/activate"

# Install locked helper-script dependencies before adding the larger SearXNG
# runtime set. --inexact avoids pruning anything if this setup is reused.
echo ""
echo "--- Installing setup helper dependencies ---"
uv sync --locked --project "$SCRIPT_DIR" --inexact --no-install-project

# Install dependencies (official order: pip, setuptools, wheel, then deps, then searxng)
echo ""
echo "--- Installing SearXNG dependencies ---"
cd "$SEARXNG_DIR"

echo "  Installing pip, setuptools, wheel..."
uv pip install -U pip setuptools wheel

echo "  Installing pyyaml, msgspec, typing-extensions, pybind11..."
uv pip install -U pyyaml msgspec typing-extensions pybind11

echo "  Installing searxng (editable)..."
uv pip install --no-build-isolation -e .

# Copy settings.yml from repo defaults
echo ""
echo "--- Configuring settings.yml ---"
cp "${SEARXNG_DIR}/searx/settings.yml" "$SETTINGS_FILE"

# Apply structured settings with locked Python script dependencies.
uv run --locked --project "$SCRIPT_DIR" --no-sync python3 "${SCRIPT_DIR}/apply-settings.py" "$SETTINGS_FILE"

echo ""
echo "=== Setup complete ==="
echo ""
echo "To start SearXNG:"
echo "  ./01-start-bg.sh        # background (recommended; agents + CI)"
echo "  ./01-start-fg.sh        # foreground (human interactive debug only)"
