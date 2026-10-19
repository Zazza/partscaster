"""Драматургия поверх прикреплённого ABC».

A1. POST /jobs с непустым abc и arc ∈ {build, wave, burst} → 200: джоба создана,
    arc сохранён в строке, req_abc='request.abc', jobs/<id>/request.abc —
    ровно присланный abc.
A2. Генерация такой джобы: заявка серверу yue2.cpp получает abc == arc.apply_arc(abc, arc)
    и style == arc.style_with_arc(style, arc); request.abc на диске не меняется
    (файл пользователя не трогаем); свой план («только план») не строится.
A3. abc без arc → заявка получает исходный abc и исходный style.
A4. arc с cot="off" — 422 (драматургии нужен план), с abc и без.
"""
import unittest

import arc as arcmod
from test_character import _job_id, _post_job
from test_jobs_lifecycle import _Case as _CppCase
from test_jobs_lifecycle import plan_bodies
from test_pure import ABC, _HAS_WORKER_DEPS, _WorkerApiCase

ARCS = ("build", "wave", "burst")
STYLE = "dark rock"  # стиль по умолчанию в _post_job


def _count_jobs(case):
    with case._conn() as c:
        return c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestArcAbcSubmit(_WorkerApiCase):
    """A1, A4: приём POST /jobs."""

    def test_a1_abc_with_arc_accepted_and_stored(self):
        for a in ARCS:
            with self.subTest(arc=a):
                jid = _job_id(self, _post_job(self, abc=ABC, arc=a))
                row = self._row(jid)
                self.assertEqual(row["arc"], a)
                self.assertEqual(row["req_abc"], "request.abc")
                f = self.jobs_dir / str(jid) / "request.abc"
                self.assertTrue(f.exists(), f)
                self.assertEqual(f.read_text(), ABC)

    def test_a4_arc_with_cot_off_422_without_abc(self):
        for a in ARCS:
            with self.subTest(arc=a):
                before = _count_jobs(self)
                r = _post_job(self, arc=a, cot="off")
                self.assertEqual(r.status_code, 422, r.text)
                self.assertEqual(_count_jobs(self), before)

    def test_a4_control_cot_off_without_arc_accepted(self):
        # контроль: 422 выше — из-за arc, а не из-за самого cot="off"
        _job_id(self, _post_job(self, cot="off"))

    def test_a4_arc_with_cot_off_422_with_abc(self):
        for a in ARCS:
            with self.subTest(arc=a):
                before = _count_jobs(self)
                r = _post_job(self, abc=ABC, arc=a, cot="off")
                self.assertEqual(r.status_code, 422, r.text)
                self.assertEqual(_count_jobs(self), before)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestArcAbcGeneration(_CppCase):
    """Что получает сервер yue2.cpp (фейк yue-server) при генерации джобы с abc."""

    def _run(self, **extra):
        before = len(self._bodies())
        jid = _job_id(self, _post_job(self, **extra))
        self.w._run_job(jid)  # исключение наружу = провал теста
        bodies = self._bodies()[before:]
        self.assertEqual(len(bodies), 1, "заявка трека не дошла до yue2.cpp")
        self.assertEqual(self._row(jid)["status"], "done", self._row(jid)["error"])
        return jid, bodies[0]

    def test_a2_server_gets_arc_applied_abc_and_style(self):
        for a in ARCS:
            with self.subTest(arc=a):
                want_abc = arcmod.apply_arc(ABC, a)
                self.assertNotEqual(want_abc, ABC, "пример ABC не чувствителен к дуге")
                want_style = arcmod.style_with_arc(STYLE, a)
                self.assertNotEqual(want_style, STYLE)
                _, body = self._run(abc=ABC, arc=a)
                self.assertEqual(body.get("abc"), want_abc)
                self.assertEqual(body["style"], want_style, "в заявку ушёл стиль без дуги")
        self.assertEqual(plan_bodies(self.fake_dir), [], "при прикреплённом плане свой не строится")

    def test_a2_request_abc_file_unchanged(self):
        for a in ARCS:
            with self.subTest(arc=a):
                jid, _ = self._run(abc=ABC, arc=a)
                f = self.jobs_dir / str(jid) / "request.abc"
                self.assertEqual(f.read_text(), ABC)

    def test_a3_abc_without_arc_passed_as_is(self):
        for extra in ({}, {"arc": ""}):
            with self.subTest(**extra):
                _, body = self._run(abc=ABC, **extra)
                self.assertEqual(body.get("abc"), ABC)
                self.assertEqual(body["style"], STYLE)


if __name__ == "__main__":
    unittest.main()
