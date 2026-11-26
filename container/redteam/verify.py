#!/usr/bin/env python3
"""Validate artifacts installed in the redteam server image."""

import importlib
import json
import os
import platform
from collections.abc import Callable
from pathlib import Path
import py_compile
import shutil
import subprocess
import sys

NODE_VERSION = "v" + os.environ.get("NORNS_IMAGE_NODE_VERSION", "24.22.1")
CLAUDE_CODE_VERSION = os.environ.get("NORNS_IMAGE_CLAUDE_CODE_VERSION", "2.1.199")
TEMPLATE_PATH = Path("/norns/config/nornsd.example.yaml")
REQUIRED_COMMANDS = (
    "python3", "node", "npm", "claude", "norns", "nornsd",
    "curl", "wget", "git", "jq", "rg", "tmux", "nc", "socat",
    "nmap", "sqlmap", "hydra", "john", "dirb", "gobuster", "nikto",
    "nuclei", "katana", "naabu", "ffuf", "dalfox", "chromium", "playwright-cli",
    "gdb", "strace", "as", "ld", "objdump", "readelf", "nm", "strings",
    "radare2", "r2", "rabin2", "ROPgadget",
    "steghide", "exiftool", "binwalk",
    "mysql", "psql", "redis-cli",
    "xxd", "ps", "killall", "lsof",
    "sqlite3", "time", "ltrace", "tracepath", "iptables", "zip", "xmllint",
    # —— 五向补齐(对齐 Dockerfile 各层) ——
    "java", "javac", "java8", "java11", "java21", "javac8", "javac11", "javac21",
    "mvn", "mvn8", "mvn11", "mvn21",
    "gdb-multiarch", "patchelf", "tesseract",
    "jadx", "apktool", "jdumpspider",
    "httpx", "subfinder",
    "7z", "tree", "rlwrap", "fcrackzip", "foremost",
    # —— 协议枚举与 PHP 复现(php-cli/snmp/ldap-utils, apt 硬依赖) ——
    "php", "snmpwalk", "ldapsearch",
    # —— 全量硬装时代: Web 指纹/WAF/云/隧道/内网/反序列化/离线漏洞库 ——
    "whatweb", "wafw00f", "cloudfox", "chisel", "aliyun",
    "bloodyAD", "enum4linux-ng", "phpggc", "jsfinder", "searchsploit",
    "rogue-jndi",
    # pwndbg 不在此列: 仓库 2026.07 布局不再提供 bin/ 独立启动器, 镜像内经
    # /root/.gdbinit 挂入 gdb/gdb-multiarch, 由 check_pwndbg 校验挂钩。
    # —— impacket 套件核心脚本(/usr/local/bin 下的 XXX.py 形式) ——
    "secretsdump.py", "wmiexec.py", "psexec.py", "mssqlclient.py",
    "GetUserSPNs.py", "getTGT.py", "smbclient.py",
)
REQUIRED_MODULES = (
    "yaml", "pwnlib", "pymongo", "pyftpdlib", "paramiko", "cryptography",
    "dns", "bs4", "requests", "dirsearch",
    "capstone", "ropgadget", "z3", "numpy",
    "jwt", "sympy", "gmpy2", "Crypto", "unicorn",
    "elftools", "keystone", "PIL", "ldap3",
    "aiohttp", "pypdf", "websockets", "websocket", "pexpect", "numba",
)
REVERSE_TOOL_VERSION_COMMANDS = (
    ("gdb", "--version"), ("strace", "-V"), ("ld", "--version"),
    ("radare2", "-v"), ("ROPgadget", "--version"),
    ("steghide", "--version"), ("exiftool", "-ver"),
    ("mysql", "--version"), ("psql", "--version"),
    ("redis-cli", "--version"),
    ("gdb-multiarch", "--version"), ("patchelf", "--version"),
    ("tesseract", "--version"),
    ("php", "--version"),
)
REQUIRED_DOMAIN_PATHS = (
    "/norns/domain/redteam/extend_agents/meta.json",
    "/norns/domain/redteam/extend_agents/decider.md",
    "/norns/domain/redteam/extend_agents/explorer.md",
    "/norns/domain/redteam/custom_skills/pentest-tools/SKILL.md",
)
# 词表校验为非空(而非仅存在):curl -o 在下载半途失败时可能留下 0 字节文件。
# john/nmap 两份是 apt 包自带(硬依赖)，其余为 Dockerfile 中下载。
REQUIRED_WORDLISTS = (
    "/usr/share/wordlists/common.txt",
    "/usr/share/wordlists/raft-medium-directories.txt",
    "/usr/share/wordlists/raft-medium-files.txt",
    "/usr/share/wordlists/big.txt",
    "/usr/share/wordlists/top-10000-passwords.txt",
    "/usr/share/wordlists/top1000.txt",
    "/usr/share/wordlists/unix_users.txt",
    "/usr/share/john/password.lst",
    "/usr/share/nmap/nselib/data/passwords.lst",
)
# Java 利用附件。用 zip 魔数而非仅存在性校验: wget 半途失败同样会留下残缺文件。
# rogue-jndi 为现场构建 shade jar(可经 MAVEN_MIRROR_URL 走国内镜像, 约 6MB);
# JDumpSpider 预编译 fat jar 上游体积实测 335,838 字节——阈值必须放给它, 否则完好的
# 也会被当残缺判死。均为硬依赖, 体积/魔数不达标即断镜像。
REQUIRED_JARS = (
    ("/opt/JDumpSpider/JDumpSpider.jar", 300_000),
    ("/opt/rogue-jndi/rogue-jndi.jar", 1_000_000),
)
# 提权枚举脚本(缺失/残缺即断):linpeas 约 1.1MB; PHPGGC 入口脚本只是 autoload
# 引导, 上游本体仅 247 字节, 真正的完整性由 check_phpggc 的 -l 功能校验兜底。
REQUIRED_SUPPORT_FILES = (
    ("/opt/peass/linpeas.sh", 500_000),
    ("/opt/phpggc/phpggc", 200),
)
MIN_NUCLEI_TEMPLATES = 1000
# Playwright 库匹配的 chromium 二进制目录(/root/.cache/ms-playwright/chromium-*)。
PLAYWRIGHT_CACHE = Path("/root/.cache/ms-playwright")


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


