#!/usr/bin/env bash
# Установка/обновление воркера Partscaster из распакованного архива (worker-<версия>/install.sh).
#
#   ./install.sh [--no-start] [--hf-home DIR]   установить эту версию и (без --no-start) перезапустить
#   ~/partscaster/current/install.sh --rollback   вернуть предыдущую версию
#   модель генерации (yue2.cpp): --cpp-model auto|bf16|q8|q6|q5 (auto — рекомендуемая по видеопамяти),
#     --cpp-low-vram — экономный режим (~4,5 ГБ); без флагов скачанная выбранная модель не меняется,
#     а если её нет — ставится рекомендуемая
#
# Раскладка: код — $PARTSCASTER_HOME/releases/<время>-<версия>/, ссылка $PARTSCASTER_HOME/current — действующая версия;
# данные ($PARTSCASTER_HOME/data) и настройки машины ($PARTSCASTER_HOME/worker.env) общие для всех версий и не трогаются.
# Необязательные компоненты (whisper, Seed-VC, RoFormer) ставит MCP-инструмент install_worker.
set -euo pipefail

# pwd -P: запуск через ссылку current/install.sh копирует саму версию, а не ссылку на неё
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"

# миграция со старого имени проекта: ~/yue-studio → ~/partscaster. Переносим данные,
# версии и worker.env, снимаем старый юнит yue-worker (иначе после перезагрузки поднялись
# бы оба и спорили за порт). Каталог ~/partscaster уже может существовать — deploy.sh
# кладёт архив в ~/partscaster/incoming, поэтому вливаем старое содержимое, а не mv целиком.
# Ссылку-синоним ~/yue-studio оставляем — страховка на случай абсолютных путей в старых
# данных. Явно заданный PARTSCASTER_HOME или уже мигрированные данные — миграция не нужна.
if [[ -z "${PARTSCASTER_HOME:-}" && -d "$HOME/yue-studio" && ! -d "$HOME/partscaster/releases" && ! -d "$HOME/partscaster/data" ]]; then
  echo "== миграция: ~/yue-studio → ~/partscaster =="
  XDG="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"
  XDG_RUNTIME_DIR="$XDG" systemctl --user disable --now yue-worker >/dev/null 2>&1 || true
  mkdir -p "$HOME/partscaster"
  shopt -s dotglob nullglob
  for f in "$HOME/yue-studio"/*; do
    dest="$HOME/partscaster/$(basename "$f")"
    [[ -e "$dest" ]] || mv -T "$f" "$dest" # занято только свежим incoming/ из deploy.sh
  done
  shopt -u dotglob nullglob
  rm -f "$HOME/.config/systemd/user/yue-worker.service" "$HOME/partscaster/units/yue-worker.service"
  XDG_RUNTIME_DIR="$XDG" systemctl --user daemon-reload || true
  # пустые каталоги (например incoming, чьё имя занял свежий deploy.sh) убираем — освобождаем имя
  find "$HOME/yue-studio" -mindepth 1 -type d -empty -delete 2>/dev/null || true
  rmdir "$HOME/yue-studio" 2>/dev/null || true # всё равно не пустой — просто без ссылки-синонима
  [[ -e "$HOME/yue-studio" ]] || ln -s partscaster "$HOME/yue-studio"
  # ключи worker.env со старым префиксом YUE_* — к этим же значениям новыми именами
  if [[ -f "$HOME/partscaster/worker.env" ]] && grep -q '^YUE_[A-Z_]' "$HOME/partscaster/worker.env"; then
    sed -i -e 's/^YUE_\([A-Z][A-Z_]*\)=/PARTSCASTER_\1=/' "$HOME/partscaster/worker.env"
    echo "worker.env: ключи YUE_* переименованы в PARTSCASTER_*"
  fi
fi

PARTSCASTER_HOME="${PARTSCASTER_HOME:-$HOME/partscaster}"
PARTSCASTER_VENV="${PARTSCASTER_VENV:-$HOME/yue/.venv}"
KEEP=3 # сколько прежних версий хранить кроме действующей

NO_START=0
ROLLBACK=0
HF_HOME_DIR="~/yue/hf-cache"
CPP_MODEL=""   # пусто — скачанный выбор пользователя, без него — рекомендуемая (как auto)
CPP_LOW_VRAM=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-start) NO_START=1 ;;
    --rollback) ROLLBACK=1 ;;
    --hf-home) HF_HOME_DIR="${2:?--hf-home: нужен каталог}"; shift ;;
    --cpp-model) CPP_MODEL="${2:?--cpp-model: auto, bf16, q8, q6 или q5}"; shift ;;
    --cpp-low-vram) CPP_LOW_VRAM=1 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    --engine) echo "неизвестный параметр: --engine — движок генерации один (yue2.cpp); модель: --cpp-model" >&2; exit 2 ;;
    *) echo "неизвестный параметр: $1" >&2; exit 2 ;;
  esac
  shift
done
case "$CPP_MODEL" in
  "" | auto | bf16 | q8 | q6 | q5) ;;
  *) echo "--cpp-model: auto, bf16, q8, q6 или q5 (получено «$CPP_MODEL»)" >&2; exit 2 ;;
esac

# пути уходят в юнит systemd: кавычки, обратный слэш, % (спецификаторы systemd) и перевод строки там
# не записать буквально — такие пути отклоняются до любых изменений (пробелы — можно, юнит их кавычит)
for p in "$PARTSCASTER_HOME" "$PARTSCASTER_VENV"; do
  if [[ "$p" == *[\"\\%]* || "$p" == *$'\n'* ]]; then
    echo "путь не годится для юнита systemd (кавычка, \\, % или перевод строки): $p" >&2
    exit 2
  fi
done

# systemctl --user по ssh без сессии не видит менеджер пользователя
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}"

expand_home() { # «~» и «~/…» → $HOME (systemd тильду в EnvironmentFile не раскрывает)
  case "$1" in
    "~") printf '%s' "$HOME" ;;
    "~/"*) printf '%s/%s' "$HOME" "${1#\~/}" ;;
    *) printf '%s' "$1" ;;
  esac
}

# ссылка current меняется атомарно: новая ссылка рядом и rename поверх старой
switch_current() {
  ln -sfn "$1" "$PARTSCASTER_HOME/current.new"
  mv -T "$PARTSCASTER_HOME/current.new" "$PARTSCASTER_HOME/current"
}

# health_ok — воркер отвечает на /health; curl есть не везде (на GPU-машине его может не быть) —
# тогда python из venv воркера, он там есть всегда
health_ok() {
  if command -v curl >/dev/null; then
    curl -sf --max-time 5 http://localhost:8091/health >/dev/null
  else
    "$PARTSCASTER_VENV/bin/python" -c 'import urllib.request; urllib.request.urlopen("http://localhost:8091/health", timeout=5)' \
      >/dev/null 2>&1
  fi
}

start_worker() {
  systemctl --user daemon-reload
  systemctl --user enable partscaster-worker
  systemctl --user restart partscaster-worker
  # модель грузится после старта, но API отвечает сразу; ждём до ~30 с
  for _ in $(seq 1 15); do
    if health_ok; then
      echo "воркер запущен: $(readlink "$PARTSCASTER_HOME/current")"
      return 0
    fi
    sleep 2
  done
  echo "воркер не отвечает на http://localhost:8091/health — journalctl --user -u partscaster-worker -n 50" >&2
  return 1
}

sed_repl() { printf '%s' "$1" | sed -e 's/[|&\\]/\\&/g'; } # путь как замена в s|…|…| буквально

install_unit() {
  mkdir -p "$PARTSCASTER_HOME/units" "$HOME/.config/systemd/user"
  local unit="$PARTSCASTER_HOME/units/partscaster-worker.service"
  # пути юнита — те, куда ставили: каталог воркера (код, worker.env, данные) и venv с пакетами;
  # по умолчанию юнит остаётся в переносимой форме %h/…
  local args=(-e "")
  [[ "$PARTSCASTER_HOME" == "$HOME/partscaster" ]] || args+=(-e "s|%h/partscaster|$(sed_repl "$PARTSCASTER_HOME")|g")
  [[ "$PARTSCASTER_VENV" == "$HOME/yue/.venv" ]] || args+=(-e "s|%h/yue/.venv|$(sed_repl "$PARTSCASTER_VENV")|g")
  sed "${args[@]}" "$PARTSCASTER_HOME/current/units/partscaster-worker.service" > "$unit"
  cp "$unit" "$HOME/.config/systemd/user/"
}

# с окружением worker.env (данные, кеши) — как его читает systemd: КЛЮЧ=значение, не исполняется
with_worker_env() {
  (
    export PARTSCASTER_DATA_DIR="$PARTSCASTER_HOME/data" PARTSCASTER_HOME
    if [[ -f "$PARTSCASTER_HOME/worker.env" ]]; then
      while IFS='=' read -r key val; do
        [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
        if [[ ${#val} -ge 2 && ( "$val" == \"*\" || "$val" == \'*\' ) ]]; then val="${val:1:${#val}-2}"; fi
        export "$key=$val"
      done < "$PARTSCASTER_HOME/worker.env"
    fi
    "$@"
  )
}

# python воркера с его окружением: системный python3 или venv (настройки — до установки пакетов не нужны)
settings_py() {
  local py=python3
  command -v python3 >/dev/null || py="$PARTSCASTER_VENV/bin/python"
  with_worker_env "$py" -c "$@"
}

# выбранная модель и экономный режим из settings.json данных воркера: «<модель> <0|1>» (модели нет — « 0»)
settings_model() {
  settings_py '
import json, os
from pathlib import Path
try:
    st = json.loads((Path(os.environ["PARTSCASTER_DATA_DIR"]) / "settings.json").read_text(encoding="utf-8"))
except (OSError, ValueError):
    st = {}
print(st.get("cpp_model") or "", int(bool(st.get("cpp_low_vram"))))
'
}

# записать модель и экономный режим (остальные ключи сохраняются; выбор движка прежних версий — убирается)
settings_set_model() { # settings_set_model <модель> <0|1>
  settings_py '
import json, os, sys
from pathlib import Path
p = Path(os.environ["PARTSCASTER_DATA_DIR"]) / "settings.json"
try:
    st = json.loads(p.read_text(encoding="utf-8"))
except (OSError, ValueError):
    st = {}
st.pop("engine", None)
st.update(cpp_model=sys.argv[1], cpp_low_vram=sys.argv[2] == "1")
p.parent.mkdir(parents=True, exist_ok=True)
tmp = p.with_name(f".{p.name}.{os.getpid()}.tmp")  # атомарно: работающий воркер не прочтёт полфайла
tmp.write_text(json.dumps(st, ensure_ascii=False, indent=1), encoding="utf-8")
os.replace(tmp, p)
' "$1" "$2"
}

CPP_COMMIT="c893629" # = engine_cpp.CPP_COMMIT
CPP_REPO="https://github.com/ServeurpersoCom/yue2.cpp.git"

cpp_runs() { # программа запускается (--help); библиотеки CUDA — из venv воркера
  local libs
  libs="$(find "$PARTSCASTER_VENV"/lib/python3*/site-packages/nvidia/cuda_runtime/lib "$PARTSCASTER_VENV"/lib/python3*/site-packages/nvidia/cublas/lib \
    -maxdepth 0 -type d 2>/dev/null | paste -sd: -)"
  [[ -x "$1" ]] || return 1
  local out rc=0
  out="$(LD_LIBRARY_PATH="${libs}${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" "$1" --help 2>&1)" || rc=$?
  # yue-server на --help печатает Usage и выходит с кодом 1; без библиотек CUDA — ошибка загрузчика без Usage
  [[ $rc == 0 ]] || grep -qi usage <<<"$out"
}

