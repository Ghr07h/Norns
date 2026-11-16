import os
import unittest
from unittest.mock import patch

from test_versions import BENCHMARK_IMAGES, load_module


class BenchmarkFlagTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        with patch.dict(os.environ, {"FLAG_PATTERN": ""}):
            cls.modules = {image: load_module(image, "benchmark") for image in BENCHMARK_IMAGES}

    def test_rejects_backticks_quotes_and_whitespace_inside_flags(self):
        forbidden = ("`", "'", '"', " ", "\t", "\n", "\r", "\v", "\f", "\u00a0", "\u3000")
        for image, module in self.modules.items():
            for character in forbidden:
                for payload in (f"{character}value", f"some{character}value", f"value{character}"):
                    with self.subTest(image=image, payload=payload):
                        self.assertEqual(module.extract_flags(f"flag{{{payload}}}"), set())

    def test_extracts_valid_flags_with_quoted_or_markdown_wrappers(self):
        flags = {"flag{0638fa84-7352-4a19-9b19-786a195f4134}", "FLAG{Mixed_Case-123}", "flag{x}"}
        for image, module in self.modules.items():
            for wrapper in ("`", "'", '"', " ", "\n"):
                with self.subTest(image=image, wrapper=wrapper):
                    text = "\n".join(f"{wrapper}{flag}{wrapper}" for flag in flags)
                    self.assertEqual(module.extract_flags(text), flags)

    def test_preserves_length_brace_and_placeholder_restrictions(self):
        invalid = ("flag{}", "flag{" + "a" * 51 + "}", "flag{a.b}", "flag{a{b}c}",
                   "flag{...}", "flag{原文}", "flag{example_flag_value}")
        valid = "flag{" + "a" * 50 + "}"
        for image, module in self.modules.items():
            with self.subTest(image=image):
                self.assertEqual(module.extract_flags("\n".join(invalid)), set())
                self.assertEqual(module.extract_flags(valid), {valid})
                self.assertEqual(module.extract_flags(""), set())

    def test_invalid_candidates_do_not_hide_nearby_valid_flags(self):
        for image, module in self.modules.items():
            with self.subTest(image=image):
                text = "flag{bad value} flag{bad`value} flag{good-value} flag{good-value}"
                self.assertEqual(module.extract_flags(text), {"flag{good-value}"})

    def test_environment_can_still_override_default_pattern(self):
        for image in BENCHMARK_IMAGES:
            with self.subTest(image=image), patch.dict(os.environ, {"FLAG_PATTERN": r"answer\{[a-z-]+\}"}):
                module = load_module(image, "benchmark")
                self.assertEqual(module.extract_flags("answer{custom-value} flag{default-value}"),
                                 {"answer{custom-value}"})


if __name__ == "__main__":
    unittest.main()
