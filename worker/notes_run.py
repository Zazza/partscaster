"""Распознавание нот дорожки (запускается воркером в окружении notes — worker/notes_install.sh, отдельной программой:
YourMT3+ под GPL-3.0). Сырые ноты моделей — в JSON; ансамбль и уборку делает воркер (guitarnotes.py).

    python notes_run.py --dir <каталог notes> --audio <дорожка> --out <ноты.json>
    python notes_run.py --dir <каталог notes> --warmup     # проверка установки

Ответ (в --out): {"engines": [...], "basic_pitch": [[start, end, midi, amp], …], "yourmt3": [проход0, проход1] | null,
"seg": длина куска YourMT3+, с | null}; второй проход YourMT3+ — со сдвигом на полкуска.
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import tempfile

YMT3_CKPT = ("amt/logs/2024/mc13_256_g4_all_v7_mt3f_sqr_rms_moe_wf4_n8k2_silu_rope_rp_b36_nops/"
             "checkpoints/last.ckpt")
YMT3_ARGS = ["mc13_256_g4_all_v7_mt3f_sqr_rms_moe_wf4_n8k2_silu_rope_rp_b36_nops@last.ckpt", "-p", "2024",
             "-tk", "mc13_full_plus_256", "-dec", "multi-t5", "-nl", "26", "-enc", "perceiver-tf", "-sqr", "1",
             "-ff", "moe", "-wf", "4", "-nmoe", "8", "-kmoe", "2", "-act", "silu", "-epe", "rope", "-rp", "1",
             "-ac", "spec", "-hop", "300", "-atc", "1", "-pr", "16"]
DRUM_CHANNEL = 9
BP_ARGS = dict(onset_threshold=0.5, frame_threshold=0.3, minimum_note_length=80,
               minimum_frequency=70, maximum_frequency=1400)


def basic_pitch(path: str) -> list:
    from basic_pitch import ICASSP_2022_MODEL_PATH
    from basic_pitch.inference import predict
    _, _, notes = predict(path, ICASSP_2022_MODEL_PATH, **BP_ARGS)
    return sorted([float(s), float(e), int(m), float(a)] for s, e, m, a, *_ in notes)


def ymt3_available(root: str) -> bool:
    ck = os.path.join(root, "ymt3", YMT3_CKPT)
    return os.path.isfile(ck) and os.path.getsize(ck) > 1_000_000


def _midi_notes(path: str, shift: float) -> list:
    import mido
    mf = mido.MidiFile(path)
    out, on, t = [], {}, 0.0
    for msg in mido.merge_tracks(mf.tracks):
        t += mido.tick2second(msg.time, mf.ticks_per_beat, 500000)
        if msg.type == "note_on" and msg.velocity > 0:
            on[(msg.channel, msg.note)] = (t, msg.velocity)
        elif msg.type in ("note_off", "note_on") and (msg.channel, msg.note) in on:
            s, v = on.pop((msg.channel, msg.note))
            # гитару модель часто подписывает фортепиано/органом — берутся все не-барабанные каналы
            if msg.channel != DRUM_CHANNEL and s - shift >= 0:
                out.append([s - shift, t - shift, msg.note, v / 127])
    return sorted(out)


def yourmt3(root: str, path: str) -> dict:
    """YourMT3+ двумя проходами — на видеокарте (нет CUDA — на процессоре). Не хватило видеопамяти — ошибка (её текст
    «CUDA out of memory» воркер показывает как «освободите видеопамять»): молча уйти на процессор нельзя — там трек
    считается десятки минут, а очередь видеокарты воркера всё это время занята. NOTES_DEVICE=cpu — на процессоре явно
    (проверка установки, ручной запуск)."""
    import torch
    dev = os.environ.get("NOTES_DEVICE") or ("cuda" if torch.cuda.is_available() else "cpu")
    return _yourmt3(root, path, dev)


def _yourmt3(root: str, path: str, dev: str) -> dict:
    import numpy as np
    import soundfile as sf
    import torch
    import torchaudio
    code = os.path.join(root, "ymt3")
    sys.path[:0] = [code, os.path.join(code, "amt", "src")]   # model_helper — в корне кода, модель — в amt/src
    os.chdir(code)   # чекпоинт и вывод MIDI модели — относительно каталога кода

    def _load(uri, *a, **k):   # torchaudio 2.10 без torchcodec не читает файлы — через soundfile
        x, sr = sf.read(uri, dtype="float32", always_2d=True)
        return torch.from_numpy(x.T.copy()), sr

    torchaudio.load = _load
    from model_helper import load_model_checkpoint, transcribe   # noqa: E402 — путь к коду модели выше
    args = list(YMT3_ARGS)
    if dev == "cpu":
        args[args.index("-pr") + 1] = "32"
        torch.cuda.is_available = lambda: False   # transcribe модели сама выбирает устройство по этому признаку
    model = load_model_checkpoint(args=args, device="cpu").to(dev)
    inf = model.inference_file
    bsz = int(os.environ.get("NOTES_BSZ", "4"))
    model.inference_file = lambda bsz_=8, audio_segments=None, **kw: inf(bsz=bsz, audio_segments=audio_segments)
    seg = model.audio_cfg["input_frames"] / model.audio_cfg["sample_rate"]
    x, sr = sf.read(path, dtype="float32", always_2d=True)
    passes = []
    with tempfile.TemporaryDirectory() as tmp:
        for k, shift in enumerate((0.0, seg / 2)):
            wav = os.path.join(tmp, f"p{k}.wav")
            pad = np.zeros((int(round(shift * sr)), x.shape[1]), dtype=np.float32)
            sf.write(wav, np.concatenate([pad, x]), sr)
            midi = transcribe(model, {"filepath": wav, "track_name": f"notes_{os.getpid()}_{k}"})
            passes.append(_midi_notes(midi, shift))
            os.unlink(midi)
    return {"seg": seg, "passes": passes}   # _run разворачивает в поля ответа


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--dir", required=True)
    ap.add_argument("--audio")
    ap.add_argument("--out")
    ap.add_argument("--warmup", action="store_true")
    a = ap.parse_args()
    root = os.path.abspath(a.dir)
    if a.warmup:
        os.environ.setdefault("NOTES_DEVICE", "cpu")   # проверка установки — без видеокарты
        import numpy as np
        import soundfile as sf
        with tempfile.TemporaryDirectory() as tmp:
            sr = 22050
            t = np.arange(int(sr * 3)) / sr
            y = (0.3 * np.sin(2 * np.pi * 196.0 * t) * np.exp(-t)).astype(np.float32)   # G3, удар с затуханием
            p = os.path.join(tmp, "warm.wav")
            sf.write(p, y, sr)
            a.audio, a.out = p, os.path.join(tmp, "warm.json")
            res = _run(root, a.audio)
        print(json.dumps({"ok": True, "engines": res["engines"], "basic_pitch": len(res["basic_pitch"])}))
        return
    if not a.audio or not a.out:
        ap.error("нужны --audio и --out")
    res = _run(root, a.audio)
    with open(a.out, "w") as f:
        json.dump(res, f)
    print(json.dumps({"ok": True, "engines": res["engines"], "out": a.out}))


def _run(root: str, audio: str) -> dict:
    audio = os.path.abspath(audio)
    res = {"engines": ["basic-pitch"], "basic_pitch": basic_pitch(audio), "yourmt3": None, "seg": None}
    if ymt3_available(root):
        y = yourmt3(root, audio)
        res.update(yourmt3=y["passes"], seg=y["seg"])
        res["engines"].append("yourmt3")
    return res


if __name__ == "__main__":
    main()
