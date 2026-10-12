"""Спецификация модуля engine_cpp — второй движок генерации yue2.cpp (internal-own-track,
этап 20, условия 157–159, ТК185–ТК188; договор интерфейсов этапа 20).

Что должно работать:
M1. MODELS — закреплённый список bf16/q8/q6/q5 {file, sha256, size} + общий VAE, репозиторий
    Serveurperso/YuE2-GGUF, ревизия e630f2b8…, коммит программы c893629 (условие 157, 158).
M2. models_dir(): $YUE_CPP_MODELS, иначе <данные>/yue2cpp-models.
    model_installed(name): файл модели И VAE есть с нужным размером.
M3. download(name, progress_cb, base_url): качает недостающее (модель и VAE) через .part,
    сверяет sha256 и переименовывает; сумма не сошлась → RuntimeError, нет ни файла, ни .part;
    готовый файл с верной суммой — без обращения к источнику (ТК185).
M4. find_binary(): $YUE_CPP_BIN → <каталог воркера>/yue2cpp/yue-server → $YUE_HOME/yue2cpp-src/
    build/yue-server; нет — None. cuda_lib_dirs(): каталоги lib пакетов nvidia из venv (ТК186).
M5. CppServer: запуск только на 127.0.0.1 с --model/--vae; экономный режим → --max-seq 8192;
    LD_LIBRARY_PATH содержит переданные lib_dirs; stop() останавливает процесс; synth() —
    POST /synth, опрос /job, результат multipart; отмена → POST /job?cancel=1 и InterruptedError;
    failed → RuntimeError с хвостом журнала; строки журнала уходят в on_log (ТК186, ТК189).
M6. build_request / continue_request / plan_request — перевод заявки (ТК187, поправка:
    output_format "wav24" — у yue-server форматы mp3|wav16|wav24|wav32).
M7. parse_multipart / save_artifacts / parse_log_line — разбор ответа и журнала (ТК188).
M8. engine_label / model_label — метки движка для jobs.engine и /version (условие 160).

Внешняя граница — программа yue-server и HTTP-источник моделей: обе подменяются
(скрипт-фейк, локальный http.server). Сеть и GPU не нужны.

Написаны по карточке и договору, без чтения реализации engine_cpp.
Запуск из каталога worker: python -m unittest test_engine_cpp -v
"""
import array
import hashlib
import http.server
import io
import json
import math
import os
import signal
import sqlite3
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import wave
from pathlib import Path
from unittest import mock

try:
    import numpy  # noqa: F401
    import soundfile  # noqa: F401
    _HAS_NUMPY = True
except ImportError:
    _HAS_NUMPY = False

WORKER_DIR = Path(__file__).resolve().parent
BOUNDARY = "yue2-batch-boundary"
ALLOWED_FORMATS = ("mp3", "wav16", "wav24", "wav32")
MODEL_FILES = {"bf16": "YuE2-3B-BF16.gguf", "q8": "YuE2-3B-Q8_0.gguf",
               "q6": "YuE2-3B-Q6_K.gguf", "q5": "YuE2-3B-Q5_K_M.gguf"}
VAE_FILE = "YuE2-Vae-F32.gguf"
LATENT_T = 10          # кадров латентов в ответе фейка
WAV_SR = 48000
WAV_FRAMES = WAV_SR // 2
DEFAULT_ABC = "X:1\nT:fake\nM:4/4\nL:1/8\nK:C\nV:Vocal\nCDEF GABc|\n"


# ---------------------------------------------------------------- фейк yue-server

