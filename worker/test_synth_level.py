"""Тесты громкости синт-партий на ухо (часть воркера; фронт — frontend/src/synthLevelDefault.test.js).

Контракт:
- громкость партии синта к треку (rel_db) меряется A-взвешенным RMS (кривая A, IEC 61672) и у
  трека, и у партии — на отсчётах, где звучат ноты партии; один множитель на всю партию (окно превью — тот же
  кусок полного рендера); тихий трек (A-RMS ниже −60 дБ) — пик партии −6 дБFS;
- блоки после synth в той же цепочке (eq, reverb, …) входят в замер: уровень на выходе цепочки =
  цель rel_db; замер детерминирован и не зависит от окна;
- умолчание rel_db у synth −8, у perc −14.

Точки входа — те же, что у test_fx_api.TestSynthRefLevel:
yue_worker.synth_window(chain, frm, win, track, sr) готовит партию к окну, fx_engine.process рендерит
цепочку; выход блока synth — сама партия (вход заменяется), блоки после него обрабатывают её.
A-взвешивание для проверки — своё (частотная характеристика A по IEC 61672 на FFT), не из кода воркера.

Толкования:
- «звучащие отсчёты» в проверке — середина партии (от 0,5 с после первой ноты до 0,5 с до конца последней):
  трек стационарный, поэтому A-RMS трека от точного набора отсчётов почти не зависит; атака/спад краёв
  в замер не попадают;
- эквалайзер «+8 дБ на 1–2,6 кГц» — полоса 1600 Гц, q 1 (ширина ≈ 1,6 кГц); партия — яркая
  (энергия в полосе есть, иначе eq ничего не меняет); rel_db −10 (не умолчание) — цель следует rel_db;
- «тихий трек» — и полная тишина (деление на ноль), и трек, у которого простой RMS выше −60 дБ,
  а A-RMS ниже (гул 30 Гц): порог — по A-уровню.
- зажим: трек — тот же «упор в низ», масштабированный к A-RMS −20 дБFS по своей кривой A; «пик ≤ 1,0» — максимум
  |отсчёта| выхода цепочки; сторож eq +8 — на этом же треке, rel_db −8, допуск ±1,5.
- «ресурсы открыты» — фабрика yue_worker.fx_resources вызвана хотя бы раз; «закрыты» — close() вызван
  столько же раз, сколько фабрика; ресурсы подменены заглушкой (внешняя граница — хранилище наборов на диске).
"""
import unittest
from unittest import mock

import test_pure as tp

try:
    import numpy as np
    import scipy.signal as sps
    import fx_engine as fx
    _HAS_DEPS = tp._HAS_WORKER_DEPS
except ImportError:   # окружение без numpy/scipy — пропуск
    _HAS_DEPS = False

_SKIP = "нужны numpy/scipy/fastapi (окружение воркера)"

SR = 44100
DUR = 40.0
T0, T1 = 1.0, 31.0          # партия звучит с 1 до 31 с; ноты ровные — отрезок замера воркера (20–30 с) не влияет
MEAS = (T0 + 0.5, T1 - 0.5)  # «звучащие отсчёты» проверки


def _a_gain(f):
    """Амплитудная характеристика A (IEC 61672), 0 дБ на 1 кГц."""
    f = np.asarray(f, dtype=np.float64)
    f2 = f * f
    ra = (12194.0 ** 2 * f2 * f2) / ((f2 + 20.6 ** 2) * np.sqrt((f2 + 107.7 ** 2) * (f2 + 737.9 ** 2))
                                    * (f2 + 12194.0 ** 2))
    return ra * 10 ** (2.0 / 20)


def _a_rms_db(x, t0, t1, sr=SR):
    """A-взвешенный RMS (дБ) отрезка [t0, t1): спектр × |A(f)| (нулевая фаза), среднее по каналам."""
    x = np.asarray(x, dtype=np.float64)
    if x.ndim == 1:
        x = x[:, None]
    seg = x[int(t0 * sr):int(t1 * sr)]
    n = len(seg)
    g = _a_gain(np.fft.rfftfreq(n, 1 / sr))[:, None]
    y = np.fft.irfft(np.fft.rfft(seg, axis=0) * g, n=n, axis=0)
    return 10 * np.log10(max(float(np.mean(y ** 2)), 1e-30))


