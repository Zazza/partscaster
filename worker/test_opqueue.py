"""Спецификация модуля worker/opqueue.py — общая очередь долгих операций воркера
(задача internal-shared-queue, раздел «Контракт» карточки).

Что должно работать (ТК карточки):
ТК1  слот выдаётся первой ждущей по месту (pos), а не по порядку регистрации;
     после перестановки (move) — в новом порядке.
ТК2  генерации и долгие работы в одной очереди; одновременно не больше одной идущей;
     next_generation отдаёт генерацию, получившую слот, один раз.
ТК4  тяжёлая секция внутри идущей op (use_op / вложенный slot) не встаёт в очередь
     повторно и не блокируется.
ТК5  метка неизвестной/завершённой op → OpGone; метка ждущей client-op → ждать её
     слота (не исполнение вне очереди).
ТК7  snapshot: поля op, порядок (идущая, ждущие по pos, завершённые новыми первыми,
     не больше keep_done), пустая очередь — пустой список + paused.
ТК8  move: край — False без изменений; идущая/завершённая — ValueError; неизвестная —
     KeyError; on_pos вызывается для обеих переставленных op.
ТК9  пауза: идущая доделывается, новая не стартует; снятие паузы — стартует.
ТК10 client-op без beat дольше ttl → interrupted, слот следующей; finish завершённой —
     ValueError.
ТК12 cancel ждущей → canceled сразу, работа не вызывается; повторный cancel — без
     падения; неизвестная — KeyError.
ТК13 cancel идущей client-цепочки: следующий шаг (вход в slot) не начинается, итог
     canceled.
ТК14 cancel идущей op с внешним процессом (run_proc): процесс убит, OpCanceled < 5 с,
     op canceled; OpCanceled не ловится `except Exception`.
ТК15 после cancel/interrupted слот получает следующая ждущая.

Время ttl — внедряемые часы (FakeClock). Потоки — только с join(timeout).
"""
import os
import sys
import tempfile
import threading
import time
import unittest
from pathlib import Path

import opqueue
from opqueue import OpCanceled, OpGone, OpQueue

FINAL = {"done", "error", "canceled", "interrupted"}
OP_KEYS = {"id", "kind", "title", "job_id", "source", "state", "pos", "result_job_id",
           "error", "cancel_requested", "client", "created_at", "started_at", "finished_at"}
JOIN = 5.0  # предел ожидания потока: дольше — считаем зависанием (дедлок)


class FakeClock:
    def __init__(self, t=1000.0):
        self.t = t

    def __call__(self):
        return self.t

    def advance(self, dt):
        self.t += dt


def _id(op):
    return op.as_dict()["id"]


def _state(q, op_id):
    for it in q.snapshot()["items"]:
        if it["id"] == op_id:
            return it["state"]
    return None


def _item(q, op_id):
    for it in q.snapshot()["items"]:
        if it["id"] == op_id:
            return it
    return None


def _running_ids(q):
    return [it["id"] for it in q.snapshot()["items"] if it["state"] == "running"]


def _waiting_ids(q):
    return [it["id"] for it in q.snapshot()["items"] if it["state"] == "waiting"]


def _wait_until(cond, timeout=JOIN):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if cond():
            return True
        time.sleep(0.01)
    return cond()


class _Thread(threading.Thread):
    """Поток, запоминающий исход: result / исключение (в т. ч. BaseException)."""

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


class _Case(unittest.TestCase):
    def setUp(self):
        self.clock = FakeClock()
        self.q = OpQueue(clock=self.clock, ttl=10)

    def _join(self, t):
        t.join(JOIN)
        self.assertFalse(t.is_alive(), "поток завис (дедлок/вечное ожидание)")

    def _holder(self):
        """client-op, занявшая свободный слот."""
        h = self.q.add("preset", "holder", client=True)
        self.assertEqual(h.as_dict()["state"], "running", "свободный слот не выдан первой op")
        return _id(h)


class TestExceptions(unittest.TestCase):
    def test_op_canceled_not_caught_by_except_exception(self):
        self.assertTrue(issubclass(OpCanceled, BaseException))
        self.assertFalse(issubclass(OpCanceled, Exception),
                         "OpCanceled проглотится обычным `except Exception`")
        caught_by_exception = False
        try:
            try:
                raise OpCanceled()
            except Exception:  # noqa: BLE001 — это и проверяем
                caught_by_exception = True
        except OpCanceled:
            pass
        self.assertFalse(caught_by_exception)

    def test_default_ttl(self):
        self.assertEqual(opqueue.OP_TTL, 180)


