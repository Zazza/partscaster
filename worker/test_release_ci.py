"""Спецификация: релиз в GitHub Actions и заранее собранный yue2.cpp (internal-release-ci).

Что должно работать:
ТК1 Пин коммита yue2.cpp. Источник правды — CPP_COMMIT в worker/engine_cpp.py. Копии:
    tools/yue2cpp-build.sh (COMMIT="<hex>"), tools/yue2cpp-build.ps1 ($Commit = '<hex>'),
    worker/install.sh (CPP_COMMIT="<hex>") — равны ему. Workflow .github/workflows/yue2cpp.yml
    своего литерала коммита не содержит: берёт его из worker/engine_cpp.py.
ТК2 Синтаксис workflow. yue2cpp.yml и release.yml — валидный YAML. yue2cpp.yml запускается на push
    (paths: сам файл, tools/yue2cpp-build.ps1, tools/yue2cpp-build.sh, worker/engine_cpp.py) и
    workflow_dispatch. В release.yml шаг выкладки (softprops/action-gh-release) берёт файлы из
    dist/release/ (постоянные имена), prerelease и make_latest зависят от дефиса в имени тега.
ТК3 Постоянные имена. tools/release-assets.sh DIR: DIR/partscaster-<V>-<os>-<arch>.<zip|tar.gz> →
    копия DIR/release/partscaster-<os>-<arch>.<ext>, побайтно та же; печатает пути созданных файлов;
    DIR/worker-<V>.tar.gz не трогает и в release/ не кладёт.

ТК4 Права на запуск (internal-ci-fixes ТК1). Каждый tools/<имя>.sh, который run-строка workflow в
    .github/workflows запускает, лежит в индексе git с режимом 100755 (git ls-files -s). Без git или вне
    репозитория — skip с причиной.
ТК5 Linux-сборка релиза (internal-ci-fixes ТК2). Матрица os в release.yml содержит ubuntu-24.04 и не
    содержит ubuntu-latest.

Без PyYAML проверки ТК2, ТК4, ТК5 пропускаются с причиной (skip), а не проходят молча.
"""
import hashlib
import os
import re
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path

WORKER_DIR = Path(__file__).resolve().parent
REPO = WORKER_DIR.parent
WF_DIR = REPO / ".github" / "workflows"
YUE2CPP_WF = WF_DIR / "yue2cpp.yml"
RELEASE_WF = WF_DIR / "release.yml"
ASSETS_SH = REPO / "tools" / "release-assets.sh"

try:
    import yaml  # noqa: F401
    HAVE_YAML = True
except ImportError:  # окружение без PyYAML: ТК2 пропускается явно, см. skipUnless
    HAVE_YAML = False


def _read(p):
    return Path(p).read_text(encoding="utf-8")


def _one(pattern, text, where):
    found = re.findall(pattern, text, flags=re.MULTILINE)
    if len(found) != 1:
        raise AssertionError(f"{where}: ожидалось ровно одно совпадение {pattern!r}, найдено {found!r}")
    return found[0]


def _engine_commit():
    return _one(r'^CPP_COMMIT\s*=\s*"([0-9a-f]{7,40})"', _read(WORKER_DIR / "engine_cpp.py"),
                "worker/engine_cpp.py")


