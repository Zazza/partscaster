"""Тесты партии гитариста на воркере: GET /jobs/{id}/guitar_part, кэш guitar-part.json, блок gtrnotes в
POST /jobs/{id}/fx и удаление распознавания нот — по карточке internal-guitar-plan-chords (условия 1, 2, 7, 8, 9),
не по реализации.

Контракт:
- GET /jobs/{id}/guitar_part?style=N → {style, bars: [{start, end, chord, section}], strikes: [{t, midi: [...]}],
  beats, tune_cents}; style по умолчанию 0 (авто); удары — моменты настоящих ударов старой гитары (пик атаки
  дорожки stem-guitar.flac), аккорд удара — аккорд плана в его такте (та же сетка, что /chord_grid);
- нет плана (score.abc) → 422 со словом «транскрипц»; нет дорожки гитары (после попытки стемов) → 422 «нет
  дорожки гитары»; нет джобы → 404;
- партия считается без видеокарты (gpu_queue не берётся);
- кэш jobs/<id>/guitar-part.json: {version, stem, plan, onsets, bars, beats, tune_cents}; stem/plan — отпечатки
  stem-guitar.flac и score.abc (mtime_ns, size); не совпал — пересчёт; удары и такты берутся из кэша;
- POST /jobs/{id}/fx с gtrnotes: ноты блока = партия по манере style блока (по умолчанию 0); нот нет → 422;
  превью меняется, когда меняется партия (новый план);
- распознавание удалено: нет маршрута /jobs/{id}/guitar_notes, в /config нет notes_available/notes_engines,
  нет файлов notes.py, notes_run.py, notes_install.sh, requirements-notes.txt; worker.env.example — без
  PARTSCASTER_NOTES_DIR; tools/worker-tar.sh — без notes_install.sh.

Данные теста — настоящие файлы трека: звук и барабаны — удары 120 BPM с трезвучиями плана (как в test_chordgrid),
план — 8 тактов Dm C Bb A (куплет, припев), гитара — щипки в известные моменты. Внешние границы: хранилище
наборов — фейк (как в test_fx_api), gpu_queue — записывающий контекст. Свои функции воркера не подменяются;
fx_engine.process только обёрнут (mock wraps), чтобы увидеть ноты, дошедшие до блока.

Предположения теста:
- у ударов до первого такта с аккордом midi пуст или такого удара нет в strikes — проверяются удары с нотами;
- аккорд такта, у которого его нет (None), — аккорд прежнего такта (так же, как в guitarpart.part);
- аппликатуры — из карточки: Dm [50,57,62,65], C [48,52,55,60,64], A [45,52,57,61,64], Bb — баррэ от 46.
"""
import contextlib
import json
import os
import threading
import unittest
from pathlib import Path
from unittest import mock

import test_fx_api as fa
from test_fx_engine import FakeResources

try:
    import numpy as np
    from test_chordgrid import _PLAN8, _beats_audio
    from test_guitarpart import _plucks
    _HAS = fa._OK
except ImportError:
    _HAS = False

SR = fa.SR
DUR = 16.0
WORKER = Path(__file__).resolve().parent
ROOT = WORKER.parent

# аппликатуры по карточке
VOICING = {"Dm": [50, 57, 62, 65], "C": [48, 52, 55, 60, 64], "A": [45, 52, 57, 61, 64],
           "Bb": [46, 53, 58, 62, 65, 70], "G": [43, 47, 50, 55, 59, 67]}
# щипки гитары — не на долях (сетка 120 BPM с 0,25 с), чтобы удары гитары не совпадали с барабанами
PLUCK_T = [round(0.42 + 0.61 * i, 3) for i in range(25)]
NEW_T = [round(0.7 + 0.9 * i, 3) for i in range(14)]
PLAN_G = _PLAN8.replace('"Dm"', '"G"').replace('"C"', '"G"').replace('"Bb"', '"G"').replace('"A"', '"G"') \
    if _HAS else ""
