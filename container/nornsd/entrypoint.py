#!/usr/bin/env python3

import os
from pathlib import Path
import secrets
import sys

import yaml


# Runtime paths and defaults. Keep these together so the image contract is easy to inspect.
TEMPLATE_PATH = Path("/norns/config/nornsd.example.yaml")
CONFIG_PATH = Path("/norns/config/nornsd.yaml")
WORKSPACE_PATH = Path("/workspace")
TOKEN_PATH = WORKSPACE_PATH / ".nornsd-token"
LEGACY_PASSWORD_PATH = WORKSPACE_PATH / ".nornsd-password"
NORNS_BIN = Path("/norns/bin/norns")
NORNSD_BIN = Path("/norns/bin/nornsd")
CLAUDE_EXEC = Path("/norns/engine/cc/node_modules/.bin/claude")

DEFAULT_SERVER_CONCURRENCY = 1
DEFAULT_SERVER_HOST = "0.0.0.0"
DEFAULT_SERVER_PORT = 28081
DEFAULT_MAX_CONCURRENT_EXPLORATIONS = 3
DEFAULT_EXPLORATION_TIMEOUT = 1200
DEFAULT_ALLOW_HUMAN_INTERVENTION = False
DEFAULT_NORNS_ENABLE = True
DEFAULT_NORNS_EXTENDS_AGENTS_DIR = ""
DEFAULT_NORNS_CUSTOM_SKILLS_DIR = ""

REQUIRED_ENV = ("AGENT_LLM_URL", "AGENT_LLM_KEY", "AGENT_LLM_MODEL")
STRING_FIELDS = (
    "AGENT_MAINAGENT_FILE", "AGENT_REUSE_RUNTIME_DIR",
    "AGENT_LLM_URL",
    "AGENT_LLM_KEY",
    "AGENT_LLM_MODEL",
    "AGENT_LLM_FAST_MODEL",
    "SERVICE_HOST",
    "SERVICE_TOKEN",
    "AGENT_NORNS_EXTENDS_AGENTS_DIR",
    "AGENT_NORNS_CUSTOM_SKILLS_DIR",
    "AGENT_NORNS_CUSTOM_AGENTS_DIR",
    "AGENT_CUSTOM_AGENTS_DIR",
    "AGENT_CUSTOM_SKILLS_DIR",
)
INTEGER_FIELDS = (
    "SERVICE_PORT",
    "AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS",
    "AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS",
)
BOOLEAN_FIELDS = (
    "AGENT_NORNS_ENABLE",
    "AGENT_NORNS_ALLOW_HUMAN_INTERVENTION",
)


def validate_resources(config: dict) -> None:
    norns_resources = (
        "AGENT_NORNS_EXTENDS_AGENTS_DIR", "AGENT_NORNS_CUSTOM_AGENTS_DIR",
        "AGENT_NORNS_CUSTOM_SKILLS_DIR",
    )
    custom_resources = ("AGENT_CUSTOM_AGENTS_DIR", "AGENT_CUSTOM_SKILLS_DIR", "AGENT_CUSTOM_PLUGIN_DIRS")
    if config.get("AGENT_REUSE_RUNTIME_DIR"):
        if any(config.get(name) for name in (*norns_resources, *custom_resources, "AGENT_MAINAGENT_FILE")):
            raise ValueError("AGENT_REUSE_RUNTIME_DIR cannot be combined with other plugin resources or AGENT_MAINAGENT_FILE")
    elif config["AGENT_NORNS_ENABLE"]:
        if any(config.get(name) for name in custom_resources):
            raise ValueError("AGENT_CUSTOM_* resources require AGENT_NORNS_ENABLE=false")
    elif any(config.get(name) for name in norns_resources):
        raise ValueError("AGENT_NORNS_* resources require AGENT_NORNS_ENABLE=true")

    for name in (*norns_resources, "AGENT_CUSTOM_AGENTS_DIR", "AGENT_CUSTOM_SKILLS_DIR", "AGENT_REUSE_RUNTIME_DIR"):
        value = config.get(name, "")
        if value and (not Path(value).is_absolute() or not Path(value).is_dir()):
            raise ValueError(f"{name} must name an existing absolute directory: {value}")
    for value in config.get("AGENT_CUSTOM_PLUGIN_DIRS", []):
        if not Path(value).is_absolute() or not Path(value).is_dir():
            raise ValueError(f"AGENT_CUSTOM_PLUGIN_DIRS must contain existing absolute directories: {value}")
    value = config.get("AGENT_MAINAGENT_FILE", "")
    if value and (not Path(value).is_absolute() or not Path(value).is_file()):
        raise ValueError(f"AGENT_MAINAGENT_FILE must name an existing absolute file: {value}")


