"""Модель генерации yue2.cpp при установке воркера (карточка internal-cpp-only, усл. 5, 6).

install.sh всегда ставит программу yue2.cpp и модель: --cpp-model auto|bf16|q8|q6|q5 (auto или не задан —
рекомендуемая по видеопамяти из engine_cpp.recommend), --cpp-low-vram — экономный режим принудительно.
Песочница — та же, что в test_install_sh (временный HOME, заглушки в PATH, журнал вызовов), плюс:
- nvidia-smi печатает объём видеопамяти (МиБ) из файла <заглушки>/vram, если в аргументах есть memory.total;
- python venv — test_cpp_only.CPP_VENV_PY: recommend/installed — настоящий engine_cpp.py, download —
  фейковая загрузка (файлы модели нужного размера, без сети; fail-download → код 1); каждый вызов с каталогом
  запуска и PARTSCASTER_DATA_DIR/PARTSCASTER_CPP_MODELS пишется в py-ctx.log;
- программа yue2.cpp из архива — <каталог версии>/yue2cpp/yue-server (заглушка; "broken" — код 1 без Usage);
- git, cmake, nvcc — заглушки (сеть и настоящая сборка не нужны): git clone/init создаёт каталог,
  cmake --build кладёт собранную программу в $PARTSCASTER_HOME/yue2cpp-src/build/yue-server .

Что должно работать. Рекомендация по видеопамяти, --engine как неизвестный параметр, повтор со скачанной
выбранной моделью, старая установка на PyTorch, прочие ключи settings.json — test_cpp_only (ТК12); здесь — остальное:
E3. Явная модель: каждое имя (bf16, q8, q6, q5) принимается и скачивается; явная модель не заменяется
    рекомендуемой и на малой видеопамяти, экономный режим при этом — как рекомендовано для карты;
    --cpp-low-vram с явной моделью → экономный режим.
E4. Модель качается: <venv>/bin/python engine_cpp.py download <имя> из каталога устанавливаемой версии,
    в каталог данных этой установки ($PARTSCASTER_HOME/data); повтор без флагов при скачанной выбранной
    модели — проверка «скачана» этой модели и без загрузки.
E5. Программа из архива отвечает на --help → она и берётся, сборки нет (git/cmake не зовутся).
E6. Программы нет (или не запускается) и нет nvcc → ненулевой код, сообщение «yue2.cpp не собран: нужны
    git, cmake, nvcc», модель не качается; новая версия не включается (current и settings.json прежние,
    каталога новой версии нет — --rollback её не выберет).
E7. Программы нет, git/cmake/nvcc есть → сборка закреплённого коммита c893629; повтор при уже собранной
    программе — без пересборки; пометка $PARTSCASTER_HOME/yue2cpp-src/.commit с другим коммитом → пересборка.
E8. Неизвестная модель (--cpp-model q4) → код 2 до любых изменений; параметр без значения — ошибка.
E9. Загрузка модели не удалась → ненулевой код, «не скачана» в выводе; новая версия не включается
    (current и выбор модели прежние); при первой установке current не появляется, воркер не запускается.
"""
import json
import os
import shutil
import sys
import unittest
from pathlib import Path

import test_install_sh as base

GIB = 1024  # МиБ в ГиБ — nvidia-smi с nounits печатает МиБ

NVIDIA_SMI = r"""#!/bin/sh
{ echo "== nvidia-smi"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
[ -f "$STUB_DIR/fail-nvidia-smi" ] && exit 1
case "$*" in *memory.total*) cat "$STUB_DIR/vram";; esac
exit 0
"""

# git: клон/инициализация создают целевой каталог (последний аргумент), остальное — только журнал
GIT = r"""#!/bin/sh
{ echo "== git"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
for a in "$@"; do last="$a"; done
case " $* " in *" clone "*|*" init "*) mkdir -p "$last";; esac
exit 0
"""

# cmake: сборка кладёт рабочую программу туда, где её ищет воркер
CMAKE = r"""#!/bin/sh
{ echo "== cmake"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
case " $* " in *" --build "*)
  out="$(cat "$STUB_DIR/yue-home")/yue2cpp-src/build"
  mkdir -p "$out"; cp "$STUB_DIR/server-ok" "$out/yue-server"; chmod +x "$out/yue-server";;
esac
exit 0
"""


