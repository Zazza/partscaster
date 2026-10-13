"""Тарелки во встроенных жанровых пресетах (genres.build()) — без сжатия и перегруза.

Контракт:
- у частей-тарелок (stems ["hh"], ["ride"], ["crash"]) в engine нет блоков comp и drive
  ни в одном пресете;
- у бочки/малого плотная обработка остаётся: хотя бы у одного пресета engine kick содержит comp;
- комната на тарелке (reverb с decay_s ≤ 0,6 и без gate_ms) — lowpass_hz ≥ 12000;
- гейт-реверб (reverb с gate_ms) и лента (tape) на тарелках допустимы (стиль).

Внешних границ нет — моков нет.
"""
import unittest

import genres

CYMBALS = ({"hh"}, {"ride"}, {"crash"})


def _specs():
    """(slug, stems, engine) по всем записям всех жанровых пресетов."""
    for p in genres.build():
        for s in p.get("specs", []):
            yield p.get("slug"), set(s.get("stems", [])), s.get("engine") or []


class GenreCymbalsTest(unittest.TestCase):

    def test_build_has_cymbal_parts(self):
        # предусловие: тарелки в пресетах есть, иначе проверки ниже ничего не доказывают
        n = sum(1 for _, stems, eng in _specs() if stems in CYMBALS and eng)
        self.assertGreater(n, 0)

    def test_cymbals_no_comp_no_drive(self):
        for slug, stems, eng in _specs():
            if stems not in CYMBALS:
                continue
            for b in eng:
                with self.subTest(slug=slug, stems=sorted(stems), block=b.get("type")):
                    self.assertNotIn(b.get("type"), ("comp", "drive"))

    def test_kick_keeps_comp_somewhere(self):
        has = any("kick" in stems and any(b.get("type") == "comp" for b in eng)
                  for _, stems, eng in _specs())
        self.assertTrue(has, "ни у одного пресета на kick нет comp — плотная обработка бочки пропала")

    def test_cymbal_room_keeps_top(self):
        for slug, stems, eng in _specs():
            if stems not in CYMBALS:
                continue
            for b in eng:
                if b.get("type") != "reverb" or b.get("gate_ms") is not None:
                    continue
                decay = b.get("decay_s")
                if decay is None or decay > 0.6:
                    continue
                with self.subTest(slug=slug, stems=sorted(stems)):
                    self.assertIsNotNone(b.get("lowpass_hz"), "у комнаты на тарелке не задан lowpass_hz")
                    self.assertGreaterEqual(b["lowpass_hz"], 12000)


if __name__ == "__main__":
    unittest.main()
