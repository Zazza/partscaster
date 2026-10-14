"""Спецификация архива воркера tools/worker-tar.sh.

Что должно работать:
A1. `tools/worker-tar.sh OUTDIR [VERSION]` создаёт OUTDIR/worker-<VERSION>.tar.gz и печатает
    его путь последней строкой stdout; без VERSION — версия из `git describe --tags --always --dirty`.
A2. Внутри один каталог верхнего уровня worker-<VERSION>/.
A3. В архиве: модули воркера *.py (кроме test_*), *.json, requirements*.txt, seedvc_install.sh,
    install.sh, VERSION (= версии), phrases/ (как worker/phrases), units/partscaster-worker.service.
A4. Список — одним правилом: каждый локальный модуль, который импортирует код воркера,
    есть в архиве (вычисляется по исходникам); тестов и __pycache__ в архиве нет.
"""
import ast
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
SCRIPT = REPO / "tools" / "worker-tar.sh"
VERSION = "9.8.7-test"


def build_archive(outdir, version=None):
    """Запуск tools/worker-tar.sh; возвращает (CompletedProcess, путь из последней строки)."""
    args = ["bash", str(SCRIPT), str(outdir)] + ([version] if version else [])
    p = subprocess.run(args, cwd=REPO, capture_output=True, text=True, timeout=120)
    lines = [ln for ln in p.stdout.splitlines() if ln.strip()]
    return p, (Path(lines[-1].strip()) if lines else None)


def worker_modules():
    """Код воркера: worker/*.py без тестов."""
    return sorted(p.name for p in WORKER_DIR.glob("*.py") if not p.name.startswith("test_"))


def imported_local_modules():
    """Локальные модули, которые импортирует код воркера (любой import/from, в том числе
    внутри функций): имя верхнего уровня, для которого в worker/ есть <имя>.py или пакет."""
    found = set()
    for name in worker_modules():
        tree = ast.parse((WORKER_DIR / name).read_text(encoding="utf-8"), filename=name)
        for node in ast.walk(tree):
            mods = []
            if isinstance(node, ast.Import):
                mods = [a.name for a in node.names]
            elif isinstance(node, ast.ImportFrom) and node.level == 0 and node.module:
                mods = [node.module]
            for m in mods:
                top = m.split(".")[0]
                if (WORKER_DIR / f"{top}.py").is_file():
                    found.add(f"{top}.py")
                elif (WORKER_DIR / top / "__init__.py").is_file():
                    found.add(f"{top}/__init__.py")
    return sorted(found)


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class TestWorkerArchive(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._td = tempfile.TemporaryDirectory()
        cls.out = Path(cls._td.name) / "dist"
        cls.out.mkdir()
        cls.proc, cls.path = build_archive(cls.out, VERSION)
        cls.names = []
        cls.top = f"worker-{VERSION}"
        if cls.proc.returncode == 0 and cls.path and cls.path.is_file():
            with tarfile.open(cls.path, "r:gz") as tf:
                cls.members = {m.name.rstrip("/"): m for m in tf.getmembers()}
                cls.names = sorted(cls.members)
                cls.version_text = None
                vm = cls.members.get(f"{cls.top}/VERSION")
                if vm is not None:
                    cls.version_text = tf.extractfile(vm).read().decode()

    @classmethod
    def tearDownClass(cls):
        cls._td.cleanup()

    def setUp(self):
        self.assertEqual(self.proc.returncode, 0,
                         f"worker-tar.sh failed:\nstdout:\n{self.proc.stdout}\nstderr:\n{self.proc.stderr}")

    def rel(self):
        """Пути внутри каталога верхнего уровня (только файлы)."""
        pre = self.top + "/"
        return {n[len(pre):] for n, m in self.members.items() if n.startswith(pre) and m.isfile()}

    def test_path_printed_and_named(self):
        self.assertEqual(self.path, self.out / f"worker-{VERSION}.tar.gz")
        self.assertTrue(self.path.is_file())

    def test_single_top_dir(self):
        tops = {n.split("/")[0] for n in self.names}
        self.assertEqual(tops, {self.top})

    def test_every_imported_local_module_present(self):
        imported = imported_local_modules()
        self.assertTrue(imported, "в коде воркера не нашлось локальных импортов — опора теста потеряна")
        missing = sorted(set(imported) - self.rel())
        self.assertEqual(missing, [], "модули, импортируемые воркером, но не попавшие в архив")

    def test_all_worker_modules_present(self):
        missing = sorted(set(worker_modules()) - self.rel())
        self.assertEqual(missing, [])

    def test_no_tests_no_cache(self):
        bad = sorted(r for r in self.rel()
                     if Path(r).name.startswith("test_") or "__pycache__" in r or r.endswith(".pyc"))
        self.assertEqual(bad, [])

    def test_json_and_requirements(self):
        rel = self.rel()
        jsons = [p.name for p in WORKER_DIR.glob("*.json")]
        self.assertTrue(jsons, "в worker/ нет *.json — опора теста потеряна")
        reqs = [p.name for p in WORKER_DIR.glob("requirements*.txt")]
        self.assertIn("requirements.txt", reqs)
        for name in jsons + reqs + ["seedvc_install.sh", "install.sh"]:
            self.assertIn(name, rel)

    def test_version_file(self):
        self.assertIn("VERSION", self.rel())
        self.assertEqual(self.version_text.strip(), VERSION)

    def test_unit(self):
        self.assertIn("units/partscaster-worker.service", self.rel())

    def test_phrases_like_worker(self):
        want = {f"phrases/{p.name}" for p in (WORKER_DIR / "phrases").iterdir() if p.is_file()}
        self.assertTrue(want, "worker/phrases пуст — опора теста потеряна")
        got = {r for r in self.rel() if r.startswith("phrases/")}
        self.assertEqual(got, want)


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar") and shutil.which("git"), "нужны bash, tar, git")
class TestWorkerArchiveDefaultVersion(unittest.TestCase):
    """A1: без VERSION — версия из git describe --tags --always --dirty."""

    def test_version_from_git(self):
        want = subprocess.run(["git", "describe", "--tags", "--always", "--dirty"], cwd=REPO,
                              capture_output=True, text=True).stdout.strip()
        self.assertTrue(want)
        with tempfile.TemporaryDirectory() as td:
            proc, path = build_archive(td)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(path, Path(td) / f"worker-{want}.tar.gz")
            with tarfile.open(path, "r:gz") as tf:
                v = tf.extractfile(f"worker-{want}/VERSION").read().decode().strip()
            self.assertEqual(v, want)


if __name__ == "__main__":
    unittest.main()
