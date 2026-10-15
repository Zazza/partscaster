"""Спецификация HTTP-слоя общей очереди воркера (задача internal-shared-queue,
раздел «Контракт → Воркер: HTTP» карточки).

Что должно работать:
- GET /queue → {paused, items}; пустая — пустой список (ТК7).
- POST /queue/ops {kind, title, job_id} → op (client=True, source из заголовка
  X-Partscaster-Source); /beat → op, 404 неизвестная; /finish → op, 404/409 (ТК10).
- /move {dir} → {moved}; край — moved=false; идущая/завершённая — 409, неизвестная — 404 (ТК8).
- /cancel → {canceled, running}; ждущая — canceled сразу; повтор — без падения;
  неизвестная — 404 (ТК12).
- POST /queue/pause {paused} → {paused}; на паузе слот не выдаётся (ТК9).
- Заголовок X-Partscaster-Op: шаг внутри идущей client-op не встаёт в очередь и не
  блокируется (ТК4); метка неизвестной/ждущей op — не исполнение вне очереди (ТК5).
- Тяжёлая секция видна в очереди как op с треком (ТК3); отменённая во время работы → 409
  с «отменено» (ТК12, ТК15).
- Генерации: POST /jobs ставит op kind=generate с названием трека и source; поля
  jobs.queue_pos/jobs.source (миграция, ТК6); /jobs/{id}/cancel ждущей — через очередь,
  прежний ответ; /stats.gpu_waiting — число ждущих op.
- Старт воркера: queued генерации — в очередь по queue_pos, старые без места — по id;
  running → error (ТК6, ТК11).

Внешние границы подменяются: разделение на дорожки (demucs_separate), поток queue_loop.
Очередь воркера — глобальный экземпляр opqueue.OpQueue; между тестами он очищается
только через публичное API модуля.
"""
import threading
import time
import unittest
from unittest import mock

from test_character import _job_id, _post_job
from test_jobs_lifecycle import _Case as _LifeCase
from test_jobs_lifecycle import _FakePipe, _FakeSong
from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase, _WorkerDbCase

FINAL = {"done", "error", "canceled", "interrupted"}
JOIN = 10.0
_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"


def _worker_queue(w):
    """Глобальная очередь воркера: единственный атрибут-экземпляр OpQueue."""
    import opqueue
    found = [v for v in vars(w).values() if isinstance(v, opqueue.OpQueue)]
    if len(found) != 1:
        raise AssertionError(f"в yue_worker ожидается один экземпляр OpQueue, найдено {len(found)}")
    return found[0]


def _drain(q):
    """Закрыть всё незавершённое публичным API: тесты не зависят друг от друга."""
    q.set_paused(False)
    for _ in range(50):
        live = [it for it in q.snapshot()["items"] if it["state"] not in FINAL]
        if not live:
            return
        for it in live:
            if it["client"]:
                q.finish(it["id"], "canceled")
            elif it["state"] == "running":
                q.release(it["id"], state="canceled")
            else:
                q.cancel(it["id"])


class _Thread(threading.Thread):
    def __init__(self, fn):
        super().__init__(daemon=True)
        self.fn = fn
        self.result = None
        self.exc = None

    def run(self):
        try:
            self.result = self.fn()
        except BaseException as e:  # noqa: BLE001 — исход проверяет тест
            self.exc = e


def _start(fn):
    t = _Thread(fn)
    t.start()
    return t


def _wait_until(cond, timeout=JOIN):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if cond():
            return True
        time.sleep(0.01)
    return cond()


class _QueueMixin:
    def _setup_queue(self):
        self.q = _worker_queue(self.w)
        _drain(self.q)
        self.addCleanup(_drain, self.q)
        p = mock.patch.object(self.w.threading, "Thread")  # _startup не запускает queue_loop
        self._thread_patch = p

    def _snap(self):
        r = self.client.get("/queue")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _live(self):
        return [it for it in self._snap()["items"] if it["state"] not in FINAL]

    def _item(self, op_id):
        for it in self._snap()["items"]:
            if it["id"] == op_id:
                return it
        return None

    def _add(self, kind="preset", title="op", job_id=0, source=None):
        h = {"X-Partscaster-Source": source} if source else {}
        r = self.client.post("/queue/ops", json={"kind": kind, "title": title, "job_id": job_id},
                             headers=h)
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _holder(self):
        op = self._add(title="holder")
        self.assertEqual(op["state"], "running", op)
        return op["id"]

    def _finish(self, op_id, state="done", **kw):
        return self.client.post(f"/queue/ops/{op_id}/finish", json={"state": state, **kw})