def parse_integer(name: str, value: str) -> int:
    try:
        return int(value, 10)
    except ValueError as error:
        raise ValueError(f"{name} must be an integer") from error


def parse_positive_integer(name: str, value: str) -> int:
    parsed = parse_integer(name, value)
    if parsed < 1:
        raise ValueError(f"{name} must be positive")
    return parsed


def parse_boolean(name: str, value: str) -> bool:
    normalized = value.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"{name} must be true or false")


def parse_string_list(name: str, value: str) -> list[str]:
    if not value.strip():
        return []
    parsed = yaml.safe_load(value)
    if isinstance(parsed, str):
        parsed = [parsed]
    if not isinstance(parsed, list) or any(not isinstance(item, str) for item in parsed):
        raise ValueError(f"{name} must be a path string or a YAML/JSON string list")
    return [item for item in parsed if item]


def server_concurrency() -> int:
    return parse_positive_integer(
        "SERVICE_CONCURRENCY",
        os.environ.get("SERVICE_CONCURRENCY", str(DEFAULT_SERVER_CONCURRENCY)),
    )


def write_server_token(token: str) -> None:
    temporary = TOKEN_PATH.with_suffix(".tmp")
    temporary.write_text(token + "\n", encoding="utf-8")
    temporary.chmod(0o600)
    temporary.replace(TOKEN_PATH)


def persisted_server_token() -> str:
    configured = os.environ.get("SERVICE_TOKEN", "").strip()
    if configured:
        write_server_token(configured)
        return configured

    for path in (TOKEN_PATH, LEGACY_PASSWORD_PATH):
        if not path.exists():
            continue
        token = path.read_text(encoding="utf-8").strip()
        if not token:
            raise ValueError(f"{path} is empty")
        if path == LEGACY_PASSWORD_PATH:
            write_server_token(token)
        return token

    token = secrets.token_urlsafe(32)
    write_server_token(token)
    print(f"nornsd: generated the server token at {TOKEN_PATH}", file=sys.stderr)
    return token


