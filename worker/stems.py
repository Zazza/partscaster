"""Разделение трека на стемы: BS-Roformer-SW + DrumSep в отдельном окружении (см. sep_run.py) —
подпроцессом, видеопамять освобождается с его концом. Другой модели нет: сбой — ошибка дорожек.
"""
import json
import logging
import os
import shutil
import subprocess
import sys
import time
from pathlib import Path

log = logging.getLogger("partscaster-worker.stems")


def _run(cmd, timeout=None, **kw):
    return subprocess.run(cmd, capture_output=True, text=True, timeout=timeout, **kw)


# запуск внешнего процесса; воркер ставит сюда QUEUE.run_proc — отмена из очереди убивает процесс
run_proc = _run

# основные дорожки: в сумме дают трек — по ним минус и замены
MAIN_STEMS = ("drums", "bass", "other", "vocals")
# подробные дорожки (гитара и клавиши RoFormer-SW): уточнение внутри
# «прочего», в сумму трека не входят (иначе гитара удвоится)
DETAIL_STEMS = ("guitar", "piano")
# части барабанов (DrumSep): уточнение внутри «барабанов»,
# в сумму трека не входят — минус и вклейки их не видят
DRUM_PARTS = ("kick", "snare", "toms", "hh", "ride", "crash")

MODEL_ROFORMER = "bs-roformer-sw"
SEP_TIMEOUT = 1800  # с: 4-минутный трек — около минуты, запас на первую загрузку весов


def _sep_python() -> Path:
    """python отдельного окружения разделения (PARTSCASTER_SEP_PY, по умолчанию ~/sep-venv; на Windows —
    Scripts\\python.exe venv)."""
    if os.environ.get("PARTSCASTER_SEP_PY"):
        return Path(os.environ["PARTSCASTER_SEP_PY"])
    venv = Path.home() / "sep-venv"
    return venv / "Scripts" / "python.exe" if sys.platform == "win32" else venv / "bin" / "python"


def _sep_models() -> Path:
    """Каталог весов RoFormer/DrumSep (PARTSCASTER_SEP_MODELS, по умолчанию ~/sep-models)."""
    return Path(os.environ.get("PARTSCASTER_SEP_MODELS") or Path.home() / "sep-models")


def roformer_available() -> bool:
    """Окружение разделения установлено (python окружения есть)."""
    return _sep_python().is_file()


def _read_as(path: Path, sr: int, n: int):
    """Дорожка модели → стерео float32 в частоте трека sr и длиной n сэмплов."""
    import numpy as np
    import soundfile as sf
    x, xsr = sf.read(str(path), dtype="float32", always_2d=True)
    if x.shape[1] == 1:
        x = np.concatenate([x, x], axis=1)
    if xsr != sr:  # audio-separator пишет в частоте модели (44,1 кГц)
        import librosa
        x = librosa.resample(x.T, orig_sr=xsr, target_sr=sr).T.astype(np.float32)
    if len(x) < n:
        x = np.pad(x, ((0, n - len(x)), (0, 0)))
    return x[:n]


def _distribute_residual(main: dict, mix, sr: int) -> dict:
    """Остаток «трек − сумма основных» раздаётся основным дорожкам пропорционально
    их амплитуде на каждой частоте и в каждый момент (кусками по 20 с). Сумма
    после этого равна треку точно — на этом держатся минус и вклейки; у RoFormer
    без этого в низах терялось до −25 дБ трека."""
    import numpy as np
    import librosa
    names = list(main)
    out = {k: v.copy() for k, v in main.items()}
    n_fft, hop, step = 2048, 512, 20 * sr
    for a in range(0, len(mix), step):
        b = min(len(mix), a + step)
        for ch in range(mix.shape[1]):
            res = mix[a:b, ch] - sum(main[k][a:b, ch] for k in names)
            if not np.any(res):
                continue
            R = librosa.stft(res, n_fft=n_fft, hop_length=hop)
            mags = [np.abs(librosa.stft(main[k][a:b, ch], n_fft=n_fft, hop_length=hop)) for k in names]
            tot = sum(mags) + 1e-9
            parts = [librosa.istft(R * (m / tot), hop_length=hop, length=b - a) for m in mags]
            # что не раздалось (тишина во всех дорожках, края) — в «прочее»
            left = res - sum(parts)
            for k, p in zip(names, parts, strict=True):
                out[k][a:b, ch] += p
            out["other"][a:b, ch] += left
    return out


