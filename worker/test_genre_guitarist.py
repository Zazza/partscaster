"""Гитарист «по аккордам плана» во всех жанровых пресетах звука (карточка internal-guitarist-presets).

Условия:
1. Во всех встроенных жанровых пресетах (genres.build()), кроме семьи «Электроника», гитара YuE2 заменяется
   гитаристом (блок gtrnotes): цепочка гитары = [gtrnotes] + префикс + собственная обработка варианта; гитара
   и манера — по обработке варианта (таблица GUITARIST ниже); «КИНО» — все варианты «Симфо-метал» и
   «Дум · траурный»: Emily (+mute), квинты, чистая цепочка вместо обработки варианта; «Электроника» — без
   изменений, кроме «Чиптюн и 8-бит · 8-бит и гитары»; варианты с guitar-notes-* — как были.
   Исключение по слуху: «Пост-панк · угловатый сухой» (postpunk-angular, guitar-jangle) — перебор (2).
2. Лента в цепочке гитариста — wow ≤ 0,15, flutter ≤ 0,1.
3. Цель громкости гитариста = ур. гитары + GTR_SHIFT семьи − 3 (и для guitar-notes-*) — test_genre_levels.
5. Подсказка варианта с гитаристом содержит «план».

Толкования (буквальные, неясности — в отчёте автора тестов):
- «Обработка варианта» — id готовой цепочки гитары варианта в таблице GENRES; снимок id по slug — GTR_ID
  (значения на коммите 368ccef), чтобы проверка не зависела от формы таблицы.
- Набор «FSBS» / «Emily» — те же kit / mute_kit, что у готовых цепочек guitar-notes-fsbs / guitar-notes-emily
  (worker/fx_presets.json). «+mute» указан только у guitar-clean и «КИНО»: у прочих вариантов Emily наличие
  mute_kit не проверяется.
- «Собственная обработка варианта» — цепочка из fx_presets.json по id; у guitar-lofi в ней лента, и по
  условию 2 она мягче: у блоков tape сверяется всё, кроме wow/flutter (те — по условию 2).
- Префикс фузза у guitar-lofi — блоки eq (highpass_hz 100) и drive (gain_db 18, mix 1; было 34 — карточка
  internal-guitarist-drive, условие 2); сверяются названные
  параметры.
- Чистая цепочка «КИНО» — ровно четыре блока eq, comp, delay, reverb с параметрами из условия
  («delay 420 мс 0,25» — time_ms 420, feedback 0.25); перегруза (drive/amp) нет.
"""
import copy
import json
import unittest
from pathlib import Path

import genres

WORKER = Path(__file__).resolve().parent

ELECTRONIC = "Электроника"
EIGHT_BIT = "chiptune-guitars"           # «Чиптюн и 8-бит · 8-бит и гитары»
KINO_FUNERAL = "doom-funeral"            # «Дум · траурный»
KINO_GENRE = "Симфо-метал"
ANGULAR = "postpunk-angular"             # исключение по слуху: перебор вместо боя