_FAKE_SERVER = r'''#!__PYTHON__
"""Фейк yue-server для тестов: пишет аргументы/окружение и отвечает как yue-server."""
import array, io, json, math, os, sys, threading, time, wave
from http.server import ThreadingHTTPServer, BaseHTTPRequestHandler
from urllib.parse import urlparse, parse_qs

HERE = os.path.dirname(os.path.abspath(__file__))
ALLOWED = ("mp3", "wav16", "wav24", "wav32")
B = "yue2-batch-boundary"


def rec(name, obj):
    with open(os.path.join(HERE, name), "a") as f:
        f.write(json.dumps(obj) + "\n")


def scenario():
    try:
        with open(os.path.join(HERE, "scenario.json")) as f:
            return json.load(f)
    except Exception:
        return {}


argv = sys.argv[1:]
SC = scenario()
if "--help" in argv or "-h" in argv:
    if SC.get("broken"):
        sys.exit(1)
    print("usage: yue-server --model M --vae V [--host H] [--port P] [--max-seq N]")
    sys.exit(0)
if SC.get("broken"):
    sys.exit(1)


def opt(name, default):
    for i, a in enumerate(argv):
        if a == name and i + 1 < len(argv):
            return argv[i + 1]
        if a.startswith(name + "="):
            return a.split("=", 1)[1]
    return default


rec("starts.jsonl", {"argv": argv, "ld": os.environ.get("LD_LIBRARY_PATH", ""), "pid": os.getpid()})
threading.Timer(float(SC.get("lifetime", 180)), lambda: os._exit(0)).start()   # страховка от сирот

LOCK = threading.Lock()
JOBS = {}
STATE = {"n": 0, "logs_sent": False}


def wav_bytes():
    buf = io.BytesIO()
    with wave.open(buf, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(48000)
        fr = array.array("h")
        for i in range(24000):
            v = int(6000 * math.sin(2 * math.pi * 220 * i / 48000))
            fr.append(v)
            fr.append(v)
        w.writeframes(fr.tobytes())
    return buf.getvalue()


def latent_bytes():
    return array.array("f", [i * 0.5 for i in range(10 * 64)]).tobytes()


def status_of(j):
    if j["cancelled"]:
        return "cancelled"
    mode = SC.get("mode", "done")
    if mode == "hold":
        return "done" if os.path.exists(os.path.join(HERE, "release")) else "running"
    ready = j["polls"] >= int(SC.get("polls", 2)) and (
        STATE["logs_sent"] or time.time() - j["t0"] > 5 or not SC.get("log_lines"))
    if not ready:
        return "running"
    return "failed" if mode == "failed" else "done"


class H(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, *a):
        pass

    def _send(self, code, body, ctype="application/json"):
        if isinstance(body, (dict, list)):
            body = json.dumps(body).encode()
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def _q(self):
        u = urlparse(self.path)
        return u.path, {k: v[0] for k, v in parse_qs(u.query).items()}

    def do_GET(self):
        path, q = self._q()
        rec("requests.jsonl", {"method": "GET", "path": path, "query": q})
        if path == "/health":
            return self._send(200, {"status": "ok"})
        if path == "/logs":
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "close")
            self.end_headers()
            try:
                for line in SC.get("log_lines", []):
                    self.wfile.write(("data: " + line + "\n\n").encode())
                    self.wfile.flush()
                STATE["logs_sent"] = True
                t0 = time.time()
                while time.time() - t0 < 60:
                    with LOCK:
                        busy = any(status_of(j) == "running" for j in JOBS.values())
                    if JOBS and not busy:
                        break
                    time.sleep(0.1)
            except Exception:
                pass
            self.close_connection = True
            return
        if path == "/job":
            with LOCK:
                j = JOBS.get(q.get("id", ""))
                if j is None:
                    return self._send(404, {"error": "no such job"})
                if q.get("result"):
                    if status_of(j) != "done":
                        return self._send(409, {"error": "not done"})
                    return self._send(200, self._multipart(j),
                                      'multipart/mixed; boundary="' + B + '"')
                j["polls"] += 1
                st = status_of(j)
            return self._send(200, {"id": q.get("id"), "status": st})
        return self._send(404, {"error": "not found"})

    def _multipart(self, j):
        body = j["body"]
        replay = {"semantic_tokens": SC.get("semantic", "1,2,3"),
                  "style": body.get("style"), "lyrics": body.get("lyrics"), "cot": body.get("cot"),
                  "lm_seed": body.get("lm_seed"), "seed": body.get("seed")}
        abc = SC.get("abc", "X:1\nT:fake\nM:4/4\nL:1/8\nK:C\nV:Vocal\nCDEF GABc|\n")
        if abc is not None:
            replay["abc"] = abc
        out = b""
        out += ("--" + B + "\r\nContent-Type: application/json\r\n\r\n").encode()
        out += json.dumps(replay).encode() + b"\r\n"
        out += ("--" + B + "\r\nContent-Type: audio/wav\r\n\r\n").encode()
        out += wav_bytes() + b"\r\n"
        out += ("--" + B + "\r\nContent-Type: application/octet-stream\r\n"
                'Content-Disposition: form-data; name="latent"\r\n\r\n').encode()
        out += latent_bytes() + b"\r\n"
        out += ("--" + B + "--\r\n").encode()
        return out

    def do_POST(self):
        path, q = self._q()
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n) if n else b""
        try:
            body = json.loads(raw.decode() or "null")
        except Exception:
            body = None
        rec("requests.jsonl", {"method": "POST", "path": path, "query": q, "body": body})
        if path == "/synth":
            if not isinstance(body, dict):
                return self._send(400, {"error": "bad json"})
            fmt = body.get("output_format")
            if fmt is not None and fmt not in ALLOWED:
                return self._send(400, {"error": "unknown output format"})
            with LOCK:
                STATE["n"] += 1
                jid = "j%d" % STATE["n"]
                JOBS[jid] = {"body": body, "polls": 0, "cancelled": False, "t0": time.time()}
            return self._send(200, {"id": jid})
        if path == "/job" and q.get("cancel"):
            with LOCK:
                j = JOBS.get(q.get("id", ""))
                if j is None:
                    return self._send(404, {"error": "no such job"})
                j["cancelled"] = True
            return self._send(200, {"ok": True})
        return self._send(404, {"error": "not found"})


srv = ThreadingHTTPServer((opt("--host", "127.0.0.1"), int(opt("--port", "8080"))), H)
srv.daemon_threads = True
srv.serve_forever()
'''


def make_fake_server(directory, **scenario) -> Path:
    """Фейк-программа yue-server в каталоге directory; scenario — поведение
    (mode done|failed|hold, polls, log_lines, abc, semantic, broken)."""
    d = Path(directory)
    d.mkdir(parents=True, exist_ok=True)
    p = d / "yue-server"
    p.write_text(_FAKE_SERVER.replace("__PYTHON__", sys.executable))
    p.chmod(0o755)
    set_scenario(d, **scenario)
    return p


def set_scenario(directory, **scenario):
    (Path(directory) / "scenario.json").write_text(json.dumps(scenario))