class TestOrder(_Case):
    """ТК1, ТК2."""

    def test_tk1_slot_follows_pos_after_move(self):
        h = self._holder()
        a = _id(self.q.add("stems", "a"))
        b = _id(self.q.add("stems", "b"))
        c = _id(self.q.add("stems", "c"))
        self.assertEqual(_waiting_ids(self.q), [a, b, c])
        self.assertTrue(self.q.move(c, -1))
        self.assertTrue(self.q.move(c, -1))
        self.assertEqual(_waiting_ids(self.q), [c, a, b])
        order = []
        cur = h
        for _ in range(3):
            self.q.release(cur)
            run = _running_ids(self.q)
            self.assertEqual(len(run), 1, run)
            cur = run[0]
            order.append(cur)
        self.assertEqual(order, [c, a, b], "слот выдан не по месту")

    def test_add_with_pos_goes_before(self):
        self._holder()
        a = self.q.add("stems", "a")
        b = _id(self.q.add("stems", "b"))
        c = _id(self.q.add("generate", "c", job_id=7, pos=a.as_dict()["pos"] - 1))
        self.assertEqual(_waiting_ids(self.q), [c, _id(a), b])

    def test_first_op_gets_free_slot_immediately(self):
        op = self.q.add("stems", "x")
        self.assertEqual(op.as_dict()["state"], "running")
        self.assertIsNotNone(op.as_dict()["started_at"])

    def test_tk2_generation_and_long_op_one_queue(self):
        h = self._holder()
        gen = _id(self.q.add("generate", "Song", job_id=5))
        st = _id(self.q.add("stems", "stems #3", job_id=3))
        self.assertTrue(self.q.move(st, -1))
        self.q.release(h)
        self.assertEqual(_running_ids(self.q), [st])
        self.assertEqual(_state(self.q, gen), "waiting")
        self.assertIsNone(self.q.next_generation(0.05), "генерация взята без слота")
        self.q.release(st)
        self.assertEqual(_running_ids(self.q), [gen])
        got = self.q.next_generation(1.0)
        self.assertIsNotNone(got)
        self.assertEqual(got.as_dict()["id"], gen)
        self.assertEqual(got.as_dict()["job_id"], 5)
        self.assertIsNone(self.q.next_generation(0.05), "одна генерация выдана дважды")

    def test_next_generation_none_on_empty(self):
        t0 = time.monotonic()
        self.assertIsNone(self.q.next_generation(0.1))
        self.assertLess(time.monotonic() - t0, JOIN)

    def test_tk2_mutual_exclusion_threads(self):
        q = OpQueue()  # реальные часы
        lock = threading.Lock()
        active = [0]
        peak = [0]

        def work(n):
            def fn():
                with q.slot("stems", f"t{n}", job_id=n):
                    with lock:
                        active[0] += 1
                        peak[0] = max(peak[0], active[0])
                    time.sleep(0.03)
                    with lock:
                        active[0] -= 1
            return fn

        ts = [_start(work(i)) for i in range(4)]
        for t in ts:
            self._join(t)
            self.assertIsNone(t.exc, t.exc)
        self.assertEqual(peak[0], 1, "две операции работали одновременно")
        states = [it["state"] for it in q.snapshot()["items"]]
        self.assertEqual(states.count("done"), 4, states)

    def test_slot_error_marks_error_and_frees_slot(self):
        with self.assertRaises(RuntimeError):
            with self.q.slot("stems", "boom"):
                raise RuntimeError("x")
        items = self.q.snapshot()["items"]
        self.assertEqual(items[0]["state"], "error")
        self.assertEqual(_running_ids(self.q), [])
        nxt = self.q.add("stems", "next")
        self.assertEqual(nxt.as_dict()["state"], "running")

    def test_acquire_waits_for_slot(self):
        h = self._holder()
        a = _id(self.q.add("stems", "a"))
        t = _start(lambda: self.q.acquire(a, timeout=JOIN))
        time.sleep(0.1)
        self.assertTrue(t.is_alive(), "acquire вернулся, пока слот занят")
        self.q.release(h)
        self._join(t)
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(_state(self.q, a), "running")


