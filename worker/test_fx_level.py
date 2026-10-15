"""Блок level движка fx_engine — «Громкость по линии» (карточка internal-live-drummer, условие 4).

Контракт:
- поле points [{t, db}]: t — секунды ТРЕКА (t ≥ 0), db −30…+12, по возрастанию t, до 200 точек;
- между точками — линейно в дБ; до первой точки — её дБ, после последней — её дБ;
- служебный _t0 (начало куска в треке, с) ставит воркер — как у tremolo: отсчёт i куска
  звучит в момент трека _t0 + i/sr; без _t0 — от 0;
- пустые/неверные points (нет поля, [], > 200 точек, db вне −30…+12, t < 0, убывающие t) — ChainError;
- fx_blocks.json: блок level описан во всех трёх копиях (воркер, фронт, internal/mcp).

Внешних границ нет — моков нет.
"""
import json
import os
import unittest

try:
    import numpy as np
    import scipy  # noqa: F401
    _HAS_DEPS = True
except ImportError:
    _HAS_DEPS = False

_SKIP = "нужны numpy/scipy (окружение воркера)"

SR = 48000
HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.dirname(HERE)


def _fx():
    import fx_engine
    return fx_engine


def _ones(seconds=4.0, ch=2, amp=0.5):
    """Постоянный сигнал: выход/вход по отсчётам = множитель блока."""
    n = int(round(SR * seconds))
    s = np.full(n, amp, dtype=np.float64)
    return s if ch == 0 else np.repeat(s[:, None], ch, axis=1)


def _lvl(points, **extra):
    return [{"type": "level", "points": points, **extra}]


def _pts(*pairs):
    return [{"t": t, "db": db} for t, db in pairs]


def _g(db):
    return 10 ** (db / 20)


def _at(t):
    return int(round(t * SR))


# линия: 0 дБ до 1 с, к +6 на 2 с, к −6 на 3 с, дальше −6
LINE = _pts((1.0, 0.0), (2.0, 6.0), (3.0, -6.0))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class LevelSpecTest(unittest.TestCase):

    def test_valid_points_parse(self):
        norm = _fx().parse_chain(_lvl(LINE))[0]
        self.assertEqual(norm["type"], "level")

    def test_single_point_parses(self):
        _fx().parse_chain(_lvl(_pts((0.0, 4.5))))

    def test_range_edges_ok(self):
        _fx().parse_chain(_lvl(_pts((0.0, -30.0), (1.0, 12.0))))

    def test_200_points_ok(self):
        pts = _pts(*[(i * 0.1, 0.0) for i in range(200)])
        _fx().parse_chain(_lvl(pts))

    def test_bad_points_are_chain_error(self):
        fx = _fx()
        cases = {
            "нет поля points": [{"type": "level"}],
            "пусто": _lvl([]),
            "201 точка": _lvl(_pts(*[(i * 0.1, 0.0) for i in range(201)])),
            "db ниже −30": _lvl(_pts((0.0, -30.5))),
            "db выше +12": _lvl(_pts((0.0, 12.5))),
            "t < 0": _lvl(_pts((-0.1, 0.0), (1.0, 0.0))),
            "убывающие t": _lvl(_pts((2.0, 0.0), (1.0, 3.0))),
            "points не список": _lvl({"t": 0, "db": 0}),
            "db не число": _lvl([{"t": 0.0, "db": "громко"}]),
            "нет db": _lvl([{"t": 0.0}]),
        }
        for name, chain in cases.items():
            with self.subTest(name):
                with self.assertRaises(fx.ChainError):
                    fx.parse_chain(chain)
                with self.assertRaises(fx.ChainError):
                    fx.process(_ones(0.5), SR, chain)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class LevelSoundTest(unittest.TestCase):

    def _gain(self, chain, seconds=4.0, ch=2):
        x = _ones(seconds, ch)
        y = _fx().process(x, SR, chain)
        self.assertEqual(y.shape, x.shape)
        self.assertTrue(np.all(np.isfinite(y)))
        return (y / x) if ch == 0 else (y[:, 0] / x[:, 0])

    def test_line_values_mid_segments_and_edges(self):
        g = self._gain(_lvl(LINE))
        for t, db in ((0.0, 0.0), (0.5, 0.0),          # до первой точки — её дБ
                      (1.0, 0.0), (1.5, 3.0),          # середина отрезка 0 → +6
                      (2.0, 6.0), (2.25, 3.0), (2.5, 0.0), (2.75, -3.0),   # +6 → −6
                      (3.0, -6.0), (3.5, -6.0), (3.99, -6.0)):              # после последней — её дБ
            with self.subTest(t=t):
                self.assertAlmostEqual(g[_at(t)], _g(db), delta=_g(db) * 2e-3)

    def test_linear_in_db_not_in_amplitude(self):
        # середина отрезка 0 → +6 дБ — ровно +3 дБ (1,413), а не среднее амплитуд (1,498)
        g = self._gain(_lvl(LINE))
        self.assertAlmostEqual(20 * np.log10(g[_at(1.5)]), 3.0, delta=0.02)

    def test_single_point_constant(self):
        g = self._gain(_lvl(_pts((2.0, 4.5))))
        np.testing.assert_allclose(g, _g(4.5), rtol=1e-3)

    def test_zero_db_output_equals_input(self):
        x = _ones(1.0)
        y = _fx().process(x, SR, _lvl(_pts((0.0, 0.0), (0.5, 0.0))))
        np.testing.assert_allclose(y, x, rtol=0, atol=1e-6)

    def test_mono_and_stereo_same_gain(self):
        for ch in (0, 1, 2):
            with self.subTest(ch=ch):
                g = self._gain(_lvl(LINE), ch=ch)
                self.assertAlmostEqual(g[_at(1.5)], _g(3.0), delta=_g(3.0) * 2e-3)

    def test_both_channels_same(self):
        x = np.random.default_rng(1).standard_normal((SR * 2, 2)) * 0.1
        y = _fx().process(x, SR, _lvl(_pts((0.0, -6.0), (2.0, 6.0))))
        np.testing.assert_allclose(y[:, 0] / x[:, 0], y[:, 1] / x[:, 1], rtol=1e-6)

    def test_length_equals_input(self):
        fx = _fx()
        for n in (1, 777, SR + 13):
            x = np.random.default_rng(n).standard_normal((n, 2)) * 0.1
            with self.subTest(n=n):
                y = fx.process(x, SR, _lvl(LINE))
                self.assertEqual(y.shape, x.shape)


