"""Тесты параметра double блока gtrnotes («Гитарист по аккордам»: второй дубль слева и справа) — по карточке
internal-guitar-double (условия 1–5, тест-кейсы ТК1–ТК7), не по реализации.

Контракт:
- double — целый 0/1, по умолчанию 0; в fx_blocks.json (подпись ru/en) и в SPEC движка; вне 0/1 или
  дробный — ChainError;
- double=0 — выход как сейчас (цепочка без ключа double), моно-гитара, размноженная на каналы;
- double=1 и вход ≥ 2 каналов: второй проход B той же партии — сэмпл со сдвигом seed (другой сэмпл того же
  слоя, если их в наборе > 1); каждый удар целиком сдвинут на ±GTR_DOUBLE_JITTER_MS (6 мс), сдвиг
  детерминирован от места удара в треке (_t0 + t); строй +GTR_DOUBLE_TUNE_CENTS (4 цента); разбег струн
  ×GTR_DOUBLE_SPREAD (1,4). Выход: L = h·(0,9·A + 0,1·B), R = h·(0,1·A + 0,9·B), h = 1/√(0,9² + 0,1²),
  A — первый проход как при double=0;
- double=1 и моно-вход — как double=0;
- RMS каждого канала при double=1 — в ±1,5 дБ от RMS канала при double=0 на той же партии.

Из формулы выхода A и B восстанавливаются точно: 0,9·L − 0,1·R = 0,8·h·A, 0,9·R − 0,1·L = 0,8·h·B.
На этом стоят проверки второго дубля (строй, разбег, сдвиг удара, другой сэмпл) — они смотрят на B отдельно.

Внешняя граница — хранилище наборов: подставное (GtrRes из test_gtrnotes_block, как в test_guitar_notes_fixes)
и набор с тремя «круговыми» сэмплами на высоту (rr1…rr3, разный тембр: только основной тон / +2-я гармоника /
+3-я гармоника) — по тембру видно, какой сэмпл сыграл. Константы движка (GTR_DOUBLE_*) подменяются только
в ТК2 «обнулённая часть B» — так велит карточка.

Предположения теста:
- ноты с одинаковым t — один удар (и при strum=0);
- имена «a3_f_rr2» — один слой (f) с круговыми сэмплами rr1…rr3;
- окно превью: ноты и _beats — от начала окна, _t0 = начало окна в треке, вход — тот же кусок старой гитары.

Запуск: cd worker && python3 -m unittest test_gtrnotes_double -v
"""
import json
import unittest
from pathlib import Path
from unittest import mock

import test_gtrnotes_block as gb

np = gb.np if gb._HAS_DEPS else None

SR = gb.SR
KIT_SR = gb.KIT_SR
BLOCKS_JSON = Path(__file__).resolve().with_name("fx_blocks.json")

H = 1.0 / (0.9 ** 2 + 0.1 ** 2) ** 0.5     # множитель дублей из условия 3
JITTER_MS = 6.0
TUNE_CENTS = 4.0
SPREAD = 1.4
LEVEL_DB = 1.5

# три круговых сэмпла на высоту: по тембру различимы
RR_HARM = {1: (1.0, 0.0, 0.0), 2: (1.0, 0.6, 0.0), 3: (1.0, 0.0, 0.6)}
RR_PITCHES = [("e2", 40), ("a2", 45), ("c3", 48), ("eb3", 51), ("gb3", 54), ("a3", 57), ("c4", 60),
              ("eb4", 63), ("gb4", 66), ("a4", 69), ("c5", 72), ("e5", 76)]


def _rr_sample(hz, harm, sec=2.5, decay=1.5):
    t = np.arange(int(sec * KIT_SR)) / KIT_SR
    x = sum(a * np.sin(2 * np.pi * (k + 1) * hz * t) for k, a in enumerate(harm) if a)
    x = 0.4 * x * np.exp(-t / decay)
    fi, fo = int(0.002 * KIT_SR), int(0.005 * KIT_SR)
    x[:fi] *= np.linspace(0, 1, fi)
    x[-fo:] *= np.linspace(1, 0, fo)
    return x.astype(np.float32)