def _jsonl(path: Path) -> list:
    if not path.exists():
        return []
    return [json.loads(ln) for ln in path.read_text().splitlines() if ln.strip()]


def fake_starts(directory) -> list:
    """Запуски фейка: [{argv, ld, pid}] по порядку."""
    return _jsonl(Path(directory) / "starts.jsonl")


def fake_requests(directory) -> list:
    return _jsonl(Path(directory) / "requests.jsonl")


def synth_bodies(directory) -> list:
    return [r["body"] for r in fake_requests(directory)
            if r["method"] == "POST" and r["path"] == "/synth"]


def pid_alive(pid: int) -> bool:
    """Процесс жив (зомби — уже нет)."""
    try:
        os.kill(pid, 0)
    except (ProcessLookupError, PermissionError):
        return False
    try:
        st = Path(f"/proc/{pid}/stat").read_text()
        return st.rsplit(")", 1)[1].split()[0] != "Z"
    except OSError:
        return True


def kill_fakes(directory):
    """Уборка теста: процессы фейка не переживают тест при любой реализации."""
    for s in fake_starts(directory):
        try:
            os.kill(s["pid"], signal.SIGKILL)
        except (ProcessLookupError, PermissionError):
            pass


def wait_until(cond, timeout=15.0, step=0.05):
    t0 = time.time()
    while time.time() - t0 < timeout:
        if cond():
            return True
        time.sleep(step)
    return cond()


def opt_value(argv: list, name: str):
    for i, a in enumerate(argv):
        if a == name and i + 1 < len(argv):
            return argv[i + 1]
        if a.startswith(name + "="):
            return a.split("=", 1)[1]
    return None


def wav_bytes(frames=WAV_FRAMES, sr=WAV_SR) -> bytes:
    buf = io.BytesIO()
    with wave.open(buf, "wb") as w:
        w.setnchannels(2)
        w.setsampwidth(2)
        w.setframerate(sr)
        fr = array.array("h")
        for i in range(frames):
            v = int(6000 * math.sin(2 * math.pi * 220 * i / sr))
            fr.extend((v, v))
        w.writeframes(fr.tobytes())
    return buf.getvalue()


def latent_bytes(t=LATENT_T) -> bytes:
    return array.array("f", [i * 0.5 for i in range(t * 64)]).tobytes()


def multipart_body(replay: dict, audio: bytes, latent: bytes) -> bytes:
    b = BOUNDARY
    return (f"--{b}\r\nContent-Type: application/json\r\n\r\n".encode() + json.dumps(replay).encode()
            + f"\r\n--{b}\r\nContent-Type: audio/wav\r\n\r\n".encode() + audio
            + (f"\r\n--{b}\r\nContent-Type: application/octet-stream\r\n"
               'Content-Disposition: form-data; name="latent"\r\n\r\n').encode() + latent
            + f"\r\n--{b}--\r\n".encode())


# ---------------------------------------------------------------- HTTP-источник моделей

class FakeModelSource:
    """Локальный HTTP-источник: отдаёт файл по последнему компоненту пути
    (база URL может включать репозиторий/ревизию — не важно). Пишет запросы;
    gate — первая отдача файла останавливается на середине до gate.set()."""

    def __init__(self, files: dict):
        self.files = dict(files)
        self.requests = []
        self.gate = None
        self.half_sent = threading.Event()
        src = self

        class H(http.server.BaseHTTPRequestHandler):
            def log_message(self, *a):
                pass

            def do_GET(self):
                name = self.path.split("?", 1)[0].rstrip("/").rsplit("/", 1)[-1]
                src.requests.append(name)
                data = src.files.get(name)
                if data is None:
                    self.send_response(404)
                    self.send_header("Content-Length", "0")
                    self.end_headers()
                    return
                self.send_response(200)
                self.send_header("Content-Type", "application/octet-stream")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                gate = src.gate
                if gate is not None and not gate.is_set():
                    half = len(data) // 2
                    self.wfile.write(data[:half])
                    self.wfile.flush()
                    src.half_sent.set()
                    gate.wait(20)
                    self.wfile.write(data[half:])
                else:
                    self.wfile.write(data)

        self.httpd = http.server.ThreadingHTTPServer(("127.0.0.1", 0), H)
        self.httpd.daemon_threads = True
        self.thread = threading.Thread(target=self.httpd.serve_forever, daemon=True)
        self.thread.start()
        self.url = f"http://127.0.0.1:{self.httpd.server_address[1]}"

    def close(self):
        if self.gate is not None:
            self.gate.set()
        self.httpd.shutdown()
        self.httpd.server_close()

    def count(self, name):
        return sum(1 for r in self.requests if r == name)


def _sha(b: bytes) -> str:
    return hashlib.sha256(b).hexdigest()


def model_bytes(name: str) -> bytes:
    """Маленький «файл модели» — у каждой модели свой."""
    return (f"GGUF fake {name} ".encode() * 64)[:1500 + len(name)]


VAE_BYTES = b"GGUF fake vae " * 50


def patch_model_sums(case, engine_cpp, names=("q8",)):
    """Подменить sha256/size закреплённого списка на значения маленьких файлов теста."""
    for n in names:
        b = model_bytes(n)
        p = mock.patch.dict(engine_cpp.MODELS[n], {"sha256": _sha(b), "size": len(b)})
        p.start()
        case.addCleanup(p.stop)
    p = mock.patch.dict(engine_cpp.VAE, {"sha256": _sha(VAE_BYTES), "size": len(VAE_BYTES)})
    p.start()
    case.addCleanup(p.stop)


