"""Установщик воркера install.sh.

Песочница: HOME — временный каталог; внешние программы (nvidia-smi, uv, systemctl, curl,
sleep) — заглушки в PATH, пишут вызовы в журнал. `uv venv <путь>` создаёт venv-заглушку,
её python тоже пишет вызовы (так видно kits_install.py). Распакованный каталог —
из архива tools/worker-tar.sh (явная версия), для каждой версии — копия с её VERSION.

Что должно работать:
I1. Установка: current → $PARTSCASTER_HOME/releases/<YYYYmmddHHMMSS>-<ver> (атомарная ссылка),
    юнит в $PARTSCASTER_HOME/units/ (WorkingDirectory=%h/partscaster/current).
I2. worker.env создаётся, если нет: HF_HOME=<HOME>/yue/hf-cache; --hf-home DIR → HF_HOME=DIR
    (~ раскрывается); существующий worker.env не меняется.
I3. venv создаётся, если нет; пакеты (uv pip) — только если venv новый или requirements.txt
    изменился; отпечаток $PARTSCASTER_HOME/.requirements.sha256 — после успешной установки (сбой pip →
    нет отпечатка, следующий запуск ставит снова); venv есть, а отпечатка нет (установка до
    архивов) → uv pip не зовётся, отпечаток пишется.
I4. Наборы сэмплов (kits_install.py через python venv), их сбой — не провал.
I5. Хранятся 3 последних версии + текущая; $PARTSCASTER_HOME/data и worker.env не трогаются.
I6. nvidia-smi нет или падает → ошибка ДО изменений (releases/ не создан).
I7. Без --no-start — daemon-reload, enable, restart, проверка /health (нет ответа → ошибка);
    с --no-start — restart не вызывается.
I8. --rollback (из current/install.sh): current → предыдущая версия, перезапуск; нет
    предыдущей → ошибка, current прежний.
"""
import os
import re
import shutil
import subprocess
import tarfile
import tempfile
import time
import unittest
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
TAR_SCRIPT = REPO / "tools" / "worker-tar.sh"
RELEASE_RE = re.compile(r"^\d{14}-(.+)$")

# Заглушка внешней программы: «== имя», затем аргументы по одному на строку. Код выхода —
# 1, если в каталоге заглушек есть файл fail-<имя> (или fail-uv-pip для `uv pip`).
STUB = r"""#!/bin/sh
name=$(basename "$0")
{ echo "== $name"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
if [ "$name" = uv ] && [ "$1" = venv ]; then
  mkdir -p "$2/bin"
  cp "$STUB_DIR/venv-python" "$2/bin/python"
  chmod +x "$2/bin/python"
fi
if [ "$name" = uv ] && [ "$1" = pip ] && [ -f "$STUB_DIR/fail-uv-pip" ]; then exit 1; fi
[ -f "$STUB_DIR/fail-$name" ] && exit 1
exit 0
"""

# python venv-заглушки: пишет вызов; kits_install.py падает, если есть fail-kits.
VENV_PY = r"""#!/bin/sh
{ echo "== venv-python"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
case "$1" in *kits_install.py) [ -f "$STUB_DIR/fail-kits" ] && exit 1;; esac
exit 0
"""


