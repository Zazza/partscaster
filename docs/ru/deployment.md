# Развёртывание
**Русский** · [English](../en/deployment.md)

Как поставить Partscaster: воркер на машину с видеокартой NVIDIA, приложение на ПК (часто это одна
и та же машина), по желанию — AI-агента через MCP. Как устроены части — [architecture.md](architecture.md).

## С чего начать

| Ваш случай | Путь |
|---|---|
| Linux с NVIDIA, готовая сборка | [Релизный пакет](#релизный-пакет) → `install.sh` |
| Пусть поставит AI-агент (Claude Code и др.) | [Через AI-агента](#через-ai-агента) — MCP-инструмент `install_worker` |
| Windows с NVIDIA, без WSL | [partscaster-worker.exe](#воркер-на-windows-без-wsl--partscaster-workerexe) из пакета Windows |
| Разработка, свои правки | [Из исходников](#из-исходников) — `make worker`, `make build` |
| Windows, но хочется Linux-воркер | [WSL2](#воркер-в-wsl2) |

После установки воркера: запустить приложение, в «Настройках» ввести адрес воркера (можно просто IP или
имя машины — порт 8091 подставится), нажать «Проверить». На той же машине — `127.0.0.1`.

## Требования

- **Видеокарта NVIDIA с драйвером** (`nvidia-smi` должен работать). Другие видеокарты не поддерживаются:
  установщики проверяют `nvidia-smi` и без него останавливаются.
- **Видеопамять** (пик генерации): основной движок ~11 ГБ (замер ~11,4); второй движок yue2.cpp — Q8 ~8 ГБ,
  экономный режим ~4,5 ГБ (песня до ~3,5 мин). Подробнее — [yue2.cpp](#второй-движок-генерации--yue2cpp-эксперимент).
- **Linux-воркер:** Linux с systemd (проверялось на Ubuntu), [`uv`](https://docs.astral.sh/uv/) — им
  `install.sh` создаёт окружение и ставит пакеты, сам он `uv` не ставит. Ищет его в `PATH`, затем в `~/.local/bin` и
  `~/.cargo/bin` (туда кладёт установщик uv; ssh без входа в оболочку их в `PATH` не добавляет). Python 3.12 uv скачает сам.
  Интернет — для пакетов и весов.
- **Диск:** база воркера ~17,5 ГБ (окружение ~7,4 + веса ~10) + наборы сэмплов ~1,2 ГБ + запас под треки.
  Необязательные компоненты — в [таблице ниже](#компоненты-и-место).
- **Модель YuE2 открыта:** веса качаются с Hugging Face без токена и входа.

## Релизный пакет

Релиз — GitHub Release тега `vX.Y.Z` (собирает `.github/workflows/release.yml`). В нём:

| Файл | Что внутри |
|---|---|
| `partscaster-<версия>-linux-<арх>.tar.gz` | приложение, `partscaster-mcp`, архив воркера (с yue2.cpp под Linux, если собран), `INSTALL.txt` |
| `partscaster-<версия>-windows-amd64.zip` | то же для Windows + `partscaster-worker.exe`; архив воркера внутри — с yue2.cpp под Windows |
| `partscaster-<версия>-darwin-<арх>.tar.gz` | приложение `partscaster.app` (`partscaster-mcp` и архив воркера — внутри `.app/Contents/MacOS/`); без yue2.cpp |
| `worker-<версия>.tar.gz` | архив воркера для Linux-машины с GPU (тот же, что в пакете Linux) |

Архивы воркера в пакетах разных ОС отличаются только программой yue2.cpp. Для Linux-машины с GPU берите
`worker-<версия>.tar.gz` или архив из пакета Linux: в архиве из пакета Windows программы под Linux нет,
и при `--engine cpp` `install.sh` начнёт собирать её из исходников.

На GPU-машине:

```bash
tar xzf worker-<версия>.tar.gz
./worker-<версия>/install.sh
```

Затем — приложение («Настройки» → адрес воркера → «Проверить») и, по желанию, AI-агенты:
«Настройки» → «Для AI-агентов» (готовые конфиги и кнопка «Подключить к Claude Code»).

**Версии.** Воркер отвечает на `GET /version` (сборка, версия API, отпечаток кода, модель, что установлено).
Приложение и MCP сверяют версию API: воркер старее — плашка «обновите воркер», новее — «обновите приложение».
Обновлять — **сначала воркер, потом приложение**.

## Через AI-агента

MCP-инструмент `install_worker` ставит воркер и необязательные компоненты: пустой `host` — на этом ПК,
`host=user@gpu-host` — на другой машине по ssh. Архив берётся рядом с `partscaster-mcp` (релизный пакет)
или собирается `make worker-tar` (MCP, собранный из исходников).

- По умолчанию — сухой прогон: **ничего не выполняется**, показываются компоненты, место и шаги.
- Установка — `dry_run=false`, `confirm=true` и явный выбор `whisper`, `seedvc`, `roformer` (true/false);
  при `roformer=true` — ещё `stems_model=htdemucs|roformer` (чем делать дорожки по умолчанию).
- Пути: `hf_home` (кеш весов, по умолчанию `~/yue/hf-cache`), `sep_dir` (RoFormer), `seedvc_dir`;
  движок — `engine=torch|cpp`, `cpp_model=auto|bf16|q8|q6|q5`.
- До установки проверяются видеокарта и свободное место в домашнем каталоге; не хватает — установка
  не начинается.
- `doctor` — что стоит и работает сейчас (воркер, компоненты, GPU, место).

Внутри это тот же `install.sh` из архива плюс шаги для компонентов (см. ниже) и запуск службы.

## Компоненты и место

| Компонент | Нужен | Место | Без него |
|---|---|---|---|
| Воркер: генерация YuE2, партитура SheetSage2, дорожки demucs | обязательно | ~17,5 ГБ (окружение ~7,4 + веса ~10, качаются при первой генерации) | — |
| Наборы сэмплов (барабаны, басы, оркестр) | ставятся вместе с воркером | ~1,2 ГБ | докачиваются при первом выборе |
| RoFormer: более чистое разделение дорожек + барабаны по частям | по желанию | ~7,3 ГБ (окружение 6,1 + веса 1,1) | дорожки делает demucs, частей барабанов нет |
| whisper: распознавание текстов треков | по желанию | ~3,2 ГБ | кнопки распознавания неактивны с подписью «не установлен» |
| Seed-VC: «голос альбома» (эксперимент) | по желанию | ~10–11 ГБ после установки, ~14 ГБ во время | блок «Голос альбома» с подписью «не установлен» |
| NAM: усилители в звуковом движке | по желанию | папка пакетов без torch | недоступен только блок `amp` |
| yue2.cpp: второй движок | по желанию | модель 2,6–7,2 ГБ + VAE 0,5 ГБ | работает основной движок |

Веса Hugging Face (`HF_HOME`), RoFormer и Seed-VC можно держать на другом диске — тогда на домашнем
нужно меньше места.

## Воркер на Linux: install.sh

Воркер поставляется архивом с версией. Его `install.sh` — и установка, и обновление:

1. проверяет видеокарту (`nvidia-smi`) до установки нового кода (перенос со старого имени, если он нужен, идёт
   ещё раньше — см. ниже);
2. кладёт код в `~/partscaster/releases/<время>-<версия>/`;
3. создаёт окружение `~/yue/.venv` (`uv venv --python 3.12`), если его нет, и ставит пакеты
   `requirements.txt` с индекса PyTorch cu128 (в том числе пакет модели `yue2-infer` — архивом закреплённого
   коммита YuE с GitHub) — только при новом окружении или изменённом `requirements.txt`
   (отпечаток — `~/partscaster/.requirements.sha256`, без комментариев и пустых строк: правка текста пакеты не
   переустанавливает);
4. создаёт `~/partscaster/worker.env`, если его нет, с одной строкой `HF_HOME=` (по умолчанию
   `~/yue/hf-cache`, `--hf-home DIR` — другой каталог; тильда раскрывается — systemd её не понимает);
5. атомарно переключает ссылку `~/partscaster/current` на новую версию и ставит юнит
   `~/.config/systemd/user/partscaster-worker.service`;
6. ставит наборы сэмплов (`kits_install.py`; не вышло — докачает следующий запуск или приложение);
7. с `--engine` — настраивает движок генерации (см. yue2.cpp);
8. оставляет 3 прежние версии, остальные удаляет;
9. перезапускает службу и ждёт ответа `/health` до ~30 с (`--no-start` — без запуска).

Модель грузится не при старте, а при первой генерации (тогда же скачиваются веса).

| Параметр | Что делает |
|---|---|
| `--no-start` | установить без запуска службы |
| `--hf-home DIR` | каталог кеша весов для нового `worker.env` |
| `--engine torch\|cpp` | движок генерации; без параметра движок не меняется (новая установка — основной) |
| `--cpp-model auto\|bf16\|q8\|q6\|q5` | модель yue2.cpp (только с `--engine cpp`), по умолчанию `auto` |
| `--cpp-low-vram` | экономный режим yue2.cpp (только с `--engine cpp`) |
| `--rollback` | `~/partscaster/current/install.sh --rollback` — вернуть предыдущую версию |

Пути меняются переменными установщика: `PARTSCASTER_HOME` (по умолчанию `~/partscaster`) и
`PARTSCASTER_VENV` (`~/yue/.venv`). Данные (`~/partscaster/data`) и `worker.env` общие для всех версий
и установщиком не трогаются. Окружение `~/yue/.venv` и кеш `~/yue/hf-cache` называются так исторически.

**Автозапуск после перезагрузки.** Служба пользовательская (`systemctl --user`) и стартует вместе с сеансом
пользователя. Чтобы воркер поднимался после перезагрузки без входа в систему (обычно нужно на отдельной
GPU-машине), включите один раз:

```bash
sudo loginctl enable-linger $USER
```

Управление: `systemctl --user status|restart|stop partscaster-worker`, журнал —
`journalctl --user -u partscaster-worker -n 50`.

**Без systemd:**

```bash
cd ~/partscaster/current && ~/yue/.venv/bin/python -m uvicorn yue_worker:app --host 0.0.0.0 --port 8091
```

Переменные из `worker.env` и `PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True` в этом случае задайте сами.

**Перенос со старого имени.** Если в домашнем каталоге есть `~/yue-studio`, а `~/partscaster` ещё нет
(и `PARTSCASTER_HOME` не задан), `install.sh` сам переносит содержимое в `~/partscaster`, отключает и
удаляет старый юнит `yue-worker`, переименовывает ключи `YUE_*` в `worker.env` в `PARTSCASTER_*` и
оставляет ссылку `~/yue-studio` → `partscaster`. Окружение `~/yue/.venv` и кеш `~/yue/hf-cache` остаются
на месте. Приложение так же один раз копирует свои настройки из старой папки.

**ffmpeg** на GPU-машине нужен только для спектрограммы в студии (`sudo apt install ffmpeg`); без него
режим «спектр» отвечает 503, волна громкости работает.

### Обновление

Порядок: **сначала воркер, потом приложение**. Новое приложение со старым воркером работает, но без
новых возможностей.

- Из релиза — `install.sh` нового архива (как при установке).
- Из исходников — `make worker` (или `./deploy.sh worker`) с `PARTSCASTER_DEPLOY_HOST=user@gpu-host`:
  архив копируется в `~/partscaster/incoming` и ставится его `install.sh`.
- Перезапуск воркера прерывает идущие операции приложения и MCP (пресет звука, пересборка — «прервано»)
  и снимает паузу очереди. Генерация, шедшая в момент перезапуска, отмечается ошибкой; ждущие генерации
  остаются в очереди.

### Переменные воркера

Задаются в `~/partscaster/worker.env` (пример — `worker.env.example` в корне репозитория), после правки —
`systemctl --user restart partscaster-worker`.

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `PARTSCASTER_DATA_DIR` | `~/partscaster/data` (юнит задаёт `<PARTSCASTER_HOME>/data`) | база SQLite, треки, настройки воркера, наборы, модели yue2.cpp |
| `PARTSCASTER_HOME` | `~/partscaster` (задаёт юнит) | каталог установки; воркер ищет там собранную `install.sh` программу yue2.cpp (`yue2cpp-src`) |
| `HF_HOME` | `~/yue/hf-cache` (пишет `install.sh` в новый `worker.env`); без строки — `~/.cache/huggingface` | кеш весов Hugging Face (YuE2, SheetSage2, demucs) |
| `PYTORCH_CUDA_ALLOC_CONF` | `expandable_segments:True` (задаёт юнит) | режим выделения видеопамяти PyTorch |
| `YUE2_MAX_TOKENS` | `16000` | бюджет семантических токенов на песню ≈ максимальная длина (16000 — до ~10 мин; пик видеопамяти растёт с длиной) |
| `YUE2_CFG_SCALE` | `1.5` | следование стилю (classifier-free guidance): выше — точнее, но суше |
| `YUE2_BACKEND` | `torch` | режим основного движка: `torch`, `torch-eager` (без CUDA-графов; ставит Windows-воркер), `vllm` |
| `PARTSCASTER_OLLAMA_URL` | `http://127.0.0.1:11434/api/chat` | Ollama для копайтера стихов, перевода стиля, перевода текста под слоги |
| `PARTSCASTER_OLLAMA_MODEL` | `qwen2.5-chat-ru:latest` | модель Ollama |
| `PARTSCASTER_WHISPER_PY` | `~/whisper-venv/bin/python` | интерпретатор окружения whisper |
| `PARTSCASTER_SEP_PY` | `~/sep-venv/bin/python` | интерпретатор окружения RoFormer |
| `PARTSCASTER_SEP_MODELS` | `~/sep-models` | веса RoFormer и DrumSep (~1,1 ГБ, качаются при первом разделении) |
| `PARTSCASTER_STEMS_MODEL` | — | `demucs` — всегда разделять demucs, даже если RoFormer выбран и установлен |
| `PARTSCASTER_SEEDVC_DIR` | `~/partscaster/seedvc` | каталог Seed-VC |
| `PARTSCASTER_NAM_DEPS` | — | папка с пакетами NAM для блока `amp` |
| `PARTSCASTER_FX_ENGINE` | — | `0` — звуковой движок выключен (`/jobs/{id}/fx` → 503), что бы ни было в настройках |
| `PARTSCASTER_CPP_BIN` | — | своя программа `yue-server` (иначе — из архива, затем собранная `install.sh`) |
| `PARTSCASTER_CPP_MODELS` | `<PARTSCASTER_DATA_DIR>/yue2cpp-models` | каталог моделей yue2.cpp (GGUF) |

Переменные других программ:

| Переменная | Где | Смысл |
|---|---|---|
| `PARTSCASTER_URL` | приложение, `partscaster-mcp` | адрес воркера (главнее сохранённого в «Настройках»; по умолчанию `http://localhost:8091`). Приложение, запущенное с ярлыка, переменных шелла не видит — задавайте адрес в «Настройках» |
| `PARTSCASTER_MCP_DIR` | `partscaster-mcp` | куда скачивать файлы (по умолчанию `~/partscaster-mcp-downloads`) |
| `PARTSCASTER_HOME`, `PARTSCASTER_VENV` | `install.sh` | каталог установки и окружение воркера |
| `PARTSCASTER_DEPLOY_HOST` | `deploy.sh`, `make worker` | `user@gpu-host` для деплоя |
| `PARTSCASTER_TORCH_INDEX` | `seedvc_install.sh` | индекс PyTorch для Seed-VC (по умолчанию CUDA 12.8) |

Адрес и модель Ollama можно поменять на лету на странице «Настройки» приложения; это действует до
перезапуска воркера, потом снова берутся значения из `worker.env`.

## Необязательные компоненты

Ставятся отдельно — агентом (`install_worker`) или руками командами ниже. Без них воркер работает.

### Разделение на дорожки — BS-Roformer-SW и DrumSep

По умолчанию дорожки делает demucs (быстро, ~20 с на 4-минутный трек). BS-Roformer-SW
(python-audio-separator) чище (медиана SDR на MUSDB18: голос 11,7 против 9,1 дБ, «прочее» 7,6 против 4,8),
но ~4,5 раза дольше; DrumSep дополнительно раскладывает барабаны на бочку, малый, томы, хай-хэт, райд,
крэш. Работает в **отдельном окружении** (у audio-separator свои torch и onnxruntime), воркер запускает
его подпроцессом:

```bash
uv venv ~/sep-venv --python 3.12
uv pip install --python ~/sep-venv/bin/python "audio-separator[gpu]" audioread
```

- В другом месте — пропишите `PARTSCASTER_SEP_PY` и `PARTSCASTER_SEP_MODELS` в `worker.env`. Агент:
  `install_worker roformer=true stems_model=roformer sep_dir=/data/sep` — окружение встанет в
  `<sep_dir>/venv`, веса — в `<sep_dir>/models`, пути допишутся в `worker.env`.
- Включение: «Настройки» → «Разделение на дорожки», MCP `config_set stems_model=roformer`, разово —
  `make_stems model=roformer`. Нет окружения или оно упало — разделяет demucs без ошибки.
- **Лицензия весов:** RoFormer-SW и DrumSep — CC BY-NC-SA (некоммерческие), качаются при первом разделении.
  Коммерчески чистый режим — `PARTSCASTER_STEMS_MODEL=demucs` (веса demucs — MIT).

### whisper — тексты треков

```bash
uv venv ~/whisper-venv && uv pip install --python ~/whisper-venv/bin/python faster-whisper
```

Путь к интерпретатору — `PARTSCASTER_WHISPER_PY`.

### «Голос альбома» — Seed-VC (эксперимент)

Заменяет тембр голоса трека на тембр образца (`POST /jobs/{id}/voice`, MCP `voice_convert`); музыка и
мелодия не меняются. Голос узнаётся, но дрожит — это свойство Seed-VC; внутри фраз слышны куски музыки,
попавшие в дорожку голоса. Ставится в свой каталог со своим окружением, воркер вызывает его отдельной
программой:

```bash
~/partscaster/current/seedvc_install.sh                 # в ~/partscaster/seedvc
~/partscaster/current/seedvc_install.sh /data/seedvc    # или в свой каталог
```

- Нужно: `git`, `uv`, ~14 ГБ на время установки, ~10–11 ГБ после (окружение ~7,5 + веса ~2,4). Скрипт
  проверяет место заранее. Всё — код закреплённой версии, окружение, веса, кеши — внутри одного каталога.
- Каталог не по умолчанию — `PARTSCASTER_SEEDVC_DIR=<каталог>` в `worker.env` и перезапуск воркера.
  Проверка: MCP `doctor` или `GET /config` (`seedvc_available`).
- Другая CUDA — `PARTSCASTER_TORCH_INDEX=<индекс>` перед запуском скрипта.
- Ресурсы (RTX 4070 Ti SUPER 16 ГБ): +3,8 ГБ видеопамяти на время замены; 50 шагов — ~0,6 от длительности голоса.
- Лицензия Seed-VC — GPL-3.0: ставится пользователем явно и работает отдельной программой.

### Усилители NAM

Звуковой движок (`POST /jobs/{id}/fx`, MCP `fx_apply`) работает на numpy/scipy и ничего не требует. Только
блоку `amp` нужен neural-amp-modeler **0.12.2** (MIT; читает захваты `.nam` v0.5). Он ставится отдельной
папкой без своего torch (torch берётся у воркера):

```bash
uv pip install --python ~/yue/.venv/bin/python --target /data/nam-deps --no-deps \
  neural-amp-modeler==0.12.2 wavio pytorch_lightning lightning_utilities torchmetrics auraloss \
  matplotlib contourpy cycler fonttools kiwisolver pillow pyparsing python-dateutil
```

Затем `PARTSCASTER_NAM_DEPS=/data/nam-deps` в `worker.env` и перезапуск воркера.

- Захваты (`.nam`) и импульсные отклики (`.wav`) в поставку не входят — загружаются пользователем
  (`fx_asset_upload`) в `<данные>/fx/amps` и `fx/irs`. Задержка захвата меряется при первом применении.
- Откат: `PARTSCASTER_FX_ENGINE=0` или `config_set fx_engine=false` — эндпоинт отвечает 503, эффекты
  ffmpeg приложения работают.

### Наборы сэмплов

Наборы для блоков движка (барабаны, бас, гитары, оркестр, фортепиано) ставятся заранее: `install.sh`
запускает `kits_install.py` (~1,2 ГБ в первый раз, дальше без сети). Набора нет — приложение докачает его
при первом выборе с прогрессом (`GET /fx/kits/progress`); вручную — MCP `fx_kit_install` или:

```bash
cd ~/partscaster/current && ~/yue/.venv/bin/python kits_install.py vsco-violin osdk
```

Запускайте с переменными из `worker.env` (`install.sh` так и делает), иначе наборы лягут в каталог данных
по умолчанию. Файлы качаются прямыми ссылками с raw.githubusercontent.com (списки закреплены в
`worker/fx_kit_files.json`), у каждого 3 попытки, оборванное докачивается. Драм-машины (`tr808`, `tr909`,
`linn`, `cr78`, `simmons`) и `synthbass` воркер синтезирует сам, без сети. Состав наборов и лицензии —
в [effects.md](effects.md).

### Ollama

Отдельный сервис на `127.0.0.1:11434`. Не обязателен: без него генерация работает, недоступны копайтер,
автоперевод стиля и перевод текста под слоги. Модель — `PARTSCASTER_OLLAMA_MODEL` или страница «Настройки»
(список моделей берётся из Ollama).

## Второй движок генерации — yue2.cpp (эксперимент)

Меньше видеопамяти — для карт на 8–12 ГБ или видеокарты, которую делят с другими программами. Тот же сид
на другом движке даёт другую песню; ноты и продолжения переносятся.

- **Установка:** `install.sh --engine cpp [--cpp-model auto|bf16|q8|q6|q5] [--cpp-low-vram]`. `auto` — по
  видеопамяти первой карты: ≥ 11,5 ГБ → Q8; 7,5–11,5 ГБ → Q8 в экономном режиме; меньше → Q5 в экономном.
  `--engine torch` — вернуть основной.
- **Видеопамять на пике** (песня 2:47): BF16 ~10 ГБ, Q8 ~8, Q6 и Q5 ~7,7; экономный режим — Q8 ~4,5,
  Q6 и Q5 ~4,2. Основной движок — ~11,4.
- **Программа:** готовая сборка из архива воркера (`worker-<версия>/yue2cpp/`, CUDA 12.8; библиотеки CUDA
  берутся из окружения воркера, пакет разработки CUDA не нужен). Нет её или не запускается — `install.sh`
  собирает закреплённый коммит в `$PARTSCASTER_HOME/yue2cpp-src` (нужны git, cmake, nvcc, ~10 мин).
  Не вышло — воркер установлен и работает на основном движке.
- **Модели** — в `<данные>/yue2cpp-models` (`PARTSCASTER_CPP_MODELS`), скачиваются с проверкой суммы:
  при установке, из приложения («Настройки» → «Движок генерации» → «Скачать») или MCP `engine_model_install`.
- **Переключение** — «Настройки» приложения или MCP `config_set engine=cpp|torch`; действует со следующей
  генерации. Основной движок остаётся установленным.

## Воркер на Windows без WSL — partscaster-worker.exe

В пакете Windows рядом с приложением лежит `partscaster-worker.exe`. Двойной щелчок открывает окно консоли:
окно открыто — воркер работает, закрыли — остановлен (процесс Python в фоне не остаётся). Автозапуска нет.

- **Первый запуск:** проверка видеокарты (`nvidia-smi`) и места (нужно ~25 ГБ), выбор папки (по умолчанию
  `%LOCALAPPDATA%\partscaster-worker`, выбор запоминается; другая — `--home D:\partscaster-worker`). В папку
  ставятся uv (закреплённая версия, проверка sha256), Python 3.12, окружение `venv` и пакеты:
  `requirements.txt`, `torch`/`torchaudio` `2.10.0+cu128`, пакет `yue2` (архив закреплённого коммита YuE),
  `truststore`. Модель YuE2 (~10 ГБ) качается при первой генерации.
- **Дальше:** запуск за секунды. Пакеты переставляются, только если изменился `requirements.txt` или
  закреплённые версии (отпечаток — `packages.sha256`). Новая версия программы кладёт новый код в `app\`;
  данные (`data\`), модели (`hf-cache\`) и пакеты остаются.
- **Адрес:** `127.0.0.1:8091` — в приложении указать `127.0.0.1`. Для других машин сети — `--lan` (воркер
  без аутентификации, поэтому по умолчанию доступен только с этого ПК).
- **Сертификаты:** uv берёт их из хранилища Windows (`UV_SYSTEM_CERTS`), Python — через `truststore`;
  загрузки работают и там, где антивирус или прокси подменяют HTTPS.
- **Движок:** при первой установке программа спрашивает — основной (PyTorch в режиме
  `YUE2_BACKEND=torch-eager`: PyTorch под Windows собран без flash-attention) или yue2.cpp. Пункт yue2.cpp
  есть, только если в пакете есть `yue2cpp\yue-server.exe`. Модель по объёму карты — как у `install.sh`: от 11,5 ГБ —
  q8; 7,5–11,5 ГБ — q8 в экономном режиме; меньше — q5 в экономном. Веса скачиваются сразу, выбор пишется в `data\settings.json` и больше не
  спрашивается. Без вопросов: `--engine cpp --cpp-model q8 [--cpp-low-vram]` или `--engine torch`.
  Сменить позже — в «Настройках» приложения.
- Необязательные компоненты (whisper, RoFormer, Seed-VC) программа не ставит; наборы сэмплов заранее
  не ставит — они докачиваются при первом выборе.

## Воркер в WSL2

Для Windows, где нужен обычный Linux-воркер (например, dual-boot с общим диском). CUDA в WSL2 работает
через драйвер NVIDIA, установленный в Windows:

```powershell
wsl --install -d Ubuntu
```

Внутри WSL поставьте `uv` и воркер так же, как на Linux (`install.sh`). Если веса и данные лежат на общем
с Linux-бутом диске Windows (в WSL он виден как `/mnt/<буква диска>`), укажите их в `~/partscaster/worker.env`:

```bash
HF_HOME=/mnt/<диск>/hf-cache                       # веса не перекачивать
PARTSCASTER_DATA_DIR=/mnt/<диск>/partscaster-data  # общие треки (или оставьте свои)
PARTSCASTER_OLLAMA_URL=http://<IP Windows>:11434/api/chat   # Ollama под Windows
```

IP Windows из WSL — `ip route show default | awk '{print $3}'`; в WSL с mirrored-сетью подойдёт и
`127.0.0.1`. В WSL без systemd запускайте командой из раздела «Без systemd».

Приложение на Windows подключается к `http://localhost:8091` (или к IP WSL — `wsl hostname -I`).
Если данные общие, не запускайте воркеры обоих бутов одновременно — база SQLite одна.

## Из исходников

Требования: Go 1.24+, Node.js 20+, [Wails CLI v2](https://wails.io) той же версии, что в `go.mod`
(v2.12.0: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`). Для воркера — `uv`; для
`make lint` — `ruff` (golangci-lint по желанию); для `make yue2cpp` — docker.

| Команда | Что делает |
|---|---|
| `make worker` | архив воркера на GPU-машину (`PARTSCASTER_DEPLOY_HOST=user@gpu-host`) и его `install.sh` |
| `make worker-tar` | только архив `dist/worker-<версия>.tar.gz` |
| `make yue2cpp` | готовая сборка yue2.cpp для архива воркера (в контейнере CUDA 12.8) |
| `make build` | приложение `build/bin/partscaster` с версией из git |
| `make mcp` | MCP-сервер `build/bin/partscaster-mcp` (подключение — [mcp.md](mcp.md)) |
| `make mcp-data` | перегенерировать данные библиотеки MCP из фронтенда |
| `make release` | пакет под эту ОС в `dist/` |
| `make dist-linux` / `dist-windows` / `dist-mac` | пакет для ОС |
| `make install-desktop` / `uninstall-desktop` | ярлык в меню приложений Linux |
| `make test`, `make lint`, `make fmt` | тесты, линтеры, форматирование Go |

### Пакеты по ОС

| Цель | Где собирается | Итог |
|---|---|---|
| `make dist-linux` | только Linux | `partscaster-<версия>-linux-<арх>.tar.gz` |
| `make dist-windows` | Linux или Windows | `partscaster-<версия>-windows-amd64.zip` (с `partscaster-worker.exe`) |
| `make dist-mac` | только Mac | `partscaster-<версия>-darwin-<арх>.tar.gz` |

Перед пакетом собирается yue2.cpp, если его ещё нет в `dist/`: на Linux — при наличии docker, на Windows —
при наличии CUDA Toolkit (`nvcc`). Не вышло — архив воркера едет без второго движка, это не ошибка.
Релиз в CI запускается тегом `v*`, который ставит и пушит человек; пакет каждой ОС собирается на своём
раннере.

### Приложение по ОС

**Linux (основная платформа):**

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
make build              # build/bin/partscaster
make install-desktop    # ярлык в ~/.local/share/applications
```

Ярлык нужен и ради иконки: GNOME на Wayland берёт её только из `.desktop`-файла. Звук — через `pw-play`
(PipeWire): у webview в webkit2gtk нет аудиовыхода.

**Windows:** `wails build` (WebView2) → `build\bin\partscaster.exe`; пакет целиком — `make dist-windows`.
Звук — через WPF MediaPlayer (PowerShell). Воркер — `partscaster-worker.exe`, WSL2 или отдельная машина:
приложению нужен только HTTP-доступ к нему.

**macOS:** пакет собирается (`make dist-mac`, WKWebView), но не тестировался. Плеер приложения на macOS —
системный `afplay` (громкость — со следующего запуска), «открыть папку» — `open`; на Mac не проверено. Воркеру нужна NVIDIA, поэтому на Mac его не поставить — только адрес воркера на другой машине.
