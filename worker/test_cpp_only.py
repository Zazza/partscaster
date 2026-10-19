"""Генерация только на yue2.cpp, движок PyTorch убран (карточка internal-cpp-only).

Тест-кейсы (ТК → условие карточки):
ТК1  (усл. 4)  engine_cpp.recommend(vram_mib) → (модель, экономный): ≥ 11776 → ("bf16", False);
               ≥ 9728 → ("q8", False); меньше и 0 (неизвестно) → ("q8", True). Границы 11776/11775,
               9728/9727, типовые карты 8188/12282/16376.
ТК2  (усл. 4)  CLI `python engine_cpp.py recommend [МиБ]` печатает «<модель> <0|1>»; без аргумента —
               первая карта по nvidia-smi; nvidia-smi нет — как неизвестно («q8 1»).
ТК3  (усл. 2, 4) GET /config: нет ключа engine; cpp_recommended = {model, low_vram, vram_mib},
               рекомендация = recommend(vram_mib); видеопамять неизвестна → vram_mib 0. Видеопамять
               воркер берёт у nvidia-smi (первая карта; нет программы — 0).
ТК4  (усл. 2)  POST /config со старым полем engine (torch/cpp) → 200, поле игнорируется: нет в ответе,
               нет в settings.json, /config без engine.
ТК5  (усл. 3)  Любая скачанная модель (bf16/q8/q6/q5) выбирается → 200 и сохраняется; нескачанная →
               422 «сначала скачайте»; экономный режим — отдельной галкой; неизвестное имя → 422.
ТК6  (усл. 1, 2) Без настроек и со старым "engine": "torch" в settings.json генерация идёт через
               yue2.cpp: job.engine = "cpp-<модель>", программа yue-server получила заявку; пакет yue2
               недоступен (импорт невозможен) — генерации это не мешает. Черновик, план нот (/plan),
               продолжение и превью куска (декод латентов) — тоже через yue2.cpp.
ТК7  (усл. 6)  Модель не скачана → генерация в error с понятным текстом про скачивание модели.
ТК8  (усл. 1)  В воркере нет _get_pipe, _continue_song, MODEL_ID, YUE2_BACKENDS, engine_pref,
               ENGINE_CHOICES; исходники worker/*.py (кроме test_*) не импортируют пакет yue2.
ТК9  (усл. 8б) /stats model_loaded: false до запуска yue2.cpp, true пока сервер yue2.cpp запущен.
ТК10 (усл. 7)  Продолжение трека PyTorch (engine "torch") идёт на yue2.cpp по его semantic.npy и
               score.abc; рецепт трека с engine "" или "torch" — worker_model не метка ни одной модели
               yue2.cpp (импорт сочтёт модель другой); трека "cpp-q8" — model_label("q8").
ТК11 (усл. 5)  requirements.txt без yue2-infer, torchaudio остаётся.
ТК12 (усл. 5, 6) install.sh: --engine — неизвестный параметр (ненулевой код, текст); без флагов —
               рекомендуемая модель по nvidia-smi, скачивается, settings.json: cpp_model/cpp_low_vram
               без engine; --cpp-model q6, --cpp-model auto, --cpp-low-vram; повторный запуск при
               скачанной выбранной модели не меняет выбор и не качает другую модель; старое
               {"engine":"torch"} без модели → рекомендуемая, ключ engine удалён; yue2.cpp не найти и
               не собрать → ненулевой код; модель не скачалась → ненулевой код, engine "torch" не записан;
               своих порогов видеопамяти (11776/9728/7680) и YUE2_BACKEND в install.sh нет.
ТК13 (усл. 6а) worker-tar.sh / release.sh с TARGET_OS=windows без dist/yue2cpp-windows-x86_64/
               yue-server.exe → ненулевой код с подсказкой про yue2.cpp; TARGET_OS=linux без сборки —
               архив воркера собирается.

Внешние границы подменяются: программа yue-server (скрипт-фейк из test_engine_cpp), nvidia-smi
(скрипт в PATH или функция воркера _gpu_vram_mib — по контракту исполнителя), загрузка моделей
(в песочнице установщика), wails/go/docker (заглушки). Свои функции воркера не мокаются.
"""
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock

from test_engine_cpp import (DEFAULT_ABC, _tokens, fake_requests, fake_starts, kill_fakes,
                             make_fake_server, patch_model_sums, place_model, synth_bodies)
from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
ALL_MODELS = ("bf16", "q8", "q6", "q5")


def _engine_cpp():
    import engine_cpp
    return engine_cpp


def _write_exe(path: Path, text: str):
    path.write_text(text)
    path.chmod(0o755)