def _template_dir(tmp):
    """Распакованный архив воркера (версия 0.0.0-tpl): шаблон для копий версий."""
    out = Path(tmp) / "dist"
    out.mkdir()
    p = subprocess.run(["bash", str(TAR_SCRIPT), str(out), "0.0.0-tpl"], cwd=REPO,
                       capture_output=True, text=True, timeout=120)
    if p.returncode != 0:
        raise AssertionError(f"worker-tar.sh failed:\n{p.stdout}\n{p.stderr}")
    path = Path([ln for ln in p.stdout.splitlines() if ln.strip()][-1].strip())
    ext = Path(tmp) / "extract"
    with tarfile.open(path, "r:gz") as tf:
        tf.extractall(ext, filter="data")
    d = ext / "worker-0.0.0-tpl"
    if not (d / "install.sh").is_file():
        raise AssertionError(f"no install.sh in archive: {sorted(x.name for x in d.iterdir())}")
    return d


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class _InstallCase(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls._tpl_td = tempfile.TemporaryDirectory()
        cls.template = _template_dir(cls._tpl_td.name)

    @classmethod
    def tearDownClass(cls):
        cls._tpl_td.cleanup()

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        root = Path(self._td.name)
        self.home = root / "home"
        self.home.mkdir()
        self.stubs = root / "stubs"
        self.stubs.mkdir()
        self.log = self.stubs / "calls.log"
        self.log.write_text("")
        for name in ("nvidia-smi", "uv", "systemctl", "curl", "sleep"):
            p = self.stubs / name
            p.write_text(STUB)
            p.chmod(0o755)
        (self.stubs / "venv-python").write_text(VENV_PY)
        self.unpacked = root / "unpacked"
        self.unpacked.mkdir()
        self._last_run = 0.0
        self._n = 0
        self.yue_home = self.home / "partscaster"
        self.venv = self.home / "yue" / ".venv"

    def make_version(self, ver, requirements=None):
        """Распакованный каталог версии ver (копия шаблона со своим VERSION)."""
        self._n += 1  # одна версия может распаковываться дважды — каждый раз в свой каталог
        d = self.unpacked / str(self._n) / f"worker-{ver}"
        shutil.copytree(self.template, d, symlinks=True)
        (d / "VERSION").write_text(ver + "\n")
        if requirements is not None:
            (d / "requirements.txt").write_text(requirements)
        return d

    def env(self, **extra):
        e = {k: v for k, v in os.environ.items() if not k.startswith("YUE_")}
        e.update(HOME=str(self.home), PATH=f"{self.stubs}:/usr/bin:/bin",
                 STUB_LOG=str(self.log), STUB_DIR=str(self.stubs))
        e.update(extra)
        return e

    def run_sh(self, script, *args, **env):
        """bash <script> args… из чужого каталога (cwd = HOME). Разные запуски — в разные
        секунды: имя релиза содержит время до секунды."""
        wait = 1.05 - (time.monotonic() - self._last_run)
        if self._last_run and wait > 0:
            time.sleep(wait)
        self.log.write_text("")
        p = subprocess.run(["bash", str(script), *args], cwd=self.home, env=self.env(**env),
                           capture_output=True, text=True, timeout=120)
        self._last_run = time.monotonic()
        return p

    def install(self, ver, *args, requirements=None, ok=True, **env):
        p = self.run_sh(self.make_version(ver, requirements) / "install.sh", *args, **env)
        if ok:
            self.assertEqual(p.returncode, 0, f"install {ver} failed:\n{p.stdout}\n{p.stderr}")
        return p

    def calls(self):
        """[(имя, [аргументы])] из журнала заглушек последнего запуска."""
        out = []
        for line in self.log.read_text().splitlines():
            if line.startswith("== "):
                out.append((line[3:], []))
            elif out:
                out[-1][1].append(line)
        return out

    def called(self, name, *first):
        return [a for n, a in self.calls() if n == name and a[:len(first)] == list(first)]

    def current(self):
        return self.yue_home / "current"

    def current_release(self):
        cur = self.current()
        self.assertTrue(cur.is_symlink(), f"{cur} must be a symlink")
        target = cur.resolve()
        self.assertEqual(target.parent, (self.yue_home / "releases").resolve())
        return target.name

    def releases(self):
        d = self.yue_home / "releases"
        return sorted(p.name for p in d.iterdir()) if d.is_dir() else []

    def worker_env(self):
        return (self.yue_home / "worker.env").read_text()

    def env_values(self, key):
        return [ln[len(key) + 1:] for ln in self.worker_env().splitlines() if ln.startswith(key + "=")]


class TestInstallBasic(_InstallCase):
    """I1, I2, I3 (новый venv), I4, I7."""

    def test_current_points_to_release(self):
        self.install("1.0.0", "--no-start")
        name = self.current_release()
        m = RELEASE_RE.match(name)
        self.assertIsNotNone(m, name)
        self.assertEqual(m.group(1), "1.0.0")
        self.assertTrue((self.current() / "yue_worker.py").is_file())
        self.assertEqual((self.current() / "VERSION").read_text().strip(), "1.0.0")

    def test_unit_installed(self):
        self.install("1.0.0", "--no-start")
        unit = self.yue_home / "units" / "partscaster-worker.service"
        self.assertTrue(unit.is_file())
        self.assertIn("WorkingDirectory=%h/partscaster/current", unit.read_text())

    def test_worker_env_default_hf_home(self):
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.env_values("HF_HOME"), [str(self.home / "yue" / "hf-cache")])

    def test_worker_env_hf_home_option(self):
        self.install("1.0.0", "--no-start", "--hf-home", "/x")
        self.assertEqual(self.env_values("HF_HOME"), ["/x"])

    def test_worker_env_hf_home_tilde_and_spaces(self):
        self.install("1.0.0", "--no-start", "--hf-home", "~/my cache")
        self.assertEqual(self.env_values("HF_HOME"), [str(self.home / "my cache")])

    def test_existing_worker_env_unchanged(self):
        self.yue_home.mkdir()
        pre = "HF_HOME=/mine\nOLLAMA_URL=http://x:11434\n"
        (self.yue_home / "worker.env").write_text(pre)
        self.install("1.0.0", "--no-start", "--hf-home", "/other")
        self.assertEqual(self.worker_env(), pre)

    def test_new_venv_and_packages(self):
        self.install("1.0.0", "--no-start")
        venvs = self.called("uv", "venv")
        self.assertEqual(len(venvs), 1, self.calls())
        self.assertEqual(venvs[0][1], str(self.venv))
        self.assertIn("--python", venvs[0])
        self.assertEqual(venvs[0][venvs[0].index("--python") + 1], "3.12")
        pips = self.called("uv", "pip", "install")
        self.assertEqual(len(pips), 1, self.calls())
        self.assertIn("-r", pips[0])
        self.assertTrue(pips[0][pips[0].index("-r") + 1].endswith("requirements.txt"))
        self.assertTrue((self.yue_home / ".requirements.sha256").is_file())

    def test_yue_home_and_venv_overrides(self):
        other_home, other_venv = self.home / "elsewhere", self.home / "venvs" / "w"
        self.install("1.0.0", "--no-start", PARTSCASTER_HOME=str(other_home), PARTSCASTER_VENV=str(other_venv))
        self.assertTrue((other_home / "current").is_symlink())
        self.assertEqual(self.called("uv", "venv")[0][1], str(other_venv))
        self.assertFalse((self.yue_home / "current").exists())

    def test_kits_installed_with_venv_python(self):
        self.install("1.0.0", "--no-start")
        kits = [a for n, a in self.calls() if n == "venv-python" and a and a[0].endswith("kits_install.py")]
        self.assertEqual(len(kits), 1, self.calls())

    def test_kits_failure_is_not_fatal(self):
        (self.stubs / "fail-kits").touch()
        self.install("1.0.0", "--no-start")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.0")

    def test_no_start_does_not_restart(self):
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.called("systemctl", "--user", "restart"), [])
        restarts = [a for n, a in self.calls() if n == "systemctl" and "restart" in a]
        self.assertEqual(restarts, [])

    def test_start_restarts_and_checks_health(self):
        self.install("1.0.0")
        sysargs = [a for n, a in self.calls() if n == "systemctl"]
        for want in ("daemon-reload", "enable", "restart"):
            self.assertTrue(any(want in a for a in sysargs), f"systemctl {want} not called: {sysargs}")
        self.assertTrue(any("restart" in a and "partscaster-worker" in a for a in sysargs), sysargs)
        self.assertTrue(all("--user" in a for a in sysargs), f"systemctl without --user: {sysargs}")
        curls = [a for n, a in self.calls() if n == "curl"]
        self.assertTrue(any(any("localhost:8091/health" in x for x in a) for a in curls), curls)

    def test_health_failure_is_error(self):
        (self.stubs / "fail-curl").touch()
        p = self.install("1.0.0", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)

    def test_unit_follows_yue_home(self):
        """PARTSCASTER_HOME доходит до юнита — код, worker.env и данные от $PARTSCASTER_HOME."""
        x = Path(self._td.name) / "x"
        self.install("1.0.0", "--no-start", PARTSCASTER_HOME=str(x))
        unit = (x / "units" / "partscaster-worker.service").read_text().splitlines()
        for want in (f"WorkingDirectory={x}/current", f"EnvironmentFile={x}/worker.env",
                     f'Environment="PARTSCASTER_DATA_DIR={x}/data"',
                     # воркер ищет собранную install.sh программу yue2.cpp в $PARTSCASTER_HOME/yue2cpp-src
                     f'Environment="PARTSCASTER_HOME={x}"'):
            self.assertIn(want, unit)
        self.assertFalse(any("%h/partscaster" in ln for ln in unit), unit)

    def test_unit_default_yue_home(self):
        """При умолчаниях юнит остаётся с %h/partscaster."""
        self.install("1.0.0", "--no-start")
        unit = (self.yue_home / "units" / "partscaster-worker.service").read_text().splitlines()
        for want in ("WorkingDirectory=%h/partscaster/current", "EnvironmentFile=%h/partscaster/worker.env",
                     'Environment="PARTSCASTER_DATA_DIR=%h/partscaster/data"'):
            self.assertIn(want, unit)


