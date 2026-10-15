# Развёртывание

Три части: GPU-машина с воркером, опционально Ollama, desktop-приложение на ПК.
Адрес воркера задаётся в «Настройках» приложения (кнопка «Проверить» — без сохранения; можно ввести
просто IP, порт 8091 подставится) или переменной `PARTSCASTER_URL`. Тот же адрес берёт MCP-сервер `partscaster-mcp`.

## Релизный пакет: скачал, поставил, работает

Релиз (GitHub Release тега `vX.Y.Z`, собирается `.github/workflows/release.yml`; локально — `make release`
под эту ОС или `make dist-linux` / `make dist-windows` / `make dist-mac`):

- `partscaster-<версия>-<ОС>-<арх>.tar.gz` (Windows — `.zip`): приложение, `partscaster-mcp` рядом с ним,
  архив воркера и `INSTALL.txt`;
- `worker-<версия>.tar.gz` — воркер для машины с GPU.

Воркер: `tar xzf worker-<версия>.tar.gz && ./worker-<версия>/install.sh`. Приложение: запустить,
в «Настройках» ввести адрес воркера, «Проверить». AI-агенты: «Настройки» → «Для AI-агентов».

**Версии.** Воркер отвечает `GET /version` (сборка, версия API, отпечаток кода, модель, что
установлено). Приложение и MCP сверяют версию API: воркер старее — плашка «обновите воркер»,
новее — «обновите приложение». Порядок обновления — по-прежнему сначала воркер.

## Установка: компоненты и где что стоит

Два сценария, установка одна и та же:

- **Всё на одном ПК** (самый частый): приложение и воркер на машине с видеокартой NVIDIA.
- **GPU отдельно:** приложение на рабочем ПК, воркер — на машине с видеокартой по сети
  (в приложении — адрес воркера в настройках; установка — по ssh).

Проще всего ставить через агента: MCP-инструмент `install_worker` (пустой `host` — этот ПК,
`host=user@gpu-host` — отдельная машина). Сухой прогон по умолчанию **ничего не выполняет** —
показывает компоненты, место и шаги; установка — `dry_run=false`, `confirm=true` и явный выбор
необязательных компонентов (`whisper`, `seedvc`). Перед установкой проверяются видеокарта и место
на диске: при нехватке установка не начинается. `doctor` — что стоит и работает сейчас.

| Компонент | Нужен | Место | Без него |
|---|---|---|---|
| Воркер: генерация YuE2, разбор нот, дорожки demucs | обязательно | ~17,5 ГБ (окружение ~7,4 + веса ~10, качаются при первом запуске) | — |
| RoFormer: более чистое разделение дорожек (+ барабаны по частям) | по желанию | ~7,3 ГБ (окружение 6,1 + веса 1,1) | дорожки делает demucs, частей барабанов нет |
| whisper: распознавание текстов треков | по желанию | ~3,2 ГБ | кнопки распознавания неактивны с подписью «не установлен» |
| Seed-VC: «голос альбома» (эксперимент) | по желанию | ~11 ГБ после установки, ~14 ГБ во время | блок «Голос альбома» в студии с подписью «не установлен» |

Веса Hugging Face (`HF_HOME`) и Seed-VC (`PARTSCASTER_SEEDVC_DIR`) можно держать на другом диске —
место на домашнем тогда нужно меньше. Пути с `~` установщик раскрывает в `$HOME`: systemd читает
`worker.env` без раскрытия тильды.

## Воркер (Python) на GPU-машине

Воркер едет **архивом с версией** (`make worker-tar` → `dist/worker-<версия>.tar.gz`: код без тестов,
данные `*.json`, фразы, юнит systemd, `install.sh`). Его `install.sh` — установка и обновление базы:

- проверяет видеокарту (`nvidia-smi`) до любых изменений;
- кладёт код в `~/partscaster/releases/<время>-<версия>/` и атомарно переключает ссылку
  `~/partscaster/current` (юнит запускает воркер из неё); хранит 3 прежние версии;