class RrRes(gb.GtrRes):
    """GtrRes + набор «rr/notes»: на каждую высоту три сэмпла одного слоя."""

    def __init__(self):
        super().__init__()
        self.kits["rr/notes"] = [(f"{n}_f_rr{i}", _rr_sample(gb._hz(m), RR_HARM[i]))
                                 for n, m in RR_PITCHES for i in (1, 2, 3)]


def _run(blk, x=None, res=None):
    if x is None:
        x = gb._input()
    return np.asarray(gb._fx().process(x, SR, [blk], res or RrRes()), dtype=np.float64)


def _takes(y):
    """A и B из L, R по формуле условия 3."""
    L, R = y[:, 0], y[:, 1]
    return (0.9 * L - 0.1 * R) / (0.8 * H), (0.9 * R - 0.1 * L) / (0.8 * H)


def _seg(x, t0, t1):
    return x[int(t0 * SR):int(t1 * SR)]


def _rms_db(x):
    return 20 * np.log10(np.sqrt(np.mean(np.asarray(x, dtype=np.float64) ** 2)) + 1e-30)


def _chord(t, midis=(45, 52, 57, 64), d=0.4, vel=0.8):
    return [gb._note(t, d, m, vel) for m in midis]


def _strums(t0=0.5, t1=3.5, step=0.5, midis=(45, 52, 57, 64)):
    out = []
    for t in np.arange(t0, t1, step):
        out += _chord(round(float(t), 3), midis)
    return out


def _harm_profile(x, f0):
    """Доли энергии 2-й и 3-й гармоник к основному тону (Ханн, узкие полосы ±3 %)."""
    s = np.abs(np.fft.rfft(x * np.hanning(len(x)))) ** 2
    f = np.fft.rfftfreq(len(x), 1 / SR)

    def band(fc):
        return float(s[(f >= fc * 0.97) & (f <= fc * 1.03)].sum())

    base = band(f0) + 1e-30
    return band(2 * f0) / base, band(3 * f0) / base


def _shift_s(a, b, hz, t0, t1, max_s=0.02):
    """Сдвиг b относительно a (с) по взаимной корреляции огибающих тона hz на [t0, t1)."""
    ea = _seg(gb._env(a, hz), t0, t1)
    eb = _seg(gb._env(b, hz), t0, t1)
    ea, eb = ea - ea.mean(), eb - eb.mean()
    k = int(max_s * SR)
    best, lag = -np.inf, 0
    for d in range(-k, k + 1):
        if d >= 0:
            v = float(np.dot(ea[:len(ea) - d], eb[d:]))
        else:
            v = float(np.dot(ea[-d:], eb[:len(eb) + d]))
        if v > best:
            best, lag = v, d
    return lag / SR


# ---------------------------------------------------------------- ТК1, условие 2

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleOffSameAsNow(unittest.TestCase):

    def test_double0_equals_chain_without_key(self):
        notes = _strums()
        base = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS))
        off = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=0))
        self.assertEqual(off.shape, base.shape)
        self.assertTrue(np.array_equal(off, base), f"расхождение {float(np.abs(off - base).max()):.2e}")

    def test_double0_mono_copied_to_channels(self):
        y = _run(gb._blk(_strums(), kit="rr/notes", _beats=gb.BEATS, double=0))
        self.assertGreater(float(np.abs(y).max()), 1e-4, "опора: гитара прозвучала")
        self.assertTrue(np.array_equal(y[:, 0], y[:, 1]), "при double=0 каналы различаются")


