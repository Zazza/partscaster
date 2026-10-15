"""Тесты распознавания нот гитары на воркере: модуль notes (окружение распознавания, как voice.py
для Seed-VC), POST /jobs/{id}/guitar_notes, /config и применение блока gtrnotes к дорожке
(POST /jobs/{id}/fx) — по контракту карточки internal-guitar-notes, не по реализации.

Контракт:
- notes.NOTES_DIR = $PARTSCASTER_NOTES_DIR или ~/partscaster/notes; available() — есть
  NOTES_DIR/venv/bin/python; engines() — список из {"basic-pitch", "yourmt3"} по наличию;
  run(stem_path, out_json) — подпроцесс notes_run.py интерпретатором окружения, ответ — JSON;
- POST /jobs/{id}/guitar_notes?force= — распознать (кэш jobs/<id>/guitar-notes.json, если есть и не
  force): 404 нет джобы; 422 нет дорожки гитары; 503 «распознавание нот не установлено:
  worker/notes_install.sh»; нехватка видеопамяти → 503 с понятным текстом; под gpu_queue("notes").
  Ответ {"notes": N, "engines": [...], "cached": bool};
- кэш: {"version": 1, "engines": [...], "notes": [[s, e, m, v]...], "beats": [...], "tune_cents": float};
  ноты — ансамбль (pick_centered → union с basic-pitch → clean); сила ноты — громкость дорожки гитары
  в момент начала; строй — основной тон баса и голоса;
- /config: notes_available, notes_engines;
- POST /jobs/{id}/fx с gtrnotes: воркер кладёт в блок ноты из кэша трека (нет кэша — распознаёт сам под
  очередью видеокарты), окно сдвигает ноты и доли; кэш превью учитывает файл guitar-notes.json.
  Распознавание одно на трек — переиспользуется всеми звуками (наборами).

Внешние границы подменяются: распознаватель — notes.run/available/engines (монкипатч модуля notes),
подпроцесс — subprocess.run (в тестах notes.run), хранилище наборов — фейк (как в test_fx_api),
gpu_queue — записывающий контекст.

Предположения теста (контракт формат ответа notes.run не фиксирует — находка для карточки):
- notes.run(stem_path, out_json) пишет out_json и возвращает тот же словарь
  {"engines": [...], "basic_pitch": [[s, e, m, v]...], "yourmt3": [проход0, проход1] (если стоит),
   "seg": длина куска YourMT3+, с};
- нехватка видеопамяти приходит из notes.run исключением RuntimeError с «CUDA out of memory»
  (как текст ошибки torch в выводе подпроцесса).
"""
import contextlib
import importlib
import json
import os
import unittest
from pathlib import Path
from unittest import mock

import test_fx_api as fa
from test_fx_engine import FakeResources

SR = fa.SR
DUR = fa.DUR


def _hz(m):
    return 440.0 * 2 ** ((m - 69) / 12)


def _tone_stereo(hz, amps, dur=DUR, sr=SR):
    """Стерео-синус hz; amps — [(t0, t1, амплитуда)]."""
    import numpy as np
    t = np.arange(int(dur * sr)) / sr
    a = np.zeros_like(t)
    for t0, t1, v in amps:
        a[int(t0 * sr):int(t1 * sr)] = v
    x = (a * np.sin(2 * np.pi * hz * t)).astype(np.float32)
    return np.stack([x, x], axis=1)


class GtrKitRes(FakeResources):
    """Хранилище наборов: kit(name) → (сэмплы, sr), kit_names(name) → имена; нет — KeyError."""

    KITS = ("fake/notes", "fake/other")

    def __init__(self):
        super().__init__()
        import numpy as np
        t = np.arange(int(1.5 * 44100)) / 44100
        self.names = ["a2_f_rr1", "c3_f_rr1", "eb3_f_rr1", "gb3_f_rr1", "a3_f_rr1", "c4_f_rr1", "eb4_f_rr1"]
        mids = [45, 48, 51, 54, 57, 60, 63]
        smp = [(0.4 * np.sin(2 * np.pi * _hz(m) * t) * np.exp(-t)).astype(np.float32) for m in mids]
        for k in self.KITS:
            self.kits[k] = (smp, 44100)

    def kit_names(self, name):
        if name not in self.kits:
            raise KeyError(name)
        return list(self.names)


