"""Тесты к находкам кросс-ревью задачи internal-guitar-notes — по уточнённому контракту, не по реализации.

Контракт (решение после кросс-ревью):
1) Фразы страницы «Инструменты» (phrases._twice): у блока gtrnotes во втором круге повторяются не только ноты,
   но и доли `_beats` (со сдвигом на длину круга) — иначе удары второго круга теряют направление "up".
2) Пустое окно — тишина, а не ошибка: ноты у трека есть, но в окно превью ни одна не попала → POST /jobs/{id}/fx
   с gtrnotes отвечает 200 и тишиной от новой гитары. Блок: notes == [] при служебном `_t0` (окно выставил
   воркер) → тишина той же формы; без `_t0` и без нот → по-прежнему ChainError «нет нот» (test_gtrnotes_block).
   У трека в кэше нет ни одной ноты → /fx отвечает 422 с текстом про отсутствие нот гитары у трека.
3) Одно распознавание на трек при одновременных запросах: два параллельных POST /jobs/{id}/guitar_notes →
   notes.run вызван один раз, оба ответа 200, кэш валиден (временный файл кэша не общий).
4) Кэш нот устаревает при новой дорожке гитары: кэш хранит отпечаток stem-guitar.flac (mtime_ns и размер);
   дорожку перезаписали → следующий POST /guitar_notes (без force) распознаёт заново (cached: false), /fx с
   gtrnotes берёт новые ноты. Кэш без отпечатка (старый формат) устарел, только если дорожка новее файла кэша.

Внешние границы — как в test_guitar_notes_api: notes.run/available/engines, gpu_queue, хранилище наборов.
В тесте одновременных запросов gpu_queue подменён реентерабельным локом — как настоящая очередь к видеокарте
(взаимоисключение, см. test_gpu_queue), иначе подмена разрешала бы то, чего на воркере не бывает.

Запуск: cd worker && python3 -m unittest test_guitar_notes_fixes -v
"""
import contextlib
import json
import os
import threading
import time
import unittest
from unittest import mock

import test_gtrnotes_block as gb
import test_guitar_notes_api as ga

GTR = [{"type": "gtrnotes", "kit": "fake/notes"}]
TAIL = [[3.6, 3.9, 57, 0.6], [3.7, 3.95, 60, 0.6]]   # гитара вступает после окна 0…3 с


def _gtr_blocks(proc):
    """Блоки gtrnotes, дошедшие до fx_engine.process (обёрнутого mock.wraps)."""
    return [b for c in proc.call_args_list for b in (c.args[2] if len(c.args) > 2 else c.kwargs["chain"])
            if b.get("type") == "gtrnotes"]


def _bump_mtime(path, delta_ns):
    st = path.stat()
    os.utime(path, ns=(st.st_atime_ns, st.st_mtime_ns + delta_ns))


# ---------- 1) фразы: доли второго круга ----------

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestPhraseTwiceBeats(unittest.TestCase):
    """Вход фразы — два круга подряд: удары второго круга считаются по долям второго круга."""

    PHRASE = "gtr-strum"

    def test_second_cycle_keeps_up_strokes(self):
        import guitarnotes
        import phrases
        _, cycle = phrases.render_dry(self.PHRASE, 1.0)
        chain = phrases.gtrnotes_chain([{"type": "gtrnotes", "kit": "fake/notes"}], self.PHRASE)
        blk = phrases._twice(chain, cycle)[0]
        eps = 1e-6
        notes = [[n["t"], n["t"] + n["d"], n["midi"][0], n["vel"]] for n in blk["notes"]]
        beats = [float(b) for b in blk.get("_beats", [])]

        def ups(lo, hi):
            ns = [n for n in notes if lo - eps <= n[0] < hi - eps]
            bs = [b for b in beats if lo - eps <= b < hi - eps]
            return sum(1 for s in guitarnotes.strikes(ns, bs) if s["dir"] == "up")

        first, second = ups(0.0, cycle), ups(cycle, 2 * cycle)
        self.assertGreater(first, 0, "опора: в первом круге фразы есть удары вверх")
        self.assertGreater(second, 0, "во втором круге нет долей — все удары стали «вниз»")
        self.assertLessEqual(abs(second - first), 2, f"ударов вверх: первый круг {first}, второй {second}")