def _rms_db(x, t0, t1, sr=SR):
    seg = np.asarray(x, dtype=np.float64)[int(t0 * sr):int(t1 * sr)]
    return 20 * np.log10(max(float(np.sqrt(np.mean(seg ** 2))), 1e-15))


def _track(seconds=DUR, sr=SR):
    """Трек с упором в низ: гул 60 Гц + шум ниже 250 Гц + немного белого шума (детерминированно)."""
    rng = np.random.default_rng(114)
    n = int(seconds * sr)
    t = np.arange(n) / sr
    low = sps.sosfilt(sps.butter(2, 250, "low", fs=sr, output="sos"), rng.standard_normal(n))
    low /= np.sqrt(np.mean(low ** 2))
    x = 0.12 * low + 0.15 * np.sin(2 * np.pi * 60 * t) + 0.005 * rng.standard_normal(n)
    return np.stack([x, x], axis=1).astype(np.float32)


def _notes(midi, t0=T0, t1=T1, step=2.0, vel=1.0):
    out, t = [], t0
    while t < t1 - 1e-9:
        out.append({"t": round(t, 6), "d": min(step, t1 - t), "midi": [midi], "vel": vel})
        t += step
    return out


DARK = {"type": "synth", "osc1": 4, "osc2": 4, "osc_mix": 0, "cutoff_hz": 600, "release_s": 0.1}
BRIGHT = {"type": "synth", "osc1": 0, "osc2": 1, "osc_mix": 0.5, "cutoff_hz": 8000, "release_s": 0.1}
DARK_MIDI, BRIGHT_MIDI = 60, 72                    # яркая — на октаву выше
EQ_UP = {"type": "eq", "bands": [{"freq_hz": 1600, "gain_db": 8, "q": 1}]}
REVERB = {"type": "reverb", "wet": 0.35}


def _render(chain, track, frm=0.0, win=None, sr=SR):
    """Рендер как у воркера: synth_window готовит партию (уровень от трека), process — цепочку на окне."""
    import yue_worker as w
    win = (len(track) / sr - frm) if win is None else win
    prepared = w.synth_window(chain, frm, win, track, sr)
    x = track[int(frm * sr):int((frm + win) * sr)]
    return np.asarray(fx.process(x, sr, prepared), dtype=np.float64)


