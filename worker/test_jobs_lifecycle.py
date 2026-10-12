"""Спецификация жизненного цикла джобы: отмена, удаление, ветки генерации, очередь.

Что должно работать:
L1. POST /jobs/{id}/cancel: queued — снимается из очереди (статус canceled, очередь
    её не запускает); running — генерация останавливается на ближайшем шаге,
    итог canceled (не error); готовая/упавшая/несуществующая — «нечего отменять»,
    статус не меняется.
L2. DELETE /jobs/{id}: строка и папка ЭТОЙ джобы удаляются; чужие папки (в т.ч. с
    похожим номером), БД и всё вне jobs/<id> не трогаются, даже если папка —
    ссылка наружу или в ней ссылка наружу; нет джобы → 404; идущую (running) —
    нельзя (409), ничего не удаляется.
L3. Генерация (_run_job):
    - отмена во время генерации → canceled, модель остаётся загруженной;
    - нехватка видеопамяти (CUDA OOM) → error с понятной причиной, модель
      выгружается, следующий трек грузит модель заново и проходит;
    - иная ошибка → error, модель не выгружается;
    - старая сборка yue2 без cfg_scale → повтор без него, трек готов;
      иная TypeError → error без повтора;
    - трек «без голоса» → дорожки строятся сами (настройка
      auto_stems_instrumental, по умолчанию да); сбой дорожек трек не роняет;
      трек с голосом и выключенная настройка — без дорожек.
L4. Очередь (queue_loop): берёт queued по порядку номеров, только queued
    (canceled/done/error/running не трогает), каждую — один раз; если модель не
    загрузилась — джоба error, очередь живёт дальше.
L5. Перезапуск воркера (_startup): джобы, «повисшие» в running, → error.

Модель (yue2), torch и разделение на дорожки — внешние границы, подменяются.
"""
import os
import sys
import types
import unittest
from pathlib import Path

from test_character import _job_id, _post_job, _Sampling
from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

INSTRUMENTAL = "instrumental post-rock, clean guitar"
OOM_TEXT = "CUDA out of memory. Tried to allocate 2.00 GiB (GPU 0; 23.6 GiB total capacity)"


class _Stop(Exception):
    """Останов бесконечного цикла очереди в тесте: «заснуть» = очередь пуста."""


