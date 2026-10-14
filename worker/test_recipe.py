"""Спецификация чистой сборки рецепта трека: recipe.build_recipe.

Что должно работать:
R1. build_recipe(jobs, ops, root_id, worker_model, stems_model, exported_at) → dict:
    format "yue-track-recipe", version 1, exported_at/worker_model/stems_model как
    переданы, track {title, folder, head}, root {id "n1", title, style, lyrics, seed,
    cot, arc, max_tokens, temperature, cfg, abc, plan_abc}, ops, warnings.
R2. Узлы: n1 — корень, далее n2, n3… по порядку появления result "job:<id>" в
    журнале (порядок журнала = id записи, не id трека).
R3. ops — записи журнала узлов рецепта по id журнала: {op, node, params, result,
    refs}; node — n-id трека записи; result "job:<id>" → "job:nK"; refs → n-id;
    в params значения ключей parent_id, child_id, job_id, voice_src, head_id на
    любой глубине (в т.ч. в списках), равные id узла, → n-id; 0 — как есть; прочие
    ключи не трогаются.
R4. Запись, чьи refs/ключи ссылаются на трек вне рецепта, пропускается + warning.
R5. Версии песни, не созданные журналом (до журнала), в рецепт не входят + warning;
    head вне рецепта → head = "n1" + warning; без таких случаев warnings пуст.
R6. Отказ RecipeError: корень — черновик; корень — импорт чужого аудио
    (style «(импорт внешнего трека)»); в песне есть версия role=voice.

jobs — строки таблицы jobs (столбцы как в БД), ops — строки журнала с params-объектом.
"""
import copy
import json
import os
import unittest
from pathlib import Path

import recipe

IMPORT_STYLE = "(импорт внешнего трека)"
EXPORTED_AT = "2026-10-11T12:00:00+03:00"


def _job(jid, parent_id=None, role="", **kw):
    """Строка jobs: значения как у готового трека."""
    row = {
        "id": jid, "title": f"track {jid}", "status": "done",
        "style": "dark rock, male baritone", "lyrics": "[verse] la la",
        "seed": 1000 + jid, "cot": "full", "arc": "", "max_tokens": 0,
        "temperature": 0.0, "cfg": 0.0, "parent_id": parent_id, "role": role,
        "overdub_of": None, "overdub_gain": 0.5, "cont_from": 0.0,
        "voice_src": None, "head_id": None, "folder": "", "draft": 0,
        "req_abc": "", "abc_file": "", "duration_sec": 60.0,
        "created_at": "2026-10-01T00:00:00", "finished_at": "2026-10-01T00:05:00",
    }
    row.update(kw)
    return row


def _op(oid, job_id, op, params=None, result="", refs=None):
    return {"id": oid, "job_id": job_id, "op": op, "params": params or {},
            "result": result, "refs": refs or [],
            "created_at": f"2026-10-02T00:00:{oid:02d}"}


def _build(jobs, ops, root_id, worker_model="yue2-test", stems_model="roformer"):
    return recipe.build_recipe({j["id"]: j for j in jobs}, ops, root_id,
                               worker_model, stems_model, EXPORTED_AT)


class _Song:
    """Песня: корень 3, продолжение 5 (журнал), вариант 7 с голосом от 5
    (журнал), правка fx на 7, head = 7."""

    def make(self):
        root = _job(3, title="Warm Wire", folder="Альбом", head_id=7,
                    seed=123, cot="full", arc="calm-build", max_tokens=3000,
                    temperature=0.9, cfg=1.5, lyrics="[verse] warm wire")
        cont = _job(5, parent_id=3, role="continue", cont_from=30.0)
        var = _job(7, parent_id=3, role="variant", voice_src=5)
        ops = [
            _op(10, 3, "continue", {"from_sec": 30.0, "parent_id": 3, "seed": 77},
                "job:5"),
            _op(11, 3, "variant_track",
                {"file": "dsp-a.flac", "title": "v", "voice_src": 5}, "job:7"),
            _op(12, 7, "fx", {"chain": "warm", "amount": 0.5}, "dsp-fx1.flac"),
        ]
        return [root, cont, var], ops