# ---------------------------------------------------------------- ТК2, условие 3

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleStereo(unittest.TestCase):

    def setUp(self):
        self.notes = _strums()
        self.y0 = _run(gb._blk(self.notes, kit="rr/notes", _beats=gb.BEATS))
        self.y1 = _run(gb._blk(self.notes, kit="rr/notes", _beats=gb.BEATS, double=1))

    def test_shape_kept(self):
        self.assertEqual(self.y1.shape, self.y0.shape)

    def test_left_differs_from_right(self):
        L, R = self.y1[:, 0], self.y1[:, 1]
        self.assertGreater(float(np.abs(L - R).max()), 1e-3 * float(np.abs(L).max()), "L = R при double=1")

    def test_correlation_below_098(self):
        L, R = _seg(self.y1[:, 0], 0.3, 3.8), _seg(self.y1[:, 1], 0.3, 3.8)
        c = float(np.corrcoef(L, R)[0, 1])
        self.assertLess(c, 0.98, f"корреляция L и R {c:.4f}")

    def test_take_a_is_double0_pass(self):
        # A, восстановленный по формуле 0,9/0,1 и h, — тот же первый проход, что при double=0
        a, _ = _takes(self.y1)
        ref = self.y0[:, 0]
        err = float(np.abs(a - ref).max())
        self.assertLess(err, 1e-3 * float(np.abs(ref).max()), f"A ≠ проход double=0: {err:.2e}")


@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleZeroedB(unittest.TestCase):
    """ТК2, «обнулённая часть B»: один сэмпл на высоту, джиттер 0, строй 0, разбег ×1 — B = A, и тогда
    каналы различаются только примесью 0,1: L = R = h·(0,9 + 0,1)·A = h·A."""

    def test_b_equals_a_channels_equal(self):
        fx = gb._fx()
        notes = _strums()
        res = gb.GtrRes()
        base = _run(gb._blk(notes, _beats=gb.BEATS), res=res)[:, 0]
        with mock.patch.object(fx, "GTR_DOUBLE_JITTER_MS", 0), \
                mock.patch.object(fx, "GTR_DOUBLE_TUNE_CENTS", 0), \
                mock.patch.object(fx, "GTR_DOUBLE_SPREAD", 1.0):
            y = _run(gb._blk(notes, _beats=gb.BEATS, double=1), res=res)
        tol = 1e-3 * float(np.abs(base).max())
        self.assertGreater(float(np.abs(base).max()), 1e-4, "опора: гитара прозвучала")
        self.assertLess(float(np.abs(y[:, 0] - y[:, 1]).max()), tol, "при B = A каналы различаются")
        self.assertLess(float(np.abs(y[:, 0] - H * base).max()), tol, "L ≠ h·A при B = A")

    def test_constants_from_card(self):
        fx = gb._fx()
        self.assertEqual(float(fx.GTR_DOUBLE_JITTER_MS), JITTER_MS)
        self.assertEqual(float(fx.GTR_DOUBLE_TUNE_CENTS), TUNE_CENTS)
        self.assertAlmostEqual(float(fx.GTR_DOUBLE_SPREAD), SPREAD, places=6)


@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestSecondTake(unittest.TestCase):
    """Условие 3: чем второй проход отличается от первого (B восстановлен из L, R)."""

    def test_other_sample_same_layer(self):
        # три одиночные ноты в разных местах: у каждой B сыграл другой круговой сэмпл (другой тембр)
        times, m = (0.5, 1.5, 2.5), 57
        notes = [gb._note(t, 0.8, m) for t in times]
        y = _run(gb._blk(notes, kit="rr/notes", strum=0, double=1))
        a, b = _takes(y)
        f0 = gb._hz(m)
        for t in times:
            with self.subTest(t=t):
                pa = _harm_profile(_seg(a, t + 0.05, t + 0.7), f0)
                pb = _harm_profile(_seg(b, t + 0.05, t + 0.7), f0)
                self.assertGreater(max(abs(pa[0] - pb[0]), abs(pa[1] - pb[1])), 0.1,
                                   f"тот же сэмпл в обоих дублях: A {pa}, B {pb}")

    def test_tune_plus_4_cents(self):
        m = 57
        y = _run(gb._blk([gb._note(0.5, 2.0, m)], strum=0, double=1), res=gb.GtrRes())
        a, b = _takes(y)
        want = gb._hz(m)
        fa = gb._f0(a, 0.7, 2.2, want * 0.9, want * 1.1)
        fb = gb._f0(b, 0.7, 2.2, want * 0.9, want * 1.1)
        cents = 1200 * np.log2(fb / fa)
        self.assertAlmostEqual(cents, TUNE_CENTS, delta=1.5, msg=f"строй B к A {cents:.2f} цента")

    def test_spread_wider_x14(self):
        # удар вниз из двух струн: задержка верхней струны после нижней у B ≈ ×1,4 от A
        t = 1.0
        notes = [gb._note(t, 1.0, gb.LOW), gb._note(t, 1.0, gb.HIGH)]
        y = _run(gb._blk(notes, _beats=gb.BEATS, strum=1, spread_ms=20, double=1),
                 x=gb._input(amp=0.4), res=gb.GtrRes())
        a, b = _takes(y)

        def lag(z):
            lo = gb._onset(z, gb._hz(gb.LOW), t - 0.03, t + 0.3)
            hi = gb._onset(z, gb._hz(gb.HIGH), t - 0.03, t + 0.3)
            return hi - lo

        la, lb = lag(a), lag(b)
        self.assertGreater(la, 0.004, f"опора: у A есть разбег ({la * 1000:.1f} мс)")
        self.assertAlmostEqual(lb / la, SPREAD, delta=0.15,
                               msg=f"разбег A {la * 1000:.1f} мс, B {lb * 1000:.1f} мс")


