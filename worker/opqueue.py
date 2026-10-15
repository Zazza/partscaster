"""Общая очередь долгих операций воркера: генерации, разделение на дорожки,
эффекты с усилителем, распознавание, цепочки приложения и ИИ (MCP).

Видеокарта одна — работает одна операция («слот»), остальные ждут по месту в
очереди (меньше — раньше), а не «кто успел» лока. Место ждущей можно менять,
очередь — ставить на паузу, любую незавершённую операцию — отменять: ждущая
снимается сразу, идущей ставится метка отмены, её внешние процессы убиваются.

Операция клиента (цепочка приложения или MCP: пресет, пересборка) живёт на
отметках «жива» (beat): нет отметки дольше ttl — «прервано», слот следующей.
Вызовы воркера внутри такой операции несут её id (contextvar current_op) и не
встают в очередь повторно; так же реентерабельны вложенные тяжёлые секции.

Модуль чистый (без FastAPI/torch); часы внедряются для тестов."""
from __future__ import annotations

import contextlib
import contextvars
import os
import signal
import subprocess
import threading
import time
from collections.abc import Callable, Iterator

OP_TTL = 180.0  # с без отметки «жива» — операция клиента прервана (процесс клиента упал)
FINAL = ("done", "error", "canceled", "interrupted")
CANCELED_TEXT = "отменено человеком"

current_op: contextvars.ContextVar[int] = contextvars.ContextVar("current_op", default=0)


class OpCanceled(BaseException):  # noqa: N818 - имя из контракта
    """Операцию отменили. Наследник BaseException: в воркере много широких
    `except Exception` (RoFormer → запасной Demucs, whisper → текст ошибки) —
    отмена не должна превращаться в «попробовать иначе»."""

    def __init__(self, msg: str = CANCELED_TEXT):
        super().__init__(msg)


class OpGone(Exception):  # noqa: N818 - имя из контракта
    """Операции нет: неизвестна, прервана или уже завершена."""


def _now_str() -> str:
    return time.strftime("%Y-%m-%dT%H:%M:%S")


class Op:
    def __init__(self, op_id: int, kind: str, title: str, job_id: int, source: str, client: bool,
                 pos: int, now: float):
        self.id = op_id
        self.kind = kind
        self.title = title
        self.job_id = int(job_id or 0)
        self.source = source or "other"
        self.client = client
        self.pos = pos
        self.state = "waiting"
        self.result_job_id = 0
        self.error = ""
        self.cancel_requested = False
        self.created_at = _now_str()
        self.started_at = ""
        self.finished_at = ""
        self.last_beat = now
        self.taken = False  # генерация: queue_loop уже забрал выданный слот
        self.active = 0     # сколько тяжёлых секций сейчас идут внутри операции (шаги цепочки клиента)
        self.hooks: list[Callable[[], None]] = []
        self.procs: set[subprocess.Popen] = set()

    def as_dict(self) -> dict:
        return {"id": self.id, "kind": self.kind, "title": self.title, "job_id": self.job_id,
                "source": self.source, "state": self.state, "pos": self.pos,
                "result_job_id": self.result_job_id, "error": self.error,
                "cancel_requested": self.cancel_requested, "client": self.client,
                "created_at": self.created_at, "started_at": self.started_at,
                "finished_at": self.finished_at}


def _kill(p: subprocess.Popen) -> None:
    """Процесс и его потомков (sep_run, Seed-VC запускают свои): своя группа — на POSIX."""
    try:
        if hasattr(os, "killpg"):
            os.killpg(p.pid, signal.SIGKILL)
        else:
            p.kill()
    except (ProcessLookupError, PermissionError, OSError):
        pass  # уже завершился — убивать нечего


