"""«Голос альбома»: прослушать кусок (internal-album-voice-draft, условие 4; ТК3).

voice.run с окном трека [src_from, src_to) — перевод Seed-VC только на куске,
и эндпоинт POST /jobs/{id}/voice_preview → {file, from, to}. По карточке задачи,
не по реализации: Seed-VC (voice.convert) и тяжёлый перевод в эндпоинте —
внешняя граница, они мокаются.

Без numpy/fastapi (CI-джоба только с ruff) классы пропускаются, как в test_voice.
"""
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

try:
    import numpy as np
    import soundfile as sf
    _HAS_AUDIO = True
except ImportError:
    _HAS_AUDIO = False

SR = 44100
TRACK_SEC = 35.0   # длиннее самого длинного куска (30 с)


def _write(path, x, sr=SR):
    sf.write(str(path), x, sr, format="FLAC")


def _tone(sec, amp=0.5, freq=440.0, sr=SR):
    t = np.arange(int(round(sec * sr))) / sr
    return (amp * np.sin(2 * np.pi * freq * t)).astype(np.float32)


def _make_track(d, sing_from=None, sing_to=None):
    """Папка трека: микс audio.flac (стерео) и дорожка голоса stem-vocals.flac.
    Голос звучит только в [sing_from, sing_to) (без окна — весь трек), вне — тишина."""
    d.mkdir(parents=True, exist_ok=True)
    n = int(TRACK_SEC * SR)
    vocals = np.zeros(n, dtype=np.float32)
    a = 0 if sing_from is None else int(sing_from * SR)
    b = n if sing_to is None else int(sing_to * SR)
    vocals[a:b] = _tone((b - a) / SR)
    music = _tone(TRACK_SEC, amp=0.2, freq=110.0)
    mix = np.stack([vocals + music, vocals + music], axis=1)
    _write(d / "stem-vocals.flac", np.stack([vocals, vocals], axis=1))
    _write(d / "audio.flac", mix)


def _frames(path):
    info = sf.info(str(path))
    return info.frames, info.samplerate