# ---------------------------------------------------------------- ТК3, условие 4

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleMonoInput(unittest.TestCase):

    def test_mono_input_same_as_double0(self):
        x = gb._input()[:, 0].copy()
        notes = _strums()
        off = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=0), x=x)
        on = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=1), x=x)
        self.assertEqual(on.shape, x.shape)
        self.assertTrue(np.array_equal(on, off), f"расхождение {float(np.abs(on - off).max()):.2e}")


# ---------------------------------------------------------------- ТК4, условие 5

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleLevel(unittest.TestCase):

    def test_each_channel_rms_within_15db(self):
        notes = _strums()
        y0 = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS))
        y1 = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=1))
        ref = _rms_db(_seg(y0[:, 0], 0.3, 3.8))
        for ch in (0, 1):
            with self.subTest(channel=ch):
                d = _rms_db(_seg(y1[:, ch], 0.3, 3.8)) - ref
                self.assertLessEqual(abs(d), LEVEL_DB, f"канал {ch}: {d:+.2f} дБ к double=0")


# ---------------------------------------------------------------- ТК5, условие 3 (детерминизм, превью)

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleDeterminism(unittest.TestCase):

    T0, END = 10.0, 13.0

    def test_two_runs_identical(self):
        notes = _strums()
        y1 = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=1))
        y2 = _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, double=1))
        self.assertTrue(np.array_equal(y1, y2))

    def _full_and_window(self, double):
        # партия только внутри окна: хвосты нот до окна не мешают сравнению
        track_notes = _strums(10.5, 12.3, 0.3)
        beats = [round(0.5 * i, 3) for i in range(int(self.END * 2))]
        x = gb._input(sec=self.END)
        full = _run(gb._blk(track_notes, kit="rr/notes", _beats=beats, _t0=0.0, double=double), x=x)
        a = int(self.T0 * SR)
        win_notes = [dict(n, t=round(n["t"] - self.T0, 6)) for n in track_notes]
        win_beats = [round(b - self.T0, 6) for b in beats if b >= self.T0]
        win = _run(gb._blk(win_notes, kit="rr/notes", _beats=win_beats, _t0=self.T0, double=double),
                   x=x[a:].copy())
        return full[a:], win

    def _check(self, double):
        full, win = self._full_and_window(double)
        f, w = _seg(full, 0.3, 2.8), _seg(win, 0.3, 2.8)
        self.assertGreater(float(np.abs(f).max()), 1e-4, "опора: в окне звучит гитара")
        err = float(np.abs(f - w).max())
        self.assertLess(err, 1e-3 * float(np.abs(f).max()), f"окно ≠ то же место трека: {err:.2e}")

    def test_window_matches_full_double0(self):
        # опора: первый проход уже совпадает — иначе расхождение ниже не про дубль
        self._check(0)

    def test_window_matches_full_double1(self):
        self._check(1)


