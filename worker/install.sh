#!/usr/bin/env bash
# Установка/обновление воркера Yue Studio из распакованного архива (worker-<версия>/install.sh).
#
#   ./install.sh [--no-start] [--hf-home DIR]   установить эту версию и (без --no-start) перезапустить
#   ~/yue-studio/current/install.sh --rollback   вернуть предыдущую версию
#   движок генерации: --engine torch (основной) | cpp (yue2.cpp — ЭКСПЕРИМЕНТ, меньше видеопамяти);
#     для cpp: --cpp-model auto|bf16|q8|q6|q5 (auto — по видеопамяти), --cpp-low-vram (экономный, ~4,5 ГБ)
#   без --engine движок не меняется (новая установка — основной)
#
# Раскладка: код — $YUE_HOME/releases/<время>-<версия>/, ссылка $YUE_HOME/current — действующая версия;
# данные ($YUE_HOME/data) и настройки машины ($YUE_HOME/worker.env) общие для всех версий и не трогаются.
# Необязательные компоненты (whisper, Seed-VC, RoFormer) ставит MCP-инструмент install_worker.
set -euo pipefail

# pwd -P: запуск через ссылку current/install.sh копирует саму версию, а не ссылку на неё
SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
YUE_HOME="${YUE_HOME:-$HOME/yue-studio}"
YUE_VENV="${YUE_VENV:-$HOME/yue/.venv}"
KEEP=3 # сколько прежних версий хранить кроме действующей

NO_START=0
ROLLBACK=0
HF_HOME_DIR="~/yue/hf-cache"
ENGINE=""      # пусто — движок не трогаем
CPP_MODEL=""   # пусто при --engine cpp — auto
CPP_LOW_VRAM=0
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-start) NO_START=1 ;;
    --rollback) ROLLBACK=1 ;;
    --hf-home) HF_HOME_DIR="${2:?--hf-home: нужен каталог}"; shift ;;
    --engine) ENGINE="${2:?--engine: torch или cpp}"; shift ;;
    --cpp-model) CPP_MODEL="${2:?--cpp-model: auto, bf16, q8, q6 или q5}"; shift ;;
    --cpp-low-vram) CPP_LOW_VRAM=1 ;;
    -h|--help) sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "неизвестный параметр: $1" >&2; exit 2 ;;
  esac
  shift
done
case "$ENGINE" in
  "" | torch | cpp) ;;
  *) echo "--engine: torch или cpp (получено «$ENGINE»)" >&2; exit 2 ;;
esac
case "$CPP_MODEL" in
  "" | auto | bf16 | q8 | q6 | q5) ;;
  *) echo "--cpp-model: auto, bf16, q8, q6 или q5 (получено «$CPP_MODEL»)" >&2; exit 2 ;;
esac
if [[ "$ENGINE" != cpp && ( -n "$CPP_MODEL" || $CPP_LOW_VRAM == 1 ) ]]; then
  echo "--cpp-model и --cpp-low-vram — только вместе с --engine cpp" >&2
  exit 2
fi

# пути уходят в юнит systemd: кавычки, обратный слэш, % (спецификаторы systemd) и перевод строки там
# не записать буквально — такие пути отклоняются до любых изменений (пробелы — можно, юнит их кавычит)
for p in "$YUE_HOME" "$YUE_VENV"; do
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
  ln -sfn "$1" "$YUE_HOME/current.new"
  mv -T "$YUE_HOME/current.new" "$YUE_HOME/current"
}

# health_ok — воркер отвечает на /health; curl есть не везде (на GPU-машине его может не быть) —
# тогда python из venv воркера, он там есть всегда
health_ok() {
  if command -v curl >/dev/null; then
    curl -sf --max-time 5 http://localhost:8091/health >/dev/null
  else
    "$YUE_VENV/bin/python" -c 'import urllib.request; urllib.request.urlopen("http://localhost:8091/health", timeout=5)' \
      >/dev/null 2>&1
  fi
}