def place_model(models_dir: Path, engine_cpp, name: str, vae=True):
    models_dir.mkdir(parents=True, exist_ok=True)
    (models_dir / engine_cpp.MODELS[name]["file"]).write_bytes(model_bytes(name))
    if vae:
        (models_dir / engine_cpp.VAE["file"]).write_bytes(VAE_BYTES)


def job_row(**kw):
    """Строка трека как из БД воркера (sqlite3.Row) с основными столбцами jobs."""
    row = {"id": 5, "title": "t", "status": "running", "style": "dark rock, male baritone",
           "lyrics": "[verse] la la", "seed": 4242, "cot": "full", "arc": "", "max_tokens": 0,
           "temperature": 1.15, "cfg": 2.5, "parent_id": None, "role": "", "overdub_of": None,
           "overdub_gain": None, "voice_src": None, "head_id": None, "folder": "", "draft": 0,
           "req_abc": "", "abc_file": "", "duration_sec": 0.0, "cont_from": None, "error": "",
           "audio_file": "", "engine": "", "created_at": "2026-10-12T00:00:00"}
    row.update(kw)
    c = sqlite3.connect(":memory:")
    c.row_factory = sqlite3.Row
    c.execute(f"CREATE TABLE jobs ({', '.join(row)})")
    c.execute(f"INSERT INTO jobs VALUES ({', '.join('?' for _ in row)})", tuple(row.values()))
    r = c.execute("SELECT * FROM jobs").fetchone()
    c.close()
    return r


def _sampling(req: dict) -> dict:
    s = req.get("semantic_sampling")
    if not isinstance(s, dict):
        raise AssertionError(f"нет semantic_sampling в заявке: {req}")
    return s


def _temperature(req: dict):
    s = req.get("semantic_sampling") or {}
    return s.get("temperature", req.get("temperature"))


def _tokens(v) -> list:
    """source_tokens/semantic_tokens: список чисел или строка «1,2,3»."""
    if isinstance(v, str):
        return [int(x) for x in v.split(",") if x.strip()]
    return [int(x) for x in v]


def _import():
    import engine_cpp
    return engine_cpp


class _EnvCase(unittest.TestCase):
    """Свой временный каталог; env движка — внутрь него, без чужих путей машины."""

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.tmp = Path(self._td.name)
        self.models = self.tmp / "models"
        self.models.mkdir()
        env = {"YUE_CPP_MODELS": str(self.models), "YUE_HOME": str(self.tmp / "home"),
               "YUE_DATA_DIR": str(self.tmp / "data")}
        p = mock.patch.dict(os.environ, env)
        p.start()
        self.addCleanup(p.stop)
        os.environ.pop("YUE_CPP_BIN", None)
        self.e = _import()


# ---------------------------------------------------------------- M1, M2, M8

class TestPinnedModels(_EnvCase):
    """M1: закреплённый список моделей, ревизия, коммит программы."""

    def test_models_list(self):
        self.assertEqual(set(self.e.MODELS), set(MODEL_FILES))
        for n, f in MODEL_FILES.items():
            m = self.e.MODELS[n]
            self.assertEqual(m["file"], f, n)
            self.assertRegex(m["sha256"], r"^[0-9a-f]{64}$", n)
            self.assertGreater(int(m["size"]), 0, n)
        self.assertEqual(self.e.VAE["file"], VAE_FILE)
        self.assertRegex(self.e.VAE["sha256"], r"^[0-9a-f]{64}$")
        self.assertGreater(int(self.e.VAE["size"]), 0)

    def test_pins(self):
        self.assertEqual(self.e.HF_REPO, "Serveurperso/YuE2-GGUF")
        self.assertTrue(str(self.e.HF_REVISION).startswith("e630f2b8"), self.e.HF_REVISION)
        self.assertEqual(self.e.CPP_COMMIT, "c893629")

    def test_no_torch_import(self):
        # модуль работает без torch (install.sh зовёт его до установки PyTorch)
        code = "import sys; sys.modules['torch'] = None; import engine_cpp; print('ok')"
        p = subprocess.run([sys.executable, "-c", code], cwd=WORKER_DIR, capture_output=True,
                           text=True, timeout=60)
        self.assertEqual(p.returncode, 0, p.stderr)


class TestModelsDir(_EnvCase):
    """M2: каталог моделей и признак «скачана»."""

    def test_env_dir(self):
        self.assertEqual(Path(self.e.models_dir()), self.models)

    def test_default_under_data(self):
        os.environ.pop("YUE_CPP_MODELS", None)
        self.assertEqual(Path(self.e.models_dir()), self.tmp / "data" / "yue2cpp-models")

    def test_installed_needs_model_and_vae(self):
        patch_model_sums(self, self.e, ("q8",))
        self.assertFalse(self.e.model_installed("q8"))
        place_model(self.models, self.e, "q8", vae=False)
        self.assertFalse(self.e.model_installed("q8"), "без VAE модель не считается скачанной")
        (self.models / VAE_FILE).write_bytes(VAE_BYTES)
        self.assertTrue(self.e.model_installed("q8"))
        self.assertFalse(self.e.model_installed("q6"), "чужая модель не скачана")

    def test_wrong_size_not_installed(self):
        patch_model_sums(self, self.e, ("q8",))
        place_model(self.models, self.e, "q8")
        (self.models / MODEL_FILES["q8"]).write_bytes(model_bytes("q8")[:-10])
        self.assertFalse(self.e.model_installed("q8"))


