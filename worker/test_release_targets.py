"""Спецификация: tools/release.sh собирает пакет под целевую ОС из TARGET_OS.

Что должно работать:
T1. TARGET_OS=darwin на не-macOS хосте → отказ (код ≠ 0), в stderr понятное сообщение
    со словом «Mac», пакет partscaster-* в dist/ не появляется.
T2. Неизвестная TARGET_OS («haiku») → отказ, в stderr упомянуто значение, пакета нет.
T3. TARGET_OS=windows на Linux → dist/partscaster-<V>-windows-amd64.zip, внутри каталог
    partscaster-<V>-windows-amd64/ с partscaster.exe, partscaster-mcp.exe, worker-<V>.tar.gz, INSTALL.txt
    (@VERSION@ заменён). Предпоследняя строка stdout — путь к zip, последняя — к архиву воркера.
    Архив воркера несёт готовую сборку yue2.cpp для Windows (yue2cpp/yue-server.exe), linux-сборки в нём нет.
    Без сборки для Windows пакет не собирается — это проверяет test_cpp_only (ТК13).
    wails вызван с `-platform windows/amd64`, go build partscaster-mcp — с GOOS=windows GOARCH=amd64.
T4. TARGET_OS не задана → пакет под хост: linux-amd64.tar.gz с partscaster (без .exe), partscaster-mcp,
    worker, INSTALL.txt; wails вызван без `-platform windows`.

Настоящие wails/go не нужны: в PATH песочницы — заглушки, которые пишут свои аргументы
и окружение в лог. Скрипт работает от корня своего репозитория, поэтому запускается копия
в песочнице tempfile; общий dist/ не трогается. Сборка yue2.cpp через docker не должна
запускаться — в песочнице подложен готовый dist/yue2cpp-linux-x86_64/yue-server (для пакета
Windows — ещё и dist/yue2cpp-windows-x86_64/yue-server.exe).
"""
import os
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
VERSION = "9.8.7-rel"
SRCHASH = "deadbeefcafe0123"

GO_STUB = r"""#!/usr/bin/env bash
# заглушка go: env / run ./cmd/srchash / build -o PATH
log="$STUB_LOG"
case "$1" in
  env)
    shift
    for v in "$@"; do
      case "$v" in
        GOHOSTOS) echo "${FAKE_HOSTOS:-linux}" ;;
        GOHOSTARCH) echo "${FAKE_HOSTARCH:-amd64}" ;;
        GOOS) echo "${GOOS:-${FAKE_HOSTOS:-linux}}" ;;
        GOARCH) echo "${GOARCH:-${FAKE_HOSTARCH:-amd64}}" ;;
        GOEXE) if [[ "${GOOS:-${FAKE_HOSTOS:-linux}}" == windows ]]; then echo .exe; else echo; fi ;;
        *) echo ;;
      esac
    done
    ;;
  run)
    echo "go run $* GOOS=${GOOS:-} GOARCH=${GOARCH:-}" >> "$log"
    echo "$SRCHASH_VALUE"
    ;;
  build)
    out=""
    prev=""
    for a in "$@"; do
      if [[ "$prev" == "-o" ]]; then out="$a"; fi
      case "$a" in -o=*) out="${a#-o=}" ;; esac
      prev="$a"
    done
    echo "go build out=$out GOOS=${GOOS:-} GOARCH=${GOARCH:-} args=$*" >> "$log"
    if [[ -n "$out" ]]; then
      mkdir -p "$(dirname "$out")"
      printf 'fake-binary\n' > "$out"
      chmod +x "$out"
    fi
    ;;
  *)
    echo "go $*" >> "$log"
    ;;
esac
"""

