"""Партия гитариста «вписывается по аккордам»: удары — моменты настоящих ударов старой гитары трека, аккорд удара —
аккорд плана в этом такте звука (сетка chord_grid), манера — бой, перебор, бас + аккорд, квинты или авто по части
песни. Чистая логика + разбор звука (librosa); ноты партии играет блок движка gtrnotes сэмплами другой гитары.

Нота — [start, end, midi, vel] (секунды трека). Распознавания нот нет: на тихой и переборной гитаре оно слышит
звенящие струны как новые ноты, а аккорд плана с ударами самой гитары звучит уместно (опыт #958–#972)."""
from __future__ import annotations

import re

AUTO, STRUM, ARPEGGIO, BASS_CHORD, POWER = 0, 1, 2, 3, 4
STYLES = (AUTO, STRUM, ARPEGGIO, BASS_CHORD, POWER)
MIN_NOTE_S = 0.15    # нота партии не короче: короче MUTED_S блок принял бы удар за глушёный
RING_MAX_S = 2.0     # нота боя звенит до следующего удара, но не дольше
VEL = 0.8            # силу удара блок берёт из громкости старой гитары; здесь — заглушка
ARP_ORDER = (0, 2, 3, 4, 3, 2)   # перебор: бас → вверх → вниз (индексы струн аппликатуры)
AUTO_SECTION = {"verse": ARPEGGIO, "chorus": STRUM, "pre-chorus": STRUM, "bridge": STRUM}
STRONG_TOL = 0.25    # бас + аккорд: удар ближе стольких долей к доле 1 или 3 — басовая нота

# открытые аккорды, midi снизу вверх (E2 = 40)
OPEN = {
    "G": [43, 47, 50, 55, 59, 67], "C": [48, 52, 55, 60, 64], "D": [50, 57, 62, 66],
    "Em": [40, 47, 52, 55, 59, 64], "Am": [45, 52, 57, 60, 64], "E": [40, 47, 52, 56, 59, 64],
    "A": [45, 52, 57, 61, 64], "Dm": [50, 57, 62, 65], "F": [41, 48, 53, 57, 60, 65],
}
_ROOT = {"C": 0, "D": 2, "E": 4, "F": 5, "G": 7, "A": 9, "B": 11}
_CHORD_RE = re.compile(r"^([A-Ga-g])([#b]?)(.*)$")


def voicing(chord: str | None) -> list[int]:
    """Аккорд плана → аппликатура (midi снизу вверх). Хвост после мажора/минора (7, sus, /бас) не учитывается:
    гитарист «вписывается» трезвучием. Открытые — для частых аккордов, прочие — баррэ от тоники на нижних струнах."""
    m = _CHORD_RE.match((chord or "").strip())
    if not m:
        return []
    letter, acc, rest = m.group(1).upper(), m.group(2), m.group(3)
    minor = rest.startswith("m") and not rest.startswith("maj")
    key = letter + acc + ("m" if minor else "")
    if key in OPEN:
        return list(OPEN[key])
    pc = (_ROOT[letter] + {"#": 1, "b": -1, "": 0}[acc]) % 12
    r = 40 + (pc - 4) % 12          # тоника на нижних струнах: E2…D#3
    third = 3 if minor else 4
    return [r, r + 7, r + 12, r + 12 + third, r + 19, r + 24]


def _bar_index(t: float, bars: list) -> int | None:
    if not bars or t < float(bars[0]["start"]):
        return None
    for i, b in enumerate(bars):
        if float(b["start"]) <= t < float(b["end"]):
            return i
    return len(bars) - 1 if t >= float(bars[-1]["start"]) else None


def _bar_voicing(i: int, bars: list) -> list[int]:
    """Аппликатура такта; такт без понятного аккорда — аккорд ближайшего прежнего."""
    for k in range(i, -1, -1):
        v = voicing(bars[k].get("chord"))
        if v:
            return v
    return []


def _manner(style: int, section: str | None) -> int:
    if style in (STRUM, ARPEGGIO, BASS_CHORD, POWER):
        return style
    return AUTO_SECTION.get((section or "").strip().lower(), BASS_CHORD)


