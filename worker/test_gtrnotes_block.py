"""Тесты блока движка gtrnotes («Гитарист по аккордам») — по контракту карточек internal-guitar-notes и
internal-guitar-plan-chords (условия 2, 4, 5: параметр style, один «чук», громкость удара), не по реализации.

Контракт блока:
- {"type": "gtrnotes", kit (строка, обязательна), mute_kit (строка, по умолчанию ""),
  strum (0/1, целое, по умолчанию 1), spread_ms (2…25, по умолчанию 8), output_db (−24…12, 0),
  style (манера гитариста 0…4, целое, по умолчанию 0 — ноты по ней ставит воркер, движок style не читает)};
  служебные поля (ставит воркер): notes [{t, d, midi: [m], vel}] от начала окна, _t0, _until,
  _ref_rms, _tune (центы), _beats (от начала окна);
- нет нот → ChainError «нет нот гитары…»;
- выход — новая гитара по нотам (старая — вход — задаёт только громкость): сила удара —
  громкость входа 80 мс от начала к _ref_rms; громкость выхода по ходу — огибающая входа;
- сэмпл ноты — ближайший по высоте в наборе, сдвиг пересэмплированием + _tune; высота сэмпла —
  по имени файла: «a2_f_rr1», «eb3_vl2_rr3_1» (Karoryfer), «A#2_s2_soft_01» (FSBS), «A#4» (нейлон);
- strum=1: удар "down" — струны снизу вверх с разбегом spread_ms (тише — шире), "up" — сверху
  вниз и на 0,8 громкости; muted — «чук» сэмплами mute_kit (нет — обрывки нот 35 мс); palm — нота
  укорочена (спад ~80 мс) и темнее; strum=0 — ноты как распознаны;
- output_db — общий уровень выхода;
- internal-guitar-plan-chords: глушёный удар с набором mute_kit — один «чук» на удар (не по «чуку» на струну);
  громкость сэмпла набора — по самому громкому окну 50 мс (короткий «чук» не раздувается); громкость удара
  из n струн × √(4/n) — одна струна ≈ аккорд из четырёх;
- parse_chain принимает gtrnotes без нот (пресет хранит цепочку без нот — ноты кладёт воркер) и
  со служебными полями; notes у прочих блоков (кроме synth/perc/gtrnotes) — по-прежнему ошибка;
- описание блока — в worker/fx_blocks.json (поля выше).

Внешняя граница — хранилище наборов: подставное (kit(name) → (сэмплы, sr), kit_names(name) → имена
без расширения в том же порядке; нет — KeyError). Сэмплы — синусы на частоте из имени, частота набора
44100 при расчёте в 48000. Вход (старая гитара) — синус 6 кГц: в выход он попадать не должен
(блок заменяет гитару), а по спектру его легко отличить от нот.

Предположения теста:
- направление удара считается по _beats так же, как strikes() в guitarnotes (доли от начала окна);
- сэмплы набора глушения называются в том же формате, что ноты, и тоже выбираются по высоте;
- «темнее» у palm — меньше доля энергии выше 1,5 кГц у сэмпла с обертонами до 4,4 кГц;
- «одна струна ≈ аккорд» меряется по самому громкому окну 50 мс (RMS), а не по пику отсчёта: у синусов
  набора пик аккорда зависит от случайного сложения фаз (±3 дБ при верной √(4/n)), RMS — нет.

Запуск: cd worker && python3 -m unittest test_gtrnotes_block -v
"""
import json
import unittest
from pathlib import Path

try:
    import numpy as np
    import scipy  # noqa: F401
    _HAS_DEPS = True
except ImportError:
    _HAS_DEPS = False

_SKIP = "нужны numpy/scipy (окружение воркера)"

SR = 48000
KIT_SR = 44100
IN_HZ = 6000.0
IN_AMP = 0.2
REF = IN_AMP / np.sqrt(2) if _HAS_DEPS else 0.14
BLOCKS_JSON = Path(__file__).resolve().with_name("fx_blocks.json")


def _fx():
    import fx_engine
    return fx_engine


def _hz(m):
    return 440.0 * 2 ** ((m - 69) / 12)


