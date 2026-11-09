import json
import threading
import unittest
from unittest.mock import Mock, call, patch
from urllib.parse import urlsplit

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkProgressTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def challenge(self, **updates):
        return {"unique_code": "challenge", "flag_count": 1,
                "correct_flag_count": 0, "is_completed": False, **updates}

    def client(self, latest):
        client = Mock()
        client.assert_task_running = Mock(return_value=True)
        client.list_challenges.return_value = [self.challenge(unique_code="other", is_completed=True), latest]
        client.start_challenge.return_value = ["target:80"]
        client.wait_until_available.return_value = ["target:80"]
        return client

    def test_platform_completion_skips_start_runtime_and_close(self):
        for image, module in self.modules.items():
            for progress in ({"is_completed": True}, {"correct_flag_count": 1},
                             {"correct_flag_count": 1, "is_completed": True}):
                with self.subTest(image=image, progress=progress):
                    challenge = self.challenge()
                    latest = self.challenge(cumulative_score=500, **progress)
                    client = self.client(latest)
                    registry = Mock()
                    nor = Mock()
                    host = module.RowContainer()
                    result = module.run_cell(challenge, client, nor, registry, host,
                                             60.0, threading.Event(), attempt_round=2)
                    module._close_row_container(client, "challenge", host)
                    self.assertEqual(result.status, "completed")
                    self.assertTrue(result.is_completed)
                    self.assertTrue(challenge["is_completed"])
                    self.assertEqual(result.correct_flag_count, latest["correct_flag_count"])
                    self.assertEqual(result.cumulative_score, 500)
                    self.assertEqual(result.awarded, 0)
                    self.assertEqual(result.flags_submitted, [])
                    client.list_challenges.assert_called_once_with()
                    client.start_challenge.assert_not_called()
                    client.wait_until_available.assert_not_called()
                    client.close_challenge.assert_not_called()
                    registry.get_or_create.assert_not_called()
                    self.assertEqual(nor.mock_calls, [])
                    self.assertTrue(host.ready.is_set())
                    self.assertFalse(host.opening)

    def test_partial_progress_updates_result_and_resume_instruction(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.object(module, "_drain_events", return_value=([], None)), \
                    patch.object(module, "_stop_and_wait"):
                challenge = self.challenge(flag_count=6)
                client = self.client(self.challenge(flag_count=3, correct_flag_count=2,
                                                    cumulative_score=200))
                registry = Mock()
                registry.get_or_create.return_value = module.RuntimeSession("subject", "runtime", None)
                nor = Mock()
                nor.get_runtime.return_value = {"state": "ended"}
                result = module.run_cell(challenge, client, nor, registry, module.RowContainer(),
                                         60.0, threading.Event(), attempt_round=2)
                self.assertEqual(result.status, "ended")
                self.assertEqual(result.flag_count, 3)
                self.assertEqual(result.correct_flag_count, 2)
                self.assertEqual(result.cumulative_score, 200)
                self.assertFalse(result.is_completed)
                self.assertIn("已通过 2/3", registry.get_or_create.call_args.args[1])
                self.assertEqual(client.method_calls[:2], [call.list_challenges(),
                                                          call.start_challenge("challenge")])

    def test_each_start_retry_checks_for_platform_completion(self):
        for image, module in self.modules.items():
            for failure in (module.SaaSError("resource_unavailable", "busy", 503),
                            module.InvalidState("already completed"),
                            module.BenchmarkError("start response lost")):
                with self.subTest(image=image, failure=type(failure).__name__), \
                        patch.multiple(module.Config, start_retries=2, start_retry_interval=0), \
                        patch.object(module.time, "sleep"):
                    client = self.client(self.challenge())
                    client.list_challenges.side_effect = [
                        [self.challenge()], [self.challenge(correct_flag_count=1)],
                    ]
                    client.start_challenge.side_effect = failure
                    registry = Mock()
                    result = module.run_cell(self.challenge(), client, Mock(), registry,
                                             module.RowContainer(), 60.0, threading.Event())
                    self.assertEqual(result.status, "completed")
                    self.assertEqual(client.list_challenges.call_count, 2)
                    client.start_challenge.assert_called_once_with("challenge")
                    client.wait_until_available.assert_not_called()
                    registry.get_or_create.assert_not_called()

    def test_state_query_failure_never_opens_from_cached_progress(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.multiple(module.Config, start_retries=2, start_retry_interval=0), \
                    patch.object(module.time, "sleep"):
                client = self.client(self.challenge())
                client.list_challenges.side_effect = module.BenchmarkError("connection reset")
                registry = Mock()
                host = module.RowContainer()
                result = module.run_cell(self.challenge(), client, Mock(), registry,
                                         host, 60.0, threading.Event())
                module._close_row_container(client, "challenge", host)
                self.assertEqual(result.status, "error")
                self.assertIn("state query network", result.error)
                self.assertEqual(client.list_challenges.call_count, 2)
                client.start_challenge.assert_not_called()
                client.close_challenge.assert_not_called()
                registry.get_or_create.assert_not_called()

    def test_failed_state_query_can_recover_without_opening_completed_challenge(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.multiple(module.Config, start_retries=2, start_retry_interval=0), \
                    patch.object(module.time, "sleep"):
                client = self.client(self.challenge())
                client.list_challenges.side_effect = [module.BenchmarkError("connection reset"),
                                                       [self.challenge(is_completed=True)]]
                result = module.run_cell(self.challenge(), client, Mock(), Mock(),
                                         module.RowContainer(), 60.0, threading.Event())
                self.assertEqual(result.status, "completed")
                client.start_challenge.assert_not_called()

    def test_platform_error_does_not_fall_back_to_cached_progress(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                client = self.client(self.challenge())
                failure = module.SaaSError("server_error", "failed", 500)
                client.list_challenges.side_effect = failure
                with self.assertRaises(module.SaaSError) as raised:
                    module.run_cell(self.challenge(), client, Mock(), Mock(),
                                    module.RowContainer(), 60.0, threading.Event())
                self.assertIs(raised.exception, failure)
                client.start_challenge.assert_not_called()

    def test_missing_challenge_does_not_open_or_create_runtime(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                client = self.client(self.challenge(unique_code="different"))
                registry = Mock()
                result = module.run_cell(self.challenge(), client, Mock(), registry,
                                         module.RowContainer(), 60.0, threading.Event())
                self.assertEqual(result.status, "error")
                self.assertEqual(result.error, "challenge_not_found")
                client.start_challenge.assert_not_called()
                registry.get_or_create.assert_not_called()

    def test_global_stop_prevents_query_or_start(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                client = self.client(self.challenge())
                stop = threading.Event()
                stop.set()
                result = module.run_cell(self.challenge(), client, Mock(), Mock(),
                                         module.RowContainer(), 60.0, stop)
                self.assertEqual(result.status, "aborted")
                self.assertEqual(client.mock_calls, [])

    def test_global_stop_during_query_prevents_start(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                client = self.client(self.challenge())
                stop = threading.Event()

                def query():
                    stop.set()
                    return [self.challenge()]

                client.list_challenges.side_effect = query
                result = module.run_cell(self.challenge(), client, Mock(), Mock(),
                                         module.RowContainer(), 60.0, stop)
                self.assertEqual(result.status, "aborted")
                client.start_challenge.assert_not_called()

    def test_platform_task_end_stops_scheduler_before_start(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"):
                client = self.client(self.challenge())
                client.list_challenges.side_effect = module.TaskEnded("evaluation ended")
                task = module.QueueTask(self.challenge(), 0)
                registry = Mock()
                scheduler = module.RoundQueueScheduler(
                    [task], client, Mock(), registry, model="model", started_iso="started")
                summary = scheduler.run_all()
                self.assertTrue(summary["global_stopped"])
                self.assertEqual(len(task.attempts), 1)
                self.assertEqual(task.result.status, "invalid_state")
                client.start_challenge.assert_not_called()
                registry.get_or_create.assert_not_called()

    def test_lost_submission_response_converges_on_next_attempt_without_reopening(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"), \
                    patch.object(module, "_drain_events", return_value=([
                        {"type": "agent.text", "payload": {"text": "flag{accepted}"}},
                    ], None)):
                progress = self.challenge(container_status="available", container_addr=["target:80"])
                operations = []

                def request(method, url, **options):
                    path = urlsplit(url).path
                    operations.append((method, path))
                    if method == "GET" and path == "/openapi/v1/challenges":
                        payload = [dict(progress)]
                    elif method == "POST" and path == "/openapi/v1/challenges/start":
                        payload = {"container_addr": ["target:80"]}
                    elif method == "POST" and path == "/openapi/v1/challenges/submit":
                        self.assertEqual(options["body"], {"unique_code": "challenge", "flag": "flag{accepted}"})
                        progress.update(correct_flag_count=1, is_completed=True, cumulative_score=500)
                        raise module.BenchmarkError("submission response lost")
                    elif method == "POST" and path == "/openapi/v1/challenges/close":
                        payload = {"closed": True}
                    else:
                        self.fail("unexpected platform request: " + method + " " + path)
                    return 200, json.dumps(payload).encode()

                client = module.SaaSClient("https://platform.test", "private")
                registry = Mock()
                registry.get_or_create.return_value = module.RuntimeSession("subject", "runtime", None)
                nor = Mock()
                nor.get_runtime.return_value = {"state": "error", "error": {"code": "AGENT_EXITED"}}
                task = module.QueueTask(self.challenge(), 0)
                scheduler = module.RoundQueueScheduler(
                    [task], client, nor, registry, model="model", started_iso="started")
                with patch.object(module, "_request", side_effect=request):
                    summary = scheduler.run_all()
                self.assertEqual(summary["completed"], 1)
                self.assertEqual(len(task.attempts), 2)
                self.assertEqual(task.attempts[0]["flags_submitted"], [])
                self.assertEqual(task.attempts[0]["correct_flag_count"], 0)
                self.assertEqual(task.result.status, "completed")
                self.assertEqual(task.result.correct_flag_count, 1)
                self.assertEqual(task.result.cumulative_score, 500)
                self.assertTrue(task.challenge["is_completed"])
                self.assertEqual(task.status, "completed")
                registry.get_or_create.assert_called_once()
                self.assertEqual(operations, [
                    ("GET", "/openapi/v1/challenges"),
                    ("POST", "/openapi/v1/challenges/start"),
                    ("GET", "/openapi/v1/challenges"),
                    ("POST", "/openapi/v1/challenges/submit"),
                    ("POST", "/openapi/v1/challenges/submit"),
                    ("POST", "/openapi/v1/challenges/close"),
                    ("GET", "/openapi/v1/challenges"),
                ])


if __name__ == "__main__":
    unittest.main()
