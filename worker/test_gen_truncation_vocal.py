"""Спецификация: обрезанные генерации и голос в инструменталах.

Что должно работать:
G1. job_budget(draft, max_tokens, role, lyrics=""): полная генерация (не draft,
    role != "fragment") — не меньше 6000; лирика-заглушка из одних тегов
    [Instrumental] — бюджет как у песни (internal-vram-context усл. 7): max_tokens
    из заявки, если задан, иначе MAX_SEM_TOKENS; число секций не влияет; итог
    всегда ≤ MAX_SEM_TOKENS. draft → 450; fragment — как раньше (пол 200,
    потолок MAX_SEM_TOKENS).
G2. retry_budget(b) = min(int(b × 1.6), MAX_SEM_TOKENS).
G3. Колонка jobs.truncated (INTEGER DEFAULT 0), миграция повторяемая.
G4. Обрезанная полная генерация — ОДИН авторетрай с retry_budget(бюджета)
    (новая заявка серверу yue2.cpp с большим потолком семантики); если ретрай не
    больше исходного — ретрая нет; повтор снова обрезан → truncated=1. draft и
    fragment не ретраятся. GET /jobs и GET /jobs/{id} отдают "truncated": bool.
G5. is_instrumental_job(style, lyrics): в лирике нет слов И (тег
    [Instrumental] или «instrumental»/«no vocals» в стиле).
G6. abcparse.mute_voice(abc, "Vocal"): ноты голоса Vocal → паузы той же
    длительности, аккорды на месте; остальное байт-в-байт.
G7. Инструментальная джоба: в заявку трека серверу yue2.cpp уходит план с
    заглушённым Vocal (свой план воркер строит сам — заявкой «только план» с сидом
    трека; прикреплённый файл request.abc не меняется); cot="off" — без плана;
    песня со словами — abc не передаётся.
G8. Инструментал (не draft, не fragment) с утечкой голоса — ОДИН авторетрай с
    сидом seed+1 (сид 0/NULL → новый ненулевой), дорожки пересчитываются,
    vocal_leak сбрасывается; снова утечка — метка остаётся. Статус «готово» —
    только после проверки голоса.

Программа yue-server (фейк из test_engine_cpp; план в ответе — PLAN) и разделение на дорожки —
внешние границы, подменяются; громкость дорожки голоса задаёт фейковое разделение, проверка
голоса — настоящая. Факта обрезания в ответе yue2.cpp нет (фейк его не выражает), поэтому в G4
он ставится поверх настоящей генерации через сервер (_on_render).
"""
import json
import unittest
from pathlib import Path
from unittest import mock

import abcparse
from test_jobs_lifecycle import _Case, plan_bodies
from test_pure import _HAS_WORKER_DEPS, _WorkerDbCase

INSTRUMENTAL_STYLE = "instrumental post-rock, clean guitar"
STUB_3 = "[Instrumental]\n\n[Instrumental]\n\n[Instrumental]"
WORDS = "[verse] la la"

# реалистичный план YuE: два голоса, секции, L:1/32 — такт 4/4 = 32 единицы
PLAN = """X:1
T:Untitled
M:4/4
L:1/32
Q:1/4=120
V: Vocal clef=treble name="Vocal Melody" snm="Vocal"
V: Ins clef=treble name="Inst." snm="Ins."
K:Fm
% intro
V: Vocal
Z4|
V: Ins
"Fm"F8C8F8A8|"Db"D8F8A8d8|"Eb"E8G8B8e8|"Fm"F16z16|
% verse
V: Vocal
"Fm"z8c4c4c4B4B4A4|"Fm"B4c8c8z12|
V: Ins
"Fm"F8A8c8A8|"Fm"F8A8c8A8|
% chorus
V: Vocal
"Db"d8c8B8A8|"Eb"G16z16|
V: Ins
Z2|
"""

PLAN_NO_VOCAL = """X:1
T:Untitled
M:4/4
L:1/32
Q:1/4=120
V: Ins clef=treble name="Inst." snm="Ins."
K:Fm
% intro
V: Ins
"Fm"F8C8F8A8|"Db"D8F8A8d8|
"""