def _sample(hz, sec=2.5, harmonics=1, decay=1.5, burst=None):
    """Сэмпл набора: синус hz (или обертоны 1/k до harmonics), спад exp(−t/decay), вход 2 мс.
    burst — короткий сэмпл этой длины (с), спад к концу."""
    if burst:
        sec = burst
    t = np.arange(int(sec * KIT_SR)) / KIT_SR
    x = np.zeros_like(t)
    for k in range(1, harmonics + 1):
        if k * hz < KIT_SR / 2 - 1000:
            x += np.sin(2 * np.pi * k * hz * t) / k
    x = 0.5 * x / max(1.0, np.abs(x).max() / 1.0) * np.exp(-t / decay)
    fi = int(0.002 * KIT_SR)
    x[:fi] *= np.linspace(0, 1, fi)
    fo = int(0.005 * KIT_SR)
    x[-fo:] *= np.linspace(1, 0, fo)
    return x.astype(np.float32)


# Karoryfer: строчные, бемоль «b»; шаг — малая терция (ближайший сэмпл, сдвиг ≤ 1,5 полутона)
KAR = [("e2_f_rr1", 40), ("a2_f_rr1", 45), ("c3_f_rr1", 48), ("eb3_f_rr1", 51), ("gb3_f_rr1", 54),
       ("a3_f_rr1", 57), ("c4_f_rr1", 60), ("eb4_f_rr1", 63), ("gb4_f_rr1", 66), ("a4_f_rr1", 69),
       ("c5_f_rr1", 72), ("e5_f_rr1", 76)]
KAR_VL = [(n.replace("_f_rr1", "_vl2_rr3_1"), m) for n, m in KAR]
# FSBS: прописные, диез «#», слой силы в имени
FSBS = [("E2_s2_soft_01", 40), ("A#2_s2_soft_01", 46), ("C#3_s2_soft_01", 49), ("E3_s2_soft_01", 52),
        ("G3_s2_soft_01", 55), ("A#3_s2_soft_01", 58), ("C#4_s2_soft_01", 61), ("E4_s2_soft_01", 64),
        ("G4_s2_soft_01", 67), ("A#4_s2_soft_01", 70), ("E5_s2_soft_01", 76)]
# нейлон FreePats: только нота
NYLON = [("E2", 40), ("A#2", 46), ("C#3", 49), ("E3", 52), ("G3", 55), ("A#3", 58), ("C#4", 61),
         ("E4", 64), ("G4", 67), ("A#4", 70), ("E5", 76)]
MUTE_HZ = 1500.0   # «чук» набора глушения — короткий тон вне диапазона нот теста


def _pad(x, sec):
    """Сэмпл, дополненный тишиной до sec секунд."""
    out = np.zeros(int(sec * KIT_SR), dtype=np.float32)
    out[:len(x)] = x
    return out


class GtrRes:
    """Хранилище наборов по контракту: kit(name) → (список сэмплов, sr), kit_names(name) → имена
    без расширения в том же порядке; нет набора — KeyError. Пишет, какие наборы просили."""

    def __init__(self):
        self.kits = {
            "kar/notes": [(n, _sample(_hz(m))) for n, m in KAR],
            "karvl/notes": [(n, _sample(_hz(m))) for n, m in KAR_VL],
            "fsbs/guitar": [(n, _sample(_hz(m))) for n, m in FSBS],
            "nylon/guitar": [(n, _sample(_hz(m))) for n, m in NYLON],
            "kar/harm": [(n, _sample(_hz(m), harmonics=40)) for n, m in KAR],
            "kar/mute": [(n, _sample(MUTE_HZ, burst=0.04)) for n, m in KAR],
            # глушение как в настоящих наборах (emilyguitar/mute: «muted1_rr1»…) — имена без высоты;
            # «чук» 40 мс, дальше в файле тишина (у записанного «чука» хвост файла тихий)
            "kar/chuck": [(f"muted1_rr{i}", _pad(_sample(MUTE_HZ, burst=0.04), 1.0)) for i in range(1, 5)],
            # тот же «чук» длинным сэмплом той же амплитуды: опора для «короткий не раздувается»
            "kar/chucklong": [(f"muted1_rr{i}", _sample(MUTE_HZ)) for i in range(1, 5)],
        }
        self.calls = []

    def kit(self, name):
        self.calls.append(name)
        items = self.kits[name]
        return [s for _, s in items], KIT_SR

    def kit_names(self, name):
        return [n for n, _ in self.kits[name]]