class TestCppCommitPin(unittest.TestCase):
    """ТК1."""

    def setUp(self):
        self.commit = _engine_commit()

    def test_engine_commit_is_hex(self):
        self.assertRegex(self.commit, r"^[0-9a-f]{7,40}$")

    def test_build_sh_matches(self):
        got = _one(r'^\s*COMMIT="([0-9a-fA-F]+)"', _read(REPO / "tools" / "yue2cpp-build.sh"),
                   "tools/yue2cpp-build.sh")
        self.assertEqual(got, self.commit)

    def test_build_ps1_matches(self):
        got = _one(r"^\s*\$Commit\s*=\s*'([0-9a-fA-F]+)'", _read(REPO / "tools" / "yue2cpp-build.ps1"),
                   "tools/yue2cpp-build.ps1")
        self.assertEqual(got, self.commit)

    def test_install_sh_matches(self):
        got = _one(r'^\s*CPP_COMMIT="([0-9a-fA-F]+)"', _read(WORKER_DIR / "install.sh"),
                   "worker/install.sh")
        self.assertEqual(got, self.commit)

    def test_workflow_has_no_commit_literal(self):
        text = _read(YUE2CPP_WF)
        self.assertNotIn(self.commit, text)
        # и короткий префикс тоже не вписан руками
        self.assertNotIn(self.commit[:7], text)

    def test_workflow_reads_commit_from_engine_cpp(self):
        text = _read(YUE2CPP_WF)
        self.assertIn("engine_cpp.py", text)
        self.assertIn("CPP_COMMIT", text)


def _load(p):
    import yaml
    data = yaml.safe_load(_read(p))
    assert isinstance(data, dict), f"{p}: корень YAML не словарь"
    return data


def _triggers(wf):
    # PyYAML (YAML 1.1) читает ключ `on` как True
    return wf.get("on", wf.get(True))


def _steps(wf):
    for job in (wf.get("jobs") or {}).values():
        yield from job.get("steps") or []


@unittest.skipUnless(HAVE_YAML, "нет PyYAML: синтаксис workflow не проверен (pip install pyyaml)")
class TestWorkflowsSyntax(unittest.TestCase):
    """ТК2."""

    def test_both_valid_yaml(self):
        for p in (YUE2CPP_WF, RELEASE_WF):
            with self.subTest(p=p.name):
                wf = _load(p)
                self.assertTrue(wf.get("jobs"), f"{p.name}: нет jobs")

    def test_yue2cpp_triggers(self):
        on = _triggers(_load(YUE2CPP_WF))
        self.assertIsInstance(on, dict)
        self.assertIn("workflow_dispatch", on)
        push = on.get("push")
        self.assertIsInstance(push, dict, "push без настроек: нет paths")
        paths = push.get("paths") or []
        for need in (".github/workflows/yue2cpp.yml", "tools/yue2cpp-build.ps1",
                     "tools/yue2cpp-build.sh", "worker/engine_cpp.py"):
            with self.subTest(path=need):
                self.assertTrue(
                    any(p == need or (("*" in p) and re.fullmatch(
                        re.escape(p).replace(r"\*\*", ".*").replace(r"\*", "[^/]*"), need))
                        for p in paths),
                    f"{need} не покрыт paths={paths}")

    def _release_step(self):
        steps = [s for s in _steps(_load(RELEASE_WF))
                 if str(s.get("uses", "")).startswith("softprops/action-gh-release")]
        self.assertEqual(len(steps), 1, "ожидался ровно один шаг softprops/action-gh-release")
        return steps[0].get("with") or {}

    def test_release_files_from_dist_release(self):
        files = str(self._release_step().get("files", ""))
        lines = [ln.strip() for ln in files.splitlines() if ln.strip()]
        self.assertTrue(any("dist/release/" in ln for ln in lines), f"files={files!r}")
        # пакеты приложения с версией в имени не выкладываются
        for ln in lines:
            self.assertNotRegex(ln, r"dist/partscaster-", f"пакет с версией в files: {ln}")

    def test_prerelease_and_latest_depend_on_dash(self):
        w = self._release_step()
        for key in ("prerelease", "make_latest"):
            with self.subTest(key=key):
                val = str(w.get(key, ""))
                self.assertIn("${{", val)
                self.assertRegex(val, r"contains\(\s*github\.ref_name\s*,\s*'-'\s*\)")
        # смысл: тег с дефисом — предварительный и не последний; без дефиса — наоборот
        for has_dash in (True, False):
            with self.subTest(has_dash=has_dash):
                self.assertEqual(_eval_gh_expr(w.get("prerelease"), has_dash), has_dash)
                self.assertEqual(_eval_gh_expr(w.get("make_latest"), has_dash), not has_dash)


