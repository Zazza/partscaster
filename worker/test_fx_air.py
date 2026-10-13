"""Блок air движка fx_engine — «воздух»: достраивает верх выше 16 кГц, который YuE2 срезает.

Контракт:
- вход (n,) или (n, ch) → выход той же формы и длины;
- параметры tilt_db (−30…0, умолч. −10), only (0|1, умолч. 0); описание — worker/fx_blocks.json,
  вне диапазона — ChainError;
- вход с обрывом верха около 16 кГц: после блока энергия полосы 16–20 кГц на tilt_db ниже
  энергии полосы 12–16 кГц (±1,5 дБ);
- ниже 15 кГц звук не меняется: энергия разности ниже 15 кГц < −40 дБ от энергии входа там же;
- верх уже есть (полоса 16–20 кГц не ниже цели) — выход = вход;
- энергия полос — по каналам, не по моно-сумме: верх в противофазе не считается «отсутствующим»
  (пик выхода растёт не больше чем на 6 дБ — нет раздувания);
- воздух не громче полосы, из которой он сделан (8–11,5 кГц): звук только в 12,5–15,5 кГц →
  пик выхода не больше пика входа + 1 дБ;
- воздух только в 16–20 кГц: при 96 кГц добавка выше 20,5 кГц < −40 дБ от добавки в 16–20 кГц,
  пик выхода ≤ 1,5× пика входа; синус 9 кГц → добавка с пиком на 18 кГц (гармоника);
- тишина → тишина; sr ≤ 40 кГц (32000, 40000): only=0 — выход = вход, only=1 — нули;
- only=1 — только добавленный воздух: выход + вход ≈ результат only=0; без нужды — нули.

Энергия полос считается в тесте сама по себе (rfft по окнам Ханна, по каналам).
Внешних границ нет — моков нет.
"""
import unittest

try:
    import numpy as np
    from scipy import signal
    _HAS_DEPS = True
except ImportError:
    _HAS_DEPS = False

_SKIP = "нужны numpy/scipy (окружение воркера)"

SR = 48000
SECONDS = 3.0
WIN = 4096


def _fx():
    import fx_engine
    return fx_engine


def _air(**p):
    return [{"type": "air", **p}]


def _white(seconds=SECONDS, ch=0, sr=SR, seed=1, amp=0.1):
    """Белый шум float32 (float32 — чтобы «выход = вход» сравнивался без потерь на float32-выходе)."""
    rng = np.random.default_rng(seed)
    n = int(sr * seconds)
    x = amp * rng.standard_normal((n,) if ch == 0 else (n, ch))
    return x.astype(np.float32)


def _cut(seconds=SECONDS, ch=0, cutoff=15500.0, seed=1):
    """Шум, срезанный крутым фильтром нижних частот около 15,5 кГц (как верх у YuE2)."""
    x = _white(seconds, ch, seed=seed).astype(np.float64)
    sos = signal.butter(16, cutoff, btype="low", fs=SR, output="sos")
    y = signal.sosfiltfilt(sos, x, axis=0)
    return y.astype(np.float32)


def _chans(x):
    x = np.asarray(x, dtype=np.float64)
    return [x] if x.ndim == 1 else [x[:, c] for c in range(x.shape[1])]


