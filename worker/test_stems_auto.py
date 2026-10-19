"""Авто-разделение на дорожки и «убрать дорожку» вместо минуса.

Спецификация yue_worker:
- GET /config отдаёт stems_auto, по умолчанию 'studio'; POST /config
  {stems_auto: 'studio'|'always'|'manual'} сохраняет и возвращает новое
  значение; иное значение → 422, настройка не меняется;
- _auto_stems_later(job_id): при stems_auto='always' у готового полного трека
  (draft=0, role '' / 'variant' / 'rebuild' / 'continue', есть audio_file, нет
  stem-vocals.flac) запускает _split_job(job_id, row) в отдельном потоке
  (имя начинается с 'auto-stems-'); не запускает при 'studio'/'manual', у
  черновика, у ролей fragment/section/voice, без audio_file и если
  stem-vocals.flac уже есть; сбой разделения в потоке не пробрасывается,
  а логируется;
- _split_job(job_id, row) — без выбора модели (модель дорожек одна, RoFormer): два
  одновременных вызова для одного трека — _split_job_now выполняется один раз, второй
  получает тот же результат (или то же исключение); после завершения следующий вызов
  делит заново;
- вызов изнутри операции очереди (opqueue.current_op != 0 в вызывающем потоке) не ждёт
  идущее разделение того же трека — делит сам и возвращает свой итог;
- после таких вызовов реестр чист: следующий вызов делит заново, не зависает;
- POST /jobs/{id}/stems: _split_job бросил opqueue.OpCanceled → 409, detail содержит
  opqueue.CANCELED_TEXT (и у старого клиента с ?model= — параметр игнорируется); opqueue.OpGone → 409;
- POST /jobs/{id}/minus больше нет (404/405).

Внешняя граница — само разделение (_split_job / _split_job_now: RoFormer на GPU);
оно подменяется моком. Воркер импортируется целиком — без fastapi/httpx/numpy
классы пропускаются, как соседние в test_pure.py.
"""
import json
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

import test_pure as tp

_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"
_JOIN_SEC = 10  # предел ожидания потоков: тест не должен виснуть