class TestLabels(_EnvCase):
    """M8: метки движка (jobs.engine) и модели (/version)."""

    def test_engine_label(self):
        for n in MODEL_FILES:
            self.assertEqual(self.e.engine_label(n), f"cpp-{n}")

    def test_model_label(self):
        self.assertEqual(self.e.model_label("q8"),
                         "Serveurperso/YuE2-GGUF:YuE2-3B-Q8_0.gguf@yue2.cpp-c893629")
        self.assertIn("YuE2-3B-Q5_K_M.gguf", self.e.model_label("q5"))


# ---------------------------------------------------------------- M3: загрузка (ТК185)

class TestDownload(_EnvCase):

    def setUp(self):
        super().setUp()
        patch_model_sums(self, self.e, ("q8", "q6"))
        self.src = FakeModelSource({MODEL_FILES["q8"]: model_bytes("q8"),
                                    MODEL_FILES["q6"]: model_bytes("q6"), VAE_FILE: VAE_BYTES})
        self.addCleanup(self.src.close)

    def _leftovers(self):
        return sorted(p.name for p in self.models.iterdir() if p.name.endswith(".part"))

    def test_ok_file_under_its_name_and_vae(self):
        self.e.download("q8", None, base_url=self.src.url)
        self.assertEqual((self.models / MODEL_FILES["q8"]).read_bytes(), model_bytes("q8"))
        self.assertEqual((self.models / VAE_FILE).read_bytes(), VAE_BYTES, "VAE не докачан")
        self.assertEqual(self._leftovers(), [])
        self.assertTrue(self.e.model_installed("q8"))

    def test_progress_cb_called(self):
        calls = []
        self.e.download("q8", lambda *a, **kw: calls.append((a, kw)), base_url=self.src.url)
        self.assertTrue(calls, "progress_cb ни разу не вызван")

    def test_corrupt_no_file_no_part(self):
        self.src.files[MODEL_FILES["q8"]] = b"x" * len(model_bytes("q8"))
        with self.assertRaises(RuntimeError):
            self.e.download("q8", None, base_url=self.src.url)
        self.assertFalse((self.models / MODEL_FILES["q8"]).exists())
        self.assertEqual(self._leftovers(), [])
        self.assertFalse(self.e.model_installed("q8"))

    def test_ready_files_no_network(self):
        place_model(self.models, self.e, "q8")
        self.e.download("q8", None, base_url=self.src.url)
        self.assertEqual(self.src.requests, [], "готовые файлы с верной суммой — без обращения к источнику")

    def test_second_model_reuses_vae(self):
        self.e.download("q8", None, base_url=self.src.url)
        self.e.download("q6", None, base_url=self.src.url)
        self.assertEqual(self.src.count(VAE_FILE), 1, "общий VAE качается один раз")
        self.assertTrue(self.e.model_installed("q6"))

    def test_only_missing_vae_downloaded(self):
        place_model(self.models, self.e, "q8", vae=False)
        self.e.download("q8", None, base_url=self.src.url)
        self.assertEqual(self.src.requests, [VAE_FILE])
        self.assertTrue(self.e.model_installed("q8"))

    def test_cli_unknown_name_fails(self):
        p = subprocess.run([sys.executable, "engine_cpp.py", "download", "q4"], cwd=WORKER_DIR,
                           capture_output=True, text=True, timeout=60, env=dict(os.environ))
        self.assertNotEqual(p.returncode, 0, p.stdout)
        self.assertEqual(list(self.models.iterdir()), [])


# ---------------------------------------------------------------- M4: программа и библиотеки

class TestFindBinary(_EnvCase):

    def _exe(self, path: Path) -> Path:
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/bin/sh\nexit 0\n")
        path.chmod(0o755)
        return path

    def test_none_when_absent(self):
        self.assertFalse((WORKER_DIR / "yue2cpp" / "yue-server").exists(),
                         "в рабочем дереве лежит yue2cpp/yue-server — тест не про чистую машину")
        self.assertIsNone(self.e.find_binary())

    def test_env_bin(self):
        b = self._exe(self.tmp / "bin" / "yue-server")
        os.environ["YUE_CPP_BIN"] = str(b)
        self.assertEqual(Path(self.e.find_binary()), b)

    def test_built_by_install_sh(self):
        b = self._exe(self.tmp / "home" / "yue2cpp-src" / "build" / "yue-server")
        self.assertEqual(Path(self.e.find_binary()), b)

    def test_env_bin_wins(self):
        self._exe(self.tmp / "home" / "yue2cpp-src" / "build" / "yue-server")
        b = self._exe(self.tmp / "bin" / "yue-server")
        os.environ["YUE_CPP_BIN"] = str(b)
        self.assertEqual(Path(self.e.find_binary()), b)


