"""Спецификация: архив воркера несёт собранный yue2.cpp.

Что должно работать:
T1. Есть dist/yue2cpp-linux-x86_64/ (yue-server + libggml*) → tools/worker-tar.sh кладёт его
    в архив как worker-<v>/yue2cpp/ (yue-server исполняемый, библиотеки рядом).
T2. Каталога нет → архив прежний: без yue2cpp/ и без ошибки.
T3. TARGET_OS=windows → из dist/yue2cpp-windows-x86_64/ (yue-server.exe + libggml*.dll);
    linux-сборка при этом в архив не попадает; без TARGET_OS — прежнее поведение (linux).

Скрипт работает от корня своего репозитория (dist/ — там), поэтому тест запускает копию
скрипта в песочнице: tools/worker-tar.sh, worker/, deploy/units/ — общий dist/ не трогается.
"""
import os
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

    def _build(self, target_os=None):
        env = dict(os.environ)
        if target_os:
            env["TARGET_OS"] = target_os
        p = subprocess.run(["bash", str(self.root / "tools" / "worker-tar.sh"), str(self.out), VERSION],
                           cwd=self.root, capture_output=True, text=True, timeout=120, env=env)
        self.assertEqual(p.returncode, 0, f"stdout:\n{p.stdout}\nstderr:\n{p.stderr}")
        # последняя строка — путь архива (под Git Bash — в MSYS-форме /tmp/…, сверяем имя);
        # сам архив открываем по ожидаемому месту: скрипт обязан положить его в OUT
        echo = [ln for ln in p.stdout.splitlines() if ln.strip()][-1].strip()
        self.assertEqual(Path(echo).name, f"{TOP}.tar.gz")
        path = self.out / f"{TOP}.tar.gz"
        self.assertTrue(path.is_file(), f"{path} не создан")
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

    def test_windows_build_target_os_windows(self):
        # TARGET_OS=windows: едет windows-сборка (yue-server.exe + libggml*.dll), а не linux-файлы
        d = self.root / "dist" / "yue2cpp-windows-x86_64"
        d.mkdir(parents=True)
        (d / "yue-server.exe").write_bytes(b"MZ fake")
        (d / "libggml-cuda.dll").write_bytes(b"MZ fake")
        m = self._build(target_os="windows")
        self.assertIn(f"{TOP}/yue2cpp/yue-server.exe", m)
        self.assertIn(f"{TOP}/yue2cpp/libggml-cuda.dll", m)

    def test_windows_target_ignores_linux_build(self):
        # пакет Windows с linux-сборкой рядом: чужая ОС не попадает в архив
        d = self.root / "dist" / "yue2cpp-linux-x86_64"
        d.mkdir(parents=True)
        (d / "yue-server").write_text("#!/bin/sh\n")
        m = self._build(target_os="windows")
        self.assertEqual([n for n in m if "/yue2cpp" in n], [])

    def test_default_target_keeps_linux(self):
        # без TARGET_OS поведение прежнее: linux-каталог
        d = self.root / "dist" / "yue2cpp-linux-x86_64"
        d.mkdir(parents=True)
        (d / "yue-server").write_text("#!/bin/sh\n")
        (d / "yue-server").chmod(0o755)
        m = self._build()
        self.assertIn(f"{TOP}/yue2cpp/yue-server", m)


if __name__ == "__main__":
    unittest.main()
