#!/usr/bin/env python3
"""Validate artifacts installed in the generic nornsd image."""

import importlib
import os
from pathlib import Path
import shutil
import subprocess
import sys

import yaml


NODE_VERSION = "v" + os.environ.get("NORNS_IMAGE_NODE_VERSION", "24.22.1")
CLAUDE_CODE_VERSION = os.environ.get("NORNS_IMAGE_CLAUDE_CODE_VERSION", "2.1.199")
TEMPLATE_PATH = Path("/norns/config/agent.example.yaml")
WORKSPACE_PATH = Path("/workspace")


def fail(message: str) -> None:
    print(f"[FAIL] {message}", file=sys.stderr)
    raise SystemExit(1)


def command_output(argv: list[str]) -> str:
    try:
        result = subprocess.run(argv, check=True, capture_output=True, text=True)
    except (OSError, subprocess.CalledProcessError) as error:
        fail(f"command failed: {' '.join(argv)}: {error}")
    return (result.stdout or result.stderr).strip()


def require_command(name: str) -> str:
    path = shutil.which(name)
    if not path:
        fail(f"missing command: {name}")
    return path


def main() -> int:
    for name in ("python3", "node", "npm", "claude", "norns", "nornsd", "curl", "wget", "git"):
        require_command(name)
    if command_output(["node", "--version"]) != NODE_VERSION:
        fail(f"unexpected Node.js version; expected {NODE_VERSION}")
    if not command_output(["claude", "--version"]).startswith(CLAUDE_CODE_VERSION):
        fail(f"unexpected Claude Code version; expected {CLAUDE_CODE_VERSION}")
    command_output(["norns", "version"])
    command_output(["nornsd", "version"])
    importlib.import_module("yaml")

    config = yaml.safe_load(TEMPLATE_PATH.read_text(encoding="utf-8"))
    if not isinstance(config, dict) or config.get("VERSION") != 1:
        fail(f"invalid template: {TEMPLATE_PATH}")
    if not Path("/norns/domain").is_dir():
        fail("missing /norns/domain")
    if not WORKSPACE_PATH.is_dir() or not os.access(WORKSPACE_PATH, os.W_OK):
        fail("/workspace is missing or not writable")
    print("nornsd image verification: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
