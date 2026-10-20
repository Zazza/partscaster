"""Карточка internal-vram-context — экономный режим по видеопамяти, честный потолок длины.

Тест-кейсы (ТК → условие карточки):
ТК1 (усл. 1) economy_seq(model, vram_mib): (видеопамять − база модели − запас 1536) / 0,21875, вниз до кратного
    256, не меньше 8192, не больше 24576. q8 8188 → 18176; q8 6144 → 8704; q8 4096 → 8192; q8 16376 → 24576;
    bf16 8188 → 8960; 0 → 8192; q6/q5 = q8. context_seq: экономный выключен → 24576 при любой карте.
ТК2 (усл. 1) CppServer(binary, model, vae, max_seq): контекст < 24576 → «--max-seq N»; = 24576 → флага нет;
    key() различает контексты; low_vram == (max_seq < 24576). Воркер запускает сервер с контекстом по карте.
ТК3 (усл. 2) song_ceiling = контекст − 2792: 8192 → 5400, 24576 → 21784; cap_budget(budget, потолок) = min;
    build_request/continue_request: semantic_sampling.max_tokens = min(бюджет, потолок).
ТК4 (усл. 2) MAX_SEM_TOKENS = 21784 без YUE2_MAX_TOKENS; job_budget полной песни без длины → 21784;
    YUE2_MAX_TOKENS задан — верхний предел.
ТК5 (усл. 5) GET /config: max_song_sec = потолок / 25, cpp_context — шагов контекста при текущих модели, режиме и
    карте (экономный q8 при 8188 → 18176 и 15384 / 25 = 615,36).
ТК10 (усл. 3, вторая часть) отказ контекста без повтора (остаток < 1000): экономный режим — текст ошибки джобы
    с «экономный режим» и «выключите экономный режим»; полный — «не помещается в память модели», без «экономный режим».
Усл. 4 (авторетрай по фактическому контексту) — в test_gen_truncation_vocal.TestCppTruncationSource.

Внешние границы: nvidia-smi (_gpu_vram_mib воркера), программа yue-server (фейк из test_engine_cpp).
Свои функции воркера и движка не мокаются.
"""
import json
import os
import subprocess
import sys
import unittest
from pathlib import Path
from unittest import mock

from test_cpp_only import _Case as _ConfigCase
from test_cpp_only import _vram
from test_engine_api import _CppCase
from test_gen_truncation_vocal import _GenCase
from test_engine_cpp import (DEFAULT_ABC, _EnvCase, _HAS_NUMPY, _sampling, _ServerCase, fake_starts, job_row,
                             opt_value, synth_bodies)
from test_pure import _HAS_WORKER_DEPS

WORKER_DIR = Path(__file__).resolve().parent
FULL = 24576          # контекст модели (усл. 1)
FULL_CEILING = 21784  # 24576 − 2792 (усл. 2)


# --- ТК1 -------------------------------------------------------------------------------------------

class TestEconomySeq(_EnvCase):

    def test_card_cases(self):
        cases = [("q8", 8188, 18176), ("q8", 6144, 8704), ("q8", 4096, 8192), ("q8", 16376, 24576),
                 ("bf16", 8188, 8960)]
        for model, mib, want in cases:
            with self.subTest(model=model, vram=mib):
                self.assertEqual(self.e.economy_seq(model, mib), want)

    def test_unknown_vram_is_8192(self):
        for model in ("bf16", "q8", "q6", "q5"):
            with self.subTest(model=model):
                self.assertEqual(self.e.economy_seq(model, 0), 8192)

    def test_q6_q5_as_q8(self):
        for mib in (0, 4096, 6144, 8188, 12282, 16376):
            with self.subTest(vram=mib):
                q8 = self.e.economy_seq("q8", mib)
                self.assertEqual(self.e.economy_seq("q6", mib), q8)
                self.assertEqual(self.e.economy_seq("q5", mib), q8)

    def test_bounds_and_step_on_any_card(self):
        """Любая карта: кратно 256, в [8192, 24576]; выше пола — кэш с весами и запасом влезает в карту."""
        base = {"bf16": 4650, "q8": 2660}
        for model in ("bf16", "q8"):
            prev = 0
            for mib in range(0, 20000, 97):
                seq = self.e.economy_seq(model, mib)
                with self.subTest(model=model, vram=mib, seq=seq):
                    self.assertEqual(seq % 256, 0)
                    self.assertGreaterEqual(seq, 8192)
                    self.assertLessEqual(seq, FULL)
                    self.assertGreaterEqual(seq, prev, "больше памяти — контекст не меньше")
                    if seq > 8192:
                        self.assertLessEqual(base[model] + 1536 + seq * 0.21875, mib, "не влезает в карту")
                    if 8192 < seq < FULL:
                        # вниз до кратного 256, но не больше чем на шаг
                        self.assertGreater(base[model] + 1536 + (seq + 256) * 0.21875, mib)
                prev = seq

    def test_context_seq_full_mode_ignores_card(self):
        for model in ("bf16", "q8", "q6", "q5"):
            for mib in (0, 4096, 8188, 24564):
                with self.subTest(model=model, vram=mib):
                    self.assertEqual(self.e.context_seq(model, False, mib), FULL)

    def test_context_seq_economy_is_economy_seq(self):
        for model, mib, want in (("q8", 8188, 18176), ("bf16", 8188, 8960), ("q8", 0, 8192),
                                 ("q8", 16376, FULL)):
            with self.subTest(model=model, vram=mib):
                self.assertEqual(self.e.context_seq(model, True, mib), want)


