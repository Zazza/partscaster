# MCP-сервер (partscaster-mcp)

**Русский** · [English](../en/mcp.md)

`partscaster-mcp` даёт AI-агенту (Claude Code, Claude Desktop, Codex и другим) управлять Partscaster так же,
как человек в приложении: ставить треки, править их в студии, накладывать эффекты и пресеты, настраивать и
устанавливать воркер. Работает по [Model Context Protocol](https://modelcontextprotocol.io) через stdio и
ходит в тот же HTTP API воркера ([api.md](api.md)). Файл нужен тем, кто подключает агента, и тем, кто
дописывает инструменты.

## Сборка

```bash
make mcp          # → build/bin/partscaster-mcp
make mcp-data     # перегенерировать данные библиотеки из frontend/src
```

MCP-сервер — отдельная программа: правки в `internal/` или `cmd/partscaster-mcp` начинают работать только
после `make mcp` и переподключения клиента (в Claude Code — `/mcp`).

`make mcp` вшивает в программу отпечаток исходников. При первом вызове и дальше раз в 30 минут MCP сверяет
его с репозиторием; если код уже другой, к ответу инструмента добавляется «⚠ MCP-сервер устарел… make mcp и
переподключите». Без репозитория рядом (программа установлена отдельно) проверки нет.

`make mcp` и `make release` вшивают и версию сборки (`git describe`) — она видна в `serverInfo.version` при
подключении. Версию воркера (`GET /version`) MCP сверяет со своей тоже раз в 30 минут: воркер старее — к
ответам приписка «⚠ … обновите воркер», новее — «обновите приложение». Версия воркера — в выводе `status` и
`doctor`.

## Подключение

Проще всего — из приложения: «Настройки» → «Для AI-агентов» показывает готовые фрагменты для каждого клиента
с настоящим путём к `partscaster-mcp` (он лежит рядом с приложением) и текущим адресом воркера. Кнопка
«Подключить к Claude Code» выполняет `claude mcp add partscaster --scope user …` (сервер виден во всех
проектах) и заодно снимает старую регистрацию `yue-studio`, если она была.

Переменные окружения:

| Переменная | Значение |
|---|---|
| `PARTSCASTER_URL` | Адрес воркера; не задан — адрес из настроек приложения, без них `http://localhost:8091` |
| `PARTSCASTER_MCP_DIR` | Куда скачивать файлы (треки, партитуры, спектрограммы); по умолчанию `~/partscaster-mcp-downloads` |

### Claude Code

```bash
claude mcp add partscaster --scope user -e PARTSCASTER_URL=http://gpu-host:8091 -- /путь/к/partscaster-mcp
```

Для клона репозитория, чтобы адрес воркера не попал в общие настройки, — `--scope local`:

```bash
make mcp
claude mcp add partscaster --scope local -e PARTSCASTER_URL=http://gpu-host:8091 -- "$PWD/build/bin/partscaster-mcp"
```

### Claude Desktop

`claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "partscaster": {
      "command": "/путь/к/partscaster-mcp",
      "env": { "PARTSCASTER_URL": "http://gpu-host:8091" }
    }
  }
}
```

### Codex (`~/.codex/config.toml`)

```toml
[mcp_servers.partscaster]
command = "/путь/к/partscaster-mcp"
env = { PARTSCASTER_URL = "http://gpu-host:8091" }
```

## Подтверждение человеком

Инструменты, отмеченные **†**, необратимы: они требуют `confirm=true`, и агент обязан сначала спросить
пользователя. Это `cancel`, `delete_job`, `queue_cancel`, `dsp_variant_delete`, `sound_preset_delete`,
`voice_delete`.

Ещё три инструмента без `confirm` только показывают план, а действуют с `confirm=true` — тоже после вопроса
пользователю: `recipe_import` (без `confirm` — превью импорта), `install_worker` и `install_app` (по
умолчанию сухой прогон; установка — `dry_run=false` и `confirm=true`).

## Инструменты

Всего 76. В колонке «Параметры» обязательные — **жирным**.

### Очередь воркера

Та же очередь, что человек видит в приложении. Долгие инструменты (генерации, `plan`, `job_preview`,
`make_stems`, `rebuild_sections`, `sound_preset_apply`, `revoice_apply`, распознавание текста и
нот, движок эффектов с усилителем и т. п.) встают в неё с пометкой «ИИ» и ждут своей очереди. Если человек
отменил операцию, инструмент вернёт ошибку «отменено человеком».

| Инструмент | Параметры | Что делает |
|---|---|---|
| `queue_list` | — | Идущая операция, ждущие по порядку, последние завершённые, пауза. У записи: `id`, `kind` (generate, preset, rebuild, stems, fx, transcribe, whisper, voice, plan, preview), `title`, `job_id`, `source` (`app` — человек, `mcp` — ИИ), `state`, `result_job_id`, `error`; ход пресета: `stage` (prepare, parts, levels, render, verify, fix), `pass` (номер сборки), `eta_s` (оценка оставшегося, с; `null` — ещё нет), `stage_since` (с с начала шага) |
| `queue_move` | **id**, **dir** | Подвинуть ждущую запись: `dir` −1 — раньше, 1 — позже |
| `queue_cancel` † | **id**, **confirm** | Ждущую — снять, идущую — остановить |
| `queue_pause` | **paused** | Пауза: новое не начинается, идущая доделывается; `false` — снять |

### Треки и генерация

| Инструмент | Параметры | Что делает |
|---|---|---|
| `status` | — | Доступен ли воркер, загружена ли модель, версия воркера |
| `jobs` | folder, limit, brief | Список треков. `folder` — папка («Альбом»…; `"-"` — без папки), `limit` — последние N, `brief` — короткие строки без стиля, текста и плана |
| `job_update` | **job_id**, title, folder, parent_id | Название и папка трека (версии следуют за корнем); `folder=""` — убрать из папки; `parent_id` — перенести под другой трек (производная версия, сворачивается под ним) |
| `submit` | **style**, **lyrics**, title, seed, n, cot, draft, arc, max_tokens, temperature, cfg, parent_id, role, sound_preset_ids | Поставить трек в очередь. `n` — веер из 1–10 треков с сидами base+0…n−1; `temperature` 0.5–1.5 (0 — по умолчанию 1.0); `cfg` 1–4 (0 — по умолчанию 1.5); остальное — как у `POST /jobs` ([api.md](api.md)). Инструментал — `lyrics='[Instrumental]'` |
| `plan` | **style**, **lyrics**, seed | План трека (ABC) без звука — посмотреть и поправить ноты до генерации. Медленно |
| `render_abc` | **abc**, **style**, lyrics, title, seed, parent_id, role | Трек по своей партитуре. С `parent_id` и `role=section` — кусок для вклейки под родителем; `seed=0` — сид родителя |
| `cancel` † | **job_id**, **confirm** | Снять трек с очереди или остановить генерацию |
| `retry_job` | **job_id** | Упавший или отменённый трек — снова в очередь с теми же параметрами |
| `delete_job` † | **job_id**, **confirm** | Удалить трек со всеми файлами |
| `artifacts` | **job_id**, file | Скачать файл трека в каталог загрузок и вернуть путь. `file` — имя в папке трека (`audio.flac`, `audio.mp3`, `audio.wav`, `score.abc`, `request.abc`…); пусто — mp3, если есть, иначе основной звук |
| `job_score` | **job_id** | Таймлайн партитуры: такты × голоса, аккорды, секции, секунды |
| `job_preview` | **job_id**, **from_sec**, **to_sec** | Быстрый кусок трека из сохранённых латентов (без новой генерации), путь к файлу |

### Тексты, транскрипция, импорт

| Инструмент | Параметры | Что делает |
|---|---|---|
| `transcribe` | **path** | Аудиофайл с ПК → ABC-партитура (SheetSage2) для кавера |
| `transcribe_job` | **job_id** | Снять партитуру с готового трека без неё (импорт без транскрипции, вариант эффекта). Готовую не перезаписывает |
| `job_lyrics` | **job_id**, language | Что поёт готовый трек (faster-whisper) — проверка для агента: спелось ли то, что задано; есть дорожка голоса — распознаётся она. `language` — `en`, `ru`, … или `auto`; пусто — по стилю трека |
| `import_track` | **path**, title, folder, transcribe | Свой трек с ПК как трек библиотеки; `transcribe=true` — сразу снять партитуру (медленно) |

### Слой над моделью

То, чего нет у самой модели YuE: те же механизмы, что в студии приложения.

| Инструмент | Параметры | Что делает |
|---|---|---|
| `continue_job` | **job_id**, **from_sec**, seed, abc, style_add, to_sec | «Продолжение с места»: трек до `from_sec` + продолжение моделью, без склейки. `abc` — изменённый план, `style_add` — что изменить в звучании, `to_sec` — только кусок до этой отметки (проверить правку в контексте) |
| `set_head` | **job_id**, **head_id** | Основная версия песни: `job_id` — корень, `head_id` — версия-потомок или 0 (сам трек) |
| `variant_track` | **job_id**, **file**, title, voice_src | Вариант эффекта → отдельный трек-версия; `voice_src` — чей голос подставлен (после «перепеть») |
| `rebuild_sections` | **job_id**, **specs**, as_track, title, voice_src | Пересборка дорожек трека с чистого оригинала, как в студии. Запись `specs`: `{child_id, from, to, stems, …}` — вклейка куска из рендера `child_id`; `child_id 0` + `db` — громкость дорожек (−100 — заглушить); `revoice` — «перепеть»; `chain`/`params`/`steps` — эффект ffmpeg на дорожки в окне; `envelope` — линия громкости; `engine` — цепочка движка; `add` — партия поверх трека; `place` — место в стерео; `master` — мастер; `mix` с `chain`/`steps`/`envelope` — эффект на весь собранный микс (после правок дорожек, до мастера). `as_track` — сразу трек-версией |
| `revoice_start` | **job_id**, **from**, **to**, abc, takes | «Перепеть с места», шаг 1: голос части поётся заново от источника голоса версии; `takes` — 1–4 дубля (2); `abc` — изменённая мелодия (не выше потолка голоса). Ответ — id дублей |
| `revoice_apply` | **job_id**, **take_id**, **from**, **to**, beat_sec, title | Шаг 2: голос готового дубля подставляется только в окно `from`–`to` → новая версия с `voice_src` = дубль |
| `splice` | **job_id**, **parts**, crossfade, title | Склеить куски версий (`{job_id, from, to, gain_db?}`) в новую версию с переходом `crossfade` (0.05 с): вернуть вырезанный проигрыш, собрать лучшие дубли. Резать по границам тактов одного исполнения |
| `plan_check` | **job_id**, **abc**, from | Что изменилось в плане: такты, время, потолок голоса (верх мелодии + 2 ступени), правки до отметки `from` (продолжение их не сыграет). То же делают `continue_job` и `revoice_start` с `abc` |
| `vocal_contour` | **job_id**, from, to | Высота голоса по тактам плана — спето ли по плану, не ушёл ли голос в писк. Нужен `make_stems` |
| `find_tones` | **job_id**, from, to, stem | Узкий «свист» в окне — частоты для цепочки `dewhistle` («Убрать свист») |
| `chord_grid` | **job_id** | Аккорды и секции плана по тактам звука `{bpm, bars}` — основа синт-партии. Нет плана — сначала `transcribe_job` |
| `beat_grid` | **job_id**, from, to | Темп и сильная доля в окне — `bpm` и `offset` для эффектов в такт (цепочка `gate`, «Ритм-гейт») и перкуссии |

### Студия трека

| Инструмент | Параметры | Что делает |
|---|---|---|
| `make_stems` | **job_id**, model | Разделить трек: основные дорожки drums/bass/other/vocals, подробные guitar/piano и (только RoFormer) части барабанов kick/snare/toms/hh/ride/crash. `model` — `htdemucs` (быстро) или `roformer` (чище, в ~4,5 раза дольше) разово; пусто — настройка `stems_model` |
| `overdub` | **job_id**, **style**, lyrics, seed, gain, abc | Партия поверх трека по его партитуре с новым стилем. `gain` 0.1–1 (0.5); 0 — кавер новым треком без смешивания. `abc` — свой план партии. Без `lyrics` — вокализ |
| `guitar_part` | **job_id**, style | Что сыграет гитарист «по аккордам плана» (блок `gtrnotes`): такты, удары с нотами. `style` 0–4: авто, бой, перебор, бас + аккорд, квинты |
| `voice_convert` | **job_id**, **ref_job_id**, ref_from, ref_dur, steps, title | ЭКСПЕРИМЕНТ «голос альбома»: голос трека тембром образца (Seed-VC на воркере). Голос узнаётся, но дрожит — см. [deployment.md](deployment.md). Без Seed-VC — ошибка 503 |
| `voice_preview` | **job_id**, **ref_job_id**, **from**, **to**, ref_from, ref_dur, steps | ЭКСПЕРИМЕНТ «голос альбома», прослушать кусок: голос куска `[from, to)` трека (3–30 с) тембром образца → файл `preview-vc-*.flac` в папке трека; версии не создаёт |
| `analyze_job` | **job_id** | Замеры трека: темп, крест-фактор, динамика, спектр — для сравнения вариантов |
| `job_peaks` | **job_id**, file, bins | Огибающая громкости числами: найти провал, вступление, место правки |
| `job_spectrum` | **job_id**, file | Спектрограмма PNG в каталог загрузок. Нужен ffmpeg на воркере |
| `draft_list` | — | Черновики студии (незаконченная работа) всех треков: `[{job_id, state, updated_at}]` |
| `draft_get` | **job_id** | Черновик трека: `state` — `{v, edits, queue, ts, gen?, applying?}` (правки «Правок трека», ожидающие рендеры, изменения с рендером — голос/приёмы плана/без голоса, ход «применить всё»); нет — пусто |
| `draft_set` | **job_id**, **state** | Записать черновик трека (JSON-объект). Студия подхватит его при следующем запуске |
| `draft_clear` | **job_id**, **confirm** | Удалить черновик трека — правки студии пропадут. Только с `confirm=true` после согласия человека |

### Эффекты ffmpeg (на ПК)

Что делает каждая цепочка — [effects.md](effects.md).

| Инструмент | Параметры | Что делает |
|---|---|---|
| `dsp_chains` | — | Цепочки эффектов, их крутилки и диапазоны. `group` — группа (как разделы effects.md: space, pedals, modulation, filter, dynamics, time, lofi, voice); `voice=true` — примочки на голос (мегафон, телефон, перегруз, слэпбэк); `for_voice=true` — общий эффект, уместный и на голосе; цепочки с `key` (`ducking`, `key=drums`) — только на дорожку |
| `dsp_presets` | — | Готовые наборы педалей: `steps` для превью и применения |
| `dsp_apply` | **job_id**, chain, params, steps, stem, from, to, db | Применить цепочку (`chain` + `params`) или педали по порядку (`steps`). Без `stem` — вариант на весь трек (педали — `dsp-pedals.flac`); со `stem` — только эта дорожка в окне `from`–`to` через пересборку, `db` — поверх выравнивания. У голосовых цепочек громкость выравнивается по исходной; реверб и дилей звучат хвостом после `to` |
| `dsp_preview` | **job_id**, chain, params, steps, stem, solo, from, to | Послушать эффект на куске (по умолчанию 20–35 с + хвост до 3 с) → вариант `dsp-preview-*.flac`; `solo` — только обработанная дорожка |
| `dsp_variants` | **job_id** | Варианты трека с замерами и подписью |
| `dsp_variant_delete` † | **job_id**, **file**, **confirm** | Удалить вариант (`dsp-*` / `overdub-*`); дорожки не удаляются |
| `volume_envelope` | **job_id**, **points**, stem | Линия громкости `[{t, db}]` (−30…+12 дБ, до 200 точек) на весь трек (`dsp-envelope.flac`) или дорожку |

### Звуковой движок (на воркере)

Цепочка блоков: педали → усилитель NAM → кабинет → пространство, плюс сэмплеры, синт, перкуссия, мастер —
24 блока, описаны в [effects.md](effects.md). Обработка не сдвигает звук — результат годится для вклейки нота
в ноту.

| Инструмент | Параметры | Что делает |
|---|---|---|
| `fx_apply` | **job_id**, **chain**, source, from, to, output, label, preview, fade, add, file, in_place | Цепочка на трек (`source=mix`) или дорожку → вариант `dsp-fx-*.flac`. `output` — `mix` или `solo`; `preview` — только послушать кусок `from`–`to`; `add` — партия поверх трека; `file` — вход из варианта-микса (так кладётся мастер), `in_place` — записать в тот же файл. Поля — как у `POST /jobs/{id}/fx` ([api.md](api.md)) |
| `fx_blocks` | — | Параметры каждого блока: умолчания, границы, шаг, подписи |
| `fx_presets` | — | Готовые цепочки страницы «Инструменты» `{id, name, note, stems, group, amp_hint, chain}`. Пустой `amp.model` — подставить первый захват из `fx_assets`, в имени которого есть подстрока из `amp_hint` |
| `fx_assets` | — | Загруженные захваты NAM, IR и наборы сэмплов |
| `fx_asset_upload` | **kind**, **path** | Загрузить свой `.nam` (`kind=amp`) или `.wav` (`kind=ir`) с ПК |
| `fx_kit_install` | **name** | Поставить набор сэмплов на воркер (каталог — в [api.md](api.md#каталог-наборов-сэмплов)): барабаны `osdk`, басы Karoryfer, оркестр `vsco-*`, фортепиано `salamander`, гитары для `gtrnotes`; драм-машины `tr808`/`tr909`/`linn`/`cr78`/`simmons` и `synthbass` синтезируются без сети |
| `fx_kit_progress` | — | Ход идущей установки набора |
| `fx_phrase` | phrase, tempo, chain, stems, bypass | Фразы без трека: без `phrase` — каталог (гитара записями, бас и барабаны нотами); с `phrase` — круг через цепочку, файл `phrase-*.wav`. `tempo` 0,5…1,5 |
| `fx_instruments` | action, id, name, base, group, stems, chain, extra | Свои инструменты на основе готовых: `action` — `list` (по умолчанию), `create`, `update`, `delete`. В приложении — рядом с готовыми |

### Пресеты звука

Пресет — рецепт обработки трека целиком: правки дорожек, финал на микс, мастер → версия «трек · пресет».

| Инструмент | Параметры | Что делает |
|---|---|---|
| `sound_presets` | id | Без `id` — краткий список (встроенных больше сотни, `family`: Рок, Тяжёлое, Электроника, Поп и другое; течение — название до « · »); с `id` — рецепт целиком |
| `sound_preset_create` | **name**, note, specs, final, master, parts, target_lufs, reference_job_id | Свой пресет. Поля — как в теле пресета ([api.md](api.md#тело-пресета)); нужно хоть что-то из `specs`, `final`, `master`, `target_lufs` |
| `sound_preset_update` | **preset_id**, **name**, note, specs, final, master, parts, target_lufs, reference_job_id | Заменить свой пресет; не переданные `master`, `parts`, `target_lufs` остаются прежними (`[]` / `null` — снять). Встроенный — нельзя |
| `sound_preset_delete` † | **preset_id**, **confirm** | Удалить свой пресет |
| `sound_preset_draft` | **job_id**, **preset_id**, db | Пресет в черновик студии (синхронно): подготовка как у `sound_preset_apply`, без сборок; у записей с целью — `verify_db`, сверка баланса — при «применить всё» в студии; трек не меняется → `{"preset": {sections, final?, master?, warnings?}}`. Запись в `state.edits` черновика (`draft_get` → `draft_set`, прежнюю запись с `preset` убрать, новую — первой): `{"childId": <отрицательное уникальное>, "instId": "preset", "from": 0, "to": 0, "lead": 0, "beat": 0, "db": 0, "stems": [], "fadeIn": 0, "fadeOut": 0, "keepHighHz": 0, "step": "done", "label": <название>, "preset": {"id", "name", …ответ}}` |
| `sound_preset_apply` | **job_id**, **preset_id**, db | Применить сейчас (синхронно, минуты): разделит трек, поставит недостающие наборы, пересоберёт, финал — на результате → `{version_id, file, warnings?}`. `db` — громкость записей на этот раз: `{"индекс": дБ}` |

Пресет можно заказать и при создании трека — `submit` с `sound_preset_ids` (до 3): их применит открытое
приложение, когда трек будет готов. У черновика нельзя.

### Рецепт трека

Перенос песни на другую установку без аудио.

| Инструмент | Параметры | Что делает |
|---|---|---|
| `recipe_export` | job_id, path | Песня трека (от корня) → файл `*.partscaster.json` в каталог загрузок или `path` (старое `*.yue-track.json` тоже принимается); `warnings` — что не вошло |
| `recipe_import` | path, mode, confirm | Без `confirm` — превью (название, шаги, модель, предупреждения); `confirm=true` — начать. `mode`: `seed` (тот же сид, по умолчанию) или `plan` (по нотам — если модель воркера другой версии) |
| `recipe_imports` | retry, dismiss | Ход импортов; `retry=<id>` — продолжить упавший с того же шага, `dismiss=<id>` — убрать из списка |

Импорт ведёт сам MCP, пока он запущен (или открытое приложение): генерации заново, правки по порядку.
Исполнение получается очень близким, но не тем же; правки звука повторяются точно. Журнал правок пишут
приложение и MCP после каждого успешного изменения.

### Голоса примерочной

| Инструмент | Параметры | Что делает |
|---|---|---|
| `voices_list` | — | Карточки голосов: ручки характера, сид прослушивания, жив ли исходный трек |
| `voice_create` | **name**, **job_id**, params, seed | Сохранить карточку голоса из трека-прослушивания; `params` — JSON ручек `{register, rough, creak, delivery, breath, extra}` |
| `voice_delete` † | **voice_id**, **confirm** | Удалить карточку и копию аудио |

### Библиотека стилей

| Инструмент | Параметры | Что делает |
|---|---|---|
| `styles` | group, search | Группы стилей (встроенные и свои) и поиск — готовые строки стиля для `submit` |
| `slot_options` | slot, search | Русская фраза → английский тег YuE по полям: genre, rhythm, guitars, keys, vocals, mood, production, language |

### Настройки

| Инструмент | Параметры | Что делает |
|---|---|---|
| `config_get` | — | Адрес воркера, пути данных и движки воркера |
| `config_set` | server_url, stems_auto, stems_model, fx_engine, engine, cpp_model, cpp_low_vram | `server_url` сохраняется и в настройки приложения; `stems_auto` — когда делать дорожки: `studio` (при открытии студии) / `always` (сразу у каждого готового трека) / `manual`; `stems_model` — `htdemucs` или `roformer`; `fx_engine` — движок эффектов вкл/выкл; `engine` — `torch` или `cpp` (yue2.cpp, эксперимент: меньше видеопамяти, тот же сид даёт другую песню), `cpp_model`, `cpp_low_vram` — со следующей генерации |
| `engine_model_install` | **name** | Скачать модель yue2.cpp: `bf16` (7,2 ГБ), `q8` (3,8 ГБ), `q6` (2,9 ГБ), `q5` (2,6 ГБ). Ждёт конца загрузки; потом — `config_set engine=cpp cpp_model=<имя>` |

### Установка и диагностика

| Инструмент | Параметры | Что делает |
|---|---|---|
| `doctor` | host | Проверка окружения: доступен ли воркер и какие компоненты у него стоят, ffmpeg, Go, Node; на машине с видеокартой (`host` — по ssh) — GPU и видеопамять, место на диске. Версия воркера |
| `install_worker` | host, archive, dry_run, confirm, whisper, seedvc, roformer, stems_model, engine, cpp_model, hf_home, seedvc_dir, sep_dir | Поставить воркер (Ubuntu + NVIDIA/CUDA). По умолчанию сухой прогон: агент показывает план и спрашивает, нужны ли whisper (~3,2 ГБ), Seed-VC (~11 ГБ) и RoFormer (~7,3 ГБ, веса некоммерческие), а при RoFormer — какой моделью делать дорожки. Установка — `dry_run=false`, `confirm=true` и явные `whisper`/`seedvc`/`roformer`. Перед установкой проверяются GPU и место на диске. Каталоги по умолчанию: `hf_home` — `~/yue/hf-cache`, `seedvc_dir` — `~/partscaster/seedvc`, `sep_dir` — `~/sep-venv` и `~/sep-models` |
| `install_app` | dry_run, confirm | Собрать приложение на этом ПК (`wails build`; нужны Go, Node, wails CLI). По умолчанию сухой прогон |

## Рецепты работы

**Минус без вокала.** «Поставь трек в стиле "70s analog reggae, 75 BPM" инструменталом» → `submit`; агент
ждёт по `jobs`, пока трек не станет `done`; «скачай mp3» → `artifacts` → путь к файлу; «сделай минус без
вокала» → `make_stems` + `rebuild_sections` с заглушенной дорожкой `vocals` → `artifacts`.

**Синт по аккордам.** `chord_grid` (аккорды по тактам звука) → блок `synth` с нотами в секундах трека →
`rebuild_sections` с записью `{child_id: 0, stems: ["mix"], add: true, engine: [synth…, эффекты…]}`.
Послушать заранее — `fx_apply` с `source=mix`, `preview`, `add`. Готовые синты — `fx_presets` со
`stems: ["synth"]`.

**Перкуссия по сетке.** Такты — `chord_grid` (если есть план) или `beat_grid` → блок `perc` с ударами
`notes: [{t, d, vel}]` → `rebuild_sections` записью с `add`, как у синта. Готовые — `fx_presets` со
`stems: ["perc"]` (у каждого `pattern` и `swing`).

**Эффект на одну дорожку.** `dsp_apply` со `stem` (и `from`/`to`, `db`) или запись `rebuild_sections` с
`chain`/`params` — остальные дорожки не меняются. Цепочки с `key` (`ducking`) — только со `stem`. В трек через
движок — запись `rebuild_sections` `{child_id: 0, stems, from, to, engine: [блоки]}`; правки копятся со
вклейками.

**Сведение и мастер.** Место в стерео — `place: {pan, width}`: у записи с `add` — место партии, отдельной
записью `{child_id: 0, stems: [дорожка], place}` — место дорожки. Мастер — запись
`{child_id: 0, master: true, engine: [glue…, limiter {target_lufs, ceiling_db}]}` на весь собранный микс;
ответ — громкость (LUFS) и истинный пик. Послушать мастер на готовом миксе — `fx_apply` с `file` и `preview`,
записать в тот же файл — `in_place`. Готовые мастера — `fx_presets` со `stems: ["master"]`.

## Установка воркера через агента

Воркер ставится архивом `worker-<версия>.tar.gz`, его `install.sh` ставит основу (см.
[deployment.md](deployment.md)). Архив — параметр `archive`; без него MCP из исходников соберёт его сам
(`make worker-tar`), а релизный MCP возьмёт архив, лежащий рядом с `partscaster-mcp`. Необязательные
компоненты (whisper, Seed-VC, RoFormer) MCP ставит после основы.

- **Всё на одном ПК**: `install_worker` без `host` — распаковка и `install.sh` локально.
- **Видеокарта на другой машине**: `install_worker` с `host=user@gpu-host` — архив копируется по scp,
  остальное идёт по ssh (нужен настроенный ssh-доступ).

## Разработка

- Сервер — `internal/mcp/` (JSON-RPC по stdio, без внешних зависимостей), точка входа —
  `cmd/partscaster-mcp`.
- Клиентская часть переиспользует `internal/yue.Service` (заглушки для тестов — в `server_test.go`).
- Данные библиотеки (`internal/mcp/*.json`) генерируются из фронтенда (`tools/mcp-gendata.sh`) и вшиваются
  через `go:embed`. **Руками их не править** — правь источники в `frontend/src/` и запускай `make mcp-data`.
- Новая возможность воркера = новый метод `yue.Service` + инструмент MCP (паритет приложения и MCP, см.
  [AGENTS.md](../../AGENTS.md)).
