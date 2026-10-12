"""Спецификация: файлы отдаются и пишутся только внутри своей папки.

P1. GET /audio/{id}/{f}: файл джобы отдаётся как есть (200); нет файла → 404;
    имя с `..`, `/` (в т.ч. закодированными), абсолютный путь — 400/404, и ни
    байта чужого файла (соседней джобы, БД, вне данных) в ответе.
P2. GET /listen/{id}: страница плеера готового трека (src = /audio/{id}/<файл>);
    не готовый/нет → 404; ?f= — только имя варианта (dsp-/overdub-/preview-/
    stem-…flac), `..`/`/`/прочее → 400; название трека на странице — текстом,
    не разметкой.
P3. GET /transcribes/{tid}/{f}: `..`/`/` в любой части → 400/404, чужое не отдаётся.
P4. DELETE /jobs/{id}/dsp/{f}: удаляет только вариант этой джобы; `..`/`/` — отказ,
    файлы вне папки целы; основной трек и дорожки — не варианты.
P5. POST /tracks/import: трек становится готовой джобой (done) с файлом в своей
    папке — его можно скачать через /audio; X-Filename с путём не выводит запись
    за папку джобы; пустое тело → 422 без новой джобы; title — название, без
    него — имя файла; транскрипция по запросу (score.abc), сбой транскрипции —
    трек всё равно импортирован, причина в transcribe_error.
"""
import unittest
from pathlib import Path
from unittest import mock

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

SECRET = b"TOP-SECRET-BYTES"


class _Case(_WorkerApiCase):
    def setUp(self):
        super().setUp()
        self.root = Path(self._td.name)
        self.tr_dir = self.root / "transcribes"
        self.tr_dir.mkdir(exist_ok=True)
        p = mock.patch.object(self.w, "TRANSCRIBES_DIR", self.tr_dir)
        p.start()
        self.addCleanup(p.stop)
        # чужое: в корне данных, в корне jobs, в соседней джобе
        (self.root / "secret.txt").write_bytes(SECRET)
        (self.jobs_dir / "secret.txt").write_bytes(SECRET)
        self.mine = self._done_job(b"MY-AUDIO")
        self.other = self._done_job(SECRET)

    def _done_job(self, audio: bytes, title="t"):
        jid = self._job(semantic=False)
        with self._conn() as c:
            c.execute("UPDATE jobs SET audio_file='audio.flac', title=? WHERE id=?", (title, jid))
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        (d / "audio.flac").write_bytes(audio)
        return jid

    def _assert_refused(self, path, method="get"):
        r = getattr(self.client, method)(path)
        self.assertIn(r.status_code, (400, 403, 404, 405, 422), f"{path}: {r.status_code}")
        self.assertNotIn(SECRET, r.content, path)
        return r

    def _tree(self):
        return sorted(str(p.relative_to(self.root)) for p in self.root.rglob("*"))

    def _endpoint(self, method, path):
        """Обработчик маршрута по пути из API (не по имени функции)."""
        for r in self.w.app.routes:
            if getattr(r, "path", "") == path and method in (getattr(r, "methods", ()) or ()):
                return r.endpoint
        self.fail(f"нет маршрута {method} {path}")

    def _assert_handler_refuses(self, fn, *args):
        """Защита в глубину: HTTP-маршрутизация сама режет «/», но обработчик
        не должен полагаться на неё — имя с путём он отклоняет сам."""
        from fastapi import HTTPException
        try:
            res = fn(*args)
        except HTTPException as e:
            self.assertIn(e.status_code, (400, 403, 404, 422), args)
            return
        path = Path(getattr(res, "path", ""))
        self.fail(f"{args}: обработчик отдал {path}")