def _input(sec=4.0, amp=IN_AMP, amps=None):
    """Старая гитара (стерео): синус 6 кГц; amps — [(t0, t1, амплитуда)] по отрезкам."""
    t = np.arange(int(sec * SR)) / SR
    a = np.full_like(t, amp)
    for t0, t1, v in amps or []:
        a[int(t0 * SR):int(t1 * SR)] = v
    x = (a * np.sin(2 * np.pi * IN_HZ * t)).astype(np.float32)
    return np.stack([x, x], axis=1)


def _note(t, d, m, vel=0.8):
    return {"t": t, "d": d, "midi": [m], "vel": vel}


def _blk(notes, kit="kar/notes", **kw):
    b = {"type": "gtrnotes", "kit": kit, "notes": notes, "_ref_rms": REF, "_tune": 0.0}
    b.update(kw)
    return b


def _run(blk, x=None, res=None):
    if x is None:
        x = _input()
    return _fx().process(x, SR, [blk], res or GtrRes())


def _mono(y):
    y = np.asarray(y, dtype=np.float64)
    return y.mean(axis=1) if y.ndim == 2 else y


def _rms(y, t0, t1):
    seg = _mono(y)[int(t0 * SR):int(t1 * SR)]
    return float(np.sqrt(np.mean(seg ** 2))) if len(seg) else 0.0


def _f0(y, t0, t1, lo, hi):
    """Частота самого сильного пика спектра в [lo, hi] на [t0, t1) (Ханн, дополнение, парабола)."""
    x = _mono(y)[int(t0 * SR):int(t1 * SR)]
    x = x - x.mean()
    n = 1 << int(np.ceil(np.log2(len(x) * 8)))
    s = np.abs(np.fft.rfft(x * np.hanning(len(x)), n))
    f = np.fft.rfftfreq(n, 1 / SR)
    band = np.where((f >= lo) & (f <= hi))[0]
    k = band[np.argmax(s[band])]
    a, b, c = (np.log(s[k + i] + 1e-30) for i in (-1, 0, 1))
    p = 0.5 * (a - c) / (a - 2 * b + c)
    return (k + p) * SR / n


def _band_energy(y, t0, t1, lo, hi):
    x = _mono(y)[int(t0 * SR):int(t1 * SR)]
    s = np.abs(np.fft.rfft(x * np.hanning(len(x)))) ** 2
    f = np.fft.rfftfreq(len(x), 1 / SR)
    return float(s[(f >= lo) & (f < hi)].sum())


def _env(y, hz, smooth_s=0.005):
    """Огибающая тона hz: демодуляция и скользящее среднее (по центру — без сдвига)."""
    x = _mono(y)
    t = np.arange(len(x)) / SR
    z = x * np.exp(-2j * np.pi * hz * t)
    k = int(smooth_s * SR)
    return 2 * np.abs(np.convolve(z, np.ones(k) / k, mode="same"))


