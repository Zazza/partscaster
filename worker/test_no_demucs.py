"""Дорожки только RoFormer, Demucs убран (карточка internal-no-demucs).

Тест-кейсы (ТК → условие карточки):
  ТК1  (усл. 1)  stems.separate(audio_path, out_dir) — без параметра prefer; в stems нет MODEL_DEMUCS,
                 _run_model, roformer_enabled;
  ТК2  (усл. 1)  исходники воркера (worker/*.py, кроме test_*) не импортируют demucs, не содержат строк
                 'htdemucs'/'htdemucs_6s' и отката PARTSCASTER_STEMS_MODEL;
  ТК3  (усл. 1, 4) рабочий RoFormer: model = bs-roformer-sw; drums/bass/other/vocals в сумме — трек;
                 guitar/piano — внутри «прочего»; части барабанов kick…crash вне суммы;
  ТК4  (усл. 1)  sep_run.py упал (код 1) → RuntimeError с текстом про RoFormer/разделение, дорожек нет,
                 запасного пути нет;
  ТК5  (усл. 1)  окружения RoFormer нет (нет файла PARTSCASTER_SEP_PY) → RuntimeError, подпроцесс не звался;
  ТК6  (усл. 1)  таймаут подпроцесса → исключение, дорожек нет;
  ТК7  (усл. 1)  PARTSCASTER_STEMS_MODEL=demucs (старый откат в worker.env) ничего не меняет — делит RoFormer;
  ТК8  (усл. 4)  сбой DrumSep → основные и подробные дорожки есть, частей нет, model = bs-roformer-sw;
  ТК9  (усл. 1)  битый/пустой файл → исключение, частичных дорожек нет;
  ТК10 (усл. 6)  stems._sep_python() без PARTSCASTER_SEP_PY: Windows → ~/sep-venv/Scripts/python.exe,
                 иначе ~/sep-venv/bin/python; с переменной — она;
  ТК11 (усл. 2)  yue_worker без STEMS_CHOICES и stems_pref; GET /config без stems_pref/stems_model,
                 roformer_available (bool) отражает наличие окружения;
  ТК12 (усл. 2)  POST /config {stems_model: htdemucs|roformer} (старый клиент) → 2xx, в settings.json
                 stems_model не пишется;
  ТК13 (усл. 2)  старое stems_model в settings.json не ломает: /config 200, дорожки — RoFormer;
  ТК14 (усл. 2)  POST /jobs/{id}/stems?model=htdemucs (старый клиент) → не 422, делит RoFormer;
  ТК15 (усл. 1)  POST /jobs/{id}/stems без окружения RoFormer → ошибка с понятным текстом (RoFormer/
                 разделение + как пройти: переустановить), дорожек Demucs нет;
  ТК16 (усл. 1)  POST /jobs/{id}/stems при сбое sep_run → ошибка, не 200 с другой моделью;
  ТК17 (усл. 3)  /version stems_model == "bs-roformer-sw" (с окружением и без);
  ТК18 (усл. 3)  рецепт: stems_model == "bs-roformer-sw";
  ТК19 (усл. 5)  `python sep_run.py download <каталог>` качает BS-Roformer-SW.ckpt и
                 MDX23C-DrumSep-aufr33-jarredou.ckpt в этот каталог (Separator(model_file_dir=…)), код 0;
                 сбой загрузки → ненулевой код;
  ТК20 (усл. 9)  requirements.txt без demucs, torchaudio есть;
  ТК21 (усл. 5)  install.sh: чистая установка создаёт окружение разделения ~/sep-venv (uv venv; uv pip
                 install --python ~/sep-venv/bin/python audio-separator[gpu]==0.47.0 audioread) и качает веса
                 `<sep python> sep_run.py download ~/sep-models`;
  ТК22 (усл. 5, 11) PARTSCASTER_SEP_PY / PARTSCASTER_SEP_MODELS из worker.env — каталог этого python и
                 этот каталог весов;
  ТК23 (усл. 5)  сбой uv pip разделения / загрузки весов → ненулевой код, current не появился (первая
                 установка) или прежний (обновление);
  ТК24 (усл. 5)  повторная установка с тем же набором пакетов не зовёт uv venv/uv pip для разделения;
  ТК25 (усл. 5, 11) окружение разделения уже есть, отпечатка нет (поставлено до архивов, GPU-сервер) —
                 не переустанавливается, в т.ч. в каталоге из worker.env;
  ТК26 (усл. 1, 5) в install.sh нет «demucs» и PARTSCASTER_STEMS_MODEL.

Внешние границы подменяются: подпроцесс RoFormer (stems.run_proc — запуск `python sep_run.py …`) — фейком,
который пишет известные сигналы и печатает JSON по контракту sep_run.py; модуль audio_separator — фейком на
PYTHONPATH; uv/nvidia-smi/systemctl — заглушками песочницы test_install_sh. Код воркера не мокается
(кроме замера дорожек analyze_file — librosa, не предмет теста, как в test_roformer_stems).
Без numpy/soundfile/librosa классы разделения пропускаются; API-классы — ещё и без fastapi/httpx.
"""
import inspect
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import test_install_sh as base
import test_pure as tp

try:
    import soundfile  # noqa: F401
    _HAS_SF = True
except ImportError:
    _HAS_SF = False

try:
    import librosa  # noqa: F401
    _HAS_LIBROSA = True
except ImportError:
    _HAS_LIBROSA = False