def _part(block, midi, **extra):
    return dict(block, notes=_notes(midi), **extra)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestAWeightingSelf(unittest.TestCase):
    """Опора: своя кривая A верна (иначе тесты ниже ничего не доказывают)."""

    def test_curve_points(self):
        # табличные значения IEC 61672: 100 Гц −19,1; 1 кГц 0; 4 кГц +1,0; 31,5 Гц −39,4
        for f, want in ((100, -19.1), (1000, 0.0), (4000, 1.0), (31.5, -39.4)):
            self.assertAlmostEqual(20 * np.log10(float(_a_gain(f))), want, delta=0.15, msg=f)

    def test_sine_1k_rms(self):
        t = np.arange(SR * 2) / SR
        x = 0.1 * np.sin(2 * np.pi * 1000 * t)
        self.assertAlmostEqual(_a_rms_db(x, 0, 2), 20 * np.log10(0.1 / np.sqrt(2)), delta=0.05)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelByEar(unittest.TestCase):
    """Тёмная и яркая партии с rel_db −8 — обе на −8 ±1 дБ к треку по A-уровню."""

    @classmethod
    def setUpClass(cls):
        cls.track = _track()
        cls.ref_a = _a_rms_db(cls.track, *MEAS)
        cls.out = {name: _render([_part(blk, midi, rel_db=-8)], cls.track)
                   for name, blk, midi in (("dark", DARK, DARK_MIDI), ("bright", BRIGHT, BRIGHT_MIDI))}

    def test_track_is_bass_heavy(self):
        # опора: у трека простой RMS заметно выше A-уровня (упор в низ)
        self.assertGreater(_rms_db(self.track, *MEAS) - self.ref_a, 6)

    def test_dark_a_level_minus8(self):
        self.assertAlmostEqual(_a_rms_db(self.out["dark"], *MEAS) - self.ref_a, -8, delta=1.0)

    def test_bright_a_level_minus8(self):
        self.assertAlmostEqual(_a_rms_db(self.out["bright"], *MEAS) - self.ref_a, -8, delta=1.0)

    def test_plain_rms_bright_lower_than_dark(self):
        # мерится не простой RMS: у яркой партии простой RMS ниже, чем у тёмной, на ≥ 2 дБ
        dark, bright = (_rms_db(self.out[k], *MEAS) for k in ("dark", "bright"))
        self.assertGreaterEqual(dark - bright, 2.0, f"простой RMS: тёмная {dark:.1f}, яркая {bright:.1f}")


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelChainOutput(unittest.TestCase):
    """synth → eq +8 дБ (1–2,6 кГц) → reverb wet 0,35 — на выходе цепочки у цели rel_db."""

    REL = -10

    @classmethod
    def setUpClass(cls):
        cls.track = _track()
        cls.ref_a = _a_rms_db(cls.track, *MEAS)
        part = _part(BRIGHT, BRIGHT_MIDI, rel_db=cls.REL)
        cls.with_fx = _a_rms_db(_render([part, EQ_UP, REVERB], cls.track), *MEAS) - cls.ref_a
        cls.bare = _a_rms_db(_render([part], cls.track), *MEAS) - cls.ref_a

    def test_eq_reverb_chain_at_target(self):
        self.assertAlmostEqual(self.with_fx, self.REL, delta=1.5,
                               msg="eq +8 дБ после synth поднял партию над целью")

    def test_bare_part_at_target(self):
        self.assertAlmostEqual(self.bare, self.REL, delta=1.0)

    def test_difference_small(self):
        self.assertLessEqual(abs(self.with_fx - self.bare), 2.0)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelWindow(unittest.TestCase):
    """Окно [20, 30] с = тот же отрезок полного рендера [0, 40] — и с блоками после synth."""

    def _check(self, chain):
        track = _track()
        full = _render(chain, track)
        win = _render(chain, track, frm=20.0, win=10.0)
        self.assertEqual(len(win), 10 * SR)
        rw = _rms_db(win, 2.0, 8.0)              # 22–28 с трека: хвосты реверба до окна уже стихли
        rf = _rms_db(full, 22.0, 28.0)
        self.assertGreater(rf, -80, "партия не звучит — сравнение ничего не доказывает")
        self.assertAlmostEqual(rw, rf, delta=0.5)

    def _part(self):
        # тихие ноты в начале, громкие дальше: уровень «по окну» и «по всей партии» различались бы
        notes = _notes(BRIGHT_MIDI, 1.0, 15.0, vel=0.3) + _notes(BRIGHT_MIDI, 15.0, 38.0, vel=1.0)
        return dict(BRIGHT, notes=notes, rel_db=-8)

    def test_window_equals_full_bare(self):
        self._check([self._part()])

    def test_window_equals_full_with_post_blocks(self):
        self._check([self._part(), EQ_UP, REVERB])


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelDefault(unittest.TestCase):
    """synth без rel_db → цель −8 по A-уровню; fx_blocks: synth −8, perc −14."""

    def test_no_rel_db_target_minus8(self):
        track = _track()
        ref_a = _a_rms_db(track, *MEAS)
        part = _part(BRIGHT, BRIGHT_MIDI)
        self.assertNotIn("rel_db", part)
        y = _render([part], track)
        self.assertAlmostEqual(_a_rms_db(y, *MEAS) - ref_a, -8, delta=1.0)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelQuietTrack(unittest.TestCase):
    """Тихий трек (A-RMS < −60 дБ) — пик партии −6 дБFS ±0,5, без деления на ноль."""

    def _peak_db(self, track):
        y = _render([_part(BRIGHT, BRIGHT_MIDI, rel_db=-8)], track)
        self.assertTrue(np.all(np.isfinite(y)), "в выходе NaN/inf — деление на ноль")
        return 20 * np.log10(max(float(np.abs(y).max()), 1e-15))

    def test_silence(self):
        self.assertAlmostEqual(self._peak_db(np.zeros((int(DUR * SR), 2), dtype=np.float32)), -6, delta=0.5)

    def test_rumble_below_minus60_a(self):
        # гул 30 Гц: простой RMS −50 дБFS, A-уровень ≈ −90 дБ — трек «тихий на ухо»
        t = np.arange(int(DUR * SR)) / SR
        x = (10 ** (-50 / 20) * np.sqrt(2) * np.sin(2 * np.pi * 30 * t)).astype(np.float32)
        track = np.stack([x, x], axis=1)
        self.assertLess(_a_rms_db(track, *MEAS), -60)                 # опора
        self.assertGreater(_rms_db(track, *MEAS), -60)
        self.assertAlmostEqual(self._peak_db(track), -6, delta=0.5)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelCorrectionLimit(unittest.TestCase):
    """synth → gain −24 → gate (−20, range −80) на треке A ≈ −20 дБFS —
    поправка цепочки зажата: не громче rel_db + 12 (+1) и без клипа; synth → eq +8 у цели."""

    REL = -8

    @classmethod
    def setUpClass(cls):
        tr = _track().astype(np.float64)
        tr *= 10 ** ((-20.0 - _a_rms_db(tr, *MEAS)) / 20)
        cls.track = tr.astype(np.float32)
        cls.ref_a = _a_rms_db(cls.track, *MEAS)

    def test_track_a_level_minus20(self):
        self.assertAlmostEqual(self.ref_a, -20.0, delta=0.2)          # опора

    def _gated(self):
        part = _part(BRIGHT, BRIGHT_MIDI, rel_db=self.REL)
        chain = [part, {"type": "gain", "gain_db": -24},
                 {"type": "gate", "threshold_db": -20, "range_db": -80}]
        y = _render(chain, self.track)
        self.assertTrue(np.all(np.isfinite(y)), "в выходе NaN/inf")
        return y

    def test_gated_not_louder_than_limit(self):
        lvl = _a_rms_db(self._gated(), *MEAS) - self.ref_a
        self.assertLessEqual(lvl, self.REL + 12 + 1,
                             f"A-уровень выхода {lvl:.1f} дБ к треку — поправка цепочки не зажата +12 дБ")

    def test_gated_no_clip(self):
        peak = float(np.abs(self._gated()).max())
        self.assertLessEqual(peak, 1.0, f"пик выхода {peak:.2f} — клип")

    def test_eq_up_still_at_target(self):
        y = _render([_part(BRIGHT, BRIGHT_MIDI, rel_db=self.REL), EQ_UP], self.track)
        self.assertAlmostEqual(_a_rms_db(y, *MEAS) - self.ref_a, self.REL, delta=1.5)


