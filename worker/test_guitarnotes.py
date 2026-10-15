"""Тесты чистой логики нот гитары (worker/guitarnotes.py) — по контракту карточки
internal-guitar-notes, не по реализации.

Контракт:
- нота — список [start, end, midi, vel] (секунды трека, vel 0…1);
- pick_centered(pass0, pass1, seg): ноты двух проходов распознавания (второй сдвинут на seg/2);
  нота берётся из прохода, где её начало ближе к середине своего куска (кусок прохода k
  начинается в k·seg/2 + i·seg); равенство — проход 0;
- union(main, second, tol=0.05): все ноты main + ноты second, у которых в main нет ноты той же
  высоты с началом ближе tol;
- clean(notes): без «лесенок» (≥4 нот короче 0,06 с подряд, шаг высоты 1–3 полутона, начала
  соседних ближе 0,03 с — выбрасываются все ноты лесенки) и без дублей (та же высота, начала
  ближе 0,03 с → одна нота: начало первой, конец — наибольший, сила — наибольшая);
- strikes(notes, beats): удары — ноты с началами ближе 0,03 с к первой ноте группы; поля
  t (начало первой), notes (по высоте снизу вверх), dir "single" | "down" | "up" (down — начало
  ближе к доле, чем к середине между долями; долей нет — down), muted (≥2 нот и все короче
  0,1 с), palm (1 нота, высота < 52 и короче 0,15 с).

Предположения теста (контракт их не фиксирует, проверки от них не зависят):
- порядок нот в ответе pick_centered/union/clean не задан — сравнение по множеству или после
  сортировки по (начало, высота); ноты в ответе — списки или кортежи;
- ноты проходов сравниваются только парами (одна и та же нота в обоих проходах): что делать с
  нотой, которой нет во втором проходе, контракт не говорит;
- кусок с отрицательным i (проход 1 до seg/2) не проверяется — не определён.

Запуск: cd worker && python3 -m unittest test_guitarnotes -v
"""
import unittest

try:
    import numpy  # noqa: F401
    _HAS_NP = True
except ImportError:
    _HAS_NP = False

_SKIP = "нужен numpy (окружение воркера)"


def _gn():
    import guitarnotes
    return guitarnotes