# --- ТК2 -------------------------------------------------------------------------------------------

class TestCppServerContext(_ServerCase):

    def _srv(self, max_seq):
        s = self.e.CppServer(self.bin, self.model, self.vae, max_seq, [])
        self.addCleanup(s.stop)
        return s

    def test_economy_context_passed(self):
        for seq in (8192, 18176):
            with self.subTest(max_seq=seq):
                s = self._srv(seq)
                s.start()
                argv = fake_starts(self.fake_dir)[-1]["argv"]
                self.assertEqual(opt_value(argv, "--max-seq"), str(seq))
                s.stop()

    def test_full_context_no_flag(self):
        s = self._srv(FULL)
        s.start()
        argv = fake_starts(self.fake_dir)[0]["argv"]
        self.assertNotIn("--max-seq", argv)
        self.assertIsNone(opt_value(argv, "--max-seq"))

    def test_low_vram_property(self):
        self.assertTrue(self._srv(8192).low_vram)
        self.assertTrue(self._srv(18176).low_vram)
        self.assertFalse(self._srv(FULL).low_vram)
        self.assertEqual(self._srv(18176).max_seq, 18176)

    def test_key_differs_by_context(self):
        keys = {self._srv(seq).key() for seq in (8192, 18176, FULL)}
        self.assertEqual(len(keys), 3, "смена контекста не видна в key() — сервер не перезапустится")
        self.assertEqual(self._srv(18176).key(), self._srv(18176).key())


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestWorkerStartsServerByCard(_CppCase):
    """Воркер запускает yue-server с контекстом по видеопамяти карты; заявка не превышает потолок."""

    def _card(self, mib):
        p = mock.patch.object(self.w, "_gpu_vram_mib", lambda: mib)
        p.start()
        self.addCleanup(p.stop)
        p = mock.patch.object(self.w, "MAX_SEM_TOKENS", FULL_CEILING)
        p.start()
        self.addCleanup(p.stop)

    def test_economy_8gb_context(self):
        self._card(8188)
        self._cpp("q8", low_vram=True)
        self._run(self._new())
        self.assertEqual(opt_value(fake_starts(self.fake_dir)[0]["argv"], "--max-seq"), "18176")
        # без длины бюджет — MAX_SEM_TOKENS, серверу уходит потолок контекста 18176 − 2792
        self.assertEqual(synth_bodies(self.fake_dir)[0]["semantic_sampling"]["max_tokens"], 15384)

    def test_economy_unknown_card_8192(self):
        self._card(0)
        self._cpp("q8", low_vram=True)
        self._run(self._new())
        self.assertEqual(opt_value(fake_starts(self.fake_dir)[0]["argv"], "--max-seq"), "8192")
        self.assertEqual(synth_bodies(self.fake_dir)[0]["semantic_sampling"]["max_tokens"], 5400)

    def test_full_mode_no_flag_full_ceiling(self):
        self._card(8188)
        self._cpp("q8", low_vram=False)
        self._run(self._new())
        self.assertIsNone(opt_value(fake_starts(self.fake_dir)[0]["argv"], "--max-seq"))
        self.assertEqual(synth_bodies(self.fake_dir)[0]["semantic_sampling"]["max_tokens"], FULL_CEILING)

    def test_toggle_economy_restarts_with_new_context(self):
        self._card(8188)
        self._cpp("q8", low_vram=False)
        self._run(self._new())
        r = self.client.post("/config", json={"cpp_low_vram": True})
        self.assertEqual(r.status_code, 200, r.text)
        self._run(self._new())
        st = fake_starts(self.fake_dir)
        self.assertEqual(len(st), 2, "смена режима не перезапустила сервер")
        self.assertEqual(opt_value(st[1]["argv"], "--max-seq"), "18176")


