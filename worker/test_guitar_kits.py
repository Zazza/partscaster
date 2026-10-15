"""Наборы гитар для переигровки по нотам (блок gtrnotes) — по контракту карточки
internal-guitar-notes.

Контракт:
- FX_KITS воркера: emilyguitar {notes, mute}, shinyguitar {acoustic, mute}, fsbsguitar {guitar},
  nylonguitar {guitar}; у каждого repo и закреплённая версия ref (не ветка), части —
  (каталог, regex имён файлов);
- worker/fx_kit_files.json — закреплённые списки файлов этих наборов по частям: списки не пусты,
  каждое имя проходит regex своей части;
- высота сэмпла нот — по имени файла («a2_f_rr1», «eb3_vl2_rr3_1» Karoryfer, «A#2_s2_soft_01» FSBS,
  «A#4» нейлон): у частей нот (не mute) имя начинается с ноты и октавы.

Предположение: имя набора в цепочке — «<набор>/<часть>», как у прочих наборов.
Сеть не нужна: проверяются только описания.
"""
import json
import re
import unittest
from pathlib import Path

import test_pure as tp

KIT_FILES = Path(__file__).resolve().with_name("fx_kit_files.json")
WANT = {"emilyguitar": {"notes", "mute"}, "shinyguitar": {"acoustic", "mute"},
        "fsbsguitar": {"guitar"}, "nylonguitar": {"guitar"}}
NOTE_PARTS = {("emilyguitar", "notes"), ("shinyguitar", "acoustic"), ("fsbsguitar", "guitar"),
              ("nylonguitar", "guitar")}
# нота и октава в начале имени: a2…, eb3…, A#2…, A#4
PITCH_NAME = re.compile(r"^[A-Ga-g](#|b)?\d(_|$)")
BRANCHES = {"", "main", "master", "head", "develop", "trunk"}


@unittest.skipUnless(tp._HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestGuitarKitsCatalog(tp._WorkerDbCase):

    def test_kits_and_parts(self):
        kits = self.w.FX_KITS
        for name, parts in WANT.items():
            with self.subTest(kit=name):
                self.assertIn(name, kits)
                self.assertEqual(set(kits[name]["parts"]), parts)

    def test_repo_and_pinned_ref(self):
        for name in WANT:
            with self.subTest(kit=name):
                spec = self.w.FX_KITS[name]
                self.assertIsInstance(spec.get("repo"), str)
                self.assertRegex(spec["repo"], r"^[\w.-]+/[\w.-]+$")
                ref = spec.get("ref")
                self.assertIsInstance(ref, str)
                self.assertNotIn(ref.strip().lower(), BRANCHES, "версия не закреплена")

    def test_pinned_files_match_part_regex(self):
        files = json.loads(KIT_FILES.read_text(encoding="utf-8"))
        for name, parts in WANT.items():
            for part in parts:
                with self.subTest(kit=name, part=part):
                    self.assertIn(name, files, f"нет списка файлов набора {name}")
                    names = files[name].get(part) or []
                    self.assertTrue(names, "список файлов пуст")
                    _folder, rx = self.w.FX_KITS[name]["parts"][part]
                    for f in names:
                        self.assertRegex(f, "^(?:" + rx + ")$")

    def test_note_parts_pitch_in_name(self):
        files = json.loads(KIT_FILES.read_text(encoding="utf-8"))
        for name, part in NOTE_PARTS:
            with self.subTest(kit=name, part=part):
                names = files.get(name, {}).get(part) or []
                self.assertTrue(names, f"нет списка файлов {name}/{part}")
                for f in names:
                    self.assertRegex(Path(f).stem, PITCH_NAME)

    def test_pinned_via_worker_reader(self):
        pinned = self.w._kit_pinned_files()
        for name, parts in WANT.items():
            with self.subTest(kit=name):
                self.assertEqual(set(pinned.get(name, {})), parts)


if __name__ == "__main__":
    unittest.main()