start_worker() {
  systemctl --user daemon-reload
  systemctl --user enable yue-worker
  systemctl --user restart yue-worker
  # модель грузится после старта, но API отвечает сразу; ждём до ~30 с
  for _ in $(seq 1 15); do
    if health_ok; then
      echo "воркер запущен: $(readlink "$YUE_HOME/current")"
      return 0
    fi
    sleep 2
  done
  echo "воркер не отвечает на http://localhost:8091/health — journalctl --user -u yue-worker -n 50" >&2
  return 1
}

sed_repl() { printf '%s' "$1" | sed -e 's/[|&\\]/\\&/g'; } # путь как замена в s|…|…| буквально

install_unit() {
  mkdir -p "$YUE_HOME/units" "$HOME/.config/systemd/user"
  local unit="$YUE_HOME/units/yue-worker.service"
  # пути юнита — те, куда ставили: каталог воркера (код, worker.env, данные) и venv с пакетами;
  # по умолчанию юнит остаётся в переносимой форме %h/…
  local args=(-e "")
  [[ "$YUE_HOME" == "$HOME/yue-studio" ]] || args+=(-e "s|%h/yue-studio|$(sed_repl "$YUE_HOME")|g")
  [[ "$YUE_VENV" == "$HOME/yue/.venv" ]] || args+=(-e "s|%h/yue/.venv|$(sed_repl "$YUE_VENV")|g")
  sed "${args[@]}" "$YUE_HOME/current/units/yue-worker.service" > "$unit"
  cp "$unit" "$HOME/.config/systemd/user/"
}