def _run_strings(wf):
    for job in (wf.get("jobs") or {}).values():
        for step in job.get("steps") or []:
            if "run" in step:
                yield str(step["run"])


# tools/<имя>.sh в позиции пути: не часть другого пути/слова; ./tools/... тоже считается
_TOOLS_SH = re.compile(r"(?<![\w/.-])(?:\./)?(tools/[\w.-]+\.sh)(?![\w.-])")


def _git_repo_reason():
    """None, если git доступен и REPO — рабочее дерево; иначе причина пропуска."""
    if not shutil.which("git"):
        return "нет git: права скриптов в индексе не проверены"
    try:
        r = subprocess.run(["git", "rev-parse", "--is-inside-work-tree"], cwd=REPO,
                           capture_output=True, text=True, timeout=30)
    except OSError as e:
        return f"git не запускается: {e}"
    if r.returncode != 0 or r.stdout.strip() != "true":
        return f"{REPO} не репозиторий git: права скриптов в индексе не проверены"
    return None


@unittest.skipUnless(HAVE_YAML, "нет PyYAML: run-строки workflow не разобраны (pip install pyyaml)")
class TestWorkflowScriptsExecutable(unittest.TestCase):
    """ТК4 (internal-ci-fixes ТК1)."""

    def setUp(self):
        reason = _git_repo_reason()
        if reason:
            self.skipTest(reason)

    def _scripts(self):
        found = set()
        for wf_path in sorted(WF_DIR.glob("*.yml")):
            for run in _run_strings(_load(wf_path)):
                found.update(_TOOLS_SH.findall(run))
        return found

    def test_scripts_found(self):
        # иначе проверка режима ниже прошла бы на пустом множестве
        scripts = self._scripts()
        for need in ("tools/site-build.sh", "tools/release-assets.sh"):
            self.assertIn(need, scripts)

    def test_mode_100755_in_index(self):
        scripts = self._scripts()
        self.assertTrue(scripts, "в run-строках workflow нет ни одного tools/*.sh")
        for path in sorted(scripts):
            with self.subTest(path=path):
                r = subprocess.run(["git", "ls-files", "-s", "--", path], cwd=REPO,
                                   capture_output=True, text=True, timeout=30)
                self.assertEqual(r.returncode, 0, r.stderr)
                lines = [ln for ln in r.stdout.splitlines() if ln.strip()]
                self.assertEqual(len(lines), 1, f"{path} нет в индексе git: {r.stdout!r}")
                mode = lines[0].split()[0]
                self.assertEqual(mode, "100755",
                                 f"{path}: режим {mode} в индексе (нужно git update-index --chmod=+x)")


def _matrix_oses(wf):
    oses = []
    for job in (wf.get("jobs") or {}).values():
        matrix = ((job.get("strategy") or {}).get("matrix")) or {}
        if not isinstance(matrix, dict):
            continue
        os_val = matrix.get("os")
        if isinstance(os_val, list):
            oses.extend(str(o) for o in os_val)
        elif os_val is not None:
            oses.append(str(os_val))
        for inc in matrix.get("include") or []:
            if isinstance(inc, dict) and "os" in inc:
                oses.append(str(inc["os"]))
    return oses


@unittest.skipUnless(HAVE_YAML, "нет PyYAML: матрица release.yml не проверена (pip install pyyaml)")
class TestReleaseLinuxRunner(unittest.TestCase):
    """ТК5 (internal-ci-fixes ТК2)."""

    def test_matrix_has_ubuntu_2404(self):
        self.assertIn("ubuntu-24.04", _matrix_oses(_load(RELEASE_WF)))

    def test_matrix_has_no_ubuntu_latest(self):
        oses = _matrix_oses(_load(RELEASE_WF))
        self.assertTrue(oses, "в release.yml нет матрицы os")
        self.assertNotIn("ubuntu-latest", oses)


