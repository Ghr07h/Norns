from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
import zipfile


PROJECT_ROOT = Path(__file__).resolve().parents[2]
IMAGES = ("nornsd", "redteam", "redteam_for_benchmark", "redteam_for_benchmark_cc")
TEMPLATE_PATH = "nornsd/configs/nornsd/agent.example.yaml"


class ConfigTemplateTests(unittest.TestCase):
    def test_docker_context_includes_relocated_template(self):
        for image in IMAGES:
            with self.subTest(image=image):
                directory = PROJECT_ROOT / "container" / image
                dockerfile = (directory / "Dockerfile").read_text()
                self.assertIn(f"COPY {TEMPLATE_PATH} /norns/config/agent.example.yaml", dockerfile)
                rules = (directory / "Dockerfile.dockerignore").read_text().splitlines()
                for allowed in ("nornsd/", "nornsd/configs/", "nornsd/configs/nornsd/", TEMPLATE_PATH):
                    self.assertIn(f"!{allowed}", rules)
                self.assertNotIn("nornsd/internal/config/", dockerfile)

    @unittest.skipUnless(shutil.which("zip") and shutil.which("bash"), "zip and bash are required")
    def test_packages_include_relocated_template_without_user_configs(self):
        for image in IMAGES:
            with self.subTest(image=image), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                directory = root / "container" / image
                directory.mkdir(parents=True)
                for name in ("package.sh", "Dockerfile", "Dockerfile.dockerignore", "build.sh", "entrypoint.py", "verify.py", "benchmark.py"):
                    source = PROJECT_ROOT / "container" / image / name
                    if source.exists():
                        shutil.copy2(source, directory / name)
                template = root / TEMPLATE_PATH
                template.parent.mkdir(parents=True)
                shutil.copy2(PROJECT_ROOT / TEMPLATE_PATH, template)
                for platform in ("linux-arm64", "linux-x64"):
                    release = root / "release" / platform
                    release.mkdir(parents=True)
                    for executable in ("norns", "nornsd"):
                        (release / executable).write_text("binary fixture")
                domain = root / "domain/redteam/norns_domain_agents"
                domain.mkdir(parents=True)
                (domain / "meta.json").write_text("{}")
                skills = root / "domain/redteam/skills"
                skills.mkdir()
                (skills / "fixture.md").write_text("skill fixture")
                (root / "bin").mkdir()
                (root / "bin/agent.yaml").write_text("private user config")
                result = subprocess.run(["bash", str(directory / "package.sh")], capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                with zipfile.ZipFile(directory / f"{image}.zip") as archive:
                    self.assertEqual(archive.read(TEMPLATE_PATH), template.read_bytes())
                    self.assertFalse(any(name.startswith(("bin/", "nornsd/internal/config/")) for name in archive.namelist()))


if __name__ == "__main__":
    unittest.main()
