"""«Проверить кусок».

POST /jobs/{id}/continue с to_sec — кусок вместо продолжения до конца песни:
  - новая джоба role='fragment', cont_from=from_sec, max_tokens = round((to_sec + 2) · 25)
    (хвост FRAGMENT_TAIL_S = 2 с, 25 семантических токенов в секунду);
  - to_sec ≤ from_sec → 422, джоба не создаётся; to_sec за длиной трека + хвост → 4xx или обрезка;
  - to_sec не число (NaN/inf) → 422, не 500;
  - без to_sec — role='continue', max_tokens 0;
  - у родителя без шагов модели (нет semantic.npy) — 4xx, как у «продолжить».
_run_job (заявка серверу yue2.cpp — engine_cpp.continue_request):
  - строка role='fragment' с cont_from — продолжение: ноты родителя (его semantic.npy) уходят
    как сыгранные (source_tokens), отметка — source_end = cont_from;
  - старый фрагмент без cont_from — обычная заявка трека с нуля, без нот родителя;
  - бюджет куска — без общего минимума полного трека: semantic_sampling.max_tokens у фрагмента =
    max(max_tokens, шагов до отметки + 200) — сыгранное входит в потолок, новых шагов не меньше
    200 (400 при отметке 5 с → 400; отметка 10 с, конец 11 с → 450); у обычной джобы минимум
    полного трека остаётся (400 → 6000);
  - название куска — с обрезанным концом (to_sec за концом трека → в названии длина трека).
Программа yue-server — фейк (харнесс test_jobs_lifecycle).
"""
import unittest

from test_engine_cpp import _tokens as _sent_tokens
from test_jobs_lifecycle import _Case as _CppCase
from test_pure import _HAS_WORKER_DEPS, _SEM_TOK_PER_SEC_SPEC, _WorkerApiCase

TAIL_S = 2  # спецификация: хвост куска FRAGMENT_TAIL_S


def _tokens(to_sec):
    return round((to_sec + TAIL_S) * _SEM_TOK_PER_SEC_SPEC)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestFragmentSubmit(_WorkerApiCase):
    """POST /jobs/{id}/continue с to_sec и без него."""

    def _count(self):
        with self._conn() as c:
            return c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]

    def test_to_sec_creates_fragment(self):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 10.0, "to_sec": 17.37})
        self.assertLess(r.status_code, 300, r.text)
        kids = self._children(parent)
        self.assertEqual(len(kids), 1)
        k = kids[0]
        self.assertEqual(k["role"], "fragment")
        self.assertAlmostEqual(k["cont_from"], 10.0)
        self.assertEqual(k["max_tokens"], _tokens(17.37))   # 19.37 · 25 = 484.25 → 484
        self.assertEqual(k["max_tokens"], 484)
        data = r.json()
        self.assertEqual(int(data.get("id", data.get("job_id"))), k["id"])

    def test_max_tokens_whole_seconds(self):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 12.5, "to_sec": 20})
        self.assertLess(r.status_code, 300, r.text)
        self.assertEqual(self._children(parent)[0]["max_tokens"], 550)  # (20 + 2) · 25

    def test_to_not_after_from_422(self):
        parent = self._job(duration=60.0)
        before = self._count()
        for to in (10.0, 9.0, 0):
            r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 10.0, "to_sec": to})
            self.assertEqual(r.status_code, 422, (to, r.text))
        self.assertEqual(self._count(), before)
        self.assertEqual(self._children(parent), [])

    def test_to_beyond_length_plus_tail_not_beyond_budget(self):
        # за длиной трека + хвост: 4xx или обрезка — бюджет не больше (длина + хвост) · 25
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 50.0, "to_sec": 60.0 + TAIL_S + 100})
        self.assertLess(r.status_code, 500, r.text)
        if r.status_code < 300:
            k = self._children(parent)[0]
            self.assertEqual(k["role"], "fragment")
            self.assertLessEqual(k["max_tokens"], _tokens(60.0))  # (60 + 2) · 25
            self.assertGreater(k["max_tokens"], round(50.0 * _SEM_TOK_PER_SEC_SPEC))
        else:
            self.assertTrue(400 <= r.status_code < 500, (r.status_code, r.text))
            self.assertEqual(self._children(parent), [])

    def test_to_at_track_end_ok(self):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 50.0, "to_sec": 60.0})
        self.assertLess(r.status_code, 300, r.text)
        self.assertEqual(self._children(parent)[0]["role"], "fragment")

    def test_parent_without_semantic_4xx(self):
        parent = self._job(duration=60.0, semantic=False)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 10.0, "to_sec": 20.0})
        self.assertTrue(400 <= r.status_code < 500, (r.status_code, r.text))
        self.assertEqual(self._children(parent), [])

    def test_to_sec_not_finite_422(self):
        # to_sec не число (NaN/inf) → 422, не 500; джоба не создаётся.
        # json.dumps не пишет NaN по стандарту JSON — тело строкой, как его могла бы прислать «кривая» сторона
        parent = self._job(duration=60.0)
        before = self._count()
        for raw in ("NaN", "Infinity", "-Infinity"):
            r = self.client.post(f"/jobs/{parent}/continue",
                                 content='{"from_sec": 5, "to_sec": ' + raw + "}",
                                 headers={"Content-Type": "application/json"})
            self.assertEqual(r.status_code, 422, (raw, r.status_code, r.text))
        self.assertEqual(self._count(), before)
        self.assertEqual(self._children(parent), [])

    def test_parent_not_found_404(self):
        r = self.client.post("/jobs/9999/continue", json={"from_sec": 10.0, "to_sec": 20.0})
        self.assertEqual(r.status_code, 404)


