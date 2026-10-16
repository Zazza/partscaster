# Участие в разработке Partscaster
[English](CONTRIBUTING.md) · **Русский**

Как поднять окружение, прогнать проверки, прислать изменения и выпустить версию.
Ошибки и идеи — в [GitHub Issues](https://github.com/Zazza/partscaster/issues).

## Окружение

| Инструмент | Версия | Зачем |
|---|---|---|
| Go | 1.24 (см. `go.mod`) | приложение, MCP-сервер, тесты Go |
| Node.js | 20 | фронтенд (Vue 3), vitest, eslint |
| [Wails CLI](https://wails.io) | v2.12.0, как в `go.mod`: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` | `make build`, релизные пакеты |
| пакеты разработки GTK 3 / WebKitGTK 4.1 (Linux) | `libgtk-3-dev libwebkit2gtk-4.1-dev` | сборка Go-части на Linux |
| Python | 3.12 | воркер и его тесты |
| [uv](https://docs.astral.sh/uv/) | свежий | установка воркера (`worker/install.sh`, `partscaster-worker.exe`) |
| ruff | свежий | без него `make lint` падает |
| golangci-lint | v2 (в CI — v2.14.0) | локально по желанию: `make lint` его пропустит, CI прогонит |
| ffmpeg | любая | тесты DSP в `internal/dsp` и воркера; без него такие тесты пропускаются |

В Ubuntu 24.04 есть только WebKitGTK 4.1 — приложение собирается с `GOFLAGS=-tags=webkit2_41`.

Полная инструкция по развёртыванию (GPU-машина, воркер, модели): [docs/ru/deployment.md](docs/ru/deployment.md).

## Как прогнать проверки

Go-код вшивает собранный фронтенд, поэтому перед тестами Go его нужно один раз собрать:

```bash
cd frontend && npm ci && npm run build && cd ..
pip install ruff fastapi httpx numpy scipy librosa soundfile   # зависимости тестов воркера, как в CI
```

Дальше из корня репозитория:

```bash
make lint     # проверка gofmt, go vet, golangci-lint, eslint (фронтенд), ruff (воркер)
make test     # go test ./..., vitest, unittest воркера
```

По частям: `make test-go`, `make test-front`, `make test-worker`, `make lint-go`, `make lint-front`,
`make lint-worker`. `make help` покажет все цели.

## Как прислать изменения

- Разработка идёт в `develop`, в `main` попадают выпущенные версии. Pull request — в `develop`.
- Перед pull request `make lint && make test` должны быть зелёными. CI (`.github/workflows/ci.yml`) гоняет те же
  проверки на каждом pull request и на push в `main`.
- Коммиты — по [Conventional Commits](https://www.conventionalcommits.org/ru/): `feat:`, `fix:`, `refactor:`,
  `docs:`, `test:`, `chore:`, `build:`, можно с областью (`fix(worker): …`). На русском или английском.
- Один pull request — одна задача, без попутных правок.

## Правила проекта

- Поведение меняем осознанно: изменение, заметное пользователю, видно в сообщении коммита и, если оно заметное,
  в [CHANGELOG.md](CHANGELOG.md) в разделе `Unreleased`.
- Тесты пишем по спецификации «как должно работать», а не фиксацией текущего поведения.
- Тексты интерфейса — через словари i18n `frontend/src/i18n/ru.js` и `en.js`, каждый ключ — в оба.
  Комментарии в коде — на русском, идентификаторы — на английском.
- Документация двуязычная: `docs/ru/` и `docs/en/` с одинаковыми именами файлов, `README.md` и `README.ru.md`,
  `CONTRIBUTING.md` и этот файл. Правка — в обоих языках одним коммитом.
- Новые переменные окружения воркера — в `worker.env.example` и таблицу переменных в
  `docs/ru/deployment.md` и `docs/en/deployment.md`.
- После правок слотов, групп или пресетов в `frontend/src/` или `worker/fx_blocks.json` — `make mcp-data` и
  закоммитить перегенерированные файлы.
- Никаких личных данных (домашние пути, адреса в локальной сети, имена машин) и секретов в коде и доке.

Полный свод правил, включая ограничения архитектуры, — в [AGENTS.md](AGENTS.md) (на английском).

## Релизы

1. В [CHANGELOG.md](CHANGELOG.md) раздел `## [Unreleased]` переименовать в `## [X.Y.Z] - ГГГГ-ММ-ДД`.
2. Тем же коммитом добавить первую запись в `frontend/src/releaseNotes.js` — окно «Что нового»: 5–10 коротких
   строк простыми словами на русском и английском, `version: 'X.Y.Z'`. Тест `releaseNotes.test.js` падает, если
   в CHANGELOG есть версия новее этой записи.
3. Слить в `main`; тег `vX.Y.Z` ставит и пушит мейнтейнер.
4. Тег запускает `.github/workflows/release.yml`. Он собирает пакеты на машинах Linux, Windows и macOS и выкладывает
   в GitHub Release этого тега:
   - `partscaster-<версия>-<ОС>-<архитектура>.tar.gz` (Windows — `.zip`): приложение, `partscaster-mcp`, архив
     воркера и `INSTALL.txt`; в пакете Windows ещё `partscaster-worker.exe` (воркер без WSL); на macOS MCP-сервер
     и архив воркера лежат также внутри `partscaster.app/Contents/MacOS/`;
   - `worker-<версия>.tar.gz` — архив воркера из сборки под Linux, отдельным файлом.

   Второй движок yue2.cpp (эксперимент) собирается в архив воркера на Linux (в контейнере CUDA 12.8) и на Windows
   (с CUDA Toolkit, который ставится в той же сборке). Архив воркера для macOS — без него. Если сборка yue2.cpp
   не удалась, пакет всё равно выходит, без второго движка.

Собрать пакет у себя: `make release` (под эту ОС), `make dist-linux`, `make dist-windows` (собирается и на Linux),
`make dist-mac` (только на Mac). Результат — в `dist/`.
