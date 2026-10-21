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


# ---------------------------------------------------------------------------
# internal-screenshots: снимки приложения для README и сайта.
# ТК1 Все картинки README.md/README.ru.md существуют; в README.md — только *-en.png, в README.ru.md —
#     только *-ru.png; заглушек «<!-- screenshot» / «<!-- скриншот» не осталось.
# ТК2 T.en.shots и T.ru.shots — одинаковые имена; на каждое имя есть docs/img/<имя>-en.png и -ru.png.
# ТК3 tools/site-build.sh OUT кладёт OUT/img/*.png (все из docs/img); pages.yml push.paths содержит docs/img/**.
# ТК4 Каждый docs/img/*.png <= 100 КБ.
# ---------------------------------------------------------------------------

IMG_DIR = REPO / "docs" / "img"
README_LANG = {"README.md": "en", "README.ru.md": "ru"}
_MD_IMG = re.compile(r"!\[([^\]]*)\]\(\s*<?([^)\s>]+)>?(?:\s+\"[^\"]*\")?\s*\)")
_HTML_IMG = re.compile(r"<img\b[^>]*\bsrc=[\"']([^\"']+)[\"']", re.I)


def _png_files():
    return sorted(IMG_DIR.glob("*.png")) if IMG_DIR.is_dir() else []


def _readme_images(name):
    """[(alt, путь)] — картинки markdown и <img>, только локальные (не http)."""
    text = _read(REPO / name)
    out = [(alt, path) for alt, path in _MD_IMG.findall(text)]
    out += [(None, path) for path in _HTML_IMG.findall(text)]
    return [(a, p) for a, p in out if not re.match(r"^[a-z]+://", p, re.I)]


class TestReadmeScreenshots(unittest.TestCase):
    """internal-screenshots ТК1."""

    def test_images_exist(self):
        for name in README_LANG:
            for _alt, path in _readme_images(name):
                with self.subTest(readme=name, path=path):
                    target = (REPO / path.split("#")[0].split("?")[0]).resolve()
                    self.assertTrue(target.is_file(), f"{name}: картинки {path} нет")

    def test_screenshots_present(self):
        # заглушки должны быть заменены снимками, а не просто удалены
        for name in README_LANG:
            with self.subTest(readme=name):
                shots = [p for _a, p in _readme_images(name) if "docs/img/" in p]
                self.assertTrue(shots, f"{name}: нет ни одного снимка из docs/img")

    def test_own_language_only(self):
        for name, lang in README_LANG.items():
            for _alt, path in _readme_images(name):
                if "docs/img/" not in path:
                    continue
                with self.subTest(readme=name, path=path):
                    self.assertRegex(Path(path).name, rf"-{lang}\.png$",
                                     f"{name}: снимок не своего языка ({lang})")

    def test_screenshots_have_alt(self):
        for name in README_LANG:
            for alt, path in _readme_images(name):
                if "docs/img/" not in path:
                    continue
                with self.subTest(readme=name, path=path):
                    self.assertIsNotNone(alt, f"{name}: снимок {path} через <img> — подпись не проверить")
                    self.assertTrue(alt.strip(), f"{name}: у снимка {path} пустая подпись")

    def test_no_placeholders(self):
        for name in README_LANG:
            with self.subTest(readme=name):
                text = _read(REPO / name).lower()
                self.assertNotIn("<!-- screenshot", text)
                self.assertNotIn("<!-- скриншот", text)


@unittest.skipUnless(NODE, "нет node: объект T из index.html разобрать нечем")
class TestSiteShots(unittest.TestCase):
    """internal-screenshots ТК2, изменено internal-site-zoom ТК1: элемент shots — [имя, подпись] или
    [имя, подпись, файл]; с третьим значением снимок общий для обоих языков — docs/img/<файл>.png."""

    @classmethod
    def setUpClass(cls):
        r = subprocess.run([NODE, "-e", _EXTRACT_T, str(INDEX)], capture_output=True, text=True, timeout=30)
        if r.returncode != 0:
            raise AssertionError(f"не удалось разобрать T из index.html:\n{r.stderr}")
        cls.T = json.loads(r.stdout)

    def _shots(self, lang):
        shots = self.T[lang].get("shots")
        self.assertIsInstance(shots, list, f"{lang}.shots — не список")
        self.assertTrue(shots, f"{lang}.shots пуст")
        for item in shots:
            self.assertIsInstance(item, list, f"{lang}.shots: элемент {item!r} — не список")
            self.assertIn(len(item), (2, 3), f"{lang}.shots: элемент {item!r} — не [имя, подпись(, файл)]")
            for i, v in enumerate(item):
                self.assertIsInstance(v, str, f"{lang}.shots: {item!r}[{i}] — не строка")
                self.assertTrue(v.strip(), f"{lang}.shots: {item!r}[{i}] пустое")
        return shots

    def test_same_names(self):
        en = [it[0] for it in self._shots("en")]
        ru = [it[0] for it in self._shots("ru")]
        self.assertEqual(sorted(en), sorted(ru), "имена снимков en и ru различаются")
        self.assertEqual(len(en), len(set(en)), "имя снимка повторяется")

    def test_files_exist(self):
        for lang in ("en", "ru"):
            for item in self._shots(lang):
                name = item[0]
                if len(item) == 3:
                    files = [f"{item[2]}.png"]
                else:
                    files = [f"{name}-en.png", f"{name}-ru.png"]
                for f in files:
                    with self.subTest(lang=lang, name=name, file=f):
                        self.assertTrue((IMG_DIR / f).is_file(), f"нет docs/img/{f}")


