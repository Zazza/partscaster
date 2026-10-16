"""Спецификация хода операции очереди и родителя трека (задача internal-preset-progress,
условия 2 и 5 карточки, тест-кейсы «Воркер»).

Что должно работать:
- POST /queue/ops/{id}/beat принимает необязательное тело {stage, pass, eta_s}:
  stage — prepare | parts | levels | render | verify | fix (иначе 422), pass ≥ 1,
  eta_s ≥ 0 или null. Без тела — как раньше: только «жива», ход не трогается.
- GET /queue отдаёт у записи stage, pass, eta_s и stage_since (сек с начала шага);
  у записи без хода — пусто/null.
- PATCH /jobs/{id} {parent_id}: существующий трек, не сам себе — сохраняется; иначе 422,
  и трек не меняется.

Допущения (в карточке не заданы, выбраны самые простые):
- pass < 1 и eta_s < 0 — 422, как неверный stage; запись не меняется.
- Повторный бит с тем же stage и pass — тот же шаг: stage_since не сбрасывается
  (клиент повторяет ход с каждым обычным битом); смена stage — новый шаг, отсчёт с нуля.
- Бит без тела оставляет последний ход как есть.

Часы очереди для stage_since — внедряемые (FakeClock, через конструктор OpQueue).
"""
import unittest
from unittest import mock

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase
from test_queue_api import _QueueMixin

_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"
STAGES = ("prepare", "parts", "levels", "render", "verify", "fix")


class FakeClock:
    def __init__(self, t=1000.0):
        self.t = t

    def __call__(self):
        return self.t

    def advance(self, dt):
        self.t += dt


