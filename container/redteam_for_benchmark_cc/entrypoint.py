#!/usr/bin/env python3
"""Start one three-slot plain-Claude nornsd and the single-model FIFO benchmark."""
from __future__ import annotations

import json
import os
import secrets
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

# yaml 仅在写 agent.yaml 时需要，延后导入，便于无 PyYAML 环境下导入本模块做单测。
yaml = None  # type: ignore

# Runtime paths and defaults. Keep these together so the image contract is easy to inspect.
TEMPLATE_PATH = Path("/norns/config/agent.example.yaml")
CONFIG_PATH = Path("/norns/config/agent.yaml")
WORKSPACE = Path("/workspace")
TOKEN_PATH = WORKSPACE / ".nornsd-token"
LEGACY_PASSWORD_PATH = WORKSPACE / ".nornsd-password"
CLAUDE_EXEC = Path("/norns/engine/cc/node_modules/.bin/claude")
NORNS_BIN = Path("/norns/bin/norns")
NORNSD_BIN = Path("/norns/bin/nornsd")
BENCHMARK_BIN = Path("/norns/bin/benchmark.py")
REDTEAM_SKILLS_DIR = "/norns/domain/redteam/skills"

DEFAULT_SERVER_CONCURRENCY = 3
DEFAULT_SERVER_HOST = "0.0.0.0"
DEFAULT_SERVER_PORT = 28081
DEFAULT_MAX_CONCURRENT_EXPLORATIONS = 3
DEFAULT_EXPLORATION_TIMEOUT = 1200
DEFAULT_ALLOW_HUMAN_INTERVENTION = False
DEFAULT_NORNS_ENABLE = False

_benchmark_proc: subprocess.Popen | None = None
_received_signal: int | None = None

REQUIRED_ENV = (
    "LLM_URL", "LLM_KEY", "LLM_MODEL",
    "BENCHMARK_BASE_URL", "BENCHMARK_TOKEN",
)

STRING_FIELDS = (
    "LLM_URL", "LLM_KEY", "LLM_MODEL", "LLM_FAST_MODEL",
    "NORNS_SERVER_HOST", "NORNS_SERVER_TOKEN",
    "CUSTOM_AGENTS_DIR", "CUSTOM_SKILLS_DIR",
)
INTEGER_FIELDS = (
    "NORNS_SERVER_PORT",
)
DIRECTORY_FIELDS = (
    "CUSTOM_AGENTS_DIR", "CUSTOM_SKILLS_DIR",
)


def log(message: str) -> None:
    print(f"[entrypoint] {message}", flush=True)


def warn(message: str) -> None:
    print(f"[entrypoint] {message}", file=sys.stderr, flush=True)


def parse_boolean(name: str, value: str) -> bool:
    normalized = value.strip().lower()
    if normalized in {"1", "true", "yes", "on"}:
        return True
    if normalized in {"0", "false", "no", "off"}:
        return False
    raise ValueError(f"{name} 必须为 true 或 false，收到 {value!r}")


def parse_positive_integer(name: str, value: str) -> int:
    try:
        parsed = int(value, 10)
    except ValueError as error:
        raise ValueError(f"{name} 必须为整数，收到 {value!r}") from error
    if parsed < 1:
        raise ValueError(f"{name} 必须为正整数，收到 {value!r}")
    return parsed


def parse_string_list(name: str, value: str) -> list[str]:
    global yaml
    if not value.strip():
        return []
    if yaml is None:
        import yaml as _yaml
        yaml = _yaml
    parsed = yaml.safe_load(value)
    if isinstance(parsed, str):
        parsed = [parsed]
    if not isinstance(parsed, list) or any(not isinstance(item, str) for item in parsed):
        raise ValueError(f"{name} 必须为路径字符串或 YAML/JSON 字符串列表")
    return [item for item in parsed if item]


def write_token_file(token: str) -> None:
    temporary = TOKEN_PATH.with_suffix(".tmp")
    temporary.write_text(token + "\n", encoding="utf-8")
    temporary.chmod(0o600)
    temporary.replace(TOKEN_PATH)


