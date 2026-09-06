#!/usr/bin/env python3
"""
apply-settings.py — Apply structured YAML modifications to SearXNG settings.yml.

Replaces fragile sed line-number operations in 00-setup.sh with robust YAML
manipulation via ruamel.yaml round-trip mode, preserving comments, key order,
and list indentation so local diffs stay focused on the intended changes.

Operations performed:
  1. Generate a random secret_key (replaces "ultrasecretkey")
  2. Ensure 'json' is in search.formats
  3. Set yahoo, bing, and ddg definitions engines to disabled: false
  4. Re-read and verify all modifications were applied successfully

Usage:
    uv run --locked --project searxng-server-test --no-sync python3 \
        searxng-server-test/apply-settings.py <path-to-settings.yml>
    uv run --locked --project searxng-server-test --no-sync python3 \
        searxng-server-test/apply-settings.py --dry-run <path-to-settings.yml>

Environment:
    SETTINGS_FILE  alternative way to pass the settings file path
"""

import argparse
import os
import secrets
import sys
import tempfile
from enum import Enum
from pathlib import Path
from typing import Any

from ruamel.yaml import YAML


TARGET_ENGINES = ("yahoo", "bing", "ddg definitions")

# Always-empty test engine for E2E. It uses SearXNG's built-in "dummy"
# engine (searx/engines/dummy.py), whose response() always returns an empty
# results array — a pure offline engine with no external server. The E2E suite
# names it explicitly via engines to deterministically exercise the
# "empty results -> tool error" path without depending on a live third-party
# engine returning zero results.
# It lives in a custom category ("empty") so it is never swept into a general
# search; it is only invoked when a query explicitly names it via engines.
EMPTY_ENGINE_NAME = "empty engine"
EMPTY_ENGINE = {
    "name": EMPTY_ENGINE_NAME,
    "engine": "dummy",
    "shortcut": "ee",
    "categories": ["empty"],
    "disabled": False,
}


_yaml = YAML()
_yaml.preserve_quotes = True
_yaml.indent(mapping=2, sequence=4, offset=2)
_yaml.width = 10**9


class EngineStatus(Enum):
    ENABLED = "enabled"
    ALREADY_ENABLED = "already_enabled"
    MISSING = "missing"


def parse_args() -> argparse.Namespace:
    """Resolve settings file path from CLI argument or SETTINGS_FILE env."""
    parser = argparse.ArgumentParser(
        description="Apply structured SearXNG test-server settings updates.",
    )
    parser.add_argument(
        "settings_file",
        nargs="?",
        help="Path to settings.yml. Falls back to SETTINGS_FILE.",
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Validate and report changes without writing settings.yml.",
    )
    args = parser.parse_args()

    if args.settings_file:
        args.path = Path(args.settings_file)
        return args

    env_path = os.environ.get("SETTINGS_FILE")
    if env_path:
        args.path = Path(env_path)
        return args

    parser.error("settings file path required (CLI arg or SETTINGS_FILE env)")


def load_settings(path: Path) -> Any:
    """Load settings.yml as a Python dict."""
    with open(path, "r", encoding="utf-8") as f:
        data = _yaml.load(f)
    if not isinstance(data, dict):
        print("Error: settings.yml root must be a YAML mapping", file=sys.stderr)
        sys.exit(1)
    return data


def save_settings(path: Path, data: Any) -> None:
    """Atomically write the modified settings back to settings.yml."""
    with tempfile.NamedTemporaryFile(
        "w",
        encoding="utf-8",
        dir=path.parent,
        prefix=f".{path.name}.",
        suffix=".tmp",
        delete=False,
    ) as f:
        temp_path = Path(f.name)
        _yaml.dump(data, f)

    try:
        os.chmod(temp_path, path.stat().st_mode)
        os.replace(temp_path, path)
    except Exception:
        temp_path.unlink(missing_ok=True)
        raise


def set_secret_key(data: Any) -> str:
    """Generate and set a random secret key. Returns the key."""
    secret_key = secrets.token_hex(16)
    data.setdefault("server", {})["secret_key"] = secret_key
    return secret_key


def ensure_json_format(data: Any) -> bool:
    """Ensure 'json' is in search.formats. Returns True if added."""
    search = data.setdefault("search", {})
    formats = search.get("formats", [])
    if not isinstance(formats, list):
        print("Error: search.formats must be a YAML list", file=sys.stderr)
        sys.exit(1)
    if "json" in formats:
        return False
    formats.append("json")
    search["formats"] = formats
    return True