PLAN_NO_CHORDS = ("X:1\nM:4/4\nL:1/16\nQ:1/4=120\nK:Dm\n% verse\nV: Ins\n"
                  "D16|C16|B,16|A,16|\n% chorus\nD16|C16|B,16|A,16|\n")


def _hz(m):
    return 440.0 * 2 ** ((m - 69) / 12)


def _tone_stereo(hz, amp=0.2, dur=DUR, sr=SR):
    t = np.arange(int(dur * sr)) / sr
    x = (amp * np.sin(2 * np.pi * hz * t)).astype(np.float32)
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


def _resolved(bars):
    """Такты ответа → [(start, end, аккорд с наследованием прежнего, section)]."""
    out, prev = [], None
    for b in bars:
        c = b.get("chord") if b.get("chord") in VOICING else prev
        out.append((float(b["start"]), float(b["end"]), c, b.get("section") or ""))
        prev = c
    return out


def _bar_of(bars, t):
    rb = _resolved(bars)
    for a, e, c, sec in rb:
        if a <= t < e:
            return c, sec
    if rb and t >= rb[-1][1]:
        return rb[-1][2], rb[-1][3]
    return None, None


def _bump(path, delta_ns):
    st = path.stat()
    os.utime(path, ns=(st.st_atime_ns, st.st_mtime_ns + delta_ns))


