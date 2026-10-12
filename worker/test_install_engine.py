"""Спецификация выбора движка при установке воркера (internal-own-track, этап 20, условие 162, ТК192).

install.sh --engine torch|cpp (по умолчанию torch), --cpp-model auto|bf16|q8|q6|q5, --cpp-low-vram.
Песочница — та же, что в test_install_sh (временный HOME, заглушки в PATH, журнал вызовов), плюс:
- nvidia-smi печатает объём видеопамяти (МиБ) из файла <заглушки>/vram, если в аргументах есть memory.total;
- venv-python дополнительно пишет каталог запуска и YUE_DATA_DIR/YUE_CPP_MODELS в py-ctx.log;
- программа yue2.cpp из архива — <каталог версии>/yue2cpp/yue-server (заглушка; fail-yue-server → код 1);
- git, cmake, nvcc — заглушки (сеть и настоящая сборка не нужны): git clone/init создаёт каталог,
  cmake --build кладёт собранную программу в $YUE_HOME/yue2cpp-src/build/yue-server (путь из условия 158).

Что должно работать:
E1. Без --engine: settings.json данных не создаётся и не меняется, engine_cpp.py download не вызывается,
    программа yue2.cpp не ищется/не собирается. --engine torch (162в) пишет engine=torch, cpp_model и
    cpp_low_vram не трогает, загрузки нет.
E2. --engine cpp --cpp-model auto (и --engine cpp без --cpp-model, 162б): видеопамять — первая строка
    nvidia-smi (162а); ≥ 11776 МиБ → q8 и cpp_low_vram false; 7680–11775 → q8 и true; < 7680 → q5 и true.
    В $YUE_HOME/data/settings.json — engine "cpp", cpp_model, cpp_low_vram; прочие ключи сохраняются.
E3. Явная модель (--cpp-model q6) — без авто, экономный режим false (162г); --cpp-low-vram → true.
    --cpp-model / --cpp-low-vram без --engine cpp → код 2 до изменений (162б).
E4. Модель качается: <venv>/bin/python engine_cpp.py download <имя> из каталога установленной версии,
    в каталог данных этой установки; повторный запуск зовёт download снова (проверка «уже есть» — внутри).
E5. Программа из архива отвечает на --help → она и берётся, сборки нет (git/cmake не зовутся).
E6. Программы нет (или не запускается) и нет nvcc → код 0, воркер установлен, сообщение
    «yue2.cpp не собран: нужны git, cmake, nvcc», в settings.json нет engine=cpp.
E7. Программы нет, git/cmake/nvcc есть → сборка закреплённого коммита c893629; повтор при уже собранной
    программе — без пересборки; пометка $YUE_HOME/yue2cpp-src/.commit с другим коммитом → пересборка (162е).
E9. Загрузка модели не удалась → код 0, воркер установлен, «модель yue2.cpp не скачана», нет engine=cpp (162д).
E8. Неизвестное значение (--cpp-model q4, --engine foo) → код 2 до любых изменений.

Запуск из каталога worker: python -m unittest test_install_engine -v
"""
import json
import shutil
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

# venv-python: как в test_install_sh, плюс контекст вызова (каталог, переменные данных) в py-ctx.log
VENV_PY = r"""#!/bin/sh
{ echo "== venv-python"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
printf '%s\t%s\t%s\t%s\n' "$PWD" "${YUE_DATA_DIR-}" "${YUE_CPP_MODELS-}" "$*" >> "$STUB_DIR/py-ctx.log"
case "$1" in *kits_install.py) [ -f "$STUB_DIR/fail-kits" ] && exit 1;; esac
case "$1" in *engine_cpp.py) [ -f "$STUB_DIR/fail-download" ] && exit 1;; esac
exit 0
"""

# git: клон/инициализация создают целевой каталог (последний аргумент), остальное — только журнал
GIT = r"""#!/bin/sh
{ echo "== git"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
for a in "$@"; do last="$a"; done
case " $* " in *" clone "*|*" init "*) mkdir -p "$last";; esac
exit 0
"""

