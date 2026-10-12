"""Спецификация: архив воркера несёт собранный yue2.cpp.

Что должно работать:
T1. Есть dist/yue2cpp-linux-x86_64/ (yue-server + libggml*) → tools/worker-tar.sh кладёт его
    в архив как worker-<v>/yue2cpp/ (yue-server исполняемый, библиотеки рядом).
T2. Каталога нет → архив прежний: без yue2cpp/ и без ошибки.

Скрипт работает от корня своего репозитория (dist/ — там), поэтому тест запускает копию
скрипта в песочнице: tools/worker-tar.sh, worker/, deploy/units/ — общий dist/ не трогается.
"""
import shutil
import subprocess
import tarfile
import tempfile
import unittest
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
VERSION = "9.8.7-cpp"
TOP = f"worker-{VERSION}"


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class TestWorkerTarEngine(unittest.TestCase):

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.root = Path(self._td.name) / "repo"
        (self.root / "tools").mkdir(parents=True)
        shutil.copy2(REPO / "tools" / "worker-tar.sh", self.root / "tools" / "worker-tar.sh")
        shutil.copytree(WORKER_DIR, self.root / "worker",
                        ignore=shutil.ignore_patterns("__pycache__", "*.pyc", "yue2cpp"))
        shutil.copytree(REPO / "deploy" / "units", self.root / "deploy" / "units")
        self.out = Path(self._td.name) / "out"

    def _build(self):
        p = subprocess.run(["bash", str(self.root / "tools" / "worker-tar.sh"), str(self.out), VERSION],
                           cwd=self.root, capture_output=True, text=True, timeout=120)
        self.assertEqual(p.returncode, 0, f"stdout:\n{p.stdout}\nstderr:\n{p.stderr}")
        path = Path([ln for ln in p.stdout.splitlines() if ln.strip()][-1].strip())
        self.assertEqual(path, self.out / f"{TOP}.tar.gz")
        with tarfile.open(path, "r:gz") as tf:
            return {m.name.rstrip("/"): m for m in tf.getmembers()}

    def test_build_included(self):
        d = self.root / "dist" / "yue2cpp-linux-x86_64"
        d.mkdir(parents=True)
        (d / "yue-server").write_text("#!/bin/sh\necho usage\n")
        (d / "yue-server").chmod(0o755)
        (d / "libggml-base.so").write_bytes(b"\x7fELF fake")
        (d / "libggml-cuda.so").write_bytes(b"\x7fELF fake")
        m = self._build()
        srv = m.get(f"{TOP}/yue2cpp/yue-server")
        self.assertIsNotNone(srv, sorted(n for n in m if "yue2cpp" in n))
        self.assertTrue(srv.isfile())
        self.assertTrue(srv.mode & 0o100, "yue-server в архиве не исполняемый")
        for lib in ("libggml-base.so", "libggml-cuda.so"):
            self.assertIn(f"{TOP}/yue2cpp/{lib}", m)
        # остальной архив на месте
        self.assertIn(f"{TOP}/yue_worker.py", m)
        self.assertIn(f"{TOP}/VERSION", m)

    def test_no_build_no_dir(self):
        self.assertFalse((self.root / "dist").exists())
        m = self._build()
        self.assertEqual([n for n in m if "/yue2cpp" in n], [])
        self.assertIn(f"{TOP}/yue_worker.py", m)
        self.assertIn(f"{TOP}/install.sh", m)

    def test_empty_dist_dir_no_error(self):
        (self.root / "dist").mkdir()
        m = self._build()
        self.assertEqual([n for n in m if "/yue2cpp" in n], [])


if __name__ == "__main__":
    unittest.main()
