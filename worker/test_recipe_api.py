"""Спецификация API рецепта трека воркера.

Что должно работать:
J1. POST /jobs/{id}/ops {op, params: obj, result: str, refs: [int]} → {id}; op —
    только из списка (иначе 422); params не объект / битый JSON → 422; трека нет → 404.
    GET /jobs/{id}/ops → [{id, op, params, result, refs, created_at}] по порядку записи.
E1. GET /jobs/{id}/recipe: корень песни (по parent_id/overdub_of до верха) — из
    столбцов; plan_abc = текст score.abc, abc = текст request.abc (если был, иначе
    null); worker_model/stems_model — как в /version; потомки — из журнала (n-id).
E2. Отказы 422: корень — черновик; корень — импорт чужого аудио; в песне есть
    версия role=voice. Трека нет — 404.
I1. POST /recipes/import {recipe, mode: seed|plan} → {id}; чужой format/version → 422.
    Сессия создаётся running; GET /recipes/imports — список; GET /recipes/imports/{id}
    — то же + recipe + state.
I2. POST /recipes/imports/{id}/claim {owner}: свободна → 200; держит другой и лиза
    (600 с, RECIPE_LEASE_S) не истекла → 409; тот же owner → 200 (продление);
    лиза истекла → другой берёт (200).
I3. PATCH /recipes/imports/{id} {state?, progress?, status?, error?, root_job_id?} →
    объект сессии; status done|error ставит finished_at.
"""
import time
import unittest
from unittest import mock

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

IMPORT_STYLE = "(импорт внешнего трека)"
ALL_OPS = ("submit", "continue", "fragment", "overdub", "variant_track", "fx", "minus",
           "update", "set_head", "rebuild_sections", "envelope", "splice", "dsp_apply",
           "pedals", "sound_preset")
PLAN = "X:1\nT:plan\nM:4/4\nL:1/8\nK:C\nV:Vocal\nCDEF GABc|\n"
REQ = "X:1\nT:request\nM:4/4\nL:1/8\nK:Am\nV:Vocal\nABcd efga|\n"