@unittest.skipUnless(_HAS, fa._SKIP)
class _PartCase(fa._FxApiCase):
    """Своя БД/данные; gpu_queue — записывающий контекст; хранилище наборов — фейк."""

    def setUp(self):
        super().setUp()
        self.gpu_sections = []
        test = self

        @contextlib.contextmanager
        def fake_queue(section="gpu"):
            test.gpu_sections.append(section)
            yield

        p = mock.patch.object(self.w, "gpu_queue", fake_queue)
        p.start()
        self.addCleanup(p.stop)
        self.resources = GtrKitRes()

    def _part_job(self, plan=_PLAN8 if _HAS else None, guitar=True, pluck_t=None, bass=None, vocals=None):
        """Трек 16 с: звук и стемы — удары 120 BPM с трезвучиями плана; гитара — щипки в pluck_t."""
        import soundfile as sf
        jid = self._job(duration=DUR, semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        mix = _beats_audio(dur=DUR)
        sf.write(str(d / "audio.flac"), mix, SR)
        sf.write(str(d / "stem-drums.flac"), mix, SR)
        sf.write(str(d / "stem-bass.flac"), mix if bass is None else bass, SR)
        sf.write(str(d / "stem-other.flac"), mix, SR)
        sf.write(str(d / "stem-vocals.flac"), (0.001 * mix) if vocals is None else vocals, SR)
        if guitar:
            sf.write(str(d / "stem-guitar.flac"),
                     _plucks(PLUCK_T if pluck_t is None else pluck_t, dur=DUR, sr=SR, stereo=True), SR)
        if plan is not None:
            (d / "score.abc").write_text(plan)
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid, d

    def _part(self, jid, style=None):
        params = {} if style is None else {"style": style}
        return self.client.get(f"/jobs/{jid}/guitar_part", params=params)

    def _ok(self, jid, style=None):
        r = self._part(jid, style)
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    @staticmethod
    def _played(out):
        """Удары с нотами: [(t, отсортированные midi)]."""
        return [(float(s["t"]), sorted(int(m) for m in s["midi"])) for s in out["strikes"] if s.get("midi")]


# ---------------------------------------------------------------- ответ эндпоинта

class TestGuitarPartEndpoint(_PartCase):

    def test_structure(self):
        jid, _ = self._part_job()
        out = self._ok(jid, 1)
        self.assertEqual(out["style"], 1)
        self.assertTrue(out["bars"], "нет тактов")
        for b in out["bars"]:
            for k in ("start", "end", "chord", "section"):
                self.assertIn(k, b)
            self.assertLess(b["start"], b["end"])
        self.assertTrue(out["strikes"], "нет ударов")
        for s in out["strikes"]:
            self.assertIsInstance(s["t"], (int, float))
            self.assertIsInstance(s["midi"], list)
        self.assertIsInstance(out["beats"], list)
        self.assertTrue(out["beats"], "нет долей (барабаны есть)")
        self.assertIsInstance(out["tune_cents"], (int, float))
        self.assertNotIsInstance(out["tune_cents"], bool)

    def test_style_echoed(self):
        jid, _ = self._part_job()
        for style in range(5):
            with self.subTest(style=style):
                self.assertEqual(self._ok(jid, style)["style"], style)

    def test_default_style_auto(self):
        jid, _ = self._part_job()
        a = self._ok(jid)
        b = self._ok(jid, 0)
        self.assertEqual(a["style"], 0)
        self.assertEqual(self._played(a), self._played(b))

    def test_strikes_are_guitar_onsets(self):
        # условие 1: удары — моменты настоящих ударов старой гитары (в пределах 30 мс)
        jid, _ = self._part_job()
        out = self._ok(jid, 1)
        played = self._played(out)
        self.assertTrue(played)
        for t, _ in played:
            with self.subTest(strike=t):
                self.assertLessEqual(min(abs(t - p) for p in PLUCK_T), 0.03, f"удар {t} не из дорожки гитары")
        first = min(b["start"] for b in out["bars"] if b.get("chord"))
        due = [p for p in PLUCK_T if p >= first + 0.05]
        found = [p for p in due if any(abs(t - p) <= 0.03 for t, _ in played)]
        self.assertGreaterEqual(len(found), 0.8 * len(due), f"найдено {len(found)} ударов из {len(due)}")

    def test_strum_plays_bar_chord(self):
        # условие 1: аккорд удара — аккорд плана в такте звука
        jid, _ = self._part_job()
        out = self._ok(jid, 1)
        played = self._played(out)
        self.assertTrue(played)
        chords = set()
        for t, midi in played:
            chord, _ = _bar_of(out["bars"], t)
            with self.subTest(strike=t, chord=chord):
                self.assertIn(chord, VOICING)
                self.assertEqual(midi, VOICING[chord])
            chords.add(chord)
        self.assertTrue({"Dm", "C", "Bb", "A"} <= chords, f"аккорды плана не все сыграны: {chords}")

    def test_arpeggio_one_note(self):
        jid, _ = self._part_job()
        out = self._ok(jid, 2)
        for t, midi in self._played(out):
            chord, _ = _bar_of(out["bars"], t)
            with self.subTest(strike=t):
                self.assertEqual(len(midi), 1)
                self.assertIn(midi[0], VOICING[chord])

    def test_power_fifths(self):
        jid, _ = self._part_job()
        out = self._ok(jid, 4)
        for t, midi in self._played(out):
            chord, _ = _bar_of(out["bars"], t)
            with self.subTest(strike=t):
                self.assertEqual([m - midi[0] for m in midi], [0, 7, 12])
                self.assertEqual(midi[0], VOICING[chord][0])

    def test_auto_by_section(self):
        # авто: куплет — перебор (одна нота), припев — бой (весь аккорд)
        jid, _ = self._part_job()
        out = self._ok(jid, 0)
        seen = set()
        for t, midi in self._played(out):
            chord, sec = _bar_of(out["bars"], t)
            if sec == "verse":
                self.assertEqual(len(midi), 1, f"куплет, удар {t}: {midi}")
                seen.add(sec)
            elif sec == "chorus":
                self.assertEqual(midi, VOICING[chord], f"припев, удар {t}")
                seen.add(sec)
        self.assertEqual(seen, {"verse", "chorus"}, "в треке не нашлось обеих частей — проверка пустая")

    def test_bars_same_grid_as_chord_grid(self):
        # «та же сетка, что chord_grid»
        jid, _ = self._part_job()
        out = self._ok(jid, 1)
        grid = self.client.get(f"/jobs/{jid}/chord_grid")
        self.assertEqual(grid.status_code, 200, grid.text)
        want = [(round(b["start"], 2), round(b["end"], 2), b["chord"], b["section"]) for b in grid.json()["bars"]]
        got = [(round(b["start"], 2), round(b["end"], 2), b["chord"], b["section"]) for b in out["bars"]]
        self.assertEqual(got, want)

    def test_no_gpu(self):
        # условие 8: партия считается без видеокарты
        jid, _ = self._part_job()
        self._ok(jid, 1)
        self.assertEqual(self.gpu_sections, [], "партия взяла очередь видеокарты")

    def test_tune_from_bass_and_vocals(self):
        # строй трека — основной тон баса и голоса (как раньше): +30 центов
        shift = 2 ** (30 / 1200)
        jid, _ = self._part_job(bass=_tone_stereo(_hz(43) * shift), vocals=_tone_stereo(_hz(69) * shift))
        self.assertAlmostEqual(self._ok(jid, 1)["tune_cents"], 30, delta=10)


class TestGuitarPartErrors(_PartCase):

    def test_no_job_404(self):
        self.assertEqual(self._part(9999, 1).status_code, 404)

    def test_no_plan_422_transcription(self):
        jid, d = self._part_job(plan=None)
        r = self._part(jid, 1)
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("транскрипц", r.text.lower())
        self.assertFalse((d / "guitar-part.json").exists())

    def test_no_guitar_422(self):
        jid, d = self._part_job(guitar=False)
        r = self._part(jid, 1)
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("нет дорожки гитары", r.text.lower())
        self.assertFalse((d / "guitar-part.json").exists())


# ---------------------------------------------------------------- кэш у трека

class TestGuitarPartCache(_PartCase):

    def _cache(self, d):
        return d / "guitar-part.json"

    def test_cache_written(self):
        jid, d = self._part_job()
        self._ok(jid, 1)
        c = json.loads(self._cache(d).read_text())
        for k in ("version", "stem", "plan", "onsets", "bars", "beats", "tune_cents"):
            self.assertIn(k, c)
        self.assertTrue(c["onsets"])
        self.assertTrue(c["bars"])

    def test_cache_reused(self):
        # удары и такты берутся из кэша: правка ударов в кэше видна в ответе (отпечатки те же)
        jid, d = self._part_job()
        out = self._ok(jid, 1)
        c = json.loads(self._cache(d).read_text())
        bar = next(b for b in out["bars"] if b.get("chord") in VOICING)
        t = round(float(bar["start"]) + 0.3, 3)
        c["onsets"] = [t]
        self._cache(d).write_text(json.dumps(c))
        again = self._played(self._ok(jid, 1))
        self.assertEqual([round(x, 3) for x, _ in again], [t], "ответ не из кэша — партия посчитана заново")
        self.assertEqual(again[0][1], VOICING[bar["chord"]])

    def test_same_cache_for_all_styles(self):
        jid, d = self._part_job()
        self._ok(jid, 1)
        st = self._cache(d).stat().st_mtime_ns
        for style in (0, 2, 3, 4):
            self._ok(jid, style)
        self.assertEqual(self._cache(d).stat().st_mtime_ns, st, "смена манеры пересчитала удары и такты")

    def _rewrite_guitar(self, d, delta_ns):
        import soundfile as sf
        g = d / "stem-guitar.flac"
        old = g.stat()
        sf.write(str(g), _plucks(NEW_T, dur=DUR - 3.0, sr=SR, stereo=True), SR)
        os.utime(g, ns=(old.st_atime_ns, old.st_mtime_ns + delta_ns))
        self.assertNotEqual(g.stat().st_size, old.st_size, "опора: размер дорожки сменился")

    def _assert_new_onsets(self, out):
        played = self._played(out)
        self.assertTrue(played)
        for t, _ in played:
            self.assertLessEqual(min(abs(t - p) for p in NEW_T), 0.03, f"удар {t} — из старой дорожки")

    def test_new_guitar_stem_recomputes(self):
        jid, d = self._part_job()
        self._ok(jid, 1)
        self._rewrite_guitar(d, 2_000_000_000)
        self._assert_new_onsets(self._ok(jid, 1))

    def test_new_guitar_stem_older_mtime_recomputes(self):
        # отпечаток — mtime_ns и размер: дорожку подменили файлом «из прошлого» — кэш всё равно устарел
        jid, d = self._part_job()
        self._ok(jid, 1)
        self._rewrite_guitar(d, -5_000_000_000)
        self._assert_new_onsets(self._ok(jid, 1))

    def test_new_plan_recomputes(self):
        jid, d = self._part_job()
        before = self._ok(jid, 1)
        self.assertTrue({"Dm", "C"} & {b["chord"] for b in before["bars"]})
        (d / "score.abc").write_text(PLAN_G)
        _bump(d / "score.abc", 2_000_000_000)
        out = self._ok(jid, 1)
        self.assertEqual({b["chord"] for b in out["bars"]} - {None}, {"G"}, "такты из старого плана")
        played = self._played(out)
        self.assertTrue(played)
        for t, midi in played:
            self.assertEqual(midi, VOICING["G"], f"удар {t}")

    def test_parallel_requests_one_cache(self):
        jid, d = self._part_job()
        results, errors = [None, None], []
        start = threading.Barrier(2)

        def call(i):
            try:
                start.wait()
                results[i] = self._part(jid, 1)
            except Exception as e:   # исключение потока — в список, иначе тест его не увидит
                errors.append(e)

        threads = [threading.Thread(target=call, args=(i,)) for i in range(2)]
        for t in threads:
            t.start()
        for t in threads:
            t.join(120)
        self.assertEqual(errors, [])
        for i, r in enumerate(results):
            with self.subTest(request=i):
                self.assertIsNotNone(r)
                self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._played(results[0].json()), self._played(results[1].json()))
        json.loads(self._cache(d).read_text())   # кэш цел
        self.assertEqual(sorted(p.name for p in d.iterdir() if "guitar-part" in p.name), ["guitar-part.json"],
                         "остался временный файл кэша")