cpp_build() { # сборка закреплённого коммита в $PARTSCASTER_HOME/yue2cpp-src (нужны git, cmake, nvcc)
  local src="$PARTSCASTER_HOME/yue2cpp-src" nvcc
  nvcc="$(command -v nvcc || true)"
  [[ -z "$nvcc" && -x /usr/local/cuda/bin/nvcc ]] && nvcc=/usr/local/cuda/bin/nvcc
  if ! command -v git >/dev/null || ! command -v cmake >/dev/null || [[ -z "$nvcc" ]]; then
    echo "yue2.cpp не собран: нужны git, cmake, nvcc (пакет разработки CUDA)" >&2
    return 1
  fi
  echo "сборка yue2.cpp $CPP_COMMIT (~10 мин)"
  if [[ ! -d "$src/.git" ]]; then
    rm -rf "$src"
    git clone --quiet --recurse-submodules "$CPP_REPO" "$src" || return 1
  fi
  git -C "$src" fetch --quiet origin || true
  git -C "$src" checkout --quiet "$CPP_COMMIT" && git -C "$src" submodule update --quiet --init --recursive || return 1
  cmake -S "$src" -B "$src/build" -DGGML_CUDA=ON -DCMAKE_BUILD_TYPE=Release -DCMAKE_CUDA_COMPILER="$nvcc" >/dev/null &&
    cmake --build "$src/build" --config Release -j "$(nproc)" --target yue-server >/dev/null || return 1
  echo "$CPP_COMMIT" > "$src/.commit"
}