def load_runtime_config() -> dict:
    missing = [name for name in REQUIRED_ENV if not os.environ.get(name, "").strip()]
    if missing:
        raise ValueError(f"missing required environment variables: {', '.join(missing)}")

    with TEMPLATE_PATH.open("r", encoding="utf-8") as stream:
        config = yaml.safe_load(stream) or {}
    if not isinstance(config, dict):
        raise ValueError(f"{TEMPLATE_PATH} must contain a YAML mapping")

    config.update({
        "SERVICE_NORNSD_ROOT_DIR": str(WORKSPACE_PATH),
        "SERVICE_CONCURRENCY": server_concurrency(),
        "AGENT_NORNS_ENABLE": DEFAULT_NORNS_ENABLE,
        "SERVICE_HOST": DEFAULT_SERVER_HOST,
        "SERVICE_PORT": DEFAULT_SERVER_PORT,
        "AGENT_NORNS_MAX_CONCURRENT_EXPLORATIONS": DEFAULT_MAX_CONCURRENT_EXPLORATIONS,
        "AGENT_NORNS_EXPLORATION_TIMEOUT_SECONDS": DEFAULT_EXPLORATION_TIMEOUT,
        "AGENT_NORNS_ALLOW_HUMAN_INTERVENTION": DEFAULT_ALLOW_HUMAN_INTERVENTION,
        "AGENT_NORNS_EXTENDS_AGENTS_DIR": DEFAULT_NORNS_EXTENDS_AGENTS_DIR,
        "AGENT_NORNS_CUSTOM_SKILLS_DIR": DEFAULT_NORNS_CUSTOM_SKILLS_DIR,
        "AGENT_NORNS_CUSTOM_AGENTS_DIR": "",
        "AGENT_CUSTOM_AGENTS_DIR": "",
        "AGENT_CUSTOM_SKILLS_DIR": "",
        "AGENT_CUSTOM_PLUGIN_DIRS": [],
    })

    # Image defaults must not introduce plugin resources in plain or reuse mode.
    enabled = parse_boolean("AGENT_NORNS_ENABLE", os.environ.get("AGENT_NORNS_ENABLE", str(DEFAULT_NORNS_ENABLE)))
    if not enabled or os.environ.get("AGENT_REUSE_RUNTIME_DIR"):
        config["AGENT_NORNS_EXTENDS_AGENTS_DIR"] = ""
        config["AGENT_NORNS_CUSTOM_SKILLS_DIR"] = ""

    for name in STRING_FIELDS:
        if name in os.environ:
            config[name] = os.environ[name]
    for name in INTEGER_FIELDS:
        if name in os.environ:
            config[name] = parse_positive_integer(name, os.environ[name])
    for name in BOOLEAN_FIELDS:
        if name in os.environ:
            config[name] = parse_boolean(name, os.environ[name])
    if "AGENT_CUSTOM_PLUGIN_DIRS" in os.environ:
        config["AGENT_CUSTOM_PLUGIN_DIRS"] = parse_string_list(
            "AGENT_CUSTOM_PLUGIN_DIRS", os.environ["AGENT_CUSTOM_PLUGIN_DIRS"])

    config["AGENT_LLM_FAST_MODEL"] = os.environ.get("AGENT_LLM_FAST_MODEL", "").strip() or config["AGENT_LLM_MODEL"]
    config["SERVICE_TOKEN"] = persisted_server_token()
    config["AGENT_EXEC"] = [str(CLAUDE_EXEC)]
    config["AGENT_MAINAGENT_FILE"] = config.get("AGENT_MAINAGENT_FILE", "")
    config["AGENT_REUSE_RUNTIME_DIR"] = config.get("AGENT_REUSE_RUNTIME_DIR", "")
    return config


def validate_runtime(config: dict) -> None:
    for name, path in (("claude", CLAUDE_EXEC), ("norns", NORNS_BIN), ("nornsd", NORNSD_BIN)):
        if not path.is_file() or not os.access(path, os.X_OK):
            raise ValueError(f"{name} is not executable: {path}")

    validate_resources(config)


def write_runtime_config(config: dict) -> None:
    temporary = CONFIG_PATH.with_suffix(".yaml.tmp")
    with temporary.open("w", encoding="utf-8") as stream:
        yaml.safe_dump(config, stream, allow_unicode=True, sort_keys=False)
    temporary.chmod(0o600)
    temporary.replace(CONFIG_PATH)


def main() -> int:
    if len(sys.argv) != 1:
        print("nornsd: this image does not accept command-line arguments", file=sys.stderr)
        return 2
    try:
        WORKSPACE_PATH.mkdir(parents=True, exist_ok=True)
        CONFIG_PATH.parent.mkdir(parents=True, exist_ok=True)
        config = load_runtime_config()
        validate_runtime(config)
        write_runtime_config(config)
    except (OSError, ValueError, yaml.YAMLError) as error:
        print(f"nornsd: container configuration failed: {error}", file=sys.stderr)
        return 2

    argv = [str(NORNSD_BIN), "server", "--configs", str(CONFIG_PATH)]
    os.execv(argv[0], argv)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
