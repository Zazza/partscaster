"""Ноты гитары — чистая логика: разбор на удары для блока движка gtrnotes (ноты партии — worker/guitarpart.py,
у фраз страницы «Инструменты» — их готовые ноты). Нота — [start, end, midi, vel]: секунды, vel 0…1.
"""
from __future__ import annotations

STRIKE_S = 0.03         # удар — ноты с началами ближе к первой ноте группы
MUTED_S = 0.1           # глушёный удар — все ноты короче (≥2 нот)
PALM_S = 0.15           # глушение ладонью — одиночная низкая нота короче
PALM_MIDI = 52          # … и ниже E3


def _note(n) -> list:
    s, e, m, v = n[:4]
    return [float(s), float(e), int(round(float(m))), float(v)]


def _direction(t: float, beats) -> str:
    """down — начало ближе к доле, чем к середине между долями; up — иначе; меньше двух долей — down. До первой и
    после последней доли сетка продлевается медианным шагом от крайней доли (вступление без барабанов)."""
    import bisect
    import math
    bs = sorted(float(b) for b in beats)
    if len(bs) < 2:
        return "down"
    i = bisect.bisect_right(bs, t) - 1
    if 0 <= i < len(bs) - 1:
        base, step = bs[i], bs[i + 1] - bs[i]
    else:
        steps = sorted(b - a for a, b in zip(bs, bs[1:]))
        base, step = (bs[0] if i < 0 else bs[-1]), steps[len(steps) // 2]
    if step <= 0:
        return "down"
    ph = (t - base) / step
    ph -= math.floor(ph)
    return "down" if min(ph, 1 - ph) <= abs(ph - 0.5) else "up"


def strikes(notes, beats) -> list:
    """Удары: ноты с началами ближе STRIKE_S к первой ноте группы. Поля: t, notes (снизу вверх), dir
    ("single" | "down" | "up"), muted (≥2 нот, все короче MUTED_S), palm (1 нота ниже PALM_MIDI, короче PALM_S)."""
    ns = sorted(_note(n) for n in notes)
    out = []
    i = 0
    while i < len(ns):
        grp = [ns[i]]
        j = i + 1
        while j < len(ns) and ns[j][0] - ns[i][0] < STRIKE_S:
            grp.append(ns[j])
            j += 1
        grp.sort(key=lambda n: (n[2], n[0]))
        one = len(grp) == 1
        out.append({
            "t": ns[i][0],
            "notes": grp,
            "dir": "single" if one else _direction(ns[i][0], beats),
            "muted": not one and all(n[1] - n[0] < MUTED_S for n in grp),
            "palm": one and grp[0][2] < PALM_MIDI and grp[0][1] - grp[0][0] < PALM_S,
        })
        i = j
    return out
