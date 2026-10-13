"""Межпроцессная блокировка файла — на Linux и на Windows (там нет fcntl)."""
import contextlib
import os
import time
from pathlib import Path

if os.name == "nt":
    import msvcrt
else:
    import fcntl


@contextlib.contextmanager
def locked(path):
    """Исключительная блокировка файла path на время блока; другой процесс ждёт."""
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    with open(p, "a+b") as fh:
        if os.name == "nt":
            # LK_LOCK сдаётся через ~10 с ожидания — ждём сами, сколько понадобится
            while True:
                try:
                    fh.seek(0)
                    msvcrt.locking(fh.fileno(), msvcrt.LK_NBLCK, 1)
                    break
                except OSError:
                    time.sleep(0.1)
            try:
                yield
            finally:
                fh.seek(0)
                msvcrt.locking(fh.fileno(), msvcrt.LK_UNLCK, 1)
        else:
            fcntl.flock(fh, fcntl.LOCK_EX)
            try:
                yield
            finally:
                fcntl.flock(fh, fcntl.LOCK_UN)