class TestReentry(_Case):
    """ТК4, ТК5."""

    def test_tk4_nested_slot_no_new_op_no_deadlock(self):
        seen = {}

        def fn():
            with self.q.slot("render", "outer", job_id=1):
                with self.q.slot("stems", "inner", job_id=1):
                    seen["inner"] = len(self.q.snapshot()["items"])
        t = _start(fn)
        self._join(t)
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(seen.get("inner"), 1, "вложенная секция встала в очередь отдельной op")

    def test_tk4_use_op_running_client_op_enters_without_queue(self):
        a = self._holder()
        ran = {}

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "step", job_id=3):
                    ran["items"] = [it["id"] for it in self.q.snapshot()["items"]]
        t = _start(fn)
        self._join(t)
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(ran.get("items"), [a], "шаг цепочки создал новую op")
        self.assertEqual(_state(self.q, a), "running", "выход из шага закрыл op цепочки")

    def test_tk5_unknown_op_gone(self):
        def fn():
            with self.q.use_op(999999):
                with self.q.slot("stems", "x"):
                    return "ran"
        t = _start(fn)
        self._join(t)
        self.assertIsInstance(t.exc, OpGone)
        self.assertIsNone(t.result)

    def test_tk5_finished_op_gone(self):
        a = self._holder()
        self.q.finish(a, "done", result_job_id=12)

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "x"):
                    return "ran"
        t = _start(fn)
        self._join(t)
        self.assertIsInstance(t.exc, OpGone)
        self.assertIsNone(t.result)

    def test_tk5_waiting_client_op_waits_for_its_slot(self):
        a = self._holder()
        b = _id(self.q.add("preset", "b", client=True))
        entered = threading.Event()

        def fn():
            with self.q.use_op(b):
                with self.q.slot("stems", "step"):
                    entered.set()
                    return _state(self.q, b)
        t = _start(fn)
        self.assertFalse(entered.wait(0.3), "шаг ждущей op исполнен вне очереди")
        self.q.finish(a, "done")
        self._join(t)
        self.assertIsNone(t.exc, t.exc)
        self.assertEqual(t.result, "running")
        self.assertEqual(_running_ids(self.q), [b])


class TestSnapshot(_Case):
    """ТК7."""

    def test_empty(self):
        self.assertEqual(self.q.snapshot(), {"paused": False, "items": []})

    def test_fields(self):
        op = self.q.add("stems", "Stems of Song", job_id=4, source="mcp", client=True)
        d = op.as_dict()
        self.assertTrue(OP_KEYS <= set(d), OP_KEYS - set(d))
        self.assertEqual(d["kind"], "stems")
        self.assertEqual(d["title"], "Stems of Song")
        self.assertEqual(d["job_id"], 4)
        self.assertEqual(d["source"], "mcp")
        self.assertIs(d["client"], True)
        self.assertIs(d["cancel_requested"], False)
        item = self.q.snapshot()["items"][0]
        self.assertTrue(OP_KEYS <= set(item), OP_KEYS - set(item))
        self.assertEqual(item["id"], d["id"])

    def test_result_fields_after_finish(self):
        a = self._holder()
        self.q.finish(a, "done", result_job_id=42)
        b = _id(self.q.add("preset", "b", client=True))
        self.q.finish(b, "error", error="нет звука")
        self.assertEqual(_item(self.q, a)["result_job_id"], 42)
        self.assertEqual(_item(self.q, b)["state"], "error")
        self.assertEqual(_item(self.q, b)["error"], "нет звука")
        self.assertIsNotNone(_item(self.q, b)["finished_at"])

    def test_order_running_waiting_done(self):
        q = OpQueue(clock=self.clock, ttl=10, keep_done=2)
        d1 = _id(q.add("stems", "d1"))
        q.release(d1)
        d2 = _id(q.add("stems", "d2"))
        q.release(d2)
        d3 = _id(q.add("stems", "d3"))
        q.release(d3)
        r = _id(q.add("stems", "r"))
        w1 = _id(q.add("stems", "w1"))
        w2 = _id(q.add("stems", "w2"))
        q.move(w2, -1)
        ids = [it["id"] for it in q.snapshot()["items"]]
        self.assertEqual(ids, [r, w2, w1, d3, d2], "порядок/лимит завершённых")


class TestMove(_Case):
    """ТК8."""

    def test_edges_no_change(self):
        self._holder()
        a = _id(self.q.add("stems", "a"))
        b = _id(self.q.add("stems", "b"))
        self.assertIs(self.q.move(a, -1), False)
        self.assertIs(self.q.move(b, +1), False)
        self.assertEqual(_waiting_ids(self.q), [a, b])

    def test_single_waiting(self):
        self._holder()
        a = _id(self.q.add("stems", "a"))
        self.assertIs(self.q.move(a, -1), False)
        self.assertIs(self.q.move(a, +1), False)

    def test_down_swaps(self):
        self._holder()
        a = _id(self.q.add("stems", "a"))
        b = _id(self.q.add("generate", "b", job_id=2))
        self.assertIs(self.q.move(a, +1), True)
        self.assertEqual(_waiting_ids(self.q), [b, a])

    def test_running_done_unknown(self):
        h = self._holder()
        with self.assertRaises(ValueError):
            self.q.move(h, -1)
        self.q.release(h)
        with self.assertRaises(ValueError):
            self.q.move(h, +1)
        with self.assertRaises(KeyError):
            self.q.move(424242, -1)

    def test_on_pos_called_for_both(self):
        calls = []
        q = OpQueue(clock=self.clock, ttl=10, on_pos=lambda op: calls.append(op.as_dict()["id"]))
        q.add("preset", "h", client=True)
        a = _id(q.add("generate", "a", job_id=1))
        b = _id(q.add("generate", "b", job_id=2))
        calls.clear()
        self.assertTrue(q.move(b, -1))
        self.assertEqual(set(calls), {a, b})
        pos = {it["id"]: it["pos"] for it in q.snapshot()["items"]}
        self.assertLess(pos[b], pos[a])