TRAVERSALS = (
    "..%2Fsecret.txt",
    "%2E%2E%2Fsecret.txt",
    "%2e%2e%2f%2e%2e%2fsecret.txt",
    "..%2F{other}%2Faudio.flac",
    "..%5Csecret.txt",
    "%2Fetc%2Fpasswd",
    "%2F{root}%2Fsecret.txt",
    "..",
    "%2E%2E",
    "%252E%252E%252Fsecret.txt",   # двойное кодирование
    "..%252F{other}%252Faudio.flac",
)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestAudio(_Case):

    def test_serves_own_file(self):
        r = self.client.get(f"/audio/{self.mine}/audio.flac")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.content, b"MY-AUDIO")

    def test_missing_404(self):
        self.assertEqual(self.client.get(f"/audio/{self.mine}/nope.flac").status_code, 404)
        self.assertEqual(self.client.get("/audio/999/audio.flac").status_code, 404)

    def test_traversal_refused(self):
        for t in TRAVERSALS:
            name = t.format(other=self.other, root=str(self.root).lstrip("/").replace("/", "%2F"))
            with self.subTest(name=name):
                self._assert_refused(f"/audio/{self.mine}/{name}")

    def test_handler_refuses_path_in_name(self):
        fn = self._endpoint("GET", "/audio/{job_id}/{fname}")
        for name in ("../secret.txt", "../../secret.txt", f"../{self.other}/audio.flac",
                     str(self.root / "secret.txt"), "..", "%2E%2E%2Fsecret.txt"):
            with self.subTest(name=name):
                self._assert_handler_refuses(fn, self.mine, name)

    def test_job_id_traversal_refused(self):
        for jid in ("..", "%2E%2E", f"{self.mine}%2F..", "-1"):
            with self.subTest(jid=jid):
                self._assert_refused(f"/audio/{jid}/secret.txt")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestListen(_Case):

    def test_page_for_done_track(self):
        r = self.client.get(f"/listen/{self.mine}")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIn("text/html", r.headers.get("content-type", ""))
        self.assertIn(f"/audio/{self.mine}/audio.flac", r.text)

    def test_variant_in_page(self):
        r = self.client.get(f"/listen/{self.mine}", params={"f": "dsp-warm.flac"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIn(f"/audio/{self.mine}/dsp-warm.flac", r.text)

    def test_not_done_or_missing_404(self):
        for st in ("queued", "running", "error", "canceled"):
            with self.subTest(status=st):
                jid = self._job(status=st, semantic=False)
                self.assertEqual(self.client.get(f"/listen/{jid}").status_code, 404)
        self.assertEqual(self.client.get("/listen/999").status_code, 404)

    def test_bad_variant_names_400(self):
        for f in ("../secret.txt", "../../secret.txt", "dsp-../../x.flac", "stem-..flac",
                  "dsp-a/b.flac", "/etc/passwd", "secret.txt", "audio.flac\"><script>",
                  f"../{self.other}/audio.flac"):
            with self.subTest(f=f):
                r = self.client.get(f"/listen/{self.mine}", params={"f": f})
                self.assertEqual(r.status_code, 400, r.text)
                self.assertNotIn("<script>", r.text)

    def test_title_is_text_not_markup(self):
        jid = self._done_job(b"x", title="<script>alert(1)</script>")
        r = self.client.get(f"/listen/{jid}")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertNotIn("<script>alert(1)</script>", r.text)
        self.assertIn("&lt;script&gt;", r.text)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestTranscribeFiles(_Case):

    def setUp(self):
        super().setUp()
        d = self.tr_dir / "123-song.flac"
        d.mkdir()
        (d / "score.abc").write_text("X:1\n")

    def test_serves_own_file(self):
        r = self.client.get("/transcribes/123-song.flac/score.abc")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.text, "X:1\n")

    def test_traversal_refused(self):
        for path in ("/transcribes/123-song.flac/..%2F..%2Fsecret.txt",
                     "/transcribes/..%2F/secret.txt",
                     "/transcribes/../secret.txt",
                     "/transcribes/%2E%2E/secret.txt",
                     "/transcribes/..%2Fjobs/secret.txt",
                     "/transcribes/%252E%252E/secret.txt",
                     "/transcribes/123-song.flac/%252E%252E%252F%252E%252E%252Fsecret.txt",
                     f"/transcribes/..%2Fjobs%2F{self.other}/audio.flac"):
            with self.subTest(path=path):
                self._assert_refused(path)


    def test_handler_refuses_path_in_name(self):
        fn = self._endpoint("GET", "/transcribes/{tid}/{fname}")
        for tid, name in (("123-song.flac", "../../secret.txt"), ("..", "secret.txt"),
                          ("../jobs", "secret.txt"), (f"../jobs/{self.other}", "audio.flac"),
                          (str(self.root), "secret.txt"),
                          ("123-song.flac", "%2E%2E%2F%2E%2E%2Fsecret.txt")):
            with self.subTest(tid=tid, name=name):
                self._assert_handler_refuses(fn, tid, name)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestDspVariantDelete(_Case):

    def test_deletes_variant_with_sidecars_only(self):
        d = self.jobs_dir / str(self.mine)
        (d / "dsp-warm.flac").write_bytes(b"v")
        (d / "dsp-warm.flac.metrics.json").write_text("{}")
        (d / "dsp-cold.flac").write_bytes(b"v2")
        r = self.client.delete(f"/jobs/{self.mine}/dsp/dsp-warm.flac")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertFalse((d / "dsp-warm.flac").exists())
        self.assertFalse((d / "dsp-warm.flac.metrics.json").exists())
        self.assertTrue((d / "dsp-cold.flac").is_file())
        self.assertTrue((d / "audio.flac").is_file())

    def test_missing_variant_404(self):
        r = self.client.delete(f"/jobs/{self.mine}/dsp/dsp-none.flac")
        self.assertEqual(r.status_code, 404, r.text)

    def test_traversal_refused_nothing_removed(self):
        before = self._tree()
        for name in ("..%2Fsecret.txt", "..%2F..%2Fsecret.txt", f"..%2F{self.other}%2Faudio.flac",
                     "audio.flac", "%2E%2E", "dsp-..%2F..%2Fsecret.txt"):
            with self.subTest(name=name):
                self._assert_refused(f"/jobs/{self.mine}/dsp/{name}", method="delete")
        self.assertEqual(self._tree(), before)

    def test_stem_is_not_a_variant(self):
        # удаление вариантов не стирает дорожки трека
        d = self.jobs_dir / str(self.mine)
        (d / "stem-vocals.flac").write_bytes(b"stem")
        r = self.client.delete(f"/jobs/{self.mine}/dsp/stem-vocals.flac")
        self.assertGreaterEqual(r.status_code, 400, r.text)
        self.assertTrue((d / "stem-vocals.flac").is_file())


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestTrackImport(_Case):

    def _import(self, data=b"IMPORTED", name="song.flac", **params):
        params.setdefault("transcribe", "false")
        return self.client.post("/tracks/import", content=data,
                                headers={"X-Filename": name}, params=params)

    def _count(self):
        with self._conn() as c:
            return c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]

    def test_import_becomes_done_job_with_downloadable_file(self):
        r = self._import()
        self.assertEqual(r.status_code, 200, r.text)
        jid = int(r.json()["id"])
        got = self.client.get(f"/jobs/{jid}").json()
        self.assertEqual(got["status"], "done")
        self.assertEqual(got["title"], "song")
        f = got["audio_file"]
        self.assertTrue(f)
        a = self.client.get(f"/audio/{jid}/{f}")
        self.assertEqual(a.status_code, 200)
        self.assertEqual(a.content, b"IMPORTED")
        self.assertEqual(self.client.get(f"/listen/{jid}").status_code, 200)

    def test_title_param(self):
        r = self._import(title="Мой трек")
        jid = int(r.json()["id"])
        self.assertEqual(self.client.get(f"/jobs/{jid}").json()["title"], "Мой трек")

    def test_empty_body_422_no_job(self):
        before = self._count()
        r = self._import(data=b"")
        self.assertEqual(r.status_code, 422, r.text)
        self.assertEqual(self._count(), before)

    def test_filename_with_path_stays_in_job_folder(self):
        for name in ("../../evil.flac", "../secret.txt", "/etc/cron.d/x.flac",
                     "..\\..\\evil.flac", "....//....//evil.flac", ".."):
            with self.subTest(name=name):
                before = set(self._tree())
                r = self._import(data=b"EVIL", name=name)
                self.assertEqual(r.status_code, 200, r.text)
                jid = int(r.json()["id"])
                new = set(self._tree()) - before
                jdir = f"jobs/{jid}"
                outside = [p for p in new if p != jdir and not p.startswith(jdir + "/")]
                self.assertEqual(outside, [], f"запись вне папки джобы: {outside}")
                self.assertEqual((self.root / "secret.txt").read_bytes(), SECRET)
                f = self.client.get(f"/jobs/{jid}").json()["audio_file"]
                self.assertEqual(self.client.get(f"/audio/{jid}/{f}").content, b"EVIL")

    def test_transcribe_writes_score(self):
        def fake_ss(src, dst):
            self.assertTrue(Path(src).is_file())
            return {"abc": "X:1\nK:C\nCDEF|\n"}

        with mock.patch.object(self.w, "ss_transcribe", side_effect=fake_ss):
            r = self._import(transcribe="true")
        self.assertEqual(r.status_code, 200, r.text)
        body = r.json()
        self.assertEqual(body["transcribe_error"], "")
        jid = int(body["id"])
        s = self.client.get(f"/audio/{jid}/score.abc")
        self.assertEqual(s.status_code, 200)
        self.assertIn("CDEF", s.text)
        self.assertTrue(self.client.get(f"/jobs/{jid}").json()["abc_file"])

    def test_transcribe_failure_still_imports(self):
        with mock.patch.object(self.w, "ss_transcribe", side_effect=RuntimeError("sheetsage down")):
            r = self._import(transcribe="true")
        self.assertEqual(r.status_code, 200, r.text)
        body = r.json()
        self.assertIn("sheetsage down", body["transcribe_error"])
        got = self.client.get(f"/jobs/{int(body['id'])}").json()
        self.assertEqual(got["status"], "done")
        self.assertFalse(got["abc_file"])

    def test_no_transcribe_does_not_call_sheetsage(self):
        with mock.patch.object(self.w, "ss_transcribe") as ss:
            r = self._import(transcribe="false")
        self.assertEqual(r.status_code, 200, r.text)
        ss.assert_not_called()


if __name__ == "__main__":
    unittest.main()