class _FragmentRunCase(_CppCase):
    """Общая подготовка _run_job: фейк yue-server с моделью."""

    def _fragment_via_api(self, from_sec, to_sec):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": from_sec, "to_sec": to_sec})
        self.assertLess(r.status_code, 300, r.text)
        return parent, self._children(parent)[0]["id"]

    def _render(self, jid):
        """_run_job → единственная заявка трека серверу."""
        self.w._run_job(jid)  # исключение наружу = провал теста
        got = self._row(jid)
        self.assertEqual(got["status"], "done", got["error"])
        bodies = self._bodies()
        self.assertEqual(len(bodies), 1, "заявка трека не дошла до yue2.cpp")
        return bodies[0]


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestFragmentRun(_FragmentRunCase):
    """_run_job: фрагмент с cont_from — продолжение по нотам родителя; без cont_from — с нуля."""

    def test_fragment_with_cont_from_continues_parent(self):
        import numpy as np
        parent, jid = self._fragment_via_api(10.0, 20.0)
        body = self._render(jid)
        self.assertIn("source_tokens", body, "кусок сгенерирован заново с нуля")
        want = np.load(self.jobs_dir / str(parent) / "semantic.npy").tolist()
        self.assertEqual(_sent_tokens(body["source_tokens"]), want, "ноты родителя не дошли до сервера")
        self.assertAlmostEqual(float(body["source_end"]), 10.0)

    def test_fragment_without_cont_from_from_scratch(self):
        parent = self._job(duration=60.0, semantic=False)
        with self._conn() as c:
            cur = c.execute(
                "INSERT INTO jobs (title, status, style, lyrics, duration_sec, parent_id, role, created_at) "
                "VALUES ('кусок', 'queued', 'dark rock', '[verse] la la', 10, ?, 'fragment', "
                "'2026-01-01T00:00:00')", (parent,))
            jid = cur.lastrowid
        body = self._render(jid)
        self.assertNotIn("source_tokens", body, "старый фрагмент без cont_from ушёл в продолжение")
        self.assertNotIn("source_end", body)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestFragmentReviewR1(_FragmentRunCase):
    """Бюджет куска и название с обрезанным концом."""

    def test_fragment_budget_400_not_raised_to_full_minimum(self):
        _, jid = self._fragment_via_api(5.0, 14.0)   # (14 + 2) · 25 = 400
        self.assertEqual(self._row(jid)["max_tokens"], 400)
        body = self._render(jid)
        self.assertIn("source_tokens", body, "фрагмент не пошёл продолжением")
        self.assertEqual(body["semantic_sampling"]["max_tokens"], 400,
                         "короткому куску поднят бюджет до общего минимума")

    def test_fragment_at_least_200_new_steps(self):
        _, jid = self._fragment_via_api(10.0, 11.0)   # (11 + 2) · 25 = 325 < 250 сыгранных + 200
        self.assertEqual(self._row(jid)["max_tokens"], 325)
        body = self._render(jid)
        self.assertEqual(body["semantic_sampling"]["max_tokens"], 450)

    def test_regular_job_budget_400_raised_to_full_minimum(self):
        with self._conn() as c:
            jid = c.execute(
                "INSERT INTO jobs (title, status, style, lyrics, duration_sec, max_tokens, created_at) "
                "VALUES ('обычная', 'queued', 'dark rock', '[verse] la la', 16, 400, "
                "'2026-01-01T00:00:00')").lastrowid
        body = self._render(jid)
        self.assertNotIn("source_tokens", body)
        # полный трек — не меньше 6000: ниже модель обрывает трек до финала
        self.assertEqual(body["semantic_sampling"]["max_tokens"], 6000)

    def test_title_has_clamped_end(self):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 50.0, "to_sec": 90.0})
        self.assertLess(r.status_code, 300, r.text)
        title = self._children(parent)[0]["title"]
        self.assertIn("60", title)
        self.assertNotIn("90", title)


if __name__ == "__main__":
    unittest.main()
