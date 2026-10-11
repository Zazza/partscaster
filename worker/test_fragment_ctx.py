"""Спецификация «честный „проверить кусок“» (карточка internal-own-track, условие 123, ТК149).

POST /jobs/{id}/continue с to_sec — кусок вместо продолжения до конца песни:
  - новая джоба role='fragment', cont_from=from_sec, max_tokens = round((to_sec + 2) · 25)
    (хвост FRAGMENT_TAIL_S = 2 с, 25 семантических токенов в секунду);
  - to_sec ≤ from_sec → 422, джоба не создаётся; to_sec за длиной трека + хвост → 4xx или обрезка;
  - to_sec не число (NaN/inf) → 422, не 500 (уточнение 123г, ТК154);
  - без to_sec — как раньше: role='continue', max_tokens 0;
  - у родителя без шагов модели (нет semantic.npy) — 4xx, как у «продолжить».
_run_job:
  - строка role='fragment' с cont_from идёт через _continue_song;
  - старый фрагмент без cont_from — обычный вызов пайплайна, _continue_song не зовётся.
Бюджет внутри _continue_song (max_tokens − k, ≥ 200) снаружи не наблюдаем без библиотеки yue2 —
тестом не покрыт (см. отчёт тест-автора).
Кросс-ревью s15 r1 (123д, 123з, ТК155):
  - бюджет куска — без общего минимума 500: в request["semantic_sampling"].max_tokens у фрагмента
    уходит max(200, max_tokens) (400 → 400); у обычной джобы минимум 500 остаётся (400 → 500);
  - название куска — с обрезанным концом (to_sec за концом трека → в названии длина трека).

Написаны по карточке, без чтения реализации.
Запуск из каталога worker: python -m unittest test_fragment_ctx -v
"""
import sqlite3
import sys
import types
import unittest

from test_character import _RecordingModel, _Sampling
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
        # уточнение 123г (ТК154): to_sec не число (NaN/inf) → 422, не 500; джоба не создаётся.
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


class _FragmentRunCase(_WorkerApiCase):
    """Общая подготовка _run_job: заглушки yue2/torch и пайплайн-самописец."""

    def setUp(self):
        super().setUp()
        from unittest import mock
        self.mock = mock
        protocol = types.ModuleType("yue2.protocol")
        protocol.Sampling = _Sampling
        mods = {"yue2": mock.MagicMock(), "yue2.protocol": protocol,
                "yue2.pipeline": mock.MagicMock(), "torch": mock.MagicMock()}
        p = mock.patch.dict(sys.modules, mods)
        p.start()
        self.addCleanup(p.stop)
        self.model = _RecordingModel()
        for name, val in (("_pipe", self.model), ("_load_error", None)):
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)
        p = mock.patch.object(self.w, "_get_pipe", lambda: self.model)
        p.start()
        self.addCleanup(p.stop)

    def _fake_continue(self):
        calls = []

        def fake(*a, **kw):
            calls.append((a, kw))
            raise RuntimeError("stop after _continue_song")
        p = self.mock.patch.object(self.w, "_continue_song", fake)
        p.start()
        self.addCleanup(p.stop)
        return calls

    def _fragment_via_api(self, from_sec, to_sec):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": from_sec, "to_sec": to_sec})
        self.assertLess(r.status_code, 300, r.text)
        return self._children(parent)[0]["id"]

    def _pipeline_generations(self):
        return [kw for _, kw in self.model.calls if "semantic_sampling" in kw or "cfg_scale" in kw]


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestFragmentRun(_FragmentRunCase):
    """_run_job: фрагмент с cont_from — через _continue_song; без cont_from — обычный пайплайн."""

    def test_fragment_with_cont_from_uses_continue_song(self):
        calls = self._fake_continue()
        jid = self._fragment_via_api(10.0, 20.0)
        self.w._run_job(jid)  # исключение наружу = провал теста
        self.assertEqual(len(calls), 1, "фрагмент с cont_from не пошёл через _continue_song")
        args = list(calls[0][0]) + list(calls[0][1].values())
        rows = [a for a in args if isinstance(a, (sqlite3.Row, dict)) and "role" in a.keys()]
        self.assertTrue(rows, f"строка джобы не передана в _continue_song: {args}")
        self.assertEqual(rows[0]["id"], jid)
        self.assertEqual(rows[0]["role"], "fragment")
        self.assertAlmostEqual(rows[0]["cont_from"], 10.0)
        self.assertEqual(self._pipeline_generations(), [], "кусок сгенерирован заново с нуля")

    def test_fragment_without_cont_from_uses_pipeline(self):
        calls = self._fake_continue()
        parent = self._job(duration=60.0, semantic=False)
        with self._conn() as c:
            cur = c.execute(
                "INSERT INTO jobs (title, status, style, lyrics, duration_sec, parent_id, role, created_at) "
                "VALUES ('кусок', 'queued', 'dark rock', '[verse] la la', 10, ?, 'fragment', "
                "'2026-01-01T00:00:00')", (parent,))
            jid = cur.lastrowid
        self.w._run_job(jid)  # исключение наружу = провал теста
        self.assertEqual(calls, [], "старый фрагмент без cont_from ушёл в _continue_song")
        self.assertTrue(self._pipeline_generations(), f"пайплайн не вызван: {self.model.calls}")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestFragmentReviewR1(_FragmentRunCase):
    """Кросс-ревью s15 r1: бюджет куска (123д) и название с обрезанным концом (123з)."""

    def _capture_request(self):
        requests = []

        def fake(pipe, row, request):
            requests.append(request)
            raise RuntimeError("stop after _continue_song")
        p = self.mock.patch.object(self.w, "_continue_song", fake)
        p.start()
        self.addCleanup(p.stop)
        return requests

    def test_fragment_budget_400_not_raised_to_500(self):
        requests = self._capture_request()
        jid = self._fragment_via_api(5.0, 14.0)   # (14 + 2) · 25 = 400
        self.assertEqual(self._row(jid)["max_tokens"], 400)
        self.w._run_job(jid)  # исключение наружу = провал теста
        self.assertEqual(len(requests), 1, "фрагмент не пошёл через _continue_song")
        s = requests[0]["semantic_sampling"]
        self.assertEqual(s.max_tokens, 400, "короткому куску поднят бюджет до общего минимума")

    def test_regular_job_budget_400_still_500(self):
        calls = self._fake_continue()
        with self._conn() as c:
            jid = c.execute(
                "INSERT INTO jobs (title, status, style, lyrics, duration_sec, max_tokens, created_at) "
                "VALUES ('обычная', 'queued', 'dark rock', '[verse] la la', 16, 400, "
                "'2026-01-01T00:00:00')").lastrowid
        self.w._run_job(jid)  # исключение наружу = провал теста
        self.assertEqual(calls, [])
        gens = self._pipeline_generations()
        self.assertTrue(gens, f"пайплайн не вызван: {self.model.calls}")
        self.assertEqual(gens[0]["semantic_sampling"].max_tokens, 500)

    def test_title_has_clamped_end(self):
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 50.0, "to_sec": 90.0})
        self.assertLess(r.status_code, 300, r.text)
        title = self._children(parent)[0]["title"]
        self.assertIn("60", title)
        self.assertNotIn("90", title)


if __name__ == "__main__":
    unittest.main()
