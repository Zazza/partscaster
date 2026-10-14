"""Второй движок генерации — yue2.cpp (GGML), ЭКСПЕРИМЕНТ: модели, программа, сервер, заявки, разбор ответа.

Модель та же (YuE2-3B), движок другой: меньше видеопамяти (Q8 ~8 ГБ, экономный режим ~4,5 ГБ против ~11 ГБ у
PyTorch) и быстрее. Сид между движками и между моделями yue2.cpp НЕ переносится — другое исполнение; ноты
(semantic) переносятся: продолжение и пересчёт идут по ним.

Сервер yue2.cpp (yue-server) запускает и гасит сам воркер — только 127.0.0.1, свободный порт. Без torch: модуль
грузится и там, где модели нет (тесты, установка). Из командной строки: python engine_cpp.py download <модель>.
"""
from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import json
import os
import re
import socket
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.request
from pathlib import Path

import proclock

# закреплённые версии: программа (проверена опытом) и файлы моделей (автор может перезалить — тот же трек
# тихо зазвучал бы иначе); обновление — осознанно, сменой этих строк
CPP_COMMIT = "c893629"
CPP_REPO = "https://github.com/ServeurpersoCom/yue2.cpp.git"
HF_REPO = "Serveurperso/YuE2-GGUF"
HF_REVISION = "e630f2b8f6aedf8254c3186438633c1b5eeaf80a"
HF_BASE = f"https://huggingface.co/{HF_REPO}/resolve/{HF_REVISION}"

MODELS = {
    "bf16": {"file": "YuE2-3B-BF16.gguf", "size": 7166072352,
             "sha256": "668ca9ffa4622449916aae5068b5737363e7ec186a824cd3d7a1a8a95269f166"},
    "q8": {"file": "YuE2-3B-Q8_0.gguf", "size": 3810232064,
           "sha256": "41121ce97786d7795a325bcf123ca196956c03bb252c9e75384cfc1f2fc19e6b"},
    "q6": {"file": "YuE2-3B-Q6_K.gguf", "size": 2943331072,
           "sha256": "42068d5e57713df11b9f6a5a3751072d9bbefe231d1d088f9c100e026ffa4ef9"},
    "q5": {"file": "YuE2-3B-Q5_K_M.gguf", "size": 2622936832,
           "sha256": "cd3efd250b734a229172800f08f33a893ad1f66b521666e176cdae4a0729b281"},
}
VAE = {"file": "YuE2-Vae-F32.gguf", "size": 530497344,
       "sha256": "93e49dfb1970e89ad64cacb17cf13b5d05f6bb30ef7ed3adae3050bcb728638a"}
MODEL_NAMES = tuple(MODELS)

FPS = 25                # семантических шагов на секунду звука
LATENT_DIM = 64
LOW_VRAM_SEQ = 8192     # экономный режим: контекст модели (КЭШ — главный расход памяти после весов)
# потолок песни в экономном режиме: контекст минус подсказка с планом (у «Gone» 2635 шагов) с запасом — ~3,6 мин;
# сервер резервирует бюджет заранее, больший запрос он отклоняет целиком
LOW_VRAM_BUDGET = 5400
DEFAULT_CFG = 1.5
WAV_FORMAT = "wav24"    # форматы yue-server: mp3 | wav16 | wav24 | wav32
BOUNDARY = b"--yue2-batch-boundary"

ABC_SAMPLING = {"temperature": 0.7, "top_p": 0.9, "top_k": 30, "repetition_penalty": 1.005,
                "penalty_window": 100, "min_tokens": 32, "max_tokens": 4096}


def _semantic_sampling(budget: int, temperature: float, min_tokens: int = 200) -> dict:
    # как у PyTorch-воркера (config.json трека): совпадают настройки — сравнимо и исполнение по характеру
    return {"temperature": temperature, "top_p": 0.95, "top_k": 100, "repetition_penalty": 1.2,
            "penalty_window": 50, "min_tokens": min(min_tokens, budget), "max_tokens": budget}