WAILS_STUB = r"""#!/usr/bin/env bash
# заглушка wails: пишет аргументы, кладёт build/bin/partscaster(.exe)
echo "wails $*" >> "$STUB_LOG"
mkdir -p build/bin
if [[ " $* " == *"windows/"* ]]; then
  printf 'fake-exe\n' > build/bin/partscaster.exe
else
  printf 'fake-elf\n' > build/bin/partscaster
  chmod +x build/bin/partscaster
fi
"""


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class TestReleaseTargets(unittest.TestCase):

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        base = Path(self._td.name)
        self.root = base / "repo"
        (self.root / "tools").mkdir(parents=True)
        for s in ("release.sh", "worker-tar.sh"):
            shutil.copy2(REPO / "tools" / s, self.root / "tools" / s)
        shutil.copytree(WORKER_DIR, self.root / "worker",
                        ignore=shutil.ignore_patterns("__pycache__", "*.pyc", "yue2cpp"))
        shutil.copytree(REPO / "deploy", self.root / "deploy")
        # готовая «сборка» yue2.cpp — чтобы docker-сборка не запускалась
        cpp = self.root / "dist" / "yue2cpp-linux-x86_64"
        cpp.mkdir(parents=True)
        (cpp / "yue-server").write_text("#!/bin/sh\necho usage\n")
        (cpp / "yue-server").chmod(0o755)
        (cpp / "libggml-base.so").write_bytes(b"\x7fELF fake")

        self.bin = base / "bin"
        self.bin.mkdir()
        for name, body in (("go", GO_STUB), ("wails", WAILS_STUB)):
            p = self.bin / name
            p.write_text(body)
            p.chmod(0o755)
        self.log = base / "stub.log"
        self.log.write_text("")

    def _run(self, target_os=None):
        env = {k: v for k, v in os.environ.items()
               if k not in ("TARGET_OS", "GOOS", "GOARCH", "WAILS")}
        env.update({
            "PATH": f"{self.bin}:/usr/bin:/bin",
            "WAILS": str(self.bin / "wails"),
            "STUB_LOG": str(self.log),
            "SRCHASH_VALUE": SRCHASH,
            "FAKE_HOSTOS": "linux",
            "FAKE_HOSTARCH": "amd64",
        })
        if target_os is not None:
            env["TARGET_OS"] = target_os
        return subprocess.run(["bash", str(self.root / "tools" / "release.sh"), VERSION],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=180)

    def _packages(self):
        d = self.root / "dist"
        return sorted(p.name for p in d.glob("partscaster-*")) if d.exists() else []

    def _log_lines(self, prefix):
        return [ln for ln in self.log.read_text().splitlines() if ln.startswith(prefix)]

    def _ok(self, p):
        self.assertEqual(p.returncode, 0, f"stdout:\n{p.stdout}\nstderr:\n{p.stderr}")

    # T1
    def test_darwin_on_linux_refused(self):
        p = self._run("darwin")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertIn("Mac", p.stderr)
        self.assertEqual(self._packages(), [])
        self.assertEqual(self._log_lines("wails"), [], "приложение не должно собираться")

    # T2
    def test_unknown_os_refused(self):
        p = self._run("haiku")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertIn("haiku", p.stderr)
        self.assertEqual(self._packages(), [])
        self.assertEqual(self._log_lines("wails"), [])

    # T3
    def test_windows_on_linux(self):
        # готовая сборка yue2.cpp для Windows (собирается на Windows tools/yue2cpp-build.ps1)
        win = self.root / "dist" / "yue2cpp-windows-x86_64"
        win.mkdir(parents=True)
        (win / "yue-server.exe").write_bytes(b"MZ fake")
        (win / "libggml-cuda.dll").write_bytes(b"MZ fake")
        p = self._run("windows")
        self._ok(p)
        top = f"partscaster-{VERSION}-windows-amd64"
        zpath = self.root / "dist" / f"{top}.zip"
        self.assertTrue(zpath.is_file(), self._packages())

        lines = [ln.strip() for ln in p.stdout.splitlines() if ln.strip()]
        self.assertGreaterEqual(len(lines), 2, p.stdout)
        # путь может быть относительным — от корня репозитория, где работает скрипт
        self.assertEqual((self.root / lines[-2]).resolve(), zpath.resolve())
        worker = self.root / lines[-1]
        self.assertEqual(worker.name, f"worker-{VERSION}.tar.gz")
        self.assertTrue(worker.is_file(), worker)
        # воркер в пакете Windows — своей ОС: сборка yue2.cpp для Windows, linux-сборки нет
        with tarfile.open(worker, "r:gz") as tf:
            names = tf.getnames()
        self.assertIn(f"worker-{VERSION}/yue2cpp/yue-server.exe", names)
        self.assertNotIn(f"worker-{VERSION}/yue2cpp/yue-server", names)
        self.assertNotIn(f"worker-{VERSION}/yue2cpp/libggml-base.so", names)

        with zipfile.ZipFile(zpath) as zf:
            names = {n.rstrip("/") for n in zf.namelist()}
            for f in ("partscaster.exe", "partscaster-mcp.exe", "partscaster-worker.exe",
                      f"worker-{VERSION}.tar.gz", "INSTALL.txt"):
                self.assertIn(f"{top}/{f}", names, sorted(names))
            self.assertTrue(all(n == top or n.startswith(top + "/") for n in names), sorted(names))
            install = zf.read(f"{top}/INSTALL.txt").decode("utf-8")
        self.assertNotIn("@VERSION@", install)
        self.assertIn(VERSION, install)

        wails = self._log_lines("wails")
        self.assertTrue(wails, self.log.read_text())
        self.assertTrue(any("-platform windows/amd64" in ln for ln in wails), wails)

        mcp = [ln for ln in self._log_lines("go build") if "partscaster-mcp" in ln]
        self.assertTrue(mcp, self.log.read_text())
        self.assertTrue(any("out=build/bin/partscaster-mcp.exe" in ln and "GOOS=windows" in ln
                            and "GOARCH=amd64" in ln and "./cmd/partscaster-mcp" in ln for ln in mcp), mcp)
        self.assertTrue(any("./cmd/srchash" in ln for ln in self._log_lines("go run")),
                        self.log.read_text())

    # T4
    def test_default_target_is_host(self):
        p = self._run(None)
        self._ok(p)
        top = f"partscaster-{VERSION}-linux-amd64"
        tpath = self.root / "dist" / f"{top}.tar.gz"
        self.assertTrue(tpath.is_file(), self._packages())
        self.assertFalse(any(n.endswith(".zip") for n in self._packages()), self._packages())

        with tarfile.open(tpath, "r:gz") as tf:
            names = {m.name.rstrip("/") for m in tf.getmembers()}
        for f in ("partscaster", "partscaster-mcp", f"worker-{VERSION}.tar.gz", "INSTALL.txt"):
            self.assertIn(f"{top}/{f}", names, sorted(names))
        self.assertFalse(any(n.endswith(".exe") for n in names), sorted(names))

        wails = self._log_lines("wails")
        self.assertTrue(wails, self.log.read_text())
        self.assertFalse(any("windows" in ln for ln in wails), wails)
        mcp = [ln for ln in self._log_lines("go build") if "partscaster-mcp" in ln]
        self.assertTrue(mcp, self.log.read_text())
        self.assertFalse(any("GOOS=windows" in ln for ln in mcp), mcp)


if __name__ == "__main__":
    unittest.main()
