"""Ноты гитары трека — чистая логика (без моделей и сети): ансамбль распознавания и разбор на удары.

Нота — [start, end, midi, vel]: секунды трека, vel 0…1. Распознают две модели (worker/notes_run.py в своём окружении):
YourMT3+ — двумя проходами (второй сдвинут на полкуска: у краёв своих кусков модель ошибается) и basic-pitch.
Ансамбль: pick_centered → union с basic-pitch → clean. Блок движка gtrnotes делит ноты на удары (strikes) и играет
их сэмплами другой гитары.
"""
from __future__ import annotations

UNION_TOL_S = 0.05      # «та же нота» двух моделей: та же высота, начало ближе (допуск оценки распознавания mir_eval)
LADDER_MIN = 4          # «лесенка» — столько коротких нот подряд по полутонам (слайд или шум, расписанный по нотам)
LADDER_NOTE_S = 0.06    # нота лесенки короче
LADDER_GAP_S = 0.03     # начала соседних нот лесенки ближе
LADDER_STEP = (1, 3)    # шаг высоты в лесенке, полутоны
DUP_S = 0.03            # дубль — та же высота, начала ближе
STRIKE_S = 0.03         # удар — ноты с началами ближе к первой ноте группы
MUTED_S = 0.1           # глушёный удар — все ноты короче (≥2 нот)
PALM_S = 0.15           # глушение ладонью — одиночная низкая нота короче
PALM_MIDI = 52          # … и ниже E3


def _note(n) -> list:
    s, e, m, v = n[:4]
    return [float(s), float(e), int(round(float(m))), float(v)]


def _off_center(t: float, shift: float, seg: float) -> float:
    """Расстояние момента t от середины своего куска в проходе, куски которого начинаются в shift + i·seg."""
    return abs(((t - shift) % seg) - seg / 2)


def pick_centered(pass0, pass1, seg: float) -> list:
    """Ноты двух проходов (второй сдвинут на seg/2): каждая нота — из прохода, где её начало ближе к середине своего
    куска; равенство — проход 0. Нота, которую нашёл только «худший» проход, теряется: оба прохода слышат почти
    одно и то же, а у края куска модель как раз и придумывает лишнее."""
    out = [_note(n) for n in pass0 if _off_center(float(n[0]), 0.0, seg) <= _off_center(float(n[0]), seg / 2, seg)]
    out += [_note(n) for n in pass1 if _off_center(float(n[0]), seg / 2, seg) < _off_center(float(n[0]), 0.0, seg)]
    return sorted(out)


def union(main, second, tol: float = UNION_TOL_S) -> list:
    """Все ноты main + ноты second, у которых в main нет ноты той же высоты с началом ближе tol."""
    import bisect
    base = [_note(n) for n in main]
    starts: dict[int, list] = {}
    for n in base:
        starts.setdefault(n[2], []).append(n[0])
    for v in starts.values():
        v.sort()
    added = []
    for n in second:
        n = _note(n)
        st = starts.get(n[2], [])
        i = bisect.bisect_left(st, n[0])
        near = [abs(st[k] - n[0]) for k in (i - 1, i) if 0 <= k < len(st)]
        if not near or min(near) >= tol:
            added.append(n)
    return sorted(base + added)


def clean(notes) -> list:
    """Без «лесенок» (≥LADDER_MIN коротких нот подряд шагом 1–3 полутона с близкими началами — все прочь) и без
    дублей (та же высота, начала ближе DUP_S — одна нота: начало первой, конец и сила — наибольшие)."""
    ns = sorted(_note(n) for n in notes)
    short = [i for i, n in enumerate(ns) if n[1] - n[0] < LADDER_NOTE_S]
    drop = set()
    k = 0
    while k < len(short):
        run = [short[k]]
        j = k + 1
        while j < len(short):
            a, b = ns[run[-1]], ns[short[j]]
            if b[0] - a[0] < LADDER_GAP_S and LADDER_STEP[0] <= abs(b[2] - a[2]) <= LADDER_STEP[1]:
                run.append(short[j])
                j += 1
            else:
                break
        if len(run) >= LADDER_MIN:
            drop.update(run)
            k = j
        else:
            k += 1
    out: list = []
    last: dict[int, list] = {}
    for i, n in enumerate(ns):
        if i in drop:
            continue
        prev = last.get(n[2])
        if prev is not None and n[0] - prev[0] < DUP_S:
            prev[1] = max(prev[1], n[1])
            prev[3] = max(prev[3], n[3])
            continue
        n = list(n)
        out.append(n)
        last[n[2]] = n
    return out


def _direction(t: float, beats) -> str:
    """down — начало ближе к доле, чем к середине между долями; up — иначе; долей нет — down."""
    import bisect
    if len(beats) < 1:
        return "down"
    bs = sorted(float(b) for b in beats)
    i = bisect.bisect_right(bs, t) - 1
    if i < 0:
        return "down" if len(bs) < 2 or t >= bs[0] - (bs[1] - bs[0]) / 4 else "up"
    if i + 1 < len(bs):
        step = bs[i + 1] - bs[i]
    elif i > 0:
        step = bs[i] - bs[i - 1]
    else:
        return "down"
    d_beat = min(t - bs[i], bs[i] + step - t)
    d_mid = abs(t - (bs[i] + step / 2))
    return "down" if d_beat <= d_mid else "up"


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