# ---------------------------------------------------------------- блок gtrnotes в /fx

def _gtr_blocks(proc):
    """Блоки gtrnotes, дошедшие до fx_engine.process (обёрнутого mock.wraps)."""
    return [b for c in proc.call_args_list for b in (c.args[2] if len(c.args) > 2 else c.kwargs["chain"])
            if b.get("type") == "gtrnotes"]


def _groups(blk):
    """Ноты блока → {t (округл.): отсортированные midi}."""
    out = {}
    for n in blk["notes"]:
        out.setdefault(round(float(n["t"]), 3), []).extend(int(m) for m in n["midi"])
    return {k: sorted(v) for k, v in out.items()}


class TestFxGtrnotes(_PartCase):

    WIN = {"from": 0.0, "to": 8.0}

    def _apply(self, jid, style=None, output="solo", **win):
        import fx_engine
        blk = {"type": "gtrnotes", "kit": "fake/notes"}
        if style is not None:
            blk["style"] = style
        with mock.patch.object(fx_engine, "process", wraps=fx_engine.process) as proc:
            r = self._fx(jid, source="guitar", chain=[blk], preview=True, output=output, **(win or self.WIN))
        return r, _gtr_blocks(proc)

    def test_block_style_arpeggio_one_note(self):
        jid, _ = self._part_job()
        r, blocks = self._apply(jid, style=2)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertTrue(blocks, "блок gtrnotes не дошёл до движка")
        g = _groups(blocks[0])
        self.assertTrue(g, "в блоке нет нот")
        for t, ms in g.items():
            self.assertEqual(len(ms), 1, f"перебор: в {t} — {ms}")

    def test_block_style_strum_chords(self):
        jid, _ = self._part_job()
        r, blocks = self._apply(jid, style=1)
        self.assertEqual(r.status_code, 200, r.text)
        g = _groups(blocks[0])
        self.assertTrue(g)
        for t, ms in g.items():
            self.assertIn(ms, list(VOICING.values()), f"бой: в {t} не аппликатура аккорда — {ms}")

    def test_block_notes_at_guitar_onsets(self):
        jid, _ = self._part_job()
        _, blocks = self._apply(jid, style=1)
        for t in _groups(blocks[0]):
            self.assertLessEqual(min(abs(t - p) for p in PLUCK_T), 0.03, f"нота в {t} не на ударе гитары")

    def test_block_without_style_is_auto(self):
        jid, _ = self._part_job()
        _, auto = self._apply(jid)
        # другой вывод — мимо кэша превью (иначе движок не позовут и нот не увидеть)
        _, zero = self._apply(jid, style=0, output="mix")
        self.assertTrue(auto and zero, "блок gtrnotes не дошёл до движка")
        self.assertEqual(_groups(auto[0]), _groups(zero[0]))

    def test_no_plan_422(self):
        jid, _ = self._part_job(plan=None)
        r, _ = self._apply(jid, style=1)
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("транскрипц", r.text.lower())

    def test_plan_without_chords_422(self):
        # план есть, но аккордов нет — нот нет → 422
        jid, _ = self._part_job(plan=PLAN_NO_CHORDS)
        r, _ = self._apply(jid, style=1)
        self.assertEqual(r.status_code, 422, r.text)

    def test_preview_follows_plan(self):
        jid, d = self._part_job()
        a, _ = self._apply(jid, style=1)
        again, _ = self._apply(jid, style=1)
        self.assertEqual(a.status_code, 200, a.text)
        self.assertEqual(again.json()["file"], a.json()["file"], "тот же запрос — не из кэша превью")
        (d / "score.abc").write_text(PLAN_G)
        _bump(d / "score.abc", 2_000_000_000)
        b, _ = self._apply(jid, style=1)
        self.assertEqual(b.status_code, 200, b.text)
        self.assertNotEqual(b.json()["file"], a.json()["file"], "план сменился, а превью старое")

    def test_no_gpu(self):
        jid, _ = self._part_job()
        r, _ = self._apply(jid, style=1)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.gpu_sections, [], "гитарист по аккордам взял очередь видеокарты")


