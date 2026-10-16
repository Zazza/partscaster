"""Спецификация черновика работы над треком (задача internal-studio-draft, условие 1, ТК1).

Что должно работать:
- таблица studio_drafts(job_id PK, state TEXT JSON, updated_at TEXT);
- GET /drafts → [{job_id, state, updated_at}];
- GET /jobs/{id}/draft → {job_id, state, updated_at}; черновика нет — 404;
- PUT /jobs/{id}/draft {state: object} — создать/заменить; трека нет — 404;
  state не объект или JSON > 1 МБ — 422;
- DELETE /jobs/{id}/draft → {deleted: bool};
- удаление трека удаляет его черновик;
- GET /jobs: у каждого трека has_draft (bool).

Модель и GPU не нужны: каждый тест — своя временная БД (_WorkerApiCase).
"""
import json
import unittest

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"


def _state(n=1):
    return {"v": 1, "edits": [{"kind": "mute", "childId": -n, "db": -6}], "queue": [], "ts": 1000 + n}


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestDraftEndpoints(_WorkerApiCase):

    def _put(self, jid, state):
        return self.client.put(f"/jobs/{jid}/draft", json={"state": state})

    def _get(self, jid):
        return self.client.get(f"/jobs/{jid}/draft")

    def test_put_then_get_returns_state(self):
        jid = self._job(semantic=False)
        st = _state(1)
        r = self._put(jid, st)
        self.assertEqual(r.status_code, 200, r.text)
        g = self._get(jid)
        self.assertEqual(g.status_code, 200, g.text)
        d = g.json()
        self.assertEqual(int(d["job_id"]), jid)
        self.assertEqual(d["state"], st)
        self.assertIsInstance(d["updated_at"], str)
        self.assertTrue(d["updated_at"])

    def test_second_put_replaces(self):
        jid = self._job(semantic=False)
        self.assertEqual(self._put(jid, _state(1)).status_code, 200)
        st2 = {"v": 1, "edits": [], "queue": [{"title": "x"}], "ts": 99}
        r = self._put(jid, st2)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._get(jid).json()["state"], st2)
        # заменяет, а не добавляет вторую запись
        mine = [d for d in self.client.get("/drafts").json() if int(d["job_id"]) == jid]
        self.assertEqual(len(mine), 1, mine)

    def test_get_without_draft_404(self):
        jid = self._job(semantic=False)
        self.assertEqual(self._get(jid).status_code, 404)

    def test_list_contains_draft(self):
        a = self._job(semantic=False)
        b = self._job(semantic=False)
        self._put(a, _state(1))
        r = self.client.get("/drafts")
        self.assertEqual(r.status_code, 200, r.text)
        items = r.json()
        self.assertIsInstance(items, list)
        by_id = {int(d["job_id"]): d for d in items}
        self.assertIn(a, by_id)
        self.assertNotIn(b, by_id)
        self.assertEqual(by_id[a]["state"], _state(1))
        self.assertIn("updated_at", by_id[a])

    def test_list_empty(self):
        r = self.client.get("/drafts")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json(), [])

    def test_delete_then_get_404(self):
        jid = self._job(semantic=False)
        self._put(jid, _state(1))
        r = self.client.delete(f"/jobs/{jid}/draft")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["deleted"], True)
        self.assertEqual(self._get(jid).status_code, 404)
        self.assertNotIn(jid, [int(d["job_id"]) for d in self.client.get("/drafts").json()])

    def test_delete_missing_draft_false(self):
        jid = self._job(semantic=False)
        r = self.client.delete(f"/jobs/{jid}/draft")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["deleted"], False)

    def test_put_unknown_track_404(self):
        r = self._put(999999, _state(1))
        self.assertEqual(r.status_code, 404, r.text)
        self.assertEqual(self.client.get("/drafts").json(), [])

    def test_put_not_object_422(self):
        jid = self._job(semantic=False)
        for bad in ([1, 2], "text", 5, None):
            with self.subTest(state=bad):
                r = self._put(jid, bad)
                self.assertEqual(r.status_code, 422, r.text)
        self.assertEqual(self._get(jid).status_code, 404, "отказ не должен создавать черновик")

    def test_put_missing_state_422(self):
        jid = self._job(semantic=False)
        r = self.client.put(f"/jobs/{jid}/draft", json={})
        self.assertEqual(r.status_code, 422, r.text)

    def test_put_over_1mb_422(self):
        jid = self._job(semantic=False)
        big = {"v": 1, "edits": [], "queue": [], "ts": 1, "pad": "x" * (1024 * 1024 + 10)}
        self.assertGreater(len(json.dumps(big)), 1024 * 1024)
        r = self._put(jid, big)
        self.assertEqual(r.status_code, 422, r.text[:200])
        self.assertEqual(self._get(jid).status_code, 404)

    def test_put_under_1mb_ok(self):
        jid = self._job(semantic=False)
        ok = {"v": 1, "pad": "x" * (512 * 1024)}
        r = self._put(jid, ok)
        self.assertEqual(r.status_code, 200, r.text[:200])
        self.assertEqual(self._get(jid).json()["state"], ok)

    def test_stored_in_studio_drafts_table(self):
        """Черновик живёт в БД воркера (переживает перезапуск), а не в памяти процесса."""
        jid = self._job(semantic=False)
        self._put(jid, _state(3))
        with self._conn() as c:
            row = c.execute("SELECT job_id, state, updated_at FROM studio_drafts WHERE job_id=?",
                            (jid,)).fetchone()
        self.assertIsNotNone(row)
        self.assertEqual(json.loads(row[1]), _state(3))
        self.assertTrue(row[2])

    def test_delete_track_deletes_draft(self):
        a = self._job(semantic=False)
        b = self._job(semantic=False)
        self._put(a, _state(1))
        self._put(b, _state(2))
        r = self.client.delete(f"/jobs/{a}")
        self.assertEqual(r.status_code, 200, r.text)
        ids = [int(d["job_id"]) for d in self.client.get("/drafts").json()]
        self.assertNotIn(a, ids)
        self.assertIn(b, ids, "черновик другого трека не трогается")
        with self._conn() as c:
            n = c.execute("SELECT COUNT(*) FROM studio_drafts WHERE job_id=?", (a,)).fetchone()[0]
        self.assertEqual(n, 0)

    def test_jobs_list_has_draft(self):
        a = self._job(semantic=False)
        b = self._job(semantic=False)
        self._put(a, _state(1))
        r = self.client.get("/jobs")
        self.assertEqual(r.status_code, 200, r.text)
        by_id = {int(j["id"]): j for j in r.json()}
        self.assertIs(by_id[a]["has_draft"], True)
        self.assertIs(by_id[b]["has_draft"], False)

    def test_jobs_list_has_draft_false_after_clear(self):
        a = self._job(semantic=False)
        self._put(a, _state(1))
        self.client.delete(f"/jobs/{a}/draft")
        by_id = {int(j["id"]): j for j in self.client.get("/jobs").json()}
        self.assertIs(by_id[a]["has_draft"], False)


if __name__ == "__main__":
    unittest.main()