@unittest.skipUnless(_HAS_AUDIO, "нужны numpy/soundfile (окружение воркера)")
class TestRunWindow(unittest.TestCase):
    """voice.run(..., src_from, src_to): голос и микс обрезаются до перевода."""

    def setUp(self):
        import voice
        self.v = voice
        self.tmp = Path(tempfile.mkdtemp(prefix="test-vc-window-"))
        self.src = self.tmp / "src"
        self.ref = self.tmp / "ref"
        self.out = self.tmp / "out"
        self.out.mkdir()
        _make_track(self.ref)
        self.received = []   # что пришло в Seed-VC: (секунды, сигнал моно)

        def fake_convert(source_wav, ref_wav, out_wav, steps, *a, **kw):
            # Seed-VC — внешняя граница: запоминаем вход и отдаём его же как «переведённый» голос
            x, sr = sf.read(str(source_wav), dtype="float32", always_2d=True)
            self.received.append((len(x) / sr, x.mean(axis=1), sr))
            sf.write(str(out_wav), x, sr)
            return {"out": str(out_wav), "seconds": 0.1}

        p = mock.patch.object(self.v, "convert", side_effect=fake_convert)
        p.start()
        self.addCleanup(p.stop)

    def tearDown(self):
        import shutil
        shutil.rmtree(self.tmp, ignore_errors=True)

    def _out_path(self, res):
        return Path(res) if res else self.out / "audio.flac"

    def test_window_output_length_is_window(self):
        _make_track(self.src, 4.0, 7.0)
        res = self.v.run(self.src, "audio.flac", self.ref, self.out, 0.0, 5.0, 30, src_from=4.0, src_to=7.0)
        frames, sr = _frames(self._out_path(res))
        self.assertEqual(sr, SR)
        self.assertAlmostEqual(frames, 3.0 * SR, delta=2)

    def test_window_only_window_goes_to_seedvc(self):
        # голос звучит только в окне 4–7 с: если в Seed-VC ушло не окно, там будет тишина или 10 с
        _make_track(self.src, 4.0, 7.0)
        self.v.run(self.src, "audio.flac", self.ref, self.out, 0.0, 5.0, 30, src_from=4.0, src_to=7.0)
        self.assertEqual(len(self.received), 1)
        sec, x, sr = self.received[0]
        self.assertAlmostEqual(sec, 3.0, delta=0.01)
        edge = int(0.2 * sr)
        for part in (x[:edge], x[len(x) // 2 - edge: len(x) // 2 + edge], x[-edge:]):
            self.assertGreater(float(np.sqrt(np.mean(part ** 2))), 0.1, "в окне трека голос должен звучать везде")

    def test_window_in_middle_other_offsets(self):
        _make_track(self.src, 2.5, 8.0)
        res = self.v.run(self.src, "audio.flac", self.ref, self.out, 0.0, 5.0, 30, src_from=2.5, src_to=8.0)
        frames, _ = _frames(self._out_path(res))
        self.assertAlmostEqual(frames, 5.5 * SR, delta=2)
        self.assertAlmostEqual(self.received[0][0], 5.5, delta=0.01)

    def test_without_window_whole_track(self):
        _make_track(self.src)
        res = self.v.run(self.src, "audio.flac", self.ref, self.out, 0.0, 5.0, 30)
        frames, _ = _frames(self._out_path(res))
        self.assertAlmostEqual(frames, TRACK_SEC * SR, delta=2)
        self.assertEqual(len(self.received), 1)
        self.assertAlmostEqual(self.received[0][0], TRACK_SEC, delta=0.01)

    def test_explicit_none_window_is_whole_track(self):
        _make_track(self.src)
        res = self.v.run(self.src, "audio.flac", self.ref, self.out, 0.0, 5.0, 30, src_from=None, src_to=None)
        frames, _ = _frames(self._out_path(res))
        self.assertAlmostEqual(frames, TRACK_SEC * SR, delta=2)


@unittest.skipUnless(_HAS_WORKER_DEPS and _HAS_AUDIO, "нужны fastapi/httpx/numpy/soundfile (окружение воркера)")
class TestVoicePreviewApi(_WorkerApiCase):
    """POST /jobs/{id}/voice_preview {ref_job_id, from, to, ref_from?, ref_dur?, steps?} → {file, from, to}."""

    def setUp(self):
        super().setUp()
        self.received = []   # что пришло в Seed-VC: секунды голоса трека

        def fake_convert(source_wav, ref_wav, out_wav, steps, *a, **kw):
            # Seed-VC — внешняя граница: отдаём вход как «переведённый» голос
            x, sr = sf.read(str(source_wav), dtype="float32", always_2d=True)
            self.received.append(len(x) / sr)
            sf.write(str(out_wav), x, sr)
            return {"out": str(out_wav), "seconds": 0.1}

        p = mock.patch.object(self.w.voicevc, "convert", side_effect=fake_convert)
        p.start()
        self.addCleanup(p.stop)
        # шпион, не подмена: voice.run работает по-настоящему, тест видит, с чем его позвали
        spy = mock.patch.object(self.w.voicevc, "run", wraps=self.w.voicevc.run)
        self.run_spy = spy.start()
        self.addCleanup(spy.stop)

    def _call(self, i=0):
        """Аргументы i-го вызова voice.run по именам сигнатуры."""
        names = ["src_dir", "audio_name", "ref_dir", "out_dir", "ref_from", "ref_dur", "steps", "src_from", "src_to"]
        args, kwargs = self.run_spy.call_args_list[i]
        got = dict(zip(names, args, strict=False))
        got.update(kwargs)
        return got

    def _avail(self, value):
        p = mock.patch.object(self.w.voicevc, "available", return_value=value)
        p.start()
        self.addCleanup(p.stop)

    def _done(self, status="done", audio=True):
        jid = self._job(status=status, semantic=False)
        d = self.jobs_dir / str(jid)
        if audio:
            _make_track(d)
            with self._conn() as c:
                c.execute("UPDATE jobs SET audio_file='audio.flac' WHERE id=?", (jid,))
        return jid

    def _post(self, track, body):
        return self.client.post(f"/jobs/{track}/voice_preview", json=body)

    def _previews(self, track):
        return sorted(p.name for p in (self.jobs_dir / str(track)).glob("preview-vc-*.flac"))

    def test_success_returns_file_in_track_dir(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        r = self._post(track, {"ref_job_id": ref, "from": 2, "to": 8})
        self.assertEqual(r.status_code, 200, r.text)
        body = r.json()
        self.assertTrue(body["file"].startswith("preview-vc-"), body)
        self.assertTrue(body["file"].endswith(".flac"), body)
        self.assertAlmostEqual(float(body["from"]), 2)
        self.assertAlmostEqual(float(body["to"]), 8)
        f = self.jobs_dir / str(track) / body["file"]
        self.assertTrue(f.is_file())
        frames, sr = _frames(f)
        self.assertAlmostEqual(frames / sr, 6.0, delta=0.05)

    def test_window_and_ref_params_reach_conversion(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        r = self._post(track, {"ref_job_id": ref, "from": 1.5, "to": 9, "ref_from": 2, "ref_dur": 10, "steps": 30})
        self.assertEqual(r.status_code, 200, r.text)
        self.assertEqual(self.run_spy.call_count, 1)
        c = self._call()
        self.assertAlmostEqual(float(c["src_from"]), 1.5)
        self.assertAlmostEqual(float(c["src_to"]), 9)
        self.assertAlmostEqual(float(c["ref_from"]), 2)
        self.assertAlmostEqual(float(c["ref_dur"]), 10)
        self.assertEqual(int(c["steps"]), 30)
        self.assertEqual(Path(c["src_dir"]), self.jobs_dir / str(track))
        self.assertEqual(Path(c["ref_dir"]), self.jobs_dir / str(ref))
        # в Seed-VC ушёл только кусок трека
        self.assertEqual(len(self.received), 1)
        self.assertAlmostEqual(self.received[0], 7.5, delta=0.01)

    def test_from_zero_accepted(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        r = self._post(track, {"ref_job_id": ref, "from": 0, "to": 5})
        self.assertEqual(r.status_code, 200, r.text)

    def test_length_bounds_3_and_30_accepted(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        for frm, to in ((1, 4), (0, 30)):
            r = self._post(track, {"ref_job_id": ref, "from": frm, "to": to})
            self.assertEqual(r.status_code, 200, (frm, to, r.text))

    def test_length_out_of_range_422(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        for frm, to in ((1, 3.9), (0, 30.5), (5, 5), (8, 2)):
            r = self._post(track, {"ref_job_id": ref, "from": frm, "to": to})
            self.assertEqual(r.status_code, 422, (frm, to, r.status_code, r.text))
        self.assertEqual(self.run_spy.call_count, 0)
        self.assertEqual(self.received, [])
        self.assertEqual(self._previews(track), [])

    def test_seedvc_unavailable_503(self):
        self._avail(False)
        track, ref = self._done(), self._done()
        r = self._post(track, {"ref_job_id": ref, "from": 0, "to": 5})
        self.assertEqual(r.status_code, 503, r.text)
        self.assertEqual(self.run_spy.call_count, 0)
        self.assertEqual(self.received, [])

    def test_track_or_ref_missing_or_not_done_404(self):
        self._avail(True)
        good = self._done()
        cases = {
            "track missing": (9999, good),
            "ref missing": (good, 9999),
            "track not done": (self._done(status="running"), good),
            "ref not done": (good, self._done(status="queued")),
        }
        for name, (track, ref) in cases.items():
            r = self._post(track, {"ref_job_id": ref, "from": 0, "to": 5})
            self.assertEqual(r.status_code, 404, (name, r.status_code, r.text))
        self.assertEqual(self.run_spy.call_count, 0)
        self.assertEqual(self.received, [])

    def test_at_most_three_previews_kept(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        last = None
        for i in range(5):
            r = self._post(track, {"ref_job_id": ref, "from": i, "to": i + 4})
            self.assertEqual(r.status_code, 200, r.text)
            last = r.json()["file"]
        files = self._previews(track)
        self.assertLessEqual(len(files), 3, files)
        self.assertIn(last, files, "свежий кусок не удаляется")

    def test_preview_does_not_create_job(self):
        self._avail(True)
        track, ref = self._done(), self._done()
        with self._conn() as c:
            before = c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0]
        r = self._post(track, {"ref_job_id": ref, "from": 0, "to": 5})
        self.assertEqual(r.status_code, 200, r.text)
        with self._conn() as c:
            self.assertEqual(c.execute("SELECT COUNT(*) FROM jobs").fetchone()[0], before)


if __name__ == "__main__":
    unittest.main()