def part(onsets, bars, style: int) -> list:
    """Удары (секунды трека) × такты {start, end, chord, section} → ноты партии [[start, end, midi, vel]]."""
    ons = sorted(float(t) for t in onsets)
    out: list = []
    arp_k, arp_bar = 0, None
    for j, t in enumerate(ons):
        i = _bar_index(t, bars)
        if i is None:
            continue
        v = _bar_voicing(i, bars)
        if not v:
            continue
        bar = bars[i]
        b0, b1 = float(bar["start"]), float(bar["end"])
        nxt = ons[j + 1] if j + 1 < len(ons) else t + RING_MAX_S
        ring = max(min(nxt, t + RING_MAX_S), t + MIN_NOTE_S)
        hold = max(min(b1, t + RING_MAX_S), t + MIN_NOTE_S)   # перебор и бас звенят до конца такта
        beat = (b1 - b0) / 4 if b1 > b0 else 0.5
        pos = (t - b0) / beat
        manner = _manner(int(style or 0), bar.get("section"))
        if manner == STRUM:
            out += [[t, ring, m, VEL] for m in v]
        elif manner == POWER:
            r = v[0]
            out += [[t, ring, m, VEL] for m in (r, r + 7, r + 12)]
        elif manner == ARPEGGIO:
            if arp_bar != i or pos < STRONG_TOL:
                arp_k = 0
            arp_bar = i
            idx = min(ARP_ORDER[arp_k % len(ARP_ORDER)], len(v) - 1)
            arp_k += 1
            out.append([t, hold, v[idx], VEL])
        else:   # BASS_CHORD
            if min(abs(pos - 0), abs(pos - 2), abs(pos - 4)) <= STRONG_TOL:
                r = v[0]
                if round(pos) % 4 == 2:   # вторая половина такта — квинта
                    r = r + 7 if r + 7 <= v[-1] else r - 5
                out.append([t, hold, r, VEL])
            else:
                upper = v[2:] if len(v) > 4 else v[1:]
                out += [[t, ring, m, VEL] for m in upper]
    return out


def onsets(guitar, sr: int) -> list:
    """Моменты ударов старой гитары: пик огибающей начала (не начало подъёма — с пиком новая гитара ложится в сетку
    барабанов: 60 % против 31–36 % у backtrack на #892)."""
    import librosa
    import numpy as np
    x = np.asarray(guitar, dtype=np.float32)
    x = x.mean(axis=1) if x.ndim == 2 else x
    if not np.any(np.abs(x) > 1e-6):
        return []
    on = librosa.onset.onset_detect(y=x, sr=sr, hop_length=256, units="time", backtrack=False)
    return [round(float(t), 4) for t in np.atleast_1d(on)]


def beats(drums, sr: int) -> list:
    """Доли — по дорожке барабанов (или миксу): для направления удара (вниз на долю, вверх между)."""
    import librosa
    import numpy as np
    x = np.asarray(drums, dtype=np.float32)
    x = x.mean(axis=1) if x.ndim == 2 else x
    if not np.any(np.abs(x) > 1e-6):
        return []
    _, bt = librosa.beat.beat_track(y=x, sr=sr, units="time")
    return [round(float(b), 4) for b in np.atleast_1d(bt)]


def tuning(stems: list, sr: int) -> float:
    """Строй трека, центы от A440: основной тон (pyin) баса и голоса — по спектру гитары врёт на ±50 центов."""
    import librosa
    import numpy as np
    devs = []
    for x, lo, hi in stems:
        x = np.asarray(x, dtype=np.float32)
        x = x.mean(axis=1) if x.ndim == 2 else x
        mid = len(x) // 2
        x = x[max(0, mid - sr * 45):mid + sr * 45]
        if not np.any(np.abs(x) > 1e-6):
            continue
        z = librosa.resample(x, orig_sr=sr, target_sr=16000)
        f0, vf, _ = librosa.pyin(z, fmin=lo, fmax=hi, sr=16000, frame_length=2048)
        m = librosa.hz_to_midi(f0[vf & ~np.isnan(f0)])
        devs.append((m - np.round(m)) * 100)
    d = np.concatenate(devs) if devs else np.array([])
    return round(float(np.median(d)), 1) if len(d) >= 20 else 0.0