WORKER_DIR = Path(__file__).resolve().parent
_SEP_OK = tp._HAS_NUMPY and _HAS_SF and _HAS_LIBROSA
_SEP_SKIP = "нужны numpy/soundfile/librosa"
_WORKER_OK = tp._HAS_WORKER_DEPS and _HAS_SF and _HAS_LIBROSA
_WORKER_SKIP = "нужны fastapi/httpx/numpy/soundfile/librosa (окружение воркера)"

RF_MODEL = "bs-roformer-sw"
SR = 44100
DUR = 2.0
MAIN = ("drums", "bass", "other", "vocals")
DETAIL = ("guitar", "piano")
PARTS = ("kick", "snare", "toms", "hh", "ride", "crash")
HZ = {"drums": 110, "bass": 220, "other": 330, "vocals": 440, "guitar": 550, "piano": 660}
PART_HZ = {"kick": 770, "snare": 880, "toms": 990, "hh": 1100, "ride": 1210, "crash": 1320}
AMP = 0.1
PART_AMP = 0.05

# понятный текст ошибки разделения: про что (RoFormer / разделение)
_ERR_WHAT = re.compile(r"roformer|разделени", re.I)


def _tone(hz, amp=AMP, dur=DUR, sr=SR):
    import numpy as np
    t = np.arange(int(dur * sr)) / sr
    x = (amp * np.sin(2 * np.pi * hz * t)).astype(np.float32)
    return np.stack([x, x], axis=1)