def models_dir() -> Path:
    data = Path(os.environ.get("PARTSCASTER_DATA_DIR", Path.home() / "partscaster" / "data"))
    return Path(os.environ.get("PARTSCASTER_CPP_MODELS") or data / "yue2cpp-models")


def _file_ok(spec: dict) -> bool:
    p = models_dir() / spec["file"]
    try:
        return p.stat().st_size == spec["size"]
    except OSError:
        return False


def model_installed(name: str) -> bool:
    """Модель и VAE на месте (по размеру — дёшево для /config; сумма проверена при загрузке)."""
    return name in MODELS and _file_ok(MODELS[name]) and _file_ok(VAE)


def models_info() -> dict:
    return {n: {"file": s["file"], "size": s["size"], "installed": model_installed(n)} for n, s in MODELS.items()}


@contextlib.contextmanager
def _download_lock(name: str):
    """Одна загрузка файла за раз — и между потоками воркера, и между воркером и install.sh."""
    with proclock.locked(models_dir() / f".{name}.lock"):
        yield


def _sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 22), b""):
            h.update(chunk)
    return h.hexdigest()


def _fetch(spec: dict, base_url: str, progress_cb) -> None:
    d = models_dir()
    dst = d / spec["file"]
    ok_mark = d / f".{spec['file']}.sha256"
    with _download_lock(spec["file"]):
        if dst.is_file() and dst.stat().st_size == spec["size"]:
            # файл положили не мы (или до отметки) — сумма один раз, дальше по отметке
            if ok_mark.is_file() and ok_mark.read_text().strip() == spec["sha256"]:
                return
            if _sha256(dst) == spec["sha256"]:
                ok_mark.write_text(spec["sha256"])
                return
            dst.unlink()
        tmp = dst.with_name(dst.name + ".part")
        h = hashlib.sha256()
        done = 0
        try:
            with urllib.request.urlopen(f"{base_url.rstrip('/')}/{spec['file']}", timeout=60) as resp, \
                    open(tmp, "wb") as out:
                total = int(resp.headers.get("Content-Length") or spec["size"])
                if progress_cb:
                    progress_cb(spec["file"], 0, total)
                while True:
                    chunk = resp.read1(1 << 20)   # что пришло — сразу: прогресс не ждёт полного мегабайта
                    if not chunk:
                        break
                    out.write(chunk)
                    h.update(chunk)
                    done += len(chunk)
                    if progress_cb:
                        progress_cb(spec["file"], done, total)
            if h.hexdigest() != spec["sha256"]:
                raise RuntimeError(f"{spec['file']}: контрольная сумма не совпала (файл испорчен при загрузке)")
            tmp.replace(dst)
            ok_mark.write_text(spec["sha256"])
        finally:
            tmp.unlink(missing_ok=True)


def download(name: str, progress_cb=None, base_url: str = HF_BASE) -> None:
    """Скачать модель и VAE (что уже есть с верной суммой — без сети). RuntimeError — сумма не сошлась."""
    if name not in MODELS:
        raise ValueError(f"неизвестная модель {name!r} (есть: {', '.join(MODEL_NAMES)})")
    for spec in (MODELS[name], VAE):
        _fetch(spec, base_url, progress_cb)


IS_WINDOWS = os.name == "nt"


def _candidates() -> list[Path]:
    exe = ".exe" if IS_WINDOWS else ""
    out = []
    if os.environ.get("PARTSCASTER_CPP_BIN"):
        out.append(Path(os.environ["PARTSCASTER_CPP_BIN"]))
    out.append(Path(__file__).resolve().parent / "yue2cpp" / f"yue-server{exe}")   # из архива релиза
    home = Path(os.environ.get("PARTSCASTER_HOME", Path.home() / "partscaster"))
    out.append(home / "yue2cpp-src" / "build" / f"yue-server{exe}")               # собран install.sh
    return out