BP = [[0.5, 1.2, 57, 0.6], [1.6, 2.3, 60, 0.6], [2.8, 3.5, 64, 0.6]]


def _raw(bp=None, mt3=None, seg=8.0):
    out = {"engines": ["basic-pitch"] + (["yourmt3"] if mt3 is not None else []),
           "basic_pitch": [list(n) for n in (BP if bp is None else bp)], "seg": seg}
    if mt3 is not None:
        out["yourmt3"] = mt3
    return out


@unittest.skipUnless(fa._OK, fa._SKIP)
class _NotesCase(fa._FxApiCase):
    """Своя БД/данные; notes.run/available/engines — подмены; gpu_queue — записывающий контекст."""

    def setUp(self):
        super().setUp()
        self.notes = importlib.import_module("notes")
        self.raw = _raw()
        self.run_calls = []
        self.run_inside_gpu = []
        self.gpu_sections = []
        self._in_gpu = False
        self.run_error = None
        test = self

        def fake_run(*args, **kw):
            test.run_calls.append((args, kw))
            test.run_inside_gpu.append(test._in_gpu)
            if test.run_error is not None:
                raise test.run_error
            out = args[1] if len(args) > 1 else kw.get("out_json")
            if out is not None:
                Path(out).write_text(json.dumps(test.raw))
            return json.loads(json.dumps(test.raw))

        @contextlib.contextmanager
        def fake_queue(section="gpu"):
            test.gpu_sections.append(section)
            test._in_gpu = True
            try:
                yield
            finally:
                test._in_gpu = False

        self.available = True
        for target, attr, val in ((self.notes, "run", fake_run),
                                  (self.notes, "available", lambda: test.available),
                                  (self.notes, "engines", lambda: ["basic-pitch"] if test.available else []),
                                  (self.w, "gpu_queue", fake_queue)):
            p = mock.patch.object(target, attr, val)
            p.start()
            self.addCleanup(p.stop)
        self.resources = GtrKitRes()

    def _gtr_job(self, guitar=True, gtr_amps=((0.0, DUR, 0.1),)):
        extra = {"guitar": _tone_stereo(_hz(57), list(gtr_amps))} if guitar else {}
        return self._audio_job(extra=extra)

    def _recognize(self, jid, force=None):
        params = {} if force is None else {"force": force}
        return self.client.post(f"/jobs/{jid}/guitar_notes", params=params)

    def _cache(self, d):
        return d / "guitar-notes.json"

    def _write_cache(self, d, notes, beats=(0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5), tune=0.0):
        self._cache(d).write_text(json.dumps({"version": 1, "engines": ["basic-pitch"],
                                              "notes": [list(n) for n in notes], "beats": list(beats),
                                              "tune_cents": tune}))


