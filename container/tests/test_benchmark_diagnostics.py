import hashlib
import json
import os
from pathlib import Path
import tempfile
import threading
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkDiagnosticsTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def records(self, logger):
        return [json.loads(call.args[1]) for call in logger.call_args_list
                if call.args[0] == "[DIAG] %s"]

    def test_disabled_diagnostics_do_not_log_or_read_artifacts(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "diagnostics", False), \
                    patch.object(module.log, "info") as logger, patch("builtins.open") as opener:
                module._diagnostic("example", message="diagnostic")
                module._log_runtime_diagnostics(
                    {"state": "error", "artifacts": {"raw_log_path": "/not/read"}},
                    "subject", "runtime")
                logger.assert_not_called()
                opener.assert_not_called()

    def test_diagnostics_redact_credentials_bound_text_and_tolerate_logging_failures(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "diagnostics", True), \
                    patch.object(module.Config, "benchmark_token", "platform-private-value"), \
                    patch.dict(os.environ, {"LLM_KEY": "provider-private-value"}), \
                    patch.object(module.log, "info") as logger:
                module._diagnostic("example", error={
                    "message": "platform-private-value provider-private-value Bearer unlisted-credential",
                    "authorization": "another-private-value",
                    "nested": {"api_key": "nested-private-value"},
                }, text="large" * 5000)
                encoded = logger.call_args.args[1]
                for secret in ("platform-private-value", "provider-private-value", "unlisted-credential",
                               "another-private-value", "nested-private-value"):
                    self.assertNotIn(secret, encoded)
                record = self.records(logger)[0]
                self.assertEqual(record["event"], "example")
                self.assertEqual(record["pid"], os.getpid())
                self.assertTrue(record["timestamp_utc"].endswith("Z"))
                self.assertLess(len(record["text"]), 2100)
                logger.side_effect = OSError("logging unavailable")
                module._diagnostic("example", message="must not affect execution")

    def test_platform_diagnostics_preserve_response_authentication_and_request_count(self):
        flag = "flag{diagnostic-test}"
        body = {"unique_code": "challenge", "flag": flag}
        cases = (
            (200, {"correct": True, "correct_flag_count": 1}),
            (409, {"code": "duplicate", "correct": True, "correct_flag_count": 1}),
            (503, {"code": "unavailable", "message": "retry later"}),
        )
        for image, module in self.modules.items():
            for status, payload in cases:
                outcomes = []
                for enabled in (False, True):
                    with self.subTest(image=image, status=status, diagnostics=enabled), \
                            patch.object(module.Config, "diagnostics", enabled), \
                            patch.object(module, "_request", return_value=(status, json.dumps(payload).encode())) as request, \
                            patch.object(module.log, "info") as logger:
                        client = module.SaaSClient("https://platform.test", "platform-private-value")
                        try:
                            outcomes.append(client._call("POST", "/openapi/v1/challenges/submit", body=body))
                        except module.SaaSError as error:
                            outcomes.append((error.code, str(error)))
                        request.assert_called_once_with(
                            "POST", "https://platform.test/openapi/v1/challenges/submit",
                            headers={"BENCHMARK_TOKEN": "platform-private-value"}, body=body, timeout=60.0)
                        records = self.records(logger)
                        if enabled:
                            self.assertEqual([record["event"] for record in records],
                                             ["platform_request", "platform_response"])
                            self.assertEqual(records[0]["request_id"], records[1]["request_id"])
                            self.assertEqual(records[0]["flag_sha256"], hashlib.sha256(flag.encode()).hexdigest())
                            self.assertEqual(records[1]["status"], status)
                            self.assertEqual(records[1]["response"], payload)
                            self.assertNotIn(flag, json.dumps(records))
                        else:
                            self.assertEqual(records, [])
                self.assertEqual(outcomes[0], outcomes[1])

    def test_transport_error_is_logged_and_reraised_without_retry(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "diagnostics", True), \
                    patch.object(module.log, "info") as logger:
                failure = module.BenchmarkError("connection reset")
                with patch.object(module, "_request", side_effect=failure) as request:
                    with self.assertRaises(module.BenchmarkError) as raised:
                        module.SaaSClient("https://platform.test", "private").submit_flag("challenge", "flag{test}")
                self.assertIs(raised.exception, failure)
                self.assertEqual(request.call_count, 1)
                records = self.records(logger)
                self.assertEqual(records[-1]["event"], "platform_transport_error")
                self.assertEqual(records[-1]["request_id"], records[0]["request_id"])

    def test_platform_snapshot_does_not_update_cached_challenge_or_add_queries(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "diagnostics", True), \
                    patch.object(module.log, "info") as logger:
                challenge = {"unique_code": "challenge", "container_status": "available",
                             "container_addr": ["target:80"], "correct_flag_count": 1,
                             "flag_count": 1, "is_completed": True}
                original = dict(challenge)
                client = module.SaaSClient("https://platform.test", "private")
                with patch.object(client, "list_challenges", return_value=[challenge]) as query:
                    self.assertEqual(client.wait_until_available("challenge", []), ["target:80"])
                query.assert_called_once_with()
                self.assertEqual(challenge, original)
                self.assertEqual(self.records(logger)[0]["correct_flag_count"], 1)

    def test_raw_tail_is_bounded_and_filters_current_runtime_stderr(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), tempfile.TemporaryDirectory() as directory, \
                    patch.object(module.Config, "diagnostics", True), \
                    patch.object(module.log, "info") as logger:
                path = Path(directory) / "raw.jsonl"
                entries = [{"runtime_id": "current", "stream": "stderr",
                            "payload": {"text": "message-" + str(index)}} for index in range(8)]
                entries.extend([
                    {"runtime_id": "previous", "stream": "stderr", "payload": {"text": "old-error"}},
                    {"runtime_id": "current", "stream": "stdout", "payload": {"text": "normal-output"}},
                ])
                path.write_text("x" * 70000 + "\n" + "\n".join(json.dumps(entry) for entry in entries)
                                + "\n{partial", encoding="utf-8")
                runtime = {"state": "error", "error": {"details": {"cause": "initialization failed"}},
                           "process": {"result": {"exit_code": 1}}, "artifacts": {"raw_log_path": str(path)}}
                module._log_runtime_diagnostics(runtime, "subject", "current", unique_code="challenge")
                records = self.records(logger)
                self.assertEqual(records[0]["error"], runtime["error"])
                self.assertEqual(records[0]["process"], runtime["process"])
                self.assertEqual(records[1]["event"], "agent_stderr_tail")
                self.assertEqual([entry["payload"]["text"] for entry in records[1]["entries"]],
                                 ["message-" + str(index) for index in range(3, 8)])

    def test_terminal_result_and_api_call_counts_are_unchanged(self):
        for image, module in self.modules.items():
            outcomes = []
            for enabled in (False, True):
                with self.subTest(image=image, diagnostics=enabled), \
                        patch.object(module.Config, "diagnostics", enabled), \
                        patch.object(module.log, "info") as logger, \
                        patch.object(module, "procure_container", return_value=SimpleNamespace(status="ready", addrs=["target:80"])), \
                        patch.object(module, "_drain_events", return_value=([
                            {"type": "diagnostic", "runtime_id": "runtime", "payload": {"message": "agent failed"}}
                        ], None)) as drain, patch.object(module, "_stop_and_wait") as stop:
                    registry = Mock()
                    registry.get_or_create.return_value = module.RuntimeSession("subject", "runtime", None)
                    nor = Mock()
                    nor.get_runtime.return_value = {"state": "error", "end_reason": "agent process exited",
                                                    "error": {"code": "AGENT_EXITED", "details": {"exit_code": 1}}}
                    saas = Mock()
                    result = module.run_cell(
                        {"unique_code": "challenge", "flag_count": 1}, saas, nor, registry,
                        module.RowContainer(), 60.0, threading.Event(), attempt_round=2)
                    outcome = result.to_dict()
                    outcome.pop("time_used")
                    outcomes.append(outcome)
                    nor.get_runtime.assert_called_once_with("subject", "runtime")
                    self.assertEqual(drain.call_count, 2)
                    stop.assert_called_once_with(nor, "subject", "runtime", module.Config.stop_grace)
                    saas.submit_flag.assert_not_called()
                    self.assertEqual(registry.get_or_create.call_count, 1)
                    records = self.records(logger)
                    self.assertEqual(any(record["event"] == "runtime_terminal" for record in records), enabled)
            self.assertEqual(outcomes[0], outcomes[1])

    def test_missing_raw_file_does_not_interrupt_diagnostics(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "diagnostics", True), \
                    patch.object(module.log, "info") as logger, \
                    patch("builtins.open", side_effect=FileNotFoundError("remote file unavailable")):
                module._log_runtime_diagnostics(
                    {"state": "error", "artifacts": {"raw_log_path": "/remote/raw.jsonl"}},
                    "subject", "runtime")
                self.assertEqual([record["event"] for record in self.records(logger)],
                                 ["runtime_terminal", "runtime_raw_unavailable"])


if __name__ == "__main__":
    unittest.main()