def _norm(notes):
    """Ноты → отсортированный список кортежей с округлением (сравнение без порядка)."""
    return sorted((round(float(s), 6), round(float(e), 6), int(round(m)), round(float(v), 6))
                  for s, e, m, v in notes)


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestPickCentered(unittest.TestCase):
    """seg = 10: середины кусков прохода 0 — 5, 15, 25…; прохода 1 — 10, 20…"""

    SEG = 10.0

    def test_note_near_pass0_center_from_pass0(self):
        p0 = [[5.0, 6.0, 60, 0.1]]
        p1 = [[5.02, 6.1, 60, 0.9]]
        self.assertEqual(_norm(_gn().pick_centered(p0, p1, self.SEG)), _norm(p0))

    def test_note_near_pass1_center_from_pass1(self):
        # 10.0 — край куска прохода 0 (до середины 5 с) и середина куска прохода 1
        p0 = [[10.0, 11.0, 62, 0.1]]
        p1 = [[10.03, 11.2, 62, 0.9]]
        self.assertEqual(_norm(_gn().pick_centered(p0, p1, self.SEG)), _norm(p1))

    def test_alternating_regions(self):
        # одни и те же ноты в обоих проходах; сила помечает проход: 0.1 — проход 0, 0.9 — проход 1
        starts = [5.0, 10.0, 15.0, 20.0, 13.0, 19.0]
        p0 = [[s, s + 0.5, 60 + i, 0.1] for i, s in enumerate(starts)]
        p1 = [[s, s + 0.5, 60 + i, 0.9] for i, s in enumerate(starts)]
        got = {int(round(m)): round(float(v), 3) for _, _, m, v in _gn().pick_centered(p0, p1, self.SEG)}
        # 5, 15 — середины прохода 0; 10, 20 — середины прохода 1;
        # 13: до середины 15 — 2, до 10 — 3 → проход 0; 19: до 20 — 1, до 15 — 4 → проход 1
        self.assertEqual(got, {60: 0.1, 61: 0.9, 62: 0.1, 63: 0.9, 64: 0.1, 65: 0.9})

    def test_tie_goes_to_pass0(self):
        # 12.5: до середины 15 (проход 0) и до середины 10 (проход 1) — по 2,5 с
        p0 = [[12.5, 13.0, 64, 0.2]]
        p1 = [[12.5, 13.4, 64, 0.8]]
        self.assertEqual(_norm(_gn().pick_centered(p0, p1, self.SEG)), _norm(p0))

    def test_seg_respected(self):
        # тот же момент 10.0 при seg = 20: середина куска прохода 0 (10) — берётся проход 0
        p0 = [[10.0, 11.0, 62, 0.1]]
        p1 = [[10.0, 11.2, 62, 0.9]]
        self.assertEqual(_norm(_gn().pick_centered(p0, p1, 20.0)), _norm(p0))

    def test_empty(self):
        self.assertEqual(list(_gn().pick_centered([], [], self.SEG)), [])


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestUnion(unittest.TestCase):

    MAIN = [[1.0, 1.5, 60, 0.5], [2.0, 2.5, 64, 0.6]]

    def test_same_pitch_within_50ms_not_added(self):
        second = [[1.03, 1.4, 60, 0.9], [1.951, 2.4, 64, 0.9]]
        self.assertEqual(_norm(_gn().union(self.MAIN, second)), _norm(self.MAIN))

    def test_same_pitch_beyond_50ms_added(self):
        extra = [[1.06, 1.4, 60, 0.9], [1.94, 2.4, 64, 0.9]]
        self.assertEqual(_norm(_gn().union(self.MAIN, extra)), _norm(self.MAIN + extra))

    def test_other_pitch_same_time_added(self):
        # другая высота в то же время — новая нота (аккорд), чужая нота main ей не мешает
        extra = [[1.0, 1.5, 67, 0.4], [2.01, 2.5, 65, 0.4]]
        self.assertEqual(_norm(_gn().union(self.MAIN, extra)), _norm(self.MAIN + extra))

    def test_main_kept_whole(self):
        # ноты main не меняются и не выбрасываются, даже если second «перекрывает» их
        second = [[1.0, 3.0, 60, 1.0]]
        got = _norm(_gn().union(self.MAIN, second))
        for n in _norm(self.MAIN):
            self.assertIn(n, got)
        self.assertEqual(len(got), 2)

    def test_custom_tol(self):
        second = [[1.08, 1.4, 60, 0.9]]
        self.assertEqual(_norm(_gn().union(self.MAIN, second, tol=0.1)), _norm(self.MAIN))
        self.assertEqual(_norm(_gn().union(self.MAIN, second, tol=0.05)), _norm(self.MAIN + second))

    def test_empty_sides(self):
        gn = _gn()
        self.assertEqual(_norm(gn.union([], self.MAIN)), _norm(self.MAIN))
        self.assertEqual(_norm(gn.union(self.MAIN, [])), _norm(self.MAIN))
        self.assertEqual(list(gn.union([], [])), [])


def _ladder(n, t0=5.0, m0=60, step=2, gap=0.02, dur=0.04, vel=0.5):
    """n коротких нот подряд: начала через gap, высота растёт на step."""
    return [[t0 + i * gap, t0 + i * gap + dur, m0 + i * step, vel] for i in range(n)]