def cuda_lib_dirs() -> list[str]:
    """Каталоги библиотек CUDA из venv воркера: готовой сборке yue2.cpp (CUDA 12.8) нужны cudart и cublas 12 —
    пакет разработки CUDA у пользователя не нужен. Linux: пакеты nvidia-* (ставит PyTorch cu128), lib/.
    Windows: torch/lib (у Windows-сборки torch библиотеки CUDA вшиты туда) и nvidia-*/bin."""
    dirs = []
    if IS_WINDOWS:
        try:
            spec = importlib.util.find_spec("torch")
        except (ImportError, ValueError):
            spec = None
        if spec and spec.submodule_search_locations:
            lib = Path(list(spec.submodule_search_locations)[0]) / "lib"
            if lib.is_dir():
                dirs.append(str(lib))
        for pkg in ("nvidia.cuda_runtime", "nvidia.cublas"):
            dirs += _nvidia_pkg_dirs(pkg, ("bin", "lib"))
        return dirs
    for pkg in ("nvidia.cuda_runtime", "nvidia.cublas"):
        dirs += _nvidia_pkg_dirs(pkg, ("lib",))
    return dirs


def _nvidia_pkg_dirs(pkg: str, subs: tuple[str, ...]) -> list[str]:
    try:
        spec = importlib.util.find_spec(pkg)
    except (ImportError, ValueError):
        return []
    if not (spec and spec.submodule_search_locations):
        return []
    root = Path(list(spec.submodule_search_locations)[0])
    return [str(root / s) for s in subs if (root / s).is_dir()]


def _env(lib_dirs: list[str]) -> dict:
    env = dict(os.environ)
    if not lib_dirs:
        return env
    if IS_WINDOWS:
        env["PATH"] = os.pathsep.join([*lib_dirs, *filter(None, [env.get("PATH")])])
    else:
        env["LD_LIBRARY_PATH"] = os.pathsep.join([*lib_dirs, *filter(None, [env.get("LD_LIBRARY_PATH")])])
    return env


_bin_cache: dict[tuple, bool] = {}


def binary_runs(path: Path) -> bool:
    """Программа запускается (--help): нет библиотек CUDA или не та архитектура — нет.
    Кэш по пути и времени изменения: /config опрашивается часто."""
    try:
        st = path.stat()
    except OSError:
        return False
    key = (str(path), st.st_mtime_ns)
    if key not in _bin_cache:
        try:
            r = subprocess.run([str(path), "--help"], capture_output=True, text=True, timeout=20,
                               env=_env(cuda_lib_dirs()))
            # yue-server на --help печатает Usage и выходит с кодом 1; без библиотек CUDA — ошибка загрузчика
            _bin_cache[key] = r.returncode == 0 or "usage" in (r.stdout + r.stderr).lower()
        except (OSError, subprocess.SubprocessError):
            _bin_cache[key] = False
    return _bin_cache[key]


def find_binary() -> Path | None:
    for p in _candidates():
        if p.is_file() and os.access(p, os.X_OK) and binary_runs(p):
            return p
    return None


def engine_label(name: str) -> str:
    """Метка движка трека (jobs.engine): по ней видно, с каким движком сид повторит исполнение."""
    return f"cpp-{name}"


def model_label(name: str) -> str:
    return f"{HF_REPO}:{MODELS[name]['file']}@yue2.cpp-{CPP_COMMIT}"


def _base(style: str, lyrics: str, cot: str, seed: int, cfg: float, budget: int, temperature: float) -> dict:
    return {
        "style": style, "lyrics": lyrics, "cot": cot or "full",
        # сид один на трек, как у PyTorch: им и ноты, и шум звука
        "lm_seed": int(seed), "seed": int(seed),
        "cfg_scale": float(cfg or DEFAULT_CFG), "steps": 32,
        # длину держит потолок семантики, не секунды
        "duration": 1200.0,
        "abc_sampling": dict(ABC_SAMPLING),
        "semantic_sampling": _semantic_sampling(budget, float(temperature or 1.0)),
        "output_format": WAV_FORMAT,
    }


def _cap(budget: int, low_vram: bool) -> int:
    return min(budget, LOW_VRAM_BUDGET) if low_vram else budget