class TestCudaLibDirs(_EnvCase):
    """Каталоги lib пакетов nvidia-* из venv (site-packages/nvidia/<пакет>/lib).
    Допущение теста: venv виден через sys.path (site-packages — там)."""

    def test_finds_nvidia_lib_dirs(self):
        site = self.tmp / "site"
        libs = [site / "nvidia" / "cublas" / "lib", site / "nvidia" / "cuda_runtime" / "lib"]
        for d in libs:
            d.mkdir(parents=True)
            (d / "libfake.so.12").write_bytes(b"")
        with mock.patch.object(sys, "path", [str(site)] + [p for p in sys.path]):
            sys.modules.pop("nvidia", None)
            got = [str(Path(x).resolve()) for x in self.e.cuda_lib_dirs()]
        for d in libs:
            self.assertIn(str(d.resolve()), got)

    def test_returns_list(self):
        self.assertIsInstance(self.e.cuda_lib_dirs(), list)


# ---------------------------------------------------------------- M5: сервер (ТК186, часть ТК189)

class _ServerCase(_EnvCase):

    def setUp(self):
        super().setUp()
        self.fake_dir = self.tmp / "fake"
        self.bin = make_fake_server(self.fake_dir)
        self.addCleanup(kill_fakes, self.fake_dir)
        self.model = self.models / MODEL_FILES["q8"]
        self.vae = self.models / VAE_FILE
        self.model.write_bytes(model_bytes("q8"))
        self.vae.write_bytes(VAE_BYTES)

    def _server(self, low_vram=False, lib_dirs=()):
        s = self.e.CppServer(self.bin, self.model, self.vae, low_vram, list(lib_dirs))
        self.addCleanup(s.stop)
        return s

    def _body(self, **kw):
        b = {"style": "dark rock", "lyrics": "[verse] la la", "cot": "full", "lm_seed": 7, "seed": 7,
             "output_format": "wav24", "semantic_sampling": {"max_tokens": 450}}
        b.update(kw)
        return b


class TestCppServerProcess(_ServerCase):

    def test_start_localhost_with_model_and_vae(self):
        s = self._server()
        s.start()
        self.assertTrue(s.running)
        st = fake_starts(self.fake_dir)
        self.assertEqual(len(st), 1)
        argv = st[0]["argv"]
        self.assertEqual(Path(opt_value(argv, "--model")), self.model)
        self.assertEqual(Path(opt_value(argv, "--vae")), self.vae)
        self.assertEqual(opt_value(argv, "--host"), "127.0.0.1")
        self.assertTrue(str(s.url).startswith("http://127.0.0.1:"), s.url)
        self.assertNotEqual(opt_value(argv, "--max-seq"), "8192", "без экономного режима — без потолка 8192")

    def test_low_vram_max_seq(self):
        s = self._server(low_vram=True)
        s.start()
        self.assertEqual(opt_value(fake_starts(self.fake_dir)[0]["argv"], "--max-seq"), "8192")

    def test_ld_library_path_has_lib_dirs(self):
        libs = [str(self.tmp / "nv" / "cublas" / "lib"), str(self.tmp / "nv" / "cudnn" / "lib")]
        for d in libs:
            Path(d).mkdir(parents=True)
        s = self._server(lib_dirs=libs)
        s.start()
        ld = fake_starts(self.fake_dir)[0]["ld"].split(":")
        for d in libs:
            self.assertIn(d, ld)

    def test_stop_kills_process(self):
        s = self._server()
        s.start()
        pid = fake_starts(self.fake_dir)[0]["pid"]
        self.assertTrue(pid_alive(pid))
        s.stop()
        self.assertFalse(s.running)
        self.assertTrue(wait_until(lambda: not pid_alive(pid), 10), "процесс yue-server не остановлен")

    def test_two_servers_free_ports(self):
        other = self.tmp / "fake2"
        b2 = make_fake_server(other)
        self.addCleanup(kill_fakes, other)
        s1 = self._server()
        s2 = self.e.CppServer(b2, self.model, self.vae, False, [])
        self.addCleanup(s2.stop)
        s1.start()
        s2.start()
        self.assertNotEqual(s1.url, s2.url)


class TestCppServerSynth(_ServerCase):

    def test_synth_returns_parts_and_logs(self):
        set_scenario(self.fake_dir, log_lines=["[AR] Semantic 100/1000"])
        s = self._server()
        s.start()
        logs = []
        parts = s.synth(self._body(), lambda: False, logs.append)
        self.assertEqual(_tokens(parts["replay"]["semantic_tokens"]), [1, 2, 3])
        self.assertTrue(parts["audio"].startswith(b"RIFF"), parts["audio"][:16])
        self.assertEqual(len(parts["latent"]), LATENT_T * 64 * 4)
        self.assertTrue(any("Semantic 100/1000" in str(x) for x in logs), logs)
        bodies = synth_bodies(self.fake_dir)
        self.assertEqual(len(bodies), 1)
        self.assertEqual(bodies[0]["style"], "dark rock")

    def test_cancel_sends_cancel_and_interrupts(self):
        set_scenario(self.fake_dir, mode="hold")
        s = self._server()
        s.start()
        n = {"i": 0}

        def cancelled():
            n["i"] += 1
            return bool(synth_bodies(self.fake_dir)) and n["i"] > 1

        with self.assertRaises(InterruptedError):
            s.synth(self._body(), cancelled, lambda _l: None)
        cancels = [r for r in fake_requests(self.fake_dir)
                   if r["method"] == "POST" and r["path"] == "/job" and r["query"].get("cancel")]
        self.assertEqual(len(cancels), 1, fake_requests(self.fake_dir))
        self.assertTrue(cancels[0]["query"].get("id"))

    # кросс-ревью s20: сервер умер посреди заявки (CUDA обрывает процесс) — причина из журнала, не «Connection refused»;
    # при нехватке памяти — CppOOM и процесс считается остановленным
    def test_server_died_mid_job_reports_oom(self):
        set_scenario(self.fake_dir, mode="hold")
        s = self._server()
        s.start()

        def cancelled():
            if synth_bodies(self.fake_dir) and s.proc is not None and s.proc.poll() is None:
                s.proc.kill()
                s.proc.wait(5)
                with open(s.log_path, "a") as f:
                    f.write("ggml_backend_cuda_buffer_type_alloc_buffer: cudaMalloc failed: out of memory\n")
            return False

        with self.assertRaises(self.e.CppOOM) as cm:
            s.synth(self._body(), cancelled, lambda _l: None, poll=0.2)
        self.assertIn("out of memory", str(cm.exception))
        self.assertFalse(s.running)

    def test_failed_raises_with_log_tail(self):
        set_scenario(self.fake_dir, mode="failed",
                     log_lines=["[AR] Semantic 10/1000", "ggml_cuda: CUDA error: out of memory"])
        s = self._server()
        s.start()
        with self.assertRaises(RuntimeError) as cm:
            s.synth(self._body(), lambda: False, lambda _l: None)
        self.assertIn("out of memory", str(cm.exception))