class _StemsAutoCase(tp._WorkerApiCase):
    """Свои настройки воркера на тест (как TestAutoStemsInstrumentalConfig)."""

    def setUp(self):
        super().setUp()
        data = Path(self._td.name) / "cfg"
        data.mkdir()
        self.settings = data / "settings.json"
        for name, val in (("DATA_DIR", data), ("SETTINGS_PATH", self.settings)):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)

    def _set_mode(self, mode):
        r = self.client.post("/config", json={"stems_auto": mode})
        self.assertLess(r.status_code, 300, r.text)

    def _track(self, role="", draft=0, audio=True, vocals_stem=False):
        """Готовый трек в БД: роль, черновик, файл аудио, готовая дорожка голоса."""
        jid = self._job(role=role, semantic=False)
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        if audio:
            (d / "audio.flac").write_bytes(b"fake")
        if vocals_stem:
            (d / "stem-vocals.flac").write_bytes(b"fake")
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file=?, draft=? WHERE id=?",
                      ("audio.flac" if audio else "", draft, jid))
        return jid


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestStemsAutoConfig(_StemsAutoCase):
    """GET/POST /config: stems_auto."""

    def _get(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def test_default_studio(self):
        self.assertFalse(self.settings.exists())
        self.assertEqual(self._get()["stems_auto"], "studio")

    def test_post_each_mode_saved_and_returned(self):
        for mode in ("always", "manual", "studio"):
            with self.subTest(mode=mode):
                r = self.client.post("/config", json={"stems_auto": mode})
                self.assertLess(r.status_code, 300, r.text)
                self.assertEqual(r.json().get("stems_auto"), mode)
                self.assertEqual(self._get()["stems_auto"], mode)
                self.assertEqual(json.loads(self.settings.read_text())["stems_auto"], mode)

    def test_unknown_mode_422_unchanged(self):
        self._set_mode("manual")
        for bad in ("zzz", "", "ALWAYS", 1):
            with self.subTest(bad=bad):
                r = self.client.post("/config", json={"stems_auto": bad})
                self.assertEqual(r.status_code, 422, r.text)
                self.assertEqual(self._get()["stems_auto"], "manual")


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestAutoStemsLater(_StemsAutoCase):
    """_auto_stems_later: когда разделение запускается само."""

    def setUp(self):
        super().setUp()
        self.calls = []  # (job_id, имя потока)

        def fake_split(job_id, row, *a, **kw):
            self.calls.append((job_id, threading.current_thread().name, row))
            return {}

        self.split = mock.patch.object(self.w, "_split_job", side_effect=fake_split)
        self.split_mock = self.split.start()
        self.addCleanup(self.split.stop)

    def _run(self, jid):
        self.w._auto_stems_later(jid)
        self._join_auto()

    def _join_auto(self):
        for t in threading.enumerate():
            if t.name.startswith("auto-stems-") and t is not threading.current_thread():
                t.join(_JOIN_SEC)
                self.assertFalse(t.is_alive(), f"поток {t.name} не завершился")

    def test_always_full_track_splits_in_thread(self):
        self._set_mode("always")
        for role in ("", "variant", "rebuild", "continue"):
            with self.subTest(role=role):
                self.calls.clear()
                jid = self._track(role=role)
                self._run(jid)
                self.assertEqual(len(self.calls), 1, self.calls)
                job_id, thread_name, row = self.calls[0]
                self.assertEqual(int(job_id), jid)
                self.assertTrue(thread_name.startswith("auto-stems-"), thread_name)
                self.assertNotEqual(thread_name, threading.current_thread().name)
                self.assertEqual(int(row["id"]), jid)

    def test_studio_and_manual_no_split(self):
        for mode in ("studio", "manual"):
            with self.subTest(mode=mode):
                self.calls.clear()
                self._set_mode(mode)
                self._run(self._track())
                self.assertEqual(self.calls, [])

    def test_default_mode_no_split(self):
        # настройку не трогали — по умолчанию studio, сам воркер не делит
        self._run(self._track())
        self.assertEqual(self.calls, [])

    def test_draft_no_split(self):
        self._set_mode("always")
        self._run(self._track(draft=1))
        self.assertEqual(self.calls, [])

    def test_partial_roles_no_split(self):
        self._set_mode("always")
        for role in ("fragment", "section", "voice"):
            with self.subTest(role=role):
                self.calls.clear()
                self._run(self._track(role=role))
                self.assertEqual(self.calls, [])

    def test_without_audio_no_split(self):
        self._set_mode("always")
        self._run(self._track(audio=False))
        self.assertEqual(self.calls, [])

    def test_stems_exist_no_split(self):
        self._set_mode("always")
        self._run(self._track(vocals_stem=True))
        self.assertEqual(self.calls, [])

    def test_missing_job_no_split_no_raise(self):
        self._set_mode("always")
        self._run(9999)
        self.assertEqual(self.calls, [])

    def test_split_failure_logged_not_raised(self):
        self._set_mode("always")
        self.split_mock.side_effect = RuntimeError("сбой разделения")
        jid = self._track()
        thread_errors = []
        with mock.patch.object(threading, "excepthook", side_effect=thread_errors.append):
            with self.assertLogs("partscaster-worker", level="WARNING"):
                self.w._auto_stems_later(jid)  # сам вызов не бросает
                self._join_auto()
        self.assertEqual(self.split_mock.call_count, 1)
        self.assertEqual(thread_errors, [], "исключение вылетело из потока")


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestSplitJobOnce(_StemsAutoCase):
    """_split_job: одновременные вызовы для одного трека делят один раз."""

    def setUp(self):
        super().setUp()
        self.jid = self._track()
        self.row = self._row(self.jid)
        self.release = threading.Event()
        self.entered = threading.Event()
        self.n = 0

    def _now(self, result=None, err=None):
        def fake(*a, **kw):
            self.n += 1
            self.entered.set()
            self.release.wait(_JOIN_SEC)
            if err is not None:
                raise err
            return result
        return fake

    def _two_calls(self):
        """Первый вызов висит в разделении, второй приходит во время него."""
        out = {}

        def call(key):
            try:
                out[key] = ("ok", self.w._split_job(self.jid, self.row))
            except Exception as e:  # исключение — результат вызова, его и сверяем
                out[key] = ("err", e)

        t1 = threading.Thread(target=call, args=("a",))
        t1.start()
        self.assertTrue(self.entered.wait(_JOIN_SEC), "разделение не началось")
        t2 = threading.Thread(target=call, args=("b",))
        t2.start()
        time.sleep(0.3)  # второй успел дойти до ожидания
        self.assertNotIn("b", out, "второй вызов не дождался первого")
        self.release.set()
        for t in (t1, t2):
            t.join(_JOIN_SEC)
            self.assertFalse(t.is_alive())
        return out

    def test_concurrent_one_split_same_result(self):
        result = {"stems": ["vocals", "drums", "bass", "other"], "model": "bs-roformer-sw"}
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now(result)):
            out = self._two_calls()
        self.assertEqual(self.n, 1)
        self.assertEqual(out["a"], ("ok", result))
        self.assertEqual(out["b"], ("ok", result))

    def test_concurrent_failure_both_get_error(self):
        err = RuntimeError("модель упала")
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now(err=err)):
            out = self._two_calls()
        self.assertEqual(self.n, 1)
        for key in ("a", "b"):
            kind, e = out[key]
            self.assertEqual(kind, "err", key)
            self.assertIsInstance(e, RuntimeError)
            self.assertIn("модель упала", str(e))

    def test_after_finish_next_call_splits_again(self):
        self.release.set()
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now({"model": "bs-roformer-sw"})):
            self.w._split_job(self.jid, self.row)
            self.w._split_job(self.jid, self.row)
        self.assertEqual(self.n, 2)

    def test_after_failure_next_call_splits_again(self):
        self.release.set()
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now(err=RuntimeError("x"))):
            with self.assertRaises(RuntimeError):
                self.w._split_job(self.jid, self.row)
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now({"model": "bs-roformer-sw"})):
            self.assertEqual(self.w._split_job(self.jid, self.row), {"model": "bs-roformer-sw"})
        self.assertEqual(self.n, 2)


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestMinusRemoved(_StemsAutoCase):
    """POST /jobs/{id}/minus убран."""

    def test_minus_404_or_405(self):
        jid = self._track()
        r = self.client.post(f"/jobs/{jid}/minus")
        self.assertIn(r.status_code, (404, 405), r.text)


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestSplitJobInsideQueueOp(_StemsAutoCase):
    """_split_job: вызов из операции очереди не ждёт идущее разделение того же трека."""

    def setUp(self):
        super().setUp()
        self.jid = self._track()
        self.row = self._row(self.jid)
        self.release = threading.Event()
        self.entered = threading.Event()
        self.n = 0

    def _now(self):
        """Первое разделение висит до release, следующие отвечают сразу; итог — номер вызова."""
        def fake(job_id, row):
            self.n += 1
            call_no = self.n   # номер этого вызова: общий счётчик к возврату первого уже ушёл дальше
            if call_no == 1:
                self.entered.set()
                self.release.wait(_JOIN_SEC)
            return {"model": "bs-roformer-sw", "call": call_no}
        return fake

    def _first_then_second(self, second_op):
        """Первый вызов висит в разделении; второй (из операции second_op) должен вернуться, не дожидаясь."""
        out = {}

        def call(key, op_id):
            tok = self.w.opqueue.current_op.set(op_id) if op_id else None
            try:
                out[key] = ("ok", self.w._split_job(self.jid, self.row))
            except Exception as e:  # исключение — результат вызова, его и сверяем
                out[key] = ("err", e)
            finally:
                if tok is not None:
                    self.w.opqueue.current_op.reset(tok)

        t1 = threading.Thread(target=call, args=("a", 0))
        t2 = None
        t1.start()
        try:
            self.assertTrue(self.entered.wait(_JOIN_SEC), "разделение не началось")
            t2 = threading.Thread(target=call, args=("b", second_op))
            t2.start()
            t2.join(5)
            self.assertFalse(t2.is_alive(), "второй вызов ждёт первый")
            self.assertIn("b", out)
            self.assertNotIn("a", out, "первый вызов не должен был закончиться")
        finally:
            self.release.set()
            for t in (t1, t2):
                if t is not None:
                    t.join(_JOIN_SEC)
        self.assertFalse(t1.is_alive())
        self.assertFalse(t2.is_alive())
        return out

    def _then_splits_again(self):
        """Реестр чист: следующий вызов делит заново и не виснет."""
        before = self.n
        done = {}
        t = threading.Thread(target=lambda: done.setdefault("r", self.w._split_job(self.jid, self.row)))
        t.start()
        t.join(5)
        self.assertFalse(t.is_alive(), "следующий вызов завис")
        self.assertEqual(self.n, before + 1)
        self.assertEqual(done["r"]["call"], before + 1)

    def test_inside_queue_op_does_not_wait(self):
        with mock.patch.object(self.w, "_split_job_now", side_effect=self._now()):
            out = self._first_then_second(second_op=7)
            self.assertEqual(self.n, 2)
            kind, res = out["b"]
            self.assertEqual(kind, "ok")
            self.assertEqual(res["call"], 2, "вызов из операции очереди получил чужой итог")
            self.assertEqual(out["a"][1]["call"], 1)
            self._then_splits_again()


