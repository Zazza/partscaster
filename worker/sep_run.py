"""Разделение трека BS-Roformer-SW (+ барабаны по частям, DrumSep) — запускается
воркером подпроцессом в ОТДЕЛЬНОМ окружении (`PARTSCASTER_SEP_PY`, по умолчанию
~/sep-venv): python-audio-separator тянет свой torch/onnxruntime, в окружении
YuE2 они бы конфликтовали. Подпроцесс к тому же сам освобождает видеопамять.

    python sep_run.py <аудио> <каталог_выхода> <каталог_весов>
    python sep_run.py download <каталог_весов>   — только скачать веса обеих моделей (установка; без видеокарты)
    python sep_run.py ready <каталог_весов>      — код 0, если веса обеих моделей на месте целиком (без сети)

Пишет в каталог выхода сырые дорожки модели как есть (<имя>.wav, частота
модели) и печатает последней строкой JSON:
    {"stems": {"vocals": путь, ...}, "drum_parts": {"kick": путь, ...},
     "drum_error": "" | текст}
Сбой DrumSep не роняет основное разделение (drum_parts пусто, drum_error).
Веса (некоммерческие, CC BY-NC-SA) скачивает установщик (download); нет — качаются при первом вызове.
"""
import json
import os
import shutil
import sys
from pathlib import Path

# vocals/drums/bass/guitar/piano/other; в сумме ≈ трек (точно — после stems._distribute_residual)
MAIN_MODEL = "BS-Roformer-SW.ckpt"
DRUM_MODEL = "MDX23C-DrumSep-aufr33-jarredou.ckpt"  # kick/snare/toms/hh/ride/crash
# веса целиком — размер и конфиг модели рядом (сверено по скачанным на GPU-машине 2026-10-06): веса, скачанные до
# отметок .ok, признаются без повторной загрузки, оборванный файл (меньше) — нет
KNOWN_WEIGHTS = {MAIN_MODEL: (699412152, "BS-Roformer-SW.yaml"),
                 DRUM_MODEL: (437652699, "config_drumsep_mdx23c.yaml")}


def _ensure_ffmpeg(models_dir: Path) -> None:
    """audio-separator требует программу ffmpeg (Separator проверяет её при создании, pydub ею пишет звук). Нет
    системной — копия из пакета imageio-ffmpeg (ffmpeg внутри колеса, без сети) под именем ffmpeg в каталоге весов
    (туда процесс пишет всегда) и в PATH процесса: на Windows и чистом Linux ffmpeg обычно не стоит."""
    if shutil.which("ffmpeg"):
        return
    import imageio_ffmpeg
    exe = Path(imageio_ffmpeg.get_ffmpeg_exe())
    bin_dir = models_dir / ".ffmpeg-bin"
    target = bin_dir / ("ffmpeg.exe" if sys.platform == "win32" else "ffmpeg")
    if not target.is_file():
        bin_dir.mkdir(parents=True, exist_ok=True)
        tmp = target.with_name(target.name + ".tmp")
        shutil.copy2(exe, tmp)
        os.replace(tmp, target)
    os.environ["PATH"] = str(bin_dir) + os.pathsep + os.environ.get("PATH", "")


def _run(model: str, src: Path, out_dir: Path, models_dir: Path, prefix: str) -> dict:
    from audio_separator.separator import Separator
    _ensure_ffmpeg(models_dir)
    # куски пачкой по 4: на свободной видеокарте ~в 1,4 раза быстрее, чем по одному
    sep = Separator(output_dir=str(out_dir), model_file_dir=str(models_dir),
                    output_format="WAV", log_level=40,
                    mdxc_params={"segment_size": 256, "override_model_segment_size": False,
                                 "batch_size": 4, "overlap": None, "pitch_shift": 0})
    sep.load_model(model_filename=model)
    out = {}
    for f in sep.separate(str(src)):
        p = Path(f) if os.path.isabs(f) else out_dir / f
        # имя дорожки — в скобках: «трек_(Guitar)_BS-Roformer-SW.wav»
        name = p.name.rsplit("(", 1)[-1].split(")")[0].lower()
        dst = out_dir / f"{prefix}{name}.wav"
        os.replace(p, dst)
        out[name] = str(dst)
    return out


def _complete(models_dir: Path, model: str) -> bool:
    """Веса model на месте целиком: файл и конфиг есть и либо отметка .ok (скачано целиком), либо (скачаны до
    отметок) полный размер. Одна отметка без файлов — не на месте (файлы удалили)."""
    size, conf = KNOWN_WEIGHTS[model]
    ckpt = models_dir / model
    if not (ckpt.is_file() and (models_dir / conf).is_file()):
        return False
    return (models_dir / f".{model}.ok").is_file() or ckpt.stat().st_size == size


def ready(models_dir: Path) -> bool:
    """Обе модели на месте — разделению ничего не нужно качать."""
    return all(_complete(models_dir, m) for m in (MAIN_MODEL, DRUM_MODEL))


def download(models_dir: Path) -> None:
    """Веса обеих моделей в каталог весов; скачанное целиком помечено .<модель>.ok и не качается заново.
    audio-separator пишет файл сразу на место и потом верит одному его наличию — оборванная загрузка выглядела бы
    готовой; поэтому качаем во временный каталог рядом и переносим, только когда всё скачалось."""
    from audio_separator.separator import Separator
    models_dir.mkdir(parents=True, exist_ok=True)
    _ensure_ffmpeg(models_dir)
    for model in (MAIN_MODEL, DRUM_MODEL):
        mark = models_dir / f".{model}.ok"
        if _complete(models_dir, model):
            mark.touch()   # на месте целиком (и скачанные до отметок — тоже)
            print(f"веса {model}: на месте", flush=True)
            continue
        mark.unlink(missing_ok=True)   # отметка без файлов весов — не в счёт: качаем заново
        tmp = models_dir / f".{model}.partial"
        shutil.rmtree(tmp, ignore_errors=True)   # остаток оборванной загрузки
        tmp.mkdir()
        Separator(model_file_dir=str(tmp), log_level=40).download_model_files(model)
        for f in tmp.iterdir():
            os.replace(f, models_dir / f.name)
        tmp.rmdir()
        mark.touch()
        print(f"веса {model}: скачаны", flush=True)


def main(argv: list[str]) -> int:
    if len(argv) == 3 and argv[1] == "ready":
        return 0 if ready(Path(argv[2])) else 1
    if len(argv) == 3 and argv[1] == "download":
        try:
            download(Path(argv[2]))
        except Exception as e:  # noqa: BLE001 — установщику нужен код и текст, а не стек
            print(f"веса разделения не скачаны: {type(e).__name__}: {e}", file=sys.stderr)
            return 1
        return 0
    src, out_dir, models_dir = Path(argv[1]), Path(argv[2]), Path(argv[3])
    out_dir.mkdir(parents=True, exist_ok=True)
    models_dir.mkdir(parents=True, exist_ok=True)
    stems = _run(MAIN_MODEL, src, out_dir, models_dir, "")
    parts, err = {}, ""
    if "drums" in stems:
        try:
            parts = _run(DRUM_MODEL, Path(stems["drums"]), out_dir, models_dir, "drums-")
        except Exception as e:  # noqa: BLE001 — части барабанов необязательны
            err = f"{type(e).__name__}: {e}"
    print(json.dumps({"stems": stems, "drum_parts": parts, "drum_error": err}))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