LONG = [[1.0, 2.0, 52, 0.7], [3.0, 3.5, 57, 0.6]]


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestCleanLadders(unittest.TestCase):

    def test_ladder_of_4_removed_whole(self):
        got = _norm(_gn().clean(LONG + _ladder(4)))
        self.assertEqual(got, _norm(LONG))

    def test_longer_ladder_removed_whole(self):
        got = _norm(_gn().clean(LONG + _ladder(7, step=1)))
        self.assertEqual(got, _norm(LONG))

    def test_steps_1_to_3_all_count(self):
        for step in (1, 2, 3):
            with self.subTest(step=step):
                self.assertEqual(_norm(_gn().clean(LONG + _ladder(4, step=step))), _norm(LONG))

    def test_three_notes_not_a_ladder(self):
        three = _ladder(3)
        self.assertEqual(_norm(_gn().clean(LONG + three)), _norm(LONG + three))

    def test_long_notes_not_touched(self):
        # тот же рисунок, но ноты не короче 0,06 с — это не лесенка
        slow = _ladder(5, dur=0.08)
        self.assertEqual(_norm(_gn().clean(LONG + slow)), _norm(LONG + slow))

    def test_step_4_not_a_ladder(self):
        wide = _ladder(4, step=4)
        self.assertEqual(_norm(_gn().clean(LONG + wide)), _norm(LONG + wide))

    def test_far_starts_not_a_ladder(self):
        # начала соседних через 0,05 с — не «подряд» в смысле лесенки
        spaced = _ladder(4, gap=0.05)
        self.assertEqual(_norm(_gn().clean(LONG + spaced)), _norm(LONG + spaced))

    def test_empty(self):
        self.assertEqual(list(_gn().clean([])), [])


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestCleanDuplicates(unittest.TestCase):

    def test_duplicates_merged(self):
        notes = [[1.0, 1.5, 60, 0.5], [1.02, 1.8, 60, 0.9]]
        self.assertEqual(_norm(_gn().clean(notes)), _norm([[1.0, 1.8, 60, 0.9]]))

    def test_merge_takes_max_end_and_vel_independently(self):
        # конец больше у первой, сила — у второй
        notes = [[2.0, 3.0, 64, 0.3], [2.01, 2.4, 64, 0.8]]
        self.assertEqual(_norm(_gn().clean(notes)), _norm([[2.0, 3.0, 64, 0.8]]))

    def test_unsorted_input_merged(self):
        notes = [[1.02, 1.8, 60, 0.9], [5.0, 6.0, 62, 0.5], [1.0, 1.5, 60, 0.5]]
        self.assertEqual(_norm(_gn().clean(notes)), _norm([[1.0, 1.8, 60, 0.9], [5.0, 6.0, 62, 0.5]]))

    def test_starts_beyond_30ms_kept(self):
        notes = [[1.0, 1.5, 60, 0.5], [1.04, 1.8, 60, 0.9]]
        self.assertEqual(_norm(_gn().clean(notes)), _norm(notes))

    def test_other_pitch_same_time_kept(self):
        chord = [[1.0, 2.0, 52, 0.5], [1.0, 2.0, 57, 0.5], [1.0, 2.0, 64, 0.5]]
        self.assertEqual(_norm(_gn().clean(chord)), _norm(chord))


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestStrikes(unittest.TestCase):

    BEATS = [0.0, 0.5, 1.0, 1.5, 2.0, 2.5, 3.0]

    @staticmethod
    def _midis(strike):
        return [int(round(n[2])) for n in strike["notes"]]

    def test_grouping_within_30ms_of_first(self):
        notes = [[1.0, 2.0, 52, 0.5], [1.02, 2.0, 57, 0.5], [1.025, 2.0, 64, 0.5], [1.04, 2.0, 69, 0.5]]
        st = _gn().strikes(notes, self.BEATS)
        self.assertEqual(len(st), 2)
        self.assertAlmostEqual(st[0]["t"], 1.0)
        self.assertEqual(self._midis(st[0]), [52, 57, 64])
        self.assertAlmostEqual(st[1]["t"], 1.04)
        self.assertEqual(self._midis(st[1]), [69])

    def test_grouping_measured_from_first_note_not_chain(self):
        # 1.045 ближе 0,03 к 1.02, но не к первой ноте 1.0 — новый удар
        notes = [[1.0, 2.0, 52, 0.5], [1.02, 2.0, 57, 0.5], [1.045, 2.0, 64, 0.5]]
        st = _gn().strikes(notes, self.BEATS)
        self.assertEqual([self._midis(s) for s in st], [[52, 57], [64]])

    def test_notes_sorted_low_to_high_and_t_is_first_start(self):
        notes = [[1.01, 2.0, 64, 0.5], [1.0, 2.0, 57, 0.5], [1.02, 2.0, 45, 0.5]]
        st = _gn().strikes(notes, self.BEATS)
        self.assertEqual(len(st), 1)
        self.assertEqual(self._midis(st[0]), [45, 57, 64])
        self.assertAlmostEqual(st[0]["t"], 1.0)

    def test_strikes_ordered_by_time_unsorted_input(self):
        notes = [[2.0, 2.4, 60, 0.5], [1.0, 1.4, 55, 0.5], [1.01, 1.4, 59, 0.5]]
        st = _gn().strikes(notes, self.BEATS)
        self.assertEqual([round(s["t"], 3) for s in st], [1.0, 2.0])

    def test_dir_single(self):
        st = _gn().strikes([[1.0, 2.0, 60, 0.5]], self.BEATS)
        self.assertEqual(st[0]["dir"], "single")

    def test_dir_down_on_beat_up_between(self):
        gn = _gn()
        chord = [(0.0, 57), (0.005, 60), (0.01, 64)]

        def at(t):
            return gn.strikes([[t + dt, t + 0.5, m, 0.5] for dt, m in chord], self.BEATS)[0]["dir"]

        self.assertEqual(at(1.0), "down")    # на доле
        self.assertEqual(at(1.1), "down")    # до доли 0,1, до середины 1,25 — 0,15
        self.assertEqual(at(1.2), "up")      # до середины 0,05, до доли 0,2
        self.assertEqual(at(1.26), "up")     # около середины между долями
        self.assertEqual(at(1.45), "down")   # до доли 1,5 — 0,05

    def test_no_beats_down(self):
        notes = [[1.25, 2.0, 57, 0.5], [1.26, 2.0, 64, 0.5]]
        self.assertEqual(_gn().strikes(notes, [])[0]["dir"], "down")

    def test_muted(self):
        gn = _gn()
        short = [[1.0, 1.08, 57, 0.5], [1.01, 1.09, 64, 0.5]]
        self.assertTrue(gn.strikes(short, self.BEATS)[0]["muted"])
        mixed = [[1.0, 1.08, 57, 0.5], [1.01, 1.3, 64, 0.5]]
        self.assertFalse(gn.strikes(mixed, self.BEATS)[0]["muted"])
        one = [[1.0, 1.05, 64, 0.5]]   # одна нота — не глушёный удар
        self.assertFalse(gn.strikes(one, self.BEATS)[0]["muted"])

    def test_palm(self):
        gn = _gn()
        self.assertTrue(gn.strikes([[1.0, 1.1, 45, 0.5]], self.BEATS)[0]["palm"])
        self.assertTrue(gn.strikes([[1.0, 1.1, 51, 0.5]], self.BEATS)[0]["palm"])
        # высота E3 (52) и выше — не глушение ладонью
        self.assertFalse(gn.strikes([[1.0, 1.1, 52, 0.5]], self.BEATS)[0]["palm"])
        # длинная низкая — не глушение
        self.assertFalse(gn.strikes([[1.0, 1.2, 45, 0.5]], self.BEATS)[0]["palm"])
        # две короткие низкие — удар, а не одиночная нота
        two = [[1.0, 1.1, 40, 0.5], [1.01, 1.1, 45, 0.5]]
        self.assertFalse(gn.strikes(two, self.BEATS)[0]["palm"])

    def test_empty(self):
        self.assertEqual(list(_gn().strikes([], self.BEATS)), [])


if __name__ == "__main__":
    unittest.main()