- создаёт venv `~/yue/.venv` (если нет) и ставит пакеты — только при новом venv или изменённом
  `requirements.txt` (отпечаток — `~/partscaster/.requirements.sha256`); venv, поставленный до архивов,
  не трогается;
- ставит наборы сэмплов, юнит, `worker.env` (если нет; `--hf-home DIR` — кеш весов);
- перезапускает воркер и проверяет `/health` (`--no-start` — без запуска).

Данные (`~/partscaster/data`) и `worker.env` общие для всех версий и не трогаются.
Откат: `~/partscaster/current/install.sh --rollback` — на предыдущую версию.
Переменные `PARTSCASTER_HOME` (по умолчанию `~/partscaster`) и `PARTSCASTER_VENV` (`~/yue/.venv`) меняют пути.

Вручную (без агента):

1. `tar xzf worker-<версия>.tar.gz && ./worker-<версия>/install.sh` на GPU-машине (Ubuntu + CUDA,
   нужен `uv`). Машинные настройки — `worker.env.example` → `~/partscaster/worker.env`.
2. Веса: `HF_HOME` — каталог кеша Hugging Face (YuE2, SheetSage2, demucs качаются при
   первом вызове; нужен интернет; YuE2 — gated-репо, нужен HF-токен).
3. Отдельный venv для Whisper (тексты треков корпусов):
   `uv venv ~/whisper-venv && uv pip install --python ~/whisper-venv/bin/python faster-whisper`
   (воркер вызывает его сам, путь — `PARTSCASTER_WHISPER_PY`).
4. Без systemd: `cd ~/partscaster/current && ~/yue/.venv/bin/python -m uvicorn yue_worker:app --host 0.0.0.0 --port 8091`.
5. Опционально: `ffmpeg` в PATH — только для спектрограммы в студии
   (`GET /jobs/{id}/spectrum.png`, готовой картинкой showspectrumpic).
   Нет бинарника — режим «спектр» честно откажет (503), волна громкости
   работает и без ffmpeg: `sudo apt install ffmpeg`.

### Автоматический деплой

Порядок при обновлении: **сначала воркер, потом приложение** — новое приложение со старым воркером
может работать без ошибок, но без новых возможностей (например, плавные края окна движка в студии).

`make worker` (или `./deploy.sh worker`) — архив воркера на GPU-машину и его `install.sh` (новая версия,
пакеты при изменении requirements, рестарт `partscaster-worker`). Хост задаётся `PARTSCASTER_DEPLOY_HOST=user@gpu-host`.
`worker.env` и данные на машине не перезаписываются. Первый деплой архивом на машину со старой раскладкой
(файлы прямо в `~/partscaster/`) переводит юнит на `~/partscaster/current`; старые `*.py` в `~/partscaster/`
больше не используются (можно удалить вручную).

### Второй движок генерации — yue2.cpp (необязательно, ЭКСПЕРИМЕНТ)

Меньше видеопамяти (Q8 ~8 ГБ, экономный режим ~4,5 ГБ против ~11 у основного) — для карт на 8–12 ГБ или
общей видеокарты. Тот же сид на другом движке даёт другую песню; ноты и продолжения переносятся.

- Установка: `install.sh --engine cpp [--cpp-model auto|bf16|q8|q6|q5] [--cpp-low-vram]`. `auto` — по
  видеопамяти первой карты: ≥ 11,5 ГБ → Q8; 7,5–11,5 → Q8 в экономном режиме; меньше → Q5 в экономном.
  Без `--engine` движок не меняется (новая установка — основной); `--engine torch` — вернуть основной.
  Пик видеопамяти на песне 2:47 (замер 2026-10-12): BF16 ~10 ГБ, Q8 ~8,0, Q6 и Q5 ~7,7; экономный режим —
  Q8 ~4,5, Q6 и Q5 ~4,2 (пик задают кэш модели и VAE, а не размер весов; у основного движка ~11,4).