def _onset(y, hz, t0, t1, frac=0.5):
    """Момент, когда огибающая тона hz впервые достигает frac своего максимума на [t0, t1)."""
    e = _env(y, hz)[int(t0 * SR):int(t1 * SR)]
    i = int(np.argmax(e >= frac * e.max()))
    return t0 + i / SR


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestNoNotes(unittest.TestCase):

    def test_empty_notes_chain_error(self):
        fx = _fx()
        with self.assertRaises(fx.ChainError) as cm:
            _run(_blk([]))
        self.assertIn("нот", str(cm.exception))
        # распознавание удалено (условие 9): ошибка не отсылает к инструменту guitar_notes
        self.assertNotIn("guitar_notes", str(cm.exception))

    def test_missing_notes_chain_error(self):
        fx = _fx()
        blk = _blk([])
        del blk["notes"]
        fx.parse_chain([blk])   # опора: блок без нот разбирается (пресет), ошибка — именно при обработке
        with self.assertRaises(fx.ChainError) as cm:
            _run(blk)
        self.assertIn("нот", str(cm.exception))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestTiming(unittest.TestCase):

    def test_shape_same_as_input(self):
        x = _input()
        y = _run(_blk([_note(1.0, 1.0, 57)], strum=0), x=x)
        self.assertEqual(np.asarray(y).shape, x.shape)

    def test_note_sounds_at_its_time(self):
        t = 1.5
        y = _run(_blk([_note(t, 1.0, 57)], strum=0))
        body = _rms(y, t, t + 0.3)
        self.assertGreater(body, 1e-3, "нота не прозвучала")
        # до ноты — тишина (старая гитара в выход не попадает)
        self.assertLess(_rms(y, 0.0, t - 0.02), body * 0.01)
        a = np.abs(_mono(y))
        peak = a[int((t - 0.1) * SR):int((t + 0.5) * SR)].max()
        first = int(np.argmax(a >= 0.1 * peak)) / SR
        self.assertGreaterEqual(first, t - 0.005, "нота началась раньше своего времени")
        self.assertLessEqual(first, t + 0.02, "нота опоздала")

    def test_old_guitar_not_in_output(self):
        y = _run(_blk([_note(0.5, 3.0, 57)], strum=0))
        self.assertLess(_band_energy(y, 1.0, 3.0, IN_HZ - 50, IN_HZ + 50),
                        1e-4 * _band_energy(y, 1.0, 3.0, 100, 1000))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestPitch(unittest.TestCase):
    """Нота MIDI m → основной тон ≈ частоте m (ближайший сэмпл, сдвиг пересэмплированием)."""

    def _pitch(self, kit, m, **kw):
        y = _run(_blk([_note(0.5, 1.2, m)], kit=kit, strum=0, **kw))
        want = _hz(m) * 2 ** (kw.get("_tune", 0.0) / 1200)
        return _f0(y, 0.6, 1.5, want * 0.85, want * 1.15), want

    def _check(self, kit, midis):
        for m in midis:
            with self.subTest(kit=kit, midi=m):
                got, want = self._pitch(kit, m)
                cents = 1200 * np.log2(got / want)
                self.assertLess(abs(cents), 10, f"{got:.2f} Гц вместо {want:.2f}")

    def test_karoryfer_names(self):
        # 57, 60 — есть сэмпл; 59, 62, 64 — между сэмплами (сдвиг)
        self._check("kar/notes", (57, 59, 60, 62, 64))

    def test_karoryfer_velocity_layer_names(self):
        self._check("karvl/notes", (57, 59, 62))

    def test_fsbs_names(self):
        self._check("fsbs/guitar", (58, 59, 62, 64))

    def test_nylon_names(self):
        self._check("nylon/guitar", (58, 59, 62, 70))

    def test_tune_shifts_pitch(self):
        for tune in (50.0, -50.0):
            with self.subTest(tune=tune):
                got, want = self._pitch("kar/notes", 59, _tune=tune)
                self.assertLess(abs(1200 * np.log2(got / want)), 10, f"{got:.2f} Гц вместо {want:.2f}")
                self.assertGreater(abs(1200 * np.log2(got / _hz(59))), 35, "строй _tune не учтён")


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestLevel(unittest.TestCase):

    @staticmethod
    def _repeated(step=0.25, t0=0.25, t1=3.75, m=57):
        return [_note(round(t, 3), 0.2, m) for t in np.arange(t0, t1, step)]

    def test_follows_input_envelope(self):
        # старая гитара: тихо в первой половине, на 18 дБ громче во второй
        x = _input(amps=[(0.0, 2.0, 0.05), (2.0, 4.0, 0.4)])
        y = _run(_blk(self._repeated(), strum=0), x=x)
        quiet, loud = _rms(y, 1.0, 1.75), _rms(y, 3.0, 3.75)
        self.assertGreater(quiet, 1e-4, "тихая часть пропала совсем")
        self.assertGreater(20 * np.log10(loud / quiet), 6, "громкость выхода не следует за входом")

    def test_follows_input_envelope_reversed(self):
        x = _input(amps=[(0.0, 2.0, 0.4), (2.0, 4.0, 0.05)])
        y = _run(_blk(self._repeated(), strum=0), x=x)
        self.assertGreater(20 * np.log10(_rms(y, 0.5, 1.25) / _rms(y, 3.0, 3.75)), 6)

    def test_output_db(self):
        notes = self._repeated()
        base = _run(_blk(notes, strum=0))
        quiet = _run(_blk(notes, strum=0, output_db=-6))
        ratio = _rms(quiet, 0.5, 3.5) / _rms(base, 0.5, 3.5)
        self.assertAlmostEqual(20 * np.log10(ratio), -6.0, delta=0.5)


