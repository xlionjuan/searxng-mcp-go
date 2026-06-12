#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")"

echo "--- Clearing existing dependencies from pyproject.toml ---"

uvx --from tomlkit python - <<'PY'
from pathlib import Path
import tomlkit

path = Path("pyproject.toml")
doc = tomlkit.parse(path.read_text())

doc["project"]["dependencies"] = tomlkit.array()
doc["project"]["dependencies"].multiline(True)

path.write_text(tomlkit.dumps(doc))
PY

# Import deps from SearXNG
uv add setuptools ruamel.yaml -r searxng/requirements.txt -r searxng/requirements-server.txt

uv lock
