# Проверка: воркер Yue Studio на Windows без WSL

Разовая разведка перед решением «делаем ли родной `yue-worker.exe` под Windows».
Читает человек и его агент на Windows-машине; ответы на вопросы ниже — итог проверки.

## Зачем

Сейчас воркер (Python-сервер `worker/yue_worker.py`, порт 8091: генерация YuE2, дорожки, эффекты,
очередь задач) ставится только на Linux — `worker/install.sh` (uv, venv, systemd). На Windows — только
внутри WSL2, руками (`docs/deployment.md`, раздел «Воркер в WSL2»). Пользователи просят простую установку
на Windows без WSL.

Задумка: отдельный `yue-worker.exe` (небольшой запускатель на Go). Двойной клик — при первом запуске он
ставит Python, пакеты, модели (и yue2.cpp) в свою папку и запускает воркер; дальше — просто запускает.
Окно открыто — воркер работает, закрыл — остановлен. Студия подключается к `localhost:8091` и не меняется.

Делать его имеет смысл, только если Python-воркер вообще работает на Windows. Это и проверяется.

## Что уже известно (проверено без Windows, 2026-10-12)

| Что | Итог |
|---|---|
| torch / torchaudio 2.10.0 + CUDA 12.8 | есть сборки `win_amd64` для Python 3.10–3.12 (download.pytorch.org/whl/cu128) |
| lameenc, soundfile, safetensors, tiktoken | есть сборки `win_amd64` на PyPI |
| demucs, librosa, pretty_midi, mido, mir_eval | чистый Python |
| flash-attn (из-за него YuE у других не идёт на Windows) | воркеру не нужен: в рабочем окружении его нет |
| Linux-код в пакете `yue2` (`fcntl`, `killpg`) | только в `yue2/fast.py` — режим vllm (`pipeline.py`: `if self.backend == "vllm"`), воркер его не использует |
| Модели `m-a-p/YuE2-3B` | открытые, токен Hugging Face не нужен |
| yue2.cpp на Windows | в репо есть `buildcuda.cmd` (Visual C++ Build Tools + CUDA Toolkit); готовых Windows-сборок нет |
| Официальная поддержка YuE2 | README YuE: только «Linux · Python 3.12 · NVIDIA». Что `yue2_infer` запускали на Windows — сведений нет |
| Пакет `yue2` (`yue2_infer` 0.1.6) | нет ни в `worker/requirements.txt`, ни на PyPI; на GPU-машине стоит из клона YuE `72272f9`. Скрипт ставит его архивом этого коммита с GitHub |
| Код воркера | `fcntl` в `worker/yue_worker.py` и `worker/engine_cpp.py` (импорт внутри функций — на импорт модуля не влияет); `start_new_session` при запуске `yue-server` |