def _separate_roformer(audio_path: Path, out_dir: Path, sr: int, n: int, mix) -> list:
    """RoFormer-SW подпроцессом → контракт файлов приложения. Исключение — при
    любом сбое основного разделения."""
    import numpy as np
    import soundfile as sf
    tmp = out_dir / ".sep-tmp"
    shutil.rmtree(tmp, ignore_errors=True)
    tmp.mkdir(parents=True)
    try:
        cmd = [str(_sep_python()), str(Path(__file__).parent / "sep_run.py"),
               str(audio_path), str(tmp), str(_sep_models())]
        p = run_proc(cmd, timeout=SEP_TIMEOUT)
        if p.returncode != 0:
            raise RuntimeError(f"sep_run exit {p.returncode}: {p.stderr.strip()[-500:]}")
        res = json.loads(p.stdout.strip().splitlines()[-1])
        got = res.get("stems") or {}
        need = {"vocals", "drums", "bass", "other"}
        if not need <= set(got):
            raise RuntimeError(f"sep_run: нет дорожек {sorted(need - set(got))}")
        st = {k: _read_as(Path(v), sr, n) for k, v in got.items()}
        zero = np.zeros((n, 2), dtype=np.float32)
        # у SW шесть дорожек в сумме дают трек; у приложения «прочее» включает гитару и клавиши
        main = {"drums": st["drums"], "bass": st["bass"], "vocals": st["vocals"],
                "other": st["other"] + st.get("guitar", zero) + st.get("piano", zero)}
        main = _distribute_residual(main, mix, sr)
        names = []
        for name in MAIN_STEMS:
            sf.write(str(out_dir / f"stem-{name}.flac"), main[name], sr)
            names.append(f"stem-{name}.flac")
        for name in DETAIL_STEMS:
            if name in st:
                sf.write(str(out_dir / f"stem-{name}.flac"), st[name], sr)
                names.append(f"stem-{name}.flac")
        if res.get("drum_error"):
            log.warning("DrumSep: части барабанов не выделены (%s): %s", audio_path.name, res["drum_error"])
        for name, f in (res.get("drum_parts") or {}).items():
            if name in DRUM_PARTS:
                sf.write(str(out_dir / f"stem-{name}.flac"), _read_as(Path(f), sr, n), sr)
                names.append(f"stem-{name}.flac")
        return names
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def separate(audio_path: Path, out_dir: Path) -> dict:
    """→ out_dir/stem-{drums,bass,other,vocals}.flac (+ stem-{guitar,piano}.flac и части барабанов
    stem-{kick,snare,toms,hh,ride,crash}.flac — по возможности). Возвращает {stems, seconds, model}.
    Окружения разделения нет или RoFormer упал — RuntimeError с текстом для человека; отмена операции
    очереди (OpCanceled) проходит как есть."""
    import numpy as np
    import soundfile as sf

    audio_path, out_dir = Path(audio_path), Path(out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    data, sr = sf.read(str(audio_path), dtype="float32", always_2d=True)
    if not len(data):
        raise ValueError(f"в файле нет звука: {audio_path.name}")
    if data.shape[1] == 1:
        data = np.concatenate([data, data], axis=1)
    if not roformer_available():
        raise RuntimeError(f"разделение на дорожки (RoFormer) не установлено: нет {_sep_python()} — "
                           "переустановите воркер")
    t0 = time.time()
    # части барабанов от прошлого разделения не должны пережить новое (сбой DrumSep их не перепишет)
    for name in DRUM_PARTS:
        (out_dir / f"stem-{name}.flac").unlink(missing_ok=True)
    try:
        names = _separate_roformer(audio_path, out_dir, sr, len(data), data)
    except (RuntimeError, OSError, ValueError, subprocess.SubprocessError) as e:
        raise RuntimeError(f"разделение на дорожки (RoFormer) не удалось: {e}") from e
    log.info("roformer: %s stems from %s in %.1fs", len(names), audio_path.name, time.time() - t0)
    return {"stems": names, "seconds": round(time.time() - t0, 1), "model": MODEL_ROFORMER}
