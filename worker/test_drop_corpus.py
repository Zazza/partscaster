"""Спецификация удаления профилей исполнителей на воркере (задача internal-drop-corpus,
условия 3 и 4 карточки).

Что должно работать:
- Маршрутов /corpus, /corpus/{id}, /corpus/{id}/track, /corpus/{id}/build,
  /corpus/{id}/tracks нет: GET/POST на них → 404 или 405.
- Инициализация БД при запуске (init_db + _migrate — то, что воркер делает со своей БД)
  на свежей базе таблицу corpus не создаёт.
- Та же инициализация на базе со старой таблицей corpus и каталогом <DATA_DIR>/corpus
  с файлами: после неё нет ни таблицы, ни каталога; удаление пишется в лог воркера.
  Остальные данные (jobs, каталог голосов) не тронуты.
- Повторная инициализация — без ошибок и без повторной записи об удалении.

Каталог данных — временный: DATA_DIR и DB_PATH воркера подменяются на него
(DB_PATH лежит в DATA_DIR, как в рабочей установке).
"""
import logging
import unittest
from unittest import mock

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase, _WorkerDbCase

_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"
_LOGGER = "partscaster-worker"

# Старая схема таблицы профилей — как её создавали прежние версии воркера.
_OLD_CORPUS_DDL = """
CREATE TABLE IF NOT EXISTS corpus (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    created_at TEXT NOT NULL
)
"""


class _DataDirMixin:
    """DATA_DIR воркера — временный каталог теста (DB_PATH база уже подменила на него)."""

    def _patch_data_dir(self):
        self.data_dir = self.db_path.parent
        p = mock.patch.object(self.w, "DATA_DIR", self.data_dir)
        p.start()
        self.addCleanup(p.stop)

    def _tables(self):
        with self._conn() as c:
            return {r[0] for r in c.execute("SELECT name FROM sqlite_master WHERE type='table'")}

    def _init(self):
        self.w.init_db()
        self.w._migrate()


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestCorpusRoutesGone(_WorkerApiCase):
    """Условие 3: эндпоинтов профилей на воркере нет."""

    PATHS = ["/corpus", "/corpus/1", "/corpus/1/track", "/corpus/1/build", "/corpus/1/tracks"]

    def test_get_corpus_routes_absent(self):
        for path in self.PATHS:
            with self.subTest(path=path):
                r = self.client.get(path)
                self.assertIn(r.status_code, (404, 405), f"GET {path}: {r.status_code} {r.text[:200]}")

    def test_post_corpus_routes_absent(self):
        for path in self.PATHS:
            with self.subTest(path=path):
                r = self.client.post(path, json={"name": "c"}, headers={"X-Filename": "song.flac"})
                self.assertIn(r.status_code, (404, 405), f"POST {path}: {r.status_code} {r.text[:200]}")

    def test_no_corpus_route_registered(self):
        paths = [getattr(rt, "path", "") for rt in self.w.app.routes]
        self.assertEqual([p for p in paths if p.startswith("/corpus")], [])


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestFreshInitHasNoCorpus(_DataDirMixin, _WorkerDbCase):
    """Условие 3: инициализация на чистой базе таблицу corpus не создаёт."""

    def test_fresh_db_has_no_corpus_table(self):
        self._patch_data_dir()
        self._init()
        tables = self._tables()
        self.assertIn("jobs", tables)
        self.assertNotIn("corpus", tables)
        self.assertFalse((self.data_dir / "corpus").exists(),
                         "инициализация не должна создавать каталог corpus")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestOldCorpusDataDropped(_DataDirMixin, _WorkerDbCase):
    """Условие 4: старые данные профилей удаляются при запуске — один раз, с записью в лог."""

    def setUp(self):
        super().setUp()
        self._patch_data_dir()
        # база прежней версии: таблица jobs уже есть (с треком), плюс таблица corpus с профилем
        self._init()
        with self._conn() as c:
            c.execute(_OLD_CORPUS_DDL)
            c.execute("INSERT INTO corpus (name, created_at) VALUES ('блюз 60-х', '2026-01-01T00:00:00')")
            cur = c.execute("INSERT INTO jobs (title, status, style, lyrics, created_at) "
                            "VALUES ('keep me', 'done', 'rock', '', '2026-01-01T00:00:00')")
            self.job_id = cur.lastrowid
        corpus = self.data_dir / "corpus" / "1" / "tracks" / "song"
        corpus.mkdir(parents=True)
        (corpus / "info.json").write_text('{"filename": "song.flac"}')
        (self.data_dir / "corpus" / "1" / "profile.json").write_text("{}")
        # чужой каталог данных рядом — не трогается
        self.voices_dir = self.data_dir / "voices_keep_probe"
        self.voices_dir.mkdir()
        (self.voices_dir / "v.json").write_text("{}")
        self.assertIn("corpus", self._tables())

    def test_init_drops_table_and_directory(self):
        self._init()
        self.assertNotIn("corpus", self._tables(), "таблица corpus осталась")
        self.assertFalse((self.data_dir / "corpus").exists(), "каталог <data>/corpus остался")

    def test_other_data_untouched(self):
        self._init()
        with self._conn() as c:
            row = c.execute("SELECT title FROM jobs WHERE id=?", (self.job_id,)).fetchone()
        self.assertIsNotNone(row, "трек из jobs пропал")
        self.assertEqual(row["title"], "keep me")
        self.assertTrue((self.voices_dir / "v.json").exists(), "соседний каталог данных тронут")

    def test_removal_is_logged(self):
        with self.assertLogs(_LOGGER, level="INFO") as cm:
            self._init()
        self.assertTrue(any("corpus" in m.lower() or "профил" in m.lower() for m in cm.output),
                        f"нет записи в лог об удалении профилей: {cm.output}")

    def test_second_init_is_noop_without_errors(self):
        self._init()
        seen = []

        class _Rec(logging.Handler):
            def emit(self, record):
                seen.append(record.getMessage())

        h = _Rec(level=logging.INFO)
        lg = logging.getLogger(_LOGGER)
        old_level = lg.level
        lg.addHandler(h)
        lg.setLevel(logging.INFO)
        try:
            self._init()  # не падает
        finally:
            lg.removeHandler(h)
            lg.setLevel(old_level)
        self.assertNotIn("corpus", self._tables())
        self.assertFalse((self.data_dir / "corpus").exists())
        self.assertEqual([m for m in seen if "corpus" in m.lower() or "профил" in m.lower()], [],
                         "повторный запуск снова пишет об удалении профилей")

    def test_directory_without_table_also_removed(self):
        with self._conn() as c:
            c.execute("DROP TABLE corpus")
        self._init()
        self.assertFalse((self.data_dir / "corpus").exists())

    def test_table_without_directory_also_dropped(self):
        import shutil
        shutil.rmtree(self.data_dir / "corpus")
        self._init()
        self.assertNotIn("corpus", self._tables())


if __name__ == "__main__":
    unittest.main()