def _band_energy(x, lo, hi, sr=SR):
    """Энергия полосы [lo, hi) Гц: сумма |rfft|² по окнам Ханна с шагом в пол-окна,
    по каждому каналу отдельно и сложенная (не по моно-сумме: противофаза не гасится)."""
    w = np.hanning(WIN)
    freqs = np.fft.rfftfreq(WIN, 1 / sr)
    sel = (freqs >= lo) & (freqs < hi)
    total = 0.0
    for m in _chans(x):
        for i in range(0, len(m) - WIN + 1, WIN // 2):
            spec = np.fft.rfft(m[i:i + WIN] * w)
            total += float(np.sum(np.abs(spec[sel]) ** 2))
    return total


def _bandpass(x, lo, hi, sr=SR):
    sos = signal.butter(8, [lo, hi], btype="band", fs=sr, output="sos")
    return signal.sosfiltfilt(sos, np.asarray(x, dtype=np.float64), axis=0)


def _sine(hz, amp, sr=SR, seconds=SECONDS):
    t = np.arange(int(sr * seconds)) / sr
    return amp * np.sin(2 * np.pi * hz * t)


def _db(a, b):
    return 10 * np.log10(max(a, 1e-30) / max(b, 1e-30))


def _air_tilt(y):
    """Наклон верха: энергия 16–20 кГц к энергии 12–16 кГц, дБ."""
    return _db(_band_energy(y, 16000, 20000), _band_energy(y, 12000, 16000))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class AirSpecTest(unittest.TestCase):

    def test_parse_defaults(self):
        norm = _fx().parse_chain(_air())[0]
        self.assertEqual(norm["type"], "air")
        self.assertAlmostEqual(norm["tilt_db"], -10)
        self.assertAlmostEqual(norm["only"], 0)

    def test_tilt_out_of_range_is_chain_error(self):
        fx = _fx()
        with self.assertRaises(fx.ChainError):
            fx.parse_chain(_air(tilt_db=-40))
        with self.assertRaises(fx.ChainError):
            fx.parse_chain(_air(tilt_db=5))

    def test_input_really_cut(self):
        # предусловие теста: у входа верха нет — на 10 дБ ниже самой низкой проверяемой
        # цели (−20), иначе проверки ниже ничего не доказывают
        self.assertLess(_air_tilt(_cut()), -30)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class AirProcessTest(unittest.TestCase):

    def test_shape_and_length(self):
        fx = _fx()
        for x in (_cut(ch=0), _cut(ch=2), _cut(ch=1)):
            y = fx.process(x, SR, _air())
            self.assertEqual(y.shape, x.shape)

    def test_tilt_restored(self):
        fx = _fx()
        for tilt in (-10, -20):
            for ch in (0, 2):
                with self.subTest(tilt=tilt, ch=ch):
                    y = fx.process(_cut(ch=ch), SR, _air(tilt_db=tilt))
                    self.assertAlmostEqual(_air_tilt(y), tilt, delta=1.5)

    def test_default_tilt_is_minus_10(self):
        y = _fx().process(_cut(), SR, _air())
        self.assertAlmostEqual(_air_tilt(y), -10, delta=1.5)

    def test_below_15k_unchanged(self):
        fx = _fx()
        for tilt in (-10, -20, 0):
            for ch in (0, 2):
                with self.subTest(tilt=tilt, ch=ch):
                    x = _cut(ch=ch)
                    y = fx.process(x, SR, _air(tilt_db=tilt))
                    xd = np.asarray(x, dtype=np.float64)
                    diff = np.asarray(y, dtype=np.float64) - xd
                    # по каждому каналу отдельно, чтобы противофаза не спряталась в среднем
                    chans = [(diff, xd)] if xd.ndim == 1 else [
                        (diff[:, c], xd[:, c]) for c in range(xd.shape[1])]
                    for d, xc in chans:
                        rel = _db(_band_energy(d, 0, 15000), _band_energy(xc, 0, 15000))
                        self.assertLess(rel, -40)

    def test_has_top_already_unchanged(self):
        fx = _fx()
        for ch in (0, 2):
            with self.subTest(ch=ch):
                x = _white(ch=ch)
                y = fx.process(x, SR, _air())
                np.testing.assert_array_equal(y, x)

    def test_silence_stays_silence(self):
        fx = _fx()
        for shape in ((int(SR * SECONDS),), (int(SR * SECONDS), 2)):
            x = np.zeros(shape, dtype=np.float32)
            y = fx.process(x, SR, _air())
            self.assertEqual(y.shape, x.shape)
            np.testing.assert_array_equal(y, x)

    def _low_sr_input(self, sr=32000):
        x = _white(sr=sr, ch=2)
        sos = signal.butter(16, 12000, btype="low", fs=sr, output="sos")
        return signal.sosfiltfilt(sos, x.astype(np.float64), axis=0).astype(np.float32)

    def test_low_sr_passthrough(self):
        x = self._low_sr_input()
        y = _fx().process(x, 32000, _air())
        np.testing.assert_array_equal(y, x)

    def test_low_sr_only_is_zeros(self):
        # «только воздух» поверх трека: вернуть вход значило бы удвоить звук
        x = self._low_sr_input()
        y = _fx().process(x, 32000, _air(only=1))
        self.assertEqual(y.shape, x.shape)
        np.testing.assert_array_equal(y, np.zeros_like(x))

    def test_sr_40000_boundary(self):
        # граница «неподходящей частоты» — sr ≤ 40000: ровно 40000 тоже без воздуха
        sr = 40000
        s = _sine(9000, 0.1, sr, seconds=1.0)
        x = np.stack([s, s], axis=1).astype(np.float32)
        fx = _fx()
        np.testing.assert_array_equal(fx.process(x, sr, _air()), x)
        y = fx.process(x, sr, _air(only=1))
        self.assertEqual(y.shape, x.shape)
        np.testing.assert_array_equal(y, np.zeros_like(x))

    def test_antiphase_top_not_louder(self):
        # верх 8–11 кГц в противофазе (левый = −правый) + общий шум 12,5–15,5 кГц:
        # в моно-сумме верх пропадает, а по каналам он есть — воздух не должен раздуться
        hf = _bandpass(_white(seed=2), 8000, 11000)
        common = _bandpass(_white(seed=3), 12500, 15500)
        x = np.stack([hf + common, -hf + common], axis=1).astype(np.float32)
        y = _fx().process(x, SR, _air())
        self.assertEqual(y.shape, x.shape)
        peak_in = float(np.max(np.abs(x)))
        peak_out = float(np.max(np.abs(y)))
        # «нет раздувания»: пик растёт не больше чем на 6 дБ
        self.assertLessEqual(peak_out, peak_in * 10 ** (6 / 20))

    def test_air_not_louder_than_source_band(self):
        # звук только в 12,5–15,5 кГц (оба канала одинаково, ниже 12 кГц пусто): воздух делается
        # из 8–11,5 кГц, а там пусто — пик выхода не больше пика входа + 1 дБ
        common = _bandpass(_white(seed=3), 12500, 15500)
        x = np.stack([common, common], axis=1).astype(np.float32)
        y = _fx().process(x, SR, _air())
        self.assertEqual(y.shape, x.shape)
        self.assertLessEqual(float(np.max(np.abs(y))), float(np.max(np.abs(x))) * 10 ** (1 / 20))

    def test_air_only_in_16_20k_at_96k(self):
        sr = 96000
        x = (_sine(9500, 0.1, sr) + _sine(14000, 0.01, sr)).astype(np.float32)
        y = _fx().process(x, sr, _air())
        self.assertEqual(y.shape, x.shape)
        self.assertLessEqual(float(np.max(np.abs(y))), 1.5 * float(np.max(np.abs(x))))
        add = np.asarray(y, dtype=np.float64) - x.astype(np.float64)
        e_air = _band_energy(add, 16000, 20000, sr)
        self.assertGreater(e_air, 0, "воздуха нет вовсе — проверка полосы ничего не доказывает")
        self.assertLess(_db(_band_energy(add, 20500, sr / 2 + 1, sr), e_air), -40)

    def test_harmonic_of_9k_at_18k(self):
        x = _sine(9000, 0.1)
        sos = signal.butter(16, 15500, btype="low", fs=SR, output="sos")
        x = signal.sosfiltfilt(sos, x).astype(np.float32)
        add = np.asarray(_fx().process(x, SR, _air(only=1)), dtype=np.float64)
        spec = np.abs(np.fft.rfft(add * np.hanning(len(add))))
        freqs = np.fft.rfftfreq(len(add), 1 / SR)
        self.assertGreater(float(spec.max()), 0, "добавки нет")
        self.assertAlmostEqual(float(freqs[int(np.argmax(spec))]), 18000, delta=100)

    def test_only_is_added_air(self):
        fx = _fx()
        for ch in (0, 2):
            with self.subTest(ch=ch):
                x = _cut(ch=ch)
                full = fx.process(x, SR, _air(tilt_db=-12))
                only = fx.process(x, SR, _air(tilt_db=-12, only=1))
                self.assertEqual(only.shape, x.shape)
                np.testing.assert_allclose(
                    only.astype(np.float64) + x.astype(np.float64),
                    full.astype(np.float64), atol=1e-5)
                # воздух действительно есть — не нули
                self.assertGreater(float(np.max(np.abs(only))), 0)

    def test_only_without_need_is_zeros(self):
        fx = _fx()
        x = _white(ch=2)
        y = fx.process(x, SR, _air(only=1))
        self.assertEqual(y.shape, x.shape)
        np.testing.assert_array_equal(y, np.zeros_like(x))


if __name__ == "__main__":
    unittest.main()