class _EngineCase(base._InstallCase):
    def setUp(self):
        super().setUp()
        nv = self.stubs / "nvidia-smi"
        nv.write_text(NVIDIA_SMI)
        nv.chmod(0o755)
        # python venv с настоящим engine_cpp (recommend/installed) и фейковой загрузкой — из test_cpp_only;
        # импорт здесь, а не в начале модуля: test_cpp_only сам импортирует этот модуль
        import test_cpp_only
        (self.stubs / "venv-python").write_text(test_cpp_only.CPP_VENV_PY.replace("__REAL_PY__", sys.executable))
        (self.stubs / "fake_download.py").write_text(test_cpp_only.FAKE_DOWNLOAD)
        (self.stubs / "server-ok").write_text(base.STUB)
        (self.stubs / "yue-home").write_text(str(self.yue_home))
        self.set_vram(16 * GIB)
        self.data = self.yue_home / "data"
        self.settings_path = self.data / "settings.json"

    def set_vram(self, *mib):
        """Объём видеопамяти по картам (МиБ), по строке на карту — как печатает nvidia-smi."""
        (self.stubs / "vram").write_text("".join(f"{m}\n" for m in mib))

    def add_build_tools(self, nvcc=True):
        for name, text in (("git", GIT), ("cmake", CMAKE), ("nvcc", base.STUB if nvcc else None)):
            if text is None:
                continue
            p = self.stubs / name
            p.write_text(text)
            p.chmod(0o755)

    def no_nvcc_guard(self):
        for d in ("/usr/bin", "/bin", "/usr/local/cuda/bin", "/opt/cuda/bin"):
            if Path(d, "nvcc").exists():
                self.skipTest(f"в системе есть {d}/nvcc — отсутствие не смоделировать")

    def make_version(self, ver, requirements=None, server="ok"):
        """Каталог версии; server: "ok" — программа yue2.cpp в архиве работает, "broken" — есть,
        но не запускается, None — архив без yue2cpp/."""
        d = super().make_version(ver, requirements)
        if server is None:
            shutil.rmtree(d / "yue2cpp", ignore_errors=True)
        else:
            (d / "yue2cpp").mkdir(exist_ok=True)
            p = d / "yue2cpp" / "yue-server"
            p.write_text(base.STUB if server == "ok" else "#!/bin/sh\nexit 1\n")
            p.chmod(0o755)
        return d

    def install(self, ver, *args, requirements=None, ok=True, server="ok", **env):
        p = self.run_sh(self.make_version(ver, requirements, server) / "install.sh", *args, **env)
        if ok:
            self.assertEqual(p.returncode, 0, f"install {ver} failed:\n{p.stdout}\n{p.stderr}")
        return p

    def settings(self, path=None):
        return json.loads((path or self.settings_path).read_text(encoding="utf-8"))

    def py_ctx(self):
        """[(cwd, PARTSCASTER_DATA_DIR, PARTSCASTER_CPP_MODELS, [аргументы])] вызовов venv-python (за все запуски)."""
        f = self.stubs / "py-ctx.log"
        out = []
        for ln in (f.read_text().splitlines() if f.exists() else []):
            cwd, data, models, args = ln.split("\t", 3)
            out.append((cwd, data, models, args.split(" ")))
        return out

    def engine_calls(self, cmd):
        """Аргументы вызовов engine_cpp.py <cmd> последнего запуска: [[cmd, …]]."""
        return [a[1:] for n, a in self.calls()
                if n == "venv-python" and a and Path(a[0]).name == "engine_cpp.py" and a[1:2] == [cmd]]

    def downloads(self):
        """Аргументы вызовов engine_cpp.py download последнего запуска: [[download, имя, …]]."""
        return self.engine_calls("download")

    def assert_model(self, model, low_vram):
        st = self.settings()
        self.assertNotIn("engine", st, st)
        self.assertEqual(st.get("cpp_model"), model, st)
        self.assertIs(st.get("cpp_low_vram"), low_vram, st)

    def model_files(self, data=None):
        d = (data or self.data) / "yue2cpp-models"
        return sorted(x.name for x in d.iterdir()) if d.is_dir() else []

    def assert_not_switched(self, p, before_link, before_settings):
        """Сбой обновления: ненулевой код, current и settings.json прежние, каталог новой версии убран."""
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertEqual(os.readlink(self.current()), before_link)
        self.assertEqual(self.settings_path.read_text(encoding="utf-8"), before_settings)
        vers = [m.group(1) for m in map(base.RELEASE_RE.match, self.releases()) if m]
        self.assertEqual(vers, ["1.0.0"], self.releases())


