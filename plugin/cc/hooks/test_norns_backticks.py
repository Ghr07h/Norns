"""Run with python3 -B -m unittest discover -s plugin/cc/hooks -p 'test_*.py'."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


HOOK = Path(__file__).with_name("pretooluse_norns_backticks.sh")


@unittest.skipUnless(shutil.which("sh") and shutil.which("awk"), "requires sh and awk")
class NornsBackticksHookTests(unittest.TestCase):
    def invoke(self, payload, env=None):
        result = subprocess.run(
            ["sh", str(HOOK)], input=payload, text=True, capture_output=True,
            env=env, timeout=5, check=True,
        )
        self.assertEqual(result.stderr, "")
        return result.stdout

    def rewrite(self, command, **fields):
        payload = {"tool_name": "Bash", "tool_input": {"command": command, **fields}}
        output = self.invoke(json.dumps(payload))
        if not output:
            return command
        hook = json.loads(output)["hookSpecificOutput"]
        self.assertEqual(hook["hookEventName"], "PreToolUse")
        self.assertNotIn("permissionDecision", hook)
        for key, value in fields.items():
            self.assertEqual(hook["updatedInput"][key], value)
        return hook["updatedInput"]["command"]

    def test_quoted_arguments_and_command_positions(self):
        cases = [
            (r'''norns cognition add --cognition-content "记录 `probe \"node\"`"''',
             r'''norns cognition add --cognition-content "记录 **probe \"node\"**"'''),
            ('''norns task open --purpose '使用 `node` 和 `norns`' ''',
             '''norns task open --purpose '使用 **node** 和 **norns**' '''),
            ('norns task open --purpose "记录 `probe "node"`"',
             'norns task open --purpose "记录 **probe "node"**"'),
            ('''norns task open --purpose 'one `two` three `four' ''',
             '''norns task open --purpose 'one **two** three `four' '''),
            ('''norns task open --purpose 'one `two' --info "three`"''',
             '''norns task open --purpose 'one `two' --info "three`"'''),
            ('''norns cognition get --cognition `cat id.txt`''',
             '''norns cognition get --cognition `cat id.txt`'''),
            ('''echo 'norns `node`' ''', '''echo 'norns `node`' '''),
            ('''printf norns 'use `node`' ''', '''printf norns 'use `node`' '''),
            ('''# norns task open --purpose 'use `node`' ''',
             '''# norns task open --purpose 'use `node`' '''),
            ('''cd /workspace; norns task open --purpose "use `node`" && echo 'keep `node`' ''',
             '''cd /workspace; norns task open --purpose "use **node**" && echo 'keep `node`' '''),
            ('''if true; then norns task open --purpose "use `node`"; fi''',
             '''if true; then norns task open --purpose "use **node**"; fi'''),
            ('''if norns task open --purpose "use `node`"; then :; fi''',
             '''if norns task open --purpose "use **node**"; then :; fi'''),
            ('''if false; then :; elif true; then :; else norns task open --purpose 'use `node`'; fi''',
             '''if false; then :; elif true; then :; else norns task open --purpose 'use **node**'; fi'''),
            ('''while true; do norns task open --purpose 'use `node`'; done''',
             '''while true; do norns task open --purpose 'use **node**'; done'''),
            ('''command -- norns task open --purpose 'use `node`' ''',
             '''command -- norns task open --purpose 'use **node**' '''),
            ('''command -p norns task open --purpose 'use `node`' ''',
             '''command -p norns task open --purpose 'use **node**' '''),
            ('''command -v norns 'keep `node`' ''', '''command -v norns 'keep `node`' '''),
            ('''env -u IGNORE LANG=C command -- norns task open --purpose 'use `node`' ''',
             '''env -u IGNORE LANG=C command -- norns task open --purpose 'use **node**' '''),
            ('''exec -a label /norns/bin/norns task open --purpose 'use `node`' ''',
             '''exec -a label /norns/bin/norns task open --purpose 'use **node**' '''),
            ('''"/path with spaces/norns" task open --purpose 'use `node`' ''',
             '''"/path with spaces/norns" task open --purpose 'use **node**' '''),
            (r'''C:\tools\norns.exe task open --purpose 'use `node`' ''',
             r'''C:\tools\norns.exe task open --purpose 'use **node**' '''),
            ('''norns task open --purpose 'use `node`'\r\nnorns task open --purpose "use `norns`"\r\n''',
             '''norns task open --purpose 'use **node**'\r\nnorns task open --purpose "use **norns**"\r\n'''),
            ('''norns cognition add --cognition-content "one `node`\ntwo `norns`"''',
             '''norns cognition add --cognition-content "one `node`\ntwo `norns`"'''),
            ("norns cognition add \\\n  --cognition-content 'use `node`'",
             "norns cognition add \\\n  --cognition-content 'use **node**'"),
        ]
        for command, expected in cases:
            with self.subTest(command=command):
                self.assertEqual(self.rewrite(command), expected)
                self.assertEqual(self.rewrite(expected), expected)

    def test_heredoc_body_is_unchanged(self):
        body = "norns task open --purpose 'keep `node`'\n"
        after = "norns task open --purpose 'use `norns`'"
        for header, delimiter, prefix in [
            ("norns cognition add <<'EOF'", "EOF", ""),
            ("cat <<'EOF'", "EOF", ""),
            ('cat <<"EOF"', "EOF", ""),
            (r"cat <<\EOF", "EOF", ""),
            ("cat <<-EOF", "EOF", "\t"),
            (r"cat <<'E\OF'", r"E\OF", ""),
            (r'cat <<"E\OF"', r"E\OF", ""),
            ('norns cognition add <<"E`node`OF"', "E`node`OF", ""),
            ('norns cognition add --cognition-content "$(cat <<\'EOF\'', "EOF", ""),
            ('norns cognition add --cognition-content "$(cat <<"EOF"', "EOF", ""),
        ]:
            with self.subTest(header=header):
                command = header + "\n" + prefix + body + prefix + delimiter + "\n"
                if "$(" in header:
                    command += ')"\n'
                self.assertEqual(self.rewrite(command + after), command + after.replace("`", "**"))
        command = "cat <<'ONE' <<'TWO'\n" + body + "ONE\n" + body + "TWO\n"
        self.assertEqual(self.rewrite(command + after), command + after.replace("`", "**"))

    def test_unicode_and_other_input_fields_survive(self):
        command = 'norns task open --purpose "中文 😀 `node`"'
        fields = {"timeout": 1000, "description": 'text "tool_input"', "run_in_background": True,
                  "future_field": {"id": 9007199254740993}}
        self.assertEqual(self.rewrite(command, **fields), command.replace("`", "**"))
        payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command, **fields}}, ensure_ascii=False)
        updated = json.loads(self.invoke(payload))["hookSpecificOutput"]["updatedInput"]
        self.assertEqual(updated, {"command": command.replace("`", "**"), **fields})
        escaped = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}}).replace("`", r"\u0060")
        self.assertEqual(json.loads(self.invoke(escaped))["hookSpecificOutput"]["updatedInput"]["command"], command.replace("`", "**"))

    def test_unrelated_or_malformed_input_is_silent(self):
        for payload in ["not JSON", "null", "[]", "{}", '{"tool_name":"Bash"}',
                        '{"tool_name":"Bash","tool_input":{"command":"norns task open \'`node`\'"}',
                        '{"tool_name":"Bash","tool_input":{"command":42}}',
                        json.dumps({"tool_name": "PowerShell", "tool_input": {"command": 'norns task open "`node`"'}}),
                        json.dumps({"tool_name": "Bash", "tool_input": {"command": "norns workspace check"}})]:
            with self.subTest(payload=payload):
                self.assertEqual(self.invoke(payload), "")

    def test_non_unix_platform_is_silent(self):
        with tempfile.TemporaryDirectory() as temp:
            uname = Path(temp) / "uname"
            uname.write_text("#!/bin/sh\nprintf MINGW64_NT\\n\n")
            uname.chmod(0o755)
            env = dict(os.environ, PATH=temp + os.pathsep + os.environ.get("PATH", ""))
            payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": 'norns task open "`node`"'}})
            self.assertEqual(self.invoke(payload, env=env), "")

    def test_registered_hook_runs_independently(self):
        registration = json.loads(HOOK.with_name("hooks.json").read_text())
        matches = [entry for entry in registration["hooks"]["PreToolUse"] if entry["matcher"] == "Bash"]
        commands = [hook["command"] for entry in matches for hook in entry["hooks"]
                    if HOOK.name in hook.get("command", "")]
        self.assertEqual(len(commands), 1)
        with tempfile.TemporaryDirectory(prefix="standalone hook ") as temp:
            hooks = Path(temp) / "hooks"
            hooks.mkdir()
            shutil.copyfile(HOOK, hooks / HOOK.name)
            env = dict(os.environ, CLAUDE_PLUGIN_ROOT=temp)
            command = 'norns task open --purpose "记录 `node`"'
            result = subprocess.run(["sh", "-c", commands[0]], env=env, text=True, capture_output=True,
                                    input=json.dumps({"tool_name": "Bash", "tool_input": {"command": command}}),
                                    check=True, timeout=5)
            self.assertEqual(result.stderr, "")
            updated = json.loads(result.stdout)["hookSpecificOutput"]["updatedInput"]["command"]
            self.assertEqual(updated, command.replace("`", "**"))

    @unittest.skipUnless(shutil.which("bash"), "requires bash")
    def test_rewritten_commands_do_not_execute_substitutions(self):
        commands = [
            'norns cognition add --cognition-content "记录 `probe "node"`"',
            'if true; then norns cognition add --cognition-content "记录 `probe`"; fi',
            'command -- norns cognition add --cognition-content "记录 `probe`"',
        ]
        with tempfile.TemporaryDirectory() as temp:
            norns = Path(temp) / "norns"
            norns.write_text('#!/bin/sh\nprintf "%s\\n" "$@"\n')
            norns.chmod(0o755)
            env = dict(os.environ, PATH=temp + os.pathsep + os.environ.get("PATH", ""))
            for command in commands:
                with self.subTest(command=command):
                    script = 'probe() { printf PROBE_EXECUTED >&2; }\n' + self.rewrite(command)
                    result = subprocess.run(["bash", "--noprofile", "--norc", "-c", script],
                                            text=True, capture_output=True, check=True, env=env)
                    self.assertNotIn("PROBE_EXECUTED", result.stderr)
                    self.assertIn("**probe", result.stdout)


if __name__ == "__main__":
    unittest.main()