def _fake_nvidia_smi(directory: Path, *mib):
    """nvidia-smi: на запрос memory.total печатает объём по строке на карту (МиБ, как с nounits)."""
    body = "".join(f"{m}\n" for m in mib)
    _write_exe(directory / "nvidia-smi",
               "#!/bin/sh\ncase \"$*\" in *memory.total*) printf '%s' '" + body + "';; esac\nexit 0\n")


# --- ТК1 ---------------------------------------------------------------------------------------

class TestRecommend(unittest.TestCase):
    """ТК1: одна функция рекомендации, пороги по замерам."""

    CASES = {
        16376: ("bf16", False),   # карта «16 ГБ»
        12282: ("bf16", False),   # карта «12 ГБ» сообщает меньше 12288
        11776: ("bf16", False),
        11775: ("q8", False),
        9728: ("q8", False),
        9727: ("q8", True),
        8188: ("q8", True),       # карта «8 ГБ»
        0: ("q8", True),          # неизвестно
    }

    def test_boundaries(self):
        e = _engine_cpp()
        for mib, want in self.CASES.items():
            with self.subTest(vram_mib=mib):
                got = e.recommend(mib)
                self.assertEqual(tuple(got), want)
                self.assertIs(got[1], want[1], "экономный режим — bool")

    def test_model_is_known(self):
        e = _engine_cpp()
        for mib in self.CASES:
            self.assertIn(e.recommend(mib)[0], e.MODELS)


# --- ТК2 ---------------------------------------------------------------------------------------

