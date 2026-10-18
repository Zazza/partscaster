"""Жанровые пресеты без кассеты (карточка internal-presets-no-cassette).

Условия:
1. Ни у одного встроенного жанрового пресета нет мастера «кассета»: 14 вариантов с мастером cassette получают
   мастер stream (glue, −14 LUFS). Мастер cassette из genres.MASTERS удаляется.
2. Гитара без ленты: у вариантов с обработкой guitar-lofi — та же цепочка без блока tape. guitar-lofi в
   fx_presets не меняется.
3. Названия/подсказки не обещают кассету/ленту, которой нет; slug не меняются.
4. Мастер «лента» (tape, лёгкий) не меняется.

Эталоны «до правки» сняты разово с genres.build()/GENRES на коммите a9e4a1b и вписаны литералами.

Толкования (неясности — в отчёте автора тестов):
- ТК2 в карточке говорит о 7 вариантах с guitar-lofi, по данным на a9e4a1b их 8 (список ниже) — проверяются все 8.
- Запись гитары — элемент specs со stems == ["guitar"].
- «Кассет»/«лент» ищутся подстрокой без учёта регистра во всей подсказке.
"""
import json
import unittest
from pathlib import Path

import genres

WORKER = Path(__file__).resolve().parent

# эталон «до правки»: варианты с мастером cassette (14)
CASSETTE_SLUGS = [
    "coldwave-tape", "coldwave-fog", "garage-lofi", "punk-garage", "psych60s-mellotron", "industrial-rust",
    "doom-sludge", "chiptune-toy", "chiptune-bleep", "triphop-rhodes", "triphop-piano", "lofihiphop-rhodes",
    "lofihiphop-piano", "lofihiphop-box",
]

# эталон «до правки»: варианты с обработкой гитары guitar-lofi (по данным — 8, в карточке написано 7)
GUITAR_LOFI_SLUGS = [
    "garage-lofi", "punk-garage", "industrial-rust", "chiptune-toy", "chiptune-bleep", "lofihiphop-rhodes",
    "lofihiphop-piano", "lofihiphop-box",
]

# ТК4: подсказки без «лент»/«кассет»
NO_TAPE_NOTE_SLUGS = [
    "coldwave-tape", "triphop-rhodes", "doom-sludge", "garage-lofi", "punk-garage", "industrial-rust",
    "lofihiphop-rhodes",
]

# мастер stream (genres.MASTERS["stream"] на a9e4a1b)
STREAM_MASTER = [{"type": "glue", "threshold_db": -18, "ratio": 2, "attack_ms": 10, "release_ms": 250, "mix": 0.7}]
STREAM_LUFS = -14.0

# прежний мастер cassette (genres.MASTERS["cassette"] на a9e4a1b)
OLD_CASSETTE_MASTER = [
    {"type": "tape", "wow": 0.3, "flutter": 0.25, "saturation": 0.35, "lowpass_hz": 9000, "hiss": 0.12},
    {"type": "glue", "threshold_db": -18, "ratio": 2, "attack_ms": 10, "release_ms": 250, "mix": 0.7},
]

# мастер «лента» (лёгкий) — условие 4, не меняется
TAPE_MASTER = (
    [
        {"type": "tape", "wow": 0.12, "flutter": 0.08, "saturation": 0.25, "lowpass_hz": 12000, "hiss": 0.04},
        {"type": "glue", "threshold_db": -18, "ratio": 2, "attack_ms": 10, "release_ms": 250, "mix": 0.7},
    ],
    -14.0,
)

# эталон «до правки» (genres.build() на a9e4a1b): 106 пресетов
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


def _guitar_spec(preset: dict) -> dict:
    specs = [s for s in preset["specs"] if "guitar" in s.get("stems", [])]
    assert len(specs) == 1 and specs[0]["stems"] == ["guitar"], (preset["slug"], [s["stems"] for s in specs])
    return specs[0]


class NoCassettePresetsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.built = genres.build()
        cls.by_slug = {p["slug"]: p for p in cls.built}
        cls.fx = _fx()

    # ТК1 / условие 1: в мастере нет «кассетного» tape (wow ≥ 0,3) и нет прежнего мастера cassette целиком
    def test_no_cassette_master_anywhere(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                heavy = [b for b in p["master"] if b.get("type") == "tape" and b.get("wow", 0) >= 0.3]
                self.assertEqual(heavy, [])
                self.assertNotEqual(p["master"], OLD_CASSETTE_MASTER)

    # ТК1 / условие 1: прежние cassette-варианты получили мастер stream, −14 LUFS
    def test_former_cassette_variants_get_stream_master(self):
        for slug in CASSETTE_SLUGS:
            with self.subTest(slug=slug):
                p = self.by_slug[slug]
                self.assertEqual(p["master"], STREAM_MASTER)
                self.assertEqual(p["target_lufs"], STREAM_LUFS)

    # условие 1: мастер cassette удалён из MASTERS, stream не изменился
    def test_masters_table_has_no_cassette(self):
        self.assertNotIn("cassette", genres.MASTERS)
        self.assertEqual(list(genres.MASTERS["stream"][0]), STREAM_MASTER)
        self.assertEqual(genres.MASTERS["stream"][1], STREAM_LUFS)

    # условие 4: мастер «лента» (лёгкий) не меняется
    def test_light_tape_master_unchanged(self):
        chain, lufs = genres.MASTERS["tape"]
        self.assertEqual(list(chain), TAPE_MASTER[0])
        self.assertEqual(lufs, TAPE_MASTER[1])

    # ТК2 / условие 2: в записи гитары нет блока tape ни у одного пресета
    def test_no_tape_in_guitar_chain(self):
        for p in self.built:
            with self.subTest(slug=p["slug"]):
                types = [b.get("type") for b in _guitar_spec(p)["engine"]]
                self.assertNotIn("tape", types)

    # ТК2 / условие 2: guitar-lofi в пресетах — та же цепочка без tape (полоса и перегруз остаются)
    def test_lofi_guitar_variants_chain_without_tape(self):
        want = [b for b in self.fx["guitar-lofi"]["chain"] if b.get("type") != "tape"]
        self.assertTrue(want, "в guitar-lofi кроме tape должны остаться блоки")
        for slug in GUITAR_LOFI_SLUGS:
            with self.subTest(slug=slug):
                self.assertEqual(_guitar_spec(self.by_slug[slug])["engine"], want)

    # ТК3 / условие 2: ручной звук guitar-lofi в fx_presets по-прежнему с лентой
    def test_fx_presets_guitar_lofi_keeps_tape(self):
        self.assertIn("guitar-lofi", self.fx)
        types = [b.get("type") for b in self.fx["guitar-lofi"]["chain"]]
        self.assertIn("tape", types)

    # ТК4 / условие 3: новые названия
    def test_renamed_variants(self):
        self.assertEqual(self.by_slug["coldwave-tape"]["name"], "Холодная волна · советская")
        self.assertEqual(self.by_slug["triphop-rhodes"]["name"], "Трип-хоп · Rhodes")

    # ТК4 / условие 3: у 14 прежних cassette-вариантов в подсказке нет «кассет»
    def test_former_cassette_notes_have_no_cassette(self):
        for slug in CASSETTE_SLUGS:
            with self.subTest(slug=slug):
                self.assertNotIn("кассет", self.by_slug[slug]["note"].lower())

    # ТК4 / условие 3: перечисленные варианты — без «лент» и «кассет» в подсказке
    def test_listed_notes_have_no_tape_or_cassette(self):
        for slug in NO_TAPE_NOTE_SLUGS:
            with self.subTest(slug=slug):
                note = self.by_slug[slug]["note"].lower()
                self.assertNotIn("лент", note)
                self.assertNotIn("кассет", note)

    # ТК5 / условие 3: slug и число пресетов прежние
    def test_same_count_and_slugs_as_before(self):
        slugs = [p["slug"] for p in self.built]
        self.assertEqual(len(slugs), len(SLUGS_BEFORE))
        self.assertEqual(sorted(slugs), sorted(SLUGS_BEFORE))


if __name__ == "__main__":
    unittest.main()