# ---------------------------------------------------------------- M6: перевод заявки (ТК187)

@unittest.skipUnless(_HAS_NUMPY, "нужен numpy")
class TestBuildRequest(_EnvCase):

    def test_track_fields(self):
        req = self.e.build_request(job_row(), 3000, False)
        self.assertEqual(req["style"], "dark rock, male baritone")
        self.assertEqual(req["lyrics"], "[verse] la la")
        self.assertEqual(req["cot"], "full")
        self.assertEqual(req["lm_seed"], 4242)
        self.assertEqual(req["seed"], 4242)
        self.assertAlmostEqual(req["cfg_scale"], 2.5)
        self.assertEqual(_sampling(req)["max_tokens"], 3000)
        self.assertAlmostEqual(_temperature(req), 1.15)
        self.assertEqual(req["output_format"], "wav24")
        self.assertIn(req["output_format"], ALLOWED_FORMATS)
        self.assertFalse(req.get("abc"), "без своего плана abc не передаётся")
        json.dumps(req)   # уходит в POST /synth как JSON

    def test_abc_passed(self):
        req = self.e.build_request(job_row(req_abc="request.abc"), 3000, False, abc=DEFAULT_ABC)
        self.assertEqual(req["abc"], DEFAULT_ABC)

    def test_draft_budget(self):
        req = self.e.build_request(job_row(draft=1), 450, False)
        self.assertEqual(_sampling(req)["max_tokens"], 450)

    def test_low_vram_caps_budget(self):
        full = self.e.build_request(job_row(), 16000, False)
        low = self.e.build_request(job_row(), 16000, True)
        self.assertEqual(_sampling(full)["max_tokens"], 16000)
        self.assertLess(_sampling(low)["max_tokens"], 8192, "экономный режим: контекст 8192 — бюджет меньше")
        self.assertLess(_sampling(low)["max_tokens"], 16000)

    def test_low_vram_small_budget_kept(self):
        low = self.e.build_request(job_row(draft=1), 450, True)
        self.assertEqual(_sampling(low)["max_tokens"], 450)


@unittest.skipUnless(_HAS_NUMPY, "нужен numpy")
class TestContinueRequest(_EnvCase):

    def _parent(self, abc=True, plan_json=False):
        import numpy as np
        d = self.tmp / "jobs" / "3"
        d.mkdir(parents=True)
        self.sem = list(range(100, 100 + 120 * 25))   # 120 с шагов модели
        np.save(d / "semantic.npy", np.array(self.sem, dtype=np.int64))
        if abc:
            (d / "score.abc").write_text(DEFAULT_ABC, encoding="utf-8")
        if plan_json:
            (d / "plan.json").write_text("{}")
        return d

    def test_from_mark_uses_parent_tokens_and_score(self):
        d = self._parent()
        row = job_row(id=6, parent_id=3, role="continue", cont_from=60.0, seed=77)
        req = self.e.continue_request(row, d, 60.0, 3000, False)
        self.assertEqual(_tokens(req["source_tokens"]), self.sem)
        self.assertEqual(req["source_start"], 0)
        self.assertAlmostEqual(float(req["source_end"]), 60.0)
        self.assertEqual(req["abc"], DEFAULT_ABC, "без нового плана — score.abc родителя")
        self.assertEqual(req["seed"], 77)
        self.assertEqual(req["output_format"], "wav24")
        json.dumps(req)

    def test_new_plan_wins(self):
        d = self._parent()
        new = DEFAULT_ABC.replace("CDEF", "EFGA")
        row = job_row(id=6, parent_id=3, role="continue", cont_from=60.0, req_abc="request.abc")
        req = self.e.continue_request(row, d, 60.0, 3000, False, abc=new)
        self.assertEqual(req["abc"], new)

    def test_torch_parent_without_plan_json(self):
        # условие 161: yue2.cpp продолжает трек PyTorch по semantic.npy и score.abc
        d = self._parent(plan_json=False)
        self.assertFalse((d / "plan.json").exists())
        req = self.e.continue_request(job_row(parent_id=3, role="continue", cont_from=30.0),
                                      d, 30.0, 3000, False)
        self.assertEqual(_tokens(req["source_tokens"]), self.sem)
        self.assertAlmostEqual(float(req["source_end"]), 30.0)

    def test_low_vram_caps(self):
        d = self._parent()
        req = self.e.continue_request(job_row(parent_id=3, role="continue", cont_from=60.0),
                                      d, 60.0, 16000, True)
        self.assertLess(_sampling(req)["max_tokens"], 8192)


