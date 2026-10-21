"""Английские названия встроенных пресетов (internal-presets-en, ТК1, ТК3).

- ТК1: PRESET_EN (frontend/src/presetI18n.js) покрывает ровно slug из presets.BUILTIN (рецепты +
  genres.build()) — без пропусков и лишних; у каждого две непустые строки без кириллицы.
- ТК3: английское название содержит « · » тогда и только тогда, когда русское содержит « · ».

PRESET_EN читается через node (импорт модуля); без node тесты пропускаются с причиной.
"""
import json
import re
import shutil
import subprocess
import unittest
from pathlib import Path

import presets

JS = Path(__file__).resolve().parent.parent / "frontend" / "src" / "presetI18n.js"
CYR = re.compile(r"[А-Яа-яЁё]")
SEP = " · "


def _load_en():
    node = shutil.which("node")
    if not node:
        raise unittest.SkipTest("нет node: PRESET_EN из presetI18n.js не прочитать")
    code = ("import(process.argv[1]).then(m => "
            "process.stdout.write(JSON.stringify(m.PRESET_EN)))")
    out = subprocess.run([node, "--input-type=module", "-e", code, JS.as_uri()],
                         capture_output=True, text=True, timeout=30)
    if out.returncode != 0:
        raise AssertionError(f"node не прочитал {JS}: {out.stderr}")
    return json.loads(out.stdout)


class PresetEnTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.en = _load_en()
        cls.builtin = {p["slug"]: p for p in presets.BUILTIN}

    def test_tk1_covers_exactly_builtin_slugs(self):
        self.assertGreater(len(self.builtin), 0)
        missing = sorted(set(self.builtin) - set(self.en))
        extra = sorted(set(self.en) - set(self.builtin))
        self.assertEqual(missing, [], "нет перевода")
        self.assertEqual(extra, [], "лишние slug")

    def test_tk1_two_nonempty_strings_without_cyrillic(self):
        for slug, v in self.en.items():
            with self.subTest(slug=slug):
                self.assertIsInstance(v, list)
                self.assertEqual(len(v), 2)
                for s in v:
                    self.assertIsInstance(s, str)
                    self.assertTrue(s.strip(), "пустая строка")
                    self.assertIsNone(CYR.search(s), f"кириллица: {s!r}")

    def test_tk3_separator_matches_russian(self):
        for slug, p in self.builtin.items():
            if slug not in self.en:
                continue   # пропуски ловит ТК1
            with self.subTest(slug=slug):
                self.assertEqual(SEP in self.en[slug][0], SEP in p["name"],
                                 f"ru={p['name']!r} en={self.en[slug][0]!r}")


if __name__ == "__main__":
    unittest.main()
