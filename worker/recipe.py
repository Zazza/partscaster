"""Рецепт трека: сборка переносимого описания песни из строк jobs и журнала job_ops.

Чистые функции без БД и HTTP: эндпоинт воркера передаёт сюда строки, тесты — словари.
Узлы рецепта — локальные id «n1», «n2»…: корень и треки, созданные записями журнала (result "job:<id>");
id треков установки в рецепт не попадают.
"""
from __future__ import annotations

import re

FORMAT = "yue-track-recipe"
VERSION = 1

# операции журнала (их пишет Go: обёртка API воркера и составные операции студии)
OPS = frozenset({
    "submit", "continue", "fragment", "overdub", "variant_track", "fx", "minus", "update", "set_head",
    "rebuild_sections", "envelope", "splice", "dsp_apply", "pedals", "sound_preset",
})

# ключи params, значения которых — id треков: при экспорте заменяются на n-id, при импорте — на новые id
ID_KEYS = frozenset({"parent_id", "child_id", "job_id", "voice_src", "head_id"})

IMPORT_STYLE = "(импорт внешнего трека)"

# файлы, в имени которых id трека: микс вклейки/пересборки — при импорте у трека будет другой id
_ID_FILE_RE = re.compile(r"^(overdub-(?:inst-|premaster-)?)(\d+)(\.flac)$")


class RecipeError(Exception):
    """Трек нельзя экспортировать рецептом — причина для человека."""


def _job_ref(result: str) -> int | None:
    if isinstance(result, str) and result.startswith("job:"):
        try:
            return int(result[4:])
        except ValueError:
            return None
    return None


def _ids_in(value, found: set[int]) -> None:
    """id треков в params: значения ID_KEYS на любой глубине (0 — «нет трека», не ссылка)."""
    if isinstance(value, dict):
        for k, v in value.items():
            if k in ID_KEYS and isinstance(v, int) and not isinstance(v, bool) and v > 0:
                found.add(v)
            else:
                _ids_in(v, found)
    elif isinstance(value, list):
        for v in value:
            _ids_in(v, found)


def _strings(value) -> set[str]:
    """Все строки params на любой глубине (имена файлов, на которые опирается правка)."""
    if isinstance(value, str):
        return {value}
    if isinstance(value, dict):
        return set().union(*(_strings(v) for v in value.values())) if value else set()
    if isinstance(value, list):
        return set().union(*(_strings(v) for v in value)) if value else set()
    return set()


def _map_ids(value, nid: dict[int, str]):
    """Копия params: id треков рецепта → n-id; в именах файлов overdub-<id>.flac — overdub-{nK}.flac."""
    if isinstance(value, str):
        m = _ID_FILE_RE.match(value)
        if m and int(m.group(2)) in nid:
            return f"{m.group(1)}{{{nid[int(m.group(2))]}}}{m.group(3)}"
        return value
    if isinstance(value, dict):
        out = {}
        for k, v in value.items():
            if k in ID_KEYS and isinstance(v, int) and not isinstance(v, bool) and v in nid:
                out[k] = nid[v]
            else:
                out[k] = _map_ids(v, nid)
        return out
    if isinstance(value, list):
        return [_map_ids(v, nid) for v in value]
    return value


def _check_reproducible(job: dict, what: str) -> None:
    if (job.get("style") or "") == IMPORT_STYLE:
        raise RecipeError(f"{what} #{job['id']} — импортированное аудио: рецептом не переносится "
                          "(нужен zip-пакет с файлом)")
    if job.get("role") == "voice":
        raise RecipeError(f"{what} #{job['id']} — «голос альбома» по образцу: рецептом не переносится "
                          "(нужен zip-пакет с образцом)")


