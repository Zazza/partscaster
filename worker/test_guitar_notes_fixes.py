"""Тесты к находкам кросс-ревью задачи internal-guitar-notes, оставшиеся после internal-guitar-plan-chords
(распознавание нот удалено; кэш партии и одновременные запросы — в test_guitar_part_api).

Контракт:
1) Фразы страницы «Инструменты» (phrases._twice) играют свои готовые ноты (условие 10 новой карточки): у блока
   gtrnotes во втором круге повторяются не только ноты, но и доли `_beats` (со сдвигом на длину круга) — иначе
   удары второго круга теряют направление "up".
2) Пустое окно — тишина, а не ошибка: блок с notes == [] при служебном `_t0` (окно выставил воркер) → тишина
   той же формы; без `_t0` и без нот → ChainError «нет нот» (test_gtrnotes_block).

Запуск: cd worker && python3 -m unittest test_guitar_notes_fixes -v
"""
import unittest

import test_gtrnotes_block as gb


# ---------- 1) фразы: доли второго круга ----------

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestPhraseTwiceBeats(unittest.TestCase):
    """Вход фразы — два круга подряд: удары второго круга считаются по долям второго круга."""

    PHRASE = "gtr-strum"

    def test_second_cycle_keeps_up_strokes(self):
        import guitarnotes
        import phrases
        _, cycle = phrases.render_dry(self.PHRASE, 1.0)
        chain = phrases.gtrnotes_chain([{"type": "gtrnotes", "kit": "fake/notes"}], self.PHRASE)
        blk = phrases._twice(chain, cycle)[0]
        eps = 1e-6
        notes = [[n["t"], n["t"] + n["d"], n["midi"][0], n["vel"]] for n in blk["notes"]]
        beats = [float(b) for b in blk.get("_beats", [])]

        def ups(lo, hi):
            ns = [n for n in notes if lo - eps <= n[0] < hi - eps]
            bs = [b for b in beats if lo - eps <= b < hi - eps]
            return sum(1 for s in guitarnotes.strikes(ns, bs) if s["dir"] == "up")

        first, second = ups(0.0, cycle), ups(cycle, 2 * cycle)
        self.assertGreater(first, 0, "опора: в первом круге фразы есть удары вверх")
        self.assertGreater(second, 0, "во втором круге нет долей — все удары стали «вниз»")
        self.assertLessEqual(abs(second - first), 2, f"ударов вверх: первый круг {first}, второй {second}")


# ---------- 2) пустое окно — тишина ----------

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestBlockEmptyWindow(unittest.TestCase):
    """Блок, которому воркер выставил окно (_t0), но нот в окне нет, — тишина той же формы."""

    def test_empty_notes_with_t0_is_silence(self):
        x = gb._input()
        y = gb._run(gb._blk([], _t0=5.0, _beats=[0.0, 0.5, 1.0]), x=x)
        y = gb.np.asarray(y)
        self.assertEqual(y.shape, x.shape)
        self.assertLess(float(gb.np.abs(y).max()), 1e-6, "вместо тишины — звук (старая гитара?)")

    def test_empty_notes_without_t0_still_error(self):
        # опора: без _t0 блок не обработан воркером — ошибка «нет нот» остаётся
        fx = gb._fx()
        with self.assertRaises(fx.ChainError):
            gb._run(gb._blk([]))


if __name__ == "__main__":
    unittest.main()