def check_jwt() -> None:
    # HMAC 密钥取 >=32 字节: PyJWT >= 2.10 按 RFC 7518 §3.2 对短密钥发
    # InsecureKeyLengthWarning。环境自检应示范规范用法(无告警噪音);
    # 短密钥/弱密钥是攻击侧测试目标, 相关示例在 playbook 里, 不在这里。
    secret = "norns-verify-hs256-roundtrip-secret"  # 35 bytes >= 32
    jwt_module = importlib.import_module("jwt")
    token = jwt_module.encode({"sub": "norns-verify"}, secret, algorithm="HS256")
    payload = jwt_module.decode(token, secret, algorithms=["HS256"])
    require(payload.get("sub") == "norns-verify", f"JWT round-trip failed: {payload}")


def check_pycryptodome() -> None:
    from Crypto.Cipher import AES
    from Crypto.Util.Padding import pad, unpad

    key = b"0123456789abcdef"
    # 明文故意取非块对齐长度(17 字节), 走真实攻击面同款 pad/unpad 往返;
    # 不 pad 直接 ECB encrypt 会抛 "Data must be aligned to block boundary"。
    plaintext = b"norns-verify-pad!"
    ciphertext = AES.new(key, AES.MODE_ECB).encrypt(pad(plaintext, AES.block_size))
    recovered = unpad(AES.new(key, AES.MODE_ECB).decrypt(ciphertext), AES.block_size)
    require(recovered == plaintext, f"AES round-trip failed: {recovered!r}")


def check_java_routers() -> None:
    """三代 JDK 路由与 maven 切换: 验证 wrapper 指向正确的 JDK 大版本。"""
    require(
        os.environ.get("JAVA_HOME") == "/opt/jdks/jdk21",
        f"JAVA_HOME should default to /opt/jdks/jdk21, got {os.environ.get('JAVA_HOME')!r}",
    )
    expectations = (
        (["java", "-version"], '"21.'),
        (["javac", "-version"], "21."),
        (["java8", "-version"], "1.8.0"),
        (["java11", "-version"], '"11.'),
        (["java21", "-version"], '"21.'),
        (["javac8", "-version"], "1.8.0"),
        (["mvn", "-v"], "Apache Maven"),
        (["mvn8", "-v"], "1.8.0"),
    )
    for argv, needle in expectations:
        output = command_output(argv)
        require(needle in output, f"{argv[0]} expected {needle!r} in output: {output!r}")


def check_tesseract_langs() -> None:
    """OCR 语言包: eng + chi_sim 都在才算就绪(统计/验证码题常用组合)。"""
    output = command_output(["tesseract", "--list-langs"])
    for lang in ("eng", "chi_sim"):
        require(lang in output, f"tesseract missing language pack: {lang}; got: {output!r}")


