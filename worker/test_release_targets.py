"""Спецификация: tools/release.sh собирает пакет под целевую ОС из TARGET_OS.

Что должно работать:
T1. TARGET_OS=darwin на не-macOS хосте → отказ (код ≠ 0), в stderr понятное сообщение
    со словом «Mac», пакет partscaster-* в dist/ не появляется.
T2. Неизвестная TARGET_OS («haiku») → отказ, в stderr упомянуто значение, пакета нет.
T3. TARGET_OS=windows на Linux → dist/partscaster-<V>-windows-amd64.zip, внутри каталог
    partscaster-<V>-windows-amd64/ с partscaster.exe, partscaster-mcp.exe, worker-<V>.tar.gz, INSTALL.txt
    (@VERSION@ заменён). Предпоследняя строка stdout — путь к zip, последняя — к архиву воркера.
    internal-win-pkg-dedupe: архив воркера рядом в zip — для Linux/WSL: без yue2cpp/yue-server.exe и
    libggml-cuda.dll, с Linux-сборкой yue2.cpp, если она есть в dist (ТК1). Архив с yue2.cpp под Windows
    лежит в cmd/partscaster-worker/payload в момент go build ./cmd/partscaster-worker (ТК1).
    Без dist/yue2cpp-windows-x86_64 пакет Windows не собирается, ошибка про yue2.cpp (ТК2).
    wails вызван с `-platform windows/amd64`, go build partscaster-mcp — с GOOS=windows GOARCH=amd64.
T5. Непустая WAILS_TAGS → wails build получает `-tags <значение>`; не задана или пустая — `-tags` нет
    (internal-ci-wails-tags ТК1).
T4. TARGET_OS не задана → пакет под хост: linux-amd64.tar.gz с partscaster (без .exe), partscaster-mcp,
    worker, INSTALL.txt; wails вызван без `-platform windows`. В архиве воркера — Linux-сборка yue2.cpp
    (yue2cpp/yue-server), сборки под Windows нет (internal-win-pkg-dedupe ТК3).

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
    # сборка воркера Windows: что лежит в payload в момент go build (go:embed) — список файлов архивов в журнал
    if [[ " $* " == *" ./cmd/partscaster-worker "* ]]; then
      for t in cmd/partscaster-worker/payload/*.tar.gz; do
        [[ -f "$t" ]] || continue
        echo "payload-archive $(basename "$t")" >> "$log"
        tar -tzf "$t" | sed 's/^/payload-file /' >> "$log"
      done
    fi
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

    def _run(self, target_os=None, wails_tags=None):
        env = {k: v for k, v in os.environ.items()
               if k not in ("TARGET_OS", "GOOS", "GOARCH", "WAILS", "WAILS_TAGS")}
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
        if wails_tags is not None:
            env["WAILS_TAGS"] = wails_tags
        return subprocess.run(["bash", str(self.root / "tools" / "release.sh"), VERSION],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=180)

    def _packages(self):
        d = self.root / "dist"
        return sorted(p.name for p in d.glob("partscaster-*")) if d.exists() else []

    def _log_lines(self, prefix):
        return [ln for ln in self.log.read_text().splitlines() if ln.startswith(prefix)]

    def _ok(self, p):
        self.assertEqual(p.returncode, 0, f"stdout:\n{p.stdout}\nstderr:\n{p.stderr}")

    def _payload_files(self):
        return {ln.split(" ", 1)[1].rstrip("/") for ln in self._log_lines("payload-file ")}

    @staticmethod
    def _tar_names_from_bytes(data):
        import io
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tf:
            return {n.rstrip("/") for n in tf.getnames()}

    def _prepare_windows(self, with_win_cpp=True):
        if with_win_cpp:
            win = self.root / "dist" / "yue2cpp-windows-x86_64"
            win.mkdir(parents=True)
            (win / "yue-server.exe").write_bytes(b"MZ fake")
            (win / "libggml-cuda.dll").write_bytes(b"MZ fake")
        (self.root / "cmd" / "partscaster-worker").mkdir(parents=True)
        (self.root / "cmd" / "partscaster-worker" / "icon.ico").write_bytes(b"\x00\x00\x01\x00 fake")
        # настоящая сборка yue2.cpp (docker/powershell) не запускается: заглушки с отказом и записью в журнал
        for name in ("docker", "nvcc", "powershell"):
            p = self.bin / name
            p.write_text(f'#!/bin/sh\necho "{name} $*" >> "$STUB_LOG"\nexit 1\n')
            p.chmod(0o755)

    # internal-win-pkg-dedupe ТК2: без сборки yue2.cpp под Windows — отказ с сообщением про неё
    def test_windows_without_win_cpp_fails(self):
        self._prepare_windows(with_win_cpp=False)
        p = self._run("windows")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        out = (p.stdout + p.stderr).lower()
        self.assertIn("yue2", out, p.stderr)
        self.assertIn("windows", out, p.stderr)
        self.assertFalse(any(n.endswith(".zip") for n in self._packages()), self._packages())
        self.assertEqual(self._log_lines("payload-archive"), [], "воркер Windows не должен собираться")

    # internal-win-pkg-dedupe ТК1 (край): Linux-сборки в dist нет — пакет Windows собирается,
    # архив рядом без yue2.cpp вовсе (install.sh соберёт), в payload — сборка под Windows
    def test_windows_without_linux_cpp(self):
        self._prepare_windows()
        shutil.rmtree(self.root / "dist" / "yue2cpp-linux-x86_64")
        p = self._run("windows")
        self._ok(p)
        top = f"partscaster-{VERSION}-windows-amd64"
        with zipfile.ZipFile(self.root / "dist" / f"{top}.zip") as zf:
            inner = self._tar_names_from_bytes(zf.read(f"{top}/worker-{VERSION}.tar.gz"))
        self.assertFalse(any(n.startswith(f"worker-{VERSION}/yue2cpp/") for n in inner), sorted(inner))
        self.assertIn(f"worker-{VERSION}/yue2cpp/yue-server.exe", self._payload_files(), self.log.read_text())
        self.assertEqual(self._log_lines("docker"), [], "docker-сборка yue2.cpp не должна запускаться")

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
        # готовая сборка yue2.cpp для Windows (tools/yue2cpp-build.ps1) и каталог воркера Windows с иконкой
        self._prepare_windows()
        p = self._run("windows")
        self._ok(p)
        winres = [ln for ln in self._log_lines("go tool") if "go-winres" in ln]
        self.assertTrue(any("--icon icon.ico" in ln and f"--product-version {VERSION}" in ln for ln in winres),
                        self.log.read_text())
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
        # ТК1: в payload при go build ./cmd/partscaster-worker — архив воркера с yue2.cpp под Windows
        payload = self._payload_files()
        self.assertIn(f"worker-{VERSION}/yue2cpp/yue-server.exe", payload, self.log.read_text())
        self.assertIn(f"worker-{VERSION}/yue2cpp/libggml-cuda.dll", payload, self.log.read_text())

        with zipfile.ZipFile(zpath) as zf:
            inner = self._tar_names_from_bytes(zf.read(f"{top}/worker-{VERSION}.tar.gz"))
        # ТК1: архив рядом в пакете — для Linux/WSL: без программы под Windows, с Linux-сборкой из dist
        self.assertNotIn(f"worker-{VERSION}/yue2cpp/yue-server.exe", inner, sorted(inner))
        self.assertNotIn(f"worker-{VERSION}/yue2cpp/libggml-cuda.dll", inner, sorted(inner))
        self.assertFalse(any(n.endswith(".exe") or n.endswith(".dll") for n in inner), sorted(inner))
        self.assertIn(f"worker-{VERSION}/yue2cpp/yue-server", inner, sorted(inner))

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
        # ТК3 (internal-win-pkg-dedupe): архив воркера в пакете Linux — с Linux-сборкой yue2.cpp
        with tarfile.open(tpath, "r:gz") as tf:
            inner = self._tar_names_from_bytes(tf.extractfile(f"{top}/worker-{VERSION}.tar.gz").read())
        self.assertIn(f"worker-{VERSION}/yue2cpp/yue-server", inner, sorted(inner))
        self.assertIn(f"worker-{VERSION}/yue2cpp/libggml-base.so", inner, sorted(inner))
        self.assertNotIn(f"worker-{VERSION}/yue2cpp/yue-server.exe", inner, sorted(inner))

        wails = self._log_lines("wails")
        self.assertTrue(wails, self.log.read_text())
        self.assertFalse(any("windows" in ln for ln in wails), wails)
        mcp = [ln for ln in self._log_lines("go build") if "partscaster-mcp" in ln]
        self.assertTrue(mcp, self.log.read_text())
        self.assertFalse(any("GOOS=windows" in ln for ln in mcp), mcp)


    # T5 (internal-ci-wails-tags ТК1): WAILS_TAGS → флаг -tags у wails build
    def _wails_build_args(self):
        lines = self._log_lines("wails")
        self.assertTrue(lines, self.log.read_text())
        return [ln.split()[1:] for ln in lines if "build" in ln.split()[1:2]] or \
            [ln.split()[1:] for ln in lines]

    def test_wails_tags_passed_as_flag(self):
        p = self._run(None, wails_tags="webkit2_41")
        self._ok(p)
        calls = self._wails_build_args()
        self.assertTrue(any(any(a == "-tags" and i + 1 < len(args) and args[i + 1] == "webkit2_41"
                                for i, a in enumerate(args)) for args in calls), calls)

    def test_no_wails_tags_no_flag(self):
        p = self._run(None)
        self._ok(p)
        calls = self._wails_build_args()
        self.assertFalse(any(a == "-tags" or a.startswith("-tags=") for args in calls for a in args),
                         calls)

    def test_empty_wails_tags_no_flag(self):
        p = self._run(None, wails_tags="")
        self._ok(p)
        calls = self._wails_build_args()
        self.assertFalse(any(a == "-tags" or a.startswith("-tags=") for args in calls for a in args),
                         calls)


if __name__ == "__main__":
    unittest.main()
