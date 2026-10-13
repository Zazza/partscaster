"""Спецификация proclock.locked(path): исключительная межпроцессная
блокировка файла (без fcntl на Windows). Пока один процесс держит
блокировку, второй ждёт; файл и каталоги создаются при необходимости.
"""
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

import proclock

_HERE = Path(__file__).resolve().parent

# Потомок: берёт блокировку, пишет метку «взял», держит HOLD секунд,
# пишет время отпускания и выходит из блокировки.
_CHILD = """
import sys, time
import proclock
lock, marks, hold = sys.argv[1], sys.argv[2], float(sys.argv[3])
with proclock.locked(lock):
    with open(marks, "a") as f:
        f.write("acquired %r\\n" % time.time())
    time.sleep(hold)
    with open(marks, "a") as f:
        f.write("released %r\\n" % time.time())
"""


def _read_marks(path):
    out = {}
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                key, val = line.split()
                out[key] = float(val)
    return out


class TestLockedAcrossProcesses(unittest.TestCase):
    HOLD = 1.0

    def test_second_process_waits(self):
        with tempfile.TemporaryDirectory() as d:
            lock = os.path.join(d, "kits.lock")
            marks = os.path.join(d, "marks.txt")
            child = subprocess.Popen(
                [sys.executable, "-c", _CHILD, lock, marks, str(self.HOLD)],
                cwd=str(_HERE),
            )
            try:
                deadline = time.time() + 20
                while "acquired" not in _read_marks(marks):
                    self.assertIsNone(child.poll(), "потомок вышел, не взяв блокировку")
                    self.assertLess(time.time(), deadline, "потомок не взял блокировку за 20 с")
                    time.sleep(0.01)
                with proclock.locked(lock):
                    entered = time.time()
                    released = _read_marks(marks).get("released")
                self.assertIsNotNone(released, "родитель вошёл, пока потомок держал блокировку")
                self.assertGreaterEqual(entered, released)
            finally:
                child.wait(timeout=20)
            self.assertEqual(child.returncode, 0)


class TestLockedSameProcess(unittest.TestCase):
    def test_reenter_after_exit(self):
        with tempfile.TemporaryDirectory() as d:
            lock = os.path.join(d, "dl.lock")
            with proclock.locked(lock):
                pass
            # Повторный вход после выхода не зависает и не падает.
            with proclock.locked(lock):
                pass
            self.assertTrue(os.path.exists(lock))

    def test_creates_missing_dirs(self):
        with tempfile.TemporaryDirectory() as d:
            lock = os.path.join(d, "a", "b", "x.lock")
            with proclock.locked(lock):
                self.assertTrue(os.path.exists(lock))

    def test_released_on_exception(self):
        # Исключение внутри блока отпускает блокировку: другой процесс входит сразу.
        with tempfile.TemporaryDirectory() as d:
            lock = os.path.join(d, "e.lock")
            marks = os.path.join(d, "marks.txt")
            with self.assertRaises(RuntimeError):
                with proclock.locked(lock):
                    raise RuntimeError("сбой внутри")
            r = subprocess.run(
                [sys.executable, "-c", _CHILD, lock, marks, "0"],
                cwd=str(_HERE), timeout=20,
            )
            self.assertEqual(r.returncode, 0)
            self.assertIn("released", _read_marks(marks))


if __name__ == "__main__":
    unittest.main()