class TestInstallPathsAndFailures(_InstallCase):
    """Пути в юните, недопустимые пути, отказы при выборе каталога версии."""

    def run_direct(self, ver, *args, timeout=120, **env):
        """Установка без паузы помощника run_sh, со своим таймаутом."""
        self.log.write_text("")
        return subprocess.run(["bash", str(self.make_version(ver) / "install.sh"), *args],
                              cwd=self.home, env=self.env(**env), capture_output=True, text=True,
                              timeout=timeout)

    def test_yue_home_with_space_quoted_in_unit(self):
        x = Path(self._td.name) / "my yue"
        p = self.run_direct("1.0.0", "--no-start", PARTSCASTER_HOME=str(x))
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        unit = (x / "units" / "partscaster-worker.service").read_text().splitlines()
        self.assertIn(f'Environment="PARTSCASTER_DATA_DIR={x}/data"', unit)

    def test_yue_home_with_bad_chars_refused(self):
        for bad in ("pct%x", 'quo"te'):
            with self.subTest(bad=bad):
                x = Path(self._td.name) / bad
                p = self.run_direct("1.0.0", "--no-start", PARTSCASTER_HOME=str(x))
                self.assertEqual(p.returncode, 2, f"{p.stdout}\n{p.stderr}")
                self.assertFalse((x / "releases").exists(), "releases/ created despite bad path")

    @unittest.skipIf(hasattr(os, "geteuid") and os.geteuid() == 0, "под root права на запись не отнять")
    def test_releases_not_writable_fails_fast(self):
        rel = self.yue_home / "releases"
        rel.mkdir(parents=True)
        rel.chmod(0o555)
        self.addCleanup(rel.chmod, 0o755)
        try:
            p = self.run_direct("1.0.0", "--no-start", timeout=20)
        except subprocess.TimeoutExpired:
            self.fail("install.sh hangs when releases/ is not writable")
        self.assertNotEqual(p.returncode, 0, p.stdout)

    def test_copy_failure_leaves_no_hidden_release(self):
        # сбой копирования — граница (файловая система): cp падает, когда копирует в releases/
        cp = self.stubs / "cp"
        cp.write_text('#!/bin/sh\nfor a in "$@"; do last="$a"; done\n'
                      'case "$last" in */releases/*) exit 1;; esac\nexec /bin/cp "$@"\n')
        cp.chmod(0o755)
        p = self.run_direct("1.0.0", "--no-start")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        rel = self.yue_home / "releases"
        left = sorted(x.name for x in rel.iterdir()) if rel.is_dir() else []
        self.assertEqual([n for n in left if n.startswith(".")], [], left)