class TestPause(_Case):
    """ТК9."""

    def test_pause_lets_running_finish_blocks_new(self):
        h = self._holder()
        self.q.set_paused(True)
        self.assertIs(self.q.paused, True)
        self.assertEqual(_state(self.q, h), "running", "пауза остановила идущую")
        a = _id(self.q.add("stems", "a"))
        self.q.release(h)
        self.assertEqual(_state(self.q, h), "done")
        self.assertEqual(_running_ids(self.q), [], "на паузе выдан слот")
        self.assertEqual(_state(self.q, a), "waiting")
        self.assertIs(self.q.snapshot()["paused"], True)
        self.q.set_paused(False)
        self.assertEqual(_running_ids(self.q), [a])

    def test_add_on_paused_empty_queue_waits(self):
        self.q.set_paused(True)
        a = self.q.add("stems", "a")
        self.assertEqual(a.as_dict()["state"], "waiting")
        self.q.set_paused(False)
        self.assertEqual(_state(self.q, _id(a)), "running")

    def test_acquire_waits_while_paused(self):
        self.q.set_paused(True)
        a = _id(self.q.add("stems", "a"))
        t = _start(lambda: self.q.acquire(a, timeout=JOIN))
        time.sleep(0.1)
        self.assertTrue(t.is_alive())
        self.q.set_paused(False)
        self._join(t)
        self.assertIsNone(t.exc, t.exc)


class TestTtl(_Case):
    """ТК10, ТК15 (interrupted)."""

    def test_stale_running_client_interrupted_next_gets_slot(self):
        a = self._holder()
        b = _id(self.q.add("stems", "b"))
        self.clock.advance(11)
        self.q.sweep()
        self.assertEqual(_state(self.q, a), "interrupted")
        self.assertEqual(_running_ids(self.q), [b])

    def test_beat_keeps_alive(self):
        a = self._holder()
        self.clock.advance(8)
        d = self.q.beat(a)
        self.assertEqual(d["state"], "running")
        self.clock.advance(8)
        self.q.sweep()
        self.assertEqual(_state(self.q, a), "running", "отметка «жива» не учтена")
        self.clock.advance(3)
        self.q.sweep()
        self.assertEqual(_state(self.q, a), "interrupted")

    def test_stale_waiting_client_interrupted(self):
        self._holder()
        self.clock.advance(5)
        w = _id(self.q.add("preset", "w", client=True))
        self.clock.advance(11)
        self.q.sweep()
        self.assertEqual(_state(self.q, w), "interrupted")

    def test_non_client_not_swept(self):
        a = _id(self.q.add("stems", "a"))
        self.clock.advance(1000)
        self.q.sweep()
        self.assertEqual(_state(self.q, a), "running")

    def test_beat_after_interrupted_does_not_revive(self):
        a = self._holder()
        self.clock.advance(11)
        self.q.sweep()
        d = self.q.beat(a)
        self.assertEqual(d["state"], "interrupted")
        self.assertEqual(_state(self.q, a), "interrupted")

    def test_beat_unknown(self):
        with self.assertRaises(KeyError):
            self.q.beat(777)

    def test_acquire_of_interrupted_raises_gone(self):
        self._holder()
        w = _id(self.q.add("preset", "w", client=True))
        t = _start(lambda: self.q.acquire(w, timeout=JOIN))
        time.sleep(0.05)
        self.clock.advance(11)
        self.q.sweep()
        self._join(t)
        self.assertIsInstance(t.exc, OpGone)

    def test_finish_rules(self):
        a = self._holder()
        d = self.q.finish(a, "done", result_job_id=9)
        self.assertEqual(d["state"], "done")
        self.assertEqual(d["result_job_id"], 9)
        with self.assertRaises(ValueError):
            self.q.finish(a, "done")
        gen = _id(self.q.add("generate", "g", job_id=1))
        with self.assertRaises(ValueError):
            self.q.finish(gen, "done")
        with self.assertRaises(KeyError):
            self.q.finish(5555, "done")

    def test_finish_waiting_client_canceled(self):
        self._holder()
        w = _id(self.q.add("preset", "w", client=True))
        self.q.finish(w, "canceled")
        self.assertEqual(_state(self.q, w), "canceled")

    def test_stale_waiting_client_not_revived_by_getting_slot(self):
        # регрессия: получение слота не считается отметкой «жива» — две давно молчащие
        # ждущие client-op после закрытия держателя обе прерываются одним sweep
        h = self._holder()
        w1 = _id(self.q.add("preset", "w1", client=True))
        w2 = _id(self.q.add("preset", "w2", client=True))
        self.clock.advance(11)
        self.q.finish(h, "done")
        self.q.sweep()
        self.assertEqual(_state(self.q, w1), "interrupted", "слот продлил жизнь молчащей op")
        self.assertEqual(_state(self.q, w2), "interrupted")
        self.assertEqual(_running_ids(self.q), [])