def _eval_gh_expr(value, has_dash):
    """Вычисляет простое выражение GitHub Actions при заданном наличии дефиса в теге."""
    if isinstance(value, bool):
        return value
    m = re.fullmatch(r"\s*\$\{\{(.*)\}\}\s*", str(value), flags=re.S)
    if not m:
        raise AssertionError(f"не выражение: {value!r}")
    expr = re.sub(r"contains\(\s*github\.ref_name\s*,\s*'-'\s*\)", str(has_dash), m.group(1))
    expr = expr.replace("&&", " and ").replace("||", " or ")
    expr = re.sub(r"!(?!=)", " not ", expr)
    expr = re.sub(r"\btrue\b", "True", expr)
    expr = re.sub(r"\bfalse\b", "False", expr)
    if re.search(r"[A-Za-z_]\w*\s*\(|github\.", expr):
        raise AssertionError(f"неожиданное выражение: {value!r}")
    res = eval(expr, {"__builtins__": {}}, {})
    if isinstance(res, str):
        return {"true": True, "false": False}[res.lower()]
    return bool(res)


def _sha(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()


@unittest.skipUnless(shutil.which("bash"), "нужен bash")
class TestReleaseAssets(unittest.TestCase):
    """ТК3."""

    PACKAGES = {
        "partscaster-1.0.0-rc1-windows-amd64.zip": "partscaster-windows-amd64.zip",
        "partscaster-v0.7.0-100-g3dc4fbb-linux-amd64.tar.gz": "partscaster-linux-amd64.tar.gz",
        "partscaster-1.0.0-darwin-arm64.tar.gz": "partscaster-darwin-arm64.tar.gz",
    }
    WORKER = "worker-1.0.0-rc1.tar.gz"

    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp(prefix="relassets-"))
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.dist = self.tmp / "dist"
        self.dist.mkdir()
        self.src_sha = {}
        for i, name in enumerate(list(self.PACKAGES) + [self.WORKER]):
            p = self.dist / name
            p.write_bytes(os.urandom(256) + name.encode() + bytes([i]))
            self.src_sha[name] = _sha(p)

    def _run(self):
        return subprocess.run(["bash", str(ASSETS_SH), str(self.dist)], capture_output=True,
                              text=True, timeout=60)

    def test_copies_with_stable_names(self):
        r = self._run()
        self.assertEqual(r.returncode, 0, r.stderr)
        rel = self.dist / "release"
        self.assertEqual(sorted(p.name for p in rel.iterdir()), sorted(self.PACKAGES.values()))
        for src, dst in self.PACKAGES.items():
            with self.subTest(src=src):
                self.assertEqual(_sha(rel / dst), self.src_sha[src], "копия не побайтно равна")

    def test_prints_created_paths(self):
        r = self._run()
        self.assertEqual(r.returncode, 0, r.stderr)
        printed = [ln.strip() for ln in r.stdout.splitlines() if ln.strip()]
        expected = {(self.dist / "release" / n).resolve() for n in self.PACKAGES.values()}
        got = {Path(ln).resolve() if os.path.isabs(ln) else (Path.cwd() / ln).resolve()
               for ln in printed}
        self.assertEqual(got, expected, r.stdout)

    def test_sources_and_worker_untouched(self):
        r = self._run()
        self.assertEqual(r.returncode, 0, r.stderr)
        for name, sha in self.src_sha.items():
            with self.subTest(name=name):
                self.assertTrue((self.dist / name).is_file(), f"{name} пропал")
                self.assertEqual(_sha(self.dist / name), sha)
        self.assertFalse(any("worker" in p.name for p in (self.dist / "release").iterdir()))
        self.assertNotIn("worker", r.stdout)

    def test_no_packages_creates_nothing(self):
        for name in self.PACKAGES:
            (self.dist / name).unlink()
        r = self._run()
        rel = self.dist / "release"
        self.assertEqual(list(rel.iterdir()) if rel.exists() else [], [])
        self.assertEqual(r.stdout.strip(), "")


if __name__ == "__main__":
    unittest.main()