- Программа: готовая сборка из архива воркера (`worker-<версия>/yue2cpp/`, CUDA 12.8; библиотеки CUDA берутся
  из venv воркера — пакеты `nvidia-*` от PyTorch, пакет разработки CUDA не нужен). Нет её или не запускается —
  install.sh собирает закреплённый коммит в `$PARTSCASTER_HOME/yue2cpp-src` (нужны git, cmake, nvcc). Не вышло —
  воркер всё равно установлен и работает на основном движке.
- Модели — в `<данные>/yue2cpp-models` (`PARTSCASTER_CPP_MODELS`), скачиваются с проверкой суммы: при установке или
  из приложения («Настройки» → «Движок генерации» → «Скачать»), MCP — `engine_model_install`.
- Переключение — «Настройки» приложения или MCP `config_set engine=cpp|torch`; действует со следующей
  генерации. Откат — выбрать основной движок: он остаётся установленным.
- Сборка для релиза: `make yue2cpp` (`tools/yue2cpp-build.sh`, нужен docker; на Linux `make release` и CI
  делают это сами). Сборка в CI раннере не проверялась (гипотеза: хватит места под образ CUDA ~10 ГБ).

### Переменные воркера

| Переменная | По умолчанию | Смысл |
|---|---|---|
| `PARTSCASTER_DATA_DIR` | `~/partscaster/data` | SQLite-очередь, джобы, профили корпусов |
| `PARTSCASTER_OLLAMA_URL` | `http://127.0.0.1:11434/api/chat` | Ollama для копайтера/перевода/стиля профиля |
| `PARTSCASTER_OLLAMA_MODEL` | `qwen2.5-chat-ru:latest` | модель Ollama |
| `PARTSCASTER_WHISPER_PY` | `~/whisper-venv/bin/python` | интерпретатор whisper-venv |
| `HF_HOME` | — | кеш весов Hugging Face |
| `YUE2_MAX_TOKENS` | `16000` | бюджет семантических токенов на песню ≈ макс. длина (дефолт протокола 9000 ≈ 4.5–6 мин; 16000 ≈ до ~10 мин; пик VRAM растёт с длиной) |
| `YUE2_CFG_SCALE` | `1.5` | следование стилю (classifier-free guidance): выше — точнее, но суше |
| `PARTSCASTER_SEEDVC_DIR` | `~/partscaster/seedvc` | каталог Seed-VC («голос альбома», необязательно) — см. ниже |
| `PARTSCASTER_SEP_PY` | `~/sep-venv/bin/python` | интерпретатор окружения разделения (RoFormer) — см. ниже |
| `PARTSCASTER_SEP_MODELS` | `~/sep-models` | каталог весов RoFormer/DrumSep (~1,1 ГБ, качаются при первом разделении) |
| `PARTSCASTER_STEMS_MODEL` | — | `demucs` — откат: разделять demucs, даже если RoFormer выбран в настройках и установлен |
| `PARTSCASTER_NAM_DEPS` | — | папка с пакетами NAM для блока `amp` звукового движка — см. ниже |
| `PARTSCASTER_FX_ENGINE` | — | `0` — откат: звуковой движок выключен (`/jobs/{id}/fx` → 503), что бы ни было в настройках |
| `PARTSCASTER_CPP_BIN` | — | своя программа `yue-server` (иначе — из архива, затем собранная install.sh) |
| `PARTSCASTER_CPP_MODELS` | `<данные>/yue2cpp-models` | каталог моделей yue2.cpp (GGUF) |

Настройки Ollama также меняются на лету из приложения (⚙ в шапке → `/config` воркера).

## «Голос альбома» — Seed-VC (необязательно, ЭКСПЕРИМЕНТ)

