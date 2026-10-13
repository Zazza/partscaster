"""Пресеты звука без двойников (internal-genres-order, условие 7 / ТК7).

В пяти парах встроенных пресетов звука обработка должна различаться по сути — цепочки стемов
(stems + engine/chain/steps), мастер, партии, — а не только громкостью.

Предположения:
- «Только громкость» = поля уровня: db, level_db, target_lufs, а также output_db блоков (выходной
  уровень). Они выкидываются на любой глубине перед сравнением; всё остальное — обработка.
- Порядок спецификаций стемов не важен (сравнивается как мультимножество), порядок блоков
  в цепочке — важен (это и есть обработка).
"""
import json
import unittest

import presets

PAIRS = [
    ("Классический рок 70-х · блюзовый", "Блюз-рок · тёплый"),
    ("Гаражный рок · фузз и лента", "Стоунер · фузз"),
    ("Хэви-метал · современный", "Альт- и ню-метал · рубленый"),
    ("Синти-поп · глянец", "Поп 80-х · баллада"),
    ("Хаус · диско-хаус", "Фанк и диско · диско"),
]
LEVEL_KEYS = {"db", "level_db", "target_lufs", "output_db"}


def _strip(v):
    """Убрать уровни громкости на любой глубине."""
    if isinstance(v, dict):
        return {k: _strip(x) for k, x in v.items() if k not in LEVEL_KEYS}
    if isinstance(v, list):
        return [_strip(x) for x in v]
    return v


def processing(p):
    """Отпечаток обработки пресета: specs (stems + engine/chain/steps) + master + parts, без громкости."""
    specs = []
    for s in p.get("specs") or []:
        one = {"stems": sorted(s.get("stems") or [])}
        for k in ("engine", "chain", "steps"):
            if k in s:
                one[k] = _strip(s[k])
        specs.append(json.dumps(one, sort_keys=True, ensure_ascii=False))
    return {
        "specs": sorted(specs),
        "master": _strip(p.get("master")),
        "parts": _strip(p.get("parts")),
    }


def by_name(name):
    found = [p for p in presets.BUILTIN if p.get("name") == name]
    return found[0] if found else None


class TestFingerprint(unittest.TestCase):
    """Самопроверка отпечатка: громкость не различает, обработка — различает."""

    def test_level_only_difference_is_equal(self):
        a = {"specs": [{"stems": ["kick"], "engine": [{"type": "eq", "gain_db": 3, "output_db": -2}],
                        "level_db": -7}], "master": [], "parts": [], "target_lufs": -14}
        b = {"specs": [{"stems": ["kick"], "engine": [{"type": "eq", "gain_db": 3, "output_db": 0}],
                        "level_db": -3}], "master": [], "parts": [], "target_lufs": -10}
        self.assertEqual(processing(a), processing(b))

    def test_chain_difference_is_not_equal(self):
        a = {"specs": [{"stems": ["kick"], "engine": [{"type": "eq"}]}], "master": [], "parts": []}
        b = {"specs": [{"stems": ["kick"], "engine": [{"type": "tape"}]}], "master": [], "parts": []}
        self.assertNotEqual(processing(a), processing(b))


class TestGenrePairs(unittest.TestCase):
    def test_pairs_exist(self):
        for a, b in PAIRS:
            with self.subTest(pair=(a, b)):
                self.assertIsNotNone(by_name(a), a)
                self.assertIsNotNone(by_name(b), b)

    def test_pairs_differ_in_processing_not_only_level(self):
        for a, b in PAIRS:
            with self.subTest(pair=(a, b)):
                pa, pb = by_name(a), by_name(b)
                if pa is None or pb is None:
                    self.fail(f"нет пресета пары: {a} / {b}")
                self.assertNotEqual(processing(pa), processing(pb),
                                    f"«{b}» обрабатывается как «{a}» — отличается только громкостью")


if __name__ == "__main__":
    unittest.main()