cpp_binary() { # cpp_binary <каталог версии>: рабочая программа yue2.cpp (из архива или собранная), иначе — сборка
  local rel="$1/yue2cpp/yue-server" built="$PARTSCASTER_HOME/yue2cpp-src/build/yue-server"
  local mark="$PARTSCASTER_HOME/yue2cpp-src/.commit" own
  # своя программа из worker.env (PARTSCASTER_CPP_BIN) — воркер берёт её первой (engine_cpp._candidates)
  own="$(with_worker_env bash -c 'printf %s "${PARTSCASTER_CPP_BIN:-}"')"
  if [[ -n "$own" ]] && cpp_runs "$own"; then echo "$own"; return 0; fi
  if cpp_runs "$rel"; then echo "$rel"; return 0; fi
  # собранная раньше — если пометки нет или коммит тот же (пометка другая — закреплён новый коммит)
  if cpp_runs "$built" && { [[ ! -f "$mark" ]] || [[ "$(cat "$mark")" == "$CPP_COMMIT" ]]; }; then
    echo "$built"; return 0
  fi
  cpp_build >&2 && cpp_runs "$built" && { echo "$built"; return 0; }
  return 1
}

# модель генерации: выбор пользователя (скачан, флагов нет) не трогается; иначе — флаг или рекомендуемая
# (engine_cpp.recommend — единственное место порогов видеопамяти). Программа yue2.cpp и модель обязательны:
# не вышло — установка падает (повторный запуск докачает), прежняя версия остаётся действующей
cpp_setup() { # cpp_setup <каталог новой версии>
  local dir="$1" py="$PARTSCASTER_VENV/bin/python" model="$CPP_MODEL" low="" rec bin current current_low
  local rec_model rec_low
  rec="$(cd "$dir" && with_worker_env "$py" engine_cpp.py recommend)" || return 1
  read -r rec_model rec_low <<<"$rec"
  read -r current current_low <<<"$(settings_model)"
  if [[ -z "$model" && -n "$current" ]] && (cd "$dir" && with_worker_env "$py" engine_cpp.py installed "$current"); then
    echo "модель генерации: $current (выбрана раньше, скачана)"
    bin="$(cpp_binary "$dir")" || { echo "yue2.cpp не установлен: нужна готовая программа из архива или git, cmake, nvcc для сборки" >&2; return 1; }
    echo "программа yue2.cpp: $bin"
    [[ $CPP_LOW_VRAM == 1 ]] && current_low=1
    # перезапись тем же выбором: убирает ключ engine прежних версий (и включает экономный режим по флагу)
    settings_set_model "$current" "$current_low"
    return 0
  fi
  if [[ -z "$model" || "$model" == auto ]]; then
    model="$rec_model"
    low="$rec_low"
    echo "модель генерации по видеопамяти: $model$([[ $low == 1 ]] && echo ', экономный режим') (рекомендуемая)"
  else
    low="$rec_low"
    echo "модель генерации: $model (рекомендуемая для этой карты — $rec_model)"
  fi
  [[ $CPP_LOW_VRAM == 1 ]] && low=1
  bin="$(cpp_binary "$dir")" || { echo "yue2.cpp не установлен: нужна готовая программа из архива или git, cmake, nvcc для сборки" >&2; return 1; }
  echo "программа yue2.cpp: $bin"
  if ! (cd "$dir" && with_worker_env "$py" engine_cpp.py download "$model"); then
    echo "модель $model не скачана — повторный запуск install.sh докачает" >&2
    return 1
  fi
  settings_set_model "$model" "$low"
}