# python venv без curl: пишет аргументы и код со stdin (python - / без аргументов); вызов, где
# упомянут localhost:8091/health, падает при файле fail-health.
VENV_PY_HEALTH = r"""#!/bin/sh
{ echo "== venv-python"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
code="$*"
if [ $# -eq 0 ] || [ "$1" = - ]; then code=$(cat); printf '%s\n' "$code" >> "$STUB_LOG"; fi
case "$code" in *localhost:8091/health*) [ -f "$STUB_DIR/fail-health" ] && exit 1;; esac
exit 0
"""

TOOLS = ("bash", "sh", "env", "sed", "date", "cp", "mv", "ln", "mkdir", "rm", "rmdir", "tr", "sha256sum",
         "cut", "ls", "sort", "awk", "tail", "head", "cat", "readlink", "basename", "dirname", "id", "seq",
         "printf", "echo", "touch", "chmod", "grep", "mktemp", "test", "true", "false", "wc", "tee")


class TestInstallHealthWithoutCurl(_InstallCase):
    """curl нет — /health проверяется python-ом venv воркера."""

    def setUp(self):
        super().setUp()
        (self.stubs / "curl").unlink()
        (self.stubs / "venv-python").write_text(VENV_PY_HEALTH)
        self.tools = Path(self._td.name) / "tools"
        self.tools.mkdir()
        for name in TOOLS:
            p = shutil.which(name)
            if p:
                (self.tools / name).symlink_to(p)

    def run_install(self):
        self.log.write_text("")
        bash = str(self.tools / "bash")
        return subprocess.run([bash, str(self.make_version("1.0.0") / "install.sh")], cwd=self.home,
                              env=self.env(PATH=f"{self.stubs}:{self.tools}"), stdin=subprocess.DEVNULL,
                              capture_output=True, text=True, timeout=120)

    def test_health_via_venv_python(self):
        p = self.run_install()
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertIn("localhost:8091/health", self.log.read_text().split("== venv-python", 1)[-1],
                      self.log.read_text())
        self.assertEqual(self.called("curl"), [])

    def test_health_failure_via_venv_python(self):
        (self.stubs / "fail-health").touch()
        p = self.run_install()
        self.assertNotEqual(p.returncode, 0, p.stdout)


