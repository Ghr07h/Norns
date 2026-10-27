#!/usr/bin/env python3
"""Validate artifacts installed in the generic nornsd image."""

import importlib
import os
from collections.abc import Callable
from pathlib import Path
import shutil
import subprocess
import sys

NODE_VERSION = "v" + os.environ.get("NORNS_IMAGE_NODE_VERSION", "24.22.1")
CLAUDE_CODE_VERSION = os.environ.get("NORNS_IMAGE_CLAUDE_CODE_VERSION", "2.1.199")
TEMPLATE_PATH = Path("/norns/config/agent.example.yaml")
WORKSPACE_PATH = Path("/workspace")


class Verification:
    def __init__(self) -> None:
        self.passed = 0
        self.failed = 0

    def check(self, name: str, action: Callable[[], object]) -> None:
        try:
            action()
        except Exception as error:
            self.failed += 1
            print(f"[FAIL] {name}: {error}", file=sys.stderr)
        else:
            self.passed += 1

    def print_summary(self) -> None:
        print(f"image verification checks: {self.passed} passed, {self.failed} failed")


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def command_output(argv: list[str]) -> str:
    result = subprocess.run(argv, check=True, capture_output=True, text=True)
    return (result.stdout or result.stderr).strip()


def require_command(name: str) -> str:
    path = shutil.which(name)
    if not path:
        raise RuntimeError(f"missing command: {name}")
    return path


def main() -> int:
    verification = Verification()
    for name in ("python3", "node", "npm", "claude", "norns", "nornsd", "curl", "wget", "git"):
        verification.check(f"command available: {name}", lambda name=name: require_command(name))
    verification.check(
        "Node.js version",
        lambda: require(
            command_output(["node", "--version"]) == NODE_VERSION,
            f"expected {NODE_VERSION}",
        ),
    )
    verification.check(
        "Claude Code version",
        lambda: require(
            command_output(["claude", "--version"]).startswith(CLAUDE_CODE_VERSION),
            f"expected {CLAUDE_CODE_VERSION}",
        ),
    )
    verification.check("norns version", lambda: command_output(["norns", "version"]))
    verification.check("nornsd version", lambda: command_output(["nornsd", "version"]))
    verification.check("Python module: yaml", lambda: importlib.import_module("yaml"))

    def check_template() -> None:
        yaml_module = importlib.import_module("yaml")
        config = yaml_module.safe_load(TEMPLATE_PATH.read_text(encoding="utf-8"))
        require(
            isinstance(config, dict) and config.get("VERSION") == 1,
            f"invalid template: {TEMPLATE_PATH}",
        )

    verification.check("agent configuration template", check_template)
    verification.check(
        "domain directory",
        lambda: require(Path("/norns/domain").is_dir(), "missing /norns/domain"),
    )
    verification.check(
        "writable workspace",
        lambda: require(
            WORKSPACE_PATH.is_dir() and os.access(WORKSPACE_PATH, os.W_OK),
            "/workspace is missing or not writable",
        ),
    )
    verification.print_summary()
    if verification.failed:
        return 1
    print("nornsd image verification: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
