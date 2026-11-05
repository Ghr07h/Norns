import importlib.util
from pathlib import Path
import re
import subprocess
import sys
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

CONTAINER_ROOT = Path(__file__).resolve().parents[1]
REDTEAM_IMAGES = ("redteam", "redteam_for_benchmark", "redteam_for_benchmark_cc")


def load_verifier(image):
    name = f"container_reverse_test_{image}_verify"
    spec = importlib.util.spec_from_file_location(name, CONTAINER_ROOT / image / "verify.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class ReverseToolTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.verifiers = {image: load_verifier(image) for image in REDTEAM_IMAGES}

    def collect_checks(self, module):
        checks = {}
        with patch.object(module.Verification, "check", side_effect=lambda name, action: checks.update({name: action})), \
                patch("builtins.print"):
            self.assertEqual(module.main(), 0)
        return checks

    def test_dockerfiles_explicitly_install_tools(self):
        for image in self.verifiers:
            with self.subTest(image=image):
                source = (CONTAINER_ROOT / image / "Dockerfile").read_text()
                normalized = source.replace("\\\n", " ")
                apt_install = re.search(r"RUN apt-get update && apt-get install .*?&& apt-get clean", normalized).group()
                pip_install = re.search(r"RUN pip install --no-cache-dir .*?(?=\n)", normalized).group()
                for package in ("gdb", "strace", "binutils"):
                    self.assertIn(package, apt_install.split())
                for package in ("pwntools", "capstone", "ROPgadget", "z3-solver", "numpy"):
                    self.assertIn(package, pip_install.split())
                self.assertIn('case "$arch" in amd64|arm64)', source)
                self.assertIn('package="radare2_${RADARE2_VERSION}_${arch}.deb"', source)
                self.assertLess(source.index("sha256sum --check --ignore-missing checksums.txt"), source.index('"/tmp/radare2/$package"'))

    def test_verifiers_require_commands_and_python_modules(self):
        commands = {"gdb", "strace", "as", "ld", "objdump", "readelf", "nm", "strings", "radare2", "r2", "rabin2", "ROPgadget"}
        for image, module in self.verifiers.items():
            with self.subTest(image=image):
                self.assertTrue(commands.issubset(module.REQUIRED_COMMANDS))
                self.assertTrue({"capstone", "ropgadget", "z3", "numpy"}.issubset(module.REQUIRED_MODULES))

    def test_version_checks_execute_tools_without_tracing_processes(self):
        commands = (("gdb", "--version"), ("strace", "-V"), ("ld", "--version"), ("radare2", "-v"), ("ROPgadget", "--version"))
        for image, module in self.verifiers.items():
            with self.subTest(image=image):
                checks = self.collect_checks(module)
                self.assertEqual(module.REVERSE_TOOL_VERSION_COMMANDS, commands)
                with patch.object(module, "command_output", return_value="tool version") as command:
                    for argv in commands:
                        checks[f"tool version: {argv[0]}"]()
                        command.assert_called_with(list(argv))
                    self.assertEqual(command.call_count, len(commands))

    def test_version_checks_report_tool_execution_failure(self):
        for image, module in self.verifiers.items():
            with self.subTest(image=image):
                checks = self.collect_checks(module)
                verification = module.Verification()
                with patch.object(module, "command_output", side_effect=subprocess.CalledProcessError(1, ["radare2", "-v"])), \
                        patch("builtins.print"):
                    verification.check("radare2", checks["tool version: radare2"])
                self.assertEqual(verification.failed, 1)

    def test_missing_commands_fail_verification(self):
        for image, module in self.verifiers.items():
            for name in ("gdb", "strace", "readelf", "radare2", "ROPgadget"):
                with self.subTest(image=image, command=name):
                    checks = self.collect_checks(module)
                    with patch.object(module.shutil, "which", return_value=None):
                        with self.assertRaisesRegex(RuntimeError, f"missing command: {name}"):
                            checks[f"command available: {name}"]()

    def test_capstone_disassembly_uses_expected_architecture_and_bytes(self):
        for image, module in self.verifiers.items():
            with self.subTest(image=image):
                disassembler = Mock()
                disassembler.disasm.return_value = [SimpleNamespace(mnemonic="nop"), SimpleNamespace(mnemonic="ret")]
                capstone = SimpleNamespace(Cs=Mock(return_value=disassembler), CS_ARCH_X86=4, CS_MODE_64=8)
                with patch.object(module.importlib, "import_module", return_value=capstone) as importer:
                    module.check_capstone()
                importer.assert_called_once_with("capstone")
                capstone.Cs.assert_called_once_with(4, 8)
                disassembler.disasm.assert_called_once_with(b"\x90\xc3", 0x1000)
                self.assertIs(self.collect_checks(module)["Capstone disassembly"], module.check_capstone)

    def test_capstone_rejects_incomplete_or_incorrect_disassembly(self):
        for image, module in self.verifiers.items():
            for mnemonics in ([], ["nop"], ["nop", "int3"]):
                with self.subTest(image=image, mnemonics=mnemonics):
                    disassembler = Mock()
                    disassembler.disasm.return_value = [SimpleNamespace(mnemonic=name) for name in mnemonics]
                    capstone = SimpleNamespace(Cs=Mock(return_value=disassembler), CS_ARCH_X86=4, CS_MODE_64=8)
                    with patch.object(module.importlib, "import_module", return_value=capstone):
                        with self.assertRaisesRegex(RuntimeError, "Capstone disassembly failed"):
                            module.check_capstone()

    def test_dockerfile_run_instructions_have_valid_shell_syntax(self):
        for image in self.verifiers:
            with self.subTest(image=image):
                source = (CONTAINER_ROOT / image / "Dockerfile").read_text().replace("\\\n", " ")
                for instruction in source.splitlines():
                    if instruction.startswith("RUN "):
                        subprocess.run(["/bin/sh", "-n"], input=instruction[4:], text=True, check=True, capture_output=True)


if __name__ == "__main__":
    unittest.main()