def enable_engine(data: Any, engine_name: str) -> EngineStatus:
    """Set disabled: false for an engine by name."""
    engines = data.get("engines", [])
    if not isinstance(engines, list):
        print("Error: engines must be a YAML list", file=sys.stderr)
        sys.exit(1)

    for engine in engines:
        if not isinstance(engine, dict):
            print("Error: every entry in engines must be a YAML mapping", file=sys.stderr)
            sys.exit(1)
        if engine.get("name") != engine_name:
            continue
        if engine.get("disabled") is True:
            engine["disabled"] = False
            return EngineStatus.ENABLED
        return EngineStatus.ALREADY_ENABLED

    return EngineStatus.MISSING


def ensure_empty_engine(data: Any) -> bool:
    """Add the always-empty test engine if it is missing. Returns True if added."""
    engines = data.get("engines", [])
    if not isinstance(engines, list):
        print("Error: engines must be a YAML list", file=sys.stderr)
        sys.exit(1)

    for engine in engines:
        if isinstance(engine, dict) and engine.get("name") == EMPTY_ENGINE_NAME:
            return False

    engines.append(dict(EMPTY_ENGINE))
    return True


def verify_data(data: Any) -> list[str]:
    """Return verification errors for required settings."""
    errors = []

    if not isinstance(data, dict):
        return ["settings.yml root must be a YAML mapping"]

    secret = data.get("server", {}).get("secret_key", "")
    if secret == "ultrasecretkey":
        errors.append("secret_key was not updated (still 'ultrasecretkey')")
    elif not secret:
        errors.append("secret_key is empty or missing")

    formats = data.get("search", {}).get("formats", [])
    if not isinstance(formats, list):
        errors.append("search.formats is not a YAML list")
    elif "json" not in formats:
        errors.append("JSON format not found in search.formats")

    engines = data.get("engines", [])
    if not isinstance(engines, list):
        errors.append("engines is not a YAML list")
        return errors

    engine_map = {}
    for engine in engines:
        if not isinstance(engine, dict):
            errors.append("every entry in engines must be a YAML mapping")
            return errors
        engine_map[engine.get("name")] = engine

    for name in TARGET_ENGINES:
        engine = engine_map.get(name)
        if engine is None:
            errors.append(f"Engine '{name}' not found in settings.yml")
        elif engine.get("disabled") is not False:
            errors.append(f"Engine '{name}' is still disabled")

    if not any(
        isinstance(e, dict) and e.get("name") == EMPTY_ENGINE_NAME for e in engines
    ):
        errors.append(f"Engine '{EMPTY_ENGINE_NAME}' not found in settings.yml")

    return errors


def verify_settings(path: Path) -> None:
    """Re-read file and verify all required keys exist with correct values."""
    data = load_settings(path)
    errors = verify_data(data)

    if errors:
        for err in errors:
            print(f"  Verification FAILED: {err}", file=sys.stderr)
        sys.exit(1)

    print("  All settings verified ✓")


def fail_on_errors(errors: list[str]) -> None:
    if not errors:
        return
    for err in errors:
        print(f"  Verification FAILED: {err}", file=sys.stderr)
    sys.exit(1)


def main() -> None:
    args = parse_args()
    path = args.path

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

    # 3. Enable target engines
    for engine_name in TARGET_ENGINES:
        status = enable_engine(data, engine_name)
        if status is EngineStatus.ENABLED:
            print(f"  Enabled {engine_name} engine ✓")
        elif status is EngineStatus.ALREADY_ENABLED:
            print(f"  {engine_name} engine already enabled ✓")
        else:
            print(f"  {engine_name} engine not found ✗", file=sys.stderr)

    # 3b. Ensure the always-empty test engine exists
    if ensure_empty_engine(data):
        print("  Added empty engine ✓")
    else:
        print("  Empty engine already present ✓")

    # 4. Verify in memory before writing; this gives dry-run and missing-engine
    # paths the same contract as the final on-disk verification.
    fail_on_errors(verify_data(data))

    if args.dry_run:
        print("  Dry run: settings not written ✓")
        return

    # 5. Write modified settings atomically
    save_settings(path, data)
    print("  Settings written ✓")

    # 6. Verify by re-reading
    verify_settings(path)


if __name__ == "__main__":
    main()