class _Case(_QueueMixin, _WorkerApiCase):
    def setUp(self):
        super().setUp()
        data = self.db_path.parent / "cfg"
        data.mkdir()
        for name, val in (("DATA_DIR", data), ("SETTINGS_PATH", data / "settings.json")):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)
        self._setup_queue()
        self.sep_calls = []
        self.sep_hook = None
        p = mock.patch.object(self.w, "demucs_separate", side_effect=self._fake_separate)
        p.start()
        self.addCleanup(p.stop)

    def _fake_separate(self, audio, out_dir, **kw):
        live = [it for it in self.q.snapshot()["items"] if it["state"] == "running"]
        self.sep_calls.append(live)
        if self.sep_hook:
            self.sep_hook()
        return {"stems": [], "seconds": 0.0}

    def _track(self):
        jid = self._job(semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        (d / "audio.flac").write_bytes(b"fLaC")
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestQueueList(_Case):
    """ТК7."""

    def test_empty(self):
        # свежая очередь (глобальная хранит завершённые записи прошлых тестов — по условию 19 они видны)
        import opqueue
        with mock.patch.object(self.w, "QUEUE", opqueue.OpQueue()):
            self.assertEqual(self._snap(), {"paused": False, "items": []})

    def test_fields(self):
        op = self._add(kind="preset", title="Пресет «пост-панк»", job_id=0, source="mcp")
        items = self._snap()["items"]
        it = next(x for x in items if x["id"] == op["id"])
        for k in ("id", "kind", "title", "job_id", "source", "state", "pos",
                  "result_job_id", "error", "created_at"):
            self.assertIn(k, it)
        self.assertEqual(it["kind"], "preset")
        self.assertEqual(it["title"], "Пресет «пост-панк»")
        self.assertEqual(it["source"], "mcp")
        self.assertIs(it["client"], True)

    def test_source_app_header(self):
        op = self._add(source="app")
        self.assertEqual(op["source"], "app")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestBeatFinish(_Case):
    """ТК10."""

    def test_beat(self):
        a = self._holder()
        r = self.client.post(f"/queue/ops/{a}/beat")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["state"], "running")
        self.assertIs(r.json()["cancel_requested"], False)

    def test_beat_unknown_404(self):
        self.assertEqual(self.client.post("/queue/ops/987654/beat").status_code, 404)

    def test_finish_done_with_result_then_409(self):
        a = self._holder()
        r = self._finish(a, "done", result_job_id=17)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["state"], "done")
        self.assertEqual(r.json()["result_job_id"], 17)
        self.assertEqual(self._item(a)["result_job_id"], 17)
        self.assertEqual(self._finish(a, "done").status_code, 409)
        # отметка «жива» не оживляет завершённую
        b = self.client.post(f"/queue/ops/{a}/beat")
        self.assertIn(b.status_code, (200, 404, 409), b.text)
        if b.status_code == 200:
            self.assertEqual(b.json()["state"], "done")

    def test_finish_error_text(self):
        a = self._holder()
        r = self._finish(a, "error", error="нет звука у трека")
        self.assertEqual(r.status_code, 200, r.text)
        it = self._item(a)
        self.assertEqual(it["state"], "error")
        self.assertEqual(it["error"], "нет звука у трека")

    def test_finish_unknown_404(self):
        self.assertEqual(self._finish(13579, "done").status_code, 404)

    def test_finish_generation_409(self):
        self._holder()
        jid = _job_id(self, _post_job(self, title="Gen"))
        gen = next(it for it in self._live() if it["kind"] == "generate" and it["job_id"] == jid)
        self.assertEqual(self._finish(gen["id"], "done").status_code, 409)

    def test_finish_releases_slot(self):
        a = self._holder()
        b = self._add(title="b")["id"]
        self.assertEqual(self._item(b)["state"], "waiting")
        self._finish(a, "done")
        self.assertEqual(self._item(b)["state"], "running")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestMoveApi(_Case):
    """ТК8, ТК1."""

    def _move(self, op_id, d):
        return self.client.post(f"/queue/ops/{op_id}/move", json={"dir": d})

    def test_up_swaps_and_slot_follows(self):
        h = self._holder()
        a = self._add(title="a")["id"]
        b = self._add(title="b")["id"]
        r = self._move(b, -1)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["moved"], True)
        waiting = [it["id"] for it in self._live() if it["state"] == "waiting"]
        self.assertEqual(waiting, [b, a])
        self._finish(h, "done")
        self.assertEqual(self._item(b)["state"], "running", "слот не у переставленной вверх")

    def test_edges(self):
        self._holder()
        a = self._add(title="a")["id"]
        b = self._add(title="b")["id"]
        for op_id, d in ((a, -1), (b, 1)):
            r = self._move(op_id, d)
            self.assertEqual(r.status_code, 200, r.text)
            self.assertIs(r.json()["moved"], False)
        waiting = [it["id"] for it in self._live() if it["state"] == "waiting"]
        self.assertEqual(waiting, [a, b])

    def test_running_done_unknown(self):
        h = self._holder()
        self.assertEqual(self._move(h, 1).status_code, 409)
        self._finish(h, "done")
        self.assertEqual(self._move(h, -1).status_code, 409)
        self.assertEqual(self._move(888888, -1).status_code, 404)

    def test_bad_dir_4xx(self):
        self._holder()
        a = self._add(title="a")["id"]
        r = self._move(a, 5)
        self.assertGreaterEqual(r.status_code, 400)
        self.assertLess(r.status_code, 500)

    def test_generation_moves_before_other_op(self):
        """Две генерации и пресет: ▲ у нижней генерации — слот у неё (критерий приёмки)."""
        h = self._holder()
        j1 = _job_id(self, _post_job(self, title="G1"))
        j2 = _job_id(self, _post_job(self, title="G2"))
        p = self._add(kind="preset", title="preset")["id"]
        live = self._live()
        g1 = next(it["id"] for it in live if it["job_id"] == j1 and it["kind"] == "generate")
        g2 = next(it["id"] for it in live if it["job_id"] == j2 and it["kind"] == "generate")
        self.assertIs(self._move(g2, -1).json()["moved"], True)
        waiting = [it["id"] for it in self._live() if it["state"] == "waiting"]
        self.assertEqual(waiting, [g2, g1, p])
        self._finish(h, "done")
        self.assertEqual(self._item(g2)["state"], "running")
        with self._conn() as c:
            pos = {r["id"]: r["queue_pos"] for r in c.execute("SELECT id, queue_pos FROM jobs")}
        self.assertLess(pos[j2], pos[j1], "место генерации не записано в jobs.queue_pos")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestCancelPauseApi(_Case):
    """ТК12, ТК9."""

    def _cancel(self, op_id):
        return self.client.post(f"/queue/ops/{op_id}/cancel")

    def test_cancel_waiting(self):
        self._holder()
        a = self._add(title="a")["id"]
        r = self._cancel(a)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json(), {"canceled": True, "running": False})
        self.assertEqual(self._item(a)["state"], "canceled")
        r2 = self._cancel(a)
        self.assertLess(r2.status_code, 500, r2.text)
        if r2.status_code == 200:
            self.assertIs(r2.json()["canceled"], False)
        self.assertEqual(self._item(a)["state"], "canceled")

    def test_cancel_unknown_404(self):
        self.assertEqual(self._cancel(246810).status_code, 404)

    def test_cancel_running_client_sets_flag(self):
        a = self._holder()
        r = self._cancel(a)
        self.assertEqual(r.json(), {"canceled": True, "running": True})
        b = self.client.post(f"/queue/ops/{a}/beat").json()
        self.assertIs(b["cancel_requested"], True)

    def test_pause(self):
        h = self._holder()
        r = self.client.post("/queue/pause", json={"paused": True})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json(), {"paused": True})
        self.assertIs(self._snap()["paused"], True)
        self.assertEqual(self._item(h)["state"], "running")
        a = self._add(title="a")["id"]
        self._finish(h, "done")
        self.assertEqual(self._item(a)["state"], "waiting", "на паузе выдан слот")
        r = self.client.post("/queue/pause", json={"paused": False})
        self.assertEqual(r.json(), {"paused": False})
        self.assertEqual(self._item(a)["state"], "running")

    def test_stats_gpu_waiting(self):
        self._holder()
        self._add(title="a")
        self._add(title="b")
        r = self.client.get("/stats")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["gpu_waiting"], 2)


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestHeavySection(_Case):
    """ТК3, ТК4, ТК5, ТК12/ТК15 на тяжёлой секции (разделение на дорожки)."""

    def _stems(self, jid, op_id=None):
        h = {"X-Partscaster-Op": str(op_id)} if op_id is not None else {}
        return self.client.post(f"/jobs/{jid}/stems", headers=h)

    def test_tk3_visible_op_with_track_then_done(self):
        jid = self._track()
        r = self._stems(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(len(self.sep_calls), 1)
        running = self.sep_calls[0]
        self.assertEqual(len(running), 1, running)
        self.assertEqual(running[0]["job_id"], jid, "op тяжёлой секции без трека")
        self.assertTrue(running[0]["title"], "op без названия")
        self.assertEqual(self._item(running[0]["id"])["state"], "done")

    def test_tk4_inside_running_client_op_no_new_op(self):
        jid = self._track()
        a = self._holder()
        t = _start(lambda: self._stems(jid, a))
        t.join(JOIN)
        self.assertFalse(t.is_alive(), "шаг внутри своей op заблокирован (дедлок)")
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(t.result.status_code, 200, t.result.text)
        self.assertEqual([it["id"] for it in self.sep_calls[0]], [a])
        self.assertEqual([it["id"] for it in self._live()], [a], "шаг цепочки создал новую op")
        self.assertEqual(self._item(a)["state"], "running")

    def test_tk5_unknown_op_header_not_outside_queue(self):
        jid = self._track()
        a = self._holder()
        t = _start(lambda: self._stems(jid, 999999))
        t.join(0.5)
        self._finish(a, "done")
        t.join(JOIN)
        self.assertFalse(t.is_alive())
        self.assertIsNone(t.exc, t.exc)
        ran_with_holder = [c for c in self.sep_calls if any(it["id"] == a for it in c)]
        self.assertEqual(ran_with_holder, [], "шаг с чужой меткой исполнен вне очереди")
        self.assertLess(t.result.status_code, 500, t.result.text)

    def test_tk5_waiting_op_header_waits(self):
        jid = self._track()
        a = self._holder()
        b = self._add(title="b")["id"]
        t = _start(lambda: self._stems(jid, b))
        t.join(0.3)
        self.assertTrue(t.is_alive(), "шаг ждущей op исполнен до её слота")
        self.assertEqual(self.sep_calls, [])
        self._finish(a, "done")
        t.join(JOIN)
        self.assertFalse(t.is_alive())
        self.assertEqual(t.result.status_code, 200, t.result.text)
        self.assertEqual([it["id"] for it in self.sep_calls[0]], [b])

    def test_canceled_during_work_409(self):
        jid = self._track()
        inside = threading.Event()
        go = threading.Event()

        def hook():
            inside.set()
            go.wait(JOIN)
        self.sep_hook = hook
        t = _start(lambda: self._stems(jid))
        self.assertTrue(inside.wait(JOIN), "тяжёлая секция не началась")
        op = next(it for it in self._live() if it["state"] == "running")
        nxt = self._add(title="next")["id"]
        r = self.client.post(f"/queue/ops/{op['id']}/cancel")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json(), {"canceled": True, "running": True})
        go.set()
        t.join(JOIN)
        self.assertFalse(t.is_alive())
        self.assertEqual(t.result.status_code, 409, t.result.text)
        self.assertIn("отменено", t.result.text)
        self.assertEqual(self._item(op["id"])["state"], "canceled")
        self.assertEqual(self._item(nxt)["state"], "running", "слот не перешёл к следующей")

    def _foreign_op_header_not_outside_queue(self, foreign):
        """ТК5: метка op, которая не client-op (генерация / op самого воркера) и держит
        слот, — шаг не исполняется под её слотом: отказ 4xx или обычная постановка."""
        jid = self._track()
        self.assertEqual(self._item(foreign)["state"], "running", self._item(foreign))
        t = _start(lambda: self._stems(jid, foreign))
        t.join(0.5)
        self.assertEqual(self.sep_calls, [], "шаг с меткой чужой op исполнен вне очереди")
        self.assertEqual(self._item(foreign)["state"], "running")
        self.q.release(foreign, state="done")
        t.join(JOIN)
        self.assertFalse(t.is_alive())
        self.assertIsNone(t.exc, t.exc)
        r = t.result
        if r.status_code == 200:  # обычная постановка: своя op, не чужая
            self.assertTrue(self.sep_calls)
            self.assertNotIn(foreign, [it["id"] for it in self.sep_calls[0]])
        else:
            self.assertTrue(400 <= r.status_code < 500, r.text)
            self.assertEqual(self.sep_calls, [], "шаг выполнен, хотя ответ — отказ")

    def test_tk5_running_generation_header_not_outside_queue(self):
        jid = _job_id(self, _post_job(self))  # свободный слот — генерация сразу running
        gen = next(it for it in self._live() if it["kind"] == "generate" and it["job_id"] == jid)
        self._foreign_op_header_not_outside_queue(gen["id"])

    def test_tk5_worker_own_op_header_not_outside_queue(self):
        op = self.q.add("stems", "auto-stems")  # op воркера, не через /queue/ops
        self._foreign_op_header_not_outside_queue(op.as_dict()["id"])


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestGenerations(_Case):
    """POST /jobs, /jobs/{id}/cancel, источник, ТК2/ТК6/ТК11."""

    def test_post_job_adds_generate_op_with_source(self):
        self._holder()
        r = self.client.post("/jobs", json={"style": "dark rock", "lyrics": "[verse] la",
                                            "title": "Night Drive"},
                             headers={"X-Partscaster-Source": "mcp"})
        jid = _job_id(self, r)
        ops = [it for it in self._live() if it["kind"] == "generate" and it["job_id"] == jid]
        self.assertEqual(len(ops), 1, self._live())
        self.assertIn("Night Drive", ops[0]["title"])
        self.assertEqual(ops[0]["source"], "mcp")
        self.assertEqual(ops[0]["state"], "waiting")
        self.assertEqual(self._row(jid)["source"], "mcp")
        self.assertIsNotNone(self._row(jid)["queue_pos"])

    def test_new_generation_goes_last(self):
        self._holder()
        p = self._add(title="preset")["id"]
        jid = _job_id(self, _post_job(self))
        waiting = [it for it in self._live() if it["state"] == "waiting"]
        self.assertEqual(waiting[0]["id"], p)
        self.assertEqual(waiting[-1]["job_id"], jid)

    def test_cancel_waiting_generation_old_answer(self):
        self._holder()
        jid = _job_id(self, _post_job(self))
        op = next(it for it in self._live() if it["job_id"] == jid)
        r = self.client.post(f"/jobs/{jid}/cancel")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["canceled"], True)
        self.assertFalse(r.json().get("running"))
        self.assertEqual(self._row(jid)["status"], "canceled")
        self.assertEqual(self._item(op["id"])["state"], "canceled")

    def test_cancel_generation_op_via_queue_cancels_job(self):
        self._holder()
        jid = _job_id(self, _post_job(self))
        op = next(it for it in self._live() if it["job_id"] == jid)
        r = self.client.post(f"/queue/ops/{op['id']}/cancel")
        self.assertEqual(r.json(), {"canceled": True, "running": False})
        self.assertEqual(self._row(jid)["status"], "canceled")

    def test_tk11_startup_queues_by_queue_pos_and_running_error(self):
        with self._conn() as c:
            for jid, status, pos in ((1, "queued", 30), (2, "queued", 10), (3, "running", 5),
                                     (4, "queued", 20), (5, "done", 1)):
                c.execute("INSERT INTO jobs (id, title, status, style, lyrics, queue_pos, created_at) "
                          "VALUES (?, 't', ?, 's', 'l', ?, '2026-01-01T00:00:00')", (jid, status, pos))
        with self._thread_patch:
            self.w._startup()
        gens = [it["job_id"] for it in self._live() if it["kind"] == "generate"]
        self.assertEqual(gens, [2, 4, 1], "порядок очереди после старта не по queue_pos")
        self.assertEqual(self._row(3)["status"], "error")
        for jid in (1, 2, 4):
            self.assertEqual(self._row(jid)["status"], "queued")