class TestInstallRequirements(_InstallCase):
    """I3: пакеты ставятся только при новом venv или изменённом requirements.txt."""

    def test_rerun_unchanged_skips_pip(self):
        self.install("1.0.0", "--no-start")
        self.install("1.0.1", "--no-start")
        self.assertEqual(self.called("uv", "pip"), [], self.calls())
        self.assertEqual(self.called("uv", "venv"), [], "existing venv must not be recreated")

    def test_same_version_rerun_ok(self):
        self.install("1.0.0", "--no-start")
        self.install("1.0.0", "--no-start")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.0")
        self.assertEqual(self.called("uv", "pip"), [])

    def test_changed_requirements_runs_pip(self):
        self.install("1.0.0", "--no-start")
        reqs = (self.template / "requirements.txt").read_text() + "einops-extra-test\n"
        self.install("1.0.1", "--no-start", requirements=reqs)
        self.assertEqual(len(self.called("uv", "pip", "install")), 1, self.calls())

    def test_pip_failure_no_fingerprint_and_retry(self):
        (self.stubs / "fail-uv-pip").touch()
        self.install("1.0.0", "--no-start", ok=False)
        self.assertFalse((self.yue_home / ".requirements.sha256").exists(),
                         "fingerprint written after failed pip")
        (self.stubs / "fail-uv-pip").unlink()
        self.install("1.0.1", "--no-start")
        self.assertEqual(len(self.called("uv", "pip", "install")), 1, "after failed pip the next run must install")
        self.assertTrue((self.yue_home / ".requirements.sha256").is_file())

    def test_existing_venv_without_fingerprint(self):
        """Установка до архивов: venv есть, отпечатка нет → пакеты не ставятся, отпечаток пишется."""
        bin_ = self.venv / "bin"
        bin_.mkdir(parents=True)
        shutil.copy(self.stubs / "venv-python", bin_ / "python")
        (bin_ / "python").chmod(0o755)
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.called("uv", "venv"), [])
        self.assertEqual(self.called("uv", "pip"), [], self.calls())
        self.assertTrue((self.yue_home / ".requirements.sha256").is_file())
        # и дальше — как обычно: изменился requirements.txt → ставит
        reqs = (self.template / "requirements.txt").read_text() + "einops-extra-test\n"
        self.install("1.0.1", "--no-start", requirements=reqs)
        self.assertEqual(len(self.called("uv", "pip", "install")), 1, self.calls())

    def test_pip_failure_on_update_keeps_previous_current(self):
        """Сбой pip при обновлении — действующей остаётся прежняя версия."""
        self.install("1.0.0", "--no-start")
        before = os.readlink(self.current())
        (self.stubs / "fail-uv-pip").touch()
        reqs = (self.template / "requirements.txt").read_text() + "einops-extra-test\n"
        p = self.install("1.0.1", "--no-start", requirements=reqs, ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertEqual(os.readlink(self.current()), before)
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.0")


class TestInstallReleases(_InstallCase):
    """I5, I8: хранение версий, откат, data/ не трогается."""

    def test_keeps_three_plus_current(self):
        for i in range(1, 6):
            self.install(f"1.0.{i}", "--no-start")
        rel = self.releases()
        self.assertLessEqual(len(rel), 4, rel)
        vers = sorted(RELEASE_RE.match(n).group(1) for n in rel)
        self.assertEqual(vers[-1], "1.0.5")
        self.assertNotIn("1.0.1", vers, "oldest version must be removed")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.5")

    def test_rollback_to_previous(self):
        self.install("1.0.1", "--no-start")
        self.install("1.0.2", "--no-start")
        p = self.run_sh(self.current() / "install.sh", "--rollback")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.1")
        restarts = [a for n, a in self.calls() if n == "systemctl" and "restart" in a]
        self.assertTrue(restarts, "rollback must restart the worker")

    def test_rollback_without_previous_fails(self):
        self.install("1.0.1", "--no-start")
        before = os.readlink(self.current())
        p = self.run_sh(self.current() / "install.sh", "--rollback")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertEqual(os.readlink(self.current()), before)

    def test_data_and_env_untouched(self):
        data = self.yue_home / "data"
        data.mkdir(parents=True)
        (data / "yue.db").write_bytes(b"SQLite-test")
        (data / "jobs").mkdir()
        (data / "jobs" / "1.flac").write_bytes(b"audio")
        env = "HF_HOME=/mine\n"
        (self.yue_home / "worker.env").write_text(env)
        self.install("1.0.1", "--no-start")
        self.install("1.0.2", "--no-start")
        self.run_sh(self.current() / "install.sh", "--rollback")
        self.assertEqual((data / "yue.db").read_bytes(), b"SQLite-test")
        self.assertEqual((data / "jobs" / "1.flac").read_bytes(), b"audio")
        self.assertEqual(self.worker_env(), env)

    def test_reinstall_from_current_copies_release(self):
        """bash current/install.sh (без --rollback) — новая запись в releases/
        настоящий каталог, не ссылка; current указывает на неё."""
        self.install("1.0.0", "--no-start")
        first = self.current_release()
        p = self.run_sh(self.current() / "install.sh", "--no-start")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        new = [n for n in self.releases() if n != first]
        self.assertEqual(len(new), 1, self.releases())
        entry = self.yue_home / "releases" / new[0]
        self.assertFalse(entry.is_symlink(), f"{entry} is a symlink")
        self.assertTrue(entry.is_dir())
        self.assertTrue((entry / "install.sh").is_file())
        self.assertEqual(os.readlink(self.current()).rstrip("/").split("/")[-1], new[0])
        self.assertEqual(self.current_release(), new[0])

    def test_failed_install_leaves_no_release_for_rollback(self):
        """Сбой pip до переключения current убирает каталог новой версии —
        --rollback не выбирает версию, которая ни разу не работала."""
        self.install("1.0.0", "--no-start")
        (self.stubs / "fail-uv-pip").touch()
        reqs = (self.template / "requirements.txt").read_text() + "einops-extra-test\n"
        p = self.install("1.0.1", "--no-start", requirements=reqs, ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        vers = [m.group(1) for m in map(RELEASE_RE.match, self.releases()) if m]
        self.assertNotIn("1.0.1", vers, self.releases())
        (self.stubs / "fail-uv-pip").unlink()
        self.install("1.0.2", "--no-start")
        p = self.run_sh(self.current() / "install.sh", "--rollback")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.0")

    def test_same_version_same_second_unique_release(self):
        """Повтор той же версии в ту же секунду — две разные записи в releases/,
        обе каталоги, первая не вложена во вторую и не содержит её."""
        # время — граница: поддельные часы стоят, пока не позовут sleep (он сдвигает их на N с);
        # обе установки начинаются в одну и ту же секунду
        clock = self.stubs / "clock"
        clock.write_text("20260101000000\n")
        date = self.stubs / "date"
        date.write_text('#!/bin/sh\ncase "$1" in +%Y%m%d%H%M%S) cat "$STUB_DIR/clock";; '
                        '*) exec /bin/date "$@";; esac\n')
        sleep = self.stubs / "sleep"
        sleep.write_text('#!/bin/sh\nt=$(cat "$STUB_DIR/clock"); n=${1%%.*}; [ -n "$n" ] || n=1\n'
                         'echo $((t + n)) > "$STUB_DIR/clock"\n')
        for f in (date, sleep):
            f.chmod(0o755)
        tpl_dirs = sorted(x.name for x in self.template.iterdir() if x.is_dir())
        for _ in range(2):  # без паузы run_sh — прямой запуск
            p = subprocess.run(["bash", str(self.make_version("1.0.0") / "install.sh"), "--no-start"],
                               cwd=self.home, env=self.env(), capture_output=True, text=True, timeout=120)
            self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
            if _ == 0:
                first = self.current_release()
        rel = self.releases()
        self.assertEqual(len(rel), 2, rel)
        second = self.current_release()
        self.assertNotEqual(first, second)
        self.assertIn(first, rel)
        for name in rel:
            entry = self.yue_home / "releases" / name
            self.assertTrue(entry.is_dir() and not entry.is_symlink(), entry)
            self.assertTrue((entry / "install.sh").is_file(), entry)
        nested = sorted(x.name for x in (self.yue_home / "releases" / first).iterdir() if x.is_dir())
        self.assertEqual(nested, tpl_dirs, "a release was copied inside the first one")


class TestInstallWorkerEnvData(_InstallCase):
    """worker.env читается как данные (КЛЮЧ=значение), не исполняется."""

    def test_worker_env_not_executed(self):
        marker = Path(self._td.name) / "pwned"
        self.yue_home.mkdir()
        (self.yue_home / "worker.env").write_text(
            f"HF_HOME=/p$(touch {marker})\nX=\"/a b\"\n")
        self.install("1.0.0", "--no-start")
        self.assertFalse(marker.exists(), "worker.env was executed: $(touch …) ran")
        self.assertEqual(RELEASE_RE.match(self.current_release()).group(1), "1.0.0")


class TestInstallGpuCheck(_InstallCase):
    """I6: нет GPU — ошибка до любых изменений."""

    def assert_untouched(self, p):
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertFalse((self.yue_home / "releases").exists(), "releases/ created despite GPU check failure")
        self.assertFalse((self.yue_home / "current").exists())
        self.assertEqual([n for n, _ in self.calls() if n in ("uv", "systemctl")], [])

    def test_nvidia_smi_fails(self):
        (self.stubs / "fail-nvidia-smi").touch()
        self.assert_untouched(self.install("1.0.0", "--no-start", ok=False))

    def test_nvidia_smi_missing(self):
        if shutil.which("nvidia-smi", path="/usr/bin:/bin"):
            self.skipTest("в системе есть nvidia-smi — отсутствие не смоделировать")
        (self.stubs / "nvidia-smi").unlink()
        self.assert_untouched(self.install("1.0.0", "--no-start", ok=False))


if __name__ == "__main__":
    unittest.main()