BEATS = [0.0, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0, 3.5]
LOW, HIGH = 45, 76   # 110 и 659 Гц — огибающие тонов разделяются


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestStrum(unittest.TestCase):

    @staticmethod
    def _strike(t, d=1.0):
        return [_note(t, d, LOW), _note(t + 0.002, d, HIGH)]

    def _lag(self, t, x=None, **kw):
        """Начало верхней струны минус начало нижней, с."""
        y = _run(_blk(self._strike(t), _beats=BEATS, spread_ms=25, **kw), x=x)
        lo = _onset(y, _hz(LOW), t - 0.05, t + 0.3)
        hi = _onset(y, _hz(HIGH), t - 0.05, t + 0.3)
        return hi - lo

    def test_down_low_string_first(self):
        self.assertGreater(self._lag(1.0, strum=1), 0.004)

    def test_up_high_string_first(self):
        # 1,23 — ближе к середине между долями (1,25), чем к доле
        self.assertLess(self._lag(1.23, strum=1), -0.004)

    def test_strum_default_on(self):
        self.assertGreater(self._lag(1.0), 0.004, "strum по умолчанию 1")

    def test_strum_off_notes_as_recognized(self):
        self.assertLess(abs(self._lag(1.0, strum=0)), 0.004)

    def test_quieter_strike_wider_spread(self):
        quiet = self._lag(1.0, x=_input(amp=0.05), strum=1)
        loud = self._lag(1.0, x=_input(amp=0.4), strum=1)
        self.assertGreater(quiet, loud + 0.001, f"разбег тихого {quiet * 1000:.1f} мс, громкого {loud * 1000:.1f} мс")

    def test_up_quieter_than_down(self):
        down = _run(_blk(self._strike(1.0), _beats=BEATS, strum=1))
        up = _run(_blk(self._strike(1.23), _beats=BEATS, strum=1))
        ratio = _rms(up, 1.23, 1.63) / _rms(down, 1.0, 1.4)
        self.assertGreater(ratio, 0.6)
        self.assertLess(ratio, 0.92, "удар вверх не тише удара вниз")


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestMutedAndPalm(unittest.TestCase):

    T = 1.0

    def _muted(self, **kw):
        notes = [_note(self.T, 0.06, 57), _note(self.T + 0.003, 0.06, 64)]
        return _run(_blk(notes, _beats=BEATS, strum=1, **kw))

    def test_muted_uses_mute_kit(self):
        res = GtrRes()
        y = _run(_blk([_note(self.T, 0.06, 57), _note(self.T + 0.003, 0.06, 64)],
                      _beats=BEATS, strum=1, mute_kit="kar/mute"), res=res)
        self.assertIn("kar/mute", res.calls, "набор глушения не запрошен")
        chuck = _band_energy(y, self.T - 0.01, self.T + 0.15, 1000, 2500)
        tones = _band_energy(y, self.T - 0.01, self.T + 0.15, 150, 400)
        self.assertGreater(chuck, 1e-9, "глушёный удар не прозвучал")
        self.assertGreater(chuck, tones, "вместо «чука» сэмплов глушения — ноты")

    def test_muted_without_mute_kit_short_fragments(self):
        y = self._muted()
        head = _rms(y, self.T, self.T + 0.035)
        self.assertGreater(head, 1e-4, "глушёный удар без набора глушения не прозвучал")
        self.assertLess(_rms(y, self.T + 0.08, self.T + 0.3), 0.1 * head, "обрывок длиннее ~35 мс")

    def _palm_pair(self):
        res = GtrRes()
        palm = _run(_blk([_note(self.T, 0.12, 45)], kit="kar/harm", _beats=BEATS, strum=1), res=res)
        norm = _run(_blk([_note(self.T, 0.2, 45)], kit="kar/harm", _beats=BEATS, strum=1), res=res)
        return palm, norm

    def test_palm_shorter(self):
        palm, norm = self._palm_pair()

        def tail(y):
            return _rms(y, self.T + 0.12, self.T + 0.19) / max(_rms(y, self.T, self.T + 0.05), 1e-12)

        self.assertLess(tail(palm), 0.5 * tail(norm), "глушение ладонью не укоротило ноту")

    def test_palm_darker(self):
        palm, norm = self._palm_pair()

        def bright(y):
            return (_band_energy(y, self.T, self.T + 0.1, 1500, 20000)
                    / max(_band_energy(y, self.T, self.T + 0.1, 50, 20000), 1e-30))

        self.assertLess(bright(palm), 0.7 * bright(norm), "глушение ладонью не темнее")

    def test_palm_only_with_strum(self):
        # strum=0 — нота как распознана: короткая низкая звучит без затемнения
        res = GtrRes()
        off = _run(_blk([_note(self.T, 0.12, 45)], kit="kar/harm", _beats=BEATS, strum=0), res=res)
        on = _run(_blk([_note(self.T, 0.12, 45)], kit="kar/harm", _beats=BEATS, strum=1), res=res)

        def bright(y):
            return (_band_energy(y, self.T, self.T + 0.1, 1500, 20000)
                    / max(_band_energy(y, self.T, self.T + 0.1, 50, 20000), 1e-30))

        self.assertLess(bright(on), bright(off))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestParseChain(unittest.TestCase):

    def test_defaults(self):
        b = _fx().parse_chain([{"type": "gtrnotes", "kit": "kar/notes"}])[0]
        self.assertEqual(b["type"], "gtrnotes")
        self.assertEqual(b["kit"], "kar/notes")
        self.assertEqual(b["mute_kit"], "")
        self.assertEqual(b["strum"], 1)
        self.assertEqual(b["spread_ms"], 8)
        self.assertEqual(b["output_db"], 0)
        self.assertEqual(b["style"], 0)

    def test_style_accepted(self):
        fx = _fx()
        for style in range(5):
            with self.subTest(style=style):
                b = fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "style": style}])[0]
                self.assertEqual(b["style"], style)

    def test_without_notes_accepted(self):
        # пресет хранит цепочку без нот (ноты кладёт воркер перед окном)
        self.assertEqual(len(_fx().parse_chain([{"type": "gtrnotes", "kit": "x/y"}, {"type": "gain"}])), 2)

    def test_service_fields_accepted(self):
        blk = {"type": "gtrnotes", "kit": "kar/notes", "mute_kit": "kar/mute", "strum": 0, "spread_ms": 25,
               "output_db": -24, "notes": [_note(0.1, 0.5, 57), _note(0.12, 0.5, 64, vel=0.3)],
               "_t0": 12.5, "_until": 3.0, "_ref_rms": 0.1, "_tune": -23.5, "_beats": [0.0, 0.5, 1.0]}
        b = _fx().parse_chain([blk])[0]
        self.assertEqual(b["strum"], 0)
        self.assertEqual(len(b["notes"]), 2)

    def test_kit_required(self):
        fx = _fx()
        fx.parse_chain([{"type": "gtrnotes", "kit": "k/p"}])   # опора: с набором — принимается
        with self.assertRaises(fx.ChainError):
            fx.parse_chain([{"type": "gtrnotes"}])

    def test_param_bounds(self):
        fx = _fx()
        fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "strum": 0, "spread_ms": 2, "output_db": 12}])
        for bad in ({"strum": 2}, {"strum": 0.5}, {"strum": -1}, {"spread_ms": 1}, {"spread_ms": 26},
                    {"output_db": -25}, {"output_db": 13}, {"kit": 5}, {"mute_kit": 5}, {"unknown": 1},
                    {"style": 5}, {"style": -1}, {"style": 0.5}):
            with self.subTest(bad=bad):
                with self.assertRaises(fx.ChainError):
                    fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", **bad}])

    def test_bad_notes_rejected(self):
        fx = _fx()
        fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "notes": [_note(0, 1, 60)]}])   # опора: верные ноты
        for notes in ([_note(0, 1, 200)], [_note(0, 0, 60)], "C4"):
            with self.subTest(notes=notes):
                with self.assertRaises(fx.ChainError):
                    fx.parse_chain([{"type": "gtrnotes", "kit": "k/p", "notes": notes}])

    def test_notes_still_rejected_on_other_blocks(self):
        fx = _fx()
        fx.parse_chain([{"type": "gain"}])
        for blk in ({"type": "gain", "notes": [_note(0, 1, 60)]},
                    {"type": "eq", "_beats": [0.0, 0.5]},
                    {"type": "gain", "_tune": 10.0}):
            with self.subTest(blk=blk):
                with self.assertRaises(fx.ChainError):
                    fx.parse_chain([blk])

    def test_in_spec(self):
        self.assertIn("gtrnotes", _fx().SPEC)