_RUN_SCRIPT = r"""
import json, sqlite3, sys
from unittest import mock
from fastapi.testclient import TestClient
import yue_worker as w
c = TestClient(w.app, raise_server_exceptions=False)
if sys.argv[1] == "first":
    r = c.post("/queue/ops", json={"kind": "preset", "title": "цепочка", "job_id": 0},
               headers={"X-Partscaster-Source": "app"})
    op = r.json()["id"]
    with sqlite3.connect(w.DB_PATH) as conn:
        for pos in range(1, 4):
            conn.execute("INSERT INTO jobs (title, status, style, lyrics, queue_pos, created_at) "
                         "VALUES ('gen', 'queued', 's', 'l', ?, '2026-01-01T00:00:00')", (pos,))
    print(json.dumps({"op": op}))
else:
    old = int(sys.argv[2])
    with mock.patch.object(w.threading, "Thread"):
        w._startup()
    items = c.get("/queue").json()["items"]
    beat = c.post(f"/queue/ops/{old}/beat")
    fin = c.post(f"/queue/ops/{old}/finish", json={"state": "done"})
    after = c.get("/queue").json()["items"]
    print(json.dumps({"items": items, "beat": beat.status_code, "beat_body": beat.text,
                      "finish": fin.status_code, "after": after}))
"""


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestRestartOpIds(unittest.TestCase):
    """ТК11 / условие 9: после перезапуска воркера номер операции клиента прошлого запуска
    не достаётся другой записи — отметка «жива» по нему даёт 404, а не состояние чужой записи.

    Перезапуск — два настоящих запуска процесса воркера на одном каталоге данных (глобальная
    очередь и _startup/_load_queued — как в жизни). Раньше номера шли с 1 в каждом запуске:
    операция прошлого запуска №1 совпадала с первой загруженной генерацией, beat давал 200."""

    def _run(self, data_dir, *args):
        import json
        import os
        import subprocess
        import sys
        env = dict(os.environ, PARTSCASTER_DATA_DIR=data_dir,
                   PYTHONPATH=os.path.dirname(os.path.abspath(__file__)))
        r = subprocess.run([sys.executable, "-W", "ignore", "-c", _RUN_SCRIPT, *args],
                           capture_output=True, text=True, env=env, timeout=120)
        self.assertEqual(r.returncode, 0, r.stderr[-3000:])
        return json.loads(r.stdout.strip().splitlines()[-1])

    def test_beat_of_previous_run_op_is_404_after_restart(self):
        import tempfile
        with tempfile.TemporaryDirectory() as d:
            old = self._run(d, "first")["op"]
            self.assertGreater(old, 0)
            time.sleep(0.05)  # второй запуск — позже
            got = self._run(d, "second", str(old))
        gens = [it for it in got["items"] if it["kind"] == "generate"]
        self.assertEqual(len(gens), 3, got["items"])
        ids = [it["id"] for it in got["items"]]
        self.assertNotIn(old, ids, "номер операции прошлого запуска достался записи нового")
        self.assertEqual(got["beat"], 404, got["beat_body"])
        self.assertEqual(got["finish"], 404)
        # чужие записи не тронуты: ни одна не стала done от finish прошлой цепочки
        self.assertFalse([it for it in got["after"] if it["state"] == "done"], got["after"])
        self.assertTrue(all(it["id"] > old for it in gens), (old, ids))


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestMigrateQueueFields(_QueueMixin, _WorkerDbCase):
    """ТК6: старая БД без queue_pos/source."""

    def setUp(self):
        super().setUp()
        from fastapi.testclient import TestClient
        with self._conn() as c:
            c.execute("""CREATE TABLE jobs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                title TEXT DEFAULT '', status TEXT NOT NULL DEFAULT 'queued',
                style TEXT NOT NULL, lyrics TEXT NOT NULL, seed INTEGER,
                cot TEXT NOT NULL DEFAULT 'full', error TEXT DEFAULT '',
                duration_sec REAL DEFAULT 0, audio_file TEXT DEFAULT '',
                abc_file TEXT DEFAULT '', created_at TEXT NOT NULL,
                finished_at TEXT DEFAULT '')""")
            for _ in range(3):
                c.execute("INSERT INTO jobs (title, status, style, lyrics, created_at) "
                          "VALUES ('old', 'queued', 'dark rock', '[verse] la', '2026-01-01T00:00:00')")
        self.w.init_db()
        self.w._migrate()
        self.client = TestClient(self.w.app, raise_server_exceptions=False)
        self._setup_queue()

    def _cols(self):
        with self._conn() as c:
            return {r["name"] for r in c.execute("PRAGMA table_info(jobs)")}

    def test_columns_added_idempotent(self):
        self.assertTrue({"queue_pos", "source"} <= self._cols())
        self.w._migrate()
        self.assertTrue({"queue_pos", "source"} <= self._cols())

    def test_old_queued_by_id_new_last(self):
        with self._thread_patch:
            self.w._startup()
        r = self.client.post("/jobs", json={"style": "dark rock", "lyrics": "[verse] la"})
        self.assertEqual(r.status_code, 200, r.text)
        new = int(r.json()["id"])
        gens = [it["job_id"] for it in self._live() if it["kind"] == "generate"]
        self.assertEqual(gens, [1, 2, 3, new])