class TestSiteBuildImages(unittest.TestCase):
    """internal-screenshots ТК3, часть сборки."""

    @classmethod
    def setUpClass(cls):
        cls.tmp = Path(tempfile.mkdtemp(prefix="site-build-img-"))
        cls.out = cls.tmp / "out"
        r = subprocess.run(["bash", str(SITE_BUILD), str(cls.out)], cwd=REPO,
                           capture_output=True, text=True, timeout=120)
        if r.returncode != 0:
            shutil.rmtree(cls.tmp, ignore_errors=True)
            raise AssertionError(f"site-build.sh упал:\n{r.stdout}\n{r.stderr}")

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tmp, ignore_errors=True)

    def test_all_pngs_copied(self):
        pngs = _png_files()
        self.assertTrue(pngs, "в docs/img нет ни одного png")
        for src in pngs:
            with self.subTest(png=src.name):
                dst = self.out / "img" / src.name
                self.assertTrue(dst.is_file(), f"нет OUT/img/{src.name}")
                self.assertEqual(dst.read_bytes(), src.read_bytes(), f"OUT/img/{src.name} отличается")


@unittest.skipUnless(HAVE_YAML, "нет PyYAML: pages.yml не разобрать")
class TestPagesWatchesImages(unittest.TestCase):
    """internal-screenshots ТК3, часть pages.yml."""

    def test_push_paths_docs_img(self):
        wf = yaml.safe_load(_read(PAGES_WF))
        on = wf.get("on", wf.get(True))
        self.assertIn("docs/img/**", on["push"]["paths"])


class TestImageSize(unittest.TestCase):
    """internal-screenshots ТК4."""

    LIMIT = 100 * 1024

    def test_each_png_under_limit(self):
        pngs = _png_files()
        self.assertTrue(pngs, "в docs/img нет ни одного png")
        for p in pngs:
            with self.subTest(png=p.name):
                size = p.stat().st_size
                self.assertGreater(size, 0, f"{p.name} пустой")
                self.assertLessEqual(size, self.LIMIT, f"{p.name}: {size} байт > 100 КБ")


# ---------------------------------------------------------------------------
# internal-site-zoom: снимки сайта крупно по щелчку; снимки окна установщика воркера.
# ТК1 — см. TestSiteShots выше (третье значение элемента shots — общий файл).
# ТК2 Картинки ../img/… в docs/ru/deployment.md и docs/en/deployment.md существуют.
# ТК3 site/index.html: функция zoom, класс .zoom в стилях, keydown с 'Escape', снимкам #shots img
#     назначается обработчик щелчка, вызывающий zoom. Проверка по коду страницы (браузера в тестах нет).
# ТК4 — TestImageSize выше.
# ---------------------------------------------------------------------------

DEPLOY_DOCS = [REPO / "docs" / lang / "deployment.md" for lang in ("ru", "en")]


class TestDeploymentImages(unittest.TestCase):
    """internal-site-zoom ТК2."""

    def test_img_links_exist(self):
        for doc in DEPLOY_DOCS:
            text = _read(doc)
            paths = [p for _a, p in _MD_IMG.findall(text)] + _HTML_IMG.findall(text)
            imgs = [p for p in paths if p.startswith("../img/")]
            with self.subTest(doc=doc.relative_to(REPO).as_posix()):
                self.assertTrue(imgs, f"{doc.name}: нет ни одной картинки ../img/…")
            for p in imgs:
                with self.subTest(doc=doc.relative_to(REPO).as_posix(), path=p):
                    target = (doc.parent / p.split("#")[0].split("?")[0]).resolve()
                    self.assertTrue(target.is_file(), f"{doc}: картинки {p} нет")


class TestSiteZoom(unittest.TestCase):
    """internal-site-zoom ТК3."""

    @classmethod
    def setUpClass(cls):
        src = _read(INDEX)
        cls.style = "\n".join(re.findall(r"<style\b[^>]*>([\s\S]*?)</style>", src, re.I))
        cls.script = "\n".join(re.findall(r"<script\b[^>]*>([\s\S]*?)</script>", src, re.I))
        cls.html = src

    def test_zoom_function(self):
        self.assertRegex(self.script, r"\bfunction\s+zoom\s*\(", "нет function zoom")

    def test_zoom_class_styled(self):
        self.assertRegex(self.style, r"(^|[\s,}])\.zoom\b[^{]*\{", "в стилях нет класса .zoom")
        self.assertRegex(self.script, r"""['"`]zoom['"`]""", "класс zoom не назначается в скрипте")

    def test_escape_closes(self):
        self.assertRegex(self.script, r"""addEventListener\(\s*['"]keydown['"]|\.onkeydown\s*=""",
                         "нет обработчика keydown")
        self.assertRegex(self.script, r"""\.key\s*===?\s*['"]Escape['"]|['"]Escape['"]\s*===?\s*\w+\.key""",
                         "нет проверки клавиши 'Escape'")

    def test_shots_click_opens_zoom(self):
        self.assertRegex(self.html, r"""id=["']shots["']""", "нет контейнера #shots")
        self.assertRegex(self.script, r"#shots\s+img", "снимки #shots img не выбираются в скрипте")
        click = r"""(\.onclick\s*=|addEventListener\(\s*['"]click['"]|onclick\s*=\s*["'])"""
        self.assertRegex(self.script, click + r"[\s\S]{0,300}?\bzoom\s*\(",
                         "обработчик щелчка не вызывает zoom")


if __name__ == "__main__":
    unittest.main()