def persisted_token() -> str:
    """Load or create the token shared by nornsd and benchmark.py."""
    configured = os.environ.get("NORNS_SERVER_TOKEN", "").strip()
    if configured:
        write_token_file(configured)
        return configured
    for path in (TOKEN_PATH, LEGACY_PASSWORD_PATH):
        try:
            value = path.read_text(encoding="utf-8").strip()
        except OSError:
            continue
        if value:
            if path == LEGACY_PASSWORD_PATH:
                write_token_file(value)
            return value
    value = secrets.token_urlsafe(32)
    write_token_file(value)
    return value


def parse_models() -> list[str]:
    raw = os.environ.get("LLM_MODEL", "")
    models = [item.strip() for item in raw.split(",") if item.strip()]
    if not models:
        warn("未通过 LLM_MODEL 传入模型")
        raise SystemExit(2)
    if len(models) != 1:
        warn("LLM_MODEL 必须且只能包含一个模型名称")
        raise SystemExit(2)
    return models


def write_agent_config(model: str, cfg_path: Path, token: str) -> dict:
    """Write a plain-Claude configuration with the redteam Skill enabled."""
    global yaml
    if yaml is None:
        import yaml as _yaml
        yaml = _yaml

    if not TEMPLATE_PATH.exists():
        raise SystemExit(f"缺少 agent 模板: {TEMPLATE_PATH}")
    config = yaml.safe_load(TEMPLATE_PATH.read_text(encoding="utf-8")) or {}
    if not isinstance(config, dict):
        raise SystemExit(f"{TEMPLATE_PATH} 必须是 YAML mapping")

    config.update({
        "NORNS_ENABLE": DEFAULT_NORNS_ENABLE,
        "NORNS_SERVER_HOST": DEFAULT_SERVER_HOST,
        "NORNS_SERVER_PORT": DEFAULT_SERVER_PORT,
        "NORNS_SERVER_TOKEN": token,
        "NORNS_MAX_CONCURRENT_EXPLORATIONS": DEFAULT_MAX_CONCURRENT_EXPLORATIONS,
        "NORNS_EXPLORATION_TIMEOUT": DEFAULT_EXPLORATION_TIMEOUT,
        "NORNS_ALLOW_HUMAN_INTERVENTION": DEFAULT_ALLOW_HUMAN_INTERVENTION,
        "NORNS_CUSTOM_DOMAIN_DIR": "",
        "NORNS_CUSTOM_SKILLS_DIR": "",
        "NORNS_CUSTOM_AGENTS_DIR": "",
        "CUSTOM_AGENTS_DIR": "",
        "CUSTOM_SKILLS_DIR": REDTEAM_SKILLS_DIR,
        "CUSTOM_PLUGIN_DIRS": [],
    })
    for name in STRING_FIELDS:
        if name in os.environ:
            config[name] = os.environ[name]
    for name in INTEGER_FIELDS:
        if name in os.environ:
            config[name] = parse_positive_integer(name, os.environ[name])
    if "CUSTOM_PLUGIN_DIRS" in os.environ:
        config["CUSTOM_PLUGIN_DIRS"] = parse_string_list(
            "CUSTOM_PLUGIN_DIRS", os.environ["CUSTOM_PLUGIN_DIRS"])
    config["LLM_MODEL"] = model
    config["LLM_FAST_MODEL"] = os.environ.get("LLM_FAST_MODEL", "").strip() or model
    config["NORNS_SERVER_TOKEN"] = token
    # 对照组语义是固定条件，不能由环境变量切回 Norns Profile。
    config["NORNS_ENABLE"] = False
    config["NORNS_CUSTOM_DOMAIN_DIR"] = ""
    config["NORNS_CUSTOM_SKILLS_DIR"] = ""
    config["NORNS_CUSTOM_AGENTS_DIR"] = ""
    config["AGENT_EXEC"] = [str(CLAUDE_EXEC)]

    cfg_path.parent.mkdir(parents=True, exist_ok=True)
    temporary = cfg_path.with_suffix(".yaml.tmp")
    temporary.write_text(
        yaml.safe_dump(config, allow_unicode=True, sort_keys=False),
        encoding="utf-8")
    temporary.chmod(0o600)
    temporary.replace(cfg_path)
    return config


