"""Тесты партии гитариста по аккордам плана (worker/guitarpart.py) — по карточке internal-guitar-plan-chords
(условия 1–4, раздел «Устройство»), не по реализации.

Контракт:
- манеры AUTO=0, STRUM=1 (бой), ARPEGGIO=2 (перебор), BASS_CHORD=3 (бас + аккорд), POWER=4 (квинты);
  MIN_NOTE_S = 0.2 (ни глушёный удар, ни глушение ладонью не возникают из длины ноты);
- voicing(chord) → midi по возрастанию: открытые G C D Em Am E A Dm F (точные списки — в карточке), прочие
  «X»/«Xm» — баррэ: бас r ∈ [40, 51] (тоника), r+7, r+12, r+12+терция, r+19, r+24; регистр первой буквы любой,
  хвост (7, sus…) не учитывается; нераспознанное / None / "" → [];
- part(onsets, bars, style) → [[start, end, midi, vel]]: такт удара — start ≤ t < end; до первого такта — нот нет;
  после последнего — последний такт; такт с пустой аппликатурой — аккорд ближайшего прежнего такта с непустой;
  начало ноты = удар; конец = max(min(следующий удар, t + 2), t + MIN_NOTE_S) (перебор и бас — до конца такта,
  но не короче MIN_NOTE_S);
  бой — все ноты аппликатуры; перебор — одна нота на удар, индексы по кругу [0, 2, 3, 4, 3, 2], счёт с нуля
  с начала каждого такта; бас + аккорд — удар в ±¼ доли от долей 1 и 3 (и 5 — начало следующего такта) — одна
  нота: в первой половине такта тоника voicing[0], во второй — квинта; иначе верх аккорда (voicing[2:] у
  аппликатуры > 4 струн, иначе voicing[1:]); квинты — [r, r+7, r+12], r = voicing[0];
  AUTO по section: verse → перебор; chorus / pre-chorus / bridge → бой; прочее (intro, "") → бас + аккорд;
- onsets(guitar, sr) → моменты ударов (пик атаки) по дорожке гитары.

Предположения теста (контракт их не фиксирует, проверки от них не зависят):
- порядок нот в ответе part не задан — ноты группируются по началу; нота — список или кортеж;
- onsets принимает дорожку как её читает воркер (sf.read always_2d → стерео) и моно-массив.

Запуск: cd worker && python3 -m unittest test_guitarpart -v
"""
import unittest

try:
    import numpy as np
    _HAS_NP = True
except ImportError:
    _HAS_NP = False

try:
    import librosa  # noqa: F401
    _HAS_LIBROSA = _HAS_NP
except ImportError:
    _HAS_LIBROSA = False

_SKIP = "нужен numpy (окружение воркера)"
_SKIP_LR = "нужны numpy/librosa (окружение воркера)"


def _gp():
    # импорт внутри теста: отсутствие модуля роняет тесты, а не пропускает
    import guitarpart
    return guitarpart


OPEN = {"G": [43, 47, 50, 55, 59, 67], "C": [48, 52, 55, 60, 64], "D": [50, 57, 62, 66],
        "Em": [40, 47, 52, 55, 59, 64], "Am": [45, 52, 57, 60, 64], "E": [40, 47, 52, 56, 59, 64],
        "A": [45, 52, 57, 61, 64], "Dm": [50, 57, 62, 65], "F": [41, 48, 53, 57, 60, 65]}