# --- ТК3 -------------------------------------------------------------------------------------------

class TestSongCeiling(_EnvCase):

    def test_ceiling(self):
        self.assertEqual(self.e.song_ceiling(8192), 5400)
        self.assertEqual(self.e.song_ceiling(FULL), FULL_CEILING)
        self.assertEqual(self.e.song_ceiling(18176), 15384)

    def test_cap_budget_is_min(self):
        self.assertEqual(self.e.cap_budget(16000, 5400), 5400)
        self.assertEqual(self.e.cap_budget(450, 5400), 450)
        self.assertEqual(self.e.cap_budget(30000, FULL_CEILING), FULL_CEILING)
        self.assertEqual(self.e.cap_budget(5400, 5400), 5400)


@unittest.skipUnless(_HAS_NUMPY, "нужен numpy")
class TestRequestsUseCeiling(_EnvCase):

    def test_build_request_capped(self):
        req = self.e.build_request(job_row(), 16000, 5400)
        self.assertEqual(_sampling(req)["max_tokens"], 5400)

    def test_build_request_below_ceiling_kept(self):
        self.assertEqual(_sampling(self.e.build_request(job_row(), 16000, FULL_CEILING))["max_tokens"], 16000)
        self.assertEqual(_sampling(self.e.build_request(job_row(draft=1), 450, 5400))["max_tokens"], 450)

    def test_build_request_full_ceiling(self):
        req = self.e.build_request(job_row(), 30000, FULL_CEILING)
        self.assertEqual(_sampling(req)["max_tokens"], FULL_CEILING)

    def test_continue_request_capped(self):
        import numpy as np
        d = self.tmp / "jobs" / "3"
        d.mkdir(parents=True)
        np.save(d / "semantic.npy", np.arange(120 * 25, dtype=np.int64))
        (d / "score.abc").write_text(DEFAULT_ABC, encoding="utf-8")
        row = job_row(parent_id=3, role="continue", cont_from=60.0)
        self.assertEqual(_sampling(self.e.continue_request(row, d, 60.0, 16000, 15384))["max_tokens"], 15384)
        self.assertEqual(_sampling(self.e.continue_request(row, d, 60.0, 3000, FULL_CEILING))["max_tokens"], 3000)


# --- ТК4 -------------------------------------------------------------------------------------------

def _max_sem_tokens_on_import(tmp: str, env_value: "str | None") -> int:
    """MAX_SEM_TOKENS воркера, импортированного в отдельном процессе с заданной YUE2_MAX_TOKENS
    (переменная читается при импорте)."""
    env = dict(os.environ, PARTSCASTER_DATA_DIR=tmp, PYTHONPATH=str(WORKER_DIR))
    env.pop("YUE2_MAX_TOKENS", None)
    if env_value is not None:
        env["YUE2_MAX_TOKENS"] = env_value
    out = subprocess.run([sys.executable, "-c", "import yue_worker; print(yue_worker.MAX_SEM_TOKENS)"],
                         cwd=str(WORKER_DIR), env=env, capture_output=True, text=True, timeout=300)
    if out.returncode != 0:
        raise AssertionError(out.stderr[-2000:])
    return int(out.stdout.strip().splitlines()[-1])


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestMaxSemTokens(_ConfigCase):

    def test_default_without_env(self):
        self.assertEqual(_max_sem_tokens_on_import(self._td.name, None), FULL_CEILING)

    def test_env_sets_value(self):
        self.assertEqual(_max_sem_tokens_on_import(self._td.name, "9000"), 9000)

    def test_job_budget_full_song_without_length(self):
        with mock.patch.object(self.w, "MAX_SEM_TOKENS", FULL_CEILING):
            self.assertEqual(self.w.job_budget(False, 0, None, "[verse]\nla la la"), FULL_CEILING)
            self.assertEqual(self.w.job_budget(False, 0, ""), FULL_CEILING)


# --- ТК5 -------------------------------------------------------------------------------------------