class TestCancel(_Case):
    """ТК12, ТК13, ТК15."""

    def test_tk12_cancel_waiting_work_not_called(self):
        self._holder()
        called = []

        def fn():
            with self.q.slot("stems", "victim", job_id=8):
                called.append(1)
        t = _start(fn)
        self.assertTrue(_wait_until(lambda: any(
            it["title"] == "victim" for it in self.q.snapshot()["items"])))
        vid = next(it["id"] for it in self.q.snapshot()["items"] if it["title"] == "victim")
        r = self.q.cancel(vid)
        self.assertEqual(r, {"canceled": True, "running": False})
        self._join(t)
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertEqual(called, [], "работа отменённой ждущей вызвана")
        self.assertEqual(_state(self.q, vid), "canceled")

    def test_tk12_repeat_cancel_and_unknown(self):
        self._holder()
        w = _id(self.q.add("stems", "w"))
        self.assertIs(self.q.cancel(w)["canceled"], True)
        r = self.q.cancel(w)
        self.assertIs(r["canceled"], False)
        self.assertEqual(_state(self.q, w), "canceled")
        with self.assertRaises(KeyError):
            self.q.cancel(31337)

    def test_canceled_waiting_never_gets_slot(self):
        h = self._holder()
        a = _id(self.q.add("stems", "a"))
        b = _id(self.q.add("stems", "b"))
        self.q.cancel(a)
        self.q.release(h)
        self.assertEqual(_running_ids(self.q), [b])

    def test_acquire_canceled_while_waiting(self):
        self._holder()
        w = _id(self.q.add("preset", "w", client=True))
        t = _start(lambda: self.q.acquire(w, timeout=JOIN))
        time.sleep(0.05)
        self.q.cancel(w)
        self._join(t)
        self.assertIsInstance(t.exc, OpCanceled)

    def test_tk13_cancel_running_chain_next_step_not_started(self):
        a = self._holder()
        hooks = []
        self.q.on_cancel(a, lambda *x, **k: hooks.append(1))
        r = self.q.cancel(a)
        self.assertEqual(r, {"canceled": True, "running": True})
        self.assertEqual(hooks, [1], "хук отмены идущей не вызван")
        d = self.q.beat(a)
        self.assertIs(d["cancel_requested"], True)
        step = []

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "next step"):
                    step.append(1)
        t = _start(fn)
        self._join(t)
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertEqual(step, [], "следующий шаг отменённой цепочки начался")
        self.q.finish(a, "canceled")
        self.assertEqual(_state(self.q, a), "canceled")
        self.assertEqual(_running_ids(self.q), [])

    def test_tk15_cancel_running_inprocess_step_ends_canceled_next_starts(self):
        """Работа без точки прерывания: отмена срабатывает по окончании шага."""
        q = OpQueue()
        inside = threading.Event()
        go = threading.Event()

        def fn():
            with q.slot("stems", "roformer", job_id=1):
                inside.set()
                go.wait(JOIN)
            return "finished"
        t = _start(fn)
        self.assertTrue(inside.wait(JOIN))
        a = _running_ids(q)[0]
        nxt = _id(q.add("stems", "next"))
        r = q.cancel(a)
        self.assertEqual(r, {"canceled": True, "running": True})
        self.assertEqual(_state(q, a), "running", "слот отдан до конца шага")
        self.assertEqual(_state(q, nxt), "waiting")
        go.set()
        self._join(t)
        self.assertIsInstance(t.exc, OpCanceled, "отменённый шаг завершился как успешный")
        self.assertEqual(_state(q, a), "canceled")
        self.assertEqual(_running_ids(q), [nxt])