def check_i386_runtime() -> None:
    """amd64 上要求 32 位 libc 真实可执行; arm64 按 Dockerfile 设计不含 i386 库。"""
    if platform.machine() != "x86_64":
        return
    libc = Path("/lib/i386-linux-gnu/libc.so.6")
    require(libc.is_file(), f"missing 32-bit libc: {libc}")
    result = subprocess.run([str(libc)], check=True, capture_output=True, text=True)
    require(
        "GNU C Library" in (result.stdout or result.stderr),
        "32-bit libc.so.6 did not execute (i386 runtime broken)",
    )


def check_elftools() -> None:
    """pyelftools 功能级校验: 真能解析出 /bin/ls 的 ELF 头才算装好。

    模块文件是小写 elffile.py——pyelftools 各版本从未有过大写 ELFFile.py,
    `from elftools.elf.ELFFile import ...` 是必然 ImportError 的错误写法。
    """
    from elftools.elf.elffile import ELFFile

    with open("/bin/ls", "rb") as fh:
        machine = ELFFile(fh).header["e_machine"]
    require(bool(machine), f"pyelftools failed to parse /bin/ls: {machine!r}")


def check_pwndbg() -> None:
    """pwndbg 硬依赖: 镜像内以 /root/.gdbinit 挂钩方式装入 gdb。

    pwndbg 2026.07 起仓库不再有 bin/ 独立启动器, 镜像里没有也不应有 `pwndbg`
    命令(Dockerfile 曾 ln -sf bin/pwndbg, 现代布局下那是悬空软链)——
    gdbinit 挂钩 + 仓库目录在位才是有效的就绪判据。
    """
    gdbinit = Path("/root/.gdbinit")
    hooked = gdbinit.is_file() and "pwndbg" in gdbinit.read_text(encoding="utf-8", errors="ignore")
    require(hooked and Path("/opt/pwndbg/gdbinit.py").is_file(),
            "pwndbg not installed (gdbinit hook missing or /opt/pwndbg incomplete)")


def check_pillow() -> None:
    """Pillow 功能级校验: 内存内 PNG 像素往返不失真才算装好。"""
    import io

    from PIL import Image

    buffer = io.BytesIO()
    Image.new("RGB", (1, 1), color=(7, 77, 177)).save(buffer, format="PNG")
    buffer.seek(0)
    pixel = Image.open(buffer).getpixel((0, 0))
    require(pixel == (7, 77, 177), f"Pillow PNG round-trip failed: {pixel!r}")


def check_jsfinder() -> None:
    """JSFinder 硬依赖: 脚本存在且可通过 Python 语法编译(不触网验证依赖 requests+bs4 已硬校验)。"""
    path = Path("/opt/jsfinder/JSFinder.py")
    require(path.is_file() and path.stat().st_size > 0, f"missing or empty: {path}")
    py_compile.compile(str(path), doraise=True)


def check_searchsploit() -> None:
    """searchsploit 硬依赖: wrapper 可执行且离线库真能检索出 MS17-010 才算就绪。

    必须带 --overflow: searchsploit 默认按列宽截断标题, "(MS17-010)" 后缀会被切掉,
    不带该参数时即使库完整也匹配不到。
    """
    require(shutil.which("searchsploit") is not None, "missing command: searchsploit")
    csv = Path("/opt/exploitdb/files_exploits.csv")
    require(csv.is_file() and csv.stat().st_size > 0,
            f"exploitdb database missing or empty: {csv}")
    output = command_output(["searchsploit", "--disable-colour", "--overflow", "ms17-010"])
    require("MS17-010" in output, f"searchsploit returned no MS17-010 hits: {output[:200]!r}")


def check_phpggc() -> None:
    """PHPGGC 硬依赖: 链列表真能列出已知 gadget(Laravel/RCE)才算可用。"""
    output = command_output(["phpggc", "-l"])
    require("Laravel/RCE" in output, f"phpggc -l missing Laravel/RCE chain: {output[:200]!r}")


def check_nuclei_templates() -> None:
    """nuclei 模板库量级校验: 少于阈值说明克隆残缺,直接判失败。"""
    templates = Path(os.environ.get("NUCLEI_TEMPLATES_DIR", "/opt/nuclei-templates"))
    require(templates.is_dir(), f"missing directory: {templates}")
    count = sum(1 for p in templates.rglob("*.yaml") if p.is_file())
    require(count >= MIN_NUCLEI_TEMPLATES, f"nuclei templates too few: {count} (min {MIN_NUCLEI_TEMPLATES})")