class TestGuitarNotesEndpoint(_NotesCase):

    def test_no_job_404(self):
        r = self._recognize(9999)
        self.assertEqual(r.status_code, 404, r.text)
        self.assertEqual(self.run_calls, [])

    def test_no_guitar_stem_422(self):
        jid, d = self._gtr_job(guitar=False)
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 422, r.text)
        self.assertEqual(self.run_calls, [])
        self.assertFalse(self._cache(d).exists())

    def test_not_installed_503(self):
        self.available = False
        jid, d = self._gtr_job()
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 503, r.text)
        self.assertIn("notes_install.sh", r.text)
        self.assertEqual(self.run_calls, [])
        self.assertFalse(self._cache(d).exists())

    def test_out_of_vram_503_plain_text(self):
        self.run_error = RuntimeError("CUDA out of memory. Tried to allocate 2.00 GiB")
        jid, d = self._gtr_job()
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 503, r.text)
        self.assertIn("видеопамят", r.text.lower())
        self.assertFalse(self._cache(d).exists(), "ошибка распознавания оставила кэш")

    def test_first_call_recognizes(self):
        jid, d = self._gtr_job()
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        out = r.json()
        self.assertEqual(out["cached"], False)
        self.assertEqual(out["notes"], len(BP))
        self.assertEqual(out["engines"], ["basic-pitch"])
        self.assertEqual(len(self.run_calls), 1)

    def test_runs_inside_gpu_queue_notes(self):
        jid, _ = self._gtr_job()
        self.assertEqual(self._recognize(jid).status_code, 200)
        self.assertIn("notes", self.gpu_sections)
        self.assertEqual(self.run_inside_gpu, [True], "распознавание вне очереди видеокарты")

    def test_second_call_cached(self):
        jid, _ = self._gtr_job()
        first = self._recognize(jid).json()
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        out = r.json()
        self.assertIs(out["cached"], True)
        self.assertEqual(out["notes"], first["notes"])
        self.assertEqual(out["engines"], first["engines"])
        self.assertEqual(len(self.run_calls), 1, "повтор вызвал распознавание")

    def test_force_recomputes(self):
        jid, d = self._gtr_job()
        self._recognize(jid)
        self.raw = _raw(bp=BP[:2])
        r = self._recognize(jid, force=1)
        self.assertEqual(r.status_code, 200, r.text)
        out = r.json()
        self.assertIs(out["cached"], False)
        self.assertEqual(len(self.run_calls), 2)
        self.assertEqual(out["notes"], 2)
        self.assertEqual(len(json.loads(self._cache(d).read_text())["notes"]), 2)

    def test_cache_file_format(self):
        jid, d = self._gtr_job()
        self._recognize(jid)
        c = json.loads(self._cache(d).read_text())
        self.assertEqual(c["version"], 1)
        self.assertEqual(c["engines"], ["basic-pitch"])
        self.assertIsInstance(c["beats"], list)
        for b in c["beats"]:
            self.assertIsInstance(b, (int, float))
        self.assertIsInstance(c["tune_cents"], (int, float))
        self.assertNotIsInstance(c["tune_cents"], bool)
        self.assertEqual(len(c["notes"]), len(BP))
        for n, want in zip(sorted(c["notes"]), BP, strict=True):
            with self.subTest(note=n):
                self.assertEqual(len(n), 4)
                s, e, m, v = n
                self.assertAlmostEqual(s, want[0], delta=0.01)
                self.assertGreater(e, s)
                self.assertEqual(int(round(m)), want[2])
                self.assertTrue(0.0 <= v <= 1.0, v)

    def test_ensemble_with_yourmt3(self):
        # оба прохода YourMT3+ нашли те же ноты, что basic-pitch (±20 мс): ансамбль их не удваивает
        mt3 = [[[s + 0.02, e, m, v] for s, e, m, v in BP] for _ in range(2)]
        self.raw = _raw(mt3=mt3, seg=8.0)
        jid, d = self._gtr_job()
        out = self._recognize(jid).json()
        self.assertEqual(sorted(out["engines"]), ["basic-pitch", "yourmt3"])
        self.assertEqual(out["notes"], len(BP))

    def test_ensemble_adds_basic_pitch_only_notes_and_cleans_ladder(self):
        # YourMT3+ нашёл две ноты; basic-pitch — их же, ещё одну и «лесенку» из 4 коротких
        mt3 = [[list(BP[0]), list(BP[1])] for _ in range(2)]
        ladder = [[3.0 + i * 0.02, 3.04 + i * 0.02, 66 + 2 * i, 0.5] for i in range(4)]
        self.raw = _raw(bp=BP + ladder, mt3=mt3, seg=8.0)
        jid, d = self._gtr_job()
        self.assertEqual(self._recognize(jid).json()["notes"], 3)
        midis = sorted(int(round(n[2])) for n in json.loads(self._cache(d).read_text())["notes"])
        self.assertEqual(midis, [57, 60, 64])

    def test_velocity_from_guitar_loudness(self):
        # гитара тихая до 2 с и громкая после: сила второй ноты больше первой
        self.raw = _raw(bp=[[0.8, 1.4, 57, 0.5], [2.8, 3.4, 57, 0.5]])
        jid, d = self._gtr_job(gtr_amps=((0.0, 2.0, 0.02), (2.0, DUR, 0.3)))
        self._recognize(jid)
        notes = sorted(json.loads(self._cache(d).read_text())["notes"])
        self.assertLess(notes[0][3], notes[1][3])

    def test_tune_from_bass_and_vocals(self):
        # бас и голос строят на +30 центов — строй трека ≈ +30
        import soundfile as sf
        jid, d = self._gtr_job()
        shift = 2 ** (30 / 1200)
        sf.write(str(d / "stem-bass.flac"), _tone_stereo(_hz(43) * shift, [(0, DUR, 0.2)]), SR)
        sf.write(str(d / "stem-vocals.flac"), _tone_stereo(_hz(69) * shift, [(0, DUR, 0.2)]), SR)
        self._recognize(jid)
        tune = json.loads(self._cache(d).read_text())["tune_cents"]
        self.assertAlmostEqual(tune, 30, delta=10)


