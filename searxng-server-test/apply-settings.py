#!/usr/bin/env python3
# /// script
# requires-python = ">=3.11"
# dependencies = [
#     "pyyaml",
# ]
# ///
"""
apply-settings.py — Apply structured YAML modifications to SearXNG settings.yml.

Replaces fragile sed line-number operations in 00-setup.sh with robust YAML
manipulation via pyyaml.

Operations performed:
  1. Generate a random secret_key (replaces "ultrasecretkey")
  2. Ensure 'json' is in search.formats
  3. Set yahoo, bing, and ddg definitions engines to disabled: false
  4. Re-read and verify all modifications were applied successfully

Usage:
    python3 apply-settings.py <path-to-settings.yml>

Environment:
    SETTINGS_FILE  alternative way to pass the settings file path
"""

import os
import secrets
import sys
from pathlib import Path

import yaml


def get_settings_path() -> Path:
    """Resolve settings file path from CLI argument or SETTINGS_FILE env."""
    if len(sys.argv) > 1:
        return Path(sys.argv[1])
    env_path = os.environ.get("SETTINGS_FILE")
    if env_path:
        return Path(env_path)
    print(
        "Error: settings file path required (CLI arg or SETTINGS_FILE env)",
        file=sys.stderr,
    )
    sys.exit(1)


def load_settings(path: Path) -> dict:
    """Load settings.yml as a Python dict."""
    with open(path, "r") as f:
        return yaml.safe_load(f)


def save_settings(path: Path, data: dict) -> None:
    """Write the modified dict back to settings.yml."""
    with open(path, "w") as f:
        yaml.dump(data, f, default_flow_style=False, allow_unicode=True)


def set_secret_key(data: dict) -> str:
    """Generate and set a random secret key. Returns the key."""
    secret_key = secrets.token_hex(16)
    data.setdefault("server", {})["secret_key"] = secret_key
    return secret_key


def ensure_json_format(data: dict) -> bool:
    """Ensure 'json' is in search.formats. Returns True if added."""
    search = data.setdefault("search", {})
    formats = search.get("formats", [])
    if "json" in formats:
        return False
    formats.append("json")
    search["formats"] = formats
    return True


def enable_engine(data: dict, engine_name: str) -> bool:
    """Set disabled: false for an engine by name. Returns True if changed."""
    engines = data.get("engines", [])
    for engine in engines:
        if engine.get("name") == engine_name and engine.get("disabled") is True:
            engine["disabled"] = False
            return True
    return False


def verify_settings(path: Path) -> None:
    """Re-read file and verify all required keys exist with correct values."""
    with open(path, "r") as f:
        raw = f.read()

    data = yaml.safe_load(raw)
    errors = []

    # Check secret key
    secret = data.get("server", {}).get("secret_key", "")
    if secret == "ultrasecretkey":
        errors.append("secret_key was not updated (still 'ultrasecretkey')")
    elif not secret:
        errors.append("secret_key is empty or missing")

    # Check json format
    formats = data.get("search", {}).get("formats", [])
    if "json" not in formats:
        errors.append("JSON format not found in search.formats")

    # Check engines
    engines = data.get("engines", [])
    engine_map = {e.get("name"): e for e in engines}

    for name in ("yahoo", "bing", "ddg definitions"):
        engine = engine_map.get(name)
        if engine is None:
            errors.append(f"Engine '{name}' not found in settings.yml")
        elif engine.get("disabled") is not False:
            errors.append(f"Engine '{name}' is still disabled")

    if errors:
        for err in errors:
            print(f"  Verification FAILED: {err}", file=sys.stderr)
        sys.exit(1)

    print("  All settings verified ✓")


def main() -> None:
    path = get_settings_path()

    if not path.exists():
        print(f"Error: settings file not found: {path}", file=sys.stderr)
        sys.exit(1)

    data = load_settings(path)

    # 1. Set secret key
    set_secret_key(data)
    print("  Generated secret_key ✓")

    # 2. Enable JSON format
    added = ensure_json_format(data)
    if added:
        print("  Enabled JSON format ✓")
    else:
        print("  JSON format already enabled ✓")

    # 3. Enable yahoo and bing
    for engine_name in ("yahoo", "bing"):
        if enable_engine(data, engine_name):
            print(f"  Enabled {engine_name} engine ✓")
        else:
            print(f"  {engine_name} engine already enabled ✓")

    # 4. Enable ddg definitions
    if enable_engine(data, "ddg definitions"):
        print("  Enabled ddg definitions engine ✓")
    else:
        print("  ddg definitions engine already enabled ✓")

    # 5. Write modified settings
    save_settings(path, data)
    print("  Settings written ✓")

    # 6. Verify by re-reading
    verify_settings(path)


if __name__ == "__main__":
    main()