def build_recipe(jobs: dict[int, dict], ops: list[dict], root_id: int, worker_model: str,
                 stems_model: str, exported_at: str) -> dict:
    """jobs — строки треков по id (поля таблицы jobs + plan_abc/request_abc текстом), ops — записи журнала
    (id, job_id, op, params, result, refs) по возрастанию id. Корень — уже найденный верх песни."""
    root = jobs.get(root_id)
    if root is None:
        raise RecipeError(f"трек #{root_id} не найден")
    if root.get("draft"):
        raise RecipeError("черновик не экспортируется: сначала сделайте полный трек")
    _check_reproducible(root, "трек")

    nid: dict[int, str] = {root_id: "n1"}
    warnings: list[str] = []
    out_ops: list[dict] = []
    dropped: set[tuple[int, str]] = set()  # (трек, файл) — результаты пропущенных правок: зависящее от них тоже
    for rec in sorted(ops, key=lambda r: r["id"]):
        if rec["job_id"] not in nid:
            continue  # запись чужой песни (или ветки, созданной до журнала)
        uses = _strings(rec.get("params") or {})
        lost = sorted(f for (j, f) in dropped if j == rec["job_id"] and f in uses)
        if lost:
            warnings.append(f"правка «{rec['op']}» #{rec['id']} берёт файл пропущенной правки ({', '.join(lost)}) — "
                            "пропущена")
            if rec.get("result") and _job_ref(rec["result"]) is None:
                dropped.add((rec["job_id"], rec["result"]))
            continue
        refs = set(rec.get("refs") or [])
        _ids_in(rec.get("params") or {}, refs)
        created = _job_ref(rec.get("result", ""))
        outside = sorted(r for r in refs if r not in nid and r != created)
        if outside:
            ids = ", ".join(f"#{r}" for r in outside)
            warnings.append(f"правка «{rec['op']}» #{rec['id']} ссылается на треки вне песни ({ids}) — пропущена")
            if rec.get("result") and _job_ref(rec["result"]) is None:
                dropped.add((rec["job_id"], rec["result"]))
            continue
        if created is not None:
            child = jobs.get(created)
            if child is None:
                warnings.append(f"трек #{created} из журнала удалён — «{rec['op']}» и всё, что сделано на нём, "
                                "в рецепт не входят")
                continue
            _check_reproducible(child, "версия")
            nid[created] = f"n{len(nid) + 1}"
        out_ops.append({
            "op": rec["op"],
            "node": nid[rec["job_id"]],
            "params": _map_ids(rec.get("params") or {}, nid),
            "result": f"job:{nid[created]}" if created is not None else _map_ids(rec.get("result", ""), nid),
            "refs": [nid[r] for r in dict.fromkeys(rec.get("refs") or []) if r in nid],  # порядок как в записи
        })

    # потомки, сделанные до журнала: в рецепт не попадут — честно говорим
    def descends(j: dict) -> bool:
        seen = set()
        while j is not None and j["id"] not in seen:
            seen.add(j["id"])
            up = j.get("parent_id") or j.get("overdub_of")
            if up == root_id:
                return True
            j = jobs.get(up) if up else None
        return False
    # «голос альбома» в песне (образец голоса — аудио) рецептом не переносится — честный отказ
    for j in sorted(jobs.values(), key=lambda x: x["id"]):
        if (j["id"] in nid or descends(j)) and j.get("role") == "voice":
            _check_reproducible(j, "версия")
    old = sorted(j["id"] for j in jobs.values() if j["id"] not in nid and descends(j) and not j.get("draft"))
    if old:
        warnings.append(f"версии {', '.join(f'#{i}' for i in old)} сделаны до журнала правок — в рецепт не входят")

    head = root.get("head_id") or 0
    if head and head not in nid:
        warnings.append(f"основная версия #{head} не входит в рецепт — основной будет сам трек")
        head = 0

    return {
        "format": FORMAT,
        "version": VERSION,
        "exported_at": exported_at,
        "worker_model": worker_model,
        "stems_model": stems_model,
        "track": {"title": root.get("title") or "", "folder": root.get("folder") or "",
                  "head": nid[head] if head else "n1"},
        "root": {
            "id": "n1", "title": root.get("title") or "", "style": root.get("style") or "",
            "lyrics": root.get("lyrics") or "", "seed": root.get("seed") or 0, "cot": root.get("cot") or "full",
            "arc": root.get("arc") or "", "max_tokens": root.get("max_tokens") or 0,
            "temperature": root.get("temperature") or 0, "cfg": root.get("cfg") or 0,
            "abc": root.get("request_abc"), "plan_abc": root.get("plan_abc"),
        },
        "ops": out_ops,
        # длительности версий: импорт сравнивает со своими (окна правок в секундах могли сместиться)
        "durations": {n: jobs[i].get("duration_sec") or 0 for i, n in nid.items()},
        "warnings": warnings,
    }


def check_recipe(recipe) -> str:
    """Проверка файла рецепта перед импортом: "" — годен, иначе причина."""
    if not isinstance(recipe, dict) or recipe.get("format") != FORMAT:
        return "это не рецепт трека Partscaster"
    if recipe.get("version") != VERSION:
        return f"версия рецепта {recipe.get('version')} не поддерживается (нужна {VERSION}) — обновите приложение"
    root = recipe.get("root")
    if not isinstance(root, dict) or not root.get("style") and not root.get("plan_abc"):
        return "в рецепте нет корня трека (стиль/план)"
    for i, op in enumerate(recipe.get("ops") or []):
        if not isinstance(op, dict) or op.get("op") not in OPS:
            return f"шаг {i + 1}: неизвестная операция {op.get('op') if isinstance(op, dict) else op!r}"
    return ""
