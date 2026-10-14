"""Второй движок yue2.cpp через API воркера.

Что должно работать:
C1. GET /config: engine (нет — torch), cpp_model (нет — q8), cpp_low_vram (нет — false),
    cpp_available (программа есть и запускается), cpp_models {имя: {file, size, installed}}.
    POST /config: неизвестное значение → 422; engine=cpp без программы → 422 «yue2.cpp не
    установлен»; engine=cpp / cpp_model с нескачанной моделью (или без VAE) → 422 «модель не
    скачана»; иначе 200, значения в settings.json данных.
C2. POST /engine/models/install?name=: качает модель и VAE (есть с верной суммой — без
    сети); сумма не сошлась → 502, нет ни файла, ни .part; неизвестное имя → 422; две загрузки
    одной модели — по очереди (файл скачан один раз). GET /engine/models/progress →
    {name, file, done, total} во время загрузки, {} после.
C3. Воркер сам запускает yue-server ($PARTSCASTER_CPP_BIN) на 127.0.0.1 с --model/--vae модели из
    настроек; экономный режим → --max-seq 8192; смена модели → следующая генерация на новом
    процессе, старый остановлен; идущую генерацию смена не трогает; перед стартом yue2.cpp
    модель PyTorch выгружается; перед загрузкой PyTorch yue2.cpp остановлен.
C4. Джоба на yue2.cpp: done, файлы audio.flac/score.abc/semantic.npy/latent.npy/
    request.json/config.json, jobs.engine = "cpp-q8" (пишется при старте); черновик — 450 шагов;
    отмена → cancel на сервер, статус canceled; failed с «out of memory» в журнале → ошибка про
    нехватку видеопамяти, сервер остановлен; торч-джоба → "torch"; старый трек — "".
    Прогресс — по строкам журнала («[AR] Semantic 100/1000» → semantic, 10 %).
    yue2.cpp продолжает трек PyTorch по semantic.npy и score.abc.
C5. /version: model — метка действующего движка, engine; рецепт: worker_model — по
    движку корня трека, не текущему.
C6. PyTorch продолжает трек без plan.json: план строится из score.abc (pipe.plan с abc).

Внешние границы — программа yue-server (скрипт-фейк из test_engine_cpp), HTTP-источник моделей
(локальный сервер; адрес источника подставляется обёрткой вокруг engine_cpp.download — контракт
не задаёт другого способа), пайплайн yue2/torch (заглушки, как в test_jobs_lifecycle).
"""
import json
import os
import sys
import threading
import types
import unittest
from pathlib import Path
from unittest import mock

from test_character import _job_id, _post_job, _Sampling
from test_engine_cpp import (DEFAULT_ABC, MODEL_FILES, VAE_BYTES, VAE_FILE, FakeModelSource,
                             _tokens, fake_requests, fake_starts, kill_fakes, make_fake_server,
                             model_bytes, opt_value, patch_model_sums, pid_alive, place_model,
                             synth_bodies, wait_until)
from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

OOM_LINE = "ggml_cuda_compute_forward: CUDA error: out of memory"


def _setup_cpp(case):
    """Общая подготовка: данные/настройки во временном каталоге, env движка туда же,
    заглушки yue2/torch, закреплённые суммы — на маленькие файлы теста."""
    tmp = Path(case._td.name)
    protocol = types.ModuleType("yue2.protocol")
    protocol.Sampling = _Sampling
    case.yue2 = mock.MagicMock()
    mods = {"yue2": case.yue2, "yue2.protocol": protocol,
            "yue2.pipeline": mock.MagicMock(), "torch": mock.MagicMock()}
    p = mock.patch.dict(sys.modules, mods)
    p.start()
    case.addCleanup(p.stop)
    data = tmp / "cfg"
    data.mkdir(exist_ok=True)
    case.settings = data / "settings.json"
    for name, val in (("_pipe", None), ("_load_error", None),
                      ("DATA_DIR", data), ("SETTINGS_PATH", case.settings)):
        p = mock.patch.object(case.w, name, val)
        p.start()
        case.addCleanup(p.stop)
    case.models = tmp / "models"
    case.models.mkdir(exist_ok=True)
    env = {"PARTSCASTER_CPP_MODELS": str(case.models), "PARTSCASTER_HOME": str(tmp / "home")}
    p = mock.patch.dict(os.environ, env)
    p.start()
    case.addCleanup(p.stop)
    os.environ.pop("PARTSCASTER_CPP_BIN", None)
    import engine_cpp
    case.e = engine_cpp
    patch_model_sums(case, engine_cpp, ("q8", "q6"))
    case.fake_dir = tmp / "fake"
    case.addCleanup(kill_fakes, case.fake_dir)


