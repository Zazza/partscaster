"""Жанровые пресеты: гитара YuE2 через обработку варианта, без гитариста (карточка internal-presets-yue-guitar).

Условия:
1. Во всех встроенных жанровых пресетах (genres.build()) запись гитары — гитара YuE2 через цепочку обработки
   варианта из fx_presets (guitar-* как есть); блока gtrnotes нет ни в одном пресете.
2. guitar-notes-fuzz → guitar-fuzz; guitar-notes-acoustic / -emily / -nylon → guitar-clean; slug не меняются.
3. Подсказка варианта не говорит о плане/гитаристе; «фузз по нотам» → «сухой фузз».
4. Громкость гитары = ур. гитары варианта + GTR_SHIFT семьи (без −3 дБ «гитарист тише»).
5. Симфо-метал и «Дум · траурный» — со своей обработкой варианта.
6. Звуки guitar-notes-* в fx_presets остаются.

Толкования (неясности — в отчёте автора тестов):
- «Цепочка обработки варианта» — id готовой цепочки в поле «гитара» варианта таблицы GENRES (вариант:
  slug, вариант, подсказка, набор, обработка, бас, ур. баса, гитара, ур. гитары, партии, мастер); если там
  guitar-notes-*, ожидается замена по условию 2. Ожидаемая цепочка — fx_presets.json[id]["chain"] целиком,
  блок в блок: так же проверяется «нет урезания drive/tape» и отсутствие вставок перед обработкой.
- Запись гитары — элемент specs со stems == ["guitar"]; у каждого пресета ровно одна.
- slug пресета = slug течения + "-" + slug варианта.
- ТК4 буквально: «план» и «гитарист» ищутся подстрокой без учёта регистра во всей подсказке (не только в
  удаляемых по условию 3 фразах).
- ТК5: эталон списка slug — genres.build() на коммите 23a84ee (до правки), порядок не проверяется.
"""
import json
import unittest
from pathlib import Path

import genres

WORKER = Path(__file__).resolve().parent

# условие 2: звук «по нотам» → обработка гитары YuE2
NOTES_TO_YUE = {
    "guitar-notes-fuzz": "guitar-fuzz",
    "guitar-notes-acoustic": "guitar-clean",
    "guitar-notes-emily": "guitar-clean",
    "guitar-notes-nylon": "guitar-clean",
}
NOTES_FX = ("guitar-notes-nylon", "guitar-notes-acoustic", "guitar-notes-emily", "guitar-notes-fsbs",
            "guitar-notes-fuzz")

# эталон «до правки» (genres.build() на коммите 23a84ee): 106 пресетов
SLUGS_BEFORE = [
    "postpunk-cold", "postpunk-angular", "postpunk-machine", "coldwave-tape", "coldwave-synth", "coldwave-fog",
    "newwave-gated", "newwave-synthbass", "newwave-electro", "goth-machine", "goth-cathedral", "goth-drive",
    "indie-britpop", "indie-garage", "indie-cold", "indie-acoustic", "garage-fuzz", "garage-lofi", "garage-organ",
    "garage-notesfuzz", "punk-fast", "punk-garage", "punk-pop", "alt90s-phaser", "alt90s-grunge", "alt90s-dreamy",
    "classic70s-organ", "classic70s-hall", "classic70s-blues", "bluesrock-warm", "bluesrock-dirty",
    "bluesrock-piano", "surf-spring", "surf-organ", "surf-dark", "psych60s-mellotron", "psych60s-organ",
    "psych60s-harpsichord", "shoegaze-wall", "shoegaze-dream", "shoegaze-haze", "postrock-ambient",
    "postrock-piano", "postrock-bells", "stoner-fuzz", "stoner-desert", "stoner-doomy", "heavy-classic",
    "heavy-modern", "heavy-epic", "numetal-chug", "numetal-machine", "numetal-dark", "industrial-machine",
    "industrial-80s", "industrial-rust", "symphonic-choir", "symphonic-strings", "symphonic-piano", "doom-slow",
    "doom-funeral", "doom-sludge", "synthpop-808", "synthpop-dx7", "synthpop-lead", "synthwave-retro",
    "synthwave-night", "synthwave-dark", "chiptune-toy", "chiptune-guitars", "chiptune-bleep", "ebm-march",
    "ebm-electro", "ebm-cold", "techno-acid", "techno-minimal", "techno-dark", "house-piano", "house-disco",
    "house-deep", "triphop-rhodes", "triphop-strings", "triphop-piano", "lofihiphop-rhodes", "lofihiphop-piano",
    "lofihiphop-box", "ambient-pad", "ambient-bells", "ambient-choir", "pop80s-ballad", "pop80s-dance",
    "pop80s-bright", "indiepop-jangle", "indiepop-toy", "indiepop-dream", "indiepop-cleangtr", "funk-funk",
    "funk-disco", "funk-boogie", "soul-organ", "soul-rhodes", "soul-strings", "lounge-rhodes", "lounge-piano",
    "lounge-bossa", "lounge-nylon",
]


def _fx() -> dict:
    data = json.loads((WORKER / "fx_presets.json").read_text(encoding="utf-8"))
    return {p["id"]: p for p in data}


def _variants() -> dict:
    """slug пресета → (семья, течение, id цепочки гитары, ур. гитары) — из таблицы GENRES."""
    out = {}
    for family, genre, gslug, variants in genres.GENRES:
        for v in variants:
            out[f"{gslug}-{v[0]}"] = (family, genre, v[7], v[8])
    return out