if [[ $ROLLBACK == 1 ]]; then
  cur="$(basename "$(readlink -f "$PARTSCASTER_HOME/current")")"
  prev="$(ls -1 "$PARTSCASTER_HOME/releases" | sort | awk -v c="$cur" '$0 < c' | tail -1 || true)"
  if [[ -z "$prev" ]]; then
    echo "откатываться некуда: до $cur версий нет" >&2
    exit 1
  fi
  switch_current "$PARTSCASTER_HOME/releases/$prev"
  install_unit
  echo "откат: $cur → $prev"
  start_worker
  exit 0
fi

# проверка до любых изменений: без видеокарты воркер не работает
if ! command -v nvidia-smi >/dev/null || ! nvidia-smi >/dev/null; then
  echo "нет NVIDIA GPU или драйвера: воркеру нужна видеокарта с CUDA" >&2
  exit 1
fi

VERSION="$(tr -d '[:space:]' < "$SRC/VERSION")"
mkdir -p "$PARTSCASTER_HOME/releases" "$PARTSCASTER_HOME/data"
# имя — время до секунды: повтор в ту же секунду ждёт следующую, а не вкладывается в чужой каталог;
# временный .<имя> занимается mkdir (атомарно) — две установки сразу не возьмут одно имя
for attempt in 1 2 3 4 5; do
  NAME="$(date +%Y%m%d%H%M%S)-$VERSION"
  if [[ ! -e "$PARTSCASTER_HOME/releases/$NAME" && ! -e "$PARTSCASTER_HOME/releases/.$NAME" ]]; then
    mkdir "$PARTSCASTER_HOME/releases/.$NAME" && break # не занято, а mkdir упал (права, место) — ошибка сразу
  fi
  [[ $attempt == 5 ]] && { echo "не удалось выбрать имя версии в $PARTSCASTER_HOME/releases" >&2; exit 1; }
  sleep 1