def _use_fake(case, **scenario):
    b = make_fake_server(case.fake_dir, **scenario)
    os.environ["PARTSCASTER_CPP_BIN"] = str(b)
    return b


def _write_settings(case, **st):
    cur = {}
    if case.settings.exists():
        cur = json.loads(case.settings.read_text())
    cur.update(st)
    case.settings.write_text(json.dumps(cur))


class _CppCase(_WorkerApiCase):
    def setUp(self):
        super().setUp()
        _setup_cpp(self)

    def _new(self, **extra):
        jid = _job_id(self, _post_job(self, **extra))
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running' WHERE id=?", (jid,))   # как после захвата очередью
        return jid

    def _start(self, jid):
        box = {}

        def run():
            try:
                self.w._run_job(jid)
            except BaseException as e:  # noqa: BLE001 - исключение наружу = провал теста
                box["exc"] = e

        t = threading.Thread(target=run, daemon=True)
        t.start()
        return t, box

    def _run(self, jid, timeout=60):
        t, box = self._start(jid)
        t.join(timeout)
        self.assertFalse(t.is_alive(), "генерация не закончилась за отведённое время")
        if "exc" in box:
            raise box["exc"]

    def _get(self, jid):
        r = self.client.get(f"/jobs/{jid}")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _cpp(self, model="q8", low_vram=False, **scenario):
        """Движок yue2.cpp в настройках, модель скачана, фейк-программа на месте."""
        _use_fake(self, **scenario)
        place_model(self.models, self.e, model)
        _write_settings(self, engine="cpp", cpp_model=model, cpp_low_vram=low_vram)

    def _release(self):
        (self.fake_dir / "release").write_text("1")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestEngineConfig(_CppCase):

    def _cfg(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def test_defaults_without_settings(self):
        self.assertFalse(self.settings.exists())
        c = self._cfg()
        self.assertEqual(c["engine"], "torch")
        self.assertEqual(c["cpp_model"], "q8")
        self.assertIs(c["cpp_low_vram"], False)
        self.assertIs(c["cpp_available"], False)
        self.assertEqual(set(c["cpp_models"]), set(MODEL_FILES))
        for n, f in MODEL_FILES.items():
            m = c["cpp_models"][n]
            self.assertEqual(m["file"], f)
            self.assertIsInstance(m["size"], int)
            self.assertIs(m["installed"], False, n)

    def test_unknown_values_422(self):
        for body in ({"engine": "foo"}, {"cpp_model": "q4"}, {"cpp_model": "Q8"}):
            r = self.client.post("/config", json=body)
            self.assertEqual(r.status_code, 422, (body, r.text))
        c = self._cfg()
        self.assertEqual((c["engine"], c["cpp_model"]), ("torch", "q8"))

    def test_cpp_without_program_422(self):
        place_model(self.models, self.e, "q8")
        r = self.client.post("/config", json={"engine": "cpp"})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("yue2.cpp не установлен", r.text)
        self.assertEqual(self._cfg()["engine"], "torch")

    def test_broken_program_not_available(self):
        _use_fake(self, broken=True)
        place_model(self.models, self.e, "q8")
        self.assertIs(self._cfg()["cpp_available"], False, "программа есть, но не запускается")
        r = self.client.post("/config", json={"engine": "cpp"})
        self.assertEqual(r.status_code, 422, r.text)

    def test_cpp_without_model_422(self):
        _use_fake(self)
        self.assertIs(self._cfg()["cpp_available"], True)
        r = self.client.post("/config", json={"engine": "cpp"})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("модель не скачана", r.text)
        self.assertEqual(self._cfg()["engine"], "torch")

    def test_cpp_without_vae_422(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8", vae=False)
        r = self.client.post("/config", json={"engine": "cpp"})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("модель не скачана", r.text)

    def test_cpp_model_not_downloaded_422(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8")
        r = self.client.post("/config", json={"cpp_model": "q6"})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIn("модель не скачана", r.text)
        self.assertEqual(self._cfg()["cpp_model"], "q8")

    def test_cpp_ok_persists(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8")
        r = self.client.post("/config", json={"engine": "cpp", "cpp_model": "q8", "cpp_low_vram": True})
        self.assertEqual(r.status_code, 200, r.text)
        c = self._cfg()
        self.assertEqual(c["engine"], "cpp")
        self.assertIs(c["cpp_low_vram"], True)
        self.assertIs(c["cpp_models"]["q8"]["installed"], True)
        self.assertIs(c["cpp_models"]["q6"]["installed"], False)
        st = json.loads(self.settings.read_text())
        self.assertEqual(st["engine"], "cpp")
        self.assertEqual(st["cpp_model"], "q8")
        self.assertIs(st["cpp_low_vram"], True)

    def test_switch_model_and_back_to_torch(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8")
        place_model(self.models, self.e, "q6")
        self.assertEqual(self.client.post("/config", json={"engine": "cpp"}).status_code, 200)
        r = self.client.post("/config", json={"cpp_model": "q6"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._cfg()["cpp_model"], "q6")
        r = self.client.post("/config", json={"engine": "torch"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(json.loads(self.settings.read_text())["engine"], "torch")

    def test_old_fields_still_work(self):
        r = self.client.post("/config", json={"auto_stems_instrumental": False})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._cfg()["engine"], "torch")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestEngineModelInstall(_CppCase):

    def setUp(self):
        super().setUp()
        self.src = FakeModelSource({MODEL_FILES["q8"]: model_bytes("q8"),
                                    MODEL_FILES["q6"]: model_bytes("q6"), VAE_FILE: VAE_BYTES})
        self.addCleanup(self.src.close)
        real = self.e.download
        url = self.src.url

        def download(name, progress_cb=None, base_url=None, **kw):
            return real(name, progress_cb, base_url=url, **kw)

        p = mock.patch.object(self.e, "download", download)
        p.start()
        self.addCleanup(p.stop)
        if hasattr(self.w, "download"):   # если воркер импортировал функцию по имени
            p = mock.patch.object(self.w, "download", download)
            p.start()
            self.addCleanup(p.stop)

    def _install(self, name):
        return self.client.post(f"/engine/models/install?name={name}")

    def _progress(self):
        r = self.client.get("/engine/models/progress")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _parts(self):
        return sorted(p.name for p in self.models.iterdir() if p.name.endswith(".part"))

    def test_install_ok(self):
        r = self._install("q8")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual((self.models / MODEL_FILES["q8"]).read_bytes(), model_bytes("q8"))
        self.assertEqual((self.models / VAE_FILE).read_bytes(), VAE_BYTES)
        self.assertEqual(self._parts(), [])
        self.assertEqual(self._progress(), {})
        self.assertIs(self.client.get("/config").json()["cpp_models"]["q8"]["installed"], True)

    def test_corrupt_502(self):
        self.src.files[MODEL_FILES["q8"]] = b"y" * len(model_bytes("q8"))
        r = self._install("q8")
        self.assertEqual(r.status_code, 502, r.text)
        self.assertFalse((self.models / MODEL_FILES["q8"]).exists())
        self.assertEqual(self._parts(), [])
        self.assertEqual(self._progress(), {})

    def test_ready_no_network(self):
        place_model(self.models, self.e, "q8")
        r = self._install("q8")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.src.requests, [])

    def test_unknown_name_422(self):
        for name in ("q4", "", "../x"):
            r = self._install(name)
            self.assertEqual(r.status_code, 422, (name, r.text))
        self.assertEqual(self.src.requests, [])

    def test_progress_during_download(self):
        self.src.gate = threading.Event()
        box = {}
        t = threading.Thread(target=lambda: box.setdefault("r", self._install("q8")), daemon=True)
        t.start()
        self.assertTrue(self.src.half_sent.wait(20), "загрузка не началась")
        got = {}
        ok = wait_until(lambda: got.update(self._progress()) or bool(got.get("total")), 10)
        self.src.gate.set()
        t.join(30)
        self.assertFalse(t.is_alive())
        self.assertTrue(ok, got)
        self.assertEqual(got["name"], "q8")
        self.assertIn(got["file"], (MODEL_FILES["q8"], VAE_FILE))
        self.assertLessEqual(got["done"], got["total"])
        self.assertGreater(got["total"], 0)
        self.assertEqual(box["r"].status_code, 200, box["r"].text)
        self.assertEqual(self._progress(), {})

    def test_two_installs_one_download(self):
        self.src.gate = threading.Event()
        res = []
        ts = [threading.Thread(target=lambda: res.append(self._install("q8")), daemon=True)
              for _ in range(2)]
        ts[0].start()
        self.assertTrue(self.src.half_sent.wait(20))
        ts[1].start()
        threading.Event().wait(0.5)   # вторая заявка успела прийти, пока первая качает
        self.src.gate.set()
        for t in ts:
            t.join(30)
            self.assertFalse(t.is_alive())
        self.assertEqual([r.status_code for r in res], [200, 200], [r.text for r in res])
        self.assertEqual(self.src.count(MODEL_FILES["q8"]), 1, "одна модель скачана дважды")
        self.assertEqual((self.models / MODEL_FILES["q8"]).read_bytes(), model_bytes("q8"))
        self.assertEqual(self._parts(), [])


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestEngineServerLifecycle(_CppCase):

    def test_started_with_selected_model(self):
        self._cpp("q8")
        self._run(self._new())
        st = fake_starts(self.fake_dir)
        self.assertEqual(len(st), 1)
        argv = st[0]["argv"]
        self.assertEqual(Path(opt_value(argv, "--model")), self.models / MODEL_FILES["q8"])
        self.assertEqual(Path(opt_value(argv, "--vae")), self.models / VAE_FILE)
        self.assertEqual(opt_value(argv, "--host"), "127.0.0.1")
        self.assertNotEqual(opt_value(argv, "--max-seq"), "8192")

    def test_low_vram_max_seq(self):
        self._cpp("q8", low_vram=True)
        self._run(self._new())
        self.assertEqual(opt_value(fake_starts(self.fake_dir)[0]["argv"], "--max-seq"), "8192")

    def test_server_reused_between_jobs(self):
        self._cpp("q8")
        self._run(self._new())
        self._run(self._new())
        self.assertEqual(len(fake_starts(self.fake_dir)), 1, "без смены настроек сервер не перезапускается")

    def test_model_switch_restarts_server(self):
        self._cpp("q8")
        place_model(self.models, self.e, "q6")
        self._run(self._new())
        old = fake_starts(self.fake_dir)[0]["pid"]
        r = self.client.post("/config", json={"cpp_model": "q6"})
        self.assertEqual(r.status_code, 200, r.text)
        self._run(self._new())
        st = fake_starts(self.fake_dir)
        self.assertEqual(len(st), 2)
        self.assertEqual(Path(opt_value(st[1]["argv"], "--model")), self.models / MODEL_FILES["q6"])
        self.assertTrue(wait_until(lambda: not pid_alive(old), 10), "старый процесс не остановлен")

    def test_switch_during_generation_waits(self):
        self._cpp("q8", mode="hold")
        place_model(self.models, self.e, "q6")
        jid = self._new()
        t, box = self._start(jid)
        self.assertTrue(wait_until(lambda: synth_bodies(self.fake_dir), 20), "генерация не дошла до сервера")
        pid = fake_starts(self.fake_dir)[0]["pid"]
        r = self.client.post("/config", json={"cpp_model": "q6", "cpp_low_vram": True})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertTrue(pid_alive(pid), "смена настроек остановила идущую генерацию")
        self._release()
        t.join(60)
        self.assertFalse(t.is_alive())
        self.assertEqual(self._get(jid)["status"], "done")
        self._run(self._new())
        st = fake_starts(self.fake_dir)
        self.assertEqual(len(st), 2)
        self.assertEqual(Path(opt_value(st[1]["argv"], "--model")), self.models / MODEL_FILES["q6"])
        self.assertEqual(opt_value(st[1]["argv"], "--max-seq"), "8192")

    def test_torch_after_cpp_stops_server(self):
        from test_jobs_lifecycle import _FakePipe
        self._cpp("q8")
        self._run(self._new())
        pid = fake_starts(self.fake_dir)[0]["pid"]
        self.assertEqual(self.client.post("/config", json={"engine": "torch"}).status_code, 200)
        pipe = _FakePipe()
        self.yue2.YuE2Pipeline.from_pretrained.return_value.__enter__.return_value = pipe
        jid = self._new()
        self._run(jid)
        self.assertEqual(self._get(jid)["status"], "done")
        self.assertEqual(len(pipe.calls), 1, "торч-джоба не дошла до пайплайна")
        self.assertTrue(wait_until(lambda: not pid_alive(pid), 10), "yue2.cpp не остановлен при PyTorch")

    def test_torch_unloaded_before_cpp(self):
        from test_jobs_lifecycle import _FakePipe
        pipe = _FakePipe()
        self.w._pipe = pipe
        self._cpp("q8")
        jid = self._new()
        self._run(jid)
        self.assertEqual(self._get(jid)["status"], "done")
        self.assertTrue(pipe.exited, "модель PyTorch не выгружена перед yue2.cpp")
        self.assertIsNone(self.w._pipe)
        self.assertEqual(pipe.calls, [])


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestCppJob(_CppCase):

    def test_done_files_and_engine(self):
        import numpy as np
        self._cpp("q8")
        jid = self._new(seed=4242)
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q8")
        self.assertEqual(got["audio_file"], "audio.flac")
        self.assertEqual(got["abc_file"], "score.abc")
        self.assertGreater(got["duration_sec"], 0)
        d = self.jobs_dir / str(jid)
        for f in ("audio.flac", "score.abc", "semantic.npy", "latent.npy", "request.json", "config.json"):
            self.assertTrue((d / f).is_file(), f)
        self.assertEqual(np.load(d / "semantic.npy").tolist(), [1, 2, 3])
        self.assertEqual(np.load(d / "latent.npy").shape[1], 64)
        cfg = json.loads((d / "config.json").read_text())
        self.assertEqual(cfg["backend"], "yue2.cpp")
        body = synth_bodies(self.fake_dir)[0]
        self.assertEqual(body["output_format"], "wav24")
        self.assertEqual(body["seed"], 4242)
        self.assertEqual(body["lm_seed"], 4242)
        listed = {j["id"]: j for j in self.client.get("/jobs").json()}
        self.assertEqual(listed[jid]["engine"], "cpp-q8")

    def test_engine_written_at_start(self):
        self._cpp("q8", mode="hold")
        jid = self._new()
        t, _ = self._start(jid)
        self.assertTrue(wait_until(lambda: synth_bodies(self.fake_dir), 20))
        during = self._row(jid)["engine"]
        self._release()
        t.join(60)
        self.assertEqual(during, "cpp-q8")

    def test_draft_450(self):
        self._cpp("q8")
        jid = self._new(draft=True)
        self._run(jid)
        self.assertEqual(synth_bodies(self.fake_dir)[0]["semantic_sampling"]["max_tokens"], 450)

    def test_low_vram_budget_fits_context(self):
        self._cpp("q8", low_vram=True)
        self._run(self._new())
        self.assertLess(synth_bodies(self.fake_dir)[0]["semantic_sampling"]["max_tokens"], 8192)

    def test_cancel(self):
        self._cpp("q8", mode="hold")
        jid = self._new()
        t, _ = self._start(jid)
        self.assertTrue(wait_until(lambda: synth_bodies(self.fake_dir), 20))
        r = self.client.post(f"/jobs/{jid}/cancel")
        self.assertEqual(r.status_code, 200, r.text)
        t.join(60)
        self.assertFalse(t.is_alive(), "отмена не остановила генерацию")
        got = self._get(jid)
        self.assertEqual(got["status"], "canceled")
        self.assertFalse(got.get("error"))
        cancels = [q for q in fake_requests(self.fake_dir)
                   if q["method"] == "POST" and q["path"] == "/job" and q["query"].get("cancel")]
        self.assertEqual(len(cancels), 1)

    def test_oom(self):
        self._cpp("q8", mode="failed", log_lines=["[AR] Semantic 10/1000", OOM_LINE])
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "error")
        self.assertIn("видеопамят", got["error"])
        pid = fake_starts(self.fake_dir)[0]["pid"]
        self.assertTrue(wait_until(lambda: not pid_alive(pid), 10), "после нехватки памяти сервер не остановлен")

    # нехватка памяти в «только план» — понятный текст и сервер остановлен (не держит видеопамять)
    def test_plan_oom_stops_server(self):
        self._cpp("q8", mode="failed", log_lines=[OOM_LINE])
        r = self.client.post("/plan", json={"style": "rock", "lyrics": "[verse] la", "cot": "full", "seed": 5})
        self.assertEqual(r.status_code, 503, r.text)
        self.assertIn("видеопамят", r.json()["detail"])
        pid = fake_starts(self.fake_dir)[0]["pid"]
        self.assertTrue(wait_until(lambda: not pid_alive(pid), 10), "после нехватки памяти сервер не остановлен")

    def test_other_failure_is_error(self):
        self._cpp("q8", mode="failed", log_lines=["something broke"])
        jid = self._new()
        self._run(jid)
        got = self._get(jid)
        self.assertEqual(got["status"], "error")
        self.assertTrue(got["error"])
        self.assertNotIn("видеопамят", got["error"])

    def test_progress_from_log(self):
        self._cpp("q8", mode="hold", log_lines=["[AR] Score 50/4096", "[AR] Semantic 100/1000"])
        jid = self._new()
        t, _ = self._start(jid)
        seen = {}
        ok = wait_until(lambda: seen.update(self._get(jid)) or seen.get("progress_pct") == 10, 15, 0.2)
        self._release()
        t.join(60)
        self.assertTrue(ok, seen)
        self.assertEqual(seen["stage"], "semantic")

    def test_torch_job_engine(self):
        from test_jobs_lifecycle import _FakePipe
        seen = {}

        def gen(kw):
            from test_jobs_lifecycle import _FakeSong
            seen["engine"] = self._row(jid)["engine"]
            return _FakeSong()

        self.w._pipe = _FakePipe(gen)
        jid = self._new()
        self._run(jid)
        self.assertEqual(seen.get("engine"), "torch", "движок пишется при старте генерации")
        self.assertEqual(self._get(jid)["engine"], "torch")

    def test_old_track_engine_empty(self):
        jid = self._job(semantic=False)
        self.assertEqual(self._get(jid)["engine"], "")

    def test_cpp_continues_torch_track(self):
        import numpy as np
        self._cpp("q8")
        parent = self._job(duration=60.0)
        with self._conn() as c:
            c.execute("UPDATE jobs SET engine='torch' WHERE id=?", (parent,))
        pdir = self.jobs_dir / str(parent)
        (pdir / "score.abc").write_text(DEFAULT_ABC, encoding="utf-8")
        self.assertFalse((pdir / "plan.json").exists())
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 30.0})
        self.assertLess(r.status_code, 300, r.text)
        kid = self._children(parent)[0]["id"]
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running' WHERE id=?", (kid,))
        self._run(kid)
        got = self._get(kid)
        self.assertEqual(got["status"], "done", got.get("error"))
        self.assertEqual(got["engine"], "cpp-q8")
        body = synth_bodies(self.fake_dir)[0]
        self.assertEqual(_tokens(body["source_tokens"]), np.load(pdir / "semantic.npy").tolist())
        self.assertAlmostEqual(float(body["source_end"]), 30.0)
        self.assertEqual(body["abc"], DEFAULT_ABC)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestEngineVersion(_CppCase):

    def test_torch_default(self):
        v = self.client.get("/version").json()
        self.assertEqual(v["engine"], "torch")
        self.assertNotIn("yue2.cpp", v["model"])
        self.assertTrue(v["model"].startswith("m-a-p/YuE2-3B"), v["model"])

    def test_cpp(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8")
        self.assertEqual(self.client.post("/config", json={"engine": "cpp"}).status_code, 200)
        v = self.client.get("/version").json()
        self.assertEqual(v["engine"], "cpp-q8")
        self.assertIn("yue2.cpp", v["model"])
        self.assertIn(MODEL_FILES["q8"], v["model"])
        self.assertEqual(v["model"], "Serveurperso/YuE2-GGUF:YuE2-3B-Q8_0.gguf@yue2.cpp-c893629")


try:
    from test_recipe_api import _RecipeCase
except ImportError:   # без окружения воркера
    _RecipeCase = _WorkerApiCase


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRecipeEngineLabel(_RecipeCase):
    """worker_model рецепта — метка движка корня трека, не текущего."""

    def setUp(self):
        super().setUp()
        _setup_cpp(self)

    def _model(self, jid):
        r = self.client.get(f"/jobs/{jid}/recipe")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()["worker_model"]

    def test_cpp_root_on_torch_worker(self):
        root = self._mk(engine="cpp-q8")
        wm = self._model(root)
        self.assertIn("yue2.cpp", wm)
        self.assertIn(MODEL_FILES["q8"], wm)

    def test_old_root_on_cpp_worker(self):
        _use_fake(self)
        place_model(self.models, self.e, "q8")
        self.assertEqual(self.client.post("/config", json={"engine": "cpp"}).status_code, 200)
        root = self._mk(engine="")
        wm = self._model(root)
        self.assertNotIn("yue2.cpp", wm)
        self.assertTrue(wm.startswith("m-a-p/YuE2-3B"), wm)

    def test_child_follows_root(self):
        root = self._mk(engine="cpp-q6")
        kid = self._mk(parent_id=root, role="continue", engine="torch")
        self._post_op(root, "continue", {"from_sec": 10.0, "parent_id": root, "seed": 1}, f"job:{kid}")
        self.assertIn(MODEL_FILES["q6"], self._model(kid))


class _PlanCalled(RuntimeError):
    pass


class _PlanRecorder:
    """Пайплайн PyTorch: записывает pipe.plan и останавливает генерацию."""

    def __init__(self):
        self.plans = []
        self.tokenizer = mock.MagicMock()
        self.weights = {}

    def __enter__(self):
        return self

    def __exit__(self, *a):
        pass

    def plan(self, *a, **kw):
        self.plans.append((a, kw))
        raise _PlanCalled("stop after plan")

    def __call__(self, **kw):
        raise AssertionError("продолжение не должно идти через новый трек с нуля")

    def __getattr__(self, name):
        if name.startswith("__"):
            raise AttributeError(name)
        raise AssertionError(f"генерация до плана: pipe.{name}")


def _plan_abc(args, kw):
    """abc из вызова pipe.plan: аргументом abc= или полем request (объект или dict)."""
    if kw.get("abc"):
        return kw["abc"]
    req = kw.get("request", args[0] if args else None)
    if isinstance(req, dict):
        return req.get("abc")
    return getattr(req, "abc", None)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestCrossContinueTorch(_CppCase):

    def setUp(self):
        super().setUp()
        # yue2.pipeline: как библиотека — без plan.json сохранённый план не читается
        pipeline = mock.MagicMock()

        def load(d):
            if not (Path(d) / "plan.json").is_file():
                raise FileNotFoundError(str(Path(d) / "plan.json"))
            return mock.MagicMock()

        pipeline.SymbolicPlan.load.side_effect = load
        protocol = mock.MagicMock()
        protocol.Sampling = _Sampling
        p = mock.patch.dict(sys.modules, {"yue2.pipeline": pipeline, "yue2.protocol": protocol})
        p.start()
        self.addCleanup(p.stop)

    def test_plan_from_score_abc(self):
        parent = self._job(duration=60.0)
        with self._conn() as c:
            c.execute("UPDATE jobs SET engine='cpp-q8' WHERE id=?", (parent,))
        pdir = self.jobs_dir / str(parent)
        (pdir / "score.abc").write_text(DEFAULT_ABC, encoding="utf-8")
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 30.0})
        self.assertLess(r.status_code, 300, r.text)
        kid = self._children(parent)[0]["id"]
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running' WHERE id=?", (kid,))
        pipe = _PlanRecorder()
        self.w._pipe = pipe
        self._run(kid)
        got = self._get(kid)
        self.assertTrue(pipe.plans, f"pipe.plan не вызван; итог: {got.get('status')} {got.get('error')}")
        self.assertEqual(_plan_abc(*pipe.plans[0]), DEFAULT_ABC)
        self.assertNotIn("plan.json", got.get("error") or "")


if __name__ == "__main__":
    unittest.main()