class TestConfig(_NotesCase):

    def test_config_reports_notes(self):
        for avail, engines in ((True, ["basic-pitch", "yourmt3"]), (False, [])):
            with self.subTest(available=avail):
                with mock.patch.object(self.notes, "available", return_value=avail), \
                        mock.patch.object(self.notes, "engines", return_value=engines):
                    cfg = self.client.get("/config").json()
                self.assertIs(cfg["notes_available"], avail)
                self.assertEqual(cfg["notes_engines"], engines)


GTR = [{"type": "gtrnotes", "kit": "fake/notes"}]


class TestFxWithGtrnotes(_NotesCase):
    """Применение цепочки с gtrnotes к дорожке гитары."""

    def _apply(self, jid, chain=None, **body):
        return self._fx(jid, source="guitar", chain=chain or GTR, **body)

    def test_recognizes_itself_without_cache(self):
        jid, d = self._gtr_job()
        r = self._apply(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(len(self.run_calls), 1)
        self.assertTrue(self._cache(d).is_file(), "результат распознавания не сохранён у трека")
        self.assertEqual(self.run_inside_gpu, [True], "распознавание вне очереди видеокарты")

    def test_one_recognition_for_all_sounds(self):
        jid, _ = self._gtr_job()
        for chain in (GTR, [{"type": "gtrnotes", "kit": "fake/other"}],
                      [{"type": "gtrnotes", "kit": "fake/notes", "output_db": -3}]):
            with self.subTest(chain=chain):
                r = self._apply(jid, chain=chain)
                self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(len(self.run_calls), 1)

    def test_uses_cache_without_recognizer(self):
        self.available = False   # распознавание не установлено, но ноты у трека уже есть
        jid, d = self._gtr_job()
        self._write_cache(d, BP)
        r = self._apply(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.run_calls, [])

    def test_cached_notes_reach_block_shifted_to_window(self):
        import fx_engine
        jid, d = self._gtr_job()
        self._write_cache(d, [[1.0, 1.5, 57, 0.8], [2.5, 2.9, 60, 0.8]])
        with mock.patch.object(fx_engine, "process", wraps=fx_engine.process) as proc:
            r = self._fx(jid, source="guitar", chain=GTR, preview=True, **{"from": 2.0, "to": 3.0})
        self.assertEqual(r.status_code, 200, r.text)
        blocks = [b for c in proc.call_args_list for b in (c.args[2] if len(c.args) > 2 else c.kwargs["chain"])
                  if b.get("type") == "gtrnotes"]
        self.assertTrue(blocks, "блок gtrnotes не дошёл до движка")
        b = blocks[0]
        got = [(round(n["t"], 3), [int(round(m)) for m in n["midi"]]) for n in b["notes"]]
        self.assertIn((0.5, [60]), got, "нота 2,5 с не сдвинута к окну с 2,0 с")
        self.assertNotIn([57], [m for _, m in got], "нота, отзвучавшая до окна, осталась")
        beats = [round(x, 3) for x in b.get("_beats", [])]
        self.assertIn(0.0, beats)   # доля 2,0 с — начало окна
        self.assertIn(0.5, beats)   # доля 2,5 с

    def test_preview_cache_depends_on_notes_file(self):
        jid, d = self._gtr_job()
        self._write_cache(d, BP)
        win = {"from": 0.0, "to": 3.0}
        a = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **win)
        self.assertEqual(a.status_code, 200, a.text)
        again = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **win)
        self.assertEqual(again.json()["file"], a.json()["file"], "тот же запрос — не из кэша превью")
        self._write_cache(d, BP[:1])
        st = self._cache(d).stat()
        os.utime(self._cache(d), ns=(st.st_atime_ns, st.st_mtime_ns + 2_000_000_000))
        b = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **win)
        self.assertEqual(b.status_code, 200, b.text)
        self.assertNotEqual(b.json()["file"], a.json()["file"], "ноты сменились, а превью старое")
        self.assertEqual(self.run_calls, [])


