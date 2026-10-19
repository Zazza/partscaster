"""Разделение на дорожки — только BS-Roformer-SW + части барабанов DrumSep (карточка internal-no-demucs).

Контракт:
- stems.separate(audio_path, out_dir) — без выбора модели; ответ {stems, seconds, model}, model —
  «bs-roformer-sw»;
- основные stem-{drums,bass,other,vocals}.flac в сумме дают трек;
  stem-other = other + guitar + piano модели;
- stem-guitar/stem-piano — подробные внутри «прочего», в сумму не входят;
- stem-{kick,snare,toms,hh,ride,crash}.flac — части внутри «барабанов»,
  в сумму не входят, минус их не видит; сбой DrumSep основное разделение не роняет
  (частей нет, в том числе оставшихся от прошлого разделения);
- окружения разделения нет / подпроцесс упал / таймаут → RuntimeError с текстом про RoFormer
  (разделение), дорожек нет; другой модели и отката нет — PARTSCASTER_STEMS_MODEL=demucs ничего не меняет;
- битый/пустой файл → исключение без частичных дорожек;
- roformer_available() — есть ли файл python окружения разделения (PARTSCASTER_SEP_PY);
- POST /jobs/{id}/stems → 200 и model «bs-roformer-sw» (в т.ч. без файла настроек); сбой разделения или
  окружения нет → 500 «разделение на дорожки не удалось: …», дорожек нет; сбой DrumSep → 200 без частей;
- GET /config: roformer_available отражает наличие окружения; stems_pref/stems_model нет.

Внешние границы подменяются: подпроцесс RoFormer (stems.run_proc — запуск python из PARTSCASTER_SEP_PY со
скриптом sep_run.py) — фейком test_no_demucs.FakeSep, который пишет известные сигналы и печатает JSON по
контракту sep_run.py. torch и веса моделей не нужны. Выбор модели, старые поля клиентов и /version —
test_no_demucs.

Без numpy/soundfile/librosa классы разделения пропускаются; API-классы ещё и без
fastapi/httpx (как соседние в test_pure.py).
"""
import os
import tempfile
import unittest
from pathlib import Path
from unittest import mock

import test_pure as tp
from test_no_demucs import (AMP, DETAIL, DUR, HZ, MAIN, PART_AMP, PART_HZ, PARTS, SR, _SepEnv, _mix, _read,
                            _tone_amp)

try:
    import soundfile  # noqa: F401
    _HAS_SF = True
except ImportError:
    _HAS_SF = False

# librosa — зависимость воркера (requirements.txt): без неё путь RoFormer в stems.py не пройти
try:
    import librosa  # noqa: F401
    _HAS_LIBROSA = True
except ImportError:
    _HAS_LIBROSA = False

_SEP_OK = tp._HAS_NUMPY and _HAS_SF and _HAS_LIBROSA
_SEP_SKIP = "нужны numpy/soundfile/librosa"
_WORKER_OK = tp._HAS_WORKER_DEPS and _HAS_SF and _HAS_LIBROSA
_WORKER_SKIP = "нужны fastapi/httpx/numpy/soundfile/librosa (окружение воркера)"
RF_MODEL = "bs-roformer-sw"
SEP_ERR = "разделение на дорожки не удалось"