class _FakeSong:
    """Результат генерации: пишет короткий настоящий audio.flac."""

    truncated = False

    def __init__(self, audio=True):
        self.audio = audio

    def save_artifacts(self, job_dir):
        job_dir = Path(job_dir)
        if self.audio:
            import numpy as np
            import soundfile as sf
            sr = 44100
            t = np.arange(sr // 2) / sr
            tone = (0.2 * np.sin(2 * np.pi * 220 * t)).astype("float32")
            sf.write(str(job_dir / "audio.flac"), np.stack([tone, tone], axis=1), sr)


class _FakePipe:
    """Пайплайн yue2: script — что вернуть/бросить на очередной вызов
    (исключение, _FakeSong или callable(kw) → одно из них); пусто — готовый трек."""

    def __init__(self, *script):
        self.script = list(script)
        self.calls = []
        self.dirs = []
        self.exited = False

    def __enter__(self):
        return self

    def __exit__(self, *a):
        self.exited = True

    def plan(self, **kw):
        raise AssertionError("план в этих тестах не нужен")

    def __call__(self, **kw):
        self.calls.append(dict(kw))
        r = self.script.pop(0) if self.script else _FakeSong()
        if callable(r) and not isinstance(r, (_FakeSong, BaseException)):
            r = r(kw)
        if isinstance(r, BaseException):
            raise r
        pipe = self

        class _Tracked:
            truncated = r.truncated

            def save_artifacts(self, d):
                pipe.dirs.append(Path(d).name)
                r.save_artifacts(d)
        return _Tracked()


class _TimeNoSleep:
    """Модуль time воркера, где sleep (очередь пуста) останавливает цикл."""

    def __init__(self):
        import time as real
        self._real = real

    def sleep(self, _s):
        raise _Stop()

    def __getattr__(self, name):
        return getattr(self._real, name)


class _Case(_WorkerApiCase):
    def setUp(self):
        super().setUp()
        from unittest import mock
        protocol = types.ModuleType("yue2.protocol")
        protocol.Sampling = _Sampling
        self.yue2 = mock.MagicMock()
        mods = {"yue2": self.yue2, "yue2.protocol": protocol,
                "yue2.pipeline": mock.MagicMock(), "torch": mock.MagicMock()}
        p = mock.patch.dict(sys.modules, mods)
        p.start()
        self.addCleanup(p.stop)
        data = Path(self._td.name) / "cfg"
        data.mkdir()
        for name, val in (("_pipe", None), ("_load_error", None),
                          ("DATA_DIR", data), ("SETTINGS_PATH", data / "settings.json")):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)
        # дорожки — внешняя граница (demucs/roformer на GPU)
        self.separations = []
        p = mock.patch.object(self.w, "demucs_separate", side_effect=self._fake_separate)
        p.start()
        self.addCleanup(p.stop)
        self.separate_error = None

    def _fake_separate(self, audio, out_dir, **kw):
        self.separations.append(Path(audio))
        if self.separate_error:
            raise self.separate_error
        import numpy as np
        import soundfile as sf
        out_dir = Path(out_dir)
        names = []
        for n in ("drums", "bass", "other", "vocals"):
            sf.write(str(out_dir / f"stem-{n}.flac"), np.zeros((4410, 2), dtype="float32"), 44100)
            names.append(f"stem-{n}.flac")
        return {"stems": names, "seconds": 0.0}

    def _use_pipe(self, pipe):
        self.w._pipe = pipe
        return pipe

    def _status(self, jid):
        r = self.client.get(f"/jobs/{jid}")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _new(self, **extra):
        return _job_id(self, _post_job(self, **extra))

    def _set_status(self, jid, status):
        with self._conn() as c:
            c.execute("UPDATE jobs SET status=? WHERE id=?", (status, jid))

    def _run_queue_once(self):
        """Один проход очереди до опустения; наружу — только _Stop."""
        from unittest import mock
        with mock.patch.object(self.w, "time", _TimeNoSleep()):
            with self.assertRaises(_Stop):
                self.w.queue_loop()


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestCancel(_Case):

    def test_queued_canceled_and_never_runs(self):
        pipe = self._use_pipe(_FakePipe())
        jid = self._new()
        r = self.client.post(f"/jobs/{jid}/cancel")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["canceled"], True)
        self.assertEqual(self._status(jid)["status"], "canceled")
        self._run_queue_once()
        self.assertEqual(pipe.calls, [], "отменённая джоба ушла в генерацию")
        self.assertEqual(self._status(jid)["status"], "canceled")

    def test_running_stops_generation_and_ends_canceled(self):
        seen = {}

        def generation(kw):
            # пока идёт генерация, пользователь жмёт «отменить»
            seen["before"] = kw["cancelled"]()
            r = self.client.post(f"/jobs/{jid}/cancel")
            seen["resp"] = (r.status_code, r.json())
            seen["after"] = kw["cancelled"]()
            seen["status_during"] = self._status(jid)["status"]
            # yue2 на ближайшем шаге видит флаг и прерывается
            return InterruptedError("canceled") if seen["after"] else _FakeSong()

        pipe = self._use_pipe(_FakePipe(generation))
        jid = self._new()
        self._set_status(jid, "running")  # как после захвата очередью
        self.w._run_job(jid)
        self.assertIs(seen["before"], False)
        self.assertEqual(seen["resp"][0], 200)
        self.assertIs(seen["resp"][1]["canceled"], True)
        self.assertIs(seen["after"], True, "флаг отмены не дошёл до пайплайна")
        self.assertEqual(seen["status_during"], "running")
        got = self._status(jid)
        self.assertEqual(got["status"], "canceled")
        self.assertFalse(got.get("error"), got)
        self.assertIs(self.w._pipe, pipe, "отмена не должна выгружать модель")
        self.assertFalse(pipe.exited)

    def test_cancel_after_finish_is_noop(self):
        self._use_pipe(_FakePipe())
        jid = self._new()
        self._set_status(jid, "running")
        self.w._run_job(jid)
        self.assertEqual(self._status(jid)["status"], "done")
        r = self.client.post(f"/jobs/{jid}/cancel")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["canceled"], False)
        self.assertEqual(self._status(jid)["status"], "done")

    def test_done_and_error_not_canceled(self):
        for st in ("done", "error"):
            with self.subTest(status=st):
                jid = self._job(status=st, semantic=False)
                r = self.client.post(f"/jobs/{jid}/cancel")
                self.assertEqual(r.status_code, 200, r.text)
                self.assertIs(r.json()["canceled"], False)
                self.assertEqual(self._row(jid)["status"], st)

    def test_unknown_job_nothing_canceled(self):
        r = self.client.post("/jobs/9999/cancel")
        self.assertTrue(r.status_code == 404 or r.json().get("canceled") is False, r.text)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestDeleteJob(_Case):

    def _insert(self, jid, status="done"):
        with self._conn() as c:
            c.execute("INSERT INTO jobs (id, title, status, style, lyrics, created_at) "
                      "VALUES (?, 't', ?, 's', 'l', '2026-01-01T00:00:00')", (jid, status))
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        (d / "audio.flac").write_bytes(b"fLaC-" + str(jid).encode())
        return d

    def test_deletes_row_and_own_folder_only(self):
        d1 = self._insert(1)
        d11 = self._insert(11)       # номер с тем же началом
        d10 = self._insert(10)
        (d1 / "dsp").mkdir()
        (d1 / "dsp" / "x.flac").write_bytes(b"x")
        stray = self.jobs_dir / "note.txt"
        stray.write_text("keep")
        r = self.client.delete("/jobs/1")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertFalse(d1.exists(), "папка удалённой джобы осталась")
        self.assertEqual(self.client.get("/jobs/1").status_code, 404)
        for d in (d10, d11):
            self.assertTrue((d / "audio.flac").is_file(), d)
        self.assertEqual(self.client.get("/jobs/11").status_code, 200)
        self.assertTrue(stray.is_file())
        self.assertTrue(self.db_path.is_file())

    def test_unknown_404_nothing_removed(self):
        d = self._insert(3)
        r = self.client.delete("/jobs/4")
        self.assertEqual(r.status_code, 404, r.text)
        self.assertTrue((d / "audio.flac").is_file())
        self.assertEqual(self.client.get("/jobs/3").status_code, 200)

    def test_running_409_row_and_files_kept(self):
        d = self._insert(5, status="running")
        r = self.client.delete("/jobs/5")
        self.assertEqual(r.status_code, 409, r.text)
        self.assertTrue((d / "audio.flac").is_file())
        self.assertEqual(self._row(5)["status"], "running")

    def test_not_a_number_does_not_touch_anything(self):
        self._insert(2)
        before = sorted(p.name for p in Path(self._td.name).rglob("*"))
        for path in ("/jobs/..", "/jobs/%2E%2E", "/jobs/..%2F..", "/jobs/2%2F..", "/jobs/-"):
            with self.subTest(path=path):
                r = self.client.delete(path)
                self.assertGreaterEqual(r.status_code, 400, r.text)
        self.assertTrue(self.jobs_dir.is_dir())
        self.assertEqual(sorted(p.name for p in Path(self._td.name).rglob("*")), before)

    def test_folder_symlinked_outside_is_not_followed(self):
        outside = Path(self._td.name) / "outside"
        outside.mkdir()
        (outside / "precious.flac").write_bytes(b"mine")
        with self._conn() as c:
            c.execute("INSERT INTO jobs (id, title, status, style, lyrics, created_at) "
                      "VALUES (7, 't', 'done', 's', 'l', '2026-01-01T00:00:00')")
        os.symlink(outside, self.jobs_dir / "7")
        r = self.client.delete("/jobs/7")
        self.assertLess(r.status_code, 500, r.text)
        self.assertTrue((outside / "precious.flac").is_file(), "удаление ушло по ссылке наружу")

    def test_symlink_inside_folder_not_followed(self):
        outside = Path(self._td.name) / "outside2"
        outside.mkdir()
        (outside / "precious.flac").write_bytes(b"mine")
        d = self._insert(8)
        os.symlink(outside, d / "link")
        r = self.client.delete("/jobs/8")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertFalse((self.jobs_dir / "8").exists())
        self.assertTrue((outside / "precious.flac").is_file(), "удаление ушло по ссылке наружу")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRunJobBranches(_Case):

    def _run(self, **extra):
        jid = self._new(**extra)
        self._set_status(jid, "running")
        self.w._run_job(jid)  # исключение наружу = провал теста
        return jid

    def test_done_on_success(self):
        pipe = self._use_pipe(_FakePipe())
        jid = self._run()
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertEqual(pipe.dirs, [str(jid)], "артефакты пишутся в папку своей джобы")
        self.assertTrue((self.jobs_dir / str(jid) / "audio.flac").is_file())

    def test_interrupted_is_canceled_not_error(self):
        pipe = self._use_pipe(_FakePipe(InterruptedError("stop")))
        jid = self._run()
        got = self._status(jid)
        self.assertEqual(got["status"], "canceled")
        self.assertFalse(got.get("error"))
        self.assertIs(self.w._pipe, pipe)

    def test_oom_error_unloads_model_and_next_job_reloads(self):
        oom = self._use_pipe(_FakePipe(RuntimeError(OOM_TEXT)))
        jid = self._run()
        got = self._status(jid)
        self.assertEqual(got["status"], "error")
        self.assertTrue(got.get("error"), "причина ошибки пуста")
        self.assertTrue(oom.exited, "после OOM модель не выгружена")
        self.assertIsNone(self.w._pipe, "после OOM модель осталась в памяти")
        # следующий трек: модель загружается заново и трек готов
        fresh = _FakePipe()
        self.yue2.YuE2Pipeline.from_pretrained.return_value = fresh
        jid2 = self._run()
        self.assertEqual(self._status(jid2)["status"], "done")
        self.assertEqual(len(fresh.calls), 1)
        self.assertEqual(oom.calls[1:], [], "выгруженная модель снова вызвана")

    def test_other_error_keeps_model(self):
        pipe = self._use_pipe(_FakePipe(RuntimeError("bad plan tokens")))
        jid = self._run()
        got = self._status(jid)
        self.assertEqual(got["status"], "error")
        self.assertIn("bad plan tokens", got.get("error", ""))
        self.assertIs(self.w._pipe, pipe)
        self.assertFalse(pipe.exited)

    def test_old_yue2_without_cfg_scale_retried_without_it(self):
        def old_lib(kw):
            if "cfg_scale" in kw:
                return TypeError("__call__() got an unexpected keyword argument 'cfg_scale'")
            return _FakeSong()

        pipe = self._use_pipe(_FakePipe(old_lib, old_lib))
        jid = self._run(cfg=2.5)
        self.assertEqual(self._status(jid)["status"], "done")
        self.assertEqual(len(pipe.calls), 2, pipe.calls)
        self.assertIn("cfg_scale", pipe.calls[0])
        self.assertNotIn("cfg_scale", pipe.calls[1])
        # остальное в повторе — то же
        first = {k: v for k, v in pipe.calls[0].items() if k != "cfg_scale"}
        self.assertEqual(set(first), set(pipe.calls[1]))
        self.assertEqual(first["style"], pipe.calls[1]["style"])

    def test_other_type_error_is_error_without_retry(self):
        pipe = self._use_pipe(_FakePipe(TypeError("unsupported operand type(s)")))
        jid = self._run()
        self.assertEqual(self._status(jid)["status"], "error")
        self.assertEqual(len(pipe.calls), 1)

    def test_instrumental_builds_stems_automatically(self):
        self._use_pipe(_FakePipe())
        jid = self._run(style=INSTRUMENTAL)
        self.assertEqual(self._status(jid)["status"], "done")
        self.assertEqual(len(self.separations), 1, "дорожки трека без голоса не построены")
        self.assertEqual(self.separations[0].parent.name, str(jid))
        r = self.client.get(f"/jobs/{jid}/stems")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIn("stem-vocals.flac", r.text)

    def test_no_vocals_style_also_instrumental(self):
        self._use_pipe(_FakePipe())
        self._run(style="ambient, no vocals")
        self.assertEqual(len(self.separations), 1)

    def test_vocal_track_no_auto_stems(self):
        self._use_pipe(_FakePipe())
        jid = self._run(style="dark rock, male baritone")
        self.assertEqual(self._status(jid)["status"], "done")
        self.assertEqual(self.separations, [])

    def test_setting_off_no_auto_stems(self):
        r = self.client.post("/config", json={"auto_stems_instrumental": False})
        self.assertLess(r.status_code, 300, r.text)
        self._use_pipe(_FakePipe())
        jid = self._run(style=INSTRUMENTAL)
        self.assertEqual(self._status(jid)["status"], "done")
        self.assertEqual(self.separations, [])

    def test_stems_failure_keeps_track_done(self):
        self.separate_error = RuntimeError("demucs crashed")
        self._use_pipe(_FakePipe())
        jid = self._run(style=INSTRUMENTAL)
        self.assertEqual(len(self.separations), 1)
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertFalse(got.get("error"))

    def test_failed_generation_no_auto_stems(self):
        self._use_pipe(_FakePipe(RuntimeError("boom")))
        self._run(style=INSTRUMENTAL)
        self.assertEqual(self.separations, [])


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestQueue(_Case):

    def test_runs_queued_in_order_once(self):
        pipe = self._use_pipe(_FakePipe())
        a, b, c = self._new(), self._new(), self._new()
        self._run_queue_once()
        self.assertEqual(pipe.dirs, [str(a), str(b), str(c)])
        for j in (a, b, c):
            self.assertEqual(self._status(j)["status"], "done")
        self._run_queue_once()  # готовые повторно не берутся
        self.assertEqual(len(pipe.calls), 3)

    def test_skips_not_queued(self):
        pipe = self._use_pipe(_FakePipe())
        ids = {}
        for st in ("canceled", "done", "error", "running"):
            ids[st] = self._new()
            self._set_status(ids[st], st)
        q = self._new()
        self._run_queue_once()
        self.assertEqual(pipe.dirs, [str(q)])
        for st, j in ids.items():
            self.assertEqual(self._row(j)["status"], st)

    def test_canceled_between_submit_and_start_skipped(self):
        pipe = self._use_pipe(_FakePipe())
        a, b = self._new(), self._new()
        self.client.post(f"/jobs/{a}/cancel")
        self._run_queue_once()
        self.assertEqual(pipe.dirs, [str(b)])
        self.assertEqual(self._status(a)["status"], "canceled")

    def test_retry_requeues_failed_job(self):
        pipe = self._use_pipe(_FakePipe(RuntimeError("boom")))
        a = self._new()
        self._run_queue_once()
        self.assertEqual(self._status(a)["status"], "error")
        r = self.client.post(f"/jobs/{a}/retry")
        self.assertEqual(r.status_code, 200, r.text)
        self._run_queue_once()
        self.assertEqual(self._status(a)["status"], "done")
        self.assertEqual(len(pipe.calls), 2)

    def test_model_load_failure_marks_error_and_queue_survives(self):
        self.yue2.YuE2Pipeline.from_pretrained.side_effect = RuntimeError("no CUDA device")
        a, b = self._new(), self._new()
        self._run_queue_once()  # наружу только _Stop: очередь не упала
        for j in (a, b):
            got = self._status(j)
            self.assertEqual(got["status"], "error", got)
            self.assertTrue(got.get("error"))

    def test_startup_marks_stale_running_as_error(self):
        from unittest import mock
        run = self._new()
        self._set_status(run, "running")
        q = self._new()
        with mock.patch.object(self.w.threading, "Thread") as th:
            self.w._startup()
        got = self._status(run)
        self.assertEqual(got["status"], "error")
        self.assertTrue(got.get("error"))
        self.assertEqual(self._status(q)["status"], "queued")
        started = [c for c in th.call_args_list if c.kwargs.get("target") is self.w.queue_loop]
        self.assertEqual(len(started), 1, "очередь не запущена после старта")


if __name__ == "__main__":
    unittest.main()
