import json
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkServiceRecoveryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def scheduler(self, module):
        task = module.QueueTask({"unique_code": "challenge", "flag_count": 1}, 0)
        return module.RoundQueueScheduler(
            [task], Mock(), Mock(), Mock(), model="model", started_iso="started",
        )

    def failure(self, module):
        return module.NornsdUnavailable(
            "SERVICE_UNAVAILABLE", "cannot begin database transaction", 503,
            request_id="req_database_failure",
            details={"sqlite_code": 5, "sqlite_error": "database is locked (SQLITE_BUSY)"},
        )

    def test_service_error_preserves_sqlite_code_and_request_id(self):
        for image, module in self.modules.items():
            payload = {"error": {"code": "SERVICE_UNAVAILABLE", "message": "cannot begin database transaction",
                                  "request_id": "req_database_failure", "details": {
                                      "sqlite_code": 5, "sqlite_error": "database is locked (SQLITE_BUSY)"}}}
            with self.subTest(image=image), patch.object(module, "_request", return_value=(503, json.dumps(payload).encode())):
                client = module.NornsdClient("http://nornsd/api", "token")
                with self.assertRaises(module.NornsdUnavailable) as raised:
                    client.submit_instruction("subject", "continue")
                self.assertEqual(raised.exception.details["sqlite_code"], 5)
                self.assertEqual(raised.exception.request_id, "req_database_failure")
                self.assertIn("sqlite_code=5", str(raised.exception))
                self.assertIn("database is locked (SQLITE_BUSY)", str(raised.exception))

    def test_failure_and_service_retry_logs_include_cause_without_diagnostics(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"), \
                    patch.object(module.Config, "diagnostics", False), \
                    patch.object(module.Config, "service_recovery_interval", 0):
                scheduler = self.scheduler(module)
                completed = module.ChallengeResult("challenge", 1, 1, True, status="completed")
                scheduler.nor.health.return_value = True
                with patch.object(module, "run_cell", side_effect=[self.failure(module), completed]), \
                        self.assertLogs(module.log, level="INFO") as captured:
                    scheduler.run_all()
                lines = [record.getMessage() for record in captured.records]
                for marker in ("nornsd 不可用，暂停队列", "queued retry pending service recovery", "后检查恢复"):
                    line = next(line for line in lines if marker in line)
                    for expected in ("database is locked (SQLITE_BUSY)", "sqlite_code=5", "req_database_failure"):
                        self.assertIn(expected, line)
                self.assertFalse(any("[DIAG]" in line for line in lines))

    def test_retry_logs_include_terminal_reasons_without_diagnostics(self):
        for image, module in self.modules.items():
            for state in ("error", "ended", "cancelled", "timeout"):
                with self.subTest(image=image, state=state), patch.object(module, "write_results"), \
                        patch.object(module, "_close_row_container"), \
                        patch.object(module.Config, "diagnostics", False):
                    scheduler = self.scheduler(module)
                    failed = module.ChallengeResult(
                        "challenge", 1, 0, False, status=state, runtime_state=state,
                        runtime_end_reason="agent stopped before solving",
                        runtime_error_code="PROCESS_EXIT_FAILED" if state == "error" else "",
                        error="agent exited during initialize" if state == "error" else "",
                    )
                    completed = module.ChallengeResult("challenge", 1, 1, True, status="completed")
                    with patch.object(module, "run_cell", side_effect=[failed, completed]), \
                            self.assertLogs(module.log, level="INFO") as captured:
                        scheduler.run_all()
                    line = next(record.getMessage() for record in captured.records
                                if "queued retry 2" in record.getMessage())
                    context = json.loads(line.split("; ")[-1])
                    self.assertEqual(context["status"], state)
                    self.assertEqual(context["end_reason"], failed.runtime_end_reason)
                    self.assertEqual(context["reason"], failed.error or failed.runtime_end_reason)
                    self.assertEqual(context["error_code"], failed.runtime_error_code)

    def test_inline_reasons_are_redacted_bounded_and_single_line(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "benchmark_token", "test-sensitive-token"):
                reason = "database error\ntest-sensitive-token Bearer another-credential\n" + "x" * 2048
                error = module.NornsdUnavailable(
                    "SERVICE_UNAVAILABLE", "cannot begin database transaction", 503,
                    details={"sqlite_error": reason},
                )
                rendered = str(error)
                self.assertNotIn("test-sensitive-token", rendered)
                self.assertNotIn("another-credential", rendered)
                self.assertNotIn("\n", rendered)
                self.assertIn("[REDACTED]", rendered)
                self.assertIn("[truncated]", rendered)
                result = module.ChallengeResult("challenge", 1, 0, False, error=reason)
                context = module._result_log_context(result)
                self.assertNotIn("test-sensitive-token", context)
                self.assertNotIn("another-credential", context)
                self.assertLess(len(json.loads(context)["reason"]), 1100)

    def test_repeated_503_stops_even_in_unlimited_tail_without_consuming_rounds(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container") as close, \
                    patch.object(module, "procure_container", return_value=SimpleNamespace(status="ready", addrs=[])) as procure, \
                    patch.object(module.Config, "service_recovery_attempts", 3), \
                    patch.object(module.Config, "service_recovery_interval", 0):
                scheduler = self.scheduler(module)
                task = scheduler.tasks[0]
                task.attempts = [{"round": number} for number in range(1, 2501)]
                scheduler.registry.get_or_create.side_effect = self.failure(module)
                scheduler.nor.health.return_value = True
                summary = scheduler.run_all()
                self.assertTrue(summary["timeout_disabled"])
                self.assertTrue(summary["global_stopped"])
                self.assertIn("cannot begin database transaction", summary["infrastructure_error"])
                self.assertEqual(summary["unresolvable"], 0)
                self.assertEqual(task.status, "pending")
                self.assertEqual(len(task.attempts), 2500)
                self.assertEqual(len(summary["service_failures"]), 4)
                self.assertTrue(all(item["round"] == 2501 for item in summary["service_failures"]))
                self.assertEqual(scheduler.nor.health.call_count, 3)
                self.assertEqual(scheduler.registry.get_or_create.call_count, 4)
                self.assertEqual(procure.call_count, 4)
                self.assertEqual(close.call_count, 4)

    def test_recovered_service_retries_same_round_and_preserves_previous_result(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"), \
                    patch.object(module.Config, "service_recovery_interval", 0):
                scheduler = self.scheduler(module)
                task = scheduler.tasks[0]
                previous = module.ChallengeResult("challenge", 1, 0, False, status="ended")
                task.result = previous
                task.attempts.append(previous.to_dict())
                completed = module.ChallengeResult("challenge", 1, 1, True, status="completed")
                options_seen = []

                def run_cell(*arguments, **options):
                    options_seen.append(options)
                    self.assertIs(options["previous_result"], previous)
                    if len(options_seen) == 1:
                        raise self.failure(module)
                    self.assertEqual(scheduler.nor.health.call_count, 1)
                    return completed

                scheduler.nor.health.return_value = True
                with patch.object(module, "run_cell", side_effect=run_cell):
                    summary = scheduler.run_all()
                self.assertEqual(summary["completed"], 1)
                self.assertFalse(summary["global_stopped"])
                self.assertEqual(summary["infrastructure_error"], "")
                self.assertEqual([options["attempt_round"] for options in options_seen], [2, 2])
                self.assertEqual(len(task.attempts), 2)
                self.assertEqual(len(summary["service_failures"]), 1)
                self.assertEqual(scheduler.recovery_attempts, 0)

    def test_service_failure_blocks_other_workers_before_opening_targets(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"), \
                    patch.object(module, "procure_container", return_value=SimpleNamespace(status="ready", addrs=[])) as procure:
                scheduler = self.scheduler(module)
                scheduler.registry.get_or_create.side_effect = self.failure(module)
                task = scheduler.tasks[0]
                scheduler._run_task(task, 1, 1, 40)
                self.assertTrue(scheduler.service_unavailable.is_set())
                original_error = scheduler.last_service_error
                scheduler._run_task(task, 2, 1, 40)
                self.assertEqual(scheduler.last_service_error, original_error)
                self.assertEqual(procure.call_count, 1)
                self.assertEqual(scheduler.registry.get_or_create.call_count, 1)
                self.assertEqual(len(task.attempts), 0)

    def test_recovery_timeout_and_global_deadline_do_not_open_more_targets(self):
        for image, module in self.modules.items():
            for global_deadline in (None, 11):
                with self.subTest(image=image, deadline=global_deadline), patch.object(module, "write_results"), \
                        patch.object(module.Config, "service_recovery_interval", 1), \
                        patch.object(module.Config, "health_wait", 3):
                    scheduler = self.scheduler(module)
                    scheduler.overall_deadline = global_deadline
                    scheduler.service_unavailable.set()
                    scheduler.last_service_error = str(self.failure(module))
                    scheduler.nor.health.return_value = False
                    clock = SimpleNamespace(now=10)

                    def wait(seconds):
                        clock.now += seconds
                        return False

                    with patch.object(module.time, "monotonic", side_effect=lambda: clock.now), \
                            patch.object(scheduler.stop_event, "wait", side_effect=wait), \
                            patch.object(module, "procure_container") as procure:
                        summary = scheduler.run_all()
                    procure.assert_not_called()
                    self.assertTrue(summary["global_stopped"])
                    self.assertEqual(bool(summary["infrastructure_error"]), global_deadline is None)
                    self.assertEqual(clock.now, global_deadline or 13)

    def test_stop_interrupts_recovery_wait(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                scheduler = self.scheduler(module)
                scheduler.service_unavailable.set()

                def wait(seconds):
                    scheduler.request_stop()
                    return True

                with patch.object(scheduler.stop_event, "wait", side_effect=wait):
                    self.assertFalse(scheduler._recover_service())
                scheduler.nor.health.assert_not_called()
                self.assertTrue(scheduler.stop_event.is_set())

    def test_new_failure_during_health_probe_is_not_cleared(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module.Config, "service_recovery_interval", 0):
                scheduler = self.scheduler(module)
                scheduler.service_unavailable.set()

                def health():
                    if scheduler.nor.health.call_count == 1:
                        with scheduler.lock:
                            scheduler.service_generation += 1
                        return True
                    return True

                scheduler.nor.health.side_effect = health
                self.assertTrue(scheduler._recover_service())
                self.assertEqual(scheduler.nor.health.call_count, 2)
                self.assertEqual(scheduler.recovery_attempts, 1)
                self.assertFalse(scheduler.service_unavailable.is_set())


if __name__ == "__main__":
    unittest.main()