# ---------- 2) пустое окно — тишина ----------

@unittest.skipUnless(gb._HAS_DEPS, gb._SKIP)
class TestBlockEmptyWindow(unittest.TestCase):
    """Блок, которому воркер выставил окно (_t0), но нот в окне нет, — тишина той же формы."""

    def test_empty_notes_with_t0_is_silence(self):
        x = gb._input()
        y = gb._run(gb._blk([], _t0=5.0, _beats=[0.0, 0.5, 1.0]), x=x)
        y = gb.np.asarray(y)
        self.assertEqual(y.shape, x.shape)
        self.assertLess(float(gb.np.abs(y).max()), 1e-6, "вместо тишины — звук (старая гитара?)")

    def test_empty_notes_without_t0_still_error(self):
        # опора: без _t0 блок не обработан воркером — ошибка «нет нот» остаётся
        fx = gb._fx()
        with self.assertRaises(fx.ChainError):
            gb._run(gb._blk([]))


class TestFxEmptyWindow(ga._NotesCase):
    """POST /jobs/{id}/fx: ноты у трека есть, а в окно превью не попали."""

    def test_window_before_guitar_entry_is_silence(self):
        import soundfile as sf
        jid, d = self._gtr_job()
        self._write_cache(d, TAIL)
        r = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **{"from": 0.0, "to": 3.0})
        self.assertEqual(r.status_code, 200, r.text)
        y, sr = sf.read(str(d / r.json()["file"]), dtype="float64", always_2d=True)
        head = y[:int(2.9 * sr)]
        self.assertGreater(len(head), 0)
        self.assertLess(float(abs(head).max()), 1e-4, "в окне до вступления гитары — звук")
        self.assertEqual(self.run_calls, [], "ноты у трека есть — распознавать заново не нужно")

    def test_track_without_any_notes_422(self):
        jid, d = self._gtr_job()
        self._write_cache(d, [])
        r = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **{"from": 0.0, "to": 3.0})
        self.assertEqual(r.status_code, 422, r.text)
        text = r.text.lower()
        for word in ("нот", "гитар", "трек"):
            with self.subTest(word=word):
                self.assertIn(word, text)


# ---------- 3) одно распознавание при одновременных запросах ----------

class TestConcurrentRecognition(ga._NotesCase):

    def setUp(self):
        super().setUp()
        test = self
        lock = threading.RLock()

        @contextlib.contextmanager
        def locked_queue(section="gpu"):
            # как настоящая gpu_queue: взаимоисключение, реентерабельность
            with lock:
                test.gpu_sections.append(section)
                test._in_gpu = True
                try:
                    yield
                finally:
                    test._in_gpu = False

        p = mock.patch.object(self.w, "gpu_queue", locked_queue)
        p.start()
        self.addCleanup(p.stop)
        real_run = self.notes.run   # подмена из _NotesCase

        def slow_run(*args, **kw):
            time.sleep(0.3)
            return real_run(*args, **kw)

        p = mock.patch.object(self.notes, "run", slow_run)
        p.start()
        self.addCleanup(p.stop)

    def test_two_parallel_requests_one_recognition(self):
        jid, d = self._gtr_job()
        results, errors = [None, None], []
        start = threading.Barrier(2)

        def call(i):
            try:
                start.wait()
                results[i] = self._recognize(jid)
            except Exception as e:   # исключение потока — в список, иначе тест его не увидит
                errors.append(e)

        threads = [threading.Thread(target=call, args=(i,)) for i in range(2)]
        for t in threads:
            t.start()
        for t in threads:
            t.join(30)
        self.assertEqual(errors, [])
        for i, r in enumerate(results):
            with self.subTest(request=i):
                self.assertIsNotNone(r)
                self.assertEqual(r.status_code, 200, r.text)
                self.assertEqual(r.json()["notes"], len(ga.BP))
        self.assertEqual(len(self.run_calls), 1, "одновременные запросы распознали трек дважды")
        c = json.loads(self._cache(d).read_text())
        self.assertEqual(len(c["notes"]), len(ga.BP))
        self.assertEqual(sorted(p.name for p in d.iterdir() if "guitar-notes" in p.name), ["guitar-notes.json"],
                         "остался временный файл кэша")