PLAN_SILENT_VOCAL = """X:1
T:Untitled
M:4/4
L:1/32
Q:1/4=120
V: Vocal clef=treble name="Vocal Melody" snm="Vocal"
V: Ins clef=treble name="Inst." snm="Ins."
K:Fm
% intro
V: Vocal
Z2|
"Fm"z32|z16z16|
V: Ins
"Fm"F8C8F8A8|"Db"D8F8A8d8|"Eb"E8G8B8e8|"Fm"F16z16|
"""


def _vocal_notes(abc: str) -> int:
    """Сколько нот у голоса Vocal по разбору плана."""
    return sum((b.get("voices") or {}).get("Vocal", 0) for b in abcparse.parse_abc(abc)["bars"])


def _ins_notes(abc: str) -> int:
    return sum((b.get("voices") or {}).get("Ins", 0) for b in abcparse.parse_abc(abc)["bars"])


def _voice_lines(abc: str, voice: str) -> list[tuple[int, str]]:
    """Строки тела голоса (номер, текст): после «V: voice» до следующей V:/% секции."""
    out, cur = [], None
    for i, line in enumerate(abc.splitlines()):
        s = line.strip()
        if s.startswith("V:") and i > 0 and not any(k in s for k in ("clef=", "name=")):
            cur = s[2:].strip()
            continue
        if s.startswith("%") or s.startswith("V:"):
            cur = None
            continue
        if cur == voice and s:
            out.append((i, line))
    return out


# ---------------------------------------------------------------- G6 mute_voice


class TestMuteVoice(unittest.TestCase):
    """ТК10: заглушка партии Vocal в ABC-плане."""

    def setUp(self):
        self.muted = abcparse.mute_voice(PLAN, "Vocal")

    def test_vocal_has_no_notes(self):
        self.assertGreater(_vocal_notes(PLAN), 0)  # исходный план поёт
        self.assertEqual(_vocal_notes(self.muted), 0)

    def test_bar_becomes_rest_of_same_length_with_chord(self):
        before = [b for b in abcparse.parse_abc(PLAN)["bars"] if (b.get("voices") or {}).get("Vocal")]
        after = {b["idx"]: b for b in abcparse.parse_abc(self.muted)["bars"]}
        self.assertTrue(before)
        for b in before:
            a = after[b["idx"]]
            self.assertEqual(a["rests"].get("Vocal"), 32, a)  # весь такт 4/4 при L:1/32
            self.assertEqual(a["chords"], b["chords"], "аккорд-аннотация пропала")
        self.assertIn('"Fm"', self.muted.splitlines()[_voice_lines(PLAN, "Vocal")[1][0]])

    def test_same_bars_and_durations(self):
        b0 = abcparse.parse_abc(PLAN)["bars"]
        b1 = abcparse.parse_abc(self.muted)["bars"]
        self.assertEqual(len(b0), len(b1))
        for x, y in zip(b0, b1, strict=True):
            self.assertEqual((x["idx"], x["section"], x["start_sec"], x["end_sec"]),
                             (y["idx"], y["section"], y["start_sec"], y["end_sec"]))

    def test_header_ins_and_sections_byte_for_byte(self):
        src, dst = PLAN.splitlines(), self.muted.splitlines()
        self.assertEqual(len(src), len(dst), "число строк изменилось")
        vocal_rows = {i for i, _ in _voice_lines(PLAN, "Vocal")}
        for i, (a, b) in enumerate(zip(src, dst, strict=True)):
            if i in vocal_rows:
                self.assertEqual(a.count("|"), b.count("|"), f"строка {i}: число тактов")
            else:
                self.assertEqual(a, b, f"строка {i} не должна меняться")
        self.assertEqual(_ins_notes(PLAN), _ins_notes(self.muted))

    def test_plan_without_vocal_unchanged(self):
        self.assertEqual(abcparse.mute_voice(PLAN_NO_VOCAL, "Vocal"), PLAN_NO_VOCAL)

    def test_rest_only_vocal_unchanged(self):
        self.assertEqual(abcparse.mute_voice(PLAN_SILENT_VOCAL, "Vocal"), PLAN_SILENT_VOCAL)

    def test_idempotent(self):
        self.assertEqual(abcparse.mute_voice(self.muted, "Vocal"), self.muted)