@unittest.skipUnless(tp._HAS_WORKER_DEPS, _SKIP)
class TestStemsEndpointQueueErrors(_StemsAutoCase):
    """POST /jobs/{id}/stems: отмена/пропажа операции очереди → 409."""

    def test_canceled_409_with_text(self):
        jid = self._track()
        with mock.patch.object(self.w, "_split_job", side_effect=self.w.opqueue.OpCanceled()):
            r = self.client.post(f"/jobs/{jid}/stems")
        self.assertEqual(r.status_code, 409, r.text)
        self.assertIn(self.w.opqueue.CANCELED_TEXT, str(r.json().get("detail")))

    def test_canceled_old_client_model_param_409(self):
        # старый клиент шлёт ?model= — параметр игнорируется, отмена — та же 409
        jid = self._track()
        with mock.patch.object(self.w, "_split_job", side_effect=self.w.opqueue.OpCanceled()):
            r = self.client.post(f"/jobs/{jid}/stems?model=roformer")
        self.assertEqual(r.status_code, 409, r.text)
        self.assertIn(self.w.opqueue.CANCELED_TEXT, str(r.json().get("detail")))

    def test_gone_409(self):
        jid = self._track()
        with mock.patch.object(self.w, "_split_job", side_effect=self.w.opqueue.OpGone("x")):
            r = self.client.post(f"/jobs/{jid}/stems")
        self.assertEqual(r.status_code, 409, r.text)


if __name__ == "__main__":
    unittest.main()