class TestRecommendCli(unittest.TestCase):
    """ТК2: python engine_cpp.py recommend [МиБ]."""

    def setUp(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        self.bin = Path(td.name) / "bin"
        self.bin.mkdir()
        self.data = Path(td.name) / "data"

    def _run(self, *args, nvidia=None):
        if nvidia is not None:
            _fake_nvidia_smi(self.bin, *nvidia)
        env = {k: v for k, v in os.environ.items() if not k.startswith("PARTSCASTER_")}
        # в PATH только каталог-песочница: настоящего nvidia-smi машины там нет
        env.update(PATH=str(self.bin), PARTSCASTER_DATA_DIR=str(self.data))
        p = subprocess.run([sys.executable, str(WORKER_DIR / "engine_cpp.py"), "recommend", *args],
                           cwd=WORKER_DIR, env=env, capture_output=True, text=True, timeout=60)
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        return p.stdout.strip()

    def test_small_card(self):
        self.assertEqual(self._run("8188"), "q8 1")

    def test_12gb_card(self):
        self.assertEqual(self._run("12282"), "bf16 0")

    def test_10gb_card(self):
        self.assertEqual(self._run("9728"), "q8 0")

    def test_no_arg_no_nvidia_smi(self):
        self.assertEqual(self._run(), "q8 1")

    def test_no_arg_first_card_from_nvidia_smi(self):
        self.assertEqual(self._run(nvidia=(12282, 6144)), "bf16 0")

    def test_no_arg_first_card_small(self):
        self.assertEqual(self._run(nvidia=(8188, 24576)), "q8 1")


# --- общая подготовка API --------------------------------------------------------------------

def _setup(case):
    """Данные/настройки во временном каталоге, каталог моделей туда же, суммы моделей —
    на маленькие файлы теста; пакет yue2 недоступен (импорт падает)."""
    tmp = Path(case._td.name)
    p = mock.patch.dict(sys.modules, {"yue2": None, "yue2.pipeline": None, "yue2.protocol": None,
                                      "torch": mock.MagicMock()})
    p.start()
    case.addCleanup(p.stop)
    data = tmp / "cfg"
    data.mkdir(exist_ok=True)
    case.settings = data / "settings.json"
    for name, val in (("DATA_DIR", data), ("SETTINGS_PATH", case.settings)):
        p = mock.patch.object(case.w, name, val)
        p.start()
        case.addCleanup(p.stop)
    case.models = tmp / "models"
    case.models.mkdir(exist_ok=True)
    p = mock.patch.dict(os.environ, {"PARTSCASTER_CPP_MODELS": str(case.models),
                                     "PARTSCASTER_HOME": str(tmp / "home")})
    p.start()
    case.addCleanup(p.stop)
    os.environ.pop("PARTSCASTER_CPP_BIN", None)
    case.e = _engine_cpp()
    patch_model_sums(case, case.e, ALL_MODELS)
    case.fake_dir = tmp / "fake"
    case.addCleanup(kill_fakes, case.fake_dir)
    case.addCleanup(_stop_cpp, case.w)


def _stop_cpp(w):
    """Сервер yue2.cpp, запущенный воркером в тесте, — остановить (не держит процесс между тестами)."""
    srv = getattr(w, "_cpp", None)
    if srv is not None and hasattr(srv, "stop"):
        try:
            srv.stop()
        except Exception:  # noqa: BLE001 - уборка после теста, процесс уже мог завершиться
            pass


def _vram(case, mib):
    """Видеопамять, которую видит воркер (граница — nvidia-smi; функция по контракту исполнителя)."""
    p = mock.patch.object(case.w, "_gpu_vram_mib", lambda: mib, create=True)
    p.start()
    case.addCleanup(p.stop)


class _Case(_WorkerApiCase):
    def setUp(self):
        super().setUp()
        _setup(self)
        _vram(self, 0)

    def _settings(self, **st):
        self.settings.write_text(json.dumps(st))

    def _stored(self):
        return json.loads(self.settings.read_text()) if self.settings.exists() else {}

    def _fake(self, **scenario):
        os.environ["PARTSCASTER_CPP_BIN"] = str(make_fake_server(self.fake_dir, **scenario))

    def _cfg(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _new(self, **extra):
        r = self.client.post("/jobs", json={"style": "dark rock", "lyrics": "[verse] la la", **extra})
        self.assertEqual(r.status_code, 200, r.text)
        d = r.json()
        jid = int(d.get("id", d.get("job_id")))
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running' WHERE id=?", (jid,))   # как после захвата очередью
        return jid

    def _run(self, jid, timeout=60):
        box = {}

        def run():
            try:
                self.w._run_job(jid)
            except BaseException as e:  # noqa: BLE001 - исключение наружу = провал теста
                box["exc"] = e

        t = threading.Thread(target=run, daemon=True)
        t.start()
        t.join(timeout)
        self.assertFalse(t.is_alive(), "генерация не закончилась за отведённое время")
        if "exc" in box:
            raise box["exc"]

    def _get(self, jid):
        r = self.client.get(f"/jobs/{jid}")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()


# --- ТК3 ---------------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestConfigRecommended(_Case):
    """ТК3: /config без engine, с подсказкой cpp_recommended."""

    def check(self, mib):
        _vram(self, mib)
        c = self._cfg()
        self.assertNotIn("engine", c)
        rec = c.get("cpp_recommended")
        self.assertIsInstance(rec, dict, c)
        model, low = self.e.recommend(mib)
        self.assertEqual(rec.get("model"), model, rec)
        self.assertIs(rec.get("low_vram"), low, rec)
        self.assertEqual(rec.get("vram_mib"), mib, rec)
        self.assertIsInstance(rec.get("vram_mib"), int)
        return c

    def test_8gb(self):
        self.check(8188)

    def test_12gb(self):
        self.check(12282)

    def test_unknown(self):
        self.check(0)

    def test_hint_does_not_select(self):
        """Подсказка ничего не выбирает: выбор пользователя (q6) остаётся, settings.json не меняется."""
        place_model(self.models, self.e, "q6")
        self._settings(cpp_model="q6", cpp_low_vram=False)
        before = self.settings.read_text()
        c = self.check(12282)
        self.assertEqual(c["cpp_model"], "q6")
        self.assertIs(c["cpp_low_vram"], False)
        self.assertEqual(self.settings.read_text(), before)

    def test_all_models_listed(self):
        c = self._cfg()
        self.assertEqual(set(c["cpp_models"]), set(ALL_MODELS))


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestGpuVramFromNvidiaSmi(_WorkerApiCase):
    """ТК3: видеопамять воркер берёт у nvidia-smi — первая карта; программы нет — 0."""

    def setUp(self):
        super().setUp()
        self.bin = Path(self._td.name) / "nvbin"
        self.bin.mkdir()

    def _measure(self):
        fn = getattr(self.w, "_gpu_vram_mib", None)
        self.assertTrue(callable(fn), "в воркере нет _gpu_vram_mib()")
        if hasattr(fn, "cache_clear"):
            fn.cache_clear()
        with mock.patch.dict(os.environ, {"PATH": str(self.bin)}):
            got = fn()
        if hasattr(fn, "cache_clear"):
            fn.cache_clear()
        return got

    def test_first_card(self):
        _fake_nvidia_smi(self.bin, 12282, 6144)
        self.assertEqual(self._measure(), 12282)

    def test_no_nvidia_smi(self):
        self.assertEqual(self._measure(), 0)


# --- ТК4, ТК5 ----------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestConfigNoEngine(_Case):
    """ТК4: старое поле engine в запросе игнорируется."""

    def test_old_client_engine_ignored(self):
        for val in ("torch", "cpp"):
            with self.subTest(engine=val):
                r = self.client.post("/config", json={"engine": val})
                self.assertEqual(r.status_code, 200, r.text)
                body = r.json() if r.content else {}
                if isinstance(body, dict):
                    self.assertNotIn("engine", body)
                self.assertNotIn("engine", self._stored())
                self.assertNotIn("engine", self._cfg())

    def test_engine_with_other_fields_applies_the_rest(self):
        r = self.client.post("/config", json={"engine": "torch", "cpp_low_vram": True})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(self._cfg()["cpp_low_vram"], True)
        self.assertNotIn("engine", self._stored())

    def test_old_settings_engine_not_shown(self):
        self._settings(engine="torch", stems_model="roformer")
        self.assertNotIn("engine", self._cfg())


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestModelChoice(_Case):
    """ТК5: любая скачанная модель, нескачанная — отказ «сначала скачайте»."""

    def test_any_installed_model_selectable(self):
        self._fake()
        for m in ALL_MODELS:
            place_model(self.models, self.e, m)
        for m in ALL_MODELS:
            with self.subTest(model=m):
                r = self.client.post("/config", json={"cpp_model": m})
                self.assertEqual(r.status_code, 200, r.text)
                self.assertEqual(self._cfg()["cpp_model"], m)
                self.assertEqual(self._stored().get("cpp_model"), m)

    def test_bf16_on_small_card_allowed(self):
        """Рекомендация — только подсказка: bf16 при 8 ГБ выбрать можно («его дело»)."""
        _vram(self, 8188)
        self._fake()
        place_model(self.models, self.e, "bf16")
        r = self.client.post("/config", json={"cpp_model": "bf16"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._cfg()["cpp_model"], "bf16")

    def test_not_downloaded_refused(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8")
        r = self.client.post("/config", json={"cpp_model": "bf16"})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("скача", r.text.lower())
        self.assertEqual(self._cfg()["cpp_model"], "q8")

    def test_unknown_model_refused(self):
        for m in ("q4", "Q8", "torch", ""):
            with self.subTest(model=m):
                r = self.client.post("/config", json={"cpp_model": m})
                self.assertEqual(r.status_code, 422, r.text)

    def test_low_vram_separate_toggle(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        for v in (True, False):
            r = self.client.post("/config", json={"cpp_low_vram": v})
            self.assertEqual(r.status_code, 200, r.text)
            self.assertIs(self._cfg()["cpp_low_vram"], v)


# --- ТК6, ТК7, ТК9 ------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestGenerationCppOnly(_Case):
    """ТК6: генерация только через yue2.cpp, без пакета yue2."""

    def test_fresh_settings_track_on_cpp(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8")
        jid = self._new(seed=7)
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q8")
        self.assertTrue(synth_bodies(self.fake_dir), "заявка не дошла до yue2.cpp")

    def test_no_settings_file_default_model(self):
        """Настроек нет вовсе — модель по умолчанию из /config, генерация на yue2.cpp."""
        self._fake()
        for m in ALL_MODELS:
            place_model(self.models, self.e, m)
        self.assertFalse(self.settings.exists())
        model = self._cfg()["cpp_model"]
        self.assertIn(model, ALL_MODELS)
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], f"cpp-{model}")

    def test_old_engine_torch_in_settings_ignored(self):
        self._fake()
        place_model(self.models, self.e, "q6")
        self._settings(engine="torch", cpp_model="q6", cpp_low_vram=False)
        self.assertNotIn("engine", self._cfg())
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q6")
        self.assertEqual(len(fake_starts(self.fake_dir)), 1)

    def test_draft_on_cpp(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(engine="torch", cpp_model="q8")
        jid = self._new(draft=True)
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q8")

    def test_plan_on_cpp(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(engine="torch", cpp_model="q8")
        r = self.client.post("/plan", json={"style": "rock", "lyrics": "[verse] la", "cot": "full", "seed": 5})
        self.assertTrue(synth_bodies(self.fake_dir), f"план нот не пошёл в yue2.cpp: {r.status_code} {r.text}")
        self.assertEqual(r.status_code, 200, r.text)

    def test_preview_decode_on_cpp(self):
        """Превью куска (декод латентов) — запрос уходит программе yue2.cpp."""
        import numpy as np
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(engine="torch", cpp_model="q8")
        jid = self._job(duration=20.0)
        np.save(self.jobs_dir / str(jid) / "latent.npy", np.zeros((500, 64), dtype=np.float32))
        r = self.client.post(f"/jobs/{jid}/preview", json={"from_sec": 2, "to_sec": 6})
        posts = [q["path"] for q in fake_requests(self.fake_dir) if q["method"] == "POST"]
        self.assertTrue(posts, f"декод не дошёл до yue2.cpp: {r.status_code} {r.text}")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestNoModel(_Case):
    """ТК7: модели нет — генерация отказывает понятным текстом."""

    def test_job_error_about_download(self):
        self._fake()
        self._settings(engine="torch")   # старая установка на PyTorch, модель yue2.cpp не скачана
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "error", got)
        self.assertIn("скача", (got.get("error") or "").lower())
        self.assertEqual(fake_starts(self.fake_dir), [], "без модели сервер не запускается")

    def test_no_settings_no_model(self):
        self._fake()
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "error", got)
        self.assertIn("скача", (got.get("error") or "").lower())


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestStatsModelLoaded(_Case):
    """ТК9: подвал «Модель YuE2 в памяти» видит запущенный yue2.cpp."""

    def _loaded(self):
        r = self.client.get("/stats")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json().get("model_loaded")

    def test_false_before_start(self):
        self.assertIs(self._loaded(), False)

    def test_true_while_cpp_running(self):
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8")
        jid = self._new()
        self._run(jid)
        self.assertEqual(self._get(jid)["status"], "done")
        self.assertIs(self._loaded(), True)


# --- ТК8 ---------------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestNoTorchEngineCode(_WorkerApiCase):
    """ТК8: в воркере нет кода движка PyTorch."""

    GONE = ("_get_pipe", "_continue_song", "MODEL_ID", "YUE2_BACKENDS", "engine_pref", "ENGINE_CHOICES")

    def test_attributes_gone(self):
        left = [n for n in self.GONE if hasattr(self.w, n)]
        self.assertEqual(left, [], "в yue_worker остался код движка PyTorch")


class TestNoYue2Imports(unittest.TestCase):
    """ТК8: исходники воркера не импортируют пакет yue2."""

    # «from yue2…» / «import yue2…» в любом месте строки; yue2.cpp и yue2cpp — не пакет yue2
    RE = re.compile(r"\b(?:from|import)\s+yue2(?!\.cpp|cpp)\b")

    def test_sources(self):
        bad = []
        for f in sorted(WORKER_DIR.glob("*.py")):
            if f.name.startswith("test_"):
                continue
            text = f.read_text(encoding="utf-8")
            if self.RE.search(text):
                bad.append(f.name)
        self.assertEqual(bad, [], "импорт пакета yue2 в воркере")


# --- ТК10 --------------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestOldTorchTracks(_Case):
    """ТК10: старые треки PyTorch продолжаются на yue2.cpp; рецепт пишет модель трека."""

    def test_continue_torch_track_on_cpp(self):
        import numpy as np
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8")
        parent = self._job(duration=60.0)
        with self._conn() as c:
            c.execute("UPDATE jobs SET engine='torch' WHERE id=?", (parent,))
        pdir = self.jobs_dir / str(parent)
        (pdir / "score.abc").write_text(DEFAULT_ABC, encoding="utf-8")
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 30.0})
        self.assertLess(r.status_code, 300, r.text)
        kid = self._children(parent)[0]["id"]
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running' WHERE id=?", (kid,))
        self._run(kid)
        got = self._get(kid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q8")
        body = synth_bodies(self.fake_dir)[0]
        self.assertEqual(_tokens(body["source_tokens"]), np.load(pdir / "semantic.npy").tolist())
        self.assertEqual(body["abc"], DEFAULT_ABC, "ноты до отметки — из score.abc старого трека")

    def _mk(self, engine):
        with self._conn() as c:
            return c.execute(
                "INSERT INTO jobs (title, status, style, lyrics, seed, duration_sec, engine, created_at) "
                "VALUES ('t', 'done', 'dark rock', '[verse] la', 42, 60.0, ?, '2026-10-01T00:00:00')",
                (engine,)).lastrowid

    def _worker_model(self, jid):
        r = self.client.get(f"/jobs/{jid}/recipe")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()["worker_model"]

    def test_recipe_old_track_not_cpp_label(self):
        # воркер настроен на yue2.cpp q8 — рецепт старого трека всё равно не выдаёт его за трек yue2.cpp
        self._fake()
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8")
        labels = {self.e.model_label(m) for m in self.e.MODELS}
        for engine in ("", "torch"):
            with self.subTest(engine=engine):
                wm = self._worker_model(self._mk(engine))
                self.assertTrue(wm, "worker_model пуст")
                self.assertNotIn(wm, labels)

    def test_recipe_cpp_track_label(self):
        self._settings(cpp_model="bf16")
        self.assertEqual(self._worker_model(self._mk("cpp-q8")), self.e.model_label("q8"))


# --- ТК11 --------------------------------------------------------------------------------------

class TestRequirements(unittest.TestCase):
    """ТК11: yue2-infer не ставится, torchaudio остаётся."""

    def _lines(self):
        out = []
        for ln in (WORKER_DIR / "requirements.txt").read_text(encoding="utf-8").splitlines():
            ln = ln.split("#", 1)[0].strip()
            if ln:
                out.append(ln.lower())
        return out

    def test_no_yue2_infer(self):
        self.assertEqual([ln for ln in self._lines() if "yue2-infer" in ln or "yue2_infer" in ln], [])

    def test_torchaudio_kept(self):
        self.assertTrue(any(re.match(r"torchaudio\b", ln) for ln in self._lines()), self._lines())


# --- ТК12: install.sh -------------------------------------------------------------------------

try:
    import test_install_engine as _ie
    _EngineCase = _ie._EngineCase
except ImportError:   # без bash/tar песочница установщика недоступна
    _ie = None
    _EngineCase = unittest.TestCase

# venv-python песочницы: kits_install.py — заглушка; engine_cpp.py download — фейковая загрузка
# (кладёт файлы модели и VAE нужного размера, без сети); остальное (recommend и любые проверки
# через python воркера) — настоящий python, чтобы рекомендация шла из настоящей функции.
CPP_VENV_PY = r"""#!/bin/sh
{ echo "== venv-python"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
printf '%s\t%s\t%s\t%s\n' "$PWD" "${PARTSCASTER_DATA_DIR-}" "${PARTSCASTER_CPP_MODELS-}" "$*" >> "$STUB_DIR/py-ctx.log"
case "$1" in
  *kits_install.py) [ -f "$STUB_DIR/fail-kits" ] && exit 1; exit 0;;
  *engine_cpp.py)
    if [ "$2" = download ]; then
      [ -f "$STUB_DIR/fail-download" ] && exit 1
      exec "__REAL_PY__" "$STUB_DIR/fake_download.py" "$1" "$3"
    fi;;
esac
exec "__REAL_PY__" "$@"
"""

FAKE_DOWNLOAD = r"""import os, sys
sys.path.insert(0, os.path.dirname(os.path.abspath(sys.argv[1])))
import engine_cpp as e
d = e.models_dir()
d.mkdir(parents=True, exist_ok=True)
for spec in (e.MODELS[sys.argv[2]], e.VAE):
    with open(d / spec["file"], "ab") as f:   # разреженный файл нужного размера
        f.truncate(spec["size"])
"""


class _CppInstall(_EngineCase):
    def setUp(self):
        if _ie is None:
            self.skipTest("нет песочницы установщика")
        super().setUp()
        (self.stubs / "venv-python").write_text(CPP_VENV_PY.replace("__REAL_PY__", sys.executable))
        (self.stubs / "fake_download.py").write_text(FAKE_DOWNLOAD)

    def e_file(self, model):
        return _engine_cpp().MODELS[model]["file"]

    def model_files(self):
        d = self.data / "yue2cpp-models"
        return sorted(p.name for p in d.iterdir()) if d.is_dir() else []

    def assert_model(self, model, low):
        st = self.settings()
        self.assertNotIn("engine", st, st)
        self.assertEqual(st.get("cpp_model"), model, st)
        self.assertIs(st.get("cpp_low_vram"), low, st)


class TestInstallCppOnly(_CppInstall):
    """ТК12: установка без выбора движка."""

    def test_engine_flag_unknown(self):
        for args in (("--engine", "cpp"), ("--engine", "torch")):
            with self.subTest(args=args):
                p = self.install("1.0.0", "--no-start", *args, ok=False)
                self.assertNotEqual(p.returncode, 0, p.stdout)
                out = p.stdout + p.stderr
                self.assertIn("неизвестный параметр", out)
                self.assertIn("--engine", out)
                self.assertFalse((self.yue_home / "releases").exists(), "установка шла с --engine")

    def check_default(self, mib, model, low):
        self.set_vram(mib)
        self.install("1.0.0", "--no-start")
        self.assert_model(model, low)
        self.assertEqual([d[:2] for d in self.downloads()], [["download", model]], self.calls())
        self.assertIn(self.e_file(model), self.model_files())

    def test_default_8gb(self):
        self.check_default(8188, "q8", True)

    def test_default_10gb(self):
        self.check_default(10 * 1024, "q8", False)

    def test_default_12gb(self):
        self.check_default(12282, "bf16", False)

    def test_default_first_card(self):
        self.set_vram(8188, 24576)
        self.install("1.0.0", "--no-start")
        self.assert_model("q8", True)

    def test_auto_is_recommended(self):
        self.set_vram(12282)
        self.install("1.0.0", "--no-start", "--cpp-model", "auto")
        self.assert_model("bf16", False)

    def test_explicit_model(self):
        self.set_vram(16376)
        self.install("1.0.0", "--no-start", "--cpp-model", "q6")
        self.assertEqual(self.settings().get("cpp_model"), "q6")
        self.assertNotIn("engine", self.settings())
        self.assertEqual([d[:2] for d in self.downloads()], [["download", "q6"]], self.calls())

    def test_low_vram_flag(self):
        self.set_vram(16376)   # рекомендация — без экономного режима
        self.install("1.0.0", "--no-start", "--cpp-low-vram")
        self.assert_model("bf16", True)

    def test_unknown_model_refused(self):
        p = self.install("1.0.0", "--no-start", "--cpp-model", "q4", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertFalse((self.yue_home / "releases").exists())

    def test_other_settings_kept(self):
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"stems_model": "roformer"}', encoding="utf-8")
        self.set_vram(8188)
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.settings().get("stems_model"), "roformer")
        self.assert_model("q8", True)


class TestInstallNoOwnThresholds(unittest.TestCase):
    """ТК12 (усл. 4): у install.sh своих порогов видеопамяти нет — рекомендация из engine_cpp.recommend."""

    def test_no_threshold_numbers(self):
        text = (WORKER_DIR / "install.sh").read_text(encoding="utf-8")
        self.assertEqual(re.findall(r"\b(?:11776|9728|7680)\b", text), [])
        self.assertNotIn("YUE2_BACKEND", text)


class TestInstallUpdate(_CppInstall):
    """ТК12 (усл. 6): обновление существующей установки."""

    def test_rerun_keeps_user_choice(self):
        self.set_vram(16376)
        self.install("1.0.0", "--no-start", "--cpp-model", "q6", "--cpp-low-vram")
        self.assert_model("q6", True)
        self.install("1.0.1", "--no-start")
        self.assert_model("q6", True)
        self.assertEqual([d for d in self.downloads() if d[1:2] != ["q6"]], [], self.calls())
        self.assertNotIn(self.e_file("bf16"), self.model_files(), "скачана рекомендуемая вместо выбранной")

    def test_rerun_drops_old_engine_key(self):
        """Скачанный выбор сохраняется, а ключ engine прежних версий из settings.json убирается."""
        self.set_vram(16376)
        self.install("1.0.0", "--no-start", "--cpp-model", "q6", "--cpp-low-vram")
        st = self.settings()
        st["engine"] = "cpp"
        self.settings_path.write_text(json.dumps(st), encoding="utf-8")
        self.install("1.0.1", "--no-start")
        self.assert_model("q6", True)

    def test_old_torch_install_gets_recommended(self):
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"engine": "torch", "stems_model": "roformer"}', encoding="utf-8")
        self.set_vram(12282)
        self.install("1.0.0", "--no-start")
        self.assert_model("bf16", False)
        self.assertEqual(self.settings().get("stems_model"), "roformer")
        self.assertEqual([d[:2] for d in self.downloads()], [["download", "bf16"]], self.calls())

    def test_removed_model_reinstalled(self):
        """Модель выбрана, но её файла нет — ставится рекомендуемая (или из флага)."""
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"cpp_model": "q5", "cpp_low_vram": false}', encoding="utf-8")
        self.set_vram(8188)
        self.install("1.0.0", "--no-start")
        self.assert_model("q8", True)