# cmake: сборка кладёт рабочую программу туда, где её ищет воркер (условие 158)
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
        (self.stubs / "venv-python").write_text(VENV_PY)
        (self.stubs / "server-ok").write_text(base.STUB)
        (self.stubs / "yue-home").write_text(str(self.yue_home))
        self.set_vram(16 * GIB)
        self.data = self.yue_home / "data"
        self.settings_path = self.data / "settings.json"

    # --- помощники ---

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
        if server is not None:
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
        """[(cwd, YUE_DATA_DIR, YUE_CPP_MODELS, [аргументы])] вызовов venv-python (за все запуски)."""
        f = self.stubs / "py-ctx.log"
        out = []
        for ln in (f.read_text().splitlines() if f.exists() else []):
            cwd, data, models, args = ln.split("\t", 3)
            out.append((cwd, data, models, args.split(" ")))
        return out

    def downloads(self):
        """Аргументы вызовов engine_cpp.py download последнего запуска: [[download, имя, …]]."""
        return [a[1:] for n, a in self.calls()
                if n == "venv-python" and a and Path(a[0]).name == "engine_cpp.py"
                and a[1:2] == ["download"]]

    def assert_cpp(self, model, low_vram):
        st = self.settings()
        self.assertEqual(st.get("engine"), "cpp", st)
        self.assertEqual(st.get("cpp_model"), model, st)
        self.assertIs(st.get("cpp_low_vram"), low_vram, st)

    def assert_no_cpp_engine(self):
        if self.settings_path.exists():
            self.assertNotEqual(self.settings().get("engine"), "cpp", self.settings_path.read_text())


class TestDefaultEngine(_EngineCase):
    """E1: без --engine поведение прежнее."""

    def test_no_engine_no_settings_no_download(self):
        self.install("1.0.0", "--no-start")
        self.assertFalse(self.settings_path.exists(), "settings.json created without --engine")
        self.assertEqual(self.downloads(), [], self.calls())

    def test_no_engine_existing_settings_unchanged(self):
        self.data.mkdir(parents=True)
        pre = '{"stems_model": "roformer", "engine": "cpp", "cpp_model": "q6"}'
        self.settings_path.write_text(pre, encoding="utf-8")
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.settings_path.read_text(encoding="utf-8"), pre)
        self.assertEqual(self.downloads(), [], self.calls())

    def test_no_engine_does_not_build(self):
        self.add_build_tools()
        self.install("1.0.0", "--no-start", server=None)
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake", "nvcc")], [])

    def test_engine_torch_writes_torch_without_download(self):
        """162в: явный выбор torch записывается."""
        self.install("1.0.0", "--no-start", "--engine", "torch")
        self.assertEqual(self.downloads(), [], self.calls())
        self.assertEqual(self.settings().get("engine"), "torch")

    def test_engine_torch_keeps_cpp_choices(self):
        """162в: возврат на torch не стирает выбранную модель и экономный режим."""
        self.data.mkdir(parents=True)
        self.settings_path.write_text(
            '{"engine": "cpp", "cpp_model": "q6", "cpp_low_vram": true, "stems_model": "roformer"}',
            encoding="utf-8")
        self.install("1.0.0", "--no-start", "--engine", "torch")
        st = self.settings()
        self.assertEqual(st.get("engine"), "torch", st)
        self.assertEqual(st.get("cpp_model"), "q6", st)
        self.assertIs(st.get("cpp_low_vram"), True, st)
        self.assertEqual(st.get("stems_model"), "roformer", st)
        self.assertEqual(self.downloads(), [], self.calls())


class TestAutoModel(_EngineCase):
    """E2: --cpp-model auto по видеопамяти."""

    def check(self, mib, model, low_vram, args=("--cpp-model", "auto")):
        self.set_vram(*(mib if isinstance(mib, tuple) else (mib,)))
        self.install("1.0.0", "--no-start", "--engine", "cpp", *args)
        self.assert_cpp(model, low_vram)
        self.assertEqual([d[:2] for d in self.downloads()], [["download", model]], self.calls())

    def test_16gb_q8(self):
        self.check(16 * GIB, "q8", False)

    def test_10gb_q8_low_vram(self):
        self.check(10 * GIB, "q8", True)

    def test_6gb_q5_low_vram(self):
        self.check(6 * GIB, "q5", True)

    def test_boundary_12gb_card_reports_less(self):
        """162а: карта «12 ГБ» сообщает 12282 МиБ — это ещё не экономный режим."""
        self.check(12282, "q8", False)

    def test_boundary_11776_not_low_vram(self):
        self.check(11776, "q8", False)

    def test_boundary_11775_low_vram(self):
        self.check(11775, "q8", True)

    def test_boundary_7680_q8_low_vram(self):
        self.check(7680, "q8", True)

    def test_boundary_7679_q5_low_vram(self):
        self.check(7679, "q5", True)

    def test_first_gpu_line_used(self):
        """162а: несколько карт — берётся первая строка (воркер работает на cuda:0)."""
        self.check((16 * GIB, 6 * GIB), "q8", False)

    def test_first_gpu_line_used_small_first(self):
        self.check((6 * GIB, 16 * GIB), "q5", True)

    def test_engine_cpp_without_model_is_auto(self):
        """162б: --engine cpp без --cpp-model = auto."""
        self.check(6 * GIB, "q5", True, args=())

    def test_engine_cpp_without_model_is_auto_big(self):
        self.check(16 * GIB, "q8", False, args=())

    def test_vram_read_from_nvidia_smi(self):
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "auto")
        queries = [a for n, a in self.calls() if n == "nvidia-smi" and any("memory.total" in x for x in a)]
        self.assertTrue(queries, self.calls())

    def test_other_settings_kept(self):
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"stems_model": "roformer", "fx_engine": false}', encoding="utf-8")
        self.set_vram(10 * GIB)
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "auto")
        st = self.settings()
        self.assertEqual(st.get("stems_model"), "roformer", st)
        self.assertIs(st.get("fx_engine"), False, st)
        self.assert_cpp("q8", True)