# ---------------------------------------------------------------- G1, G2, G5 — чистые функции воркера


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestJobBudget(_WorkerDbCase):
    """ТК1: бюджет семантических токенов."""

    def setUp(self):
        super().setUp()
        p = mock.patch.object(self.w, "MAX_SEM_TOKENS", 16000)
        p.start()
        self.addCleanup(p.stop)

    def b(self, draft=False, max_tokens=0, role="", lyrics=""):
        return self.w.job_budget(draft, max_tokens, role, lyrics)

    def test_draft_450(self):
        self.assertEqual(self.b(draft=True), 450)
        self.assertEqual(self.b(draft=True, max_tokens=9000, lyrics=WORDS), 450)

    def test_full_without_length_is_max(self):
        self.assertEqual(self.b(lyrics=WORDS), 16000)

    def test_full_minimum_6000(self):
        self.assertEqual(self.b(max_tokens=2500, lyrics=WORDS), 6000)
        self.assertEqual(self.b(max_tokens=9000, lyrics=WORDS), 9000)

    def test_full_capped_by_max(self):
        self.assertEqual(self.b(max_tokens=40000, lyrics=WORDS), 16000)

    def test_lyrics_default_argument(self):
        # lyrics необязателен: старые вызовы с тремя аргументами работают
        self.assertEqual(self.w.job_budget(False, 2500, ""), 6000)

    def test_fragment_keeps_old_rule(self):
        self.assertEqual(self.b(max_tokens=1000, role="fragment"), 1000)
        self.assertEqual(self.b(max_tokens=100, role="fragment"), 200)
        self.assertEqual(self.b(max_tokens=40000, role="fragment"), 16000)

    def test_instrumental_stub_by_sections(self):
        """internal-vram-context ТК9: заглушка без max_tokens — потолок MAX_SEM_TOKENS при любом числе секций."""
        stub = lambda n: "\n\n".join(["[Instrumental]"] * n)  # noqa: E731
        for n in (1, 3, 8, 36):
            self.assertEqual(self.b(lyrics=stub(n)), self.w.MAX_SEM_TOKENS, f"{n} секций")

    def test_instrumental_stub_uses_max_tokens(self):
        """internal-vram-context ТК9: заглушка с max_tokens — как песня: max_tokens из заявки, пол 6000."""
        stub8 = "\n\n".join(["[Instrumental]"] * 8)
        self.assertEqual(self.b(max_tokens=7000, lyrics=STUB_3), 7000)
        self.assertEqual(self.b(max_tokens=7000, lyrics=stub8), 7000)
        self.assertEqual(self.b(max_tokens=100, lyrics=STUB_3), 6000)

    def test_small_max_sem_tokens_wins_over_minimum(self):
        with mock.patch.object(self.w, "MAX_SEM_TOKENS", 5000):
            self.assertEqual(self.b(lyrics=WORDS), 5000)
            self.assertEqual(self.b(max_tokens=2500, lyrics=WORDS), 5000)
            self.assertEqual(self.b(lyrics=STUB_3), 5000)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestRetryBudget(_WorkerDbCase):
    """ТК2: бюджет повтора обрезанной генерации."""

    def test_values(self):
        with mock.patch.object(self.w, "MAX_SEM_TOKENS", 16000):
            self.assertEqual(self.w.retry_budget(6000), 9600)
            self.assertEqual(self.w.retry_budget(12000), 16000)
            self.assertEqual(self.w.retry_budget(16000), 16000)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestIsInstrumentalJob(_WorkerDbCase):
    """ТК11: заказан ли трек без голоса."""

    def test_stub_lyrics_any_style(self):
        for style in ("dark rock", "", INSTRUMENTAL_STYLE):
            with self.subTest(style=style):
                self.assertTrue(self.w.is_instrumental_job(style, "[Instrumental]\n\n[Instrumental]"))

    def test_empty_lyrics_no_vocals_style(self):
        self.assertTrue(self.w.is_instrumental_job("ambient, no vocals", ""))

    def test_words_never(self):
        self.assertFalse(self.w.is_instrumental_job("instrumental", "[verse] la la"))
        self.assertFalse(self.w.is_instrumental_job("no vocals", "[Instrumental]\nla la"))

    def test_tag_without_words_and_vocal_style(self):
        self.assertFalse(self.w.is_instrumental_job("rock", "[verse]"))
        self.assertFalse(self.w.is_instrumental_job("rock", ""))