class _RecipeCase(_WorkerApiCase):
    def _mk(self, **kw):
        """Трек в БД со всеми нужными столбцами (без semantic.npy)."""
        row = {"title": "t", "status": "done", "style": "dark rock, male baritone",
               "lyrics": "[verse] la la", "seed": 42, "cot": "full", "arc": "",
               "max_tokens": 0, "temperature": 0.0, "cfg": 0.0, "parent_id": None,
               "role": "", "overdub_of": None, "voice_src": None, "head_id": None,
               "folder": "", "draft": 0, "req_abc": "", "abc_file": "",
               "duration_sec": 60.0, "created_at": "2026-10-01T00:00:00"}
        row.update(kw)
        cols = ", ".join(row)
        marks = ", ".join("?" for _ in row)
        with self._conn() as c:
            return c.execute(f"INSERT INTO jobs ({cols}) VALUES ({marks})",
                             tuple(row.values())).lastrowid

    def _file(self, jid, name, text):
        d = self.jobs_dir / str(jid)
        d.mkdir(parents=True, exist_ok=True)
        (d / name).write_text(text, encoding="utf-8")

    def _post_op(self, jid, op, params=None, result="", refs=None):
        r = self.client.post(f"/jobs/{jid}/ops", json={
            "op": op, "params": params or {}, "result": result, "refs": refs or []})
        self.assertLess(r.status_code, 300, r.text)
        return r.json()["id"]

    def _recipe(self, jid):
        r = self.client.get(f"/jobs/{jid}/recipe")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def _song(self):
        """Корень (с планами) + продолжение и вариант из журнала + fx на варианте."""
        root = self._mk(title="Warm Wire", folder="Альбом", seed=123, arc="calm-build",
                        max_tokens=3000, temperature=0.9, cfg=1.5,
                        abc_file="score.abc", req_abc="request.abc")
        self._file(root, "score.abc", PLAN)
        self._file(root, "request.abc", REQ)
        cont = self._mk(parent_id=root, role="continue", seed=77)
        var = self._mk(parent_id=root, role="variant", voice_src=cont)
        self._post_op(root, "continue", {"from_sec": 30.0, "parent_id": root, "seed": 77},
                      f"job:{cont}")
        self._post_op(root, "variant_track",
                      {"file": "dsp-a.flac", "title": "v", "voice_src": cont}, f"job:{var}")
        self._post_op(var, "fx", {"chain": "warm"}, "dsp-fx1.flac")
        with self._conn() as c:
            c.execute("UPDATE jobs SET head_id=? WHERE id=?", (var, root))
        return root, cont, var


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestJobOps(_RecipeCase):
    """J1: запись и чтение журнала."""

    def test_records_what_was_sent(self):
        jid = self._mk()
        params = {"specs": [{"part": "bass", "gain": -3}], "nested": {"a": [1, 2]}}
        oid = self._post_op(jid, "rebuild_sections", params, "mix-1.flac", [jid])
        r = self.client.get(f"/jobs/{jid}/ops")
        self.assertEqual(r.status_code, 200, r.text)
        ops = r.json()
        self.assertEqual(len(ops), 1)
        o = ops[0]
        self.assertEqual(o["id"], oid)
        self.assertEqual((o["op"], o["params"], o["result"], o["refs"]),
                         ("rebuild_sections", params, "mix-1.flac", [jid]))
        self.assertTrue(o["created_at"])

    def test_order_of_records(self):
        jid = self._mk()
        for op in ("fx", "minus", "update", "set_head"):
            self._post_op(jid, op)
        ops = self.client.get(f"/jobs/{jid}/ops").json()
        self.assertEqual([o["op"] for o in ops], ["fx", "minus", "update", "set_head"])
        ids = [o["id"] for o in ops]
        self.assertEqual(ids, sorted(ids))

    def test_journal_is_per_track(self):
        a, b = self._mk(), self._mk()
        self._post_op(a, "fx")
        self.assertEqual(self.client.get(f"/jobs/{b}/ops").json(), [])

    def test_all_contract_ops_accepted(self):
        jid = self._mk()
        for op in ALL_OPS:
            self._post_op(jid, op)
        self.assertEqual(len(self.client.get(f"/jobs/{jid}/ops").json()), len(ALL_OPS))

    def test_unknown_op_422(self):
        jid = self._mk()
        for op in ("voice_convert", "preview", "", "FX"):
            r = self.client.post(f"/jobs/{jid}/ops",
                                 json={"op": op, "params": {}, "result": "", "refs": []})
            self.assertEqual(r.status_code, 422, op)
        self.assertEqual(self.client.get(f"/jobs/{jid}/ops").json(), [])

    def test_bad_body_422(self):
        jid = self._mk()
        r = self.client.post(f"/jobs/{jid}/ops", content=b"{not json",
                             headers={"Content-Type": "application/json"})
        self.assertEqual(r.status_code, 422)
        for bad in ({"op": "fx", "params": "x", "result": "", "refs": []},
                    {"op": "fx", "params": [1], "result": "", "refs": []},
                    {"op": "fx", "params": {}, "result": "", "refs": ["abc"]}):
            r = self.client.post(f"/jobs/{jid}/ops", json=bad)
            self.assertEqual(r.status_code, 422, bad)
        self.assertEqual(self.client.get(f"/jobs/{jid}/ops").json(), [])

    def test_overdub_without_seed_gets_real_seed(self):
        # сид выбирает воркер и пишет в строку трека — журнал повторит исполнение
        src = self._mk(abc_file="score.abc")
        self._file(src, "score.abc", PLAN)
        for body in ({"style": "warm strings"}, {"style": "warm strings", "seed": None}):
            r = self.client.post(f"/jobs/{src}/overdub", json=body)
            self.assertLess(r.status_code, 300, r.text)
            seed = self._row(r.json()["id"])["seed"]
            self.assertIsNotNone(seed, body)
            self.assertNotEqual(seed, 0, body)
        r = self.client.post(f"/jobs/{src}/overdub", json={"style": "x", "seed": 4242})
        self.assertEqual(self._row(r.json()["id"])["seed"], 4242)  # явный сид — как есть

    def test_missing_track_404(self):
        r = self.client.post("/jobs/9999/ops",
                             json={"op": "fx", "params": {}, "result": "", "refs": []})
        self.assertEqual(r.status_code, 404)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRecipeExport(_RecipeCase):
    """E1: экспорт по журналу; E2: отказы."""

    def test_root_and_header(self):
        root, cont, var = self._song()
        rec = self._recipe(root)
        ver = self.client.get("/version").json()
        self.assertEqual(rec["format"], "yue-track-recipe")
        self.assertEqual(rec["version"], 1)
        self.assertTrue(rec["exported_at"])
        self.assertEqual(rec["worker_model"], ver["model"])
        self.assertEqual(rec["stems_model"], ver["stems_model"])
        self.assertEqual(rec["track"], {"title": "Warm Wire", "folder": "Альбом", "head": "n3"})
        r = rec["root"]
        self.assertEqual(r["id"], "n1")
        expect = {"title": "Warm Wire", "style": "dark rock, male baritone",
                  "lyrics": "[verse] la la", "seed": 123, "cot": "full", "arc": "calm-build",
                  "max_tokens": 3000, "temperature": 0.9, "cfg": 1.5,
                  "plan_abc": PLAN, "abc": REQ}
        for k, v in expect.items():
            self.assertEqual(r[k], v, k)

    def test_ops_by_node_ids(self):
        root, cont, var = self._song()
        rec = self._recipe(root)
        self.assertEqual(rec["ops"], [
            {"op": "continue", "node": "n1",
             "params": {"from_sec": 30.0, "parent_id": "n1", "seed": 77},
             "result": "job:n2", "refs": []},
            {"op": "variant_track", "node": "n1",
             "params": {"file": "dsp-a.flac", "title": "v", "voice_src": "n2"},
             "result": "job:n3", "refs": []},
            {"op": "fx", "node": "n3", "params": {"chain": "warm"},
             "result": "dsp-fx1.flac", "refs": []},
        ])
        self.assertEqual(rec["warnings"], [])

    def test_no_request_abc_is_null(self):
        root = self._mk(abc_file="score.abc")
        self._file(root, "score.abc", PLAN)
        r = self._recipe(root)["root"]
        self.assertEqual(r["plan_abc"], PLAN)
        self.assertIsNone(r["abc"])

    def test_no_plan_is_null(self):
        root = self._mk()
        r = self._recipe(root)["root"]
        self.assertIsNone(r["plan_abc"])
        self.assertIsNone(r["abc"])

    def test_export_from_descendant_takes_song_root(self):
        root, cont, var = self._song()
        a = self._recipe(root)
        a.pop("exported_at")
        for jid in (cont, var):
            b = self._recipe(jid)
            b.pop("exported_at")
            self.assertEqual(a, b, jid)

    def test_export_from_overdub_takes_song_root(self):
        root = self._mk(title="Song")
        od = self._mk(overdub_of=root, role="overdub")
        self._post_op(root, "overdub", {"gain": 0.5}, f"job:{od}")
        rec = self._recipe(od)
        self.assertEqual(rec["root"]["title"], "Song")
        self.assertEqual(rec["ops"][0]["result"], "job:n2")

    def test_foreign_track_record_skipped(self):
        root = self._mk()
        other = self._mk(title="other")
        piece = self._mk(parent_id=root, role="splice")
        self._post_op(root, "splice", {"pieces": [{"job_id": root}, {"job_id": other}]},
                      f"job:{piece}", [root, other])
        self._post_op(root, "update", {"title": "kept"})
        rec = self._recipe(root)
        self.assertEqual([o["op"] for o in rec["ops"]], ["update"])
        self.assertTrue(rec["warnings"])

    def test_child_without_journal_warning(self):
        root, cont, var = self._song()
        self._mk(parent_id=root, role="rebuild")  # версия до журнала
        rec = self._recipe(root)
        self.assertEqual(len(rec["ops"]), 3)
        self.assertNotIn("job:n4", [o["result"] for o in rec["ops"]])
        self.assertTrue(rec["warnings"])

    def test_head_outside_recipe(self):
        root = self._mk()
        old = self._mk(parent_id=root, role="rebuild")
        with self._conn() as c:
            c.execute("UPDATE jobs SET head_id=? WHERE id=?", (old, root))
        rec = self._recipe(root)
        self.assertEqual(rec["track"]["head"], "n1")
        self.assertTrue(rec["warnings"])

    def test_draft_root_422(self):
        root = self._mk(draft=1)
        r = self.client.get(f"/jobs/{root}/recipe")
        self.assertEqual(r.status_code, 422, r.text)
        self.assertTrue(r.json().get("detail"))

    def test_imported_root_422(self):
        root = self._mk(style=IMPORT_STYLE, lyrics="", seed=None)
        r = self.client.get(f"/jobs/{root}/recipe")
        self.assertEqual(r.status_code, 422, r.text)
        self.assertTrue(r.json().get("detail"))

    def test_voice_version_422(self):
        root, cont, var = self._song()
        self._mk(parent_id=var, role="voice")
        r = self.client.get(f"/jobs/{root}/recipe")
        self.assertEqual(r.status_code, 422, r.text)
        self.assertTrue(r.json().get("detail"))

    def test_missing_track_404(self):
        self.assertEqual(self.client.get("/jobs/9999/recipe").status_code, 404)

    def test_export_after_deleted_middle_version(self):
        # корень → V (удалена) → X → W; экспорт от X и W — 422 с номером V,
        # корень не перевыбирается; от корня — 200, warning про удалённую версию
        root = self._mk(title="Song")
        mid = self._mk(parent_id=root, role="continue", title="deleted mid")
        x = self._mk(parent_id=mid, role="continue", title="live X")
        w = self._mk(parent_id=x, role="variant", title="W")
        self._post_op(root, "continue", {"parent_id": root}, f"job:{mid}")
        self._post_op(mid, "continue", {"parent_id": mid}, f"job:{x}")
        self._post_op(x, "variant_track", {"file": "dsp-a.flac"}, f"job:{w}")
        with self._conn() as c:
            c.execute("DELETE FROM jobs WHERE id=?", (mid,))
        for jid in (w, x):
            r = self.client.get(f"/jobs/{jid}/recipe")
            self.assertEqual(r.status_code, 422, (jid, r.text))
            self.assertIn(f"#{mid}", str(r.json().get("detail")), jid)
        rec = self._recipe(root)
        self.assertEqual(rec["root"]["title"], "Song")
        self.assertTrue(any(str(mid) in s for s in rec["warnings"]), rec["warnings"])


