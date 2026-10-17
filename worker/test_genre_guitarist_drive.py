"""Перегруз гитариста не сильнее 18 дБ (карточка internal-guitarist-drive).

Условия:
1. Во всех встроенных жанровых пресетах (genres.build()), где гитару играет гитарист (цепочка гитары начинается
   с gtrnotes), у каждого блока drive в цепочке гитары gain_db ≤ 18 (GUITARIST_DRIVE_MAX). Блоки drive с
   gain_db ≤ 18 — без изменений; прочие параметры drive (mix, output_db) не меняются; число и порядок блоков
   цепочки не меняются.
2. Префикс фузза у guitar-lofi — eq highpass 100 + drive 18 (было 34), mix 1; второй перегруз кассетной
   обработки guitar-lofi (drive 10, mix 0,4) остаётся.
3. Варианты без гитариста (семья «Электроника», кроме «8-бит и гитары») — цепочка гитары без изменений
   (EBM · марш — guitar-industrial с drive 38).
4. «КИНО» (симфо-метал, дум · траурный) — без изменений (перегруза нет).
5. Готовая цепочка guitar-notes-fuzz — drive 18 вместо 34; guitar-fuzz, guitar-industrial, guitar-symphonic…
   на странице «Инструменты» — без изменений.

Толкования (буквальные, неясности — в отчёте автора тестов):
- «Без изменений» / «число и порядок не меняются» — относительно ожидаемой цепочки
  [gtrnotes] + (префикс фузза у guitar-lofi) + готовая цепочка варианта из fx_presets.json, где у каждого drive
  gain_db = min(было, 18), а остальные ключи drive те же. У блоков tape не сверяются wow/flutter — их
  ограничивает другая задача (test_genre_guitarist, условие 2).
- Обработка варианта — снимок GTR_ID из test_genre_guitarist (id готовой цепочки на коммите 368ccef).
- Готовые цепочки YuE2 (guitar-fuzz/-industrial/-symphonic/-altrock/-shoegaze/-lofi) в fx_presets.json — силы
  drive до задачи (34/38/32/22/20/10): их условие 5 запрещает трогать; снимок — FX_DRIVE_BEFORE.
- GUITARIST_DRIVE_MAX — имя из условия 1; проверяется, что genres экспортирует его со значением 18.
- «КИНО» — как в test_genre_guitarist: варианты «Симфо-метал» и «Дум · траурный».
"""
import json
import unittest
from pathlib import Path

import genres
from test_genre_guitarist import ELECTRONIC, EIGHT_BIT, GTR_ID, _guitar, _is_kino

WORKER = Path(__file__).resolve().parent
DRIVE_MAX = 18.0

# снимок: готовая цепочка → gain_db её блоков drive до задачи (условие 5 — не меняются, кроме notes-fuzz)
FX_DRIVE_BEFORE = {
    "guitar-fuzz": [34], "guitar-industrial": [38], "guitar-symphonic": [32],
    "guitar-altrock": [22], "guitar-shoegaze": [20], "guitar-lofi": [10],
}
# снимок: прочие параметры drive готовых цепочек (mix) — условие 1 их не меняет
FX_DRIVE_MIX = {
    "guitar-fuzz": [1], "guitar-industrial": [1], "guitar-symphonic": [1],
    "guitar-altrock": [0.85], "guitar-shoegaze": [0.7], "guitar-lofi": [0.4],
}


def _fx():
    return {p["id"]: p for p in json.loads((WORKER / "fx_presets.json").read_text(encoding="utf-8"))}


def _drives(chain):
    return [b for b in chain if b.get("type") == "drive"]


def _capped(chain):
    """Ожидаемая цепочка по условию 1: drive с gain_db > 18 → 18, остальное как есть."""
    out = []
    for b in chain:
        b = dict(b)
        if b.get("type") == "drive" and float(b.get("gain_db", 0)) > DRIVE_MAX:
            b["gain_db"] = DRIVE_MAX
        out.append(b)
    return out


def _norm(b):
    """Блок для сравнения: без wow/flutter у ленты, числа как float."""
    skip = ("wow", "flutter") if b.get("type") == "tape" else ()
    return {k: (float(v) if isinstance(v, (int, float)) and not isinstance(v, bool) else v)
            for k, v in b.items() if k not in skip}


def _with_guitarist(p):
    eng = _guitar(p)
    return bool(eng) and eng[0].get("type") == "gtrnotes"