@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestConfigSongLength(_ConfigCase):

    def setUp(self):
        super().setUp()
        p = mock.patch.object(self.w, "MAX_SEM_TOKENS", FULL_CEILING)
        p.start()
        self.addCleanup(p.stop)

    def check(self, context, song_sec):
        c = self._cfg()
        self.assertIn("cpp_context", c)
        self.assertIn("max_song_sec", c)
        self.assertIsInstance(c["cpp_context"], int)
        self.assertNotIsInstance(c["cpp_context"], bool)
        self.assertIsInstance(c["max_song_sec"], (int, float))
        self.assertEqual(c["cpp_context"], context)
        self.assertAlmostEqual(c["max_song_sec"], song_sec, places=2)
        return c

    def test_economy_q8_8gb(self):
        _vram(self, 8188)
        self._settings(cpp_model="q8", cpp_low_vram=True)
        self.check(18176, 615.36)

    def test_full_mode(self):
        _vram(self, 8188)
        self._settings(cpp_model="q8", cpp_low_vram=False)
        self.check(FULL, 871.36)

    def test_economy_unknown_card(self):
        _vram(self, 0)
        self._settings(cpp_model="q8", cpp_low_vram=True)
        self.check(8192, 216.0)

    def test_economy_bf16(self):
        _vram(self, 8188)
        self._settings(cpp_model="bf16", cpp_low_vram=True)
        self.check(8960, 246.72)

    def test_env_limit_is_upper_bound(self):
        _vram(self, 8188)
        self._settings(cpp_model="q8", cpp_low_vram=False)
        with mock.patch.object(self.w, "MAX_SEM_TOKENS", 10000):
            self.check(FULL, 400.0)
        with mock.patch.object(self.w, "MAX_SEM_TOKENS", 30000):
            self.check(FULL, 871.36)   # предел, а не замена потолка контекста

    def test_follows_settings_change(self):
        _vram(self, 8188)
        self._fake()
        from test_engine_cpp import place_model
        place_model(self.models, self.e, "q8")
        self._settings(cpp_model="q8", cpp_low_vram=False)
        self.check(FULL, 871.36)
        r = self.client.post("/config", json={"cpp_low_vram": True})
        self.assertEqual(r.status_code, 200, r.text)
        self.check(18176, 615.36)


# --- ТК10 ------------------------------------------------------------------------------------------

def _overflow_line(prefix: int, budget: int, context: int) -> str:
    """Строка журнала yue-server при заявке, не помещающейся в контекст (формат — test_engine_cpp)."""
    return f"[AR] FATAL: prefix {prefix} + budget {budget} + end exceeds context {context}"


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestContextOverflowErrorByMode(_GenCase):
    """ТК10: сервер отказал «не помещается в контекст», остаток контекста меньше 1000 — повтора нет, джоба падает
    с текстом по режиму. Граница — фейк yue-server (отказ с числами в журнале) и nvidia-smi."""

    def _card(self, mib):
        p = mock.patch.object(self.w, "_gpu_vram_mib", lambda: mib)
        p.start()
        self.addCleanup(p.stop)

    def _fail_overflow(self, low_vram, mib, prefix, context):
        self._card(mib)
        self.settings.write_text(json.dumps({"cpp_model": "q8", "cpp_low_vram": low_vram}))
        budget = min(16000, context - 2792)
        self._scenario(mode="failed", log_lines=[_overflow_line(prefix, budget, context)])
        jid = self._run()   # песня со словами — без заявки «только план»
        got = self._status(jid)
        self.assertEqual(got["status"], "error", got)
        self.assertEqual(len(self._budgets()), 1, "остаток < 1000 — повтора быть не должно")
        return got.get("error") or ""

    def test_economy_mode_error_advises_turning_off(self):
        err = self._fail_overflow(True, 0, prefix=7900, context=8192)   # остаток 292
        self.assertIn("экономный режим", err)
        self.assertIn("выключите экономный режим", err)
        self.assertIn("не помещается в память модели", err)

    def test_economy_big_card_same_text(self):
        err = self._fail_overflow(True, 8188, prefix=17800, context=18176)   # остаток 376
        self.assertIn("выключите экономный режим", err)

    def test_full_mode_error_without_economy_advice(self):
        err = self._fail_overflow(False, 8188, prefix=24000, context=FULL)   # остаток 576
        self.assertIn("не помещается в память модели", err)
        self.assertNotIn("экономный режим", err)
        self.assertNotIn("экономн", err.lower())


if __name__ == "__main__":
    unittest.main()