# снимок: slug → id цепочки гитары варианта (коммит 368ccef)
GTR_ID = {
    "postpunk-cold": "guitar-postpunk", "postpunk-angular": "guitar-jangle", "postpunk-machine": "guitar-ambient",
    "coldwave-tape": "guitar-postpunk", "coldwave-synth": "guitar-tremolo", "coldwave-fog": "guitar-ambient",
    "newwave-gated": "guitar-jangle", "newwave-synthbass": "guitar-postpunk", "newwave-electro": "guitar-clean",
    "goth-machine": "guitar-shoegaze", "goth-cathedral": "guitar-ambient", "goth-drive": "guitar-tremolo",
    "indie-britpop": "guitar-jangle", "indie-garage": "guitar-fuzz", "indie-cold": "guitar-postpunk",
    "indie-acoustic": "guitar-notes-acoustic", "garage-fuzz": "guitar-fuzz", "garage-lofi": "guitar-lofi",
    "garage-organ": "guitar-altrock", "garage-notesfuzz": "guitar-notes-fuzz", "punk-fast": "guitar-fuzz",
    "punk-garage": "guitar-lofi", "punk-pop": "guitar-altrock", "alt90s-phaser": "guitar-altrock",
    "alt90s-grunge": "guitar-fuzz", "alt90s-dreamy": "guitar-shoegaze", "classic70s-organ": "guitar-fuzz",
    "classic70s-hall": "guitar-symphonic", "classic70s-blues": "guitar-clean", "bluesrock-warm": "guitar-clean",
    "bluesrock-dirty": "guitar-fuzz", "bluesrock-piano": "guitar-clean", "surf-spring": "guitar-surf",
    "surf-organ": "guitar-surf", "surf-dark": "guitar-tremolo", "psych60s-mellotron": "guitar-tremolo",
    "psych60s-organ": "guitar-ambient", "psych60s-harpsichord": "guitar-jangle", "shoegaze-wall": "guitar-shoegaze",
    "shoegaze-dream": "guitar-ambient", "shoegaze-haze": "guitar-tremolo", "postrock-ambient": "guitar-ambient",
    "postrock-piano": "guitar-ambient", "postrock-bells": "guitar-shoegaze", "stoner-fuzz": "guitar-fuzz",
    "stoner-desert": "guitar-altrock", "stoner-doomy": "guitar-symphonic", "heavy-classic": "guitar-symphonic",
    "heavy-modern": "guitar-industrial", "heavy-epic": "guitar-symphonic", "numetal-chug": "guitar-industrial",
    "numetal-machine": "guitar-industrial", "numetal-dark": "guitar-altrock", "industrial-machine": "guitar-industrial",
    "industrial-80s": "guitar-industrial", "industrial-rust": "guitar-lofi", "symphonic-choir": "guitar-symphonic",
    "symphonic-strings": "guitar-symphonic", "symphonic-piano": "guitar-symphonic", "doom-slow": "guitar-fuzz",
    "doom-funeral": "guitar-symphonic", "doom-sludge": "guitar-fuzz", "synthpop-808": "guitar-clean",
    "synthpop-dx7": "guitar-clean", "synthpop-lead": "guitar-clean", "synthwave-retro": "guitar-ambient",
    "synthwave-night": "guitar-ambient", "synthwave-dark": "guitar-clean", "chiptune-toy": "guitar-lofi",
    "chiptune-guitars": "guitar-fuzz", "chiptune-bleep": "guitar-lofi", "ebm-march": "guitar-industrial",
    "ebm-electro": "guitar-clean", "ebm-cold": "guitar-tremolo", "techno-acid": "guitar-clean",
    "techno-minimal": "guitar-clean", "techno-dark": "guitar-ambient", "house-piano": "guitar-clean",
    "house-disco": "guitar-clean", "house-deep": "guitar-clean", "triphop-rhodes": "guitar-ambient",
    "triphop-strings": "guitar-tremolo", "triphop-piano": "guitar-ambient", "lofihiphop-rhodes": "guitar-lofi",
    "lofihiphop-piano": "guitar-lofi", "lofihiphop-box": "guitar-lofi", "ambient-pad": "guitar-ambient",
    "ambient-bells": "guitar-ambient", "ambient-choir": "guitar-shoegaze", "pop80s-ballad": "guitar-clean",
    "pop80s-dance": "guitar-clean", "pop80s-bright": "guitar-jangle", "indiepop-jangle": "guitar-jangle",
    "indiepop-toy": "guitar-clean", "indiepop-dream": "guitar-ambient", "indiepop-cleangtr": "guitar-notes-emily",
    "funk-funk": "guitar-clean", "funk-disco": "guitar-jangle", "funk-boogie": "guitar-clean",
    "soul-organ": "guitar-clean", "soul-rhodes": "guitar-clean", "soul-strings": "guitar-clean",
    "lounge-rhodes": "guitar-clean", "lounge-piano": "guitar-clean", "lounge-bossa": "guitar-clean",
    "lounge-nylon": "guitar-notes-nylon",
}

