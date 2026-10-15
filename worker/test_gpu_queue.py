"""Тесты очереди к GPU (gpu_queue поверх общей очереди QUEUE в yue_worker).

Импорт yue_worker создаёт каталоги данных и БД — уводим их во временную
папку через PARTSCASTER_DATA_DIR до импорта. GPU в тестах не трогаем: проверяем
только саму очередь (взаимоисключение, реентерабельность, освобождение при ошибке).
"""
import os
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

_tmp = tempfile.TemporaryDirectory(prefix="yue-gpuq-test-")
os.environ["PARTSCASTER_DATA_DIR"] = str(Path(_tmp.name) / "data")

import opqueue  # noqa: E402
import yue_worker  # noqa: E402


def _running():
    return [it for it in yue_worker.QUEUE.snapshot()["items"] if it["state"] == "running"]


class TestGpuQueue(unittest.TestCase):
    def setUp(self):
        p = mock.patch.object(yue_worker, "QUEUE", opqueue.OpQueue())  # своя очередь на тест
        p.start()
        self.addCleanup(p.stop)

    def test_mutual_exclusion(self):
        """Два потока под gpu_queue не бывают внутри одновременно."""
        active = 0
        peak = 0
        bad = []

        def worker(n):
            nonlocal active, peak
            with yue_worker.gpu_queue(f"t{n}"):
                active += 1
                peak = max(peak, active)
                if active > 1:
                    bad.append(n)
                time.sleep(0.05)
                active -= 1

        ts = [threading.Thread(target=worker, args=(i,)) for i in range(4)]
        for t in ts:
            t.start()
        for t in ts:
            t.join()
        self.assertEqual(bad, [])
        self.assertEqual(peak, 1)

    def test_lock_released_on_error(self):
        """Исключение внутри секции не оставляет слот занятым; запись — «ошибка»."""
        with self.assertRaises(RuntimeError):
            with yue_worker.gpu_queue("boom"):
                raise RuntimeError("x")
        self.assertEqual(_running(), [], "слот не освобождён после исключения")
        self.assertEqual(yue_worker.QUEUE.snapshot()["items"][0]["state"], "error")

    def test_reentrant(self):
        """Вложенные gpu_queue в том же потоке не дедлочат и не создают вторую запись."""
        with yue_worker.gpu_queue("outer"):
            with yue_worker.gpu_queue("inner"):
                pass
        self.assertEqual(len(yue_worker.QUEUE.snapshot()["items"]), 1)

    def test_waiters_counter(self):
        """Число ждущих: растёт, пока поток ждёт слот, и возвращается к нулю."""
        held = threading.Event()
        let_go = threading.Event()
        waiter_started = threading.Event()
        seen = []

        def holder():
            with yue_worker.gpu_queue("holder"):
                held.set()
                let_go.wait(2)

        def waiter():
            held.wait(1)
            waiter_started.set()
            with yue_worker.gpu_queue("waiter"):
                pass

        t1, t2 = threading.Thread(target=holder), threading.Thread(target=waiter)
        t1.start()
        t2.start()
        waiter_started.wait(1)
        deadline = time.monotonic() + 2  # ждём условие, а не фиксированную паузу
        while yue_worker.QUEUE.waiting_count() < 1 and time.monotonic() < deadline:
            time.sleep(0.005)
        seen.append(yue_worker.QUEUE.waiting_count())
        let_go.set()
        t1.join(2)
        t2.join(2)
        seen.append(yue_worker.QUEUE.waiting_count())
        self.assertGreaterEqual(seen[0], 1, "ожидающий не посчитан")
        self.assertEqual(seen[1], 0, "счётчик не обнулился после очереди")


if __name__ == "__main__":
    unittest.main()