class TestRunProc(_Case):
    """ТК14: внешний процесс убивается отменой."""

    def setUp(self):
        super().setUp()
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)

    def test_run_proc_plain(self):
        q = OpQueue()
        with q.slot("whisper", "w"):
            r = q.run_proc([sys.executable, "-c", "print('hi')"])
        self.assertEqual(r.returncode, 0)
        self.assertEqual(r.stdout.strip(), "hi")

    def test_tk14_cancel_kills_process(self):
        q = OpQueue()
        pidfile = Path(self._td.name) / "pid"
        code = ("import os, sys, time; open(sys.argv[1], 'w').write(str(os.getpid())); "
                "time.sleep(30)")
        out = {}

        def fn():
            try:
                with q.slot("stems", "roformer", job_id=2):
                    q.run_proc([sys.executable, "-c", code, str(pidfile)])
                    out["after"] = True
            except Exception as e:  # noqa: BLE001 — OpCanceled сюда попасть не должен
                out["caught_by_exception"] = e
                raise
        t = _start(fn)
        self.assertTrue(_wait_until(lambda: pidfile.is_file() and pidfile.read_text()),
                        "процесс не стартовал")
        pid = int(pidfile.read_text())
        a = _running_ids(q)[0]
        nxt = _id(q.add("stems", "next"))
        t0 = time.monotonic()
        r = q.cancel(a)
        self.assertIs(r["running"], True)
        t.join(5)
        self.assertFalse(t.is_alive(), "отмена не прервала внешний процесс за 5 с")
        self.assertLess(time.monotonic() - t0, 5)
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertNotIn("caught_by_exception", out, "OpCanceled пойман except Exception")
        self.assertNotIn("after", out, "после отмены шаг продолжился")
        self.assertTrue(_wait_until(lambda: not _alive(pid), 2), "процесс жив после отмены")
        self.assertEqual(_state(q, a), "canceled")
        self.assertEqual(_running_ids(q), [nxt])


class TestNoTwoOnGpuAfterCancel(_Case):
    """ТК2 + усл. 12/13: после отмены/прерывания идущей client-op следующая получает
    слот только когда шаг вышел из slot() — двух работающих на видеокарте не бывает."""

    def setUp(self):
        super().setUp()
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)

    def test_finish_canceled_while_step_inside_slot_next_waits_for_step(self):
        a = self._holder()
        inside = threading.Event()
        go = threading.Event()
        seen = {}

        def fn():
            with self.q.use_op(a):
                try:
                    with self.q.slot("stems", "roformer", job_id=1):
                        inside.set()
                        go.wait(JOIN)
                        seen["nxt_in_step"] = _state(self.q, nxt)
                except OpCanceled:
                    seen["canceled"] = True
                    raise
        nxt = _id(self.q.add("stems", "next"))
        t = _start(fn)
        self.assertTrue(inside.wait(JOIN), "шаг не вошёл в слот")
        self.q.cancel(a)
        self.q.finish(a, "canceled")
        time.sleep(0.2)
        self.assertEqual(_state(self.q, nxt), "waiting",
                         "слот отдан следующей, пока шаг отменённой op ещё на видеокарте")
        self.assertNotIn(nxt, _running_ids(self.q))
        go.set()
        self._join(t)
        self.assertTrue(t.exc is None or isinstance(t.exc, OpCanceled), t.exc)
        self.assertEqual(seen.get("nxt_in_step"), "waiting")
        self.assertTrue(_wait_until(lambda: _state(self.q, nxt) == "running"),
                        "после выхода шага слот не перешёл к следующей")
        self.assertEqual(_running_ids(self.q), [nxt])
        self.assertEqual(_state(self.q, a), "canceled")

    def test_sweep_interrupted_kills_proc_next_waits_for_step(self):
        a = self._holder()
        pidfile = Path(self._td.name) / "pid"
        code = ("import os, sys, time; open(sys.argv[1], 'w').write(str(os.getpid())); "
                "time.sleep(30)")
        seen = {}

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "roformer", job_id=2):
                    try:
                        seen["ret"] = self.q.run_proc([sys.executable, "-c", code, str(pidfile)])
                    except OpCanceled:
                        seen["nxt_at_raise"] = _state(self.q, nxt)
                        raise
        nxt = _id(self.q.add("stems", "next"))
        t = _start(fn)
        self.assertTrue(_wait_until(lambda: pidfile.is_file() and pidfile.read_text()),
                        "процесс не стартовал")
        pid = int(pidfile.read_text())
        self.clock.advance(11)  # ttl=10, a без beat; nxt — не client-op, порогу не подлежит
        t0 = time.monotonic()
        self.q.sweep()
        self.assertEqual(_state(self.q, a), "interrupted")
        t.join(5)
        self.assertFalse(t.is_alive(), "прерывание порогом не остановило внешний процесс за 5 с")
        self.assertLess(time.monotonic() - t0, 5)
        self.assertNotIn("ret", seen, "run_proc вернул результат вместо OpCanceled")
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertEqual(seen.get("nxt_at_raise"), "waiting",
                         "следующая получила слот, пока шаг прерванной op ещё внутри slot()")
        self.assertTrue(_wait_until(lambda: not _alive(pid), 2), "процесс жив после прерывания")
        self.assertTrue(_wait_until(lambda: _state(self.q, nxt) == "running"),
                        "после выхода шага слот не перешёл к следующей")
        self.assertEqual(_running_ids(self.q), [nxt])
        self.assertEqual(_state(self.q, a), "interrupted")