T0, T1 = 1.25, 2.75   # кусок поперёк обеих наклонных частей линии


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class LevelTrackTimeTest(unittest.TestCase):
    """Точки — секунды трека: кусок с _t0 = тот же отрезок обработки целого сигнала."""

    def test_chunk_with_t0_equals_slice_of_full(self):
        fx = _fx()
        x = _ones(4.0)
        a, b = _at(T0), _at(T1)
        full = fx.process(x, SR, _lvl(LINE))
        chunk = fx.process(x[a:b].copy(), SR, _lvl(LINE, _t0=T0))
        self.assertEqual(chunk.shape, (b - a, 2))
        np.testing.assert_allclose(chunk, full[a:b], rtol=0, atol=1e-5)

    def test_t0_shifts_line(self):
        # кусок с _t0 = 1,5: его первый отсчёт — момент трека 1,5 с (+3 дБ)
        fx = _fx()
        x = _ones(1.0)
        y = fx.process(x, SR, _lvl(LINE, _t0=1.5))
        self.assertAlmostEqual(y[0, 0] / x[0, 0], _g(3.0), delta=_g(3.0) * 2e-3)
        # через 0,5 с куска — момент трека 2,0 с (+6 дБ)
        self.assertAlmostEqual(y[_at(0.5), 0] / x[0, 0], _g(6.0), delta=_g(6.0) * 2e-3)

    def test_without_t0_from_zero(self):
        fx = _fx()
        x = _ones(4.0)
        a, b = _at(T0), _at(T1)
        full = fx.process(x, SR, _lvl(LINE))
        chunk0 = fx.process(x[a:b].copy(), SR, _lvl(LINE))
        explicit0 = fx.process(x[a:b].copy(), SR, _lvl(LINE, _t0=0))
        np.testing.assert_allclose(chunk0, explicit0, rtol=0, atol=1e-9)
        # без _t0 кусок начинается с 0 с трека — с отрезком целого он не совпадает
        self.assertGreater(float(np.abs(chunk0 - full[a:b]).max()), 1e-2)
        np.testing.assert_allclose(chunk0, full[:b - a], rtol=0, atol=1e-5)


try:
    import yue_worker as _w  # noqa: F401
    _W_OK, _W_SKIP = True, ""
except Exception as e:  # окружение без зависимостей воркера
    _W_OK, _W_SKIP = False, f"нет окружения воркера: {e}"