def _win_rms_max(y, t0, t1, win=0.05):
    """Самое громкое окно win (RMS) на [t0, t1): громкость удара без случайности сложения фаз."""
    x = _mono(y)[int(t0 * SR):int(t1 * SR)]
    k = int(win * SR)
    if len(x) < k:
        return 0.0
    c = np.concatenate([[0.0], np.cumsum(x ** 2)])
    return float(np.sqrt(np.max((c[k:] - c[:-k]) / k)))


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestStyleNotReadByEngine(unittest.TestCase):
    """style — манера гитариста: ноты по ней ставит воркер, звук блока при тех же нотах от style не зависит."""

    def test_same_notes_same_sound(self):
        notes = [_note(1.0, 0.4, 45), _note(1.002, 0.4, 52), _note(1.5, 0.4, 57)]
        base = np.asarray(_run(_blk(notes, _beats=BEATS)))
        for style in (1, 2, 3, 4):
            with self.subTest(style=style):
                y = np.asarray(_run(_blk(notes, _beats=BEATS, style=style)))
                self.assertEqual(y.shape, base.shape)
                self.assertLess(float(np.abs(y - base).max()), 1e-6)


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestOneChuck(unittest.TestCase):
    """Условие 4: глушёный удар — один «чук» набора, а не «чук» на каждую струну; громкость «чука» — по самому
    громкому окну 50 мс (короткий «чук» в длинном файле не раздувается)."""

    T4, T2 = 1.0, 1.6

    @staticmethod
    def _muted(t, strings):
        return [_note(t + 0.002 * i, 0.06, m) for i, m in enumerate(strings)]

    def _render(self, mute_kit, notes):
        return _run(_blk(notes, _beats=BEATS, strum=1, mute_kit=mute_kit))

    def test_four_strings_one_chuck(self):
        # в одном проходе: глушёный удар из 4 струн и из 2. «Чук» на струну дал бы 4 «чука» против 2 (+6 дБ);
        # один «чук» на удар — 4 струны не громче 2 (множитель удара √(4/n) их только сближает или тишит)
        notes = self._muted(self.T4, (52, 57, 62, 67)) + self._muted(self.T2, (52, 57))
        y = self._render("kar/chuck", notes)
        r4 = _win_rms_max(y, self.T4 - 0.01, self.T4 + 0.2)
        r2 = _win_rms_max(y, self.T2 - 0.01, self.T2 + 0.2)
        self.assertGreater(r2, 1e-5, "глушёный удар не прозвучал")
        self.assertLess(20 * np.log10(r4 / r2), 2.0, f"4 струны {r4:.4f}, 2 струны {r2:.4f}: «чук» на каждую струну?")

    def test_chuck_from_mute_kit(self):
        res = GtrRes()
        y = _run(_blk(self._muted(self.T4, (52, 57, 62, 67)), _beats=BEATS, strum=1, mute_kit="kar/chuck"), res=res)
        self.assertIn("kar/chuck", res.calls, "набор глушения не запрошен")
        chuck = _band_energy(y, self.T4 - 0.01, self.T4 + 0.15, 1000, 2500)
        tones = _band_energy(y, self.T4 - 0.01, self.T4 + 0.15, 100, 500)
        self.assertGreater(chuck, tones, "вместо «чука» набора — ноты")

    def test_short_chuck_not_inflated(self):
        # «чук» 40 мс в файле 1 с и длинный сэмпл той же амплитуды: пик короткого не выше длинного на > 6 дБ
        notes = self._muted(self.T4, (52, 57, 62, 67))
        short = self._render("kar/chuck", notes)
        long_ = self._render("kar/chucklong", notes)
        ps = float(np.abs(_mono(short)[int((self.T4 - 0.01) * SR):int((self.T4 + 0.3) * SR)]).max())
        pl = float(np.abs(_mono(long_)[int((self.T4 - 0.01) * SR):int((self.T4 + 0.3) * SR)]).max())
        self.assertGreater(pl, 1e-5, "опора: длинный «чук» прозвучал")
        self.assertLess(20 * np.log10(ps / pl), 6.0, f"короткий «чук» громче длинного: {ps:.4f} против {pl:.4f}")


