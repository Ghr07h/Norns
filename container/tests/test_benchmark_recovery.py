import threading
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkRecoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def result(self, module, status, **options):
        return module.ChallengeResult(
            unique_code="challenge", flag_count=6, correct_flag_count=4,
            is_completed=False, status=status, **options,
        )

    def test_all_recovery_prefixes_require_recording_target_changes(self):
        ending = "请重新打开任务、如果靶场变更则记录为全局认知并继续推进任务完成。"
        for image, module in self.modules.items():
            for reason, prefix in module.RESUME_INSTRUCTION_PREFIXES.items():
                with self.subTest(image=image, reason=reason):
                    self.assertTrue(prefix.endswith(ending))

    def test_recovery_prefix_precedes_shared_context(self):
        cases = (
            ("ended", "", "ended"),
            ("error", "RUNTIME_MEMORY_LIMIT_EXCEEDED", "memory_limit"),
            ("timeout", "", "timeout"),
            ("cancelled", "", "cancelled"),
            ("aborted", "", "cancelled"),
        )
        for image, module in self.modules.items():
            for status, error_code, prefix in cases:
                with self.subTest(image=image, status=status), patch.object(module.Config, "test_env", False):
                    previous = self.result(module, status, runtime_error_code=error_code)
                    text = module.render_instruction(
                        {"unique_code": "challenge", "flag_count": 6, "correct_flag_count": 4},
                        ["current-target:1234", "current-target:5678"],
                        resume_session=True, previous_result=previous,
                    )
                    self.assertTrue(text.startswith(module.RESUME_INSTRUCTION_PREFIXES[prefix] + "\n\n"))
                    self.assertIn("继续处理当前 benchmark 题目：challenge。", text)
                    self.assertIn("平台验收进度：已通过 4/6，尚未通关。", text)
                    self.assertIn("注意：靶场重启后，将重置靶场状态并可能变更靶场地址。", text)
                    self.assertIn("current-target:1234", text)
                    self.assertIn("current-target:5678", text)
                    self.assertNotIn("--- 题目入口信息 ---", text)

    def test_unknown_errors_and_missing_history_do_not_claim_timeout_or_memory_failure(self):
        for image, module in self.modules.items():
            cases = (
                (self.result(module, "error", runtime_error_code="PROCESS_EXIT_FAILED"), "PROCESS_EXIT_FAILED"),
                (self.result(module, "error", error="read_events: disconnected"), "read_events: disconnected"),
                (None, "未记录上轮结束原因"),
            )
            for previous, reason in cases:
                with self.subTest(image=image, reason=reason):
                    text = module.render_resume_instruction({"unique_code": "challenge"}, [], previous)
                    self.assertIn("异常原因：" + reason, text)
                    self.assertNotIn("超时检测已通过", text)
                    self.assertNotIn("发生内存超限", text)

    def test_initial_instruction_and_test_environment_note_are_preserved(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "test_env", True):
                initial = module.render_instruction({"unique_code": "challenge"}, [])
                resumed = module.render_instruction(
                    {"unique_code": "challenge"}, [], resume_session=True,
                    previous_result=self.result(module, "ended"),
                )
                self.assertIn("--- 题目入口信息 ---", initial)
                self.assertNotIn("上轮智能体", initial)
                self.assertIn(module.TEST_ENV_NOTE, initial)
                self.assertIn(module.TEST_ENV_NOTE, resumed)

    def run_terminal_cell(self, module, runtime, previous=None):
        registry = Mock()
        registry.get_or_create.return_value = module.RuntimeSession("subject", "runtime", None)
        nor = Mock()
        nor.get_runtime.return_value = runtime
        with patch.object(module, "procure_container", return_value=SimpleNamespace(status="ready", addrs=["new-target:1234"])), \
                patch.object(module, "_drain_events", return_value=([], None)), \
                patch.object(module, "_stop_and_wait"):
            result = module.run_cell(
                {"unique_code": "challenge", "flag_count": 6, "correct_flag_count": 4},
                Mock(), nor, registry, module.RowContainer(), 60.0, threading.Event(),
                attempt_round=2, previous_result=previous,
            )
        return result, registry.get_or_create.call_args

    def test_terminal_states_and_real_error_codes_are_preserved(self):
        cases = (
            {"state": "ended", "end_reason": "norns task closed", "error": None},
            {"state": "error", "end_reason": "RUNTIME_MEMORY_LIMIT_EXCEEDED",
             "error": {"code": "RUNTIME_MEMORY_LIMIT_EXCEEDED", "details": {}}},
            {"state": "cancelled", "end_reason": "user_graceful_stop", "error": None},
        )
        for image, module in self.modules.items():
            for runtime in cases:
                with self.subTest(image=image, state=runtime["state"]):
                    result, _ = self.run_terminal_cell(module, runtime)
                    self.assertEqual(result.status, runtime["state"])
                    self.assertEqual(result.runtime_state, runtime["state"])
                    self.assertEqual(result.runtime_end_reason, runtime["end_reason"])
                    expected_code = (runtime["error"] or {}).get("code", "")
                    self.assertEqual(result.runtime_error_code, expected_code)
                    self.assertEqual(result.to_dict()["runtime_error_code"], expected_code)
                    if runtime["state"] == "error":
                        self.assertEqual(result.error, expected_code)

    def test_previous_runtime_reason_reaches_resumed_subject_instruction(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                previous = self.result(module, "error", runtime_error_code="RUNTIME_MEMORY_LIMIT_EXCEEDED")
                _, arguments = self.run_terminal_cell(module, {"state": "ended"}, previous)
                self.assertTrue(arguments.kwargs["resume_session"])
                self.assertTrue(arguments.args[1].startswith(module.RESUME_INSTRUCTION_PREFIXES["memory_limit"]))
                self.assertIn("new-target:1234", arguments.args[1])
                self.assertIn("已通过 4/6", arguments.args[1])

    def test_scheduler_passes_previous_result_to_each_retry(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"):
                task = module.QueueTask({"unique_code": "challenge", "flag_count": 6}, 0)
                scheduler = module.RoundQueueScheduler(
                    [task], Mock(), Mock(), Mock(), model="model", started_iso="started",
                )
                results = [
                    self.result(module, "ended"),
                    self.result(module, "error", runtime_error_code="RUNTIME_MEMORY_LIMIT_EXCEEDED"),
                    self.result(module, "timeout"),
                    self.result(module, "cancelled"),
                    self.result(module, "completed"),
                ]

                def run_cell(*arguments, **options):
                    round_number = options["attempt_round"]
                    self.assertIs(options["previous_result"], results[round_number - 2] if round_number > 1 else None)
                    return results[round_number - 1]

                with patch.object(module, "run_cell", side_effect=run_cell):
                    summary = scheduler.run_all()
                self.assertEqual(summary["completed"], 1)
                self.assertEqual(len(task.attempts), 5)
                self.assertEqual(task.attempts[1]["status"], "error")
                self.assertEqual(task.attempts[1]["runtime_error_code"], "RUNTIME_MEMORY_LIMIT_EXCEEDED")

    def test_platform_expiration_does_not_resume_or_create_a_runtime(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.object(module, "procure_container", return_value=SimpleNamespace(status="invalid_state")):
                registry = Mock()
                result = module.run_cell(
                    {"unique_code": "challenge", "flag_count": 6}, Mock(), Mock(), registry,
                    module.RowContainer(), 60.0, threading.Event(), attempt_round=2,
                    previous_result=self.result(module, "cancelled"),
                )
                self.assertEqual(result.status, "invalid_state")
                registry.get_or_create.assert_not_called()


if __name__ == "__main__":
    unittest.main()