class TestPlanRequest(_EnvCase):

    def test_one_step(self):
        req = self.e.plan_request("dark rock", "[verse] la la", "full", 11)
        s = _sampling(req)
        self.assertEqual(s["max_tokens"], 1)
        self.assertEqual(s["min_tokens"], 0)
        self.assertEqual(req["style"], "dark rock")
        self.assertEqual(req["lyrics"], "[verse] la la")
        self.assertEqual(req["cot"], "full")
        self.assertEqual(req["lm_seed"], 11)
        self.assertIn(req.get("output_format", "wav24"), ALLOWED_FORMATS)


# ---------------------------------------------------------------- M7: разбор ответа (ТК188)

@unittest.skipUnless(_HAS_NUMPY, "нужны numpy/soundfile")
class TestParseAndSave(_EnvCase):

    def _replay(self, abc=DEFAULT_ABC):
        r = {"semantic_tokens": "5,6,7,8", "style": "dark rock", "lyrics": "[verse] la",
             "cot": "full", "lm_seed": 3, "seed": 3}
        if abc is not None:
            r["abc"] = abc
        return r

    def test_parse_multipart(self):
        audio, latent = wav_bytes(), latent_bytes()
        parts = self.e.parse_multipart(multipart_body(self._replay(), audio, latent))
        self.assertEqual(parts["replay"]["semantic_tokens"], "5,6,7,8")
        self.assertEqual(parts["replay"]["abc"], DEFAULT_ABC)
        self.assertEqual(parts["audio"], audio)
        self.assertEqual(parts["latent"], latent)

    def test_parse_binary_with_crlf_inside(self):
        # двоичные части могут содержать \r\n и «--» — граница одна: --yue2-batch-boundary
        latent = array.array("f", [0.0] * 64).tobytes() + b"\r\n--x\r\n" + b"\0" * 2
        parts = self.e.parse_multipart(multipart_body(self._replay(), wav_bytes(), latent))
        self.assertEqual(parts["latent"], latent)

    def _save(self, abc=DEFAULT_ABC):
        d = self.tmp / "job"
        d.mkdir()
        parts = self.e.parse_multipart(multipart_body(self._replay(abc), wav_bytes(), latent_bytes()))
        self.e.save_artifacts(d, parts, {"model": "q8", "low_vram": False})
        return d

    def test_save_files(self):
        import numpy as np
        import soundfile as sf
        d = self._save()
        info = sf.info(str(d / "audio.flac"))
        self.assertEqual(info.samplerate, WAV_SR)
        self.assertEqual(info.frames, WAV_FRAMES)
        self.assertEqual(info.channels, 2)
        self.assertEqual((d / "score.abc").read_text(encoding="utf-8"), DEFAULT_ABC)
        sem = np.load(d / "semantic.npy")
        self.assertTrue(np.issubdtype(sem.dtype, np.integer), sem.dtype)
        self.assertEqual(sem.tolist(), [5, 6, 7, 8])
        lat = np.load(d / "latent.npy")
        self.assertEqual(lat.dtype, np.float32)
        self.assertEqual(lat.shape, (LATENT_T, 64))
        self.assertEqual(float(lat[1, 0]), 32.0)   # значение i·0.5, i = 64
        cfg = json.loads((d / "config.json").read_text())
        self.assertEqual(cfg["backend"], "yue2.cpp")

    def test_no_abc_no_score_file(self):
        d = self._save(abc=None)
        self.assertFalse((d / "score.abc").exists())
        self.assertTrue((d / "audio.flac").exists())

    def _counters(self):
        return {"phase": "load", "sem": 0, "budget": 0}

    def test_log_score_is_plan(self):
        c = self._counters()
        self.e.parse_log_line("[AR] Score 120/4096", c)
        self.assertEqual(c["phase"], "plan")

    def test_log_semantic_percent(self):
        c = self._counters()
        self.e.parse_log_line("[AR] Semantic 1200/14500", c)
        self.assertEqual(c["phase"], "semantic")
        self.assertEqual(c["sem"], 1200)
        self.assertEqual(c["budget"], 14500)
        self.assertEqual(c["sem"] * 100 // c["budget"], 8)

    def test_log_decode(self):
        c = self._counters()
        self.e.parse_log_line("[NAR] Step 3/32", c)
        self.assertEqual(c["phase"], "decode")

    def test_log_foreign_line_unchanged(self):
        c = {"phase": "semantic", "sem": 50, "budget": 1000}
        for line in ("load_tensors: CUDA0 model buffer size = 3200 MiB", "", "[AR] Semantic x/y"):
            self.e.parse_log_line(line, c)
        self.assertEqual(c, {"phase": "semantic", "sem": 50, "budget": 1000})


if __name__ == "__main__":
    unittest.main()
