"""Спецификация: страница проекта на GitHub Pages, ru/en (internal-site).

Что должно работать:
ТК1 tools/site-build.sh OUT: в OUT лежат index.html, logo.png, clips.json (побайтно = frontend/src/welcomeClips.json)
    и clips/*.mp3 — каждый клип, упомянутый в clips.json, есть в OUT/clips.
ТК2 site/index.html: словари T.en и T.ru — одинаковые наборы ключей; длины списков features/req/install совпадают.
ТК3 Ссылки «Скачать» — releases/latest/download/partscaster-windows-amd64.zip и
    releases/latest/download/partscaster-linux-amd64.tar.gz; ссылка «все файлы» — releases/latest.
ТК4 .github/workflows/pages.yml — валидный YAML; push.paths включают site/**, internal/welcome/clips/**,
    frontend/src/welcomeClips.json; шаги configure-pages, upload-pages-artifact (path _site), deploy-pages;
    permissions pages: write, id-token: write.

Без node ТК2 пропускается с причиной, без PyYAML — ТК4 (skip, а не молчаливый проход).
"""
import json
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
SITE_BUILD = REPO / "tools" / "site-build.sh"
INDEX = REPO / "site" / "index.html"
CLIPS_JSON = REPO / "frontend" / "src" / "welcomeClips.json"
PAGES_WF = REPO / ".github" / "workflows" / "pages.yml"

try:
    import yaml  # noqa: F401
    HAVE_YAML = True
except ImportError:  # окружение без PyYAML: ТК4 пропускается явно, см. skipUnless
    HAVE_YAML = False

NODE = shutil.which("node")


def _read(p):
    return Path(p).read_text(encoding="utf-8")


def _mp3_names(obj):
    """Все строки *.mp3 из clips.json, где бы они ни лежали."""
    out = set()
    if isinstance(obj, dict):
        for v in obj.values():
            out |= _mp3_names(v)
    elif isinstance(obj, list):
        for v in obj:
            out |= _mp3_names(v)
    elif isinstance(obj, str) and obj.lower().endswith(".mp3"):
        out.add(obj)
    return out


class TestSiteBuild(unittest.TestCase):
    """ТК1."""

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="site-build-"))
        self.addCleanup(shutil.rmtree, self.tmp, ignore_errors=True)
        self.out = self.tmp / "out"
        r = subprocess.run(["bash", str(SITE_BUILD), str(self.out)], cwd=REPO,
                           capture_output=True, text=True, timeout=120)
        self.assertEqual(r.returncode, 0, f"site-build.sh упал:\n{r.stdout}\n{r.stderr}")

    def test_index_and_logo(self):
        self.assertTrue((self.out / "index.html").is_file())
        self.assertTrue((self.out / "logo.png").is_file())
        self.assertGreater((self.out / "logo.png").stat().st_size, 0)

    def test_clips_json_equals_welcome_clips(self):
        self.assertEqual((self.out / "clips.json").read_bytes(), CLIPS_JSON.read_bytes())

    def test_all_clips_present(self):
        names = _mp3_names(json.loads(_read(self.out / "clips.json")))
        self.assertTrue(names, "в clips.json не найдено ни одного клипа")
        missing = sorted(n for n in names if not (self.out / "clips" / Path(n).name).is_file())
        self.assertEqual(missing, [], "клипов нет в OUT/clips")

    def test_clips_not_duplicated_in_site(self):
        # клипы при сборке копируются в OUT, а не дублируются в site/
        self.assertFalse(list((REPO / "site").rglob("*.mp3")), "mp3 в site/ — клипы продублированы")


_EXTRACT_T = r"""
const fs = require('fs');
const src = fs.readFileSync(process.argv[1], 'utf8');
const m = src.indexOf('const T =');
if (m < 0) { console.error('const T не найден'); process.exit(3); }
let i = src.indexOf('{', m), depth = 0, q = null, start = i;
for (; i < src.length; i++) {
  const c = src[i];
  if (q) { if (c === '\\') { i++; continue; } if (c === q) q = null; continue; }
  if (c === '"' || c === "'" || c === '`') { q = c; continue; }
  if (c === '/' && src[i + 1] === '/') { i = src.indexOf('\n', i); continue; }
  if (c === '/' && src[i + 1] === '*') { i = src.indexOf('*/', i) + 1; continue; }
  if (c === '{') depth++;
  else if (c === '}') { depth--; if (depth === 0) break; }
}
const T = new Function('return (' + src.slice(start, i + 1) + ');')();
process.stdout.write(JSON.stringify(T));
"""