done
# пока версия не стала действующей, сбой (копия, venv, пакеты) убирает её каталоги: иначе --rollback
# выбрал бы по имени версию, которая ни разу не работала, а недокопированная копия занимала бы место
trap 'rm -rf "$PARTSCASTER_HOME/releases/.$NAME" "$PARTSCASTER_HOME/releases/$NAME"' EXIT
# копия во временный каталог и rename — недокопированная версия не станет действующей
cp -R "$SRC/." "$PARTSCASTER_HOME/releases/.$NAME"
mv -T "$PARTSCASTER_HOME/releases/.$NAME" "$PARTSCASTER_HOME/releases/$NAME"

# uv: его установщик кладёт программу в ~/.local/bin (старые — в ~/.cargo/bin), а ssh без входа в
# оболочку этих каталогов в PATH не добавляет — установка по ssh (deploy.sh, install_worker) падала
# «uv: команда не найдена»
need_uv() {
  command -v uv >/dev/null 2>&1 && return
  export PATH="$HOME/.local/bin:$HOME/.cargo/bin:$PATH"
  command -v uv >/dev/null 2>&1 && return
  echo "не найден uv (PATH, ~/.local/bin, ~/.cargo/bin) — поставьте: https://docs.astral.sh/uv/" >&2
  exit 1
}