class TestExplicitModel(_EngineCase):
    """E3: явная модель и экономный режим."""

    def test_every_model_name_accepted(self):
        for name in ("bf16", "q8", "q6", "q5"):
            with self.subTest(name=name):
                self.install("1.0.0", "--no-start", "--cpp-model", name)
                self.assertEqual(self.settings().get("cpp_model"), name)
                self.assertEqual([d[:2] for d in self.downloads()], [["download", name]], self.calls())

    def test_explicit_model_not_replaced_on_small_vram(self):
        """Явная модель на карте 8 ГБ не заменяется рекомендуемой; экономный режим — как рекомендовано."""
        self.set_vram(8188)  # рекомендация — q8 + экономный
        self.install("1.0.0", "--no-start", "--cpp-model", "bf16")
        self.assert_model("bf16", True)
        self.assertEqual([d[:2] for d in self.downloads()], [["download", "bf16"]], self.calls())

    def test_explicit_model_big_card_not_low_vram(self):
        self.set_vram(16 * GIB)  # рекомендация — без экономного режима
        self.install("1.0.0", "--no-start", "--cpp-model", "q5")
        self.assert_model("q5", False)

    def test_low_vram_flag_with_explicit_model(self):
        self.install("1.0.0", "--no-start", "--cpp-model", "bf16", "--cpp-low-vram")
        self.assert_model("bf16", True)