@unittest.skipUnless(NODE, "нет node: объект T из index.html разобрать нечем")
class TestDictionaries(unittest.TestCase):
    """ТК2."""

    @classmethod
    def setUpClass(cls):
        r = subprocess.run([NODE, "-e", _EXTRACT_T, str(INDEX)], capture_output=True, text=True, timeout=30)
        if r.returncode != 0:
            raise AssertionError(f"не удалось разобрать T из index.html:\n{r.stderr}")
        cls.T = json.loads(r.stdout)

    def test_both_languages(self):
        self.assertIn("en", self.T)
        self.assertIn("ru", self.T)

    def test_same_keys(self):
        en, ru = set(self.T["en"]), set(self.T["ru"])
        self.assertTrue(en, "словарь en пуст")
        self.assertEqual(sorted(en - ru), [], "ключи есть в en, нет в ru")
        self.assertEqual(sorted(ru - en), [], "ключи есть в ru, нет в en")

    def test_list_lengths_match(self):
        for key in ("features", "req", "install"):
            with self.subTest(key=key):
                en, ru = self.T["en"].get(key), self.T["ru"].get(key)
                self.assertIsInstance(en, list, f"en.{key} — не список")
                self.assertIsInstance(ru, list, f"ru.{key} — не список")
                self.assertTrue(en, f"en.{key} пуст")
                self.assertEqual(len(en), len(ru), f"{key}: разная длина en/ru")

    def test_values_non_empty(self):
        for lang in ("en", "ru"):
            for k, v in self.T[lang].items():
                with self.subTest(lang=lang, key=k):
                    if isinstance(v, str):
                        self.assertTrue(v.strip(), f"{lang}.{k} пустая строка")


class TestDownloadLinks(unittest.TestCase):
    """ТК3."""

    def setUp(self):
        self.hrefs = re.findall(r'href="([^"]+)"', _read(INDEX))

    def _one_ending(self, tail):
        found = [h for h in self.hrefs if h.endswith(tail)]
        self.assertTrue(found, f"нет ссылки, оканчивающейся на {tail}")
        for h in found:
            self.assertRegex(h, r"^https://github\.com/[^/]+/[^/]+/" + re.escape(tail) + "$")

    def test_windows(self):
        self._one_ending("releases/latest/download/partscaster-windows-amd64.zip")

    def test_linux(self):
        self._one_ending("releases/latest/download/partscaster-linux-amd64.tar.gz")

    def test_all_files(self):
        self._one_ending("releases/latest")

    def test_no_versioned_release_links(self):
        # ссылки не должны указывать на конкретную версию — только latest
        bad = [h for h in self.hrefs if "/releases/download/" in h or "/releases/tag/" in h]
        self.assertEqual(bad, [])


@unittest.skipUnless(HAVE_YAML, "нет PyYAML: pages.yml не разобрать")
class TestPagesWorkflow(unittest.TestCase):
    """ТК4."""

    @classmethod
    def setUpClass(cls):
        cls.wf = yaml.safe_load(_read(PAGES_WF))

    def _on(self):
        # PyYAML читает ключ `on` как True
        return self.wf.get("on", self.wf.get(True))

    def _steps(self):
        steps = []
        for job in self.wf["jobs"].values():
            steps += job.get("steps", [])
        return steps

    def _step(self, action):
        found = [s for s in self._steps() if str(s.get("uses", "")).startswith(f"actions/{action}@")]
        self.assertEqual(len(found), 1, f"шаг actions/{action}: найдено {len(found)}")
        return found[0]

    def test_push_paths(self):
        paths = self._on()["push"]["paths"]
        for p in ("site/**", "internal/welcome/clips/**", "frontend/src/welcomeClips.json"):
            self.assertIn(p, paths)

    def test_push_branches(self):
        branches = self._on()["push"].get("branches", [])
        self.assertIn("develop", branches)
        self.assertIn("main", branches)

    def test_steps(self):
        self._step("configure-pages")
        self._step("deploy-pages")
        upload = self._step("upload-pages-artifact")
        self.assertEqual(str(upload.get("with", {}).get("path", "")).rstrip("/"), "_site")

    def test_builds_with_site_build(self):
        runs = " ".join(str(s.get("run", "")) for s in self._steps())
        self.assertRegex(runs, r"tools/site-build\.sh\s+_site\b")

    def test_permissions(self):
        perms = dict(self.wf.get("permissions") or {})
        for job in self.wf["jobs"].values():
            perms.update(job.get("permissions") or {})
        self.assertEqual(perms.get("pages"), "write")
        self.assertEqual(perms.get("id-token"), "write")


if __name__ == "__main__":
    unittest.main()