# ---------------------------------------------------------------- G3 миграция


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestMigrateTruncated(_WorkerDbCase):
    """ТК9: старая БД без колонки truncated."""

    def _legacy_db(self):
        with self._conn() as c:
            c.execute("""CREATE TABLE jobs (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                title TEXT DEFAULT '', status TEXT NOT NULL DEFAULT 'queued',
                style TEXT NOT NULL, lyrics TEXT NOT NULL, seed INTEGER,
                cot TEXT NOT NULL DEFAULT 'full', error TEXT DEFAULT '',
                duration_sec REAL DEFAULT 0, audio_file TEXT DEFAULT '',
                abc_file TEXT DEFAULT '', created_at TEXT NOT NULL,
                finished_at TEXT DEFAULT '')""")
            c.execute("INSERT INTO jobs (title, status, style, lyrics, seed, created_at) "
                      "VALUES ('old song', 'done', 'dark rock', '[verse] la', 7, '2026-01-01T00:00:00')")

    def test_column_added_default_zero_and_idempotent(self):
        self._legacy_db()
        self.assertNotIn("truncated", self._cols())
        self.w._migrate()
        self.assertIn("truncated", self._cols())
        self.w._migrate()  # повторная миграция не падает
        with self._conn() as c:
            old = c.execute("SELECT truncated FROM jobs WHERE title='old song'").fetchone()
            c.execute("INSERT INTO jobs (title, status, style, lyrics, created_at) "
                      "VALUES ('new', 'queued', 's', 'l', '2026-01-02T00:00:00')")
            new = c.execute("SELECT truncated FROM jobs WHERE title='new'").fetchone()
        self.assertEqual(old[0], 0)
        self.assertEqual(new[0], 0)


# ---------------------------------------------------------------- генерация: общие заготовки


class _GenCase(_Case):
    def setUp(self):
        super().setUp()
        p = mock.patch.object(self.w, "MAX_SEM_TOKENS", 16000)
        p.start()
        self.addCleanup(p.stop)
        self._scenario(abc=PLAN)   # план, который модель пишет (и отдаёт заявке «только план»)
        # громкость дорожки голоса на каждое разделение: True — голос слышен
        self.loud = []
        self.status_at_separate = []

    def _fake_separate(self, audio, out_dir, **kw):
        res = super()._fake_separate(audio, out_dir, **kw)
        loud = self.loud.pop(0) if self.loud else False
        self.status_at_separate.append(self._row(int(Path(out_dir).name))["status"])
        if loud:
            import numpy as np
            import soundfile as sf
            sr = 44100
            t = np.arange(2 * sr) / sr
            tone = (0.3 * np.sin(2 * np.pi * 330 * t)).astype("float32")
            sf.write(str(Path(out_dir) / "stem-vocals.flac"), np.stack([tone, tone], axis=1), sr)
        return res

    def _on_render(self, hook):
        """hook(row) — перед каждой генерацией трека (заявка идёт фейку yue-server как есть);
        вернул bool — факт обрезания этого прогона."""
        real = self.w._cpp_render

        def render(cpp, row, *a, **kw):
            flag = hook(row)
            song = real(cpp, row, *a, **kw)
            if flag is not None:
                song.truncated = flag
            return song
        p = mock.patch.object(self.w, "_cpp_render", render)
        p.start()
        self.addCleanup(p.stop)

    def _truncated(self, *flags):
        """Обрезан ли каждый очередной прогон (дальше — не обрезан)."""
        script = list(flags)
        self._on_render(lambda row: script.pop(0) if script else False)

    def _run(self, **extra):
        jid = self._new(**extra)
        self._set_status(jid, "running")
        self.w._run_job(jid)
        return jid

    def _prepare(self, **extra):
        jid = self._new(**extra)
        self._set_status(jid, "running")
        return jid

    def _budgets(self):
        return [b["semantic_sampling"]["max_tokens"] for b in self._bodies()]