class TestModelDownload(_EngineCase):
    """E4, E9: загрузка модели кодом воркера."""

    def test_download_from_version_dir_with_venv_python(self):
        self.install("1.0.0", "--no-start", "--cpp-model", "q8")
        rel = (self.yue_home / "releases" / self.current_release()).resolve()
        dl = [c for c in self.py_ctx() if c[3][0].endswith("engine_cpp.py") and c[3][1:2] == ["download"]]
        self.assertEqual(len(dl), 1, self.py_ctx())
        cwd, _, _, args = dl[0]
        script = (Path(cwd) / args[0]).resolve()
        self.assertEqual(script, rel / "engine_cpp.py", "engine_cpp.py must be run from the installed version")

    def test_download_goes_to_this_install_data(self):
        """Каталог моделей по умолчанию — <данные>/yue2cpp-models: данные — те, что у юнита."""
        x = Path(self._td.name) / "other-home"
        (self.stubs / "yue-home").write_text(str(x))
        self.install("1.0.0", "--no-start", "--cpp-model", "q8", PARTSCASTER_HOME=str(x))
        dl = [c for c in self.py_ctx() if c[3][0].endswith("engine_cpp.py") and c[3][1:2] == ["download"]]
        self.assertEqual(len(dl), 1, self.py_ctx())
        _, data, models, _ = dl[0]
        want = x / "data"
        self.assertTrue(data == str(want) or (models and Path(models).is_relative_to(want)),
                        f"download not pointed at {want}: "
                        f"PARTSCASTER_DATA_DIR={data!r} PARTSCASTER_CPP_MODELS={models!r}")
        self.assertEqual(self.settings(want / "settings.json").get("cpp_model"), "q8")
        self.assertTrue(self.model_files(want), "модель не в каталоге данных этой установки")
        self.assertFalse(self.data.exists(), "тронуты данные по умолчанию при другом PARTSCASTER_HOME")

    def test_rerun_without_flags_no_download(self):
        """Повтор без флагов: проверяется, скачана ли выбранная модель; скачана — загрузки нет."""
        self.install("1.0.0", "--no-start", "--cpp-model", "q6")
        self.install("1.0.1", "--no-start")
        self.assertEqual(self.downloads(), [], self.calls())
        self.assertEqual(self.engine_calls("installed"), [["installed", "q6"]], self.calls())
        self.assertEqual(base.RELEASE_RE.match(self.current_release()).group(1), "1.0.1")

    def test_download_failure_keeps_previous_version(self):
        """Обновление: новая модель не скачалась — действующими остаются прежняя версия и прежний выбор."""
        self.install("1.0.0", "--no-start", "--cpp-model", "q8")
        before_link, before_st = os.readlink(self.current()), self.settings_path.read_text(encoding="utf-8")
        (self.stubs / "fail-download").touch()
        p = self.install("1.0.1", "--no-start", "--cpp-model", "q6", ok=False)
        self.assertEqual(len(self.downloads()), 1, self.calls())
        self.assertIn("не скачана", p.stdout + p.stderr)
        self.assert_not_switched(p, before_link, before_st)

    def test_download_failure_fresh_no_current(self):
        """Первая установка: модель не скачалась — current не появляется, версия не остаётся, воркер не запущен."""
        (self.stubs / "fail-download").touch()
        p = self.install("1.0.0", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertFalse(self.current().exists() or self.current().is_symlink(), "current без модели")
        self.assertEqual(self.releases(), [])
        self.assertEqual([a for n, a in self.calls() if n == "systemctl" and "restart" in a], [])


class TestBinary(_EngineCase):
    """E5, E6, E7: программа yue2.cpp — из архива, сборка или отказ без включения версии."""

    MSG = "yue2.cpp не собран: нужны git, cmake, nvcc"

    def test_archive_binary_used_no_build(self):
        self.add_build_tools()
        self.install("1.0.0", "--no-start", "--cpp-model", "q8")
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake", "nvcc")], [])
        helps = [a for n, a in self.calls() if n == "yue-server" and "--help" in a]
        self.assertTrue(helps, f"archive yue-server not checked with --help: {self.calls()}")
        self.assert_model("q8", False)

    def check_no_program(self, server):
        """Прежняя версия работает; у новой нет рабочей программы, nvcc нет — отказ, прежняя остаётся."""
        self.no_nvcc_guard()
        self.install("1.0.0", "--no-start", "--cpp-model", "q8")
        before_link, before_st = os.readlink(self.current()), self.settings_path.read_text(encoding="utf-8")
        self.add_build_tools(nvcc=False)
        p = self.install("1.0.1", "--no-start", "--cpp-model", "q6", server=server, ok=False)
        self.assertIn(self.MSG, p.stdout + p.stderr)
        self.assert_not_switched(p, before_link, before_st)
        self.assertEqual(self.downloads(), [], "модель качается, хотя программы нет")

    def test_no_binary_no_nvcc(self):
        self.check_no_program(None)

    def test_broken_binary_no_nvcc(self):
        self.check_no_program("broken")

    def test_build_from_pinned_commit(self):
        self.add_build_tools()
        self.install("1.0.0", "--no-start", "--cpp-model", "q8", server=None)
        gits = [a for n, a in self.calls() if n == "git"]
        self.assertTrue(any(any("c893629" in x for x in a) for a in gits), f"pinned commit not used: {gits}")
        self.assertTrue(self.called("cmake"), self.calls())
        self.assert_model("q8", False)

    def put_built(self, commit=None):
        """Программа, собранная прошлым запуском; commit — пометка yue2cpp-src/.commit (None — нет)."""
        built = self.yue_home / "yue2cpp-src" / "build" / "yue-server"
        built.parent.mkdir(parents=True)
        shutil.copy(self.stubs / "server-ok", built)
        built.chmod(0o755)
        if commit is not None:
            (self.yue_home / "yue2cpp-src" / ".commit").write_text(commit + "\n")

    def test_already_built_not_rebuilt(self):
        """Пометки нет — программа берётся как есть."""
        self.add_build_tools()
        self.put_built()
        self.install("1.0.0", "--no-start", "--cpp-model", "q8", server=None)
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake")], [])
        self.assert_model("q8", False)

    def test_built_pinned_commit_not_rebuilt(self):
        self.add_build_tools()
        self.put_built("c893629")
        self.install("1.0.0", "--no-start", "--cpp-model", "q8", server=None)
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake")], [])
        self.assert_model("q8", False)

    def test_built_other_commit_rebuilt(self):
        """Пометка другого коммита — пересборка закреплённого."""
        self.add_build_tools()
        self.put_built("0123abc")
        self.install("1.0.0", "--no-start", "--cpp-model", "q8", server=None)
        gits = [a for n, a in self.calls() if n == "git"]
        self.assertTrue(any(any("c893629" in x for x in a) for a in gits), f"pinned commit not rebuilt: {gits}")
        self.assertTrue(self.called("cmake"), self.calls())
        self.assert_model("q8", False)


class TestBadValues(_EngineCase):
    """E8: неизвестные значения — код 2 до изменений."""

    def test_unknown_model(self):
        p = self.install("1.0.0", "--no-start", "--cpp-model", "q4", ok=False)
        self.assertEqual(p.returncode, 2, f"{p.stdout}\n{p.stderr}")
        self.assertFalse((self.yue_home / "releases").exists(), "releases/ created despite bad option")
        self.assertFalse(self.settings_path.exists())
        self.assertEqual([n for n, _ in self.calls() if n in ("uv", "systemctl", "venv-python")], [])

    def test_missing_values(self):
        for args in (("--cpp-model",), ("--hf-home",)):
            with self.subTest(args=args):
                p = self.install("1.0.0", "--no-start", *args, ok=False)
                self.assertNotEqual(p.returncode, 0, p.stdout)
                self.assertFalse((self.yue_home / "releases").exists())


if __name__ == "__main__":
    unittest.main()