@unittest.skipUnless(_SEP_OK, _SEP_SKIP)
class TestSeparateRoformer(_SepEnv, unittest.TestCase):
    """stems.separate(audio_path, out_dir) — RoFormer подпроцессом."""

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

    def _files(self):
        return sorted(p.name for p in self.out.iterdir())

    def _stem_files(self):
        return [f for f in self._files() if f.startswith("stem-")]

    def test_main_and_detail_files_model_roformer(self):
        res = self._separate()
        self.assertEqual(len(self.sep.calls), 1, "RoFormer не вызван")
        for n in MAIN + DETAIL:
            self.assertTrue((self.out / f"stem-{n}.flac").is_file(), f"нет stem-{n}.flac: {self._files()}")
        self.assertEqual(res.get("model"), RF_MODEL)

    def test_other_is_other_plus_guitar_plus_piano(self):
        self._separate()
        x = _read(self.out / "stem-other.flac")
        for n in ("other", "guitar", "piano"):
            self.assertAlmostEqual(_tone_amp(x, HZ[n]), AMP, delta=0.005,
                                   msg=f"в stem-other нет {n} на своём уровне")
        for n in ("drums", "bass", "vocals"):
            self.assertLess(_tone_amp(x, HZ[n]), 0.002, f"{n} попал в stem-other")

    def test_main_stems_are_model_stems(self):
        self._separate()
        for n in ("drums", "bass", "vocals"):
            with self.subTest(stem=n):
                x = _read(self.out / f"stem-{n}.flac")
                self.assertAlmostEqual(_tone_amp(x, HZ[n]), AMP, delta=0.005)
                for other in set(HZ) - {n}:
                    self.assertLess(_tone_amp(x, HZ[other]), 0.002, f"{other} в stem-{n}")

    def test_detail_stems_are_model_guitar_piano(self):
        self._separate()
        for n in DETAIL:
            x = _read(self.out / f"stem-{n}.flac")
            self.assertAlmostEqual(_tone_amp(x, HZ[n]), AMP, delta=0.005, msg=f"stem-{n}")

    def _assert_main_sum_is_input(self):
        import numpy as np
        ref = _read(self.audio)
        s = np.sum([_read(self.out / f"stem-{n}.flac") for n in MAIN], axis=0)
        n = min(len(s), len(ref))
        self.assertGreater(n, len(ref) - SR // 100, "дорожки короче трека")
        resid = float(np.sqrt(np.mean((s[:n] - ref[:n]) ** 2)))
        level = float(np.sqrt(np.mean(ref[:n] ** 2)))
        self.assertLess(20 * np.log10(resid / level + 1e-12), -40.0,
                        "сумма основных дорожек не совпадает с треком")

    def test_main_stems_sum_to_input(self):
        self._separate()
        self._assert_main_sum_is_input()

    def test_detail_not_in_main_sum(self):
        import numpy as np
        self._separate()
        without_other = np.sum([_read(self.out / f"stem-{n}.flac") for n in ("drums", "bass", "vocals")], axis=0)
        for n in DETAIL:
            self.assertLess(_tone_amp(without_other, HZ[n]), 0.002, f"{n} вне «прочего»")
        full = np.sum([_read(self.out / f"stem-{n}.flac") for n in MAIN], axis=0)
        for n in DETAIL:
            self.assertAlmostEqual(_tone_amp(full, HZ[n]), AMP, delta=0.005,
                                   msg=f"{n} в сумме основных не на уровне трека (удвоен/потерян)")

    def test_six_drum_part_files(self):
        self._separate()
        for n in PARTS:
            with self.subTest(part=n):
                f = self.out / f"stem-{n}.flac"
                self.assertTrue(f.is_file(), f"нет {f.name}: {self._files()}")
                self.assertAlmostEqual(_tone_amp(_read(f), PART_HZ[n]), PART_AMP, delta=0.003)

    def test_drum_parts_not_in_main_stems(self):
        import numpy as np
        self._separate()
        full = np.sum([_read(self.out / f"stem-{n}.flac") for n in MAIN], axis=0)
        for n in PARTS:
            self.assertLess(_tone_amp(full, PART_HZ[n]), 0.002, f"часть {n} попала в основные дорожки")

    def _assert_fails(self, **kw):
        with self.assertRaises(RuntimeError) as cm:
            self._separate(**kw)
        self.assertRegex(str(cm.exception), r"(?i)roformer|разделени")
        self.assertEqual(self._stem_files(), [], "дорожки после сбоя разделения")
        return cm.exception

    def test_no_sep_python_raises_without_subprocess(self):
        e = self._assert_fails(sep_py_exists=False)
        self.assertEqual(self.sep.calls, [], "RoFormer вызван без окружения")
        self.assertRegex(str(e), r"(?i)переустанов|install", "нет подсказки, как пройти")

    def test_subprocess_error_raises(self):
        self._assert_fails(sep_mode="fail")
        self.assertEqual(len(self.sep.calls), 1, "запасного второго прохода быть не должно")

    def test_subprocess_timeout_raises(self):
        self._assert_fails(sep_mode="raise")

    def test_old_rollback_env_still_roformer(self):
        res = self._separate(old_rollback="demucs")  # строка старого отката в worker.env
        self.assertEqual(len(self.sep.calls), 1, "PARTSCASTER_STEMS_MODEL=demucs обошёл RoFormer")
        self.assertEqual(res.get("model"), RF_MODEL)
        self.assertTrue((self.out / "stem-drums.flac").is_file())

    def test_drumsep_failure_keeps_main(self):
        res = self._separate(sep_mode="drum_fail")
        self.assertEqual(res.get("model"), RF_MODEL)
        for n in MAIN + DETAIL:
            self.assertTrue((self.out / f"stem-{n}.flac").is_file(), f"нет stem-{n}.flac")
        for n in PARTS:
            self.assertFalse((self.out / f"stem-{n}.flac").exists(), f"stem-{n}.flac при упавшем DrumSep")

    def test_drumsep_failure_drops_parts_of_previous_split(self):
        # части от прошлого разделения не должны остаться рядом с новыми основными дорожками
        import soundfile as sf
        for n in PARTS:
            sf.write(str(self.out / f"stem-{n}.flac"), _mix(), SR)
        self._separate(sep_mode="drum_fail")
        for n in PARTS:
            self.assertFalse((self.out / f"stem-{n}.flac").exists(), f"старая stem-{n}.flac осталась")

    def _broken(self, content: bytes):
        self.audio.write_bytes(content)
        with self.assertRaises(Exception):  # noqa: B017 — тип ошибки не задан, важно «упало»
            self._separate()
        stems_left = self._stem_files()
        self.assertEqual(stems_left, [], f"частичные дорожки после сбоя: {stems_left}")

    def test_empty_file_raises_without_stems(self):
        self._broken(b"")

    def test_garbage_file_raises_without_stems(self):
        self._broken(b"not an audio file at all" * 100)

class TestRoformerAvailability(unittest.TestCase):
    """Без аудио-зависимостей: roformer_available отражает наличие файла PARTSCASTER_SEP_PY."""

    def _check(self, exists):
        import stems
        with tempfile.TemporaryDirectory() as td:
            py = Path(td) / "python"
            if exists:
                py.write_text("")
            with mock.patch.dict(os.environ, {"PARTSCASTER_SEP_PY": str(py)}):
                return stems.roformer_available()

    def test_available_reflects_file(self):
        self.assertTrue(self._check(True))
        self.assertFalse(self._check(False))


@unittest.skipUnless(_WORKER_OK, _WORKER_SKIP)
class TestStemsApiRoformer(_SepEnv, tp._WorkerApiCase):
    """POST /jobs/{id}/stems и GET /config: дорожки только RoFormer."""

    def setUp(self):
        super().setUp()
        # настройки воркера — свои на тест (как TestAutoStemsInstrumentalConfig)
        data = Path(self._td.name) / "cfg"
        data.mkdir()
        self.settings = data / "settings.json"
        for name, val in (("DATA_DIR", data), ("SETTINGS_PATH", self.settings)):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)
        # замер каждой дорожки (librosa) после разделения — не предмет этих тестов
        p = mock.patch.object(self.w, "analyze_file", return_value={})
        p.start()
        self.addCleanup(p.stop)

    def _audio_job(self, content=None):
        import soundfile as sf
        jid = self._job(duration=DUR, semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        if content is None:
            sf.write(str(d / "audio.flac"), _mix(), SR)
        else:
            (d / "audio.flac").write_bytes(content)
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid, d

    def _stems(self, jid):
        return self.client.post(f"/jobs/{jid}/stems")

    def _stem_files(self, d):
        return sorted(p.name for p in d.iterdir() if p.name.startswith("stem-"))

    def _config(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def test_stems_response_model_roformer(self):
        self._setup_sep(Path(self._td.name))
        self.assertFalse(self.settings.exists())   # без файла настроек — тоже RoFormer
        jid, d = self._audio_job()
        r = self._stems(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json().get("model"), RF_MODEL)
        self.assertEqual(len(self.sep.calls), 1)
        for n in MAIN + DETAIL + PARTS:
            self.assertTrue((d / f"stem-{n}.flac").is_file(), f"нет stem-{n}.flac")

    def test_no_sep_python_500_clear_error(self):
        self._setup_sep(Path(self._td.name), sep_py_exists=False)
        jid, d = self._audio_job()
        r = self._stems(jid)
        self.assertEqual(r.status_code, 500, r.text)
        detail = r.json().get("detail", "")
        self.assertIn(SEP_ERR, detail)
        self.assertRegex(detail, r"(?i)переустанов|install")
        self.assertEqual(self.sep.calls, [])
        self.assertEqual(self._stem_files(d), [])

    def test_subprocess_error_500(self):
        self._setup_sep(Path(self._td.name), sep_mode="fail")
        jid, d = self._audio_job()
        r = self._stems(jid)
        self.assertEqual(r.status_code, 500, r.text)
        self.assertIn(SEP_ERR, r.json().get("detail", ""))
        self.assertEqual(len(self.sep.calls), 1)
        self.assertEqual(self._stem_files(d), [], "дорожки при упавшем RoFormer")

    def test_drumsep_failure_200_roformer(self):
        self._setup_sep(Path(self._td.name), sep_mode="drum_fail")
        jid, d = self._audio_job()
        r = self._stems(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json().get("model"), RF_MODEL)
        self.assertTrue((d / "stem-vocals.flac").is_file())
        for n in PARTS:
            self.assertFalse((d / f"stem-{n}.flac").exists())

    def test_broken_audio_500_separation_failed(self):
        self._setup_sep(Path(self._td.name))
        for content in (b"", b"garbage" * 200):
            with self.subTest(size=len(content)):
                jid, d = self._audio_job(content)
                r = self._stems(jid)
                self.assertEqual(r.status_code, 500, r.text)
                self.assertIn(SEP_ERR, r.json().get("detail", ""))
                left = self._stem_files(d)
                self.assertEqual(left, [], f"частичные дорожки: {left}")

    def test_config_availability_without_choice(self):
        for installed in (True, False):
            with self.subTest(installed=installed):
                self._setup_sep(Path(self._td.name), sep_py_exists=installed)
                cfg = self._config()
                self.assertIs(cfg.get("roformer_available"), installed, cfg)
                self.assertNotIn("stems_model", cfg)
                self.assertNotIn("stems_pref", cfg)


if __name__ == "__main__":
    unittest.main()