@unittest.skipUnless(_HAS_DEPS, _SKIP)
class TestStrikeLevelByStrings(unittest.TestCase):
    """Условие 5: громкость удара не зависит от числа струн сильнее √(4/n) — одна струна ≈ аккорд из 4."""

    T = 1.0

    def _level(self, midis):
        notes = [_note(self.T + 0.002 * i, 0.6, m) for i, m in enumerate(midis)]
        y = _run(_blk(notes, _beats=BEATS, strum=1))
        return _win_rms_max(y, self.T - 0.01, self.T + 0.45)

    def test_single_note_vs_four_note_chord(self):
        one = self._level([57])
        four = self._level([45, 52, 57, 64])
        self.assertGreater(one, 1e-5)
        diff = 20 * np.log10(four / one)
        self.assertLess(abs(diff), 3.0, f"аккорд из 4 громче одной струны на {diff:.1f} дБ")

    def test_six_strings_not_much_louder(self):
        # шесть струн: √(4/6) — громче четырёх не более чем на ~1,8 дБ (с запасом — 3 дБ)
        four = self._level([45, 52, 57, 64])
        six = self._level([40, 45, 52, 57, 60, 64])
        self.assertLess(20 * np.log10(six / four), 3.0)


class TestBlocksJson(unittest.TestCase):
    """Описание блока для фронта и MCP — в worker/fx_blocks.json."""

    def setUp(self):
        blocks = json.loads(BLOCKS_JSON.read_text(encoding="utf-8"))
        self.assertIn("gtrnotes", blocks, "нет описания блока gtrnotes в fx_blocks.json")
        self.b = blocks["gtrnotes"]

    def test_label(self):
        lab = self.b.get("label") or {}
        self.assertTrue(lab.get("ru", "").strip() and lab.get("en", "").strip())

    def test_strings(self):
        s = {x["id"]: x for x in self.b.get("strings", [])}
        self.assertEqual(set(s), {"kit", "mute_kit"})
        self.assertTrue(s["kit"].get("required"))
        self.assertEqual(s["kit"].get("asset"), "kit")
        self.assertFalse(s["mute_kit"].get("required"))
        self.assertEqual(s["mute_kit"].get("default"), "")
        self.assertEqual(s["mute_kit"].get("asset"), "kit")

    def test_params(self):
        p = {x["id"]: x for x in self.b.get("params", [])}
        self.assertEqual(set(p), {"strum", "spread_ms", "output_db", "style", "double"})
        want = {"strum": (0, 1, 1), "spread_ms": (2, 25, 8), "output_db": (-24, 12, 0), "style": (0, 4, 0),
                "double": (0, 1, 0)}
        for k, (lo, hi, d) in want.items():
            with self.subTest(param=k):
                self.assertEqual((p[k]["min"], p[k]["max"], p[k]["default"]), (lo, hi, d))
        self.assertTrue(p["strum"].get("integer"))
        self.assertTrue(p["style"].get("integer"))
        self.assertEqual(p["style"].get("step"), 1)


if __name__ == "__main__":
    unittest.main()
