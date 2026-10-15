"""Распознавание нот гитары трека — необязательная часть воркера (как Seed-VC): своё окружение
(worker/notes_install.sh) с basic-pitch и YourMT3+ (GPL-3.0 — вызывается отдельной программой notes_run.py).
Воркер зовёт run(), собирает ансамбль (guitarnotes) и кладёт результат у трека: jobs/<id>/guitar-notes.json —
{"version", "engines", "notes": [[s, e, m, v]…], "beats", "tune_cents"}; его играет блок движка gtrnotes."""
from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path

NOTES_DIR = Path(os.environ.get("PARTSCASTER_NOTES_DIR", Path.home() / "partscaster" / "notes"))
RUNNER = Path(__file__).with_name("notes_run.py")
YMT3_CKPT = ("ymt3/amt/logs/2024/mc13_256_g4_all_v7_mt3f_sqr_rms_moe_wf4_n8k2_silu_rope_rp_b36_nops/"
             "checkpoints/last.ckpt")
CACHE_NAME = "guitar-notes.json"
CACHE_VERSION = 1
RUN_TIMEOUT_S = 3 * 3600   # без видеокарты YourMT3+ на 5-минутном треке — десятки минут
VEL_WIN_S = 0.08           # сила ноты — громкость дорожки гитары столько от начала
VEL_RANGE_DB = 24.0        # сила 0…1 — громкость ±12 дБ вокруг средней громкости дорожки


def _py() -> Path:
    return NOTES_DIR / "venv" / "bin" / "python"


def available() -> bool:
    """Окружение распознавания установлено (worker/notes_install.sh)."""
    return _py().is_file()


def engines() -> list:
    """Какие модели есть: basic-pitch — всегда с окружением; YourMT3+ — если скачан его чекпоинт."""
    if not available():
        return []
    ck = NOTES_DIR / YMT3_CKPT
    return ["basic-pitch"] + (["yourmt3"] if ck.is_file() and ck.stat().st_size > 1_000_000 else [])


def run(stem_path, out_json) -> dict:
    """notes_run.py интерпретатором окружения → сырые ноты моделей (и они же — в out_json). Ошибка подпроцесса —
    RuntimeError с хвостом вывода (нехватка видеопамяти — «CUDA out of memory»)."""
    r = subprocess.run(
        [str(_py()), str(RUNNER), "--dir", str(NOTES_DIR), "--audio", str(stem_path), "--out", str(out_json)],
        capture_output=True, text=True, timeout=RUN_TIMEOUT_S)
    if r.returncode != 0:
        tail = (r.stderr or r.stdout).strip().splitlines()[-5:]
        raise RuntimeError("notes: " + " | ".join(tail))
    return json.loads(Path(out_json).read_text())


def ensemble(raw: dict) -> list:
    """Ноты ансамбля: YourMT3+ (два прохода — pick_centered) ∪ basic-pitch, затем уборка; без YourMT3+ — basic-pitch."""
    import guitarnotes
    bp = raw.get("basic_pitch") or []
    passes = raw.get("yourmt3")
    if passes and raw.get("seg"):
        main = guitarnotes.pick_centered(passes[0], passes[1] if len(passes) > 1 else [], float(raw["seg"]))
        return guitarnotes.clean(guitarnotes.union(main, bp))
    return guitarnotes.clean(bp)


def velocities(notes: list, guitar, sr: int) -> list:
    """Сила ноты — громкость дорожки гитары VEL_WIN_S от её начала к средней громкости звучащей дорожки."""
    import numpy as np
    x = np.asarray(guitar, dtype=np.float64)
    x = x.mean(axis=1) if x.ndim == 2 else x
    w = max(1, int(VEL_WIN_S * sr))
    frames = [np.sqrt(np.mean(x[i:i + w] ** 2)) for i in range(0, max(1, len(x) - w), w)]
    loud = [f for f in frames if f > 1e-6]
    ref = float(np.median(loud)) if loud else 1e-6
    out = []
    for n in notes:
        i = int(n[0] * sr)
        r = float(np.sqrt(np.mean(x[i:i + w] ** 2))) if i < len(x) else 0.0
        db = 20 * np.log10(max(r, 1e-9) / ref)
        out.append([n[0], n[1], n[2], float(np.clip(0.5 + db / VEL_RANGE_DB, 0.0, 1.0))])
    return out


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
