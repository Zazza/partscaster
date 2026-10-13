"""Спецификация YUE2_BACKEND (условие 7 карточки): переменная окружения
читается при загрузке модели и уходит в YuE2Pipeline.from_pretrained(backend=…);
не задана — "torch"; допустимы torch | torch-eager | vllm, иное — понятная
ошибка 503 с именем переменной, модель не грузится.
"""
import os
import sys
import types
import unittest
from unittest import mock

from test_pure import _HAS_WORKER_DEPS


class _FakePipe:
    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def _fake_yue2():
    mod = types.ModuleType("yue2")
    pipeline = mock.MagicMock(name="YuE2Pipeline")
    pipeline.from_pretrained.return_value = _FakePipe()
    mod.YuE2Pipeline = pipeline
    return mod, pipeline.from_pretrained


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestBackendEnv(unittest.TestCase):
    def setUp(self):
        import yue_worker
        self.w = yue_worker
        self.mod, self.from_pretrained = _fake_yue2()
        patches = [
            mock.patch.dict(sys.modules, {"yue2": self.mod}),
            mock.patch.object(yue_worker, "_stop_cpp", lambda: None),
        ]
        for p in patches:
            p.start()
            self.addCleanup(p.stop)
        self._reset()
        self.addCleanup(self._reset)
        env = mock.patch.dict(os.environ)
        env.start()
        self.addCleanup(env.stop)
        os.environ.pop("YUE2_BACKEND", None)

    def _reset(self):
        self.w._pipe = None
        self.w._load_error = None

    def _assert_loaded_with(self, backend):
        self.w._get_pipe()
        self.from_pretrained.assert_called_once_with(
            self.w.MODEL_ID, vae=self.w.MODEL_VAE, device="cuda", backend=backend,
        )

    def test_default_torch(self):
        self._assert_loaded_with("torch")

    def test_valid_values_pass_through(self):
        for value in ("torch", "torch-eager", "vllm"):
            with self.subTest(value=value):
                self._reset()
                self.from_pretrained.reset_mock()
                os.environ["YUE2_BACKEND"] = value
                self._assert_loaded_with(value)

    def test_read_at_call_not_import(self):
        # Модуль уже импортирован в setUp; переменная задана после — должна учитываться.
        os.environ["YUE2_BACKEND"] = "torch-eager"
        self._assert_loaded_with("torch-eager")

    def test_invalid_value_is_clear_error(self):
        from fastapi import HTTPException
        for value in ("cuda", "TORCH", "torch_eager", "llama.cpp"):
            with self.subTest(value=value):
                self._reset()
                self.from_pretrained.reset_mock()
                os.environ["YUE2_BACKEND"] = value
                with self.assertRaises(HTTPException) as ctx:
                    self.w._get_pipe()
                self.assertEqual(ctx.exception.status_code, 503)
                self.assertIn("YUE2_BACKEND", str(ctx.exception.detail))
                self.from_pretrained.assert_not_called()


if __name__ == "__main__":
    unittest.main()
