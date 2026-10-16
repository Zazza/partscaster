"""Спецификация удаления Ollama и «из трека» на воркере (задача internal-drop-ollama,
условия 1 и 3 карточки).

Что должно работать:
- Маршрутов /copilot, /translate, /lyrics/adapt, /ollama_models нет (Ollama удалена):
  GET/POST на них → 404 или 405.
- Загрузки чужого файла на распознавание текста нет: POST /lyrics (так маршрут назывался
  до задачи) и /lyrics/recognize (так его называет карточка) → 404 или 405.
- GET /config не отдаёт ключей ollama_*; POST /config с ollama_url/ollama_model их не
  сохраняет (принять с игнором или отвергнуть 422 — оба ответа допустимы).
- POST /jobs/{id}/lyrics (распознать, что поёт готовый трек) остаётся зарегистрированным.
"""
import unittest

from test_pure import _HAS_WORKER_DEPS, _WorkerApiCase

_SKIP = "нужны fastapi/httpx/numpy (окружение воркера)"

_GONE = ["/copilot", "/translate", "/lyrics/adapt", "/ollama_models", "/lyrics", "/lyrics/recognize"]


def _ollama_keys(d):
    return [k for k in d if str(k).lower().startswith("ollama")]


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestOllamaRoutesGone(_WorkerApiCase):
    """Условия 1 и 3: эндпоинтов Ollama и загрузки трека на распознавание нет."""

    def test_get_routes_absent(self):
        for path in _GONE:
            with self.subTest(path=path):
                r = self.client.get(path)
                self.assertIn(r.status_code, (404, 405), f"GET {path}: {r.status_code} {r.text[:200]}")

    def test_post_routes_absent(self):
        body = {"text": "мрачный панк", "lyrics": "[verse] la la", "to": "Russian", "prompt": "x"}
        for path in _GONE:
            with self.subTest(path=path):
                r = self.client.post(path, json=body, headers={"X-Filename": "song.flac"})
                self.assertIn(r.status_code, (404, 405), f"POST {path}: {r.status_code} {r.text[:200]}")

    def test_routes_not_registered(self):
        paths = {getattr(rt, "path", "") for rt in self.w.app.routes}
        self.assertEqual(sorted(paths & set(_GONE)), [])

    def test_job_lyrics_route_still_registered(self):
        found = [rt for rt in self.w.app.routes
                 if getattr(rt, "path", "") == "/jobs/{job_id}/lyrics"
                 and "POST" in (getattr(rt, "methods", None) or set())]
        self.assertTrue(found, "POST /jobs/{job_id}/lyrics пропал — распознавание готового трека должно остаться")


@unittest.skipUnless(_HAS_WORKER_DEPS, _SKIP)
class TestConfigHasNoOllama(_WorkerApiCase):
    """Условие 1: в /config нет полей ollama_* ни на чтение, ни на запись."""

    def test_get_config_has_no_ollama_keys(self):
        r = self.client.get("/config")
        self.assertEqual(r.status_code, 200, r.text[:200])
        cfg = r.json()
        self.assertTrue(cfg, "GET /config пуст — проверка ничего не доказывает")
        self.assertEqual(_ollama_keys(cfg), [])

    def test_post_config_does_not_store_ollama(self):
        r = self.client.post("/config", json={"ollama_url": "http://llm:11434/api/chat",
                                              "ollama_model": "qwen3:8b"})
        self.assertIn(r.status_code, (200, 422), f"POST /config: {r.status_code} {r.text[:200]}")
        if r.status_code == 200:
            self.assertEqual(_ollama_keys(r.json() or {}), [], "ответ POST /config с ollama_*")
        cfg = self.client.get("/config").json()
        self.assertEqual(_ollama_keys(cfg), [])
        self.assertNotIn("qwen3:8b", repr(cfg))


if __name__ == "__main__":
    unittest.main()