class TestInstallFailures(_CppInstall):
    """ТК12 (усл. 5): yue2.cpp и модель обязательны."""

    def test_no_binary_no_build_fails(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        p = self.install("1.0.0", "--no-start", server=None, ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertIn("yue2.cpp", p.stdout + p.stderr)

    def test_own_binary_from_worker_env_used(self):
        """Своя программа в worker.env (PARTSCASTER_CPP_BIN) — воркер берёт её первой; установка без программы в
        архиве и без инструментов сборки проходит."""
        self.no_nvcc_guard()
        own = self.stubs / "own-yue-server"
        own.write_text(_ie.base.STUB)
        own.chmod(0o755)
        self.yue_home.mkdir(parents=True, exist_ok=True)
        (self.yue_home / "worker.env").write_text(f"HF_HOME=/data/hf-cache\nPARTSCASTER_CPP_BIN={own}\n")
        self.set_vram(8188)
        self.install("1.0.0", "--no-start", server=None)
        self.assert_model("q8", True)

    def test_broken_binary_no_build_fails(self):
        self.no_nvcc_guard()
        self.add_build_tools(nvcc=False)
        p = self.install("1.0.0", "--no-start", server="broken", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)

    def test_download_failure_fails_without_torch(self):
        (self.stubs / "fail-download").touch()
        self.data.mkdir(parents=True)
        self.settings_path.write_text('{"stems_model": "roformer"}', encoding="utf-8")
        p = self.install("1.0.0", "--no-start", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        st = self.settings()
        self.assertNotEqual(st.get("engine"), "torch", st)
        self.assertEqual(st.get("stems_model"), "roformer")
        env = self.yue_home / "worker.env"
        if env.exists():
            self.assertNotIn("YUE2_BACKEND", env.read_text())

    def test_download_failure_fresh_no_torch(self):
        (self.stubs / "fail-download").touch()
        p = self.install("1.0.0", "--no-start", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        if self.settings_path.exists():
            self.assertNotEqual(self.settings().get("engine"), "torch")


# --- ТК13: релиз --------------------------------------------------------------------------------

def _sandbox_repo(root: Path, with_deploy_all=False):
    (root / "tools").mkdir(parents=True)
    for s in ("release.sh", "worker-tar.sh"):
        shutil.copy2(REPO / "tools" / s, root / "tools" / s)
    shutil.copytree(WORKER_DIR, root / "worker",
                    ignore=shutil.ignore_patterns("__pycache__", "*.pyc", "yue2cpp"))
    if with_deploy_all:
        shutil.copytree(REPO / "deploy", root / "deploy")
    else:
        shutil.copytree(REPO / "deploy" / "units", root / "deploy" / "units")


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class TestWorkerTarTargets(unittest.TestCase):
    """ТК13: архив воркера Windows без сборки yue2.cpp не собирается; Linux — собирается."""

    VERSION = "9.8.7-only"

    def setUp(self):
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        self.root = Path(td.name) / "repo"
        _sandbox_repo(self.root)
        self.out = Path(td.name) / "out"

    def _build(self, target_os):
        env = dict(os.environ, TARGET_OS=target_os)
        return subprocess.run(["bash", str(self.root / "tools" / "worker-tar.sh"), str(self.out), self.VERSION],
                              cwd=self.root, capture_output=True, text=True, timeout=120, env=env)

    def test_windows_without_build_fails(self):
        p = self._build("windows")
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertIn("yue2", (p.stdout + p.stderr).lower())
        self.assertFalse((self.out / f"worker-{self.VERSION}.tar.gz").exists(), "архив Windows без yue2.cpp")

    def test_windows_linux_build_only_fails(self):
        d = self.root / "dist" / "yue2cpp-linux-x86_64"
        d.mkdir(parents=True)
        _write_exe(d / "yue-server", "#!/bin/sh\n")
        p = self._build("windows")
        self.assertNotEqual(p.returncode, 0, p.stdout)

    def test_windows_with_build_ok(self):
        d = self.root / "dist" / "yue2cpp-windows-x86_64"
        d.mkdir(parents=True)
        (d / "yue-server.exe").write_bytes(b"MZ fake")
        p = self._build("windows")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        with tarfile.open(self.out / f"worker-{self.VERSION}.tar.gz", "r:gz") as tf:
            self.assertIn(f"worker-{self.VERSION}/yue2cpp/yue-server.exe", tf.getnames())

    def test_linux_without_build_ok(self):
        p = self._build("linux")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertTrue((self.out / f"worker-{self.VERSION}.tar.gz").is_file())


@unittest.skipUnless(shutil.which("bash") and shutil.which("tar"), "нужны bash и tar")
class TestReleaseWindowsNeedsCpp(unittest.TestCase):
    """ТК13: release.sh TARGET_OS=windows без сборки yue2.cpp — отказ с подсказкой."""

    def setUp(self):
        import test_release_targets as rt
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        base = Path(td.name)
        self.root = base / "repo"
        _sandbox_repo(self.root, with_deploy_all=True)
        self.bin = base / "bin"
        self.bin.mkdir()
        _write_exe(self.bin / "go", rt.GO_STUB)
        _write_exe(self.bin / "wails", rt.WAILS_STUB)
        # docker/nvcc/powershell — заглушки с отказом: настоящая сборка yue2.cpp не запускается
        for name in ("docker", "nvcc", "powershell"):
            _write_exe(self.bin / name, "#!/bin/sh\nexit 1\n")
        self.log = base / "stub.log"
        self.log.write_text("")
        self.rt = rt

    def test_windows_without_cpp_build_fails(self):
        env = {k: v for k, v in os.environ.items() if k not in ("TARGET_OS", "GOOS", "GOARCH", "WAILS")}
        env.update(PATH=f"{self.bin}:/usr/bin:/bin", WAILS=str(self.bin / "wails"), STUB_LOG=str(self.log),
                   SRCHASH_VALUE=self.rt.SRCHASH, FAKE_HOSTOS="linux", FAKE_HOSTARCH="amd64",
                   TARGET_OS="windows")
        p = subprocess.run(["bash", str(self.root / "tools" / "release.sh"), self.rt.VERSION],
                           cwd=self.root, env=env, capture_output=True, text=True, timeout=180)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertIn("yue2", (p.stdout + p.stderr).lower())
        pk = sorted(x.name for x in (self.root / "dist").glob("partscaster-*")) \
            if (self.root / "dist").exists() else []
        self.assertEqual([n for n in pk if n.endswith(".zip")], [], "пакет Windows без yue2.cpp собран")


if __name__ == "__main__":
    unittest.main()