# ---------------------------------------------------------------- распознавание удалено

class TestRecognitionRemoved(_PartCase):

    def test_no_guitar_notes_route(self):
        jid, _ = self._part_job()
        for method in ("post", "get"):
            with self.subTest(method=method):
                r = getattr(self.client, method)(f"/jobs/{jid}/guitar_notes")
                self.assertIn(r.status_code, (404, 405), r.text)
        paths = {getattr(r, "path", "") for r in self.w.app.routes}
        self.assertNotIn("/jobs/{job_id}/guitar_notes", paths)
        self.assertIn("/jobs/{job_id}/guitar_part", paths)

    def test_config_without_notes(self):
        cfg = self.client.get("/config").json()
        for k in ("notes_available", "notes_engines"):
            self.assertNotIn(k, cfg)


class TestRecognitionFilesRemoved(unittest.TestCase):

    def test_files_gone(self):
        for name in ("notes.py", "notes_run.py", "notes_install.sh", "requirements-notes.txt"):
            with self.subTest(file=name):
                self.assertFalse((WORKER / name).exists(), f"worker/{name} остался")

    def test_env_example_without_notes_dir(self):
        self.assertNotIn("PARTSCASTER_NOTES_DIR", (ROOT / "worker.env.example").read_text(encoding="utf-8"))

    def test_worker_tar_without_notes_install(self):
        self.assertNotIn("notes_install.sh", (ROOT / "tools" / "worker-tar.sh").read_text(encoding="utf-8"))


if __name__ == "__main__":
    unittest.main()