class _Case(_QueueMixin, _WorkerApiCase):
    def setUp(self):
        super().setUp()
        self._setup_queue()

    def _beat(self, op_id, body=None):
        if body is None:
            return self.client.post(f"/queue/ops/{op_id}/beat")
        return self.client.post(f"/queue/ops/{op_id}/beat", json=body)

    def _ok_beat(self, op_id, body=None):
        r = self._beat(op_id, body)
        self.assertEqual(r.status_code, 200, (body, r.text))
        return r.json()


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestBeatProgress(_Case):
    def test_beat_with_body_stored_and_listed(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "render", "pass": 2, "eta_s": 80.5})
        it = self._item(a)
        self.assertEqual(it["stage"], "render")
        self.assertEqual(it["pass"], 2)
        self.assertAlmostEqual(it["eta_s"], 80.5)
        self.assertIsInstance(it["stage_since"], (int, float))
        self.assertGreaterEqual(it["stage_since"], 0)
        self.assertLess(it["stage_since"], 30, "шаг только что начался")

    def test_all_six_stages_accepted(self):
        a = self._holder()
        for st in STAGES:
            self._ok_beat(a, {"stage": st, "pass": 1, "eta_s": None})
            self.assertEqual(self._item(a)["stage"], st)

    def test_eta_null_listed_as_null(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "verify", "pass": 1, "eta_s": None})
        it = self._item(a)
        self.assertEqual((it["stage"], it["pass"]), ("verify", 1))
        self.assertIsNone(it["eta_s"])

    def test_eta_zero_is_a_value(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "fix", "pass": 2, "eta_s": 0})
        self.assertEqual(self._item(a)["eta_s"], 0)

    def test_record_without_progress_is_empty(self):
        a = self._holder()
        self._ok_beat(a)  # прежний бит без тела
        it = self._item(a)
        for k in ("stage", "pass", "eta_s", "stage_since"):
            self.assertIn(k, it, f"запись очереди без поля {k}")
        self.assertFalse(it["stage"], it)
        self.assertIn(it["pass"], (None, 0), it)
        self.assertIsNone(it["eta_s"], it)
        self.assertIsNone(it["stage_since"], it)

    def test_beat_without_body_keeps_old_behaviour(self):
        a = self._holder()
        d = self._ok_beat(a)
        self.assertEqual(d["state"], "running")
        self.assertIs(d["cancel_requested"], False)

    def test_beat_without_body_keeps_last_progress(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "render", "pass": 1, "eta_s": None})
        self._ok_beat(a)
        it = self._item(a)
        self.assertEqual((it["stage"], it["pass"]), ("render", 1))

    def test_unknown_stage_422_and_unchanged(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "parts", "pass": 1, "eta_s": None})
        for bad in ("mixing", "", "RENDER", "render "):
            r = self._beat(a, {"stage": bad, "pass": 1, "eta_s": None})
            self.assertEqual(r.status_code, 422, (bad, r.text))
        self.assertEqual(self._item(a)["stage"], "parts")

    def test_pass_below_one_422(self):
        a = self._holder()
        for bad in (0, -1):
            r = self._beat(a, {"stage": "render", "pass": bad, "eta_s": None})
            self.assertEqual(r.status_code, 422, (bad, r.text))
        self.assertFalse(self._item(a)["stage"])

    def test_negative_eta_422(self):
        a = self._holder()
        r = self._beat(a, {"stage": "render", "pass": 1, "eta_s": -1})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertFalse(self._item(a)["stage"])

    def test_unknown_op_with_body_404(self):
        r = self._beat(987654, {"stage": "render", "pass": 1, "eta_s": None})
        self.assertEqual(r.status_code, 404, r.text)


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestStageSince(_Case):
    """stage_since — секунды с начала текущего шага по часам очереди."""

    def setUp(self):
        super().setUp()
        import opqueue
        self.clock = FakeClock()
        p = mock.patch.object(self.w, "QUEUE", opqueue.OpQueue(clock=self.clock))
        p.start()
        self.addCleanup(p.stop)

    def test_counts_from_step_start(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "prepare", "pass": 1, "eta_s": None})
        self.assertAlmostEqual(self._item(a)["stage_since"], 0, delta=0.5)
        self.clock.advance(7)
        self.assertAlmostEqual(self._item(a)["stage_since"], 7, delta=0.5)

    def test_same_step_repeated_does_not_reset(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "render", "pass": 1, "eta_s": None})
        self.clock.advance(5)
        self._ok_beat(a, {"stage": "render", "pass": 1, "eta_s": None})  # повтор с обычным битом
        self.clock.advance(4)
        self.assertAlmostEqual(self._item(a)["stage_since"], 9, delta=0.5)

    def test_bodyless_beat_does_not_reset(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "levels", "pass": 1, "eta_s": None})
        self.clock.advance(6)
        self._ok_beat(a)
        self.assertAlmostEqual(self._item(a)["stage_since"], 6, delta=0.5)

    def test_new_stage_resets(self):
        a = self._holder()
        self._ok_beat(a, {"stage": "render", "pass": 1, "eta_s": None})
        self.clock.advance(30)
        self._ok_beat(a, {"stage": "verify", "pass": 1, "eta_s": None})
        self.clock.advance(3)
        it = self._item(a)
        self.assertEqual(it["stage"], "verify")
        self.assertAlmostEqual(it["stage_since"], 3, delta=0.5)


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestPatchJobParent(_WorkerApiCase):
    """PATCH /jobs/{id} {parent_id}: промежуточные версии пресета — под итоговой."""

    def test_valid_parent_saved(self):
        final = self._job(semantic=False)
        inter = self._job(semantic=False)
        r = self.client.patch(f"/jobs/{inter}", json={"parent_id": final})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(r.json()["parent_id"], final)
        self.assertEqual(self._row(inter)["parent_id"], final)

    def test_replaces_previous_parent(self):
        src = self._job(semantic=False)
        final = self._job(semantic=False, parent_id=src)
        inter = self._job(semantic=False, parent_id=src)
        r = self.client.patch(f"/jobs/{inter}", json={"parent_id": final})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._row(inter)["parent_id"], final)
        self.assertEqual(self._row(final)["parent_id"], src, "итоговая не тронута")

    def test_self_parent_422(self):
        src = self._job(semantic=False)
        jid = self._job(semantic=False, parent_id=src)
        r = self.client.patch(f"/jobs/{jid}", json={"parent_id": jid})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertEqual(self._row(jid)["parent_id"], src)

    def test_missing_parent_422(self):
        jid = self._job(semantic=False)
        r = self.client.patch(f"/jobs/{jid}", json={"parent_id": 987654})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertIsNone(self._row(jid)["parent_id"])

    def test_failed_parent_does_not_apply_title(self):
        jid = self._job(semantic=False)
        r = self.client.patch(f"/jobs/{jid}", json={"title": "Новое", "parent_id": jid})
        self.assertEqual(r.status_code, 422, r.text)
        self.assertEqual(self._row(jid)["title"], "t", "неуспешный запрос ничего не меняет")

    def test_title_patch_keeps_parent(self):
        src = self._job(semantic=False)
        jid = self._job(semantic=False, parent_id=src)
        r = self.client.patch(f"/jobs/{jid}", json={"title": "Имя"})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self._row(jid)["parent_id"], src)


if __name__ == "__main__":
    unittest.main()