Похожий проект под Windows — [siliconsense/yue2-studio-pc](https://github.com/siliconsense/yue2-studio-pc):
`start.bat` → uv, PyTorch по версии драйвера, модели, локальный сервер. Но движок там ComfyUI, не `yue2_infer`.

Главный неизвестный — **генерирует ли `yue2_infer` на Windows** (CUDA graphs, SDPA). Его закрывает только запуск.

## Как запустить

Нужно: Windows 10/11, NVIDIA с драйвером под CUDA 12.8, ~25 ГБ на диске, интернет; репозиторий Yue Studio
на этой машине (ветка с этой папкой). Для шага 6 дополнительно — git, cmake, ninja, CUDA Toolkit и
Visual Studio Build Tools **версии 18** (`buildcuda.cmd` ищет `vcvars64.bat` по жёсткому пути
`C:\Program Files (x86)\Microsoft Visual Studio\18\BuildTools\...`). Нет их — шаг 6 честно падает с
«пропущено: нет …», остальное от него не зависит; `-SkipCpp` пропускает его совсем.

```powershell
powershell -ExecutionPolicy Bypass -File tools\windows-check\check-windows.ps1
# другая папка: -Root D:\yue-check ; без yue2.cpp: -SkipCpp
```

Всё ставится в `-Root` (по умолчанию `C:\yue-check`): uv, Python, venv, кеш пакетов, модели (`HF_HOME`),
данные воркера (`YUE_DATA_DIR`). Система, PATH, реестр не меняются. Повторный запуск докачивает и
пропускает готовое. Убрать всё — удалить папку. Время — от получаса до часа, в основном скачивание
(пакеты ~7 ГБ, модель YuE2 ~10 ГБ).

## Шаги и как читать итог

В конце печатается таблица `OK / FAIL` по шагам, полный вывод — `<Root>\report.txt`.

| Шаг | Что проверяет | FAIL значит |
|---|---|---|
| 1 система | видеокарта, драйвер, место, наличие git/cmake/ninja/nvcc | нет NVIDIA / драйвера или < 25 ГБ — дальше смысла нет |
| 2 uv и Python 3.12 | uv 0.12.19 (sha256 сверяется), Python в папку | сеть/прокси/антивирус |
| 3 пакеты воркера | `requirements.txt` + `yue2` с torch cu128; печатает `torch.cuda.is_available()` | какой пакет не встал — в выводе uv. `gpu False` при OK — torch без CUDA или драйвер старый |
| 4 тесты воркера | `unittest` воркера (CPU) | не блокер сам по себе: список упавших — это список правок переносимости нашего кода (`<Root>\unittest.txt`) |
| 5 генерация через воркер | запуск `uvicorn yue_worker:app` на `127.0.0.1:8091`, черновик ~20 с через `POST /jobs` | **главный шаг**. Ошибка воркера — `<Root>\worker.err.log`; трек — в `<Root>\data` |
| 6 сборка yue2.cpp | клон закреплённого коммита `c893629`, `buildcuda.cmd`, `yue-server.exe --help` | без него на Windows только основной движок (PyTorch) |

Если шаг 5 упал, полезно отделить «не работает YuE2» от «не работает наш воркер»: прогнать пакет напрямую —

```powershell
$env:HF_HOME="C:\yue-check\hf-cache"
C:\yue-check\venv\Scripts\python.exe -c "from yue2 import YuE2Pipeline as P; p=P.from_pretrained('m-a-p/YuE2-3B', vae='m-a-p/YuE2-Vae', device='cuda').__enter__(); s=p(style='English, piano pop, female voice', lyrics='[Verse]\nNeon fades along the lane', cot='full', seed=1); s.save_artifacts(r'C:\yue-check\direct'); print(s.truncated)"
```

Прошло напрямую, но не через воркер — дело в нашем коде (правится). Не прошло и напрямую — YuE2 на
Windows не работает, родной воркер возможен только с yue2.cpp.

## Что вернуть в основную задачу

Короткий вывод (можно прямо в этот файл, раздел «Итог» ниже) и `report.txt`:

1. Шаги 3 и 5 — OK? Если нет — на чём упало (строка ошибки), и прошла ли прямая генерация.
2. Время генерации черновика и пик видеопамяти (`nvidia-smi` во время шага 5), видеокарта и драйвер.
3. Шаг 4 — какие тесты упали и почему (одной строкой на причину).
4. Шаг 6 — собрался ли yue2.cpp; если нет — на чём.
5. Что пришлось делать руками сверх скрипта (это потом должен будет делать `yue-worker.exe`).

Не делать в рамках проверки: правки кода воркера в develop, push, теги. Проверка разовая — после решения
папка `tools/windows-check/` удаляется или переезжает в задачу про `yue-worker.exe`.

## Итог

Проверено 2026-10-12, Windows 11, RTX 4070 Ti SUPER 16 ГБ, антивирус Касперского активен.

1. **Шаги 1–3** — OK, но с оговоркой: uv при пине `torch==2.10.0` взял CPU-сборку с PyPI вместо
   `2.10.0+cu128` со `--index-url` (нужен явный пин `torch==...+cu128`). Ручная доустановка
   `uv pip install torch==2.10.0+cu128 torchaudio==2.10.0+cu128 --index-url https://download.pytorch.org/whl/cu128`
   встаёт и работает: `torch.cuda.is_available() == True`.
2. **Шаг 5** — падал дважды по внешним причинам, сама YuE2 **работает**:
   - модель (~10 ГБ) скачалась до 6.8 ГиБ и оборвалась на SSL `self-signed certificate in
     certificate chain` — Касперский перехватывает HTTPS; лечится `REQUESTS_CA_BUNDLE` /
     `SSL_CERT_FILE` на bundle из корневых сертификатов хранилища Windows + certifi;
   - после доустановки cu128-torch генерация через воркер упала на `RuntimeError:
     USE_FLASH_ATTENTION was not enabled for build` — **Windows-сборка PyTorch 2.10.0+cu128
     скомпилирована без aten flash-attention** (op есть в схеме, реализация — заглушка).
     Дефолтный `backend="torch"` yue2 идёт через CUDA-граф с прямым `_flash_attention_forward`
     и на Windows умирает; авто-детект пакета (`hasattr` + `seqused_k` в схеме) этого не различает.
3. **Прямая генерация прошла**: `backend="torch-eager"` (без CUDA-графа, SDPA-внимание) —
   **24.8 с аудио за 74.3 с, пик VRAM 7.8 ГиБ**. Артефакт `C:\yue-check\direct\audio.flac`,
   без усечения. Вывод: **родной Windows-воркер возможен** — нужен `backend="torch-eager"`
   (или патч детекта flash в yue2), пока Windows-torch без FA.
4. **Шаг 4** — 6 failures + 47 errors, 770 skipped: почти всё переносимость (`/tmp` vs
   `C:/Users/.../Temp` во временных путях, install.sh-тесты требуют bash/posix).
   Полный список `C:\yue-check\unittest.txt`.
5. **Шаг 6** — пропущен: нет cmake/ninja/nvcc/VS 18 Build Tools.
6. **Руками сверх скрипта**: UTF-8 BOM для ps1 (Windows PowerShell 5.1 без BOM читает как ANSI
   и падает на русских комментариях), cu128-torch, CA-bundle, прямой прогон `torch-eager`.
   Для шага 5 пришлось временно остановить рабочий воркер в WSL (порт 8091) и вернуть после.

Не делалось: правки develop, push. Папка `C:\yue-check` оставлена (модель в кеше) до решения.