def build_request(row, budget: int, low_vram: bool, abc: str | None = None, *, style: str | None = None,
                  seed: int | None = None, cfg_default: float = DEFAULT_CFG) -> dict:
    """Заявка трека: стиль/текст/режим плана строки, свой план (req_abc или драматургия), сид, характер, потолок."""
    body = _base(style if style is not None else row["style"], row["lyrics"], row["cot"],
                 seed if seed is not None else (row["seed"] or 0), row["cfg"] or cfg_default,
                 _cap(int(budget), low_vram), row["temperature"])
    if abc:
        body["abc"] = abc
    return body


def continue_request(row, parent_dir: Path, cont_from: float, budget: int, low_vram: bool,
                     abc: str | None = None, *, style: str | None = None, cfg_default: float = DEFAULT_CFG) -> dict:
    """Продолжение с отметки: ноты родителя до отметки подаются как сыгранные, дальше модель пишет сама (новый сид,
    новый план или план родителя) — тот же механизм, что у PyTorch (_continue_song). Звук всей песни пересчитывается."""
    import numpy as np
    tokens = [int(t) for t in np.load(Path(parent_dir) / "semantic.npy")]
    k = max(1, min(len(tokens), round(float(cont_from or 0) * FPS)))
    if abc is None:
        p = Path(parent_dir) / "score.abc"
        abc = p.read_text(encoding="utf-8") if p.is_file() else None
    # бюджет считает и сыгранное; не меньше 200 новых шагов — как у PyTorch
    total = max(int(budget), k + 200)
    body = _base(style if style is not None else row["style"], row["lyrics"], row["cot"], row["seed"] or 0,
                 row["cfg"] or cfg_default, _cap(total, low_vram), row["temperature"])
    body["source_tokens"] = ",".join(map(str, tokens))
    body["source_start"] = 0.0
    body["source_end"] = k / FPS
    if abc:
        body["abc"] = abc
    return body


def plan_request(style: str, lyrics: str, cot: str, seed: int) -> dict:
    """Только план: семантика на 1 шаг — план берётся из ответа (отдельного запроса плана у сервера нет).
    С тем же сидом модель потом пишет по нему тот же план."""
    body = _base(style, lyrics, cot, seed, DEFAULT_CFG, 1, 1.0)
    body["semantic_sampling"]["min_tokens"] = 0
    return body


def parse_multipart(body: bytes) -> dict:
    """Ответ /job?result=1 (один трек): replay JSON (план, ноты, сиды), аудио, латенты [T,64] f32."""
    out = {}
    for part in body.split(BOUNDARY):
        head, sep, payload = part.partition(b"\r\n\r\n")
        if not sep:
            continue
        if payload.endswith(b"\r\n"):
            payload = payload[:-2]
        h = head.lower()
        if b"application/json" in h and "replay" not in out:
            out["replay"] = json.loads(payload)
        elif b"content-type: audio/" in h and "audio" not in out:
            out["audio"] = payload
        elif b"application/octet-stream" in h and "latent" not in out:
            out["latent"] = payload
    if "replay" not in out or "audio" not in out:
        raise RuntimeError("yue2.cpp: в ответе нет трека")
    out.setdefault("latent", b"")
    return out


def semantic_of(replay: dict) -> list[int]:
    s = replay.get("semantic_tokens") or ""
    return [int(t) for t in s.split(",") if t.strip()] if isinstance(s, str) else [int(t) for t in s]


def save_artifacts(job_dir: Path, parts: dict, meta: dict) -> None:
    """Файлы трека — те же, что читает остальной воркер: audio.flac, score.abc, semantic.npy, latent.npy,
    request.json, config.json."""
    import numpy as np
    import soundfile as sf
    job_dir = Path(job_dir)
    audio, sr = sf.read(io.BytesIO(parts["audio"]), dtype="float32", always_2d=True)
    sf.write(str(job_dir / "audio.flac"), audio, sr, subtype="PCM_24")
    replay = parts["replay"]
    abc = replay.get("abc") or ""
    if abc:
        (job_dir / "score.abc").write_text(abc, encoding="utf-8")
    np.save(job_dir / "semantic.npy", np.asarray(semantic_of(replay), dtype=np.int64))
    lat = np.frombuffer(parts.get("latent") or b"", dtype="<f4")
    if lat.size and lat.size % LATENT_DIM == 0:
        np.save(job_dir / "latent.npy", lat.reshape(-1, LATENT_DIM).astype(np.float32))
    req = {k: v for k, v in replay.items() if k not in ("semantic_tokens", "source_tokens")}
    (job_dir / "request.json").write_text(json.dumps(req, ensure_ascii=False, indent=2), encoding="utf-8")
    config = {"backend": "yue2.cpp", "cpp_commit": CPP_COMMIT, **(meta or {})}
    (job_dir / "config.json").write_text(json.dumps(config, ensure_ascii=False, indent=2), encoding="utf-8")


