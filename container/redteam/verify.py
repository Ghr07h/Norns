#!/usr/bin/env python3
"""Validate artifacts installed in the redteam nornsd image."""

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
REQUIRED_COMMANDS = (
    "python3", "node", "npm", "claude", "norns", "nornsd",
    "curl", "wget", "git", "jq", "rg", "tmux", "nc", "socat",
    "nmap", "sqlmap", "hydra", "john", "dirb", "gobuster", "nikto",
    "nuclei", "katana", "naabu", "ffuf", "dalfox", "chromium", "playwright-cli",
    "gdb", "strace", "as", "ld", "objdump", "readelf", "nm", "strings",
    "radare2", "r2", "rabin2", "ROPgadget",
)
REQUIRED_MODULES = (
    "yaml", "pwnlib", "pymongo", "pyftpdlib", "paramiko", "cryptography",
    "dns", "bs4", "requests", "dirsearch",
    "capstone", "ropgadget",
)
REVERSE_TOOL_VERSION_COMMANDS = (
    ("gdb", "--version"), ("strace", "-V"), ("ld", "--version"),
    ("radare2", "-v"), ("ROPgadget", "--version"),
)
REQUIRED_DOMAIN_PATHS = (
    "/norns/domain/redteam/norns_domain_agents/meta.json",
    "/norns/domain/redteam/norns_domain_agents/decider.md",
    "/norns/domain/redteam/norns_domain_agents/explorer.md",
    "/norns/domain/redteam/skills/pentest-tools/SKILL.md",
)
# 词表校验为非空(而非仅存在):curl -o 在下载半途失败时可能留下 0 字节文件。
# john/nmap 两份是 apt 包自带(硬依赖)，其余为 Dockerfile 中下载。
REQUIRED_WORDLISTS = (
    "/usr/share/wordlists/common.txt",
    "/usr/share/wordlists/raft-medium-directories.txt",
    "/usr/share/wordlists/top-10000-passwords.txt",
    "/usr/share/wordlists/top1000.txt",
    "/usr/share/john/password.lst",
    "/usr/share/nmap/nselib/data/passwords.lst",
)


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


def import_module(name: str) -> None:
    if name == "pwnlib":
        os.environ["PWNLIB_NOTERM"] = "1"
    importlib.import_module(name)


def check_capstone() -> None:
    capstone = importlib.import_module("capstone")
    disassembler = capstone.Cs(capstone.CS_ARCH_X86, capstone.CS_MODE_64)
    mnemonics = [instruction.mnemonic for instruction in disassembler.disasm(b"\x90\xc3", 0x1000)]
    require(mnemonics == ["nop", "ret"], f"Capstone disassembly failed: {mnemonics}")


def main() -> int:
    verification = Verification()
    for name in REQUIRED_COMMANDS:
        verification.check(
            f"command available: {name}",
            lambda name=name: require(shutil.which(name) is not None, f"missing command: {name}"),
        )
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
    # playwright 库经 npm 全局安装，经 npx 走一遍完整解析路径。
    # --no-install 防止库缺失时 npx 临时从 registry 拉包，把问题掩盖成"看起来正常"。
    verification.check(
        "playwright library via npx",
        lambda: command_output(["npx", "--no-install", "playwright", "--version"]),
    )
    for name in REQUIRED_MODULES:
        verification.check(f"Python module: {name}", lambda name=name: import_module(name))
    for argv in REVERSE_TOOL_VERSION_COMMANDS:
        verification.check(
            f"tool version: {argv[0]}",
            lambda argv=argv: command_output(list(argv)),
        )
    verification.check("Capstone disassembly", check_capstone)

    def check_template() -> None:
        yaml_module = importlib.import_module("yaml")
        config = yaml_module.safe_load(TEMPLATE_PATH.read_text(encoding="utf-8"))
        require(
            isinstance(config, dict) and config.get("VERSION") == 1,
            f"invalid template: {TEMPLATE_PATH}",
        )

    verification.check("agent configuration template", check_template)
    for path in REQUIRED_DOMAIN_PATHS:
        verification.check(
            f"required file: {path}",
            lambda path=path: require(Path(path).is_file(), f"missing file: {path}"),
        )
    for path in REQUIRED_WORDLISTS:
        verification.check(
            f"wordlist: {path}",
            lambda path=path: require(
                Path(path).is_file() and Path(path).stat().st_size > 0,
                f"missing or empty wordlist: {path}",
            ),
        )
    workspace = Path("/workspace")
    verification.check(
        "writable workspace",
        lambda: require(
            workspace.is_dir() and os.access(workspace, os.W_OK),
            "/workspace is missing or not writable",
        ),
    )
    verification.print_summary()
    if verification.failed:
        return 1
    print("redteam nornsd image verification: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