def validate_runtime(config: dict) -> None:
    for name, path in (("claude", CLAUDE_EXEC), ("norns", NORNS_BIN), ("nornsd", NORNSD_BIN)):
        if not path.is_file() or not os.access(path, os.X_OK):
            raise ValueError(f"{name} 不可执行: {path}")
    for name in DIRECTORY_FIELDS:
        value = str(config.get(name, "")).strip()
        if value and not Path(value).is_dir():
            raise ValueError(f"{name} 目录不存在: {value}")
    for value in config.get("CUSTOM_PLUGIN_DIRS", []):
        if not Path(value).is_dir():
            raise ValueError(f"CUSTOM_PLUGIN_DIRS 目录不存在: {value}")


def wait_healthy(proc: subprocess.Popen, base: str, deadline: float, token: str) -> tuple[bool, str]:
    # --host 0.0.0.0(非 loopback)+ --token 让 nornsd 对所有端点(含 /api/v1/health)强制
    # Bearer 鉴权(那条 "serving a Bearer token over plaintext HTTP on a non-loopback
    # address" warning 即此)。不带 Authorization 头则 health 恒 401 → wait_healthy 死等
    # 到 ENTRYPOINT_HEALTH_WAIT 超时。故探活必须带
    # Authorization: Bearer <token>，与启动 nornsd 和 benchmark.py 使用同一持久 token。
    while time.monotonic() < deadline:
        if _received_signal is not None:
            return False, f"收到停止信号({_received_signal})"
        if proc.poll() is not None:
            return False, f"nornsd 进程已退出(code={proc.returncode})"
        request = urllib.request.Request(base + "/api/v1/health")
        request.add_header("Authorization", f"Bearer {token}")
        try:
            with urllib.request.urlopen(request, timeout=3) as response:
                if getattr(response, "status", 200) == 200:
                    return True, ""
        except (urllib.error.URLError, OSError):
            pass
        time.sleep(2.0)
    return False, f"nornsd 健康检查超时({base})"


def start_nornsd(workspace: Path, settings: Path, port: int, token: str) -> subprocess.Popen:
    del port, token
    argv = [
        str(NORNSD_BIN), "server",
        "--workspace-root", str(workspace),
        "--settings", str(settings),
        "--concurrency", str(DEFAULT_SERVER_CONCURRENCY),
    ]
    # nornsd 的 slog 会把每条 http request(benchmark 轮询事件、抽 flag、提交、健康探活)
    # 全行打到 stderr,量极大;若直接接回 terminal,会淹没 entrypoint/benchmark 自己的输出
    # (用户看不到调度日志)。故把 nornsd 的 stdout+stderr 重定向到工作区下的
    # nornsd-cc.log,terminal 只留 entrypoint/benchmark 的输出;nornsd 异常时再 tail 它的
    # 日志行出来定位(本会话两次都是靠 nornsd stderr 才定位到 PATH 缺失与 health 401)。
    log_path = workspace / "nornsd-cc.log"
    log_handle = open(log_path, "wb", buffering=0)
    log("启动 nornsd: " + " ".join(argv) + f"  (日志 -> {log_path})")
    try:
        proc = subprocess.Popen(argv, stdout=log_handle, stderr=subprocess.STDOUT)
    except Exception:
        log_handle.close()
        raise
    proc.log_handle = log_handle  # type: ignore[attr-defined]
    proc.log_path = log_path      # type: ignore[attr-defined]
    return proc


def stop_nornsd(proc: subprocess.Popen, wait_seconds: float) -> None:
    if proc.poll() is not None:
        return
    log("停止 nornsd(SIGTERM)")
    try:
        proc.terminate()
    except ProcessLookupError:
        return
    try:
        proc.wait(wait_seconds)
        return
    except subprocess.TimeoutExpired:
        pass
    log("SIGTERM 超时，升级 SIGKILL")
    try:
        proc.kill()
        proc.wait(15)
    except Exception as error:  # pragma: no cover
        warn(f"SIGKILL 失败: {error}")


def _tail_log(path: Path | None, n: int = 20) -> list[str]:
    """读取 nornsd 日志末尾 n 行。

    nornsd 起不来/不健康时,把它的日志末尾打到 stderr,定位"为什么没起来":
    PATH 缺失、端口占用、agent.yaml 解析失败等都在这里(本会话两次定位靠的就是它)。
    """
    if path is None:
        return []
    try:
        with open(path, "rb") as handle:
            data = handle.read()
    except OSError:
        return []
    return data.decode("utf-8", "replace").splitlines()[-n:]