# пакеты: только если venv новый или requirements.txt изменился
FP="$PARTSCASTER_HOME/.requirements.sha256"
MARK="$PARTSCASTER_VENV/.yue-installing" # venv создан этим установщиком, пакеты ещё не встали
# отпечаток — без комментариев и пустых строк: правка текста в файле не переустанавливает пакеты
# (комментарий, как у pip, — # в начале строки или после пробела; #egg= в ссылке — не он)
want="$(sed -E -e 's/(^|[[:space:]]+)#.*//' -e '/^[[:space:]]*$/d' "$SRC/requirements.txt" | sha256sum | cut -d' ' -f1)"
if [[ ! -x "$PARTSCASTER_VENV/bin/python" ]]; then
  need_uv
  uv venv "$PARTSCASTER_VENV" --python 3.12
  touch "$MARK"
elif [[ ! -f "$FP" && ! -f "$MARK" ]]; then
  # venv из установки до архивов: рабочее окружение не трогаем, запоминаем отпечаток
  echo "$want" > "$FP"
fi
if [[ -f "$MARK" || "$(cat "$FP" 2>/dev/null)" != "$want" ]]; then
  echo "== пакеты (torch cu128 + API; первый раз — долго) =="
  need_uv
  uv pip install --python "$PARTSCASTER_VENV/bin/python" -r "$SRC/requirements.txt" \
    --index-url https://download.pytorch.org/whl/cu128 --extra-index-url https://pypi.org/simple
  echo "$want" > "$FP"
  rm -f "$MARK"
fi

# worker.env — настройки машины; создаётся один раз
if [[ ! -f "$PARTSCASTER_HOME/worker.env" ]]; then
  printf 'HF_HOME=%s\n' "$(expand_home "$HF_HOME_DIR")" > "$PARTSCASTER_HOME/worker.env"
fi

# движок генерации yue2.cpp: программа из архива релиза (готовая сборка CUDA 12.8; библиотеки CUDA — из venv
# воркера), не запускается — сборка закреплённого коммита; модель — с проверкой суммы
echo "== модель генерации (yue2.cpp) =="
if ! cpp_setup "$PARTSCASTER_HOME/releases/$NAME"; then
  echo "!! yue2.cpp или модель не установлены — версия $VERSION не включена" >&2
  exit 1
fi

# действующей версия становится, когда пакеты и движок под неё встали: сбой выше оставляет прежнюю
switch_current "$PARTSCASTER_HOME/releases/$NAME"
trap - EXIT
install_unit
echo "версия $VERSION → $PARTSCASTER_HOME/releases/$NAME"

# наборы сэмплов (барабаны, басы, оркестр ~1 ГБ) — заранее; не встали — приложение докачает.
# worker.env читается как данные (КЛЮЧ=значение, как его читает systemd), а не исполняется:
# путь с пробелом или $(…) в значении не ломает шаг и не выполняется
echo "== наборы сэмплов =="
(
  cd "$PARTSCASTER_HOME/current"
  while IFS='=' read -r key val; do
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ ${#val} -ge 2 && ( "$val" == \"*\" || "$val" == \'*\' ) ]]; then val="${val:1:${#val}-2}"; fi # кавычки — как у systemd
    export "$key=$val"
  done < "$PARTSCASTER_HOME/worker.env"
  "$PARTSCASTER_VENV/bin/python" kits_install.py
) || echo "!! не все наборы поставились — следующий запуск install.sh докачает"

# хранятся действующая и KEEP последних прочих версий
cur="$(basename "$(readlink -f "$PARTSCASTER_HOME/current")")"
others=()
for d in "$PARTSCASTER_HOME"/releases/*; do # глоб отсортирован, скрытые .<имя> (недокопированные) не попадают
  [[ "$(basename "$d")" == "$cur" ]] || others+=("$d")
done
for ((i = 0; i < ${#others[@]} - KEEP; i++)); do
  rm -rf "${others[$i]}"
done

if [[ $NO_START == 1 ]]; then
  echo "установлено без запуска (--no-start)"
  exit 0
fi
start_worker