@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestDeleteQueuedGeneration(_Case):
    """Регрессия: удалённый ждущий трек не остаётся незавершённой записью очереди."""

    def _delete_and_check(self, jid):
        op = next(it for it in self._live() if it["kind"] == "generate" and it["job_id"] == jid)
        self.assertEqual(op["state"], "waiting")
        r = self.client.delete(f"/jobs/{jid}")
        self.assertEqual(r.status_code, 200, r.text)
        gens = [it for it in self._snap()["items"] if it["kind"] == "generate" and it["job_id"] == jid]
        self.assertTrue(all(it["state"] == "canceled" for it in gens), gens)
        self.assertNotIn(op["id"], [it["id"] for it in self._live()], "запись удалённого трека висит")
        st = self.client.get("/stats")
        self.assertEqual(st.status_code, 200, st.text)
        self.assertEqual(st.json()["gpu_waiting"], 0, "удалённый трек считается ждущим")

    def test_delete_waiting_generation(self):
        self._holder()
        self._delete_and_check(_job_id(self, _post_job(self)))

    def test_delete_waiting_generation_on_pause(self):
        r = self.client.post("/queue/pause", json={"paused": True})
        self.assertEqual(r.status_code, 200, r.text)
        self._delete_and_check(_job_id(self, _post_job(self)))
        self.assertIs(self._snap()["paused"], True)


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestLateRenderCancelAndRetry(_LifeCase):
    """Регрессия: отмена, опоздавшая к концу прогона, не отменяет повторную генерацию."""

    def test_late_cancel_does_not_cancel_retry(self):
        seen = []

        def honest(kw):  # пайплайн честно смотрит на флаг отмены
            seen.append(kw["cancelled"]())
            return InterruptedError("canceled") if seen[-1] else _FakeSong()

        pipe = self._use_pipe(_FakePipe(RuntimeError("boom"), honest))
        jid = self._new()
        self._run_queue_once()
        self.assertEqual(self._status(jid)["status"], "error")
        # запоздалый запрос отмены: флаг рендера ставится уже после конца прогона
        self.w._flag_render_cancel(jid)
        r = self.client.post(f"/jobs/{jid}/retry")
        self.assertEqual(r.status_code, 200, r.text)
        self._run_queue_once()
        self.assertEqual(len(pipe.calls), 2)
        self.assertEqual(seen, [False], "повторный прогон увидел старую отмену")
        self.assertEqual(self._status(jid)["status"], "done")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestImportTranscribeCanceled(_Case):
    """Регрессия: отмена распознавания при импорте — трек создан, ответ 200 с причиной."""

    def test_cancel_transcribe_during_import(self):
        started, release = threading.Event(), threading.Event()

        def slow_transcribe(src, dst):
            started.set()
            release.wait(JOIN)
            return {"abc": "X:1\nK:C\nC"}

        def cancel_from_queue():
            if not started.wait(JOIN):
                return "распознавание не началось"
            run = [it for it in self.q.snapshot()["items"] if it["state"] == "running"]
            r = self.client.post(f"/queue/ops/{run[0]['id']}/cancel")
            release.set()
            return r.status_code, r.json()

        with self._conn() as c:
            before = c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]
        with mock.patch.object(self.w, "ss_transcribe", side_effect=slow_transcribe):
            t = _start(cancel_from_queue)
            try:
                r = self.client.post("/tracks/import", content=b"IMPORTED",
                                     headers={"X-Filename": "song.flac"},
                                     params={"transcribe": "true"})
            finally:
                release.set()
                t.join(JOIN)
        self.assertFalse(t.is_alive(), "поток отмены завис")
        self.assertIsNone(t.exc)
        self.assertEqual(t.result, (200, {"canceled": True, "running": True}))
        self.assertEqual(r.status_code, 200, r.text)
        body = r.json()
        self.assertIn("отменено", body["transcribe_error"])
        jid = int(body["id"])
        self.assertEqual(self.client.get(f"/jobs/{jid}").json()["status"], "done")
        with self._conn() as c:
            after = c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]
        self.assertEqual(after - before, 1, "трек должен быть создан ровно один")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestCorpusLyricsCanceled(_Case):
    """Регрессия кросс-ревью (усл. 11/13): отмена шага распознавания слов при добавлении трека
    в корпус. Трек и его ноты уже на диске — ответ 200, info.json записан, в lyrics_error —
    «отменено». Процесс whisper настоящий (заменена только команда на sleep 30), отмена —
    через HTTP очереди. Старое поведение: 409 из тяжёлой секции whisper пробрасывался из
    обработчика — ответ 409, info.json не записан, трек корпуса оставался без паспорта."""

    def test_cancel_whisper_step(self):
        import json
        import sys
        import tempfile
        from pathlib import Path
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        corpus_dir = Path(td.name) / "corpus"
        started = threading.Event()
        real_run_proc = self.q.run_proc

        def slow_whisper_proc(cmd, timeout=None, **kw):
            started.set()
            return real_run_proc([sys.executable, "-c", "import time; time.sleep(30)"],
                                 timeout=timeout, **kw)

        def cancel_from_queue():
            if not started.wait(JOIN):
                return "распознавание слов не началось"
            run = [it for it in self.q.snapshot()["items"] if it["state"] == "running"]
            r = self.client.post(f"/queue/ops/{run[0]['id']}/cancel")
            return r.status_code, r.json()

        patches = [
            mock.patch.object(self.w, "CORPUS_DIR", corpus_dir),
            mock.patch.object(self.w, "WHISPER_PY", Path(sys.executable)),
            mock.patch.object(self.w, "analyze_file", return_value={}),
            mock.patch.object(self.w, "ss_transcribe",
                              return_value={"abc": "X:1\nK:C\nC", "stats": {}}),
            mock.patch.object(self.q, "run_proc", side_effect=slow_whisper_proc),
        ]
        for p in patches:
            p.start()
            self.addCleanup(p.stop)
        r = self.client.post("/corpus", json={"name": "c"})
        self.assertEqual(r.status_code, 200, r.text)
        cid = r.json()["id"]
        t = _start(cancel_from_queue)
        t0 = time.monotonic()
        r = self.client.post(f"/corpus/{cid}/track", content=b"AUDIO",
                             headers={"X-Filename": "song.flac"})
        elapsed = time.monotonic() - t0
        t.join(JOIN)
        self.assertFalse(t.is_alive(), "поток отмены завис")
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(t.result, (200, {"canceled": True, "running": True}))
        self.assertLess(elapsed, 10, "процесс whisper не остановлен отменой")
        self.assertEqual(r.status_code, 200, r.text)
        body = r.json()
        self.assertIn("отменено", body["lyrics_error"])
        self.assertEqual(body.get("abc"), "X:1\nK:C\nC", "ноты, распознанные до отмены, потеряны")
        infos = list(corpus_dir.glob(f"{cid}/tracks/*/info.json"))
        self.assertEqual(len(infos), 1, "info.json трека корпуса не записан")
        saved = json.loads(infos[0].read_text())
        self.assertIn("отменено", saved["lyrics_error"])
        self.assertEqual([it for it in self._live()], [], "в очереди осталась повисшая запись")


if __name__ == "__main__":
    unittest.main()