# обработка варианта → (гитара, манера, нужен mute Emily)
FSBS, EMILY = "fsbs", "emily"
GUITARIST = {
    "guitar-postpunk": (FSBS, 2, False),
    "guitar-jangle": (FSBS, 1, False),
    "guitar-tremolo": (EMILY, 2, False),
    "guitar-ambient": (EMILY, 2, False),
    "guitar-shoegaze": (FSBS, 1, False),
    "guitar-clean": (EMILY, 3, True),
    "guitar-surf": (FSBS, 2, False),
    "guitar-altrock": (FSBS, 0, False),
    "guitar-lofi": (FSBS, 4, False),
    "guitar-fuzz": (FSBS, 4, False),
    "guitar-industrial": (FSBS, 4, False),
    "guitar-symphonic": (FSBS, 4, False),
}
STYLE_OVERRIDE = {ANGULAR: 2}   # исключение по слуху (дополнение карточки, условие 1)
LOFI_PREFIX = [("eq", {"highpass_hz": 100}), ("drive", {"gain_db": 18, "mix": 1})]
KINO_CHAIN = [
    ("eq", {"highpass_hz": 80}),
    ("comp", {"threshold_db": -24, "ratio": 3, "makeup_db": 3}),
    ("delay", {"time_ms": 420, "feedback": 0.25, "wet": 0.12, "lowpass_hz": 4000}),
    ("reverb", {"decay_s": 2.6, "predelay_ms": 30, "wet": 0.2, "lowpass_hz": 7000}),
]
TAPE_WOW_MAX, TAPE_FLUTTER_MAX = 0.15, 0.1


def _fx():
    return {p["id"]: p for p in json.loads((WORKER / "fx_presets.json").read_text(encoding="utf-8"))}


def _guitar(p):
    found = [s for s in p["specs"] if s.get("stems") == ["guitar"]]
    if len(found) != 1:
        raise AssertionError(f"{p['slug']}: записей [guitar] {len(found)}, want 1")
    return found[0].get("engine") or []


def _texts(v):
    if isinstance(v, dict):
        return [x for x in v.values() if isinstance(x, str)]
    return [v] if isinstance(v, str) else []


def _is_kino(p):
    return p["name"].split(" · ")[0] == KINO_GENRE or p["slug"] == KINO_FUNERAL


def _with_guitarist(p):
    """Вариант, где по карточке играет гитарист (включая guitar-notes-*)."""
    return p["family"] != ELECTRONIC or p["slug"] == EIGHT_BIT


def _params_ok(test, blk, typ, params, where):
    test.assertEqual(blk.get("type"), typ, f"{where}: блок {blk}, want {typ}")
    for k, v in params.items():
        test.assertIn(k, blk, f"{where}: у {typ} нет {k}: {blk}")
        test.assertAlmostEqual(float(blk[k]), float(v), places=6, msg=f"{where}: {typ}.{k}")


def _same_but_tape_wow(test, got, want, where):
    """Цепочки равны; у блоков tape не сверяются wow/flutter (их ограничивает условие 2)."""
    test.assertEqual([b.get("type") for b in got], [b.get("type") for b in want], f"{where}: блоки")
    def strip(b):
        return {k: v for k, v in b.items() if k not in ("wow", "flutter")}

    for g, w in zip(got, want, strict=False):
        if w.get("type") == "tape":
            test.assertEqual(strip(g), strip(w), f"{where}: лента")
        else:
            test.assertEqual(g, w, where)