_RE_SCORE = re.compile(r"\[AR\] Score (?:song \d+: )?(\d+)/(\d+)")
_RE_SEM = re.compile(r"\[AR\] Semantic (\d+)/(\d+)")
_RE_SYNTH = re.compile(r"\[(?:NAR|VAE)\]")
_RE_JOB = re.compile(r"\[Server\] Job (\S+?):")


def parse_log_line(line: str, counters: dict) -> None:
    """Строка журнала сервера → ход генерации {phase, sem, budget}. Чужие строки не трогают счётчики:
    смени автор текст — полоса стоит на стадии, генерация не падает."""
    if m := _RE_SEM.search(line):
        counters.update(phase="semantic", sem=int(m[1]), budget=int(m[2]))
    elif _RE_SCORE.search(line):
        counters["phase"] = "plan"
    elif _RE_SYNTH.search(line):
        counters["phase"] = "decode"


class CppOOM(RuntimeError):
    """Нехватка видеопамяти у yue2.cpp (в тексте «CUDA out of memory» — его узнаёт friendly_error воркера)."""


_OOM_MARKS = ("out of memory", "cudamalloc", "failed to allocate", "cuda error 2")


def _free_port() -> int:
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class _LogRouter:
    """Строки журнала yue-server → прогресс и разбор ошибки ОДНОЙ заявки. При подключении сервер шлёт и историю
    прошлых заявок: строки после метки «[Server] Job <id>» чужой заявки пропускаются. Метку своей заявки сервер
    может напечатать раньше, чем прочитан ответ /synth, — до номера заявки строки копятся."""

    def __init__(self, on_log=None):
        self.on_log = on_log
        self._lock = threading.Lock()
        self._job = None
        self._owner = None
        self._pending: list[str] = []
        self._mine: list[str] = []

    def _deliver(self, line: str) -> None:
        if m := _RE_JOB.match(line):
            self._owner = m[1]
        if self._owner is None or self._owner == self._job:
            self._mine.append(line)
            if self.on_log:
                self.on_log(line)

    def feed(self, line: str) -> None:
        with self._lock:
            if self._job is None:
                self._pending.append(line)
            else:
                self._deliver(line)

    def set_job(self, job: str) -> None:
        with self._lock:
            self._job = job
            for line in self._pending:
                self._deliver(line)
            self._pending.clear()

    def lines(self) -> list[str]:
        with self._lock:
            return list(self._mine)


