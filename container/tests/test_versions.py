import importlib.util
import json
import os
from pathlib import Path
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch


CONTAINER_ROOT = Path(__file__).resolve().parents[1]
IMAGES = ("nornsd", "redteam", "redteam_for_benchmark", "redteam_for_benchmark_cc")
BENCHMARK_IMAGES = IMAGES[2:]


def load_module(image: str, filename: str):
    name = f"container_test_{image}_{filename}"
    spec = importlib.util.spec_from_file_location(name, CONTAINER_ROOT / image / f"{filename}.py")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


class TemplateVersionTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.verifiers = {image: load_module(image, "verify") for image in IMAGES}

    def check_template(self, module, config, version_info):
        yaml_module = SimpleNamespace(safe_load=lambda source: config)
        with patch.object(module.importlib, "import_module", return_value=yaml_module), \
                patch.object(Path, "read_text", return_value="template"), \
                patch.object(module, "command_output", return_value=json.dumps(version_info)) as command:
            module.check_template()
            command.assert_called_once_with(["nornsd", "version", "--json"])

    def test_follows_binary_schema_without_pinning_product_or_api_versions(self):
        for image, module in self.verifiers.items():
            for schema in (1, 3, 42):
                with self.subTest(image=image, schema=schema):
                    self.check_template(module, {"VERSION": schema}, {
                        "version": "9.8.7", "api_schema": 99, "agent_config_version": schema,
                    })

    def test_rejects_mismatched_or_invalid_templates(self):
        for image, module in self.verifiers.items():
            for config in ({"VERSION": 1}, {"VERSION": "3"}, {"VERSION": True}, {}, [], None):
                with self.subTest(image=image, config=config):
                    with self.assertRaisesRegex(RuntimeError, "template VERSION must match"):
                        self.check_template(module, config, {"agent_config_version": 3})

    def test_rejects_missing_or_invalid_binary_schema(self):
        for image, module in self.verifiers.items():
            for version_info in ({}, [], None, *({"agent_config_version": value} for value in (0, -1, "3", True, 3.5))):
                with self.subTest(image=image, version_info=version_info):
                    with self.assertRaisesRegex(RuntimeError, "positive integer agent_config_version"):
                        self.check_template(module, {"VERSION": 3}, version_info)

    def test_reports_invalid_binary_output(self):
        for image, module in self.verifiers.items():
            with self.subTest(image=image), \
                    patch.object(module.importlib, "import_module", return_value=SimpleNamespace(safe_load=lambda source: {"VERSION": 3})), \
                    patch.object(Path, "read_text", return_value="template"), \
                    patch.object(module, "command_output", return_value="not JSON"):
                with self.assertRaises(json.JSONDecodeError):
                    module.check_template()


class APIHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        external = self.path.startswith("/openapi/")
        token = self.headers.get("BENCHMARK_TOKEN" if external else "Authorization")
        self.server.requests.append((self.path, token))
        if token != ("token" if external else "Bearer token"):
            self.send_error(401)
            return
        responses = {
            "/api/": {"service": "nornsd", "version": "9.8.7", "api_schema": 99},
            "/api/health": {"status": "ok"},
            "/api/subjects/subject": {"subject_id": "subject"},
            "/openapi/v1/challenges": {"challenges": []},
        }
        if self.path not in responses:
            self.send_error(404)
            return
        payload = json.dumps(responses[self.path]).encode("utf-8")
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(payload)))
        self.end_headers()
        self.wfile.write(payload)

    def log_message(self, format_string, *arguments):
        pass


class BenchmarkAPITests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.clients = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}
        cls.entrypoints = {image: load_module(image, "entrypoint") for image in BENCHMARK_IMAGES}
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), APIHandler)
        cls.server.requests = []
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def setUp(self):
        self.server.requests.clear()

    def test_config_and_client_use_stable_routes_with_authentication(self):
        for image, module in self.clients.items():
            with self.subTest(image=image), \
                    patch.multiple(module.Config, benchmark_base_url=self.base_url, benchmark_token="token", nornsd_base_urls=[self.base_url]), \
                    patch.object(module, "_read_nornsd_token", return_value="token"), \
                    patch.dict(os.environ, {"LLM_MODEL": "test-model"}):
                module.Config.validate()
                self.assertEqual(module.Config.nornsd_apis, [self.base_url + "/api"])
                client = module.NornsdClient(module.Config.nornsd_apis[0] + "/", "token")
                self.assertEqual(client.service_info(), {"service": "nornsd", "version": "9.8.7", "api_schema": 99})
                self.assertTrue(client.health())
                self.assertEqual(client.get_subject("subject"), {"subject_id": "subject"})
        self.assertEqual([path for path, token in self.server.requests], ["/api/", "/api/health", "/api/subjects/subject"] * 2)
        self.assertTrue(all(token == "Bearer token" for path, token in self.server.requests))

    def test_entrypoint_health_uses_stable_route_and_bearer_token(self):
        for image, module in self.entrypoints.items():
            with self.subTest(image=image), patch.object(module, "_received_signal", None):
                ready, error = module.wait_healthy(Mock(poll=Mock(return_value=None)), self.base_url + "/", time.monotonic() + 2, "token")
                self.assertTrue(ready, error)
        self.assertEqual(self.server.requests, [("/api/health", "Bearer token")] * 2)

    def test_external_saas_retains_its_versioned_routes(self):
        for image, module in self.clients.items():
            with self.subTest(image=image):
                client = module.SaaSClient(self.base_url, "token")
                self.assertEqual(client._call("GET", "/openapi/v1/challenges"), {"challenges": []})
        self.assertEqual(self.server.requests, [("/openapi/v1/challenges", "token")] * 2)


if __name__ == "__main__":
    unittest.main()