def _recipe_doc(title="Warm Wire", fmt="yue-track-recipe", version=1):
    return {"format": fmt, "version": version, "exported_at": "2026-10-11T12:00:00+03:00",
            "worker_model": "yue2-x", "stems_model": "roformer",
            "track": {"title": title, "folder": "Альбом", "head": "n1"},
            "root": {"id": "n1", "title": title, "style": "dark rock", "lyrics": "[verse] la",
                     "seed": 1, "cot": "full", "arc": "", "max_tokens": 0,
                     "temperature": 0, "cfg": 0, "abc": None, "plan_abc": None},
            "ops": [], "warnings": []}


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRecipeImport(_RecipeCase):
    """I1–I3: сессии импорта, захват, сохранение состояния."""

    def _start(self, doc=None, mode="seed"):
        r = self.client.post("/recipes/import", json={"recipe": doc or _recipe_doc(),
                                                      "mode": mode})
        self.assertLess(r.status_code, 300, r.text)
        return r.json()["id"]

    def _claim(self, sid, owner):
        return self.client.post(f"/recipes/imports/{sid}/claim", json={"owner": owner})

    def test_session_created_running(self):
        doc = _recipe_doc(title="Gone")
        sid = self._start(doc, mode="plan")
        lst = self.client.get("/recipes/imports").json()
        s = [x for x in lst if x["id"] == sid]
        self.assertEqual(len(s), 1)
        s = s[0]
        self.assertEqual((s["title"], s["status"], s["mode"]), ("Gone", "running", "plan"))
        self.assertFalse(s["error"])
        self.assertFalse(s["finished_at"])
        self.assertTrue(s["created_at"])
        one = self.client.get(f"/recipes/imports/{sid}")
        self.assertEqual(one.status_code, 200, one.text)
        self.assertEqual(one.json()["recipe"], doc)
        self.assertIn("state", one.json())

    def test_export_roundtrip_accepted(self):
        root, _, _ = self._song()
        sid = self._start(self._recipe(root))
        self.assertEqual(self.client.get(f"/recipes/imports/{sid}").json()["status"], "running")

    def test_foreign_format_or_version_422(self):
        for doc in (_recipe_doc(fmt="other-recipe"), _recipe_doc(version=2),
                    {"hello": "world"}):
            r = self.client.post("/recipes/import", json={"recipe": doc, "mode": "seed"})
            self.assertEqual(r.status_code, 422, doc)
        self.assertEqual(self.client.get("/recipes/imports").json(), [])

    def test_bad_mode_422(self):
        r = self.client.post("/recipes/import", json={"recipe": _recipe_doc(), "mode": "fast"})
        self.assertEqual(r.status_code, 422)

    def test_missing_session_404(self):
        self.assertEqual(self.client.get("/recipes/imports/9999").status_code, 404)
        self.assertEqual(self._claim(9999, "app").status_code, 404)
        self.assertEqual(self.client.patch("/recipes/imports/9999",
                                           json={"progress": "x"}).status_code, 404)

    def test_claim_second_owner_409_same_owner_200(self):
        sid = self._start()
        r = self._claim(sid, "app")
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["id"], sid)
        self.assertEqual(self._claim(sid, "mcp").status_code, 409)
        self.assertEqual(self._claim(sid, "app").status_code, 200)  # продление
        self.assertEqual(self._claim(sid, "mcp").status_code, 409)

    def test_lease_is_ten_minutes(self):
        self.assertEqual(self.w.RECIPE_LEASE_S, 600)

    def test_claim_after_lease_expired(self):
        sid = self._start()
        with mock.patch.object(self.w, "RECIPE_LEASE_S", 2):
            self.assertEqual(self._claim(sid, "app").status_code, 200)
            self.assertEqual(self._claim(sid, "mcp").status_code, 409)
            time.sleep(2.2)
            self.assertEqual(self._claim(sid, "mcp").status_code, 200)
            self.assertEqual(self._claim(sid, "app").status_code, 409)  # теперь держит mcp

    def test_sessions_claimed_independently(self):
        a, b = self._start(), self._start()
        self.assertEqual(self._claim(a, "app").status_code, 200)
        self.assertEqual(self._claim(b, "mcp").status_code, 200)

    def test_patch_saves_state_progress(self):
        sid = self._start()
        state = {"next": 3, "mapping": {"n1": 101, "n2": 102},
                 "files": {"n2|dsp-a.flac": "dsp-b.flac"}, "wait": 102}
        r = self.client.patch(f"/recipes/imports/{sid}",
                              json={"state": state, "progress": "Узел 2 из 5: рендер…",
                                    "root_job_id": 101})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["progress"], "Узел 2 из 5: рендер…")
        s = self.client.get(f"/recipes/imports/{sid}").json()
        self.assertEqual(s["state"], state)
        self.assertEqual(s["progress"], "Узел 2 из 5: рендер…")
        self.assertEqual(s["root_job_id"], 101)
        self.assertEqual(s["status"], "running")
        self.assertFalse(s["finished_at"])

    def test_patch_partial_keeps_other_fields(self):
        sid = self._start()
        self.client.patch(f"/recipes/imports/{sid}", json={"state": {"next": 1}})
        self.client.patch(f"/recipes/imports/{sid}", json={"progress": "шаг 1"})
        s = self.client.get(f"/recipes/imports/{sid}").json()
        self.assertEqual(s["state"], {"next": 1})
        self.assertEqual(s["progress"], "шаг 1")

    def test_status_done_sets_finished_at(self):
        sid = self._start()
        r = self.client.patch(f"/recipes/imports/{sid}", json={"status": "done"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["status"], "done")
        self.assertTrue(r.json()["finished_at"])
        lst = [x for x in self.client.get("/recipes/imports").json() if x["id"] == sid]
        self.assertEqual(lst[0]["status"], "done")
        self.assertTrue(lst[0]["finished_at"])

    def test_status_error_sets_finished_at_and_error(self):
        sid = self._start()
        r = self.client.patch(f"/recipes/imports/{sid}",
                              json={"status": "error", "error": "узел n3: сбой рендера"})
        self.assertEqual(r.status_code, 200, r.text)
        s = r.json()
        self.assertEqual((s["status"], s["error"]), ("error", "узел n3: сбой рендера"))
        self.assertTrue(s["finished_at"])

    def test_patch_by_other_owner_409_state_kept(self):
        # импорт держит A — запись B отвергается, состояние не меняется
        sid = self._start()
        self.assertEqual(self._claim(sid, "A").status_code, 200)
        ok = self.client.patch(f"/recipes/imports/{sid}",
                               json={"owner": "A", "state": {"next": 2}, "progress": "A"})
        self.assertEqual(ok.status_code, 200, ok.text)
        r = self.client.patch(f"/recipes/imports/{sid}",
                              json={"owner": "B", "state": {"next": 9}, "progress": "B"})
        self.assertEqual(r.status_code, 409, r.text)
        s = self.client.get(f"/recipes/imports/{sid}").json()
        self.assertEqual((s["state"], s["progress"]), ({"next": 2}, "A"))
        # без owner — действие человека, проходит
        r = self.client.patch(f"/recipes/imports/{sid}", json={"progress": "человек"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.client.get(f"/recipes/imports/{sid}").json()["progress"],
                         "человек")

    def test_status_dismissed_sets_finished_at(self):
        sid = self._start()
        self.client.patch(f"/recipes/imports/{sid}", json={"status": "error", "error": "x"})
        r = self.client.patch(f"/recipes/imports/{sid}", json={"status": "dismissed"})
        self.assertEqual(r.status_code, 200, r.text)
        s = self.client.get(f"/recipes/imports/{sid}").json()
        self.assertEqual(s["status"], "dismissed")
        self.assertTrue(s["finished_at"])

    def test_bad_status_422(self):
        sid = self._start()
        r = self.client.patch(f"/recipes/imports/{sid}", json={"status": "paused"})
        self.assertEqual(r.status_code, 422)
        self.assertEqual(self.client.get(f"/recipes/imports/{sid}").json()["status"], "running")


if __name__ == "__main__":
    unittest.main()