def check_playwright_browser() -> None:
    """playwright 浏览器二进制: 恰好一套 chromium(含 headless shell)才合格。

    多于一套即两套 revision 并存(chromium 一套 726MB raw), tar 导出会破 3GB 预算;
    且 CLI 与库各认各的 revision 时, 必有一方的 revision 从未被点验可用。
    """
    chromes = [p for p in PLAYWRIGHT_CACHE.glob("chromium-*") if p.is_dir()]
    require(len(chromes) == 1,
            f"expected exactly 1 chromium-* under {PLAYWRIGHT_CACHE}, got {len(chromes)}")


def check_playwright_alignment() -> None:
    """@playwright/cli 嵌装的 playwright 与顶层库必须是同一版本。

    CLI 内钉的是 alpha 版 playwright, 与 npm 稳定 @latest 永远错位; 错位即两套
    chromium(每套 726MB raw), tar 会破 3GB 预算。Dockerfile 构建期已按 CLI 嵌装
    版本把顶层库对齐并断言, 此处复核镜像实际状态, 拦非标准构建带回的双 revision。
    """
    node_script = (
        "const a = require('/usr/local/lib/node_modules/@playwright/cli/node_modules"
        "/playwright/package.json').version;"
        "const b = require('/usr/local/lib/node_modules/playwright/package.json').version;"
        "if (a !== b) { console.error(a + ' != ' + b); process.exit(1); }"
        "console.log(a);"
    )
    version = command_output(["node", "-e", node_script])
    require(bool(version), "empty playwright version from node -e")


def require_zip_jar(path: Path, min_size: int) -> None:
    require(path.is_file() and path.stat().st_size >= min_size,
            f"missing or truncated jar: {path} (min {min_size} bytes)")
    with path.open("rb") as fh:
        require(fh.read(2) == b"PK", f"not a jar/zip file: {path}")


def check_template() -> None:
    yaml_module = importlib.import_module("yaml")
    config = yaml_module.safe_load(TEMPLATE_PATH.read_text(encoding="utf-8"))
    version_info = json.loads(command_output(["nornsd", "version", "--json"]))
    schema = version_info.get("agent_config_version") if isinstance(version_info, dict) else None
    require(
        type(schema) is int and schema > 0,
        "nornsd version --json must report a positive integer agent_config_version",
    )
    require(
        isinstance(config, dict) and bool(config) and "VERSION" not in config
        and all(isinstance(key, str) and key.startswith(("AGENT_", "SERVICE_")) for key in config)
        and all(key in config for key in ("SERVICE_NORNSD_ROOT_DIR", "SERVICE_CONCURRENCY", "AGENT_TYPE", "AGENT_LLM_MODEL")),
        f"template must use current SERVICE_* / AGENT_* fields without VERSION: {TEMPLATE_PATH}",
    )


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
    verification.check("JWT round-trip", check_jwt)
    verification.check("PyCryptodome AES round-trip", check_pycryptodome)
    verification.check("Pillow PNG round-trip", check_pillow)
    # —— 功能级校验(命令存在 ≠ 可用, 全部为硬断言) ——
    verification.check("Java version routers (8/11/21 + maven)", check_java_routers)
    verification.check("tesseract language packs (eng/chi_sim)", check_tesseract_langs)
    verification.check("i386 runtime libc", check_i386_runtime)
    verification.check("pyelftools ELF parsing", check_elftools)
    verification.check("pwndbg install", check_pwndbg)
    verification.check("JSFinder script", check_jsfinder)
    verification.check("searchsploit offline query", check_searchsploit)
    verification.check("PHPGGC gadget chains", check_phpggc)
    verification.check("nuclei templates volume", check_nuclei_templates)
    verification.check("playwright chromium browser", check_playwright_browser)
    verification.check("playwright CLI/lib version alignment", check_playwright_alignment)
    for path, min_size in REQUIRED_JARS:
        verification.check(
            f"jar file: {path}",
            lambda path=path, min_size=min_size: require_zip_jar(Path(path), min_size),
        )
    for path, min_size in REQUIRED_SUPPORT_FILES:
        verification.check(
            f"support file: {path}",
            lambda path=path, min_size=min_size: require(
                Path(path).is_file() and Path(path).stat().st_size >= min_size,
                f"missing or truncated: {path} (min {min_size} bytes)",
            ),
        )
    for path in ("/usr/local/bin/nornsd-entrypoint",):
        verification.check(
            f"Python syntax: {path}",
            lambda path=path: py_compile.compile(path, doraise=True),
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
    print("redteam server image verification: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