@unittest.skipUnless(_W_OK, _W_SKIP)
class LevelWorkerWindowTest(unittest.TestCase):
    """Воркер ставит блоку level _t0 = начало окна (как tremolo); присланный _t0 не берётся."""

    def test_window_sets_t0(self):
        import yue_worker as w
        out = w.synth_window(_lvl(LINE), 20.0, 10.0)
        self.assertEqual(out[0]["type"], "level")
        self.assertAlmostEqual(out[0]["_t0"], 20.0)
        self.assertEqual(out[0]["points"], LINE)

    def test_window_overrides_sent_t0(self):
        import yue_worker as w
        out = w.synth_window(_lvl(LINE, _t0=99.0), 3.5, 2.0)
        self.assertAlmostEqual(out[0]["_t0"], 3.5)

    def test_full_render_window_t0_zero(self):
        import yue_worker as w
        out = w.synth_window(_lvl(LINE, _t0=7.3), 0.0, 40.0)
        self.assertAlmostEqual(out[0]["_t0"], 0.0)


try:
    import test_fx_api as _fa
    _API_OK, _API_SKIP = _fa._OK, _fa._SKIP
    _ApiBase = _fa._FxApiCase
except Exception as e:  # окружение без fastapi/httpx — пропуск, как у test_fx_api
    _API_OK, _API_SKIP = False, f"нет окружения воркера для API: {e}"
    _ApiBase = unittest.TestCase


@unittest.skipUnless(_API_OK, _API_SKIP)
class LevelPreviewMatchesRenderTest(_ApiBase):
    """Превью окна [1,25; 2,75) (source mix, output mix) = тот же отрезок полного /fx-рендера ±1e-4."""

    def _long_job(self):
        import soundfile as sf
        sr = _fa.SR
        jid = self._job(duration=4.0, semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        t = np.arange(int(4.0 * sr)) / sr
        left = 0.2 * np.sin(2 * np.pi * 440 * t)
        right = 0.2 * np.sin(2 * np.pi * 660 * t)
        sf.write(str(d / "audio.flac"), np.stack([left, right], axis=1).astype(np.float32), sr)
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid, d

    def test_preview_equals_slice_of_render(self):
        jid, d = self._long_job()
        full = self._ok(jid, source="mix", output="mix", chain=_lvl(LINE))
        y, sr = _fa._read(d / full["file"])
        a, b = int(round(T0 * sr)), int(round(T1 * sr))
        want = y[a:b]
        for t0 in (None, 0, 123.4):
            chain = _lvl(LINE) if t0 is None else _lvl(LINE, _t0=t0)
            with self.subTest(t0=t0):
                r = self._fx(jid, source="mix", output="mix", chain=chain, preview=True,
                             **{"from": T0, "to": T1})
                self.assertEqual(r.status_code, 200, r.text)
                got, _ = _fa._read(d / r.json()["file"])
                self.assertEqual(got.shape, want.shape)
                np.testing.assert_allclose(got, want, rtol=0, atol=1e-4)

    def test_bad_points_422(self):
        jid, _ = self._long_job()
        r = self._fx(jid, source="mix", output="mix", chain=_lvl([]))
        self.assertEqual(r.status_code, 422, r.text)


class LevelBlocksJsonTest(unittest.TestCase):
    """Блок level описан во всех трёх копиях fx_blocks.json (копии совпадают побайтно) и в таблице блоков
    docs/effects.md (строка «| `level` |»)."""

    COPIES = ("worker/fx_blocks.json", "frontend/src/fxBlocks.json", "internal/mcp/fx_blocks.json")

    def test_level_in_all_copies(self):
        for rel in self.COPIES:
            with self.subTest(rel):
                with open(os.path.join(ROOT, rel), encoding="utf-8") as f:
                    blocks = json.load(f)
                self.assertIn("level", blocks)
                lbl = blocks["level"].get("label", {})
                self.assertTrue(lbl.get("ru") and lbl.get("en"), f"{rel}: у level нет подписи ru/en")

    def test_effects_doc_row(self):
        with open(os.path.join(ROOT, "docs", "effects.md"), encoding="utf-8") as f:
            self.assertIn("| `level` |", f.read())

    def test_copies_identical(self):
        with open(os.path.join(ROOT, self.COPIES[0]), "rb") as f:
            base = f.read()
        for rel in self.COPIES[1:]:
            with self.subTest(rel):
                with open(os.path.join(ROOT, rel), "rb") as f:
                    self.assertEqual(f.read(), base)


if __name__ == "__main__":
    unittest.main()
