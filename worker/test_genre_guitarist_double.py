"""Второй дубль гитариста в жанровых пресетах (карточка internal-guitar-double, условие 6, тест-кейс ТК8).

Условие 6: у гитариста (блок gtrnotes в цепочке гитары, genres.build()) double=1, если манера style — 1 (бой),
4 (квинты) или 0 (авто); у 2 (перебор), 3 (бас + аккорд) и «как в КИНО» — без double. Варианты guitar-notes-*
(гитарист уже в цепочке) — по тому же правилу по их манере. «Электроника» без гитариста — без изменений.

Толкования:
- манера без ключа style — 0 (по умолчанию блока);
- «без double» — ключа нет или double = 0 (по поведению блока это одно и то же: по умолчанию 0);
- «КИНО» и «Электроника без гитариста» — те же определения, что в test_genre_guitarist
  (_is_kino, _with_guitarist, снимок GTR_ID).

Внешних границ нет — моков нет.

Запуск: cd worker && python3 -m unittest test_genre_guitarist_double -v
"""
import unittest

import genres
import test_genre_guitarist as tg

DOUBLE_STYLES = {0, 1, 4}
SINGLE_STYLES = {2, 3}


def _gtrnotes(p):
    eng = tg._guitar(p)
    found = [b for b in eng if b.get("type") == "gtrnotes"]
    return found[0] if found else None


class TestGuitaristDouble(unittest.TestCase):

    def setUp(self):
        self.built = genres.build()

    def _guitarists(self):
        out = [(p, _gtrnotes(p)) for p in self.built]
        return [(p, b) for p, b in out if b is not None]

    def test_strum_power_auto_doubled(self):
        checked = set()
        for p, b in self._guitarists():
            style = b.get("style", 0)
            if tg._is_kino(p) or style not in DOUBLE_STYLES:
                continue
            checked.add(style)
            with self.subTest(slug=p["slug"], style=style):
                self.assertIn("double", b, f"нет double: {b}")
                self.assertEqual(b["double"], 1)
                self.assertIsInstance(b["double"], int)
        # проверка не вхолостую: в пресетах есть и бой, и квинты, и авто
        self.assertEqual(checked, DOUBLE_STYLES, f"манеры с гитаристом в пресетах: {sorted(checked)}")

    def test_picking_and_bass_chord_single(self):
        checked = set()
        for p, b in self._guitarists():
            style = b.get("style", 0)
            if style not in SINGLE_STYLES:
                continue
            checked.add(style)
            with self.subTest(slug=p["slug"], style=style):
                self.assertEqual(b.get("double", 0), 0, f"double у манеры {style}: {b}")
        self.assertEqual(checked, SINGLE_STYLES, f"манеры с гитаристом в пресетах: {sorted(checked)}")

    def test_kino_single(self):
        kino = [(p, b) for p, b in self._guitarists() if tg._is_kino(p)]
        self.assertGreaterEqual(len(kino), 4)
        for p, b in kino:
            with self.subTest(slug=p["slug"]):
                self.assertEqual(b.get("style"), 4, "опора: «КИНО» — квинты (правило по манере дало бы double)")
                self.assertEqual(b.get("double", 0), 0, f"double у «КИНО»: {b}")

    def test_notes_variants_by_style(self):
        notes = [p for p in self.built if tg.GTR_ID[p["slug"]].startswith("guitar-notes-")]
        self.assertEqual(len(notes), 4)
        for p in notes:
            b = _gtrnotes(p)
            with self.subTest(slug=p["slug"]):
                self.assertIsNotNone(b)
                style = b.get("style", 0)
                want = 1 if style in DOUBLE_STYLES and not tg._is_kino(p) else 0
                self.assertEqual(b.get("double", 0), want, f"манера {style}: {b}")

    def test_electronic_without_guitarist_unchanged(self):
        fx = tg._fx()
        el = [p for p in self.built if not tg._with_guitarist(p)]
        self.assertTrue(el)
        for p in el:
            with self.subTest(slug=p["slug"]):
                eng = tg._guitar(p)
                self.assertNotIn("gtrnotes", [b.get("type") for b in eng])
                self.assertFalse(any("double" in b for b in eng), f"double без гитариста: {eng}")
                self.assertEqual(eng, fx[tg.GTR_ID[p["slug"]]]["chain"])


if __name__ == "__main__":
    unittest.main()
