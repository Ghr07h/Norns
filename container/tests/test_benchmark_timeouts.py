import threading
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkTimeoutTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def scheduler(self, module, count):
        tasks = [module.QueueTask({"unique_code": str(index), "flag_count": 1}, index) for index in range(count)]
        return module.RoundQueueScheduler(
            tasks, Mock(), Mock(), Mock(), model="model", started_iso="started",
        )

    def result(self, module, task, completed=True):
        return module.ChallengeResult(
            unique_code=task.code, flag_count=1,
            correct_flag_count=1 if completed else 0,
            is_completed=completed,
            status="completed" if completed else "ended",
        )

    def test_queued_tasks_do_not_enable_tail_mode_before_dispatch(self):
        for image, module in self.modules.items():
            for count in (0, 1, 2, 3, 4):
                with self.subTest(image=image, count=count):
                    scheduler = self.scheduler(module, count)
                    self.assertFalse(scheduler.timeout_disabled.is_set())

    def test_policy_requires_all_remaining_tasks_running_up_to_three_inclusive(self):
        for image, module in self.modules.items():
            for count in (1, 2, 3, 4):
                with self.subTest(image=image, count=count):
                    scheduler = self.scheduler(module, count)
                    with scheduler.lock:
                        for index, task in enumerate(scheduler.tasks):
                            task.running = True
                            scheduler._update_timeout_policy_locked()
                            self.assertEqual(
                                scheduler.timeout_disabled.is_set(),
                                index == count - 1 and count <= 3,
                            )

    def test_pending_retries_and_queued_tasks_still_count_as_unfinished(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"):
                scheduler = self.scheduler(module, 4)
                for task in scheduler.tasks[:3]:
                    task.running = True
                scheduler._record_result(scheduler.tasks[0], self.result(module, scheduler.tasks[0], False), 1, "started", 1, 40)
                self.assertFalse(scheduler.timeout_disabled.is_set())
                scheduler._record_result(scheduler.tasks[1], self.result(module, scheduler.tasks[1]), 2, "started", 1, 40)
                self.assertFalse(scheduler.timeout_disabled.is_set())
                with scheduler.lock:
                    scheduler.tasks[3].running = True
                    scheduler._update_timeout_policy_locked()
                    self.assertFalse(scheduler.timeout_disabled.is_set())
                    scheduler.tasks[0].running = True
                    scheduler._update_timeout_policy_locked()
                self.assertTrue(scheduler.timeout_disabled.is_set())
                self.assertEqual([task.status for task in scheduler.tasks], ["pending", "completed", "pending", "pending"])

    def run_cell_with_clock(self, module, *, initially_disabled=False,
                            disable_after_poll=False, overall_deadline=None,
                            stop_after_poll=False, pass_event=True):
        clock = SimpleNamespace(now=0.0)
        timeout_disabled = threading.Event()
        if initially_disabled:
            timeout_disabled.set()
        stop_event = threading.Event()
        registry = Mock()
        registry.get_or_create.return_value = module.RuntimeSession("subject", "runtime", None)
        nor = Mock()
        polls = []

        def runtime_state(*arguments):
            polls.append(arguments)
            if len(polls) == 1:
                clock.now = 10.0
                if disable_after_poll:
                    timeout_disabled.set()
                if stop_after_poll:
                    stop_event.set()
                return {"state": "running"}
            return {"state": "ended"}

        nor.get_runtime.side_effect = runtime_state
        with patch.object(module, "procure_container", return_value=SimpleNamespace(status="ready", addrs=[])), \
                patch.object(module, "_drain_events", return_value=([], None)), \
                patch.object(module, "_stop_and_wait") as stop_runtime, \
                patch.object(module.time, "monotonic", side_effect=lambda: clock.now), \
                patch.object(module.time, "sleep"), \
                patch.object(module.Config, "max_continue", 0):
            result = module.run_cell(
                {"unique_code": "challenge", "flag_count": 1}, Mock(), nor, registry,
                module.RowContainer(), 1.0, stop_event,
                overall_deadline=overall_deadline,
                timeout_disabled=timeout_disabled if pass_event else None,
            )
            stop_runtime.assert_called_once()
            nor.submit_instruction.assert_not_called()
        return result, len(polls)

    def test_running_runtime_ignores_expired_deadline_after_policy_changes(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                result, polls = self.run_cell_with_clock(module, disable_after_poll=True)
                self.assertEqual(result.status, "ended")
                self.assertEqual(polls, 2)

    def test_timeout_is_preserved_without_tail_policy(self):
        for image, module in self.modules.items():
            for pass_event in (True, False):
                with self.subTest(image=image, pass_event=pass_event):
                    result, polls = self.run_cell_with_clock(module, pass_event=pass_event)
                    self.assertEqual(result.status, "timeout")
                    self.assertEqual(polls, 1)

    def test_initially_underfilled_runtime_runs_past_its_deadline(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                result, polls = self.run_cell_with_clock(module, initially_disabled=True)
                self.assertEqual(result.status, "ended")
                self.assertEqual(polls, 2)

    def test_global_deadline_and_stop_still_apply_when_single_task_timeout_is_disabled(self):
        for image, module in self.modules.items():
            with self.subTest(image=image, condition="global deadline"):
                result, polls = self.run_cell_with_clock(module, initially_disabled=True, overall_deadline=1.0)
                self.assertEqual(result.status, "timeout")
                self.assertEqual(polls, 1)
            with self.subTest(image=image, condition="stop"):
                result, polls = self.run_cell_with_clock(module, initially_disabled=True, stop_after_poll=True)
                self.assertEqual(result.status, "aborted")
                self.assertEqual(polls, 1)

    def test_existing_workers_switch_to_unlimited_without_restarting(self):
        for image, module in self.modules.items():
            for count in (1, 2, 3, 4, 5):
                with self.subTest(image=image, count=count), \
                        patch.object(module, "write_results"), \
                        patch.object(module, "_close_row_container"):
                    scheduler = self.scheduler(module, count)
                    started = threading.Barrier(min(count, 3), timeout=2)
                    calls = []
                    errors = []
                    lock = threading.Lock()

                    def run_cell(challenge, *arguments, **options):
                        code = challenge["unique_code"]
                        with lock:
                            calls.append(code)
                        if code in ("0", "1", "2"):
                            try:
                                started.wait()
                            except threading.BrokenBarrierError:
                                errors.append("initial workers did not start together")
                        if count <= 3 or code in ("1", "2", str(count - 1)):
                            if not options["timeout_disabled"].wait(2):
                                errors.append("running worker did not observe unlimited tail mode")
                        elif options["timeout_disabled"].is_set():
                            errors.append("timeout disabled while work was still queued")
                        return self.result(module, scheduler.tasks[int(code)])

                    with patch.object(module, "run_cell", side_effect=run_cell):
                        summary = scheduler.run_all()
                    self.assertEqual(errors, [])
                    self.assertEqual(sorted(calls), [str(index) for index in range(count)])
                    self.assertEqual(summary["completed"], count)
                    self.assertTrue(summary["timeout_disabled"])
                    self.assertFalse(any(task.running for task in scheduler.tasks))
                    unlimited_indices = range(count) if count <= 3 else (1, 2, count - 1)
                    for index in unlimited_indices:
                        self.assertEqual(len(scheduler.tasks[index].attempts), 1)
                        self.assertTrue(scheduler.tasks[index].attempts[0]["timeout_disabled"])

    def test_underfilled_queue_passes_timeout_policy_to_retries(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"), \
                    patch.multiple(module.Config, initial_limit_minutes=40.0, increment_minutes=30.0, max_challenge_timeout_minutes=240.0):
                scheduler = self.scheduler(module, 2)
                for task in scheduler.tasks:
                    task.attempts.append({"round": 1, "timeout_minutes": 40})
                options_seen = []

                def run_cell(challenge, *arguments, **options):
                    options_seen.append(options)
                    return self.result(module, scheduler.tasks[int(challenge["unique_code"])])

                with patch.object(module, "run_cell", side_effect=run_cell):
                    summary = scheduler.run_all()
                self.assertEqual(summary["completed"], 2)
                self.assertEqual(summary["unresolvable"], 0)
                self.assertEqual(len(options_seen), 2)
                self.assertTrue(all(options["timeout_disabled"].is_set() for options in options_seen))
                self.assertTrue(all(task.attempts[-1]["timeout_minutes"] == 70 for task in scheduler.tasks))

    def test_retry_ceiling_is_preserved_when_runtime_timeout_is_disabled(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), \
                    patch.object(module, "write_results"), \
                    patch.object(module, "_close_row_container"), \
                    patch.multiple(module.Config, initial_limit_minutes=40.0, increment_minutes=30.0, max_challenge_timeout_minutes=40.0):
                scheduler = self.scheduler(module, 2)
                scheduler.timeout_disabled.set()
                for task in scheduler.tasks:
                    task.attempts.append({"round": 1, "timeout_minutes": 40})

                def run_cell(challenge, *arguments, **options):
                    return self.result(module, scheduler.tasks[int(challenge["unique_code"])])

                with patch.object(module, "run_cell", side_effect=run_cell) as attempt:
                    summary = scheduler.run_all()
                self.assertEqual(summary["unresolvable"], 2)
                self.assertEqual(summary["completed"], 0)
                attempt.assert_not_called()
                self.assertTrue(summary["timeout_disabled"])

    def test_global_stop_prevents_dispatch_even_when_timeouts_are_disabled(self):
        for image, module in self.modules.items():
            with self.subTest(image=image), patch.object(module, "write_results"):
                scheduler = self.scheduler(module, 2)
                scheduler.timeout_disabled.set()
                scheduler.request_stop()
                with patch.object(module, "run_cell") as attempt:
                    summary = scheduler.run_all()
                attempt.assert_not_called()
                self.assertTrue(summary["global_stopped"])


if __name__ == "__main__":
    unittest.main()
