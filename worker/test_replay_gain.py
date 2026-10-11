"""Спецификация «Переиграть в другом стиле»: громкость слоя (карточка internal-own-track, условие 125, ТК153).

Джоба-овердаб (overdub_of = родитель) после рендера:
  - overdub_gain 0 → смешивания нет: _mix_overdub не вызывается, у родителя не появляется
    overdub-<id>.flac, сама джоба — готовый новый трек (кавер);
  - overdub_gain 0,5 → _mix_overdub вызывается с 0,5, у родителя появляется overdub-<id>.flac;
  - overdub_gain NULL (старые строки) → как раньше, 0,5.
Раньше 0 подменялся на 0,5 (`or 0.5`) — кавер без смешивания был невозможен.

Тяжёлое (пайплайн YuE, ffmpeg-форматы, анализ метрик) подменено; файлы аудио — настоящие короткие flac.
Написаны по карточке, без чтения реализации.
Запуск из каталога worker: python -m unittest test_replay_gain -v
"""
import sys
import types
import unittest

from test_character import _Sampling
from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

SR = 48000


def _write_flac(path, amp=0.2, seconds=0.5):
    import numpy as np
    import soundfile as sf
    path.parent.mkdir(parents=True, exist_ok=True)
    t = np.arange(int(SR * seconds)) / SR
    tone = (amp * np.sin(2 * np.pi * 220 * t)).astype("float32")
    sf.write(str(path), np.stack([tone, tone], axis=1), SR)


class _Song:
    """Результат пайплайна: пишет audio.flac в каталог джобы."""
    truncated = False

    def save_artifacts(self, job_dir):
        _write_flac(job_dir / "audio.flac", amp=0.3)


class _Pipe:
    def __init__(self):
        self.calls = 0

    def __call__(self, **kw):
        self.calls += 1
        return _Song()

    def __enter__(self):
        return self

    def __exit__(self, *a):
        pass


_UNSET = object()


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class _ReplayRunCase(_WorkerApiCase):
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
        self.pipe = _Pipe()
        patches = [("_pipe", self.pipe), ("_load_error", None),
                   ("_get_pipe", lambda: self.pipe),
                   ("_make_formats", lambda *a, **kw: ("", "")),
                   ("auto_stems_instrumental", lambda *a, **kw: False)]
        if hasattr(self.w, "analyze_file"):
            patches.append(("analyze_file", lambda *a, **kw: {}))
        for name, val in patches:
            p = mock.patch.object(self.w, name, val)
            p.start()
            self.addCleanup(p.stop)

    def _overdub_row(self, gain=_UNSET):
        """Родитель (готовый трек с audio.flac) + строка-овердаб в очереди."""
        parent = self._job(duration=1.0, semantic=False)
        _write_flac(self.jobs_dir / str(parent) / "audio.flac", amp=0.2)
        with self._conn() as c:
            if gain is _UNSET:
                cur = c.execute(
                    "INSERT INTO jobs (title, status, style, lyrics, seed, cot, overdub_of, created_at) "
                    "VALUES ('overdub', 'queued', 'post-punk', '[Instrumental]', 7, 'full', ?, "
                    "'2026-10-11T00:00:00')", (parent,))
            else:
                cur = c.execute(
                    "INSERT INTO jobs (title, status, style, lyrics, seed, cot, overdub_of, overdub_gain, "
                    "created_at) VALUES ('overdub', 'queued', 'post-punk', '[Instrumental]', 7, 'full', ?, ?, "
                    "'2026-10-11T00:00:00')", (parent, gain))
            child = cur.lastrowid
        return parent, child

    def _spy_mix(self):
        calls = []
        real = self.w._mix_overdub

        def spy(*a, **kw):
            calls.append((a, kw))
            return real(*a, **kw)
        p = self.mock.patch.object(self.w, "_mix_overdub", spy)
        p.start()
        self.addCleanup(p.stop)
        return calls

    @staticmethod
    def _gain_of(call):
        a, kw = call
        if "gain" in kw:
            return kw["gain"]
        return a[2] if len(a) > 2 else 0.5  # дефолт сигнатуры _mix_overdub(parent, child, gain=0.5)


class TestReplayGain(_ReplayRunCase):

    def test_gain_zero_no_mix(self):
        calls = self._spy_mix()
        parent, child = self._overdub_row(gain=0.0)
        self.w._run_job(child)  # исключение наружу = провал теста
        self.assertEqual(self.pipe.calls, 1, "рендер не состоялся — тест ничего не проверил")
        self.assertEqual(calls, [], "gain 0 (кавер) — смешивания с оригиналом быть не должно")
        self.assertFalse((self.jobs_dir / str(parent) / f"overdub-{child}.flac").exists())
        row = self._row(child)
        self.assertEqual(row["status"], "done", "кавер — готовый новый трек")
        self.assertTrue((self.jobs_dir / str(child) / "audio.flac").is_file())

    def test_gain_half_mixes_with_half(self):
        calls = self._spy_mix()
        parent, child = self._overdub_row(gain=0.5)
        self.w._run_job(child)
        self.assertEqual(len(calls), 1, "слой поверх оригинала не смешан")
        a, kw = calls[0]
        args = list(a) + [kw.get("parent_id"), kw.get("child_id")]
        self.assertIn(parent, args)
        self.assertIn(child, args)
        self.assertAlmostEqual(self._gain_of(calls[0]), 0.5)
        self.assertTrue((self.jobs_dir / str(parent) / f"overdub-{child}.flac").is_file())
        self.assertEqual(self._row(child)["status"], "done")

    def test_gain_other_value_sets_layer_level(self):
        # родитель — тон 220 Гц с амплитудой 0,2, слой — тот же тон 0,3 в фазе: в миксе 0,2 + gain·0,3
        import numpy as np
        import soundfile as sf
        parent, child = self._overdub_row(gain=0.3)
        self.w._run_job(child)
        y, sr = sf.read(str(self.jobs_dir / str(parent) / f"overdub-{child}.flac"), always_2d=True)
        body = y[:int(0.4 * sr)]   # до кроссфейда в конце
        self.assertAlmostEqual(float(np.abs(body).max()), 0.2 + 0.3 * 0.3, delta=0.01)

    def test_gain_null_mixes_with_half(self):
        calls = self._spy_mix()
        parent, child = self._overdub_row(gain=None)
        self.assertIsNone(self._row(child)["overdub_gain"])  # строка действительно с NULL
        self.w._run_job(child)
        self.assertEqual(len(calls), 1, "старая строка без громкости — смешивается, как раньше")
        self.assertAlmostEqual(self._gain_of(calls[0]), 0.5)
        self.assertTrue((self.jobs_dir / str(parent) / f"overdub-{child}.flac").is_file())

    def test_not_overdub_no_mix(self):
        calls = self._spy_mix()
        jid = self._job(duration=1.0, semantic=False, status="queued")
        self.w._run_job(jid)
        self.assertEqual(self.pipe.calls, 1)
        self.assertEqual(calls, [], "обычная джоба не смешивается")


if __name__ == "__main__":
    unittest.main()