def _close_log_handle(proc: subprocess.Popen) -> None:
    """关闭 start_nornsd 为该 proc 打开的日志句柄。"""
    handle = getattr(proc, "log_handle", None)
    if handle is None:
        return
    try:
        handle.close()
    except OSError:
        pass


def _env_float_or(name: str, default: float) -> float:
    value = os.environ.get(name)
    if not value:
        return default
    try:
        return float(value)
    except ValueError:
        raise SystemExit(f"环境变量 {name} 不是合法数字: {value!r}")


def run_benchmark(specs: list, token: str, initial: float, increment: float, ceiling: float,
                  run_id: str) -> int:
    """Run benchmark.py once against the single healthy nornsd."""
    if len(specs) != 1:
        raise ValueError("benchmark requires exactly one healthy nornsd")
    env = dict(os.environ)
    env["BENCHMARK_NORNSD_BASE_URLS"] = ",".join(spec["base_url"] for spec in specs)
    env["BENCHMARK_NORNSD_TOKEN"] = token
    env["LLM_MODEL"] = ",".join(spec["model"] for spec in specs)
    env["BENCHMARK_INITIAL_LIMIT_MINUTES"] = str(initial)
    env["BENCHMARK_LIMIT_INCREMENT_MINUTES"] = str(increment)
    env["BENCHMARK_MAX_CHALLENGE_TIMEOUT_MINUTES"] = str(ceiling)
    env["BENCHMARK_RUN_ID"] = run_id
    # 旧设计(外层轮×内层模型 + 3 并发池)相关变量必须清除，避免 benchmark.py 误读。
    env.pop("BENCHMARK_CONCURRENCY", None)
    env.pop("BENCHMARK_MODEL_LABEL", None)
    env.pop("BENCHMARK_PER_CHALLENGE_TIMEOUT", None)
    env.pop("BENCHMARK_MAX_PASSES", None)
    env.pop("NORNSD_BASE_URL", None)
    env.pop("NORNSD_TOKEN", None)
    log(f"运行 benchmark.py: 初始单题限时={initial}min "
        f"递增={increment}min 封顶={ceiling}min model={env['LLM_MODEL']}")
    global _benchmark_proc
    proc = subprocess.Popen([sys.executable, str(BENCHMARK_BIN)], env=env)
    _benchmark_proc = proc
    try:
        returncode = proc.wait()
    finally:
        _benchmark_proc = None
    log(f"benchmark.py 退出 code={returncode}")
    return returncode


def _handle_signal(signum: int, _frame) -> None:
    """把容器停止信号转发给 benchmark；nornsd 由 main 的 finally 统一回收。"""
    global _received_signal
    _received_signal = signum
    proc = _benchmark_proc
    if proc is not None and proc.poll() is None:
        try:
            proc.terminate()
        except ProcessLookupError:
            pass


def read_result(path: Path) -> dict | None:
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except (OSError, ValueError):
        return None


def build_specs(models: list, token: str) -> list:
    """Build the one nornsd specification expected by the current scheduler."""
    specs = []
    for i, model in enumerate(models):
        n = i + 1
        port = DEFAULT_SERVER_PORT + i
        workspace = WORKSPACE
        cfg_path = CONFIG_PATH
        workspace.mkdir(parents=True, exist_ok=True)
        config = write_agent_config(model, cfg_path, token)
        validate_runtime(config)
        specs.append({
            "model": model, "index": n, "workspace": workspace,
            "config": cfg_path, "port": port,
            "base_url": f"http://127.0.0.1:{config['NORNS_SERVER_PORT']}",
        })
    return specs