class CppServer:
    """Процесс yue-server: старт с моделью/VAE/режимом, заявки, отмена, декод латентов."""

    def __init__(self, binary, model_path, vae_path, low_vram: bool, lib_dirs: list[str] | None = None,
                 log_path: Path | None = None):
        self.binary, self.model_path, self.vae_path = Path(binary), Path(model_path), Path(vae_path)
        self.low_vram = bool(low_vram)
        self.lib_dirs = list(lib_dirs if lib_dirs is not None else cuda_lib_dirs())
        self.log_path = Path(log_path) if log_path else models_dir() / "yue-server.log"
        self.proc: subprocess.Popen | None = None
        self.port = 0
        self._opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))   # 127.0.0.1 — мимо прокси

    @property
    def url(self) -> str:
        return f"http://127.0.0.1:{self.port}"

    @property
    def running(self) -> bool:
        return self.proc is not None and self.proc.poll() is None

    def key(self) -> tuple:
        return (str(self.binary), str(self.model_path), self.low_vram)

    def start(self, timeout: float = 120) -> None:
        if self.running:
            return
        self.port = _free_port()
        cmd = [str(self.binary), "--host", "127.0.0.1", "--port", str(self.port),
               "--model", str(self.model_path), "--vae", str(self.vae_path)]
        if self.low_vram:
            cmd += ["--max-seq", str(LOW_VRAM_SEQ)]
        self.log_path.parent.mkdir(parents=True, exist_ok=True)
        log = open(self.log_path, "wb")
        # отдельная группа/сессия: на Windows start_new_session запрещён — эквивалент через creationflags
        popen_kwargs = ({"creationflags": subprocess.CREATE_NEW_PROCESS_GROUP} if IS_WINDOWS
                        else {"start_new_session": True})
        try:
            self.proc = subprocess.Popen(cmd, stdout=log, stderr=subprocess.STDOUT, stdin=subprocess.DEVNULL,
                                         env=_env(self.lib_dirs), **popen_kwargs)
        finally:
            log.close()   # дескриптор унаследован процессом
        t0 = time.monotonic()
        while time.monotonic() - t0 < timeout:
            if self.proc.poll() is not None:
                raise RuntimeError(f"yue2.cpp не запустился: {self.log_tail()}")
            try:
                with self._opener.open(self.url + "/health", timeout=2):
                    return
            except (OSError, urllib.error.URLError):
                time.sleep(0.3)
        self.stop()
        raise RuntimeError("yue2.cpp не ответил за отведённое время")

    def stop(self) -> None:
        p, self.proc = self.proc, None
        if p is None or p.poll() is not None:
            return
        p.terminate()
        try:
            p.wait(10)
        except subprocess.TimeoutExpired:
            p.kill()
            p.wait(5)

    def log_tail(self, n: int = 12) -> str:
        try:
            lines = self.log_path.read_text(errors="replace").splitlines()
        except OSError:
            return ""
        keep = [ln for ln in lines if "Step " not in ln]
        return "\n".join(keep[-n:])

    def _call(self, method: str, path: str, data: bytes | None = None, ctype: str = "application/json",
              timeout: float = 60):
        r = urllib.request.Request(self.url + path, method=method, data=data, headers={"Content-Type": ctype})
        with self._opener.open(r, timeout=timeout) as resp:
            return resp.read(), resp.headers.get("Content-Type", "")

    def _job_status(self, job: str, lines=None) -> str:
        """Статус заявки; сервер умер (CUDA обрывает процесс, порт закрыт) — причина из журнала, не «Connection
        refused»: нехватка памяти распознаётся и здесь."""
        try:
            return json.loads(self._call("GET", f"/job?id={job}")[0]).get("status", "")
        except (OSError, urllib.error.URLError):
            if self.proc is not None and self.proc.poll() is None:
                raise
            raise self._failed(lines) from None

    def _failed(self, lines=None) -> RuntimeError:
        """Ошибка заявки по журналу; при нехватке видеопамяти сервер останавливается сам — память освобождается
        на любом пути (генерация, план, превью), следующий вызов поднимет его заново."""
        exc = self._fail(self.low_vram, lines)
        if isinstance(exc, CppOOM):
            self.stop()
        return exc

    def _fail(self, low_vram_hint: bool, job_lines: list[str] | None = None) -> RuntimeError:
        tail = "\n".join([*(job_lines or [])[-12:], self.log_tail()])
        low = tail.lower()
        if any(m in low for m in _OOM_MARKS):
            return CppOOM(f"CUDA out of memory (yue2.cpp): {tail[-400:]}")
        if "exceeds context" in low and low_vram_hint:
            return RuntimeError("экономный режим yue2.cpp: песня не помещается в память модели — сократите текст "
                                "или план, уменьшите длину, либо выключите экономный режим в настройках")
        return RuntimeError(f"yue2.cpp: генерация не удалась: {tail[-600:]}")

    def _follow_logs(self, stop: threading.Event, router: _LogRouter) -> None:
        """Поток журнала (SSE /logs) → router; обрыв журнала генерацию не останавливает."""
        try:
            with self._opener.open(self.url + "/logs", timeout=3600) as resp:
                for raw in resp:
                    if stop.is_set():
                        return
                    s = raw.decode(errors="replace").strip()
                    if s.startswith("data:"):
                        router.feed(s[5:].strip())
        except Exception:  # noqa: BLE001 - журнал — только прогресс и текст ошибки; без него генерация идёт
            return

    def synth(self, body: dict, cancelled=None, on_log=None, poll: float = 1.0) -> dict:
        if not self.running:
            self.start()
        stop = threading.Event()
        router = _LogRouter(on_log)
        th = threading.Thread(target=self._follow_logs, args=(stop, router), daemon=True, name="yue2cpp-logs")
        th.start()
        time.sleep(0.2)   # подключение к журналу — до заявки: строки её начала не теряются
        try:
            raw, _ = self._call("POST", "/synth", json.dumps(body).encode())
            job = json.loads(raw)["id"]
            router.set_job(job)
            while True:
                if cancelled and cancelled():
                    self._call("POST", f"/job?id={job}&cancel=1")
                    t0 = time.monotonic()
                    while self._job_status(job, router.lines()) == "running" and time.monotonic() - t0 < 30:
                        time.sleep(0.3)
                    raise InterruptedError("canceled")
                st = self._job_status(job, router.lines())
                if st == "done":
                    return parse_multipart(self._call("GET", f"/job?id={job}&result=1", timeout=300)[0])
                if st == "cancelled":
                    raise InterruptedError("canceled")
                if st == "failed":
                    th.join(1.0)   # последние строки журнала (причина) — в разбор ошибки
                    raise self._failed(router.lines())
                if not self.running:
                    raise self._failed(router.lines())
                time.sleep(poll)
        finally:
            stop.set()

    def decode(self, latents):
        """Латенты [T,64] → стерео 48 кГц (превью куска без генерации)."""
        import numpy as np
        import soundfile as sf
        z = np.ascontiguousarray(np.asarray(latents, dtype="<f4"))
        b = b"----partscaster-vae"
        req = json.dumps({"output_format": WAV_FORMAT}).encode()
        data = b"".join([
            b"--" + b + b"\r\n", b'Content-Disposition: form-data; name="request"; filename="request.json"\r\n',
            b"Content-Type: application/json\r\n\r\n", req, b"\r\n",
            b"--" + b + b"\r\n", b'Content-Disposition: form-data; name="src_latents"; filename="z.f32"\r\n',
            b"Content-Type: application/octet-stream\r\n\r\n", z.tobytes(), b"\r\n", b"--" + b + b"--\r\n"])
        raw, _ = self._call("POST", "/vae", data, ctype="multipart/form-data; boundary=" + b.decode())
        job = json.loads(raw)["id"]
        while (st := self._job_status(job)) == "running":
            time.sleep(0.2)
        if st != "done":
            raise self._failed()
        body, ctype = self._call("GET", f"/job?id={job}&result=1", timeout=120)
        if "multipart" in ctype:
            body = parse_multipart(body)["audio"]
        audio, _ = sf.read(io.BytesIO(body), dtype="float32", always_2d=True)
        return audio


def _cli(argv: list[str]) -> int:
    if len(argv) != 2 or argv[0] != "download":
        print("usage: engine_cpp.py download <bf16|q8|q6|q5>", file=sys.stderr)
        return 2
    name = argv[1]
    if name not in MODELS:
        print(f"неизвестная модель {name!r} (есть: {', '.join(MODEL_NAMES)})", file=sys.stderr)
        return 2
    last = [0.0]

    def show(file, done, total):
        if time.monotonic() - last[0] > 5 or done == total:
            last[0] = time.monotonic()
            print(f"  {file}: {done / 2**30:.2f} / {total / 2**30:.2f} ГБ", flush=True)

    try:
        download(name, show)
    except (OSError, RuntimeError) as e:
        print(f"модель {name} не скачана: {e}", file=sys.stderr)
        return 1
    print(f"модель {name} на месте: {models_dir()}")
    return 0


if __name__ == "__main__":
    sys.exit(_cli(sys.argv[1:]))