> Экспериментально: голос узнаётся (сходство с образцом по модели говорящего —
> почти как у самого образца), но **дрожит** — «как будто стесняется петь». Это
> свойство Seed-VC: не уходит ни от шагов/cfg, ни от дообучения на голос, ни от
> другого разделителя (BS-RoFormer на наших треках вышел хуже demucs). Внутри фраз
> музыка слегка страдает: дорожка голоса demucs несёт куски музыки.

Замена тембра голоса трека на тембр образца: голос из другого трека (например, «голос
альбома») накладывается на новую песню, музыка и мелодия не меняются
(`POST /jobs/{id}/voice`, MCP `voice_convert`). Делает это
[Seed-VC](https://github.com/Plachtaa/seed-vc) — отдельная установка в своём каталоге
и venv; воркер вызывает его отдельной программой (`seedvc_run.py`), как whisper.
Без установки воркер работает как раньше, запрос отвечает 503 с подсказкой.

Установка на GPU-машине (после `make worker` файлы уже в `~/partscaster/`):

```bash
~/partscaster/current/seedvc_install.sh                 # в ~/partscaster/seedvc
~/partscaster/current/seedvc_install.sh /data/yue-seedvc  # или в свой каталог
```

- Нужно: `git`, [`uv`](https://docs.astral.sh/uv/), CUDA-GPU, **~14 ГБ** на время установки,
  ~10 ГБ после: venv 7,5 (из них CUDA-библиотеки 4,3) + веса 2,4 (скрипт проверит место до установки и уберёт кэш пакетов в конце). Всё — код закреплённой версии, venv, веса,
  кэши uv и Hugging Face — внутри одного каталога: мимо него на системный диск
  ничего не пишется, каталог можно перенести на другой диск целиком.
- Каталог не по умолчанию — `PARTSCASTER_SEEDVC_DIR=<каталог>` в `~/partscaster/worker.env`
  и перезапуск воркера. Проверка: MCP `doctor` или `GET /config` (`seedvc_available`).
- PyTorch ставится с индекса CUDA 12.8; другая CUDA — `PARTSCASTER_TORCH_INDEX=<индекс>` перед
  запуском скрипта.
- Ресурсы (замер на RTX 4070 Ti SUPER 16 ГБ): +3,8 ГБ видеопамяти на время замены
  (рядом с загруженной YuE2 ~7.7 ГБ помещается); 50 шагов диффузии — ~0.6 от
  длительности голоса.
- Лицензия Seed-VC — GPL-3.0: ставится пользователем явно и работает отдельной
  программой, код Partscaster под неё не попадает.

## Гитарист по аккордам

Звуки «Гитарист вписывается по аккордам» (блок движка `gtrnotes`) отдельной установки не требуют: партия
строится по аккордам плана трека и ударам его дорожки гитары, без видеокарты. Наборы гитар (Emilyguitar,
Shinyguitar, FSBS, нейлон — CC0, ~0,7 ГБ) качаются как остальные наборы движка: по требованию или
`kits_install.py`. Прежнее распознавание нот (YourMT3+, basic-pitch) удалено: если оно стояло, каталог
`~/partscaster/notes` (~5 ГБ) можно удалить, а `PARTSCASTER_NOTES_DIR` — убрать из `worker.env`.

## Звуковой движок и усилители NAM

Звуковой движок (`POST /jobs/{id}/fx`, MCP `fx_apply`) работает на numpy/scipy — они уже есть у
воркера, ставить ничего не нужно. Только блоку `amp` (усилитель NAM) нужен пакет
neural-amp-modeler **0.12.2** (MIT; читает захваты `.nam` формата v0.5, 0.13 — уже нет). Импортом он
тянет matplotlib и pytorch_lightning, поэтому ставится отдельной папкой без своего torch (torch берётся
у воркера), на любой диск:

```bash
uv pip install --python ~/yue/.venv/bin/python --target /opt/yue/nam-deps --no-deps \
  neural-amp-modeler==0.12.2 wavio pytorch_lightning lightning_utilities torchmetrics auraloss \
  matplotlib contourpy cycler fonttools kiwisolver pillow pyparsing python-dateutil
echo 'PARTSCASTER_NAM_DEPS=/opt/yue/nam-deps' >> ~/partscaster/worker.env   # и перезапуск воркера
```

- Захваты усилителей (`.nam`) и импульсные отклики кабинетов/залов (`.wav`) в поставку не входят:
  пользователь загружает свои (`fx_asset_upload`, у каждого захвата своя лицензия). Лежат в
  `<PARTSCASTER_DATA_DIR>/fx/amps` и `fx/irs`. Задержка захвата меряется щелчком при первом применении и
  кэшируется рядом (`<имя>.nam.latency.json`).
- Ресурсы (замер на RTX 4070 Ti SUPER): первая загрузка ~13 с (инициализация CUDA), дальше — доли
  секунды; 4 минуты стерео через усилитель — ~2 с; остальные блоки — 0,4–2 с на CPU.
- Без `PARTSCASTER_NAM_DEPS` движок работает, недоступен только блок `amp` (ошибка с причиной).
- Откат: `PARTSCASTER_FX_ENGINE=0` в `worker.env` или `config_set fx_engine=false` — эндпоинт отвечает 503,
  эффекты ffmpeg приложения работают как прежде (движок их не заменяет).
- Наборы сэмплов барабанов (блок `sampler`) воркер качает сам по требованию (`POST /fx/kits/install`, MCP
  `fx_kit_install`) в `<PARTSCASTER_DATA_DIR>/fx/kits`, нужен доступ к GitHub. Первый набор — The Open Source Drum Kit
  (Real Music Media, github.com/crabacus/the-open-source-drumkit): автор объявил сэмплы общественным
  достоянием — можно в своих треках без указания автора (заявление автора на KVR:
  https://www.kvraudio.com/forum/viewtopic.php?t=277132; в репозитории файла лицензии нет). Версия набора
  закреплена за коммитом репозитория. В поставку не входит; приложение качает набор само, когда он нужен цепочке.
- Части набора osdk для хэта и тарелок (`hh-closed`, `hh-half`, `hh-open`, `ride`, `crash`) — из того же
  репозитория и той же версии; уже установленный osdk докачивает их при следующей установке (около 330 МБ, сэмплы
  тарелок 96 кГц по 8–12 с).
- Набор бас-гитары для блока `bass` — Growlybass (Karoryfer Lecolds, Squier Jazz Bass,
  github.com/sfzinstruments/karoryfer.growlybass, лицензия CC0 — без ограничений и без указания автора):
  `fx_kit_install growlybass`, 224 сэмпла (~170 МБ, часть `sustain`), версия закреплена за коммитом.
- Ещё бас-гитары и контрабас Karoryfer (github.com/sfzinstruments, CC0; один слой силы, первый повтор ноты, версии
  закреплены): `swagbass` (Ibanez BTB, плоские струны; часть `bass`, 19 сэмплов, ~13 МБ), `blackblue` (5-струнные:
  `darkblack` пальцами и `babyblue` медиатором, 42 + 41, ~55 МБ), `meatbass` (контрабас щипком, часть `pizz`, 14,
  ~7 МБ), `pastabass` (Squier Bass VI медиатором, часть `linguine`, 17, ~10 МБ).
- Оркестр VSCO-2 Community Edition (github.com/sgossner/VSCO-2-CE, CC0; версия закреплена): `vsco-violin` (solo, ens,
  pizz, spic), `vsco-viola`, `vsco-cello` (ens, pizz, spic), `vsco-contrabass` (sus, pizz, spic), `vsco-harp`,
  `vsco-flute`/`oboe`/`clarinet`/`bassoon`/`trumpet`/`horn`/`trombone`/`tuba` (sus, stac), `vsco-mallets` (glock,
  marimba, xylo) — по 5…100 МБ на инструмент, один слой силы. Файлы ставятся под точной высотой `m<MIDI>.wav`: нота из
  имени + поправка октавы части (VSCO почти везде считает C3 = 60; замер по каждому сэмплу).
- Наборы ставятся заранее: `make worker` и установщик (`install_worker`) после файлов запускают
  `~/yue/.venv/bin/python kits_install.py` — все наборы каталога (~1 ГБ в первый раз, дальше — без сети) с прогрессом
  по наборам; руками — тот же вызов в `~/partscaster` (можно с именами: `kits_install.py vsco-violin osdk`). Списки
  файлов новых наборов закреплены в `worker/fx_kit_files.json`: файлы качаются прямыми ссылками
  raw.githubusercontent.com, без API GitHub (его лимит без ключа — 60 запросов в час на адрес; оркестр — 31 часть).
  У каждого файла 3 попытки; оборвалось — повтор докачивает, не начиная заново. Набора нет — приложение докачает его при
  первом выборе и покажет прогресс («качаю набор …: часть, файлы, МБ»; `GET /fx/kits/progress`).
- Драм-машины `tr808`, `tr909`, `linn`, `cr78`, `simmons` и синт-басы `synthbass` воркер синтезирует сам
  (`worker/drumsynth.py`, без сети и чужих лицензий): `fx_kit_install tr808` пишет 12 частей по 8 слоёв силы удара
  (wav 48 кГц, PCM 16, ~10 МБ на машину), `synthbass` — три баса по полутонам E1…G3 (~15 МБ). Звучание — приближение
  к машинам, не их копия.
- Фортепиано для блока `synth` — Salamander Grand Piano V3 (Alexander Holm, github.com/sfzinstruments/SalamanderGrandPiano,
  лицензия CC-BY 3.0 — свободно, с указанием автора; указан в подсказке пресета): `fx_kit_install salamander`, части
  `piano` (слой силы v10) и `piano-soft` (v4), по 30 файлов flac (~47 МБ на часть), версия закреплена за коммитом.
- pedalboard (Spotify) не используется: его лицензия GPL-3, проект — MIT.

## Разделение на дорожки — BS-Roformer-SW

Дорожки по умолчанию делает demucs (быстро, ~20 с на 4-минутный трек). По желанию — BS-Roformer-SW
(python-audio-separator): заметно чище (замер на MUSDB18, медиана SDR: голос 11,7 против 9,1 дБ,
«прочее» 7,6 против 4,8), но ~4,5 раза дольше (~1,5 мин на трек); плюс DrumSep раскладывает барабаны
на бочку, малый, томы, хай-хэт, райд, крэш. Включается в настройках приложения («Разделение на
дорожки»), через MCP — `config_set stems_model=roformer`, разово для трека — `make_stems model=roformer`.
Работает в
**отдельном окружении** (у audio-separator свой torch/onnxruntime — в окружении YuE2 они бы
конфликтовали), воркер вызывает его подпроцессом `sep_run.py`:

```bash
uv venv ~/sep-venv --python 3.12
uv pip install --python ~/sep-venv/bin/python "audio-separator[gpu]" audioread
```

Окружение ~6,1 ГБ; при нехватке места на домашнем — поставьте на другой диск и укажите
`PARTSCASTER_SEP_PY` (и `PARTSCASTER_SEP_MODELS` для весов) в `worker.env`. Через агента — `install_worker roformer=true
stems_model=roformer|htdemucs` (агент спрашивает, чем делать дорожки по умолчанию; выбор пишется в
настройки воркера, после установки проверяется, что дорожки действительно делает RoFormer; без RoFormer и без выбора
настройка не меняется), например `install_worker roformer=true stems_model=roformer
sep_dir=/mnt/d/sep`: окружение встанет в `<sep_dir>/venv`, веса — в `<sep_dir>/models`, пути
допишутся в `worker.env` сами. Нет окружения или оно упало —
разделяет demucs, ошибки нет; `PARTSCASTER_STEMS_MODEL=demucs` выключает RoFormer принудительно, что бы ни
было выбрано в настройках.
Какая модель активна — в настройках приложения и в `/config` (`stems_model`).

**Лицензия весов:** RoFormer-SW и DrumSep — CC BY-NC-SA (некоммерческие); в репозиторий не
входят, качаются при первом разделении. Для коммерчески чистого режима — `PARTSCASTER_STEMS_MODEL=demucs`
(веса demucs — MIT).

## Ollama

Отдельный сервис на `127.0.0.1:11434`, модель `qwen2.5`. Не обязателен: без него
генерация работает, недоступны копайтер/автоперевод/профиль-стиль.

## Воркер на Windows без WSL — partscaster-worker.exe

В пакете Windows (`partscaster-<версия>-windows-amd64.zip`) рядом с приложением лежит `partscaster-worker.exe`.
Двойной щелчок открывает окно консоли; окно открыто — воркер работает, закрыли — остановлен (процесс Python
привязан к программе объектом задания Windows и не остаётся в фоне). Автозапуска нет.

- **Первый запуск:** проверка видеокарты (`nvidia-smi`), выбор папки (по умолчанию
  `%LOCALAPPDATA%\partscaster-worker`, выбор запоминается; другая — `partscaster-worker.exe --home D:\partscaster-worker`), затем в эту
  папку ставятся uv (закреплённая версия, проверка sha256), Python 3.12, окружение `venv` и пакеты:
  `requirements.txt` + `torch`/`torchaudio` `2.10.0+cu128` (явная метка: без неё uv на Windows берёт сборку
  без CUDA) + `yue2-infer` архивом закреплённого коммита YuE + `truststore`. Около 25 ГБ вместе с моделью;
  модель YuE2 (~10 ГБ) качается при первой генерации.
- **Дальше:** запуск за секунды. Пакеты переставляются, только если изменился `requirements.txt` или
  закреплённые версии (отпечаток — `packages.sha256` в папке). Новая версия программы кладёт новый код
  воркера в `app\`; данные (`data\`), модели (`hf-cache\`) и пакеты остаются.
- **Адрес:** `127.0.0.1:8091` — в приложении указать `127.0.0.1`. Для других машин сети —
  `partscaster-worker.exe --lan` (воркер без аутентификации, поэтому по умолчанию только этот ПК).
- **Сертификаты:** uv берёт их из хранилища Windows (`UV_SYSTEM_CERTS`), Python — через `truststore`;
  так загрузки работают и там, где антивирус или прокси подменяют HTTPS своим сертификатом.
- **Движок:** при первой установке программа спрашивает — основной (PyTorch, режим
  `YUE2_BACKEND=torch-eager`: PyTorch под Windows собран без flash-attention, которую вызывает режим
  CUDA-графов YuE2) или **yue2.cpp** (эксперимент: меньше видеопамяти — q8 ≈ 8 ГБ, экономный режим меньше).
  Пункт yue2.cpp появляется, только если в этой сборке пакета есть его программа (`yue2cpp\yue-server.exe`;
  собирается в релизе, см. `tools/yue2cpp-build.ps1` — пакет без неё деградирует до «только основной»).
  Модель (bf16/q8/q6/q5) и экономный режим предлагаются по объёму карты (8 ГБ → q6, экономный),
  веса (~2,6–7,2 ГБ) скачиваются тут же; после скачивания выбор пишется в `data\settings.json` и больше
  не спрашивается. Без вопросов: `partscaster-worker.exe --engine cpp --cpp-model q8 [--cpp-low-vram]`
  (или `--engine torch`). Сменить движок позже — в «Настройках» приложения (модель, «Скачать»,
  экономный режим). Библиотеки CUDA пользователю не нужны: программа yue2.cpp берёт их из venv
  (torch cu128), но пакет разработки CUDA для сборки — нужен только тому, кто собирает релиз.
- Необязательные компоненты (whisper, RoFormer, Seed-VC) программа не ставит.

## Воркер в WSL2 (dual-boot Windows/Linux)

На Windows-буте поднимается клон в WSL2, GPU пробрасывается (CUDA в WSL2 поддерживается):

```powershell
wsl --install -d Ubuntu            # один раз; драйвер NVIDIA уже стоит в Windows
```

Внутри WSL (Ubuntu):
```bash
# 1. окружение — как в разделе выше (venv, пакеты, whisper-venv)
# 2. веса не перекачивать: HF_HOME указывает на общий с Linux-бутом диск
#    (пример для диска D:):
export HF_HOME=/mnt/d/AI/hf-cache
# 3. общие данные с Linux-бутом — та же папка на общем диске (или своя):
export PARTSCASTER_DATA_DIR=/mnt/d/partscaster-data
# 4. Ollama под Windows: localhost Windows из WSL2
export PARTSCASTER_OLLAMA_URL=http://$(ip route show default | awk '{print $3}'):11434/api/chat
#    (в WSL с mirrored-сетью подойдёт и http://127.0.0.1:11434/api/chat)
# 5. запуск:
cd ~/partscaster/current && ~/yue/.venv/bin/python -m uvicorn yue_worker:app --host 0.0.0.0 --port 8091
```

Приложение на Windows подключается к `http://localhost:8091` (или к IP WSL —
`wsl hostname -I`). Нюанс: если данные делятся через общий диск, не запускайте
воркеры обоих бутов одновременно — SQLite-база одна.

## Сборка desktop-приложения по ОС

Требования: Go 1.22+, Node.js 20+, [Wails CLI v2](https://wails.io)
(`go install github.com/wailsapp/wails/v2/cmd/wails@latest`).

**Пакеты по ОС** (приложение + `partscaster-mcp` + архив воркера + `INSTALL.txt` в `dist/`):

| Цель | Где собирается | Итог |
|---|---|---|
| `make dist-linux` | Linux | `partscaster-<версия>-linux-<арх>.tar.gz` |
| `make dist-windows` | Linux или Windows (Wails под Windows собирается без cgo) | `partscaster-<версия>-windows-amd64.zip` |
| `make dist-mac` | только Mac (Wails под macOS — cgo и SDK Apple) | `partscaster-<версия>-darwin-<арх>.tar.gz` |

На чужой ОС `dist-linux` и `dist-mac` отказывают с подсказкой; в CI пакет каждой ОС собирается на своём
раннере. На Linux с docker перед пакетом собирается yue2.cpp (`make yue2cpp`), если его ещё нет в `dist/`, —
архив воркера одинаков во всех пакетах.

**Linux (основная, всё протестировано):**
```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
make build          # wails build с версией из git (make release — пакет в dist/)
# бинарник: build/bin/partscaster
make install-desktop   # ярлык в меню приложений (~/.local/share/applications)
```
Ярлык нужен и ради иконки: GNOME на Wayland берёт её только из `.desktop`-файла
(сопоставляет по имени `partscaster`), иконку самого окна там не показывает.
Запуск с ярлыка не видит переменных окружения шелла — адрес воркера задаётся
в настройках приложения, а не через `PARTSCASTER_URL`. Убрать — `make uninstall-desktop`.
Звук — через встроенный pw-play плеер (PipeWire): в webkit2gtk у webview нет
аудиовыхода, это ограничение именно Linux-сборки.

**Windows:**
```powershell
wails build          # webview2 (Edge) — ставится автоматически
# бинарник: build\bin\partscaster.exe
```
Звук — через WPF MediaPlayer (PowerShell-обёртка). Воркер — архивом из релиза
(или `make worker` из WSL/Git Bash). Воркер может оставаться на GPU-машине —
приложению нужен только HTTP-доступ к нему.

**macOS:** `wails build` (WKWebView). Те же нюансы, что и Windows: pw-play нет,
нужен аудио-бэкенд (afplay или `<audio>` — в WKWebView звук работает). Не тестировалось.