# ---------------------------------------------------------------- ТК6, условие 3 (сдвиг удара)

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestStrikeJitter(unittest.TestCase):
    """B играет тот же сэмпл (набор с одним сэмплом на высоту): сдвиг B к A по каждой струне удара."""

    MIDIS = (45, 64, 76)   # 110, 330, 659 Гц — огибающие разделяются

    def test_strike_shifted_as_whole_within_6ms(self):
        times = (0.5, 1.1, 1.7, 2.3, 2.9)
        notes = []
        for t in times:
            notes += [gb._note(t, 0.5, m) for m in self.MIDIS]
        y = _run(gb._blk(notes, strum=0, double=1), res=gb.GtrRes())
        a, b = _takes(y)
        shifts = []
        for t in times:
            per = [_shift_s(a, b, gb._hz(m), t - 0.03, t + 0.3) for m in self.MIDIS]
            with self.subTest(t=t):
                self.assertLess(max(per) - min(per), 0.0005,
                                f"струны удара сдвинуты по-разному: {[round(s * 1000, 2) for s in per]} мс")
                for s in per:
                    self.assertLessEqual(abs(s), (JITTER_MS + 0.5) / 1000, f"сдвиг {s * 1000:.2f} мс > 6 мс")
            shifts.append(float(np.median(per)))
        # джиттер есть: хоть один удар сдвинут заметно
        self.assertGreater(max(abs(s) for s in shifts), 0.0005, f"удары не сдвинуты: {shifts}")


# ---------------------------------------------------------------- ТК7, условие 1

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestParseDouble(unittest.TestCase):

    def test_default_zero(self):
        b = gb._fx().parse_chain([{"type": "gtrnotes", "kit": "k/p"}])[0]
        self.assertEqual(b["double"], 0)

    def test_accepts_0_and_1(self):
        fx = gb._fx()
        for v in (0, 1):
            with self.subTest(double=v):
                self.assertEqual(fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "double": v}])[0]["double"], v)

    def test_bad_values(self):
        fx = gb._fx()
        for v in (2, -1, 0.5, 1.5, "1"):
            with self.subTest(double=v):
                with self.assertRaises(fx.ChainError):
                    fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "double": v}])

    def test_in_spec(self):
        self.assertIn("double", json.dumps(gb._fx().SPEC["gtrnotes"], default=str))


class TestBlocksJsonDouble(unittest.TestCase):
    """Условие 1: double описан в fx_blocks.json (целый 0…1, по умолчанию 0, подпись ru/en)."""

    def test_param(self):
        blocks = json.loads(BLOCKS_JSON.read_text(encoding="utf-8"))
        p = {x["id"]: x for x in blocks["gtrnotes"].get("params", [])}
        self.assertIn("double", p)
        d = p["double"]
        self.assertEqual((d["min"], d["max"], d["default"]), (0, 1, 0))
        self.assertTrue(d.get("integer"))
        lab = d.get("label") or {}
        self.assertTrue(lab.get("ru", "").strip() and lab.get("en", "").strip(), f"подпись: {lab}")