def _tone_amp(x, hz, sr=SR):
    """Амплитуда синуса частоты hz (одна точка ДПФ по целым секундам)."""
    import numpy as np
    if x.ndim > 1:
        x = x.mean(axis=1)
    n = (len(x) // sr) * sr
    x = x[:n].astype(np.float64)
    t = np.arange(n) / sr
    return 2 * abs(np.sum(x * np.exp(-2j * np.pi * hz * t))) / n


def _mix():
    """Трек = сумма шести источников RoFormer."""
    import numpy as np
    return np.sum([_tone(h) for h in HZ.values()], axis=0).astype(np.float32)


def _read(path):
    import soundfile as sf
    x, _ = sf.read(str(path), dtype="float32", always_2d=True)
    return x


class FakeSep:
    """Фейк запуска `python sep_run.py <аудио> <выход> <веса>`: пишет сырые дорожки <имя>.wav и
    drums-<часть>.wav, последней строкой печатает JSON по контракту sep_run.py. Чужие вызовы — в настоящий."""

    _real = staticmethod(subprocess.run)

    def __init__(self, mode="ok"):
        # ok | fail (код 1) | drum_fail (DrumSep упал) | raise (таймаут)
        self.mode = mode
        self.calls = []

    def __call__(self, cmd, *args, **kw):
        argv = [str(a) for a in (cmd if isinstance(cmd, (list, tuple)) else [cmd])]
        idx = next((i for i, a in enumerate(argv) if Path(a).name == "sep_run.py"), None)
        if idx is None:
            return self._real(cmd, *args, **kw)
        self.calls.append(argv)
        src, out = Path(argv[idx + 1]), Path(argv[idx + 2])
        if self.mode == "raise":
            raise subprocess.TimeoutExpired(argv, 1)
        if self.mode == "fail":
            err = "Traceback ...\nRuntimeError: CUDA out of memory"
            return subprocess.CompletedProcess(argv, 1, stdout="", stderr=err)
        import soundfile as sf
        try:
            sf.read(str(src))
        except Exception as e:  # noqa: BLE001 — как настоящий sep_run на битом файле
            return subprocess.CompletedProcess(argv, 1, stdout="", stderr=f"{type(e).__name__}: {e}")
        out.mkdir(parents=True, exist_ok=True)
        stems = {}
        for name, hz in HZ.items():
            p = out / f"{name}.wav"
            sf.write(str(p), _tone(hz), SR)
            stems[name] = str(p)
        parts, err = {}, ""
        if self.mode == "drum_fail":
            err = "RuntimeError: DrumSep weights download failed"
        else:
            for name, hz in PART_HZ.items():
                p = out / f"drums-{name}.wav"
                sf.write(str(p), _tone(hz, amp=PART_AMP), SR)
                parts[name] = str(p)
        stdout = "loading model...\n" + json.dumps({"stems": stems, "drum_parts": parts, "drum_error": err}) + "\n"
        return subprocess.CompletedProcess(argv, 0, stdout=stdout, stderr="")


class _SepEnv:
    """Окружение разделения на тест: файл PARTSCASTER_SEP_PY (есть — «установлено»), каталог весов,
    фейк подпроцесса. PARTSCASTER_STEMS_MODEL — только если задан (старый откат в worker.env)."""

    def _setup_sep(self, tmp: Path, sep_mode="ok", sep_py_exists=True, old_rollback=None):
        import stems
        sep_py = tmp / "sep-python"
        if sep_py_exists:
            sep_py.write_text("")
        elif sep_py.exists():
            sep_py.unlink()
        p = mock.patch.dict(os.environ, {"PARTSCASTER_SEP_PY": str(sep_py),
                                         "PARTSCASTER_SEP_MODELS": str(tmp / "sep-models")})
        p.start()
        self.addCleanup(p.stop)
        os.environ.pop("PARTSCASTER_STEMS_MODEL", None)
        if old_rollback is not None:
            os.environ["PARTSCASTER_STEMS_MODEL"] = old_rollback
        self.sep = FakeSep(sep_mode)
        p = mock.patch.object(stems, "run_proc", self.sep)
        p.start()
        self.addCleanup(p.stop)


# --- ТК1, ТК2: Demucs нет в коде ---

class TestNoDemucsCode(unittest.TestCase):

    def test_separate_has_no_prefer(self):
        import stems
        params = inspect.signature(stems.separate).parameters
        self.assertNotIn("prefer", params, "выбора модели нет: stems.separate без prefer")
        self.assertEqual(list(params)[:2], ["audio_path", "out_dir"], list(params))

    def test_stems_has_no_demucs_attrs(self):
        import stems
        for name in ("MODEL_DEMUCS", "_run_model", "roformer_enabled"):
            with self.subTest(attr=name):
                self.assertFalse(hasattr(stems, name), f"stems.{name} остался")

    def test_worker_sources_without_demucs(self):
        imp = re.compile(r"^\s*(import\s+demucs|from\s+demucs)\b", re.M)
        lit = re.compile(r"""["']htdemucs(_6s)?["']""")
        for f in sorted(WORKER_DIR.glob("*.py")):
            if f.name.startswith("test_"):
                continue
            text = f.read_text(encoding="utf-8")
            with self.subTest(file=f.name):
                self.assertIsNone(imp.search(text), "импорт demucs")
                self.assertIsNone(lit.search(text), "строка 'htdemucs' в коде")
                self.assertNotIn("PARTSCASTER_STEMS_MODEL", text, "откат на Demucs")


# --- ТК3–ТК9: stems.separate ---

@unittest.skipUnless(_SEP_OK, _SEP_SKIP)
class TestSeparateOnlyRoformer(_SepEnv, unittest.TestCase):

    def setUp(self):
        import soundfile as sf
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.tmp = Path(self._td.name)
        self.audio = self.tmp / "audio.flac"
        sf.write(str(self.audio), _mix(), SR)
        self.out = self.tmp / "job"
        self.out.mkdir()

    def _separate(self, **kw):
        import stems
        self._setup_sep(self.tmp, **kw)
        return stems.separate(self.audio, self.out)

    def _stem_files(self):
        return sorted(p.name for p in self.out.iterdir() if p.name.startswith("stem-"))

    def test_roformer_default_files_and_model(self):
        res = self._separate()
        self.assertEqual(len(self.sep.calls), 1, "RoFormer не вызван")
        self.assertEqual(res.get("model"), RF_MODEL)
        self.assertIn("seconds", res)
        for n in MAIN + DETAIL + PARTS:
            self.assertTrue((self.out / f"stem-{n}.flac").is_file(), f"нет stem-{n}.flac: {self._stem_files()}")
        names = {Path(s).name for s in res.get("stems", [])}
        self.assertTrue(names >= {f"stem-{n}.flac" for n in MAIN}, res.get("stems"))

    def test_main_sum_is_track_detail_inside_other_parts_outside(self):
        import numpy as np
        self._separate()
        ref = _read(self.audio)
        full = np.sum([_read(self.out / f"stem-{n}.flac") for n in MAIN], axis=0)
        n = min(len(full), len(ref))
        resid = float(np.sqrt(np.mean((full[:n] - ref[:n]) ** 2)))
        level = float(np.sqrt(np.mean(ref[:n] ** 2)))
        self.assertLess(20 * np.log10(resid / level + 1e-12), -40.0, "сумма основных дорожек ≠ трек")
        other = _read(self.out / "stem-other.flac")
        for d in DETAIL:
            self.assertAlmostEqual(_tone_amp(other, HZ[d]), AMP, delta=0.005, msg=f"{d} не внутри «прочего»")
        for p in PARTS:
            self.assertLess(_tone_amp(full, PART_HZ[p]), 0.002, f"часть {p} в сумме основных")

    def test_subprocess_failure_raises_clear_error(self):
        with self.assertRaises(RuntimeError) as cm:
            self._separate(sep_mode="fail")
        self.assertRegex(str(cm.exception), _ERR_WHAT)
        self.assertEqual(len(self.sep.calls), 1)
        self.assertEqual(self._stem_files(), [], "после сбоя RoFormer дорожки другой моделью")

    def test_no_sep_env_raises_without_subprocess(self):
        with self.assertRaises(RuntimeError) as cm:
            self._separate(sep_py_exists=False)
        self.assertRegex(str(cm.exception), _ERR_WHAT)
        self.assertEqual(self.sep.calls, [], "RoFormer запущен без окружения")
        self.assertEqual(self._stem_files(), [])

    def test_timeout_raises_without_stems(self):
        with self.assertRaises(Exception):  # noqa: B017 — тип не задан условием, важно «упало», а не подменилось
            self._separate(sep_mode="raise")
        self.assertEqual(self._stem_files(), [])

    def test_old_rollback_env_ignored(self):
        res = self._separate(old_rollback="demucs")
        self.assertEqual(len(self.sep.calls), 1, "PARTSCASTER_STEMS_MODEL=demucs не должен ничего менять")
        self.assertEqual(res.get("model"), RF_MODEL)

    def test_drumsep_failure_keeps_main(self):
        res = self._separate(sep_mode="drum_fail")
        self.assertEqual(res.get("model"), RF_MODEL)
        for n in MAIN + DETAIL:
            self.assertTrue((self.out / f"stem-{n}.flac").is_file(), f"нет stem-{n}.flac")
        for n in PARTS:
            self.assertFalse((self.out / f"stem-{n}.flac").exists(), f"stem-{n}.flac при упавшем DrumSep")

    def test_broken_audio_raises_without_partial_stems(self):
        for content in (b"", b"not an audio file at all" * 100):
            with self.subTest(size=len(content)):
                self.audio.write_bytes(content)
                with self.assertRaises(Exception):  # noqa: B017
                    self._separate()
                self.assertEqual(self._stem_files(), [])


# --- ТК10: python окружения разделения по умолчанию ---

class TestSepPythonDefault(unittest.TestCase):

    def _call(self, home, windows):
        """_sep_python() с HOME=home; windows — подмена признака Windows. Подмена os.name на «nt» ломает
        pathlib на Linux, поэтому сначала пробуется sys.platform, затем (если ответ не Windows) os.name."""
        import stems
        env = {"HOME": str(home), "USERPROFILE": str(home)}
        with mock.patch.dict(os.environ, env):
            os.environ.pop("PARTSCASTER_SEP_PY", None)
            if not windows:
                return str(stems._sep_python())
            with mock.patch.object(sys, "platform", "win32"):
                got = str(stems._sep_python())
            if "Scripts" in got:
                return got
            try:
                with mock.patch.object(os, "name", "nt"):
                    return str(stems._sep_python())
            except Exception as e:  # noqa: BLE001 — pathlib не строит WindowsPath на Linux
                self.fail(f"Windows (sys.platform=win32) не распознан: {got}; с os.name=nt — {e!r}")

    def _norm(self, s):
        return s.replace("\\", "/")

    def test_windows_scripts_python_exe(self):
        with tempfile.TemporaryDirectory() as td:
            got = self._norm(self._call(td, True))
            self.assertEqual(got, self._norm(os.path.join(td, "sep-venv", "Scripts", "python.exe")))

    def test_posix_bin_python(self):
        if os.name == "nt":
            self.skipTest("проверка POSIX")
        with tempfile.TemporaryDirectory() as td:
            self.assertEqual(self._call(td, False), os.path.join(td, "sep-venv", "bin", "python"))

    def test_env_wins(self):
        import stems
        with mock.patch.dict(os.environ, {"PARTSCASTER_SEP_PY": "/opt/sep/venv/bin/python"}):
            self.assertEqual(str(stems._sep_python()), "/opt/sep/venv/bin/python")


# --- ТК11–ТК18: API воркера ---

@unittest.skipUnless(_WORKER_OK, _WORKER_SKIP)
class TestStemsApiNoChoice(_SepEnv, tp._WorkerApiCase):

    def setUp(self):
        super().setUp()
        data = Path(self._td.name) / "cfg"
        data.mkdir()
        self.settings = data / "settings.json"
        for name, val in (("DATA_DIR", data), ("SETTINGS_PATH", self.settings)):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)
        p = mock.patch.object(self.w, "analyze_file", return_value={})
        p.start()
        self.addCleanup(p.stop)

    def _audio_job(self):
        import soundfile as sf
        jid = self._job(duration=DUR, semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        sf.write(str(d / "audio.flac"), _mix(), SR)
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid, d

    def _config(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _stem_files(self, d):
        return sorted(p.name for p in d.iterdir() if p.name.startswith("stem-"))

    def _assert_error(self, r, d, how_to=False):
        """Ошибка разделения: статус ошибки (или status=error в ответе), понятный текст, дорожек нет."""
        body = r.text
        failed = r.status_code >= 400 or (r.headers.get("content-type", "").startswith("application/json")
                                          and isinstance(r.json(), dict) and r.json().get("status") == "error")
        self.assertTrue(failed, f"{r.status_code}: {body}")
        self.assertRegex(body, _ERR_WHAT)
        if how_to:
            self.assertRegex(body.lower(), r"переустанов|reinstall|install", "нет подсказки, как пройти")
        self.assertEqual(self._stem_files(d), [], "дорожки другой моделью")

    def test_worker_has_no_choice_symbols(self):
        for name in ("STEMS_CHOICES", "stems_pref"):
            with self.subTest(name=name):
                self.assertFalse(hasattr(self.w, name), f"yue_worker.{name} остался")

    def test_config_without_choice_with_availability(self):
        for installed in (True, False):
            with self.subTest(installed=installed):
                self._setup_sep(Path(self._td.name), sep_py_exists=installed)
                cfg = self._config()
                self.assertNotIn("stems_pref", cfg)
                self.assertNotIn("stems_model", cfg)
                self.assertIs(cfg.get("roformer_available"), installed, cfg)

    def test_post_config_stems_model_ignored(self):
        self._setup_sep(Path(self._td.name))
        for val in ("htdemucs", "roformer"):
            with self.subTest(val=val):
                r = self.client.post("/config", json={"stems_model": val})
                self.assertLess(r.status_code, 300, r.text)
                if self.settings.exists():
                    saved = json.loads(self.settings.read_text(encoding="utf-8"))
                    self.assertNotIn("stems_model", saved, saved)
                self.assertNotIn("stems_model", self._config())

    def test_post_config_with_old_field_keeps_other_settings(self):
        self._setup_sep(Path(self._td.name))
        r = self.client.post("/config", json={"stems_model": "htdemucs", "stems_auto": "manual"})
        self.assertLess(r.status_code, 300, r.text)
        self.assertEqual(self._config().get("stems_auto"), "manual", "поле старого клиента сломало сохранение")

    def test_old_setting_in_file_harmless(self):
        self._setup_sep(Path(self._td.name))
        self.settings.write_text(json.dumps({"stems_model": "htdemucs"}), encoding="utf-8")
        self._config()
        jid, _ = self._audio_job()
        r = self.client.post(f"/jobs/{jid}/stems")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json().get("model"), RF_MODEL)
        self.assertEqual(len(self.sep.calls), 1)

    def test_stems_old_model_param_ignored(self):
        self._setup_sep(Path(self._td.name))
        for model in ("htdemucs", "roformer", "xyz"):
            with self.subTest(model=model):
                jid, _ = self._audio_job()
                r = self.client.post(f"/jobs/{jid}/stems", params={"model": model})
                self.assertNotEqual(r.status_code, 422, r.text)
                self.assertEqual(r.status_code, 200, r.text)
                self.assertEqual(r.json().get("model"), RF_MODEL)

    def test_stems_without_roformer_env_clear_error(self):
        self._setup_sep(Path(self._td.name), sep_py_exists=False)
        jid, d = self._audio_job()
        r = self.client.post(f"/jobs/{jid}/stems")
        self._assert_error(r, d, how_to=True)
        self.assertEqual(self.sep.calls, [])

    def test_stems_subprocess_failure_error(self):
        self._setup_sep(Path(self._td.name), sep_mode="fail")
        jid, d = self._audio_job()
        r = self.client.post(f"/jobs/{jid}/stems")
        self._assert_error(r, d)

    def test_version_stems_model_constant(self):
        for installed in (True, False):
            with self.subTest(installed=installed):
                self._setup_sep(Path(self._td.name), sep_py_exists=installed)
                r = self.client.get("/version")
                self.assertEqual(r.status_code, 200, r.text)
                self.assertEqual(r.json().get("stems_model"), RF_MODEL)


@unittest.skipUnless(tp._HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRecipeStemsModel(tp._WorkerApiCase):

    def test_recipe_stems_model_roformer(self):
        from test_recipe_api import _RecipeCase
        root = _RecipeCase._mk(self)
        r = self.client.get(f"/jobs/{root}/recipe")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json().get("stems_model"), RF_MODEL)


# --- ТК19: sep_run.py download ---

FAKE_AUDIO_SEPARATOR = r'''
import json, os


class Separator:
    def __init__(self, *args, **kw):
        import shutil
        self.kw = {k: str(v) for k, v in kw.items()}
        # как audio-separator: без программы ffmpeg Separator не создаётся
        _log({"init": self.kw, "ffmpeg": shutil.which("ffmpeg")})
        if not shutil.which("ffmpeg"):
            raise FileNotFoundError("ffmpeg")

    def download_model_files(self, *args, **kw):
        name = str(args[0] if args else kw.get("model_filename"))
        d = self.kw.get("model_file_dir")
        _log({"download": name, "dir": d})
        # как библиотека: веса и конфиг модели пишутся сразу в каталог весов; сбой — оборванный файл весов остаётся
        conf = {"BS-Roformer-SW.ckpt": "BS-Roformer-SW.yaml",
                "MDX23C-DrumSep-aufr33-jarredou.ckpt": "config_drumsep_mdx23c.yaml"}.get(name)
        if conf:
            with open(os.path.join(d, conf), "w") as f:
                f.write("cfg")
        with open(os.path.join(d, name), "wb") as f:
            f.write(b"part" if os.environ.get("FAKE_SEP_FAIL") else b"full")
        if os.environ.get("FAKE_SEP_FAIL"):
            raise RuntimeError("network down")
        return None

    def load_model(self, *args, **kw):
        raise AssertionError("download не должен грузить модель (без видеокарты)")

    def separate(self, *args, **kw):
        raise AssertionError("download не должен делить")


def _log(rec):
    with open(os.environ["FAKE_SEP_LOG"], "a", encoding="utf-8") as f:
        f.write(json.dumps(rec) + "\n")
'''


class TestSepRunDownload(unittest.TestCase):

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.tmp = Path(self._td.name)
        pkg = self.tmp / "fake" / "audio_separator"
        pkg.mkdir(parents=True)
        (pkg / "separator.py").write_text(FAKE_AUDIO_SEPARATOR)
        (pkg / "__init__.py").write_text("from .separator import Separator  # noqa: F401\n")
        self.log = self.tmp / "sep.log"
        self.models = self.tmp / "sep-models"

    def _run(self, **env):
        # ffmpeg в PATH — заглушка (как системный); тест без ffmpeg задаёт PATH сам
        ff = self.tmp / "ffbin"
        ff.mkdir(exist_ok=True)
        (ff / "ffmpeg").write_text("#!/bin/sh\necho ffmpeg version fake\n")
        (ff / "ffmpeg").chmod(0o755)
        env.setdefault("PATH", f"{ff}{os.pathsep}{os.environ.get('PATH', '')}")
        e = dict(os.environ, PYTHONPATH=str(self.tmp / "fake"), FAKE_SEP_LOG=str(self.log), **env)
        return subprocess.run([sys.executable, str(WORKER_DIR / "sep_run.py"), "download", str(self.models)],
                              cwd=self.tmp, env=e, capture_output=True, text=True, timeout=120)

    def _records(self):
        if not self.log.exists():
            return []
        return [json.loads(ln) for ln in self.log.read_text().splitlines() if ln.strip()]

    MODELS = ("BS-Roformer-SW.ckpt", "MDX23C-DrumSep-aufr33-jarredou.ckpt")

    def test_downloads_both_models_into_dir(self):
        """Обе модели скачаны и лежат в каталоге весов целиком (через временный каталог рядом)."""
        p = self._run()
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        recs = self._records()
        got = {r["download"] for r in recs if "download" in r}
        self.assertEqual(got, set(self.MODELS), recs)
        for name in self.MODELS:
            self.assertEqual((self.models / name).read_bytes(), b"full")
        self.assertEqual([x.name for x in self.models.iterdir() if x.name.endswith(".partial")], [])

    def test_second_run_no_download(self):
        """Скачанное целиком не качается заново."""
        self.assertEqual(self._run().returncode, 0)
        self.log.unlink()
        p = self._run()
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertEqual([r for r in self._records() if "download" in r], [])

    def test_download_failure_nonzero(self):
        p = self._run(FAKE_SEP_FAIL="1")
        self.assertNotEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")

    def test_interrupted_download_not_taken_as_done(self):
        """Обрыв загрузки: оборванный файл не попадает в каталог весов, повтор качает заново (кросс-ревью)."""
        self.assertNotEqual(self._run(FAKE_SEP_FAIL="1").returncode, 0)
        self.assertFalse((self.models / self.MODELS[0]).exists(), "оборванные веса легли на место")
        self.log.unlink()
        p = self._run()
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertIn(self.MODELS[0], {r["download"] for r in self._records() if "download" in r})
        self.assertEqual((self.models / self.MODELS[0]).read_bytes(), b"full")

    def test_complete_old_weights_not_redownloaded(self):
        """Веса, скачанные до отметок (сервер): размер как у полной модели и конфиг рядом — не качаются заново;
        файл меньше (оборван) — качается."""
        import sep_run
        self.models.mkdir()
        (main, (size, conf)), (drum, (_dsize, dconf)) = sep_run.KNOWN_WEIGHTS.items()
        with open(self.models / main, "wb") as f:
            f.truncate(size)
        (self.models / conf).write_text("cfg")
        with open(self.models / drum, "wb") as f:
            f.truncate(1000)                  # оборванный
        (self.models / dconf).write_text("cfg")
        p = self._run()
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        self.assertEqual({r["download"] for r in self._records() if "download" in r}, {drum})

    def test_ready_only_when_both_complete(self):
        """`sep_run.py ready` (проверка программы Windows при старте): 0 — обе модели на месте, иначе 1; без сети."""
        def ready():
            return subprocess.run([sys.executable, str(WORKER_DIR / "sep_run.py"), "ready", str(self.models)],
                                  capture_output=True, text=True, timeout=60).returncode
        self.models.mkdir()
        self.assertEqual(ready(), 1)
        self.assertEqual(self._run().returncode, 0)   # скачали обе
        self.assertEqual(ready(), 0)
        (self.models / f".{self.MODELS[1]}.ok").unlink()
        (self.models / self.MODELS[1]).write_bytes(b"part")    # оборванная вторая
        self.assertEqual(ready(), 1)
        self.assertEqual(self._run().returncode, 0)   # докачали
        self.assertEqual(ready(), 0)
        (self.models / self.MODELS[0]).unlink()                # файл удалили, отметка .ok осталась
        self.assertEqual(ready(), 1, "одна отметка без файла весов — не «на месте»")
        self.log.unlink()
        self.assertEqual(self._run().returncode, 0)   # установка докачивает удалённое, несмотря на отметку
        self.assertIn(self.MODELS[0], {r["download"] for r in self._records() if "download" in r})
        self.assertEqual(ready(), 0)

    def test_no_system_ffmpeg_uses_bundled(self):
        """Нет системного ffmpeg — берётся из пакета imageio-ffmpeg (кросс-ревью: без ffmpeg установка падала)."""
        bundled = self.tmp / "fake" / "imageio_ffmpeg"
        bundled.mkdir()
        exe = self.tmp / "ffmpeg-bundled-v7"
        exe.write_text("#!/bin/sh\necho ffmpeg version bundled\n")
        exe.chmod(0o755)
        (bundled / "__init__.py").write_text(f"def get_ffmpeg_exe():\n    return {str(exe)!r}\n")
        # PATH без ffmpeg; python запускается по полному пути
        p = self._run(PATH="/nonexistent")
        self.assertEqual(p.returncode, 0, f"{p.stdout}\n{p.stderr}")
        seen = [r["ffmpeg"] for r in self._records() if "init" in r]
        self.assertTrue(seen and all(x and x.endswith("ffmpeg") for x in seen), seen)


# --- ТК20: requirements.txt ---

class TestRequirements(unittest.TestCase):

    def _pkgs(self):
        out = []
        for ln in (WORKER_DIR / "requirements.txt").read_text(encoding="utf-8").splitlines():
            ln = ln.split("#", 1)[0].strip()
            if ln:
                out.append(re.split(r"[\s<>=!~\[;]", ln, maxsplit=1)[0].lower())
        return out

    def test_no_demucs_torchaudio_kept(self):
        pkgs = self._pkgs()
        self.assertNotIn("demucs", pkgs)
        self.assertIn("torchaudio", pkgs)


# --- ТК21–ТК26: install.sh ---

# uv: `uv venv <каталог> [--python X]` создаёт <каталог>/bin/python (копия venv-python заглушек);
# `uv pip` с audio-separator падает при fail-sep-pip; любой `uv pip` — при fail-uv-pip
UV_STUB = r"""#!/bin/sh
{ echo "== uv"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
if [ "$1" = venv ]; then
  shift; dir=""; skip=0
  for a in "$@"; do
    if [ "$skip" = 1 ]; then skip=0; continue; fi
    case "$a" in --python|-p) skip=1;; -*) ;; *) [ -z "$dir" ] && dir="$a";; esac
  done
  mkdir -p "$dir/bin"
  cp "$STUB_DIR/venv-python" "$dir/bin/python"
  chmod +x "$dir/bin/python"
  exit 0
fi
if [ "$1" = pip ]; then
  case " $* " in *audio-separator*) [ -f "$STUB_DIR/fail-sep-pip" ] && exit 1;; esac
  [ -f "$STUB_DIR/fail-uv-pip" ] && exit 1
fi
exit 0
"""

# python venv-заглушки: как в test_install_sh, плюс журнал «какой python, какие аргументы» и
# `sep_run.py download` (fail-sep-download — сбой)
SEP_VENV_PY = r"""#!/bin/sh
{ echo "== venv-python"; for a in "$@"; do printf '%s\n' "$a"; done; } >> "$STUB_LOG"
printf '%s\t%s\n' "$0" "$*" >> "$STUB_DIR/py-argv.log"
case "$1" in *sep_run.py)
  [ "$2" = download ] && [ -f "$STUB_DIR/fail-sep-download" ] && exit 1
  exit 0;;
esac
case "$1" in *kits_install.py) [ -f "$STUB_DIR/fail-kits" ] && exit 1;; esac
""" + base.ENGINE_CPP_SH + "exit 0\n"

SEP_PKGS = ("audio-separator[gpu]==0.47.0", "audioread", "imageio-ffmpeg==0.6.0")  # ffmpeg — машинам без системного


class _SepInstallCase(base._InstallCase):
    def setUp(self):
        super().setUp()
        (self.stubs / "uv").write_text(UV_STUB)
        (self.stubs / "venv-python").write_text(SEP_VENV_PY)
        self.argv_log = self.stubs / "py-argv.log"

    def run_sh(self, script, *args, **env):
        if self.argv_log.exists():
            self.argv_log.unlink()  # журнал python — только последнего запуска
        return super().run_sh(script, *args, **env)

    def write_env(self, **kv):
        self.yue_home.mkdir(exist_ok=True)
        lines = [f"HF_HOME={self.home / 'yue' / 'hf-cache'}"] + [f"{k}={v}" for k, v in kv.items()]
        (self.yue_home / "worker.env").write_text("\n".join(lines) + "\n")

    def existing_python(self, py: Path):
        py.parent.mkdir(parents=True, exist_ok=True)
        py.write_text(SEP_VENV_PY)
        py.chmod(0o755)

    def sep_pips(self):
        return [a for a in self.called("uv", "pip", "install") if any("audio-separator" in x for x in a)]

    def venvs_for(self, d: Path):
        return [a for a in self.called("uv", "venv") if str(d) in a]

    def sep_downloads(self):
        """[(python, [аргументы])] вызовов `sep_run.py download` последнего запуска."""
        out = []
        if not self.argv_log.exists():
            return out
        for ln in self.argv_log.read_text().splitlines():
            py, _, args = ln.partition("\t")
            a = args.split(" ")
            if a and Path(a[0]).name == "sep_run.py" and a[1:2] == ["download"]:
                out.append((py, a))
        return out

    def assert_sep_install(self, venv: Path, models: Path):
        venvs = self.venvs_for(venv)
        self.assertEqual(len(venvs), 1, self.calls())
        pips = self.sep_pips()
        self.assertEqual(len(pips), 1, self.calls())
        pip = pips[0]
        self.assertIn("--python", pip)
        self.assertEqual(pip[pip.index("--python") + 1], str(venv / "bin" / "python"))
        for pkg in SEP_PKGS:
            self.assertIn(pkg, pip)
        dls = self.sep_downloads()
        self.assertEqual(len(dls), 1, self.argv_log.read_text() if self.argv_log.exists() else "нет вызовов python")
        py, args = dls[0]
        self.assertEqual(py, str(venv / "bin" / "python"))
        self.assertEqual(args[2:3], [str(models)])


    def assert_used_for_weights(self, py: Path):
        """Веса качаются при установке именно этим python окружения разделения."""
        dls = self.sep_downloads()
        self.assertTrue(dls, "веса разделения не качались при установке")
        for used, _ in dls:
            self.assertEqual(used, str(py))


class TestInstallSep(_SepInstallCase):

    def test_fresh_default_dirs(self):
        self.install("1.0.0", "--no-start")
        self.assert_sep_install(self.home / "sep-venv", self.home / "sep-models")

    def test_dirs_from_worker_env(self):
        venv = Path(self._td.name) / "data" / "sep" / "venv"
        models = Path(self._td.name) / "data" / "sep" / "models"
        self.write_env(PARTSCASTER_SEP_PY=venv / "bin" / "python", PARTSCASTER_SEP_MODELS=models)
        self.install("1.0.0", "--no-start")
        self.assert_sep_install(venv, models)
        self.assertFalse((self.home / "sep-venv").exists(), "окружение поставлено мимо PARTSCASTER_SEP_PY")

    def test_sep_pip_failure_fresh_no_current(self):
        (self.stubs / "fail-sep-pip").touch()
        p = self.install("1.0.0", "--no-start", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertFalse(self.current().exists() or self.current().is_symlink(), "current включён при сбое")

    def test_weights_failure_fresh_no_current(self):
        (self.stubs / "fail-sep-download").touch()
        p = self.install("1.0.0", "--no-start", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertFalse(self.current().exists() or self.current().is_symlink(), "current включён при сбое")

    def test_sep_failure_on_update_keeps_current(self):
        self.install("1.0.0", "--no-start")
        before = os.readlink(self.current())
        shutil.rmtree(self.home / "sep-venv", ignore_errors=True)  # окружения нет — обновление обязано его поставить
        (self.stubs / "fail-sep-pip").touch()
        p = self.install("2.0.0", "--no-start", ok=False)
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertEqual(os.readlink(self.current()), before)

    def test_repeat_same_packages_no_reinstall(self):
        self.install("1.0.0", "--no-start")
        self.assert_sep_install(self.home / "sep-venv", self.home / "sep-models")  # первый раз — ставит
        self.install("2.0.0", "--no-start")
        self.assertEqual(self.sep_pips(), [], self.calls())
        self.assertEqual(self.venvs_for(self.home / "sep-venv"), [], self.calls())

    def test_existing_env_without_fingerprint_default(self):
        py = self.home / "sep-venv" / "bin" / "python"
        self.existing_python(py)
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.sep_pips(), [], self.calls())
        self.assertEqual(self.venvs_for(self.home / "sep-venv"), [], self.calls())
        self.assert_used_for_weights(py)

    def test_env_last_line_without_newline_read(self):
        """worker.env, где PARTSCASTER_SEP_PY — последняя строка без перевода строки (кросс-ревью): путь всё равно
        читается, второе окружение в ~/sep-venv не ставится."""
        py = Path(self._td.name) / "lab" / "sep-venv" / "bin" / "python"
        self.existing_python(py)
        self.yue_home.mkdir(exist_ok=True)
        (self.yue_home / "worker.env").write_text(f"HF_HOME={self.home / 'yue' / 'hf-cache'}\nPARTSCASTER_SEP_PY={py}")
        self.install("1.0.0", "--no-start")
        self.assertFalse((self.home / "sep-venv").exists(), "последняя строка worker.env потеряна")
        self.assert_used_for_weights(py)

    def test_existing_env_from_worker_env_used(self):
        """GPU-сервер: окружение уже стоит в своём каталоге (worker.env) — его не трогают и им пользуются."""
        py = Path(self._td.name) / "lab" / "sep-venv" / "bin" / "python"
        self.existing_python(py)
        self.write_env(PARTSCASTER_SEP_PY=py, PARTSCASTER_SEP_MODELS=Path(self._td.name) / "lab" / "sep-models")
        self.install("1.0.0", "--no-start")
        self.assertEqual(self.sep_pips(), [], self.calls())
        self.assertEqual([a for a in self.called("uv", "venv") if str(self.home / "yue" / ".venv") not in a], [],
                         self.calls())
        self.assertFalse((self.home / "sep-venv").exists(), "поставлено второе окружение в ~/sep-venv")
        self.assert_used_for_weights(py)


class TestInstallShText(unittest.TestCase):

    def test_no_demucs_no_rollback_var(self):
        text = (WORKER_DIR / "install.sh").read_text(encoding="utf-8")
        self.assertNotIn("demucs", text.lower())
        self.assertNotIn("PARTSCASTER_STEMS_MODEL", text)


class TestInstallSepDirFlag(_SepInstallCase):
    """усл. 7: install.sh --sep-dir DIR — окружение DIR/venv, веса DIR/models; пути пишутся в worker.env, если их
    там ещё нет (заданное раньше не перетирается), перед дописыванием — перевод строки в конце файла."""

    def _env(self):
        return (self.yue_home / "worker.env").read_text()

    def test_fresh_paths_written_and_used(self):
        d = Path(self._td.name) / "big" / "sep"
        self.install("1.0.0", "--no-start", "--sep-dir", str(d))
        env = self._env()
        self.assertIn(f"PARTSCASTER_SEP_PY={d / 'venv' / 'bin' / 'python'}\n", env)
        self.assertIn(f"PARTSCASTER_SEP_MODELS={d / 'models'}\n", env)
        self.assertIn("HF_HOME=", env, "worker.env без HF_HOME")
        self.assert_sep_install(d / "venv", d / "models")

    def test_existing_paths_kept_and_newline_added(self):
        """Ключи уже есть — не перетираются; файл без перевода строки в конце — ключ не склеивается."""
        own = Path(self._td.name) / "lab" / "sep-venv" / "bin" / "python"
        self.existing_python(own)
        self.yue_home.mkdir(exist_ok=True)
        (self.yue_home / "worker.env").write_text(f"HF_HOME=/data/hf\nPARTSCASTER_SEP_PY={own}")  # без \n в конце
        d = Path(self._td.name) / "other"
        self.install("1.0.0", "--no-start", "--sep-dir", str(d))
        lines = self._env().splitlines()
        self.assertIn(f"PARTSCASTER_SEP_PY={own}", lines)
        self.assertEqual([ln for ln in lines if ln.startswith("PARTSCASTER_SEP_PY=")], [f"PARTSCASTER_SEP_PY={own}"])
        self.assertIn(f"PARTSCASTER_SEP_MODELS={d / 'models'}", lines)
        self.assertEqual(self.sep_pips(), [], self.calls())   # своё окружение без отпечатка — не переустанавливается


if __name__ == "__main__":
    unittest.main()