class TestCloseDuringProc(_Case):
    """Регрессия кросс-ревью (усл. 11–13): клиент закрыл свою операцию (finish error/done),
    пока её шаг внутри slot() держит внешний процесс. Процесс должен быть убит, run_proc —
    поднять OpCanceled, а следующая операция — получить слот только после выхода шага.
    Старое поведение: finish закрывал запись и отдавал слот, но процесс шага жил до конца
    (30 с), run_proc возвращал результат, а следующая op стартовала параллельно."""

    def setUp(self):
        super().setUp()
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)

    def _run(self, state):
        a = self._holder()
        pidfile = Path(self._td.name) / f"pid-{state}"
        code = ("import os, sys, time; open(sys.argv[1], 'w').write(str(os.getpid())); "
                "time.sleep(30)")
        seen = {}

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "roformer", job_id=3):
                    try:
                        seen["ret"] = self.q.run_proc([sys.executable, "-c", code, str(pidfile)])
                    except OpCanceled:
                        seen["nxt_at_raise"] = _state(self.q, nxt)
                        raise
        nxt = _id(self.q.add("stems", "next"))
        t = _start(fn)
        self.assertTrue(_wait_until(lambda: pidfile.is_file() and pidfile.read_text()),
                        "процесс не стартовал")
        pid = int(pidfile.read_text())
        t0 = time.monotonic()
        self.q.finish(a, state, error="клиент ушёл" if state == "error" else "")
        t.join(5)
        self.assertFalse(t.is_alive(), f"finish {state} не остановил внешний процесс за 5 с")
        self.assertLess(time.monotonic() - t0, 5)
        self.assertNotIn("ret", seen, "run_proc вернул результат вместо OpCanceled")
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertEqual(seen.get("nxt_at_raise"), "waiting",
                         "следующая получила слот, пока шаг закрытой op ещё внутри slot()")
        self.assertTrue(_wait_until(lambda: not _alive(pid), 2), "процесс жив после finish")
        self.assertTrue(_wait_until(lambda: _state(self.q, nxt) == "running"),
                        "после выхода шага слот не перешёл к следующей")
        self.assertEqual(_running_ids(self.q), [nxt])
        self.assertEqual(_state(self.q, a), state)

    def test_finish_error_kills_proc(self):
        self._run("error")

    def test_finish_done_kills_proc(self):
        self._run("done")


class TestCloseBeforeProcRegistered(_Case):
    """Регрессия кросс-ревью (усл. 12/13): операция закрыта (cancel или sweep по порогу)
    в окне между запуском процесса и его привязкой к op. Процесс должен быть убит, run_proc —
    поднять OpCanceled. Старое поведение: отмена убивала только уже привязанные процессы,
    процесс из окна оставался жить (здесь — 30 с), run_proc ждал его до конца."""

    def setUp(self):
        super().setUp()
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)

    def _run(self, close):
        from unittest import mock
        a = self._holder()
        pidfile = Path(self._td.name) / "pid"
        code = ("import os, sys, time; open(sys.argv[1], 'w').write(str(os.getpid())); "
                "time.sleep(30)")
        real_popen = opqueue.subprocess.Popen
        box = {}

        def popen(*args, **kw):
            p = real_popen(*args, **kw)
            box["pid"] = p.pid
            close(a)  # закрытие op до того, как run_proc получил процесс обратно
            return p

        seen = {}

        def fn():
            with self.q.use_op(a):
                with self.q.slot("stems", "roformer", job_id=4):
                    seen["ret"] = self.q.run_proc([sys.executable, "-c", code, str(pidfile)])
        with mock.patch.object(opqueue.subprocess, "Popen", popen):
            t0 = time.monotonic()
            t = _start(fn)
            t.join(5)
        self.assertFalse(t.is_alive(), "процесс из окна запуска не убит за 5 с")
        self.assertLess(time.monotonic() - t0, 5)
        self.assertNotIn("ret", seen, "run_proc вернул результат вместо OpCanceled")
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertIn("pid", box, "run_proc не запускал процесс через subprocess.Popen")
        self.assertTrue(_wait_until(lambda: not _alive(box["pid"]), 2), "процесс жив")
        return a

    def test_cancel_in_window(self):
        a = self._run(self.q.cancel)
        # client-op: отмена идущей ставит метку, итог закрывает сам клиент (finish)
        self.assertIs(_item(self.q, a)["cancel_requested"], True)

    def test_sweep_in_window(self):
        def close(_op_id):
            self.clock.advance(11)  # ttl=10, без beat
            self.q.sweep()
        a = self._run(close)
        self.assertEqual(_state(self.q, a), "interrupted")


def _alive(pid):
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return False
    try:  # зомби (не дождались) — тоже мёртв
        with open(f"/proc/{pid}/stat") as f:
            return f.read().split(")")[-1].split()[0] != "Z"
    except OSError:
        return False