class OpQueue:
    def __init__(self, clock: Callable[[], float] = time.monotonic, ttl: float = OP_TTL, keep_done: int = 20,
                 on_pos: Callable[[Op], None] | None = None, first_id: int = 0):
        self._clock = clock
        self._ttl = ttl
        self._keep = keep_done
        self._on_pos = on_pos
        self._cv = threading.Condition(threading.RLock())
        self._ops: dict[int, Op] = {}   # незавершённые
        self._done: list[Op] = []       # завершённые, новые первыми
        # первый id − 1: воркер начинает со времени запуска — номер прошлого запуска не достаётся
        # чужой записи (клиент со старым номером получит 404 → «прервано», а не чужое состояние)
        self._seq = max(0, int(first_id) - 1)
        self._pos = 0
        self._running = 0
        self.paused = False

    # --- внутреннее (под self._cv) ---

    def _find(self, op_id: int) -> Op | None:
        op = self._ops.get(op_id)
        if op is not None:
            return op
        return next((o for o in self._done if o.id == op_id), None)

    def _waiting(self) -> list[Op]:
        return sorted((o for o in self._ops.values() if o.state == "waiting"), key=lambda o: (o.pos, o.id))

    def _dispatch(self) -> None:
        if self._running or self.paused:
            return
        w = self._waiting()
        if not w:
            return
        op = w[0]
        op.state = "running"
        op.started_at = _now_str()
        self._running = op.id  # last_beat не трогаем: мёртвый клиент в голове очереди не держит слот ещё ttl
        self._cv.notify_all()

    def _finalize(self, op: Op, state: str, result_job_id: int = 0, error: str = "",
                  dispatch: bool = True) -> None:
        op.state = state
        op.result_job_id = int(result_job_id or 0)
        op.error = error or ""
        op.finished_at = _now_str()
        # процессы у закрываемой операции бывают, только если её шаг ещё идёт, а клиент уже закрыл операцию
        # (отмена, порог «жива», приложение закрылось посреди шага) — досчитывать некому, на GPU не оставляем
        for p in op.procs:
            _kill(p)
        op.procs.clear()
        self._ops.pop(op.id, None)
        self._done.insert(0, op)
        del self._done[self._keep:]
        # шаг внутри процесса воркера ещё считает (клиент закрыл операцию после отмены, порог «жива») —
        # видеокарта занята, слот отдаётся, когда шаг выйдет из секции (_leave)
        if self._running == op.id and op.active == 0:
            self._running = 0
        if dispatch:
            self._dispatch()
        self._cv.notify_all()

    def _sweep(self) -> None:
        # сначала все просроченные, потом выдача слота: иначе ждущая просроченная
        # получила бы слот (и свежую отметку) раньше, чем до неё дошла проверка
        now = self._clock()
        stale = [op for op in self._ops.values() if op.client and now - op.last_beat > self._ttl]
        for op in stale:
            self._finalize(op, "interrupted", error="клиент перестал отмечаться", dispatch=False)
        if stale:
            self._dispatch()

    # --- публичное ---

    def add(self, kind: str, title: str, job_id: int = 0, source: str = "other", client: bool = False,
            pos: int | None = None) -> Op:
        with self._cv:
            self._seq += 1
            if pos is None:
                self._pos += 1
                pos = self._pos
            else:
                self._pos = max(self._pos, int(pos))
            op = Op(self._seq, kind, title, job_id, source, client, int(pos), self._clock())
            self._ops[op.id] = op
            self._dispatch()
            self._cv.notify_all()
            return op

    def acquire(self, op_id: int, timeout: float | None = None) -> Op:
        """Ждать слота для op; отменена в ожидании — OpCanceled, пропала — OpGone."""
        deadline = None if timeout is None else time.monotonic() + timeout
        with self._cv:
            while True:
                op = self._find(op_id)
                if op is None:
                    raise OpGone(f"операции {op_id} нет")
                if op.state == "running":
                    return op
                if op.state == "canceled":
                    raise OpCanceled()
                if op.state != "waiting":
                    raise OpGone(f"операция {op_id}: {op.state}")
                left = None if deadline is None else deadline - time.monotonic()
                if left is not None and left <= 0:
                    raise TimeoutError(f"операция {op_id} не дождалась очереди")
                self._cv.wait(left if left is not None else 1.0)

    def release(self, op_id: int, state: str = "done", result_job_id: int = 0, error: str = "") -> None:
        with self._cv:
            op = self._ops.get(op_id)
            if op is not None:
                self._finalize(op, state, result_job_id, error)

    @contextlib.contextmanager
    def use_op(self, op_id: int) -> Iterator[None]:
        token = current_op.set(int(op_id or 0))
        try:
            yield
        finally:
            current_op.reset(token)

    @contextlib.contextmanager
    def slot(self, kind: str, title: str, job_id: int = 0, source: str = "other") -> Iterator[Op]:
        """Тяжёлая секция. Внутри своей идущей операции — без очереди (проверка
        отмены на входе и выходе); иначе — новая операция: ждать, работать, закрыть."""
        cur = current_op.get()
        if cur:
            with self._cv:
                op = self._find(cur)
                state = op.state if op is not None else ""
            if op is None or state in FINAL:
                if state == "canceled":
                    raise OpCanceled()
                raise OpGone(f"операция {cur}: {state or 'нет'}")
            if state == "waiting":
                op = self.acquire(cur)
            with self._cv:
                if op.cancel_requested:
                    raise OpCanceled()
                if op.state != "running":
                    raise OpGone(f"операция {cur}: {op.state}")
                op.active += 1
            try:
                yield op
            finally:
                self._leave(op)
            if op.cancel_requested:
                raise OpCanceled()  # шаг без точки прерывания доработал — итог всё равно «отменено»
            return
        op = self.add(kind, title, job_id, source)
        token = current_op.set(op.id)
        try:
            try:
                self.acquire(op.id)
            except OpCanceled:
                raise
            try:
                yield op
            except OpCanceled:
                self.release(op.id, "canceled", error=CANCELED_TEXT)
                raise
            except BaseException as e:
                if op.cancel_requested:
                    self.release(op.id, "canceled", error=CANCELED_TEXT)
                    raise OpCanceled() from e
                self.release(op.id, "error", error=str(e)[:500])
                raise
            if op.cancel_requested:
                self.release(op.id, "canceled", error=CANCELED_TEXT)
                raise OpCanceled()
            self.release(op.id, "done")
        finally:
            current_op.reset(token)

    def _leave(self, op: Op) -> None:
        """Шаг вышел из секции; операция уже закрыта и шагов больше нет — слот следующей."""
        with self._cv:
            op.active -= 1
            if op.active == 0 and op.state in FINAL and self._running == op.id:
                self._running = 0
                self._dispatch()
                self._cv.notify_all()

    def beat(self, op_id: int) -> dict:
        with self._cv:
            self._sweep()
            op = self._find(op_id)
            if op is None:
                raise KeyError(op_id)
            if op.state not in FINAL:
                op.last_beat = self._clock()
            return op.as_dict()

    def finish(self, op_id: int, state: str, result_job_id: int = 0, error: str = "") -> dict:
        if state not in ("done", "error", "canceled"):
            raise ValueError(f"итог {state!r}: done | error | canceled")
        with self._cv:
            op = self._find(op_id)
            if op is None:
                raise KeyError(op_id)
            if not op.client or op.state in FINAL:
                raise ValueError(f"операция {op_id} ({op.state}) не закрывается клиентом")
            if state == "done" and op.cancel_requested:
                # отмену приняли раньше, чем пришло «готово»: итог — отменено (клиент откатит сделанное)
                state, result_job_id, error = "canceled", 0, CANCELED_TEXT
            self._finalize(op, state, result_job_id, error)
            return op.as_dict()

    def move(self, op_id: int, direction: int) -> bool:
        if direction not in (-1, 1):
            raise ValueError("dir: -1 | 1")
        with self._cv:
            op = self._find(op_id)
            if op is None:
                raise KeyError(op_id)
            if op.state != "waiting":
                raise ValueError(f"операция {op_id} не ждёт ({op.state})")
            w = self._waiting()
            i = w.index(op)
            j = i + direction
            if j < 0 or j >= len(w):
                return False
            other = w[j]
            op.pos, other.pos = other.pos, op.pos
            if op.pos == other.pos:  # равные места (старт воркера) — развести
                op.pos += direction
            changed = [op, other]
            self._dispatch()
        if self._on_pos:
            for o in changed:
                self._on_pos(o)
        return True

    def cancel(self, op_id: int) -> dict:
        with self._cv:
            op = self._find(op_id)
            if op is None:
                raise KeyError(op_id)
            if op.state in FINAL:
                return {"canceled": False, "running": False}
            if op.state == "waiting":
                self._finalize(op, "canceled", error=CANCELED_TEXT)
                return {"canceled": True, "running": False}
            op.cancel_requested = True
            hooks = list(op.hooks)
            procs = list(op.procs)
            self._cv.notify_all()
        # вне лока: хук генерации пишет свой флаг, убийство процесса — системный вызов
        for fn in hooks:
            fn()
        for p in procs:
            _kill(p)
        return {"canceled": True, "running": True}

    def on_cancel(self, op_id: int, fn: Callable[[], None]) -> None:
        with self._cv:
            op = self._find(op_id)
            if op is None:
                return
            op.hooks.append(fn)
            now = op.cancel_requested
        if now:
            fn()

    def set_paused(self, paused: bool) -> None:
        with self._cv:
            self.paused = bool(paused)
            self._dispatch()

    def sweep(self) -> None:
        with self._cv:
            self._sweep()

    def info(self, op_id: int) -> dict | None:
        with self._cv:
            op = self._find(op_id)
            return op.as_dict() if op is not None else None

    def waiting_count(self) -> int:
        with self._cv:
            return len(self._waiting())

    def find_job(self, job_id: int, kind: str = "generate") -> Op | None:
        """Незавершённая операция трека данного вида (генерация по id джобы)."""
        with self._cv:
            return next((o for o in self._ops.values() if o.kind == kind and o.job_id == job_id), None)

    def snapshot(self) -> dict:
        with self._cv:
            self._sweep()
            run = [o for o in self._ops.values() if o.state == "running"]
            items = run + self._waiting() + list(self._done)
            return {"paused": self.paused, "items": [o.as_dict() for o in items]}

    def next_generation(self, timeout: float) -> Op | None:
        deadline = time.monotonic() + timeout
        with self._cv:
            while True:
                op = self._ops.get(self._running)
                if op is not None and op.kind == "generate" and not op.taken:
                    op.taken = True
                    return op
                left = deadline - time.monotonic()
                if left <= 0:
                    return None
                self._cv.wait(left)

    def run_proc(self, cmd, timeout: float | None = None, **popen_kw) -> subprocess.CompletedProcess:
        """subprocess.run(capture_output=True, text=True), привязанный к текущей
        операции: её отмена убивает процесс, вызов поднимает OpCanceled."""
        op_id = current_op.get()
        kw = {"stdout": subprocess.PIPE, "stderr": subprocess.PIPE, "text": True}
        if hasattr(os, "killpg"):
            kw["start_new_session"] = True
        kw.update(popen_kw)
        p = subprocess.Popen(cmd, **kw)
        with self._cv:
            op = self._find(op_id) if op_id else None
            if op is not None and op.state not in FINAL:
                op.procs.add(p)
            # операцию закрыли, пока процесс запускался (отмена, порог «жива»; закрытая могла уже выпасть
            # из истории последних) — процесс не должен работать
            gone = bool(op_id) and op is None
            canceled = gone or (op is not None and (op.cancel_requested or op.state in FINAL))
        if canceled:
            _kill(p)
        try:
            try:
                out, err = p.communicate(timeout=timeout)
            except subprocess.TimeoutExpired:
                _kill(p)
                p.communicate()
                raise
        finally:
            with self._cv:
                if op is not None:
                    op.procs.discard(p)
        if gone or (op is not None and (op.cancel_requested or op.state in FINAL)):
            # процесс убит отменой или закрытием операции — не «упал» (иначе RoFormer ушёл бы на запасной Demucs)
            raise OpCanceled()
        return subprocess.CompletedProcess(cmd, p.returncode, out, err)