class TestRecipeShape(unittest.TestCase):
    """R1: заголовок рецепта и корень из столбцов."""

    def setUp(self):
        jobs, ops = _Song().make()
        self.r = _build(jobs, ops, 3)

    def test_header(self):
        r = self.r
        self.assertEqual(r["format"], "yue-track-recipe")
        self.assertEqual(r["version"], 1)
        self.assertEqual(r["exported_at"], EXPORTED_AT)
        self.assertEqual(r["worker_model"], "yue2-test")
        self.assertEqual(r["stems_model"], "roformer")

    def test_track(self):
        self.assertEqual(self.r["track"]["title"], "Warm Wire")
        self.assertEqual(self.r["track"]["folder"], "Альбом")
        self.assertEqual(self.r["track"]["head"], "n3")

    def test_root_from_columns(self):
        root = self.r["root"]
        self.assertEqual(root["id"], "n1")
        expect = {"title": "Warm Wire", "style": "dark rock, male baritone",
                  "lyrics": "[verse] warm wire", "seed": 123, "cot": "full",
                  "arc": "calm-build", "max_tokens": 3000, "temperature": 0.9,
                  "cfg": 1.5}
        for k, v in expect.items():
            self.assertEqual(root[k], v, k)
        # планы — ключи есть всегда (текст или null); тексты проверяет API-тест
        self.assertIn("abc", root)
        self.assertIn("plan_abc", root)

    def test_json_serializable(self):
        json.dumps(self.r, ensure_ascii=False)  # рецепт — обычный JSON-файл

    def test_inputs_not_mutated(self):
        jobs, ops = _Song().make()
        jobs0, ops0 = copy.deepcopy(jobs), copy.deepcopy(ops)
        _build(jobs, ops, 3)
        self.assertEqual(jobs, jobs0)
        self.assertEqual(ops, ops0)


class TestRecipeOps(unittest.TestCase):
    """R2–R3: узлы и записи журнала по n-id."""

    def test_two_children_and_fx_in_order(self):
        jobs, ops = _Song().make()
        r = _build(jobs, ops, 3)
        self.assertEqual(r["ops"], [
            {"op": "continue", "node": "n1",
             "params": {"from_sec": 30.0, "parent_id": "n1", "seed": 77},
             "result": "job:n2", "refs": []},
            {"op": "variant_track", "node": "n1",
             "params": {"file": "dsp-a.flac", "title": "v", "voice_src": "n2"},
             "result": "job:n3", "refs": []},
            {"op": "fx", "node": "n3", "params": {"chain": "warm", "amount": 0.5},
             "result": "dsp-fx1.flac", "refs": []},
        ])
        self.assertEqual(r["warnings"], [])

    def test_ops_sorted_by_journal_id(self):
        jobs, ops = _Song().make()
        r = _build(jobs, list(reversed(ops)), 3)
        self.assertEqual([o["op"] for o in r["ops"]], ["continue", "variant_track", "fx"])
        self.assertEqual(r["ops"][0]["result"], "job:n2")

    def test_node_numbers_follow_journal_not_job_ids(self):
        # трек 9 создан журналом раньше трека 5 → 9 = n2, 5 = n3
        jobs = [_job(3), _job(5, parent_id=3, role="continue"),
                _job(9, parent_id=3, role="fragment")]
        ops = [_op(20, 3, "fragment", {"parent_id": 3}, "job:9"),
               _op(21, 3, "continue", {"parent_id": 3}, "job:5"),
               _op(22, 5, "update", {"title": "x"}, "")]
        r = _build(jobs, ops, 3)
        self.assertEqual([o["result"] for o in r["ops"]], ["job:n2", "job:n3", ""])
        self.assertEqual(r["ops"][2]["node"], "n3")

    def test_root_without_journal(self):
        r = _build([_job(3)], [], 3)
        self.assertEqual(r["ops"], [])
        self.assertEqual(r["warnings"], [])
        self.assertEqual(r["track"]["head"], "n1")

    def test_nested_ids_replaced(self):
        jobs = [_job(3), _job(5, parent_id=3, role="continue"),
                _job(7, parent_id=3, role="rebuild")]
        params = {
            "pieces": [{"job_id": 5, "from": 1.0, "to": 2.0}, {"job_id": 3, "from": 4.0}],
            "specs": [{"part": "drums", "child_id": 5, "gain": 5}],
            "nested": {"deep": [{"head_id": 3}]},
            "parent_id": 0,     # 0 — «нет», как есть
            "gain": 5,          # не ключ-ссылка — число не трогается
            "count": 3,
        }
        ops = [_op(30, 3, "continue", {}, "job:5"),
               _op(31, 3, "splice", params, "job:7", refs=[5, 3])]
        r = _build(jobs, ops, 3)
        got = r["ops"][1]
        self.assertEqual(got["params"], {
            "pieces": [{"job_id": "n2", "from": 1.0, "to": 2.0},
                       {"job_id": "n1", "from": 4.0}],
            "specs": [{"part": "drums", "child_id": "n2", "gain": 5}],
            "nested": {"deep": [{"head_id": "n1"}]},
            "parent_id": 0,
            "gain": 5,
            "count": 3,
        })
        self.assertEqual(got["refs"], ["n2", "n1"])
        self.assertEqual(got["result"], "job:n3")

    def test_non_job_result_kept(self):
        jobs = [_job(3)]
        ops = [_op(40, 3, "minus", {"gain": 1}, "minus.flac"),
               _op(41, 3, "update", {"title": "new"}, "")]
        r = _build(jobs, ops, 3)
        self.assertEqual([o["result"] for o in r["ops"]], ["minus.flac", ""])
        self.assertEqual([o["node"] for o in r["ops"]], ["n1", "n1"])