# ---------------------------------------------------------------- G4 обрезание


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestCppTruncationSource(_GenCase):
    """Источник обрезания (internal-cpp-only): ответ yue2.cpp факта не несёт — трек обрезан, когда шагов
    семантики в ответе столько же, сколько потолок заявки (модель не закончила песню сама); только у полного
    трека — черновик и кусок короткие по замыслу."""

    def _semantic(self, n):
        self._scenario(abc=PLAN, semantic=",".join(["7"] * n))

    def test_steps_at_cap_retried_with_bigger_budget(self):
        self._semantic(6000)       # первый прогон упёрся в потолок 6000; повтор с потолком 9600 — уже нет
        jid = self._run(max_tokens=6000)
        self.assertEqual(self._budgets(), [6000, 9600], "обрезанный по потолку трек не повторён")
        self.assertIs(self._status(jid)["truncated"], False)

    def test_steps_below_cap_not_truncated(self):
        self._semantic(5999)
        jid = self._run(max_tokens=6000)
        self.assertEqual(self._budgets(), [6000])
        self.assertIs(self._status(jid)["truncated"], False)

    def _card(self, mib):
        """Видеопамять карты (граница — nvidia-smi)."""
        p = mock.patch.object(self.w, "_gpu_vram_mib", lambda: mib)
        p.start()
        self.addCleanup(p.stop)

    def test_economy_no_retry_with_same_cap(self):
        """Экономный режим, карта неизвестна: контекст 8192, потолок 5400 — повтор ушёл бы с тем же потолком и
        тем же сидом и обрезался бы так же; трек остаётся обрезанным после одного прогона."""
        self._card(0)
        self.settings.write_text(json.dumps({"cpp_model": "q8", "cpp_low_vram": True}))
        cap = 5400   # 8192 − 2792
        self._semantic(cap)
        jid = self._run(max_tokens=6000)
        self.assertEqual(self._budgets(), [cap], "в экономном режиме повтор с тем же потолком")
        self.assertIs(self._status(jid)["truncated"], True)

    def test_economy_big_context_retried(self):
        """internal-vram-context усл. 4: экономный режим на 8 ГБ — контекст 18176, потолок 15384; «потолок станет
        больше» считается по фактическому контексту, а не по флагу — обрезанный на 6000 трек повторяется с 9600."""
        self._card(8188)
        self.settings.write_text(json.dumps({"cpp_model": "q8", "cpp_low_vram": True}))
        self._semantic(6000)
        jid = self._run(max_tokens=6000)
        self.assertEqual(self._budgets(), [6000, 9600], "в экономном режиме с запасом контекста повтора нет")
        self.assertIs(self._status(jid)["truncated"], False)

    def test_economy_retry_capped_by_context(self):
        """Повтор в экономном режиме — не выше потолка контекста: 12000 × 1,6 → 16000 → потолок 15384."""
        self._card(8188)
        self.settings.write_text(json.dumps({"cpp_model": "q8", "cpp_low_vram": True}))
        self._semantic(12000)
        self._run(max_tokens=12000)
        self.assertEqual(self._budgets(), [12000, 15384])

    def test_economy_budget_at_context_ceiling_no_retry(self):
        """Бюджет уже упёрся в потолок контекста (15384) — повтор ушёл бы с тем же потолком: его нет."""
        self._card(8188)
        self.settings.write_text(json.dumps({"cpp_model": "q8", "cpp_low_vram": True}))
        self._semantic(15384)
        jid = self._run()   # без длины → MAX_SEM_TOKENS (16000) → потолок 15384
        self.assertEqual(self._budgets(), [15384])
        self.assertIs(self._status(jid)["truncated"], True)

    def test_continuation_at_cap_retried(self):
        """Продолжение: в ответе вся песня (сыгранное до отметки + новое), потолок тоже считает сыгранное."""
        parent = self._job(duration=60.0)
        r = self.client.post(f"/jobs/{parent}/continue", json={"from_sec": 10.0})
        self.assertLess(r.status_code, 300, r.text)
        jid = self._children(parent)[0]["id"]
        # длина продолжения ниже потолка воркера — повтору есть куда расти (без длины потолок уже максимальный)
        with self._conn() as c:
            c.execute("UPDATE jobs SET max_tokens=6000 WHERE id=?", (jid,))
        self._set_status(jid, "running")
        first = self._new_cap_of(jid)
        self._semantic(first)
        self.w._run_job(jid)
        budgets = self._budgets()
        self.assertEqual(budgets[0], first)
        self.assertEqual(len(budgets), 2, "обрезанное продолжение не повторено")
        self.assertGreater(budgets[1], first)
        self.assertIs(self._status(jid)["truncated"], False)

    def _new_cap_of(self, jid):
        """Потолок первой заявки продолжения — как его считает воркер (бюджет джобы + сыгранное)."""
        row = self._row(jid)
        budget = self.w.job_budget(bool(row["draft"]), row["max_tokens"], row["role"], row["lyrics"])
        k = round(float(row["cont_from"]) * self.e.FPS)
        return max(budget, k + 200)

    def test_draft_at_cap_not_truncated(self):
        """Черновик короткий по замыслу: 450 из 450 — его длина, а не обрыв (без метки «обрезан» и повтора)."""
        self._semantic(450)
        jid = self._run(draft=1)
        self.assertEqual(self._budgets(), [450])
        self.assertIs(self._status(jid)["truncated"], False)


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestTruncationRetry(_GenCase):
    """ТК3–ТК8: факт обрезания, один авторетрай с большим бюджетом."""

    def test_tk3_not_truncated_single_call(self):
        self._truncated(False)
        jid = self._run(max_tokens=6000)
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertIs(got["truncated"], False)
        self.assertEqual(self._row(jid)["truncated"], 0)
        self.assertEqual(len(self._bodies()), 1)

    def test_tk4_truncated_then_ok(self):
        self._truncated(True, False)
        jid = self._run(max_tokens=6000)
        self.assertEqual(self._budgets(), [6000, 9600], "обрезанная генерация не повторена с бюджетом × 1,6")
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertIs(got["truncated"], False)
        self.assertEqual(self._row(jid)["truncated"], 0)

    def test_tk4_retry_budget_clamped(self):
        self._truncated(True, False)
        self._run(max_tokens=12000)
        self.assertEqual(self._budgets(), [12000, 16000])

    def test_tk5_truncated_twice(self):
        self._truncated(True, True, False)
        jid = self._run(max_tokens=6000)
        self.assertEqual(len(self._bodies()), 2, "третьего прогона быть не должно")
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertIs(got["truncated"], True)
        self.assertEqual(self._row(jid)["truncated"], 1)
        listed = {j["id"]: j for j in self.client.get("/jobs").json()}
        self.assertIs(listed[jid]["truncated"], True)

    def test_list_reports_false_for_whole_track(self):
        self._truncated(False)
        jid = self._run(max_tokens=6000)
        listed = {j["id"]: j for j in self.client.get("/jobs").json()}
        self.assertIs(listed[jid]["truncated"], False)

    def test_tk7_draft_no_retry(self):
        self._truncated(True, False)
        jid = self._run(draft=True)
        self.assertEqual(len(self._bodies()), 1)
        self.assertEqual(self._row(jid)["truncated"], 1)
        self.assertIs(self._status(jid)["truncated"], True)

    def test_tk7_fragment_no_retry(self):
        self._truncated(True, False)
        jid = self._new()
        with self._conn() as c:
            c.execute("UPDATE jobs SET status='running', role='fragment', max_tokens=1000 WHERE id=?", (jid,))
        self.w._run_job(jid)
        self.assertEqual(self._budgets(), [1000])
        self.assertEqual(self._row(jid)["truncated"], 1)

    def test_tk8_budget_already_max_no_retry(self):
        self._truncated(True, False)
        jid = self._run()  # без max_tokens → MAX_SEM_TOKENS
        self.assertEqual(self._budgets(), [16000])
        self.assertEqual(self._row(jid)["truncated"], 1)

    def test_retry_same_job_same_folder(self):
        self._truncated(True, False)
        before = self.client.get("/jobs").json()
        jid = self._run(max_tokens=6000)
        self.assertEqual(len(self._bodies()), 2)
        self.assertEqual([d.name for d in self.jobs_dir.iterdir() if d.is_dir()], [str(jid)],
                         "артефакты ретрая — в папке той же джобы")
        self.assertTrue((self.jobs_dir / str(jid) / "audio.flac").is_file())
        self.assertEqual(len(self.client.get("/jobs").json()), len(before) + 1, "ретрай создал новую джобу")