def _guitar_spec(preset: dict) -> dict:
    specs = [s for s in preset["specs"] if "guitar" in s.get("stems", [])]
    assert len(specs) == 1 and specs[0]["stems"] == ["guitar"], (preset["slug"], [s["stems"] for s in specs])
    return specs[0]


def _blocks(obj):
    """Все словари-блоки внутри пресета (specs, final, parts — любая глубина)."""
    if isinstance(obj, dict):
        yield obj
        for v in obj.values():
            yield from _blocks(v)
    elif isinstance(obj, list):
        for v in obj:
            yield from _blocks(v)


class YueGuitarPresetsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.built = genres.build()
        cls.fx = _fx()
        cls.variants = _variants()

    def _expected_chain_id(self, slug: str) -> str:
        gtr = self.variants[slug][2]
        return NOTES_TO_YUE.get(gtr, gtr)

    # ТК1 / условие 1
    def test_no_gtrnotes_block_anywhere(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                types = {b.get("type") for b in _blocks(p)}
                self.assertNotIn("gtrnotes", types)

    # ТК2 / условия 1, 2
    def test_guitar_chain_is_variant_fx_chain(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                cid = self._expected_chain_id(p["slug"])
                self.assertIn(cid, self.fx, "цепочки варианта нет в fx_presets")
                self.assertEqual(_guitar_spec(p)["engine"], self.fx[cid]["chain"])

    # ТК2 / условие 2: guitar-notes-* заменены обработкой гитары YuE2
    def test_notes_variants_get_yue_processing(self):
        notes_slugs = [s for s, v in self.variants.items() if v[2] in NOTES_TO_YUE]
        for slug in notes_slugs:
            with self.subTest(slug=slug):
                p = next(x for x in self.built if x["slug"] == slug)
                target = NOTES_TO_YUE[self.variants[slug][2]]
                self.assertEqual(_guitar_spec(p)["engine"], self.fx[target]["chain"])
        # варианты, где стоял звук «по нотам» (эталон до правки): обработка — guitar-fuzz / guitar-clean
        expected = {"garage-notesfuzz": "guitar-fuzz", "indie-acoustic": "guitar-clean",
                    "indiepop-cleangtr": "guitar-clean", "lounge-nylon": "guitar-clean"}
        for slug, target in expected.items():
            with self.subTest(slug=slug):
                p = next(x for x in self.built if x["slug"] == slug)
                self.assertEqual(_guitar_spec(p)["engine"], self.fx[target]["chain"])

    # ТК2: нет second take (double), drive/tape не урезаны относительно fx_presets
    def test_no_double_and_no_softened_drive_or_tape(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                spec = _guitar_spec(p)
                self.assertNotIn("double", spec)
                for b in spec["engine"]:
                    self.assertNotIn("double", b)
                ref = self.fx[self._expected_chain_id(p["slug"])]["chain"]
                for kind in ("drive", "tape"):
                    got = [b for b in spec["engine"] if b.get("type") == kind]
                    want = [b for b in ref if b.get("type") == kind]
                    self.assertEqual(got, want, kind)

    # условие 5: Симфо-метал и «Дум · траурный» — своя обработка варианта
    def test_symphonic_and_funeral_doom_use_own_processing(self):
        slugs = [s for s, v in self.variants.items() if v[1] == "Симфо-метал"] + ["doom-funeral"]
        self.assertGreaterEqual(len(slugs), 4)
        for slug in slugs:
            with self.subTest(slug=slug):
                p = next(x for x in self.built if x["slug"] == slug)
                self.assertEqual(_guitar_spec(p)["engine"], self.fx[self._expected_chain_id(slug)]["chain"])

    # ТК3 / условие 4
    def test_guitar_level_is_variant_level_plus_family_shift(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                family, _, _, gtr_lv = self.variants[p["slug"]]
                self.assertEqual(p["family"], family)
                want = float(gtr_lv) + genres.GTR_SHIFT[family]
                self.assertAlmostEqual(_guitar_spec(p)["level_db"], want, places=6)

    # ТК4 / условие 3
    def test_note_has_no_plan_or_guitarist(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                note = p["note"].lower()
                self.assertNotIn("план", note)
                self.assertNotIn("гитарист", note)

    def test_notes_fuzz_renamed_dry_fuzz(self):
        names = {p["slug"]: p["name"] for p in self.built}
        for slug, name in names.items():
            with self.subTest(slug=slug):
                self.assertNotIn("фузз по нотам", name)
        self.assertEqual(names.get("garage-notesfuzz"), "Гаражный рок · сухой фузз")

    # ТК5 / условие 2 (slug не меняются)
    def test_same_count_and_slugs_as_before(self):
        slugs = [p["slug"] for p in self.built]
        self.assertEqual(len(slugs), len(SLUGS_BEFORE))
        self.assertEqual(sorted(slugs), sorted(SLUGS_BEFORE))

    # ТК6 / условие 6
    def test_fx_presets_keep_guitar_notes_sounds(self):
        for pid in NOTES_FX:
            with self.subTest(id=pid):
                self.assertIn(pid, self.fx)
                self.assertTrue(any(b.get("type") == "gtrnotes" for b in self.fx[pid]["chain"]))


if __name__ == "__main__":
    unittest.main()