def main() -> int:
    global _received_signal
    _received_signal = None
    # --test(仅本地试跑用)：进程内把 env BENCHMARK_TEST_ENV=1 透传给 benchmark.py，
    # 由其把"临时环境/无 pentest-tools,基于当前环境解题"追加到提示词末尾。
    # 真实/官方调用不传 --test 即为关闭：给 benchmark.py 的子进程参数恒为
    # [python3, /norns/bin/benchmark.py]，从不携带 --test，提示词保持原样。
    if "--test" in sys.argv:
        os.environ["BENCHMARK_TEST_ENV"] = "1"
        log("启用 --test：提示词末尾加注临时环境/无 pentest-tools(仅本地试跑用)")
    missing = [name for name in REQUIRED_ENV if not os.environ.get(name, "").strip()]
    if missing:
        warn(f"缺少必填环境变量: {', '.join(missing)}")
        return 2

    models = parse_models()
    WORKSPACE.mkdir(parents=True, exist_ok=True)

    initial = _env_float_or("BENCHMARK_INITIAL_LIMIT_MINUTES", 40.0)
    increment = _env_float_or("BENCHMARK_LIMIT_INCREMENT_MINUTES", 30.0)
    ceiling = _env_float_or("BENCHMARK_MAX_CHALLENGE_TIMEOUT_MINUTES", 240.0)
    health_wait = _env_float_or("ENTRYPOINT_HEALTH_WAIT", 180.0)
    stop_wait = _env_float_or("ENTRYPOINT_STOP_WAIT", 45.0)
    token = persisted_token()
    result_file = Path(os.environ.get(
        "BENCHMARK_RESULT_FILE", "/workspace/benchmark-cc-result.json"))
    run_id = uuid.uuid4().hex
    try:
        if result_file.exists():
            archived = result_file.with_name(f"{result_file.name}.previous-{run_id}")
            result_file.replace(archived)
            log(f"归档上次结果: {archived}")
    except OSError as error:
        warn(f"无法归档旧结果文件 {result_file}: {error}")
        return 1

    log(f"模式: 普通 Claude Code; 模型: {models[0]}; 端口 {DEFAULT_SERVER_PORT}; "
        f"nornsd concurrency={DEFAULT_SERVER_CONCURRENCY}; "
        f"单题限时初始={initial}min 递增={increment}min 封顶={ceiling}min; 共享 token 已就绪")

    started: list = []  # (spec, proc)
    healthy_specs: list = []
    benchmark_rc = 1
    previous_handlers = {}
    for sig in (signal.SIGTERM, signal.SIGINT):
        previous_handlers[sig] = signal.signal(sig, _handle_signal)
    try:
        try:
            specs = build_specs(models, token)
        except (OSError, ValueError, yaml.YAMLError) as error:
            warn(f"容器运行配置校验失败: {error}")
            return 2
        # Start the single nornsd and verify it before launching the benchmark.
        for spec in specs:
            proc = start_nornsd(spec["workspace"], spec["config"], spec["port"], token)
            started.append((spec, proc))
            ok, reason = wait_healthy(proc, spec["base_url"], time.monotonic() + health_wait, token)
            if _received_signal is not None:
                warn(f"启动阶段收到信号 {_received_signal}，停止继续启动")
                return 128 + _received_signal
            if ok:
                healthy_specs.append(spec)
                log(f"nornsd {spec['model']} 健康于 {spec['base_url']}")
            else:
                warn(f"nornsd {spec['model']}({spec['base_url']}) 不健康: {reason}")
                path = getattr(proc, "log_path", None)
                tail = _tail_log(path)
                if tail:
                    warn(f"  —— {path} 末尾 ——")
                    for line in tail:
                        warn(f"  {line}")

        if not healthy_specs:
            warn("nornsd 不健康，benchmark 不启动")
            return 1
        benchmark_rc = run_benchmark(healthy_specs, token, initial, increment, ceiling, run_id)
    finally:
        log("停止 nornsd")
        for spec, proc in started:
            try:
                stop_nornsd(proc, stop_wait)
            except Exception as error:  # pragma: no cover
                warn(f"停止 nornsd {spec['model']} 失败(忽略): {error}")
            _close_log_handle(proc)
        for sig, handler in previous_handlers.items():
            signal.signal(sig, handler)

    if _received_signal is not None:
        warn(f"收到信号 {_received_signal}，benchmark 已停止")
        return 128 + _received_signal
    if benchmark_rc != 0:
        warn(f"benchmark.py 非正常退出(code={benchmark_rc})")
        return benchmark_rc if benchmark_rc > 0 else 1

    final = read_result(result_file)
    if not isinstance(final, dict):
        warn("未产生有效结果文件")
        return 1
    if final.get("run_id") != run_id:
        warn("结果文件不属于本次运行(run_id 不匹配)")
        return 1
    completed = final.get("completed", 0)
    total = final.get("total", 0)
    log(f"完成: 题目 {completed}/{total}"
        f"{', global_stopped' if final.get('global_stopped') else ''};"
        f" 结果文件 {result_file}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