# ---------------------------------------------------------------- G7 заглушённый план


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestInstrumentalPlan(_GenCase):
    """ТК12, ТК13: инструментал рендерится по плану без нот Vocal."""

    def test_tk12_builds_plan_and_mutes_vocal(self):
        jid = self._run(style=INSTRUMENTAL_STYLE, lyrics=STUB_3, seed=77)
        self.assertEqual(self._status(jid)["status"], "done")
        plans = plan_bodies(self.fake_dir)
        self.assertEqual(len(plans), 1, "план не построен")
        self.assertEqual(plans[0]["seed"], 77, "план строится с сидом трека")
        sent = self._bodies()[0].get("abc")
        self.assertIsNotNone(sent, "заявка трека без плана")
        self.assertEqual(_vocal_notes(sent), 0, "в плане инструментала поёт Vocal")
        self.assertEqual(_ins_notes(sent), _ins_notes(PLAN))
        self.assertEqual(sent, abcparse.mute_voice(PLAN, "Vocal"))

    def test_tk12_no_vocals_style_empty_lyrics(self):
        self._run(style="ambient, no vocals", lyrics="")
        sent = self._bodies()[0].get("abc")
        self.assertIsNotNone(sent, "заявка трека без плана")
        self.assertEqual(_vocal_notes(sent), 0)

    def test_tk12_song_with_words_no_abc(self):
        self._run(style=INSTRUMENTAL_STYLE, lyrics=WORDS)
        self.assertEqual(plan_bodies(self.fake_dir), [])
        self.assertNotIn("abc", self._bodies()[0])

    def test_cot_off_no_plan(self):
        jid = self._run(style=INSTRUMENTAL_STYLE, lyrics=STUB_3, cot="off")
        self.assertEqual(self._status(jid)["status"], "done")
        self.assertEqual(plan_bodies(self.fake_dir), [])
        self.assertNotIn("abc", self._bodies()[0])

    def test_tk13_attached_abc_muted_file_untouched(self):
        jid = self._run(style=INSTRUMENTAL_STYLE, lyrics=STUB_3, abc=PLAN)
        self.assertEqual(plan_bodies(self.fake_dir), [], "при прикреплённом плане свой не строится")
        sent = self._bodies()[0].get("abc")
        self.assertIsNotNone(sent)
        self.assertEqual(_vocal_notes(sent), 0)
        self.assertEqual(_ins_notes(sent), _ins_notes(PLAN))
        f = self.jobs_dir / str(jid) / "request.abc"
        self.assertEqual(f.read_text(encoding="utf-8"), PLAN, "файл пользователя изменён")


