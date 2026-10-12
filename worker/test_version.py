"""Спецификация версии воркера.

Что должно работать:
V1. GET /version → {version: str, api: int, source_hash: str (12 hex), model: str,
    components: {whisper: bool, seedvc: bool, roformer: bool}, stems_model: str}.
V2. version — строка из файла VERSION рядом с yue_worker.py (без пробелов и перевода
    строки по краям); файла нет — "dev".
V3. api — целое WORKER_API ≥ 1.
V4. source_hash — 12 hex sha256 по коду воркера: *.py (кроме test_*.py) и *.json в
    каталоге воркера (не рекурсивно); меняется при правке .py/.json, не зависит от
    test_*.py и прочих файлов.
V5. components и stems_model — та же правда, что /config (whisper_available,
    seedvc_available, roformer_available, stems_model).
"""
import re
import shutil
import tempfile
import unittest
from pathlib import Path

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase, _import_worker

HEX12 = re.compile(r"^[0-9a-f]{12}$")
WORKER_DIR = Path(__file__).resolve().parent


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestVersionEndpoint(_WorkerApiCase):
    """V1, V3, V5: поля и типы ответа /version."""

    def _get(self):
        r = self.client.get("/version")
        self.assertEqual(r.status_code, 200, r.text)
        return r.json()

    def test_fields_and_types(self):
        v = self._get()
        self.assertEqual(set(v), {"version", "api", "source_hash", "model", "engine", "components", "stems_model"})
        self.assertIsInstance(v["version"], str)
        self.assertTrue(v["version"])
        self.assertIsInstance(v["api"], int)
        self.assertNotIsInstance(v["api"], bool)
        self.assertRegex(v["source_hash"], HEX12)
        self.assertIsInstance(v["model"], str)
        self.assertIsInstance(v["stems_model"], str)
        self.assertEqual(set(v["components"]), {"whisper", "seedvc", "roformer"})
        for k, b in v["components"].items():
            self.assertIsInstance(b, bool, k)

    def test_api_is_worker_api(self):
        self.assertIsInstance(self.w.WORKER_API, int)
        self.assertGreaterEqual(self.w.WORKER_API, 1)
        self.assertEqual(self._get()["api"], self.w.WORKER_API)

    def test_version_from_worker_dir(self):
        """version — то, что лежит в VERSION рядом с yue_worker.py (или "dev")."""
        here = Path(self.w.__file__).resolve().parent
        self.assertEqual(self._get()["version"], self.w._read_version(here))

    def test_source_hash_of_worker_dir(self):
        here = Path(self.w.__file__).resolve().parent
        self.assertEqual(self._get()["source_hash"], self.w._source_hash(here))

    def test_components_match_config(self):
        v = self._get()
        c = self.client.get("/config").json()
        self.assertEqual(v["components"]["whisper"], c["whisper_available"])
        self.assertEqual(v["components"]["seedvc"], c["seedvc_available"])
        self.assertEqual(v["components"]["roformer"], c["roformer_available"])
        self.assertEqual(v["stems_model"], c["stems_model"])


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestReadVersion(unittest.TestCase):
    """V2: строка из VERSION; нет файла — "dev"."""

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.w = _import_worker(self._td.name)
        self.dir = Path(tempfile.mkdtemp(dir=self._td.name))

    def test_no_file_is_dev(self):
        self.assertEqual(self.w._read_version(self.dir), "dev")

    def test_file_content(self):
        (self.dir / "VERSION").write_text("v0.12.0-3-gabc1234\n")
        self.assertEqual(self.w._read_version(self.dir), "v0.12.0-3-gabc1234")

    def test_strips_spaces(self):
        (self.dir / "VERSION").write_text("  1.4.2 \n\n")
        self.assertEqual(self.w._read_version(self.dir), "1.4.2")

    def test_other_files_ignored(self):
        (self.dir / "version.txt").write_text("9.9.9\n")
        self.assertEqual(self.w._read_version(self.dir), "dev")


@unittest.skipUnless(_HAS_WORKER_DEPS, "нужны fastapi/httpx/numpy (окружение воркера)")
class TestSourceHash(unittest.TestCase):
    """V4: отпечаток кода воркера."""

    def setUp(self):
        self._td = tempfile.TemporaryDirectory()
        self.addCleanup(self._td.cleanup)
        self.w = _import_worker(self._td.name)
        self.dir = Path(tempfile.mkdtemp(dir=self._td.name))
        (self.dir / "yue_worker.py").write_text("print('worker')\n")
        (self.dir / "llm.py").write_text("X = 1\n")
        (self.dir / "fx_blocks.json").write_text('{"a": 1}\n')
        (self.dir / "test_llm.py").write_text("# тест\n")

    def h(self):
        return self.w._source_hash(self.dir)

    def test_format(self):
        self.assertRegex(self.h(), HEX12)

    def test_deterministic(self):
        self.assertEqual(self.h(), self.h())
        # тот же код в другом каталоге — тот же отпечаток
        other = Path(tempfile.mkdtemp(dir=self._td.name))
        for p in self.dir.iterdir():
            shutil.copy(p, other / p.name)
        self.assertEqual(self.w._source_hash(other), self.h())

    def test_changes_on_py_edit(self):
        before = self.h()
        (self.dir / "llm.py").write_text("X = 2\n")
        self.assertNotEqual(self.h(), before)

    def test_changes_on_py_added(self):
        before = self.h()
        (self.dir / "media.py").write_text("Y = 1\n")
        self.assertNotEqual(self.h(), before)

    def test_changes_on_json_edit(self):
        before = self.h()
        (self.dir / "fx_blocks.json").write_text('{"a": 2}\n')
        self.assertNotEqual(self.h(), before)

    def test_ignores_test_files(self):
        before = self.h()
        (self.dir / "test_llm.py").write_text("# правка теста\n")
        (self.dir / "test_new.py").write_text("# новый тест\n")
        self.assertEqual(self.h(), before)

    def test_ignores_other_files(self):
        before = self.h()
        (self.dir / "VERSION").write_text("1.0.0\n")
        (self.dir / "requirements.txt").write_text("fastapi\n")
        (self.dir / "notes.md").write_text("# заметки\n")
        self.assertEqual(self.h(), before)

    def test_not_recursive(self):
        before = self.h()
        sub = self.dir / "phrases"
        sub.mkdir()
        (sub / "extra.py").write_text("Z = 1\n")
        (sub / "data.json").write_text("{}\n")
        pyc = self.dir / "__pycache__"
        pyc.mkdir()
        (pyc / "llm.cpython-312.pyc").write_bytes(b"\x00\x01")
        self.assertEqual(self.h(), before)


if __name__ == "__main__":
    unittest.main()