# с окружением worker.env (данные, кеши) — как его читает systemd: КЛЮЧ=значение, не исполняется
with_worker_env() {
  (
    export YUE_DATA_DIR="$YUE_HOME/data" YUE_HOME
    if [[ -f "$YUE_HOME/worker.env" ]]; then
      while IFS='=' read -r key val; do
        [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
        if [[ ${#val} -ge 2 && ( "$val" == \"*\" || "$val" == \'*\' ) ]]; then val="${val:1:${#val}-2}"; fi
        export "$key=$val"
      done < "$YUE_HOME/worker.env"
    fi
    "$@"
  )
}

# settings.json данных воркера: дописать ключи engine/cpp_* (остальные сохраняются)
settings_set() { # settings_set '{"engine": "cpp", ...}'
  local py=python3
  command -v python3 >/dev/null || py="$YUE_VENV/bin/python"
  with_worker_env "$py" -c '
import json, os, sys
from pathlib import Path
p = Path(os.environ["YUE_DATA_DIR"]) / "settings.json"
try:
    st = json.loads(p.read_text(encoding="utf-8"))
except (OSError, ValueError):
    st = {}
st.update(json.loads(sys.argv[1]))
p.parent.mkdir(parents=True, exist_ok=True)
tmp = p.with_name(f".{p.name}.{os.getpid()}.tmp")  # атомарно: работающий воркер не прочтёт полфайла
tmp.write_text(json.dumps(st, ensure_ascii=False, indent=1), encoding="utf-8")
os.replace(tmp, p)
' "$1"
}

CPP_COMMIT="c893629" # = engine_cpp.CPP_COMMIT
CPP_REPO="https://github.com/ServeurpersoCom/yue2.cpp.git"

cpp_runs() { # программа запускается (--help); библиотеки CUDA — из venv воркера
  local libs
  libs="$(find "$YUE_VENV"/lib/python3*/site-packages/nvidia/cuda_runtime/lib "$YUE_VENV"/lib/python3*/site-packages/nvidia/cublas/lib \
    -maxdepth 0 -type d 2>/dev/null | paste -sd: -)"
  [[ -x "$1" ]] || return 1
  local out rc=0
  out="$(LD_LIBRARY_PATH="${libs}${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}" "$1" --help 2>&1)" || rc=$?
  # yue-server на --help печатает Usage и выходит с кодом 1; без библиотек CUDA — ошибка загрузчика без Usage
  [[ $rc == 0 ]] || grep -qi usage <<<"$out"
}

cpp_build() { # сборка закреплённого коммита в $YUE_HOME/yue2cpp-src (нужны git, cmake, nvcc)
  local src="$YUE_HOME/yue2cpp-src" nvcc
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

cpp_binary() { # путь к рабочей программе yue2.cpp (из архива или собранной), иначе — сборка
  local rel="$YUE_HOME/current/yue2cpp/yue-server" built="$YUE_HOME/yue2cpp-src/build/yue-server"
  local mark="$YUE_HOME/yue2cpp-src/.commit"
  if cpp_runs "$rel"; then echo "$rel"; return 0; fi
  # собранная раньше — если пометки нет или коммит тот же (пометка другая — закреплён новый коммит)
  if cpp_runs "$built" && { [[ ! -f "$mark" ]] || [[ "$(cat "$mark")" == "$CPP_COMMIT" ]]; }; then
    echo "$built"; return 0
  fi
  cpp_build >&2 && cpp_runs "$built" && { echo "$built"; return 0; }
  return 1
}

cpp_auto_model() { # по видеопамяти первой карты (воркер работает на cuda:0): 12 ГБ сообщают ~12282 МиБ
  local mib
  mib="$(nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits 2>/dev/null | head -1 | tr -dc '0-9')"
  mib="${mib:-0}"
  if ((mib >= 11776)); then
    echo "q8 0"
  elif ((mib >= 7680)); then
    echo "q8 1"
  else
    echo "q5 1"
  fi
}

engine_setup() {
  if [[ "$ENGINE" == torch ]]; then
    settings_set '{"engine": "torch"}'
    echo "движок: основной (PyTorch)"
    return 0
  fi
  local model="$CPP_MODEL" low="$CPP_LOW_VRAM" bin
  if [[ -z "$model" || "$model" == auto ]]; then
    read -r model low < <(cpp_auto_model)
    [[ $CPP_LOW_VRAM == 1 ]] && low=1
    echo "модель yue2.cpp по видеопамяти: $model$([[ $low == 1 ]] && echo ', экономный режим')"
  fi
  if ! bin="$(cpp_binary)"; then
    echo "yue2.cpp не собран: нужны git, cmake, nvcc (пакет разработки CUDA)"
    return 1
  fi
  echo "программа yue2.cpp: $bin"
  if ! (cd "$YUE_HOME/current" && with_worker_env "$YUE_VENV/bin/python" engine_cpp.py download "$model"); then
    echo "модель yue2.cpp не скачана: $model (повтор install.sh докачает)"
    return 1
  fi
  settings_set "{\"engine\": \"cpp\", \"cpp_model\": \"$model\", \"cpp_low_vram\": $([[ $low == 1 ]] && echo true || echo false)}"
  echo "движок: yue2.cpp (эксперимент), модель $model"
}

if [[ $ROLLBACK == 1 ]]; then
  cur="$(basename "$(readlink -f "$YUE_HOME/current")")"
  prev="$(ls -1 "$YUE_HOME/releases" | sort | awk -v c="$cur" '$0 < c' | tail -1 || true)"
  if [[ -z "$prev" ]]; then
    echo "откатываться некуда: до $cur версий нет" >&2
    exit 1
  fi
  switch_current "$YUE_HOME/releases/$prev"
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
mkdir -p "$YUE_HOME/releases" "$YUE_HOME/data"
# имя — время до секунды: повтор в ту же секунду ждёт следующую, а не вкладывается в чужой каталог;
# временный .<имя> занимается mkdir (атомарно) — две установки сразу не возьмут одно имя
for attempt in 1 2 3 4 5; do
  NAME="$(date +%Y%m%d%H%M%S)-$VERSION"
  if [[ ! -e "$YUE_HOME/releases/$NAME" && ! -e "$YUE_HOME/releases/.$NAME" ]]; then
    mkdir "$YUE_HOME/releases/.$NAME" && break # не занято, а mkdir упал (права, место) — ошибка сразу
  fi
  [[ $attempt == 5 ]] && { echo "не удалось выбрать имя версии в $YUE_HOME/releases" >&2; exit 1; }
  sleep 1
done
# пока версия не стала действующей, сбой (копия, venv, пакеты) убирает её каталоги: иначе --rollback
# выбрал бы по имени версию, которая ни разу не работала, а недокопированная копия занимала бы место
trap 'rm -rf "$YUE_HOME/releases/.$NAME" "$YUE_HOME/releases/$NAME"' EXIT
# копия во временный каталог и rename — недокопированная версия не станет действующей
cp -R "$SRC/." "$YUE_HOME/releases/.$NAME"
mv -T "$YUE_HOME/releases/.$NAME" "$YUE_HOME/releases/$NAME"

# пакеты: только если venv новый или requirements.txt изменился
FP="$YUE_HOME/.requirements.sha256"
MARK="$YUE_VENV/.yue-installing" # venv создан этим установщиком, пакеты ещё не встали
want="$(sha256sum "$SRC/requirements.txt" | cut -d' ' -f1)"
if [[ ! -x "$YUE_VENV/bin/python" ]]; then
  uv venv "$YUE_VENV" --python 3.12
  touch "$MARK"
elif [[ ! -f "$FP" && ! -f "$MARK" ]]; then
  # venv из установки до архивов: рабочее окружение не трогаем, запоминаем отпечаток
  echo "$want" > "$FP"
fi
if [[ -f "$MARK" || "$(cat "$FP" 2>/dev/null)" != "$want" ]]; then
  echo "== пакеты (torch cu128 + API; первый раз — долго) =="
  uv pip install --python "$YUE_VENV/bin/python" -r "$SRC/requirements.txt" \
    --index-url https://download.pytorch.org/whl/cu128 --extra-index-url https://pypi.org/simple
  echo "$want" > "$FP"
  rm -f "$MARK"
fi

# worker.env — настройки машины; создаётся один раз
if [[ ! -f "$YUE_HOME/worker.env" ]]; then
  printf 'HF_HOME=%s\n' "$(expand_home "$HF_HOME_DIR")" > "$YUE_HOME/worker.env"
fi

# действующей версия становится, когда пакеты под неё встали: сбой pip выше оставляет прежнюю
switch_current "$YUE_HOME/releases/$NAME"
trap - EXIT
install_unit
echo "версия $VERSION → $YUE_HOME/releases/$NAME"

# наборы сэмплов (барабаны, басы, оркестр ~1 ГБ) — заранее; не встали — приложение докачает.
# worker.env читается как данные (КЛЮЧ=значение, как его читает systemd), а не исполняется:
# путь с пробелом или $(…) в значении не ломает шаг и не выполняется
echo "== наборы сэмплов =="
(
  cd "$YUE_HOME/current"
  while IFS='=' read -r key val; do
    [[ "$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$ ]] || continue
    if [[ ${#val} -ge 2 && ( "$val" == \"*\" || "$val" == \'*\' ) ]]; then val="${val:1:${#val}-2}"; fi # кавычки — как у systemd
    export "$key=$val"
  done < "$YUE_HOME/worker.env"
  "$YUE_VENV/bin/python" kits_install.py
) || echo "!! не все наборы поставились — следующий запуск install.sh докачает"

# движок генерации: без --engine не трогаем. yue2.cpp — ЭКСПЕРИМЕНТ: программа из архива релиза
# (готовая сборка CUDA 12.8; библиотеки CUDA — из venv воркера), не запускается — сборка закреплённого коммита;
# модель — с проверкой суммы. Не вышло — воркер всё равно установлен, движок — основной (PyTorch).
if [[ -n "$ENGINE" ]]; then
  echo "== движок генерации: $ENGINE =="
  if ! engine_setup; then
    # прежний выбор yue2.cpp без рабочей программы/модели этой версии ломал бы генерации — основной движок
    settings_set '{"engine": "torch"}' || true
    echo "!! движок yue2.cpp не включён — воркер работает на основном (PyTorch); повтор: install.sh --engine cpp"
  fi
fi

# хранятся действующая и KEEP последних прочих версий
cur="$(basename "$(readlink -f "$YUE_HOME/current")")"
others=()
for d in "$YUE_HOME"/releases/*; do # глоб отсортирован, скрытые .<имя> (недокопированные) не попадают
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