class TestExplicitModel(_EngineCase):
    """E3: явная модель и экономный режим."""

    def test_explicit_q6_no_auto(self):
        self.set_vram(16 * GIB)
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q6")
        self.assert_cpp("q6", False)
        self.assertEqual([d[:2] for d in self.downloads()], [["download", "q6"]], self.calls())

    def test_explicit_model_overrides_small_vram(self):
        """162г: явная модель без флага — экономный режим false даже на малой видеопамяти."""
        self.set_vram(6 * GIB)  # авто дало бы q5 + экономный
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q6")
        self.assert_cpp("q6", False)

    def test_every_model_name_accepted(self):
        for name in ("bf16", "q8", "q6", "q5"):
            with self.subTest(name=name):
                self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", name)
                self.assertEqual(self.settings().get("cpp_model"), name)
                self.assertEqual([d[:2] for d in self.downloads()], [["download", name]], self.calls())

    def test_low_vram_flag(self):
        self.set_vram(16 * GIB)  # авто дало бы false
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "auto", "--cpp-low-vram")
        self.assert_cpp("q8", True)

    def test_low_vram_flag_with_explicit_model(self):
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "bf16", "--cpp-low-vram")
        self.assert_cpp("bf16", True)


class TestModelDownload(_EngineCase):
    """E4: загрузка модели кодом воркера."""

    def test_download_from_version_dir_with_venv_python(self):
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        rel = (self.yue_home / "releases" / self.current_release()).resolve()
        dl = [c for c in self.py_ctx() if c[3][0].endswith("engine_cpp.py") and c[3][1:2] == ["download"]]
        self.assertEqual(len(dl), 1, self.py_ctx())
        cwd, _, _, args = dl[0]
        script = (Path(cwd) / args[0]).resolve()
        self.assertEqual(script, rel / "engine_cpp.py", "engine_cpp.py must be run from the installed version")

    def test_download_goes_to_this_install_data(self):
        """Каталог моделей по умолчанию — <данные>/yue2cpp-models (157): данные — те, что у юнита."""
        x = Path(self._td.name) / "other-home"
        (self.stubs / "yue-home").write_text(str(x))
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", YUE_HOME=str(x))
        dl = [c for c in self.py_ctx() if c[3][0].endswith("engine_cpp.py")]
        self.assertEqual(len(dl), 1, self.py_ctx())
        _, data, models, _ = dl[0]
        want = x / "data"
        self.assertTrue(data == str(want) or (models and Path(models).is_relative_to(want)),
                        f"download not pointed at {want}: YUE_DATA_DIR={data!r} YUE_CPP_MODELS={models!r}")
        self.assertEqual(self.settings(want / "settings.json").get("engine"), "cpp")

    def test_download_failure_not_fatal(self):
        """162д: загрузка не удалась — воркер установлен, движок не переключён."""
        (self.stubs / "fail-download").touch()
        p = self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        self.assertEqual(len(self.downloads()), 1, self.calls())
        self.assertEqual(base.RELEASE_RE.match(self.current_release()).group(1), "1.0.0")
        self.assertIn("модель yue2.cpp не скачана", p.stdout + p.stderr)
        self.assert_no_cpp_engine()

    def test_download_failure_keeps_other_settings(self):
        (self.stubs / "fail-download").touch()
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"stems_model": "roformer"}', encoding="utf-8")
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        self.assertEqual(self.settings().get("stems_model"), "roformer")
        self.assert_no_cpp_engine()

    def test_rerun_calls_download_again(self):
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        self.assertEqual(len(self.downloads()), 1, self.calls())
        self.install("1.0.1", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        self.assertEqual(len(self.downloads()), 1, self.calls())
        self.assert_cpp("q8", False)


class TestBinary(_EngineCase):
    """E5, E6, E7: программа yue2.cpp — из архива, сборка или понятная ошибка."""

    MSG = "yue2.cpp не собран: нужны git, cmake, nvcc"

    def test_archive_binary_used_no_build(self):
        self.add_build_tools()
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8")
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake", "nvcc")], [])
        helps = [a for n, a in self.calls() if n == "yue-server" and "--help" in a]
        self.assertTrue(helps, f"archive yue-server not checked with --help: {self.calls()}")
        self.assert_cpp("q8", False)

    def assert_not_built(self, p):
        self.assertEqual(base.RELEASE_RE.match(self.current_release()).group(1), "1.0.0")
        self.assertIn(self.MSG, p.stdout + p.stderr)
        self.assert_no_cpp_engine()

    def test_no_binary_no_nvcc(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        p = self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        self.assert_not_built(p)

    def test_broken_binary_no_nvcc(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        p = self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server="broken")
        self.assert_not_built(p)

    def test_no_binary_no_nvcc_keeps_previous_engine_settings(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"stems_model": "roformer"}', encoding="utf-8")
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        self.assertEqual(self.settings().get("stems_model"), "roformer")
        self.assert_no_cpp_engine()

    # кросс-ревью s20: прежний выбор yue2.cpp, а новая версия без рабочей программы и без сборки — основной движок,
    # иначе генерации падали бы на неработающем cpp
    def test_failed_setup_resets_previous_cpp_to_torch(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"engine": "cpp", "cpp_model": "q8", "stems_model": "roformer"}',
                                      encoding="utf-8")
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        st = self.settings()
        self.assertEqual(st.get("engine"), "torch")
        self.assertEqual(st.get("stems_model"), "roformer")

    def test_build_from_pinned_commit(self):
        self.add_build_tools()
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        gits = [a for n, a in self.calls() if n == "git"]
        self.assertTrue(any(any("c893629" in x for x in a) for a in gits), f"pinned commit not used: {gits}")
        self.assertTrue(self.called("cmake"), self.calls())
        self.assert_cpp("q8", False)

    def put_built(self, commit=None):
        """Программа, собранная прошлым запуском; commit — пометка yue2cpp-src/.commit (None — нет)."""
        built = self.yue_home / "yue2cpp-src" / "build" / "yue-server"
        built.parent.mkdir(parents=True)
        shutil.copy(self.stubs / "server-ok", built)
        built.chmod(0o755)
        if commit is not None:
            (self.yue_home / "yue2cpp-src" / ".commit").write_text(commit + "\n")

    def test_already_built_not_rebuilt(self):
        """162е: пометки нет — программа берётся как есть."""
        self.add_build_tools()
        self.put_built()
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake")], [])
        self.assert_cpp("q8", False)

    def test_built_pinned_commit_not_rebuilt(self):
        self.add_build_tools()
        self.put_built("c893629")
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        self.assertEqual([n for n, _ in self.calls() if n in ("git", "cmake")], [])
        self.assert_cpp("q8", False)

    def test_built_other_commit_rebuilt(self):
        """162е: пометка другого коммита — пересборка закреплённого."""
        self.add_build_tools()
        self.put_built("0123abc")
        self.install("1.0.0", "--no-start", "--engine", "cpp", "--cpp-model", "q8", server=None)
        gits = [a for n, a in self.calls() if n == "git"]
        self.assertTrue(any(any("c893629" in x for x in a) for a in gits), f"pinned commit not rebuilt: {gits}")
        self.assertTrue(self.called("cmake"), self.calls())
        self.assert_cpp("q8", False)


class TestBadValues(_EngineCase):
    """E8: неизвестные значения — код 2 до изменений."""

    def assert_refused(self, *args):
        p = self.install("1.0.0", "--no-start", *args, ok=False)
        self.assertEqual(p.returncode, 2, f"{p.stdout}\n{p.stderr}")
        self.assertFalse((self.yue_home / "releases").exists(), "releases/ created despite bad option")
        self.assertFalse(self.settings_path.exists())
        self.assertEqual([n for n, _ in self.calls() if n in ("uv", "systemctl", "venv-python")], [])

    def test_unknown_model(self):
        self.assert_refused("--engine", "cpp", "--cpp-model", "q4")

    def test_unknown_engine(self):
        self.assert_refused("--engine", "foo")

    def test_cpp_options_without_engine_cpp(self):
        """162б: параметры yue2.cpp без --engine cpp — ошибка до изменений."""
        for args in (("--cpp-model", "q8"), ("--cpp-low-vram",), ("--engine", "torch", "--cpp-model", "q8"),
                     ("--engine", "torch", "--cpp-low-vram")):
            with self.subTest(args=args):
                self.assert_refused(*args)

    def test_missing_values(self):
        for args in (("--engine",), ("--engine", "cpp", "--cpp-model")):
            with self.subTest(args=args):
                p = self.install("1.0.0", "--no-start", *args, ok=False)
                self.assertNotEqual(p.returncode, 0, p.stdout)
                self.assertFalse((self.yue_home / "releases").exists())


if __name__ == "__main__":
    unittest.main()