# ---------------------------------------------------------------- G8 утечка голоса


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestVocalLeakRetry(_GenCase):
    """ТК14–ТК17 и условие 8: один повтор с другим сидом при голосе в инструментале."""

    def _instrumental(self, **extra):
        body = {"style": INSTRUMENTAL_STYLE, "lyrics": STUB_3, "seed": 1234}
        body.update(extra)
        return body

    def _seeds(self):
        return [b["seed"] for b in self._bodies()]

    def test_tk14_leak_then_clean(self):
        status_at_render = []
        self._on_render(lambda row: status_at_render.append(self._row(row["id"])["status"]))
        self.loud = [True, False]
        jid = self._prepare(**self._instrumental())
        self.w._run_job(jid)
        self.assertEqual(self._seeds(), [1234, 1235], "утечка голоса не перегенерирована с сидом + 1")
        self.assertEqual(len(self.separations), 2, "дорожки не пересчитаны")
        got = self._status(jid)
        self.assertEqual(got["status"], "done", got)
        self.assertFalse(self._row(jid)["vocal_leak"], "метка голоса не сброшена")
        # условие 8: «готово» — только после проверки голоса
        self.assertEqual(len(status_at_render), 2)
        self.assertNotEqual(status_at_render[1], "done", "трек «готов», пока перегенерируется")
        self.assertNotIn("done", self.status_at_separate)

    def test_tk15_leak_twice(self):
        self.loud = [True, True, True]
        jid = self._run(**self._instrumental())
        self.assertEqual(len(self._bodies()), 2, "ретрай по голосу — только один")
        self.assertTrue(self._row(jid)["vocal_leak"], "метка голоса пропала")
        self.assertEqual(self._status(jid)["status"], "done")

    def test_stale_stems_from_previous_run_rechecked(self):
        # «повторить» после отмены на дорожках: дорожки прошлого звука не должны подменить проверку нового
        import numpy as np
        import soundfile as sf
        self.loud = [True, False]
        jid = self._prepare(**self._instrumental())
        jdir = self.jobs_dir / str(jid)
        jdir.mkdir(parents=True, exist_ok=True)
        sf.write(str(jdir / "stem-vocals.flac"), np.zeros((4410, 2), dtype="float32"), 44100)
        self.w._run_job(jid)
        self.assertGreaterEqual(len(self.separations), 1, "дорожки нового звука не построены")
        self.assertEqual(len(self._bodies()), 2, "голос в новом звуке не проверен")

    def test_tk16_song_with_words_no_retry(self):
        self.loud = [True, True]
        jid = self._run(style=INSTRUMENTAL_STYLE, lyrics=WORDS, seed=1234)
        self.assertEqual(len(self._bodies()), 1)
        self.assertEqual(self._status(jid)["status"], "done")

    def test_draft_no_leak_retry(self):
        self.loud = [True, True]
        self._run(**self._instrumental(draft=True))
        self.assertEqual(len(self._bodies()), 1)

    def test_clean_instrumental_single_call(self):
        self.loud = [False]
        jid = self._run(**self._instrumental())
        self.assertEqual(len(self._bodies()), 1)
        self.assertEqual(len(self.separations), 1)
        self.assertNotIn("done", self.status_at_separate)
        self.assertEqual(self._status(jid)["status"], "done")

    def test_tk17_seed_zero_or_null_gets_new_seed(self):
        for seed in (0, None):
            with self.subTest(seed=seed):
                self.loud = [True, False]
                self.separations.clear()
                before = len(self._bodies())
                jid = self._prepare(**self._instrumental())
                with self._conn() as c:
                    c.execute("UPDATE jobs SET seed=? WHERE id=?", (seed, jid))
                self.w._run_job(jid)
                seeds = self._seeds()[before:]
                self.assertEqual(len(seeds), 2)
                self.assertTrue(seeds[1], f"ретрай без нового сида: {seeds[1]!r}")
                self.assertNotEqual(seeds[1], seeds[0], "ретрай с тем же сидом")
                self.assertTrue(self._row(jid)["seed"], "новый сид не записан в строку трека")


if __name__ == "__main__":
    unittest.main()