# ---------------------------------------------------------------- ТК5б, условие 3 (аккорд до окна превью)

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestDoubleWindowRingingChord(unittest.TestCase):
    """Аккорд начат ДО окна (9,8 с, длина 1 с) и ещё звенит в окне [_t0 = 10 с]; следующий удар — 10,5 с.
    Ноты до окна воркер передаёт с отрицательным временем (gtrnotes_window: t − frm). Первые 0,5 с окна —
    как то же место полного трека, в обоих каналах."""

    T0, END = 10.0, 12.0

    def _full_and_window(self, double):
        track_notes = _chord(9.8, d=1.0) + _chord(10.5, d=0.4)
        beats = [round(0.5 * i, 3) for i in range(int(self.END * 2))]
        x = gb._input(sec=self.END)
        full = _run(gb._blk(track_notes, kit="rr/notes", _beats=beats, _t0=0.0, double=double), x=x)
        a = int(self.T0 * SR)
        win_notes = [dict(n, t=round(n["t"] - self.T0, 6)) for n in track_notes]
        win_beats = [round(b - self.T0, 6) for b in beats if -2.0 < b - self.T0 < self.END - self.T0 + 2.0]
        win = _run(gb._blk(win_notes, kit="rr/notes", _beats=win_beats, _t0=self.T0, double=double),
                   x=x[a:].copy())
        return full[a:], win

    def _check(self, double):
        full, win = self._full_and_window(double)
        for ch in (0, 1):
            with self.subTest(channel=ch):
                f, w = _seg(full[:, ch], 0.0, 0.5), _seg(win[:, ch], 0.0, 0.5)
                self.assertGreater(float(np.abs(f).max()), 1e-4, "опора: аккорд до окна звенит в окне")
                err = float(np.abs(f - w).max())
                self.assertLess(err, 1e-3 * float(np.abs(f).max()),
                                f"канал {ch}: начало окна ≠ то же место трека: {err:.2e}")

    def test_window_start_matches_full_double0(self):
        # опора: первый проход доигрывает аккорд до окна так же, как полный трек
        self._check(0)

    def test_window_start_matches_full_double1(self):
        self._check(1)


# ---------------------------------------------------------------- ТК9, условие 3 (другой сэмпл слоя)

def _rr_id(x, f0):
    """Какой круговой сэмпл rr1…rr3 сыграл — ближайший профиль гармоник (2-я, 3-я к основному тону)."""
    p = _harm_profile(x, f0)
    want = {i: (h[1] ** 2, h[2] ** 2) for i, h in RR_HARM.items()}
    return min(want, key=lambda i: (p[0] - want[i][0]) ** 2 + (p[1] - want[i][1]) ** 2), p


@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestSecondTakeOtherSample(unittest.TestCase):
    """Слой из трёх сэмплов (rr1…rr3, различимы по тембру): у КАЖДОЙ ноты B сыграл не тот сэмпл, что A."""

    def test_each_note_other_sample(self):
        m, d = 57, 0.6
        times = (0.501, 1.25, 2.0, 2.75, 3.5, 4.25, 5.003, 5.75)
        notes = [gb._note(t, d, m) for t in times]
        y = _run(gb._blk(notes, kit="rr/notes", strum=0, double=1), x=gb._input(sec=6.5))
        a, b = _takes(y)
        f0 = gb._hz(m)
        for t in times:
            with self.subTest(t=t):
                ia, pa = _rr_id(_seg(a, t + 0.05, t + 0.55), f0)
                ib, pb = _rr_id(_seg(b, t + 0.05, t + 0.55), f0)
                self.assertNotEqual(ia, ib, f"тот же сэмпл rr{ia} в обоих дублях: A {pa}, B {pb}")


# ---------------------------------------------------------------- ТК10, условие 3 (тот же слой силы)

# слои mp и mf по два сэмпла: слой виден по гармонике (mp — 2-я, mf — 3-я), сэмпл в слое — по её доле
LAYER_HARM = {("mp", 1): (1.0, 0.4, 0.0), ("mp", 2): (1.0, 0.8, 0.0),
              ("mf", 1): (1.0, 0.0, 0.4), ("mf", 2): (1.0, 0.0, 0.8)}


class LayerRes(gb.GtrRes):
    """GtrRes + набор «lay/notes»: a3 в двух слоях силы (mp, mf), по два круговых сэмпла в слое."""

    def __init__(self):
        super().__init__()
        self.kits["lay/notes"] = [(f"a3_{lay}_rr{i}", _rr_sample(gb._hz(57), h))
                                  for (lay, i), h in LAYER_HARM.items()]


def _layer_of(x, f0):
    h2, h3 = _harm_profile(x, f0)
    return ("mp" if h2 > h3 else "mf"), (h2, h3)