class TestRecipeForeignRefs(unittest.TestCase):
    """R4: запись со ссылкой на трек вне рецепта — пропуск + предупреждение."""

    def test_foreign_splice_skipped_with_warning(self):
        other = _job(50, title="other song")  # корень другой песни
        jobs = [_job(3), other, _job(7, parent_id=3, role="splice")]
        ops = [_op(60, 3, "splice",
                   {"pieces": [{"job_id": 3}, {"job_id": 50}]}, "job:7", refs=[3, 50]),
               _op(61, 3, "update", {"title": "kept"}, "")]
        r = _build(jobs, ops, 3)
        self.assertEqual([o["op"] for o in r["ops"]], ["update"])
        self.assertTrue(r["warnings"])
        for o in r["ops"]:  # чужой id не утёк в рецепт
            self.assertNotIn("50", json.dumps(o))

    def test_foreign_ref_only_in_refs(self):
        jobs = [_job(3), _job(50)]
        ops = [_op(62, 3, "envelope", {"points": [[0, 1]]}, "env.flac", refs=[50])]
        r = _build(jobs, ops, 3)
        self.assertEqual(r["ops"], [])
        self.assertTrue(r["warnings"])

    def test_edit_on_skipped_file_skipped_too(self):
        # splice с чужим куском пропущен → variant_track из его файла
        # тоже пропущен с warning; правка другого файла того же трека остаётся
        jobs = [_job(3), _job(50, title="other"), _job(7, parent_id=3, role="variant"),
                _job(8, parent_id=3, role="variant")]
        ops = [_op(70, 3, "splice", {"pieces": [{"job_id": 3}, {"job_id": 50}]},
                   "dsp-splice-1.flac", refs=[3, 50]),
               _op(71, 3, "variant_track", {"file": "dsp-splice-1.flac", "title": "s"},
                   "job:7"),
               _op(72, 3, "fx", {"source": "mix", "chain": [{"type": "comp"}]},
                   "dsp-fx-2.flac"),
               _op(73, 3, "variant_track", {"file": "dsp-fx-2.flac", "title": "f"}, "job:8")]
        r = _build(jobs, ops, 3)
        self.assertEqual([(o["op"], o["result"]) for o in r["ops"]],
                         [("fx", "dsp-fx-2.flac"), ("variant_track", "job:n2")])
        self.assertEqual(r["ops"][1]["params"]["file"], "dsp-fx-2.flac")
        self.assertTrue(any("dsp-splice-1.flac" in w or "variant_track" in w
                            for w in r["warnings"]), r["warnings"])
        self.assertGreaterEqual(len(r["warnings"]), 2, r["warnings"])  # и splice, и вариант


class TestRecipeOldVersions(unittest.TestCase):
    """R5: версии без журнала и head вне рецепта."""

    def test_child_without_journal_not_in_recipe(self):
        jobs, ops = _Song().make()
        jobs.append(_job(9, parent_id=3, role="rebuild"))  # сделан до журнала
        jobs[0]["head_id"] = 7
        r = _build(jobs, ops, 3)
        self.assertEqual(len(r["ops"]), 3)
        nids = {o["node"] for o in r["ops"]} | {o["result"] for o in r["ops"]}
        self.assertNotIn("job:n4", nids)
        self.assertNotIn("n4", nids)
        self.assertTrue(r["warnings"])

    def test_head_outside_recipe_falls_back_to_root(self):
        jobs = [_job(3, head_id=9), _job(9, parent_id=3, role="rebuild")]
        r = _build(jobs, [], 3)
        self.assertEqual(r["track"]["head"], "n1")
        self.assertTrue(r["warnings"])

    def test_head_on_root(self):
        r = _build([_job(3, head_id=3)], [], 3)
        self.assertEqual(r["track"]["head"], "n1")
        self.assertEqual(r["warnings"], [])


class TestRecipeRefusals(unittest.TestCase):
    """R6: отказ RecipeError с понятным текстом."""

    def _refuse(self, jobs, ops, root_id=3):
        with self.assertRaises(recipe.RecipeError) as cm:
            _build(jobs, ops, root_id)
        self.assertTrue(str(cm.exception).strip(), "текст отказа пуст")

    def test_draft_root(self):
        self._refuse([_job(3, draft=1)], [])

    def test_imported_root(self):
        self._refuse([_job(3, style=IMPORT_STYLE, seed=None, lyrics="")], [])

    def test_voice_version(self):
        jobs, ops = _Song().make()
        jobs.append(_job(11, parent_id=3, role="voice"))
        self._refuse(jobs, ops)

    def test_voice_version_created_from_journal_node(self):
        # голос подменён у варианта 7 (родитель — узел рецепта)
        jobs, ops = _Song().make()
        jobs.append(_job(12, parent_id=7, role="voice"))
        self._refuse(jobs, ops)