if __name__ == "__main__":
    unittest.main()


class TestLateCancelBeatsDone(_Case):
    """Регрессия кросс-ревью круга 2 (усл. 11): отмена принята между последней сверкой клиента
    и его «готово». Итог — «отменено», а не «готово» с версией. Старое поведение: finish(done)
    перезаписывал принятую отмену — op закрывалась done с result_job_id."""

    def test_finish_done_after_cancel_is_canceled(self):
        a = self._holder()
        self.assertEqual(self.q.cancel(a), {"canceled": True, "running": True})
        d = self.q.finish(a, "done", result_job_id=42)
        self.assertEqual(d["state"], "canceled")
        self.assertEqual(d["result_job_id"], 0)
        self.assertEqual(_state(self.q, a), "canceled")
        self.assertEqual(_item(self.q, a)["result_job_id"], 0)


class TestClosedAndEvictedBeforeProcRegistered(unittest.TestCase):
    """Регрессия кросс-ревью круга 2 (усл. 12/13): операцию закрыли в окне между запуском процесса
    и его привязкой к op, и она уже вытеснена из истории последних (keep_done). Процесс должен быть
    убит, run_proc — поднять OpCanceled. Старое поведение: закрытую op искали в записях очереди;
    вытесненная не находилась — процесс считался «своим живым», жил до конца (30 с)."""

    def test_closed_and_evicted_in_window(self):
        from unittest import mock
        keep = 1
        clock = FakeClock()
        q = OpQueue(clock=clock, ttl=10, keep_done=keep)
        a = _id(q.add("preset", "holder", client=True))
        self.assertEqual(_state(q, a), "running")
        td = tempfile.TemporaryDirectory()
        self.addCleanup(td.cleanup)
        pidfile = Path(td.name) / "pid"
        code = ("import os, sys, time; open(sys.argv[1], 'w').write(str(os.getpid())); "
                "time.sleep(30)")
        real_popen = opqueue.subprocess.Popen
        box = {}

        def popen(*args, **kw):
            p = real_popen(*args, **kw)
            box["pid"] = p.pid
            q.finish(a, "canceled")
            for i in range(keep + 1):  # вытеснить закрытую op из истории последних
                o = _id(q.add("preset", f"other{i}", client=True))
                q.finish(o, "canceled")
            box["evicted"] = _item(q, a) is None
            return p

        seen = {}

        def fn():
            with q.use_op(a):
                with q.slot("stems", "roformer", job_id=4):
                    seen["ret"] = q.run_proc([sys.executable, "-c", code, str(pidfile)])
        with mock.patch.object(opqueue.subprocess, "Popen", popen):
            t0 = time.monotonic()
            t = _start(fn)
            t.join(JOIN)
        self.assertFalse(t.is_alive(), "процесс из окна запуска не убит за 5 с")
        self.assertLess(time.monotonic() - t0, JOIN)
        self.assertIn("pid", box, "run_proc не запускал процесс через subprocess.Popen")
        self.assertIs(box.get("evicted"), True, "сценарий: закрытая op не вытеснена из истории")
        self.assertNotIn("ret", seen, "run_proc вернул результат вместо OpCanceled")
        self.assertIsInstance(t.exc, OpCanceled)
        self.assertTrue(_wait_until(lambda: not _alive(box["pid"]), 2), "процесс жив")


class TestRestartIds(unittest.TestCase):
    """ТК11 / условие 9: номер операции клиента прошлого запуска воркера не достаётся записи
    нового запуска. Перезапуск — новая очередь с first_id позже прошлой (воркер берёт время
    запуска в мс)."""

    def test_first_id_starts_numbering(self):
        q = OpQueue(clock=FakeClock(), first_id=5000)
        ids = [_id(q.add("preset", f"op{i}", client=True)) for i in range(3)]
        self.assertTrue(all(i >= 5000 for i in ids), ids)
        self.assertEqual(len(set(ids)), 3)

    def test_restart_old_client_id_unknown_to_new_queue(self):
        old = OpQueue(clock=FakeClock(), first_id=1_700_000_000_000)
        old_ids = [_id(old.add("preset", f"old{i}", client=True)) for i in range(5)]
        new = OpQueue(clock=FakeClock(), first_id=1_700_000_060_000)  # запуск минутой позже
        new_ids = [_id(new.add("generate", f"gen{i}")) for i in range(5)]
        self.assertFalse(set(old_ids) & set(new_ids), (old_ids, new_ids))
        for op_id in old_ids:
            with self.assertRaises(KeyError):
                new.beat(op_id)

    def test_default_queue_still_numbers(self):
        q = OpQueue(clock=FakeClock())
        a, b = _id(q.add("preset", "a", client=True)), _id(q.add("preset", "b", client=True))
        self.assertNotEqual(a, b)