class TestGuitaristDrive(unittest.TestCase):

    def setUp(self):
        self.built = genres.build()
        self.fx = _fx()
        self.by_slug = {p["slug"]: p for p in self.built}

    def _chain(self, slug):
        self.assertIn(slug, self.by_slug, f"нет варианта {slug}")
        return _guitar(self.by_slug[slug])

    def _assert_chain(self, got, want, where):
        self.assertEqual([b.get("type") for b in got], [b.get("type") for b in want], f"{where}: блоки/порядок")
        for i, (g, w) in enumerate(zip(got, want, strict=True)):
            self.assertEqual(_norm(g), _norm(w), f"{where}: блок {i} ({w.get('type')})")

    # --- условие 1 ---

    def test_constant_named(self):
        # условие 1 называет предел GUITARIST_DRIVE_MAX
        self.assertEqual(getattr(genres, "GUITARIST_DRIVE_MAX", None), 18)

    def test_tk6_all_guitarist_drive_le_18(self):
        # ТК6: у всех вариантов с гитаристом все drive ≤ 18
        checked = 0
        for p in self.built:
            if not _with_guitarist(p):
                continue
            for b in _drives(_guitar(p)):
                checked += 1
                with self.subTest(slug=p["slug"]):
                    self.assertLessEqual(float(b.get("gain_db", 0)), DRIVE_MAX, b)
        self.assertGreater(checked, 0, "ни одного drive у гитариста — проверка вхолостую")

    def test_guitarist_chain_capped_otherwise_same(self):
        # условие 1: только gain_db > 18 → 18; прочие параметры, число и порядок блоков — прежние
        for p in self.built:
            if not _with_guitarist(p) or _is_kino(p):
                continue
            gid = GTR_ID[p["slug"]]
            with self.subTest(slug=p["slug"], chain=gid):
                eng = _guitar(p)
                if gid.startswith("guitar-notes-"):
                    # готовая цепочка гитариста целиком (guitar-notes-fuzz — уже с 18 по условию 5); второй дубль —
                    # по манере (internal-guitar-double, условие 6: авто, бой, квинты)
                    want = _capped(self.fx[gid]["chain"])
                    if int(want[0].get("style", 0)) in (0, 1, 4):
                        want[0] = dict(want[0], double=1)
                    self._assert_chain(eng, want, p["slug"])
                    continue
                rest = eng[1:]
                if gid == "guitar-lofi":
                    rest = rest[2:]   # префикс фузза — test_tk2_punk_garage
                self._assert_chain(rest, _capped(self.fx[gid]["chain"]), f"{p['slug']}: обработка {gid}")

    def test_tk1_punk_fast(self):
        # ТК1: «Панк · быстрый» — gtrnotes первым, есть drive, все ≤ 18, первый drive 18 и mix 1
        eng = self._chain("punk-fast")
        self.assertEqual(eng[0].get("type"), "gtrnotes")
        drv = _drives(eng)
        self.assertTrue(drv, "в цепочке нет drive")
        for b in drv:
            self.assertLessEqual(float(b["gain_db"]), DRIVE_MAX, b)
        self.assertAlmostEqual(float(drv[0]["gain_db"]), 18.0)
        self.assertAlmostEqual(float(drv[0]["mix"]), 1.0)

    def test_tk3_industrial_with_guitarist(self):
        # ТК3: индастриал с гитаристом на guitar-industrial — drive 38 → 18, прочие блоки = готовая цепочка
        slugs = [s for s, g in GTR_ID.items() if s.startswith("industrial-") and g == "guitar-industrial"]
        self.assertTrue(slugs)
        src = self.fx["guitar-industrial"]["chain"]
        for slug in slugs:
            with self.subTest(slug=slug):
                eng = self._chain(slug)
                self.assertEqual(eng[0].get("type"), "gtrnotes")
                rest = eng[1:]
                self.assertEqual([b.get("type") for b in rest], [b.get("type") for b in src])
                for g, w in zip(rest, src, strict=True):
                    if w.get("type") == "drive":
                        self.assertAlmostEqual(float(g["gain_db"]), 18.0, msg=f"{slug}: drive {g}")
                        self.assertEqual({k: v for k, v in g.items() if k != "gain_db"},
                                         {k: v for k, v in w.items() if k != "gain_db"}, f"{slug}: прочее у drive")
                    else:
                        self.assertEqual(_norm(g), _norm(w), f"{slug}: {w.get('type')}")

    def test_tk4_altrock_shoegaze(self):
        # ТК4: альт-рок 22 → 18 (mix 0,85), шугейз 20 → 18 (mix 0,7)
        for gid, mix in (("guitar-altrock", 0.85), ("guitar-shoegaze", 0.7)):
            slugs = [s for s, g in GTR_ID.items() if g == gid and _with_guitarist(self.by_slug[s])]
            self.assertTrue(slugs, gid)
            for slug in slugs:
                with self.subTest(slug=slug, chain=gid):
                    drv = _drives(self._chain(slug))
                    self.assertEqual(len(drv), 1, drv)
                    self.assertAlmostEqual(float(drv[0]["gain_db"]), 18.0)
                    self.assertAlmostEqual(float(drv[0]["mix"]), mix)

    # --- условие 2 ---

    def test_tk2_punk_garage(self):
        # ТК2: gtrnotes, eq(hp 100), drive(18, mix 1), затем guitar-lofi с drive 10 mix 0,4
        eng = self._chain("punk-garage")
        self.assertGreaterEqual(len(eng), 3, eng)
        self.assertEqual(eng[0].get("type"), "gtrnotes")
        self.assertEqual(eng[1].get("type"), "eq")
        self.assertAlmostEqual(float(eng[1]["highpass_hz"]), 100.0)
        self.assertEqual(eng[2].get("type"), "drive")
        self.assertAlmostEqual(float(eng[2]["gain_db"]), 18.0)
        self.assertAlmostEqual(float(eng[2]["mix"]), 1.0)
        lofi = self.fx["guitar-lofi"]["chain"]
        self._assert_chain(eng[3:], lofi, "punk-garage: обработка guitar-lofi")
        tail = _drives(eng[3:])
        self.assertEqual(len(tail), 1, tail)
        self.assertAlmostEqual(float(tail[0]["gain_db"]), 10.0)
        self.assertAlmostEqual(float(tail[0]["mix"]), 0.4)

    def test_lofi_prefix_everywhere(self):
        # условие 2: префикс фузза 18 дБ у всех вариантов гитариста на guitar-lofi
        slugs = [s for s, g in GTR_ID.items() if g == "guitar-lofi" and _with_guitarist(self.by_slug[s])]
        self.assertTrue(slugs)
        for slug in slugs:
            with self.subTest(slug=slug):
                eng = self._chain(slug)
                self.assertEqual([b.get("type") for b in eng[1:3]], ["eq", "drive"])
                self.assertAlmostEqual(float(eng[2]["gain_db"]), 18.0)
                self.assertAlmostEqual(float(eng[2]["mix"]), 1.0)

    # --- условие 3 ---

    def test_tk5_ebm_march(self):
        # ТК5: EBM · марш — без гитариста, цепочка = готовая guitar-industrial (drive 38)
        eng = self._chain("ebm-march")
        self.assertNotIn("gtrnotes", [b.get("type") for b in eng])
        self.assertEqual(eng, self.fx["guitar-industrial"]["chain"])
        self.assertEqual([float(b["gain_db"]) for b in _drives(eng)], [38.0])

    def test_tk6_without_guitarist_unchanged(self):
        # ТК6: варианты без гитариста — цепочка = готовая из fx_presets.json
        el = [p for p in self.built if p["family"] == ELECTRONIC and p["slug"] != EIGHT_BIT]
        self.assertTrue(el)
        for p in el:
            with self.subTest(slug=p["slug"]):
                self.assertFalse(_with_guitarist(p))
                self.assertEqual(_guitar(p), self.fx[GTR_ID[p["slug"]]]["chain"])

    def test_eight_bit_capped(self):
        # «8-бит и гитары» (электроника с гитаристом) — тоже ≤ 18
        eng = self._chain(EIGHT_BIT)
        self.assertEqual(eng[0].get("type"), "gtrnotes")
        drv = _drives(eng)
        self.assertTrue(drv)
        for b in drv:
            self.assertLessEqual(float(b["gain_db"]), DRIVE_MAX, b)

    # --- условие 4 ---

    def test_kino_no_drive(self):
        # «КИНО» — без перегруза, как было
        kino = [p for p in self.built if _is_kino(p)]
        self.assertTrue(kino)
        for p in kino:
            with self.subTest(slug=p["slug"]):
                eng = _guitar(p)
                self.assertEqual(eng[0].get("type"), "gtrnotes")
                self.assertFalse({"drive", "amp"} & {b.get("type") for b in eng}, eng)

    # --- условие 5 ---

    def test_tk7_notes_fuzz_18(self):
        # ТК7: готовая guitar-notes-fuzz — drive 18 (worker/fx_presets.json после make mcp-data)
        drv = _drives(self.fx["guitar-notes-fuzz"]["chain"])
        self.assertEqual(len(drv), 1, drv)
        self.assertAlmostEqual(float(drv[0]["gain_db"]), 18.0)

    def test_tk7_yue_chains_untouched(self):
        # ТК7 / условие 5: готовые цепочки гитары YuE2 — прежние силы и mix drive
        for gid, gains in FX_DRIVE_BEFORE.items():
            with self.subTest(chain=gid):
                drv = _drives(self.fx[gid]["chain"])
                self.assertEqual([float(b["gain_db"]) for b in drv], [float(x) for x in gains])
                self.assertEqual([float(b["mix"]) for b in drv], [float(x) for x in FX_DRIVE_MIX[gid]])


if __name__ == "__main__":
    unittest.main()