PC = {"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}


def _barre(root_pc, minor):
    """Баррэ по контракту: бас — тоника в [40, 51]."""
    r = 40 + (root_pc - 4) % 12
    return [r, r + 7, r + 12, r + 12 + (3 if minor else 4), r + 19, r + 24]


def _bar(start, end, chord, section="verse"):
    return {"start": start, "end": end, "chord": chord, "section": section}


def _by_start(notes):
    """Ноты → {начало (округл.): (отсортированные midi, [концы])}."""
    out = {}
    for n in notes:
        s, e, m = float(n[0]), float(n[1]), int(round(float(n[2])))
        k = round(s, 4)
        ms, es = out.setdefault(k, ([], []))
        ms.append(m)
        es.append(e)
    return {k: (sorted(ms), es) for k, (ms, es) in out.items()}


# ---------------------------------------------------------------- voicing

@unittest.skipUnless(_HAS_NP, _SKIP)
class TestVoicing(unittest.TestCase):

    def test_open_chords(self):
        gp = _gp()
        for name, want in OPEN.items():
            with self.subTest(chord=name):
                self.assertEqual(list(gp.voicing(name)), want)

    def test_g_open_from_card(self):
        self.assertEqual(list(_gp().voicing("G")), [43, 47, 50, 55, 59, 67])

    def test_barre_examples_from_card(self):
        gp = _gp()
        cases = {"F#m": (6, True), "Bb": (10, False), "Db": (1, False), "Bbm": (10, True)}
        for name, (pc, minor) in cases.items():
            with self.subTest(chord=name):
                v = list(gp.voicing(name))
                self.assertEqual(v, _barre(pc, minor))
                self.assertTrue(40 <= v[0] <= 51, f"бас {v[0]} вне E2…D#3")
                self.assertEqual(v[0] % 12, pc, "в басе не тоника")

    def test_barre_triad_correct(self):
        # трезвучие баррэ верно: классы высот = тоника, терция (м/б), квинта
        gp = _gp()
        for name, (pc, minor) in {"F#m": (6, True), "Bb": (10, False), "Db": (1, False), "Bbm": (10, True),
                                  "Gm": (7, True), "B": (11, False), "Cm": (0, True), "Eb": (3, False)}.items():
            with self.subTest(chord=name):
                v = list(gp.voicing(name))
                want = {pc, (pc + (3 if minor else 4)) % 12, (pc + 7) % 12}
                self.assertEqual({m % 12 for m in v}, want)

    def test_all_non_open_roots_bass_in_range(self):
        gp = _gp()
        for letter, base in PC.items():
            for acc, d in (("", 0), ("#", 1), ("b", -1)):
                for q in ("", "m"):
                    name = letter + acc + q
                    if name in OPEN:
                        continue
                    with self.subTest(chord=name):
                        v = list(gp.voicing(name))
                        self.assertEqual(v, _barre((base + d) % 12, q == "m"))

    def test_sorted_ascending(self):
        gp = _gp()
        for name in list(OPEN) + ["F#m", "Bb", "Db", "Bbm", "C#", "Abm"]:
            with self.subTest(chord=name):
                v = list(gp.voicing(name))
                self.assertEqual(v, sorted(v))

    def test_lowercase_root(self):
        # регистр первой буквы любой: «fm» = Fm, «db» = Db (оба — баррэ)
        gp = _gp()
        self.assertEqual(list(gp.voicing("fm")), _barre(5, True))
        self.assertEqual(list(gp.voicing("db")), _barre(1, False))

    def test_tail_ignored(self):
        # хвост после «m»/мажора (7, sus…) не меняет аппликатуру
        gp = _gp()
        self.assertEqual(list(gp.voicing("F#m7")), list(gp.voicing("F#m")))
        self.assertEqual(list(gp.voicing("Bb7")), list(gp.voicing("Bb")))
        self.assertEqual(list(gp.voicing("Ebsus4")), list(gp.voicing("Eb")))

    def test_garbage_empty(self):
        gp = _gp()
        for bad in (None, "", "xyz", "H", "N.C.", "7", "#m", "  "):
            with self.subTest(chord=bad):
                self.assertEqual(list(gp.voicing(bad)), [])


# ---------------------------------------------------------------- part: общие правила

BEAT = 0.5             # такт 2 с — доля 0,5 с
G_BAR = _bar(1.0, 3.0, "G")
C_BAR = _bar(3.0, 5.0, "C")


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestPartCommon(unittest.TestCase):

    def test_constants(self):
        gp = _gp()
        self.assertEqual((gp.AUTO, gp.STRUM, gp.ARPEGGIO, gp.BASS_CHORD, gp.POWER), (0, 1, 2, 3, 4))
        self.assertAlmostEqual(gp.MIN_NOTE_S, 0.2)

    def test_note_format(self):
        gp = _gp()
        for style in (gp.STRUM, gp.ARPEGGIO, gp.BASS_CHORD, gp.POWER, gp.AUTO):
            with self.subTest(style=style):
                notes = gp.part([1.0, 1.5, 2.0], [G_BAR], style)
                self.assertTrue(notes)
                for n in notes:
                    self.assertEqual(len(n), 4)
                    s, e, m, v = n
                    self.assertGreater(e, s)
                    self.assertEqual(float(m), int(round(float(m))))
                    self.assertTrue(0.0 < float(v) <= 1.0, v)

    def test_empty_onsets(self):
        gp = _gp()
        for style in range(5):
            with self.subTest(style=style):
                self.assertEqual(list(gp.part([], [G_BAR], style)), [])

    def test_no_bars(self):
        gp = _gp()
        self.assertEqual(list(gp.part([1.0, 2.0], [], gp.STRUM)), [])

    def test_before_first_bar_silent(self):
        gp = _gp()
        for style in range(5):
            with self.subTest(style=style):
                got = _by_start(gp.part([0.2, 0.9, 1.0], [G_BAR], style))
                self.assertNotIn(0.2, got)
                self.assertNotIn(0.9, got)
                self.assertIn(1.0, got, "удар на начале такта — уже в такте")

    def test_after_last_bar_uses_last_bar(self):
        gp = _gp()
        got = _by_start(gp.part([1.0, 3.5], [G_BAR], gp.STRUM))
        self.assertEqual(got[3.5][0], OPEN["G"])

    def test_bar_boundary_start_inclusive_end_exclusive(self):
        # удар ровно в 3,0 — в такте C, а не G
        gp = _gp()
        got = _by_start(gp.part([2.5, 3.0], [G_BAR, C_BAR], gp.STRUM))
        self.assertEqual(got[2.5][0], OPEN["G"])
        self.assertEqual(got[3.0][0], OPEN["C"])

    def test_bar_without_chord_takes_previous(self):
        gp = _gp()
        bars = [G_BAR, _bar(3.0, 5.0, None), _bar(5.0, 7.0, "nonsense"), _bar(7.0, 9.0, "")]
        got = _by_start(gp.part([1.0, 3.0, 5.0, 7.0], bars, gp.STRUM))
        for t in (3.0, 5.0, 7.0):
            with self.subTest(t=t):
                self.assertEqual(got[t][0], OPEN["G"], "такт без аккорда — не аккорд прежнего такта")

    def test_previous_is_nearest_with_chord(self):
        gp = _gp()
        bars = [G_BAR, C_BAR, _bar(5.0, 7.0, None)]
        got = _by_start(gp.part([5.5], bars, gp.STRUM))
        self.assertEqual(got[5.5][0], OPEN["C"])

    def test_before_first_chord_silent(self):
        # такты есть, но аккорда до них не было — нот нет
        gp = _gp()
        bars = [_bar(1.0, 3.0, None), _bar(3.0, 5.0, "xyz"), _bar(5.0, 7.0, "C")]
        got = _by_start(gp.part([1.0, 2.0, 3.5, 5.0], bars, gp.STRUM))
        self.assertEqual(set(got), {5.0})

    def test_unsorted_onsets(self):
        gp = _gp()
        a = _by_start(gp.part([2.0, 1.0, 1.5], [G_BAR], gp.STRUM))
        b = _by_start(gp.part([1.0, 1.5, 2.0], [G_BAR], gp.STRUM))
        self.assertEqual({k: v[0] for k, v in a.items()}, {k: v[0] for k, v in b.items()})


# ---------------------------------------------------------------- манеры

@unittest.skipUnless(_HAS_NP, _SKIP)
class TestStrum(unittest.TestCase):

    def test_all_strings_every_strike(self):
        gp = _gp()
        ons = [1.0, 1.5, 2.0, 2.5, 3.0, 3.25]
        got = _by_start(gp.part(ons, [G_BAR, C_BAR], gp.STRUM))
        self.assertEqual(sorted(got), ons)
        for t in ons:
            with self.subTest(t=t):
                self.assertEqual(got[t][0], OPEN["G"] if t < 3.0 else OPEN["C"])

    def test_note_end_next_strike(self):
        gp = _gp()
        got = _by_start(gp.part([1.0, 1.5, 2.0], [G_BAR], gp.STRUM))
        for t, nxt in ((1.0, 1.5), (1.5, 2.0)):
            for e in got[t][1]:
                self.assertAlmostEqual(e, nxt, places=3)

    def test_note_end_capped_2s(self):
        gp = _gp()
        bars = [_bar(1.0, 9.0, "G")]
        got = _by_start(gp.part([1.0, 6.0], bars, gp.STRUM))
        for e in got[1.0][1]:
            self.assertAlmostEqual(e, 3.0, places=3)

    def test_end_not_shorter_than_min(self):
        gp = _gp()
        got = _by_start(gp.part([1.0, 1.05, 1.1], [G_BAR], gp.STRUM))
        for t, (_, ends) in got.items():
            for e in ends:
                self.assertGreaterEqual(e, t + gp.MIN_NOTE_S - 1e-9)


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestArpeggio(unittest.TestCase):

    def test_one_note_per_strike_cycle(self):
        # G — 6 струн: индексы [0, 2, 3, 4, 3, 2] по кругу
        gp = _gp()
        ons = [1.0 + 0.25 * i for i in range(8)]
        got = _by_start(gp.part(ons, [G_BAR], gp.ARPEGGIO))
        g = OPEN["G"]
        want = [g[i] for i in (0, 2, 3, 4, 3, 2, 0, 2)]
        self.assertEqual([got[round(t, 4)][0] for t in ons], [[m] for m in want])

    def test_new_bar_starts_with_bass(self):
        gp = _gp()
        ons = [1.0, 1.5, 2.0, 3.0, 3.5]
        got = _by_start(gp.part(ons, [G_BAR, C_BAR], gp.ARPEGGIO))
        self.assertEqual(got[3.0][0], [OPEN["C"][0]], "первый удар такта — не бас")
        self.assertEqual(got[3.5][0], [OPEN["C"][2]])

    def test_first_strike_late_in_bar_is_bass(self):
        # первый удар такта не на его начале — всё равно бас
        gp = _gp()
        got = _by_start(gp.part([1.0, 1.5, 3.4, 3.9], [G_BAR, C_BAR], gp.ARPEGGIO))
        self.assertEqual(got[3.4][0], [OPEN["C"][0]])
        self.assertEqual(got[3.9][0], [OPEN["C"][2]])

    def test_notes_in_voicing_short_chord(self):
        # D — 4 струны: каждая нота перебора из аппликатуры, первый — бас
        gp = _gp()
        ons = [1.0 + 0.25 * i for i in range(8)]
        got = _by_start(gp.part(ons, [_bar(1.0, 3.0, "D")], gp.ARPEGGIO))
        self.assertEqual(got[1.0][0], [OPEN["D"][0]])
        for t in ons:
            ms = got[round(t, 4)][0]
            self.assertEqual(len(ms), 1)
            self.assertIn(ms[0], OPEN["D"])

    def test_notes_ring_to_bar_end(self):
        gp = _gp()
        got = _by_start(gp.part([1.0, 1.5, 2.95], [G_BAR, C_BAR], gp.ARPEGGIO))
        self.assertAlmostEqual(got[1.0][1][0], 3.0, places=3)
        self.assertAlmostEqual(got[1.5][1][0], 3.0, places=3)
        # у конца такта — не короче MIN_NOTE_S
        self.assertGreaterEqual(got[2.95][1][0], 2.95 + gp.MIN_NOTE_S - 1e-9)


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestBassChord(unittest.TestCase):

    def test_strong_beats_bass_others_top(self):
        gp = _gp()
        g = OPEN["G"]
        ons = [1.0, 1.5, 2.05, 2.5]
        got = _by_start(gp.part(ons, [G_BAR, C_BAR], gp.BASS_CHORD))
        self.assertEqual(got[1.0][0], [g[0]], "доля 1 — тоника")
        self.assertEqual(got[2.05][0], [g[0] + 7], "доля 3 — квинта")
        for t in (1.5, 2.5):
            with self.subTest(t=t):
                self.assertEqual(got[t][0], g[2:], "слабая доля — верх аккорда")

    def test_strike_before_beat_5_is_fifth(self):
        # удар за 0,1 с до начала следующего такта (в ±¼ доли от доли 5) — одна нота; по контракту
        # «в первой половине такта — тоника, во второй — квинта» он во второй половине → квинта
        gp = _gp()
        g = OPEN["G"]
        got = _by_start(gp.part([1.0, 2.9], [G_BAR, C_BAR], gp.BASS_CHORD))
        self.assertEqual(got[2.9][0], [g[0] + 7])

    def test_quarter_beat_window(self):
        # ¼ доли = 0,125 с: 1,1 — ещё доля 1, 1,2 — уже нет
        gp = _gp()
        got = _by_start(gp.part([1.1, 1.2], [G_BAR], gp.BASS_CHORD))
        self.assertEqual(len(got[1.1][0]), 1)
        self.assertGreaterEqual(len(got[1.2][0]), 2)

    def test_four_string_chord_top(self):
        gp = _gp()
        d = OPEN["D"]
        got = _by_start(gp.part([1.0, 1.5], [_bar(1.0, 3.0, "D")], gp.BASS_CHORD))
        self.assertEqual(got[1.0][0], [d[0]])
        self.assertEqual(got[1.5][0], d[1:])

    def test_bass_in_voicing_and_root_or_fifth(self):
        gp = _gp()
        for chord in ("G", "C", "Am", "F#m", "Bb", "Dm"):
            with self.subTest(chord=chord):
                v = list(gp.voicing(chord))
                got = _by_start(gp.part([1.0, 2.0], [_bar(1.0, 3.0, chord)], gp.BASS_CHORD))
                self.assertEqual(got[1.0][0], [v[0]])
                (m,) = got[2.0][0]
                self.assertIn(m % 12, {v[0] % 12, (v[0] + 7) % 12})
                self.assertEqual((m - v[0]) % 12, 7, "во второй половине такта — квинта")


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestPower(unittest.TestCase):

    def test_root_fifth_octave_low(self):
        gp = _gp()
        for chord in list(OPEN) + ["F#m", "Bb", "B", "C#m", "Eb", "Ab"]:
            with self.subTest(chord=chord):
                got = _by_start(gp.part([1.0], [_bar(1.0, 3.0, chord)], gp.POWER))
                ms = got[1.0][0]
                self.assertEqual(len(ms), 3)
                r = ms[0]
                self.assertEqual([m - r for m in ms], [0, 7, 12])
                self.assertLessEqual(r, 52)
                self.assertEqual(r, list(gp.voicing(chord))[0])


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestAuto(unittest.TestCase):

    def _style_of(self, section, chord="G"):
        """Сыгранное в такте с section → как его сыграли бы манеры 1, 2, 3."""
        gp = _gp()
        bars = [_bar(1.0, 3.0, chord, section)]
        ons = [1.0, 1.25, 1.5, 2.0, 2.5]
        auto = _by_start(gp.part(ons, bars, gp.AUTO))
        return {s: _by_start(gp.part(ons, bars, s)) == auto for s in (gp.STRUM, gp.ARPEGGIO, gp.BASS_CHORD)}

    def test_verse_arpeggio(self):
        gp = _gp()
        self.assertEqual(self._style_of("verse"), {gp.STRUM: False, gp.ARPEGGIO: True, gp.BASS_CHORD: False})

    def test_chorus_like_strum(self):
        gp = _gp()
        for sec in ("chorus", "pre-chorus", "bridge"):
            with self.subTest(section=sec):
                self.assertEqual(self._style_of(sec), {gp.STRUM: True, gp.ARPEGGIO: False, gp.BASS_CHORD: False})

    def test_other_bass_chord(self):
        gp = _gp()
        for sec in ("intro", "outro", "", "solo"):
            with self.subTest(section=sec):
                self.assertEqual(self._style_of(sec), {gp.STRUM: False, gp.ARPEGGIO: False, gp.BASS_CHORD: True})

    def test_style_per_bar(self):
        # манера выбирается по части каждого такта
        gp = _gp()
        bars = [_bar(1.0, 3.0, "G", "verse"), _bar(3.0, 5.0, "C", "chorus")]
        got = _by_start(gp.part([1.0, 1.5, 3.0, 3.5], bars, gp.AUTO))
        self.assertEqual(len(got[1.5][0]), 1, "куплет — перебор")
        self.assertEqual(got[3.5][0], OPEN["C"], "припев — бой")


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestMinNoteNoMuted(unittest.TestCase):
    """Удары вплотную (80 мс): нота не короче 0,2 с, глушёных ударов (guitarnotes.strikes) нет."""

    def test_close_strikes_not_muted(self):
        import guitarnotes
        gp = _gp()
        ons = [1.0 + 0.08 * i for i in range(12)]
        beats = [1.0 + BEAT * i for i in range(5)]
        for style in range(5):
            with self.subTest(style=style):
                notes = gp.part(ons, [_bar(1.0, 3.0, "G", "chorus"), _bar(3.0, 5.0, "C", "verse")], style)
                self.assertTrue(notes)
                for n in notes:
                    self.assertGreaterEqual(float(n[1]) - float(n[0]), gp.MIN_NOTE_S - 1e-9, n)
                st = guitarnotes.strikes([list(n) for n in notes], beats)
                self.assertFalse([s for s in st if s["muted"]], "удар вплотную стал глушёным")


# ---------------------------------------------------------------- onsets

def _plucks(times, dur=4.0, sr=44100, hz=196.0, stereo=False):
    """Дорожка гитары: щипок (щелчок шума + затухающий тон, вход 2 мс) в каждый момент times."""
    n = int(dur * sr)
    x = np.zeros(n)
    rng = np.random.default_rng(1)
    k = np.arange(int(0.35 * sr))
    env = np.exp(-k / (0.08 * sr))
    att = np.minimum(1.0, k / (0.002 * sr))
    note = att * env * (np.sin(2 * np.pi * hz * k / sr) + 0.5 * np.sin(2 * np.pi * 2 * hz * k / sr))
    click = 0.3 * np.exp(-k / (0.002 * sr)) * rng.standard_normal(len(k))
    for t in times:
        p = int(t * sr)
        m = min(len(k), n - p)
        x[p:p + m] += 0.4 * (note + click)[:m]
    x = x.astype(np.float32)
    return np.stack([x, x], axis=1) if stereo else x


PLUCKS = [0.5, 1.13, 1.9, 2.4, 3.05]


@unittest.skipUnless(_HAS_LIBROSA, _SKIP_LR)
class TestOnsets(unittest.TestCase):

    def _check(self, got):
        got = [float(t) for t in got]
        for t in PLUCKS:
            with self.subTest(pluck=t):
                self.assertTrue(got and min(abs(g - t) for g in got) <= 0.03,
                                f"удар {t} не найден в пределах 30 мс: {got}")
        for g in got:
            with self.subTest(onset=g):
                self.assertLessEqual(min(abs(g - t) for t in PLUCKS), 0.05, f"лишний удар {g}: {got}")

    def test_mono(self):
        self._check(_gp().onsets(_plucks(PLUCKS), 44100))

    def test_stereo_as_read_by_worker(self):
        self._check(_gp().onsets(_plucks(PLUCKS, stereo=True), 44100))

    def test_sorted(self):
        got = [float(t) for t in _gp().onsets(_plucks(PLUCKS), 44100)]
        self.assertEqual(got, sorted(got))

    def test_silence_no_onsets(self):
        self.assertEqual(list(_gp().onsets(np.zeros(4 * 44100, dtype=np.float32), 44100)), [])


if __name__ == "__main__":
    unittest.main()


class TestNoPalmFromLength(unittest.TestCase):
    """Условие 4: одиночная низкая нота партии у конца такта (дробные моменты, округление) — не глушение ладонью,
    удар из нескольких нот — не глушёный (разбор ударов блока — guitarnotes.strikes)."""

    def test_bar_end_bass_not_palm(self):
        import guitarnotes
        gp = _gp()
        for style in (gp.BASS_CHORD, gp.ARPEGGIO):
            for k in range(200):
                b0 = 1.0 + k * 2.2187 / 7
                bars = [{"start": b0, "end": b0 + 2.2187, "chord": "Em", "section": ""}]
                t = b0 + 2.2187 - 0.1 - (k % 13) * 1e-4   # удар у конца такта: до конца меньше минимума ноты
                notes = gp.part([b0, t], bars, style)
                for st in guitarnotes.strikes(notes, []):
                    self.assertFalse(st["palm"], (style, k, st))
                    self.assertFalse(st["muted"], (style, k, st))