@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestSecondTakeSameLayer(unittest.TestCase):
    """Вход молчит до 0,5 с, потом звучит; нота в 0,5 с. Слой силы B — тот же, что у A. Громкость входа
    перебирается около средней (_ref_rms) — там проходит граница слоёв mp/mf, и там сдвиг удара B на
    миллисекунды раньше начала звука не должен перекидывать его в другой слой."""

    def test_b_layer_equals_a_layer(self):
        m, t = 57, 0.5
        f0 = gb._hz(m)
        seen = set()
        for k in range(0, 11):
            db = 0.1 * k
            with self.subTest(input_db=db):
                x = gb._input(sec=2.0, amp=gb.IN_AMP * 10 ** (db / 20), amps=[(0.0, 0.5, 0.0)])
                y = _run(gb._blk([gb._note(t, 1.0, m)], kit="lay/notes", strum=0, double=1),
                         x=x, res=LayerRes())
                a, b = _takes(y)
                la, pa = _layer_of(_seg(a, t + 0.05, t + 0.9), f0)
                lb, pb = _layer_of(_seg(b, t + 0.05, t + 0.9), f0)
                seen.add(la)
                self.assertEqual(lb, la, f"слой B {lb} ≠ слой A {la}: A {pa}, B {pb}")
        self.assertIn("mf", seen, "опора: при громком входе A играет слой mf")


# ---------------------------------------------------------------- ТК9б, условие 3 (другой сэмпл слоя, бой)

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestSecondTakeOtherSampleStrum(unittest.TestCase):
    """Игра боем (strum=1), аккорды из 4 струн, набор rr/notes (три сэмпла слоя на высоту), разные spread_ms:
    у КАЖДОЙ струны каждого аккорда B сыграл не тот сэмпл, что A.

    Струны аккорда звучат одновременно и по тембру не разделяются (2-я гармоника a2 = основной тон a3), поэтому
    тест наблюдает за выбором: обёртка над fx_engine._gtr_pick (настоящая функция вызывается, результат не
    меняется) записывает, какую высоту просили и какой объект-сэмпл вернулся. Порядок вызовов: сначала весь
    проход A, затем весь проход B — проверяется опорой: первые N вызовов при double=1 совпадают с N вызовами
    при double=0 (A = проход double=0, условие 3), и B просит те же высоты в том же порядке."""

    SPREADS = (4, 5, 6, 10, 12)

    def _calls(self, notes, spread, double):
        fx = gb._fx()
        real = fx._gtr_pick
        log = []

        def spy(kit, midi, layer, seed):
            out = real(kit, midi, layer, seed)
            log.append((round(float(midi), 3), out[0]))
            return out

        with mock.patch.object(fx, "_gtr_pick", spy):
            _run(gb._blk(notes, kit="rr/notes", _beats=gb.BEATS, strum=1, spread_ms=spread, double=double))
        return log

    def test_each_string_other_sample(self):
        notes = _strums()
        n_strings = len(notes)
        for spread in self.SPREADS:
            with self.subTest(spread_ms=spread):
                c0 = self._calls(notes, spread, 0)
                c1 = self._calls(notes, spread, 1)
                n = len(c0)
                self.assertEqual(n, n_strings, f"опора: каждая струна каждого аккорда — один выбор сэмпла ({n})")
                self.assertEqual(len(c1), 2 * n, "опора: при double=1 сэмпл выбирается дважды на струну")
                a, b = c1[:n], c1[n:]
                self.assertEqual([m for m, _ in a], [m for m, _ in c0], "опора: первые вызовы — проход A")
                self.assertTrue(all(np.array_equal(x, y) for (_, x), (_, y) in zip(a, c0, strict=True)),
                                "опора: A выбрал те же сэмплы, что проход double=0")
                self.assertEqual([m for m, _ in b], [m for m, _ in a], "опора: B играет те же струны")
                same = [(i, m) for i, ((m, sa), (_, sb)) in enumerate(zip(a, b, strict=True)) if sa is sb]
                self.assertEqual(same, [], f"тот же сэмпл у A и B (номер струны в партии, высота): {same}")


if __name__ == "__main__":
    unittest.main()
