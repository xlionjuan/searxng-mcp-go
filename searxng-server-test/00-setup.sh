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

# Generate random secret key
SECRET_KEY=$(openssl rand -hex 16 2>/dev/null || python3 -c "import secrets; print(secrets.token_hex(16))")
sed -i "s/ultrasecretkey/${SECRET_KEY}/g" "$SETTINGS_FILE"
echo "  Generated secret_key ✓"

# Enable JSON format
if grep -q "formats:" "$SETTINGS_FILE"; then
    if ! grep -q "\- json" "$SETTINGS_FILE"; then
        sed -i '/formats:/,/^[^ ]/{
            s/- html$/- html\n    - json/
        }' "$SETTINGS_FILE"
        echo "  Enabled JSON format ✓"
    else
        echo "  JSON format already enabled"
    fi
else
    echo "  Warning: could not find formats section"
fi

# Enable yahoo and bing for more reliable E2E test results
sed -i '/^  - name: yahoo$/{n;n;n;s/disabled: true/disabled: false/}' "$SETTINGS_FILE"
sed -i '/^  - name: bing$/{n;n;n;s/disabled: true/disabled: false/}' "$SETTINGS_FILE"
echo "  Enabled yahoo and bing engines ✓"

# Enable ddg definitions for infobox content
sed -i '/^  - name: ddg definitions$/{n;n;n;n;s/disabled: true/disabled: false/}' "$SETTINGS_FILE"
echo "  Enabled ddg definitions engine ✓"

# Post-edit validation
echo ""
echo "--- Validating settings.yml ---"

# Check secret_key replaced
if grep -q "ultrasecretkey" "$SETTINGS_FILE"; then
    echo "Error: secret_key not replaced" >&2
    exit 1
fi
echo "  secret_key validated ✓"

# Check JSON format enabled
if ! grep -A 10 "formats:" "$SETTINGS_FILE" | grep -q "json"; then
    echo "Error: JSON format not enabled" >&2
    exit 1
fi
echo "  JSON format validated ✓"

# Check engines enabled
for engine in "yahoo" "bing" "ddg definitions"; do
    if ! grep -A 5 "name: $engine" "$SETTINGS_FILE" | grep -q "disabled: false"; then
        echo "Error: $engine not enabled" >&2
        exit 1
    fi
    echo "  $engine engine validated ✓"
done

echo ""
echo "=== Setup complete ==="
echo ""
echo "To start SearXNG:"
echo "  ./01-start-bg.sh        # background (recommended; agents + CI)"
echo "  ./01-start-fg.sh        # foreground (human interactive debug only)"