class TestGuitaristInGenrePresets(unittest.TestCase):

    def setUp(self):
        self.built = genres.build()
        self.fx = _fx()
        self.kit = {
            FSBS: self.fx["guitar-notes-fsbs"]["chain"][0]["kit"],
            EMILY: self.fx["guitar-notes-emily"]["chain"][0]["kit"],
        }
        self.emily_mute = self.fx["guitar-notes-emily"]["chain"][0]["mute_kit"]

    def test_snapshot_covers_all(self):
        self.assertEqual({p["slug"] for p in self.built}, set(GTR_ID))

    def test_kino_set_nonempty(self):
        kino = {p["slug"] for p in self.built if _is_kino(p)}
        self.assertIn(KINO_FUNERAL, kino)
        self.assertGreaterEqual(len(kino), 4, f"«КИНО»: {sorted(kino)} (три варианта симфо-метала + дум траурный)")

    def _check_gtrnotes(self, p, first, guitar, style, mute):
        where = p["slug"]
        self.assertEqual(first.get("type"), "gtrnotes", f"{where}: первый блок гитары {first}")
        self.assertEqual(first.get("kit"), self.kit[guitar], f"{where}: набор гитариста")
        self.assertEqual(first.get("style"), style, f"{where}: манера")
        if mute:
            self.assertEqual(first.get("mute_kit"), self.emily_mute, f"{where}: mute Emily")

    def test_guitarist_chain_by_table(self):
        # условие 1: [gtrnotes] + префикс + обработка варианта (не-электроника и «8-бит и гитары»)
        for p in self.built:
            gid = GTR_ID[p["slug"]]
            if not _with_guitarist(p) or gid.startswith("guitar-notes-") or _is_kino(p):
                continue
            with self.subTest(slug=p["slug"], chain=gid):
                eng = _guitar(p)
                self.assertTrue(eng, "пустая цепочка гитары")
                guitar, style, mute = GUITARIST[gid]
                style = STYLE_OVERRIDE.get(p["slug"], style)
                self._check_gtrnotes(p, eng[0], guitar, style, mute)
                rest = eng[1:]
                if gid == "guitar-lofi":
                    self.assertGreaterEqual(len(rest), len(LOFI_PREFIX), f"нет префикса фузза: {rest}")
                    for blk, (typ, params) in zip(rest, LOFI_PREFIX, strict=False):
                        _params_ok(self, blk, typ, params, f"{p['slug']} префикс фузза")
                    rest = rest[len(LOFI_PREFIX):]
                # перегруз гитариста — не сильнее 18 дБ (карточка internal-guitarist-drive, условие 1)
                want = [dict(b, gain_db=min(b["gain_db"], 18)) if b["type"] == "drive" and "gain_db" in b else b
                        for b in self.fx[gid]["chain"]]
                _same_but_tape_wow(self, rest, want, f"{p['slug']}: обработка варианта {gid}")

    def test_angular_is_picking(self):
        # дополнение карточки: «Пост-панк · угловатый сухой» — перебор, а не бой guitar-jangle
        p = next(x for x in self.built if x["slug"] == ANGULAR)
        first = _guitar(p)[0]
        self.assertEqual(first.get("type"), "gtrnotes")
        self.assertEqual(first.get("kit"), self.kit[FSBS])
        self.assertEqual(first.get("style"), 2)

    def test_jangle_others_strum(self):
        # исключение — только угловатый: прочие варианты на guitar-jangle — бой (1)
        others = [p for p in self.built if GTR_ID[p["slug"]] == "guitar-jangle" and p["slug"] != ANGULAR
                  and _with_guitarist(p)]
        self.assertTrue(others)
        for p in others:
            with self.subTest(slug=p["slug"]):
                self.assertEqual(_guitar(p)[0].get("style"), 1)

    def test_kino_clean_chain(self):
        # «КИНО»: Emily (+mute), квинты, чистая цепочка вместо обработки варианта
        kino = [p for p in self.built if _is_kino(p)]
        self.assertTrue(kino)
        for p in kino:
            with self.subTest(slug=p["slug"]):
                eng = _guitar(p)
                self._check_gtrnotes(p, eng[0], EMILY, 4, True)
                rest = eng[1:]
                self.assertEqual([b.get("type") for b in rest], [t for t, _ in KINO_CHAIN],
                                 f"чистая цепочка «КИНО»: {rest}")
                for blk, (typ, params) in zip(rest, KINO_CHAIN, strict=False):
                    _params_ok(self, blk, typ, params, p["slug"])
                bands = rest[0].get("bands") or []
                self.assertTrue(any(abs(float(b.get("freq_hz", 0)) - 3000) < 1e-6 and
                                    abs(float(b.get("gain_db", 0)) - 2) < 1e-6 for b in bands),
                                f"eq «КИНО» без полосы 3 кГц +2 дБ: {bands}")
                self.assertFalse({"drive", "amp"} & {b.get("type") for b in rest}, "в чистой цепочке перегруз")

    def test_notes_variants_unchanged(self):
        # варианты, где гитарист уже стоял (guitar-notes-*), — как были; второй дубль — по манере
        # (internal-guitar-double, условие 6: авто, бой, квинты — double=1)
        notes = [p for p in self.built if GTR_ID[p["slug"]].startswith("guitar-notes-")]
        self.assertEqual(len(notes), 4)
        for p in notes:
            with self.subTest(slug=p["slug"]):
                want = copy.deepcopy(self.fx[GTR_ID[p["slug"]]]["chain"])
                if int(want[0].get("style", 0)) in (0, 1, 4):
                    want[0]["double"] = 1
                self.assertEqual(_guitar(p), want)

    def test_electronic_keeps_yue_guitar(self):
        # «Электроника» — гитара YuE2 через цепочку варианта как в fx_presets, без gtrnotes; кроме 8-бит и гитар
        el = [p for p in self.built if p["family"] == ELECTRONIC and p["slug"] != EIGHT_BIT]
        self.assertTrue(el)
        for p in el:
            with self.subTest(slug=p["slug"]):
                eng = _guitar(p)
                self.assertNotIn("gtrnotes", [b.get("type") for b in eng])
                self.assertEqual(eng, self.fx[GTR_ID[p["slug"]]]["chain"])

    def test_eight_bit_has_guitarist(self):
        p = next(x for x in self.built if x["slug"] == EIGHT_BIT)
        self.assertEqual(p["family"], ELECTRONIC)
        self.assertEqual(_guitar(p)[0].get("type"), "gtrnotes")

    def test_non_electronic_all_gtrnotes(self):
        # условие 1 целиком: ни одного не-электронного варианта без гитариста
        for p in self.built:
            if p["family"] == ELECTRONIC:
                continue
            with self.subTest(slug=p["slug"]):
                eng = _guitar(p)
                self.assertTrue(eng and eng[0].get("type") == "gtrnotes", f"гитара без гитариста: {eng[:1]}")
                self.assertEqual([b.get("type") for b in eng].count("gtrnotes"), 1, "gtrnotes дважды")

    def test_guitarist_tape_soft(self):
        # условие 2: лента в цепочке гитариста — wow ≤ 0,15, flutter ≤ 0,1
        checked = 0
        for p in self.built:
            eng = _guitar(p)
            if not eng or eng[0].get("type") != "gtrnotes":
                continue
            for blk in eng:
                if blk.get("type") != "tape":
                    continue
                checked += 1
                with self.subTest(slug=p["slug"]):
                    self.assertLessEqual(float(blk.get("wow", 0)), TAPE_WOW_MAX, blk)
                    self.assertLessEqual(float(blk.get("flutter", 0)), TAPE_FLUTTER_MAX, blk)
        # guitar-lofi (с лентой) стоит в гаражном и панке — проверка не должна пройти вхолостую
        self.assertGreater(checked, 0, "ни одной ленты в цепочках гитариста — проверять нечего")

    def test_note_mentions_plan(self):
        # условие 5: подсказка варианта с гитаристом — про план
        for p in self.built:
            if not _with_guitarist(p):
                continue
            with self.subTest(slug=p["slug"]):
                texts = _texts(p.get("note"))
                self.assertTrue(texts, "нет подсказки")
                self.assertTrue(any("план" in t.lower() for t in texts), f"в подсказке нет «план»: {texts}")


if __name__ == "__main__":
    unittest.main()
