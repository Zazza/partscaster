"""Пресеты и фразы гитары «по нотам» — по контракту карточек internal-guitar-notes и
internal-guitar-plan-chords (условия 2, 10: манера у пяти пресетов, фразы играют свои готовые ноты).

Контракт:
- готовые цепочки (worker/fx_presets.json, копия фронтовых fxPresets): guitar-notes-nylon,
  guitar-notes-acoustic, guitar-notes-emily, guitar-notes-fsbs, guitar-notes-fuzz — stems ['guitar'],
  группа guitar-notes, первый блок gtrnotes с набором своей гитары (+ обработка у электро);
  прежние 16 гитарных цепочек и их id не меняются; манера (style блока gtrnotes): нейлон 2 (перебор),
  полуакустика 0 (авто), Emily 3 (бас + аккорд), FSBS 1 (бой), фузз 4 (квинты);
- пресеты звука (genres.build): «Инди-рок · акустический» (acoustic), «Гаражный рок · фузз по нотам»
  (fuzz), «Инди-поп · чистая гитара» (emily), «Джаз-лаунж · нейлон» (nylon) — у спека гитары цепочка
  guitar-notes-*: первый блок gtrnotes с нужным набором;
- фразы страницы «Инструменты»: у гитарных фраз — phrases/<id>.notes.json (те же поля, что кэш трека:
  notes [[s, e, m, v]...]), блок gtrnotes играет по ним; phrases.NOTE_BLOCKS содержит gtrnotes;
  POST /fx/phrase с цепочкой gtrnotes на гитарной фразе — круг звучит (нот хватает, ChainError нет).

Предположения: набор пресета — «<набор>/<часть>»: nylon → nylonguitar, acoustic → shinyguitar,
emily → emilyguitar, fsbs и fuzz → fsbsguitar (карточка: «FSBS через фузз»); «обработка у электро» —
в цепочке emily/fsbs/fuzz после gtrnotes есть ещё хотя бы один блок.
"""
import json
import unittest
from pathlib import Path

import genres
import test_fx_api as fa

WORKER = Path(__file__).resolve().parent
FX_PRESETS = WORKER / "fx_presets.json"
PHRASES_DIR = WORKER / "phrases"

KIT_OF = {"guitar-notes-nylon": "nylonguitar", "guitar-notes-acoustic": "shinyguitar",
          "guitar-notes-emily": "emilyguitar", "guitar-notes-fsbs": "fsbsguitar",
          "guitar-notes-fuzz": "fsbsguitar"}
STYLE_OF = {"guitar-notes-nylon": 2, "guitar-notes-acoustic": 0, "guitar-notes-emily": 3,
            "guitar-notes-fsbs": 1, "guitar-notes-fuzz": 4}
ELECTRIC = ("guitar-notes-emily", "guitar-notes-fsbs", "guitar-notes-fuzz")
OLD_GUITAR = ("guitar-amp-clean", "guitar-amp-hot", "guitar-amp-spring", "guitar-clean", "guitar-amp-highgain",
              "guitar-postpunk", "guitar-jangle", "guitar-surf", "guitar-fuzz", "guitar-industrial",
              "guitar-symphonic", "guitar-altrock", "guitar-shoegaze", "guitar-ambient", "guitar-tremolo",
              "guitar-lofi")
GENRE_VARIANTS = {"Инди-рок · акустический": "shinyguitar", "Гаражный рок · фузз по нотам": "fsbsguitar",
                  "Инди-поп · чистая гитара": "emilyguitar", "Джаз-лаунж · нейлон": "nylonguitar"}
GUITAR_PHRASES = ("gtr-arpeggio", "gtr-strum", "gtr-sustain", "gtr-shred", "gtr-funk", "gtr-jazz")


def _kit_name(blk):
    return str(blk.get("kit", "")).split("/")[0]


class TestFxPresetChains(unittest.TestCase):

    def setUp(self):
        self.by_id = {p["id"]: p for p in json.loads(FX_PRESETS.read_text(encoding="utf-8"))}

    def test_five_notes_presets(self):
        for pid, kit in KIT_OF.items():
            with self.subTest(preset=pid):
                self.assertIn(pid, self.by_id)
                p = self.by_id[pid]
                self.assertEqual(p.get("stems"), ["guitar"])
                self.assertEqual(p.get("group"), "guitar-notes")
                first = p["chain"][0]
                self.assertEqual(first.get("type"), "gtrnotes")
                self.assertEqual(_kit_name(first), kit)
                self.assertNotIn("notes", first, "ноты кладёт воркер, в пресете их нет")

    def test_style_per_preset(self):
        for pid, style in STYLE_OF.items():
            with self.subTest(preset=pid):
                first = self.by_id[pid]["chain"][0]
                self.assertEqual(first.get("type"), "gtrnotes")
                self.assertEqual(first.get("style"), style)

    def test_styles_valid_for_block(self):
        # манера пресета проходит разбор цепочки движком (0…4, целое)
        import fx_engine
        for pid in STYLE_OF:
            with self.subTest(preset=pid):
                fx_engine.parse_chain(self.by_id[pid]["chain"])

    def test_electric_have_processing(self):
        for pid in ELECTRIC:
            with self.subTest(preset=pid):
                self.assertGreater(len(self.by_id[pid]["chain"]), 1)

    def test_old_guitar_presets_kept(self):
        for pid in OLD_GUITAR:
            with self.subTest(preset=pid):
                self.assertIn(pid, self.by_id)
                self.assertNotEqual(self.by_id[pid].get("group"), "guitar-notes")
                self.assertNotEqual(self.by_id[pid]["chain"][0].get("type"), "gtrnotes")


