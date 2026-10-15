"""Тесты чистой логики ударов гитары (worker/guitarnotes.py) — по карточкам internal-guitar-notes
(разбор на удары) и internal-guitar-plan-chords (условие 6: направление по продлённой сетке долей),
не по реализации. Распознавание нот (pick_centered/union/clean) удалено вместе с моделями.

Контракт:
- нота — список [start, end, midi, vel] (секунды трека, vel 0…1);
- strikes(notes, beats): удары — ноты с началами ближе 0,03 с к первой ноте группы; поля
  t (начало первой), notes (по высоте снизу вверх), dir "single" | "down" | "up" (down — начало
  ближе к доле, чем к середине между долями; меньше двух долей — down), muted (≥2 нот и все короче
  0,1 с), palm (1 нота, высота < 52 и короче 0,15 с);
- направление (условие 6): до первой и после последней доли сетка продлевается медианным шагом долей
  от крайней доли — вступление без барабанов играется вниз/вверх по той же сетке.

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

    def test_removed_recognition_helpers(self):
        # распознавание удалено целиком (условие 9): склейки проходов и чистки распознанных нот нет
        gn = _gn()
        for name in ("pick_centered", "union", "clean"):
            with self.subTest(name=name):
                self.assertFalse(hasattr(gn, name), f"guitarnotes.{name} остался")


@unittest.skipUnless(_HAS_NP, _SKIP)
class TestDirectionExtendedGrid(unittest.TestCase):
    """Условие 6: удар вне сетки долей — направление по сетке, продлённой средним (медианным) шагом."""

    # доли с 2,0 с шагом 0,5 (медиана 0,5; одна неровная доля медиану не сдвигает)
    BEATS = [2.0, 2.5, 3.0, 3.52, 4.0, 4.5]

    @staticmethod
    def _dir(t, beats):
        chord = [[t, t + 0.4, 52, 0.5], [t + 0.005, t + 0.4, 57, 0.5]]
        return _gn().strikes(chord, beats)[0]["dir"]

    def test_before_first_beat_on_extended_beat_down(self):
        for t in (1.5, 1.0, 0.5, 0.0):
            with self.subTest(t=t):
                self.assertEqual(self._dir(t, self.BEATS), "down")

    def test_before_first_beat_between_extended_beats_up(self):
        for t in (1.75, 1.25, 0.25):
            with self.subTest(t=t):
                self.assertEqual(self._dir(t, self.BEATS), "up")

    def test_after_last_beat_extended(self):
        self.assertEqual(self._dir(5.0, self.BEATS), "down")
        self.assertEqual(self._dir(5.5, self.BEATS), "down")
        self.assertEqual(self._dir(4.75, self.BEATS), "up")
        self.assertEqual(self._dir(5.25, self.BEATS), "up")

    def test_extension_uses_median_step_not_first_gap(self):
        # первый промежуток 0,3 с, остальные 0,5 с: продление — шагом 0,5 (1,5 — доля, 1,7 — нет)
        beats = [1.7, 2.0, 2.5, 3.0, 3.5, 4.0]
        self.assertEqual(self._dir(1.2, beats), "down")
        self.assertEqual(self._dir(1.45, beats), "up")

    def test_fewer_than_two_beats_down(self):
        for beats in ([], [2.0]):
            with self.subTest(beats=beats):
                self.assertEqual(self._dir(1.25, beats), "down")
                self.assertEqual(self._dir(2.25, beats), "down")


if __name__ == "__main__":
    unittest.main()