# ---------- 4) кэш устаревает при новой дорожке гитары ----------

NEW_BP = [[0.4, 0.9, 64, 0.6], [1.1, 1.8, 67, 0.6]]


class TestCacheFingerprint(ga._NotesCase):

    def _stem(self, d):
        return d / "stem-guitar.flac"

    def _rewrite_stem(self, d, mtime_delta_ns):
        """Другая гитара (другой тон и длина), mtime сдвинут на mtime_delta_ns от прежнего."""
        import soundfile as sf
        old = self._stem(d).stat()
        sf.write(str(self._stem(d)), ga._tone_stereo(ga._hz(64), [(0.0, 3.0, 0.2)], dur=3.0), ga.SR)
        os.utime(self._stem(d), ns=(old.st_atime_ns, old.st_mtime_ns + mtime_delta_ns))
        self.assertNotEqual(self._stem(d).stat().st_size, old.st_size, "опора: размер дорожки сменился")

    def test_rewritten_stem_recognized_again(self):
        jid, d = self._gtr_job()
        self.assertIs(self._recognize(jid).json()["cached"], False)
        self.raw = ga._raw(bp=NEW_BP)
        self._rewrite_stem(d, 2_000_000_000)
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["cached"], False, "дорожка гитары новая, а ноты из старого кэша")
        self.assertEqual(r.json()["notes"], len(NEW_BP))
        self.assertEqual(len(self.run_calls), 2)
        again = self._recognize(jid)
        self.assertIs(again.json()["cached"], True, "после пересчёта кэш снова должен работать")
        self.assertEqual(len(self.run_calls), 2)

    def test_rewritten_stem_with_older_mtime_still_stale(self):
        # отпечаток — mtime_ns и размер: дорожку подменили файлом «из прошлого» — кэш всё равно устарел
        jid, d = self._gtr_job()
        self._recognize(jid)
        self.raw = ga._raw(bp=NEW_BP)
        self._rewrite_stem(d, -5_000_000_000)
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["cached"], False)
        self.assertEqual(len(self.run_calls), 2)

    def test_fx_takes_new_notes_after_stem_rewrite(self):
        import fx_engine
        jid, d = self._gtr_job()
        self._recognize(jid)
        self.raw = ga._raw(bp=NEW_BP)
        self._rewrite_stem(d, 2_000_000_000)
        with mock.patch.object(fx_engine, "process", wraps=fx_engine.process) as proc:
            r = self._fx(jid, source="guitar", chain=GTR, preview=True, output="solo", **{"from": 0.0, "to": 2.5})
        self.assertEqual(r.status_code, 200, r.text)
        blocks = _gtr_blocks(proc)
        self.assertTrue(blocks, "блок gtrnotes не дошёл до движка")
        midis = sorted({int(round(m)) for n in blocks[0]["notes"] for m in n["midi"]})
        self.assertEqual(midis, [64, 67], "в блок пришли ноты старой дорожки")
        self.assertEqual(len(self.run_calls), 2)

    def test_old_format_cache_newer_than_stem_used(self):
        # кэш без отпечатка, записан после дорожки — действует
        jid, d = self._gtr_job()
        self._write_cache(d, ga.BP)
        _bump_mtime(self._cache(d), 2_000_000_000)
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["cached"], True)
        self.assertEqual(self.run_calls, [])

    def test_old_format_cache_older_than_stem_stale(self):
        # кэш без отпечатка, дорожка новее файла кэша — распознать заново
        jid, d = self._gtr_job()
        self._write_cache(d, ga.BP)
        self.raw = ga._raw(bp=NEW_BP)
        _bump_mtime(self._stem(d), 2_000_000_000)
        r = self._recognize(jid)
        self.assertEqual(r.status_code, 200, r.text)
        self.assertIs(r.json()["cached"], False)
        self.assertEqual(r.json()["notes"], len(NEW_BP))
        self.assertEqual(len(self.run_calls), 1)


if __name__ == "__main__":
    unittest.main()