class TestNotesModule(unittest.TestCase):
    """notes.py: каталог окружения, наличие, движки, вызов подпроцесса."""

    def setUp(self):
        import tempfile
        self.notes = importlib.import_module("notes")
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        self.dir = Path(td.name)
        p = mock.patch.object(self.notes, "NOTES_DIR", self.dir)
        p.start()
        self.addCleanup(p.stop)

    def _py(self):
        py = self.dir / "venv" / "bin" / "python"
        py.parent.mkdir(parents=True, exist_ok=True)
        py.write_text("")
        return py

    def test_dir_from_env(self):
        with mock.patch.dict(os.environ, {"PARTSCASTER_NOTES_DIR": str(self.dir / "custom")}):
            mod = importlib.reload(self.notes)
            self.assertEqual(Path(mod.NOTES_DIR), self.dir / "custom")
        importlib.reload(self.notes)

    def test_dir_default_in_home(self):
        env = {k: v for k, v in os.environ.items() if k != "PARTSCASTER_NOTES_DIR"}
        env["HOME"] = str(self.dir)
        with mock.patch.dict(os.environ, env, clear=True):
            mod = importlib.reload(self.notes)
            self.assertEqual(Path(mod.NOTES_DIR), self.dir / "partscaster" / "notes")
        importlib.reload(self.notes)

    def test_unavailable_without_python(self):
        self.assertFalse(self.notes.available())
        self.assertNotIn("yourmt3", self.notes.engines())

    def test_available_with_python(self):
        self._py()
        self.assertTrue(self.notes.available())
        self.assertEqual(self.notes.engines(), ["basic-pitch"], "без чекпоинта YourMT3+ — только basic-pitch")

    def test_run_calls_notes_run_with_env_python(self):
        py = self._py()
        out = self.dir / "out.json"
        payload = _raw()
        calls = []

        def fake_sub(cmd, *a, **kw):
            calls.append([str(x) for x in cmd])
            out.write_text(json.dumps(payload))
            import subprocess
            return subprocess.CompletedProcess(cmd, 0, stdout=json.dumps(payload) + "\n", stderr="")

        with mock.patch("subprocess.run", fake_sub):
            got = self.notes.run(self.dir / "stem-guitar.wav", out)
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0][0], str(py))
        self.assertTrue(any(Path(x).name == "notes_run.py" for x in calls[0]), calls[0])
        self.assertEqual(got, payload)

    def test_run_failure_raises(self):
        self._py()

        def fake_sub(cmd, *a, **kw):
            import subprocess
            return subprocess.CompletedProcess(cmd, 1, stdout="", stderr="Traceback\nboom")

        with mock.patch("subprocess.run", fake_sub):
            with self.assertRaises(RuntimeError):   # как voice.convert: ошибка подпроцесса — RuntimeError
                self.notes.run(self.dir / "stem-guitar.wav", self.dir / "out.json")


class TestInstallFiles(unittest.TestCase):
    """Окружение распознавания ставится своим скриптом (как Seed-VC)."""

    WORKER = Path(__file__).resolve().parent

    def test_files_exist(self):
        for name in ("notes.py", "notes_run.py", "notes_install.sh", "requirements-notes.txt"):
            with self.subTest(file=name):
                self.assertTrue((self.WORKER / name).is_file(), f"нет worker/{name}")

    def test_install_uses_notes_dir_and_torch_index(self):
        text = (self.WORKER / "notes_install.sh").read_text(encoding="utf-8")
        self.assertIn("PARTSCASTER_NOTES_DIR", text)
        self.assertIn("PARTSCASTER_TORCH_INDEX", text)


if __name__ == "__main__":
    unittest.main()