class _StubResources:
    """Заглушка хранилища: набор из одного щелчка; считает открытия (фабрика), kit и close."""

    def __init__(self, counter):
        self.counter = counter
        counter["open"] += 1

    def kit(self, name):
        self.counter["kit"] += 1
        n = int(0.2 * SR)
        click = (np.exp(-np.arange(n) / (0.01 * SR))
                 * np.random.default_rng(120).standard_normal(n)).astype(np.float32) * 0.5
        return [click], SR

    def close(self):
        self.counter["close"] += 1


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestSynthLevelPostResources(unittest.TestCase):
    """synth → sampler(kit) — замер открывает хранилище и закрывает его, без ChainError;
    synth → reverb без ir — хранилище не открывается."""

    def _run(self, chain):
        import yue_worker as w
        counter = {"open": 0, "kit": 0, "close": 0}
        with mock.patch.object(w, "fx_resources", lambda: _StubResources(counter)):
            try:
                prepared = w.synth_window(chain, 0.0, DUR, _track(), SR)
            except fx.ChainError as e:
                self.fail(f"synth_window бросил ChainError: {e}")
        return prepared, counter

    def test_sampler_after_synth_opens_and_closes(self):
        chain = [_part(DARK, DARK_MIDI, rel_db=-8), {"type": "sampler", "kit": "stub/kick"}]
        prepared, c = self._run(chain)
        self.assertIsNotNone(prepared)
        self.assertGreaterEqual(c["open"], 1, "хранилище для sampler после synth не открыто")
        self.assertEqual(c["close"], c["open"], f"открыто {c['open']}, закрыто {c['close']}")

    def test_reverb_without_ir_no_resources(self):
        _, c = self._run([_part(DARK, DARK_MIDI, rel_db=-8), REVERB])
        self.assertEqual(c["open"], 0, "reverb без ir открыл хранилище")


if __name__ == "__main__":
    unittest.main()