# Фикстура общая с Go-тестом ведущего: он проигрывает её на фейке сервиса.
GOLDEN = Path(__file__).resolve().parent.parent / "internal" / "studio" / "testdata" / "recipe_e2e.json"
GOLDEN_ENV = "PARTSCASTER_UPDATE_GOLDEN"


def _e2e_recipe():
    """Корень 3 → continue 5 → fx на 5 (dsp-fx-1.flac) → variant_track 7 из этого
    файла → overdub 9 над 7 → variant_track 11 из overdub-9.flac → set_head 11."""
    jobs = [
        _job(3, title="Warm Wire", folder="Альбом", head_id=11, seed=123,
             arc="calm-build", max_tokens=3000, temperature=0.9, cfg=1.5,
             lyrics="[verse] warm wire", duration_sec=95.0),
        _job(5, parent_id=3, role="continue", seed=77, cont_from=30.0, duration_sec=120.0),
        _job(7, parent_id=5, role="variant", voice_src=5, duration_sec=120.0),
        _job(9, role="overdub", overdub_of=7, seed=88, style="warm strings",
             duration_sec=120.0),
        _job(11, parent_id=7, role="variant", duration_sec=120.0),
    ]
    ops = [
        _op(10, 3, "continue", {"from_sec": 30.0, "parent_id": 3, "seed": 77}, "job:5"),
        _op(11, 5, "fx", {"source": "mix", "label": "warm",
                          "chain": [{"type": "comp", "threshold_db": -18, "ratio": 3},
                                    {"type": "reverb", "wet": 0.2, "decay_s": 1.8}],
                          "from": 10.0, "to": 40.0}, "dsp-fx-1.flac"),
        _op(12, 5, "variant_track", {"file": "dsp-fx-1.flac", "title": "warm"}, "job:7"),
        _op(13, 7, "overdub", {"style": "warm strings", "seed": 88, "gain": 0.5}, "job:9"),
        _op(14, 7, "variant_track", {"file": "overdub-9.flac", "title": "with strings"},
            "job:11"),
        _op(15, 3, "set_head", {"head_id": 11}, ""),
    ]
    return _build(jobs, ops, 3)


class TestRecipeGolden(unittest.TestCase):
    """build_recipe по сквозному журналу = фикстура recipe_e2e.json."""

    def test_spec_properties(self):
        r = _e2e_recipe()
        self.assertEqual([o["result"] for o in r["ops"]],
                         ["job:n2", "dsp-fx-1.flac", "job:n3", "job:n4", "job:n5", ""])
        self.assertEqual([o["node"] for o in r["ops"]], ["n1", "n2", "n2", "n3", "n3", "n1"])
        self.assertEqual(r["track"]["head"], "n5")
        self.assertEqual(r["ops"][5]["params"], {"head_id": "n5"})
        # имя файла overdub-<id> несёт id узла, а не id трека этой установки
        f = r["ops"][4]["params"]["file"]
        self.assertNotIn("9", f)
        self.assertIn("n4", f)
        self.assertTrue(f.startswith("overdub-") and f.endswith(".flac"), f)
        self.assertEqual(r["durations"], {"n1": 95.0, "n2": 120.0, "n3": 120.0,
                                          "n4": 120.0, "n5": 120.0})
        self.assertEqual(r["warnings"], [])
        self.assertNotIn("job:3", json.dumps(r))

    def test_matches_fixture(self):
        r = _e2e_recipe()
        if os.environ.get(GOLDEN_ENV) == "1":
            GOLDEN.parent.mkdir(parents=True, exist_ok=True)
            GOLDEN.write_text(json.dumps(r, ensure_ascii=False, indent=2) + "\n",
                              encoding="utf-8")
        self.assertTrue(GOLDEN.exists(), f"нет фикстуры {GOLDEN}")
        want = json.loads(GOLDEN.read_text(encoding="utf-8"))
        self.assertEqual(
            r, want,
            "build_recipe выдаёт не то, что в фикстуре. Если изменение намеренное (формат "
            f"рецепта поменялся) — обновите: cd worker && {GOLDEN_ENV}=1 python -m unittest "
            "test_recipe.TestRecipeGolden, затем прогоните Go-тест ведущего на этой фикстуре.")


if __name__ == "__main__":
    unittest.main()