class TestGenreVariants(unittest.TestCase):

    def setUp(self):
        self.by_name = {p["name"]: p for p in genres.build()}

    def test_variants_exist_with_gtrnotes(self):
        for name, kit in GENRE_VARIANTS.items():
            with self.subTest(variant=name):
                self.assertIn(name, self.by_name)
                specs = [s for s in self.by_name[name]["specs"] if "guitar" in s.get("stems", [])]
                self.assertEqual(len(specs), 1)
                eng = specs[0].get("engine") or []
                self.assertTrue(eng, "у гитары нет цепочки")
                self.assertEqual(eng[0].get("type"), "gtrnotes")
                self.assertEqual(_kit_name(eng[0]), kit)
                self.assertIsNotNone(specs[0].get("level_db"), "у гитары нет цели громкости")

    def test_other_variants_untouched(self):
        # internal-guitarist-presets: гитарист теперь во всех вариантах, кроме «Электроники» (там — только
        # «8-бит и гитары»); прочие записи (барабаны, бас) гитару по нотам по-прежнему не переигрывают
        for p in genres.build():
            if p["name"] in GENRE_VARIANTS:
                continue
            electronic_yue = p["family"] == "Электроника" and p["slug"] != "chiptune-guitars"
            for s in p["specs"]:
                if "guitar" in s.get("stems", []) and not electronic_yue:
                    continue
                with self.subTest(preset=p["slug"], stems=s.get("stems")):
                    self.assertFalse(any(b.get("type") == "gtrnotes" for b in s.get("engine") or []))


class TestPhraseNotes(unittest.TestCase):

    def test_note_blocks_include_gtrnotes(self):
        import phrases
        self.assertIn("gtrnotes", phrases.NOTE_BLOCKS)
        for t in ("synth", "perc"):
            self.assertIn(t, phrases.NOTE_BLOCKS)

    def test_guitar_phrases_have_notes_file(self):
        import phrases
        self.assertEqual(set(phrases.GUITAR), set(GUITAR_PHRASES))
        for pid in GUITAR_PHRASES:
            with self.subTest(phrase=pid):
                f = PHRASES_DIR / f"{pid}.notes.json"
                self.assertTrue(f.is_file(), f"нет {f.name}")
                notes = json.loads(f.read_text(encoding="utf-8")).get("notes")
                self.assertIsInstance(notes, list)
                self.assertTrue(notes, "ноты фразы пусты")
                for n in notes:
                    s, e, m, v = n
                    self.assertTrue(0 <= s < e, n)
                    self.assertTrue(0 <= m <= 127, n)
                    self.assertTrue(0 <= v <= 1, n)


@unittest.skipUnless(fa._OK, fa._SKIP)
class TestPhraseApiGtrnotes(fa._FxApiCase):
    """Гитарная фраза с блоком gtrnotes — круг считается по нотам фразы."""

    def setUp(self):
        super().setUp()
        from test_guitar_part_api import GtrKitRes
        self.resources = GtrKitRes()

    def test_plays_by_phrase_notes(self):
        import io

        import numpy as np
        import soundfile as sf
        r = self.client.post("/fx/phrase", json={"phrase": "gtr-sustain",
                                                 "chain": [{"type": "gtrnotes", "kit": "fake/notes"}]})
        self.assertEqual(r.status_code, 200, r.text)
        f = self.client.get(f"/fx/phrase/files/{r.json()['file']}")
        self.assertEqual(f.status_code, 200, f.text)
        y, _ = sf.read(io.BytesIO(f.content), always_2d=True)
        self.assertGreater(float(np.abs(y).max()), 1e-3, "круг тишина")

    def test_bypass_keeps_gtrnotes(self):
        # без эффектов остаются блоки, которые сами играют ноты, — gtrnotes тоже
        body = {"phrase": "gtr-sustain", "chain": [{"type": "gtrnotes", "kit": "fake/notes"}, {"type": "gain"}]}
        a = self.client.post("/fx/phrase", json=dict(body, bypass=True))
        b = self.client.post("/fx/phrase", json={"phrase": "gtr-sustain", "chain": [], "bypass": True})
        self.assertEqual(a.status_code, 200, a.text)
        self.assertEqual(b.status_code, 200, b.text)
        self.assertNotEqual(a.json()["file"], b.json()["file"], "bypass выбросил gtrnotes — сухая фраза")


if __name__ == "__main__":
    unittest.main()
