#!/usr/bin/env bash
# Установка/обновление воркера Yue Studio из распакованного архива (worker-<версия>/install.sh).
#
#   ./install.sh [--no-start] [--hf-home DIR]   установить эту версию и (без --no-start) перезапустить
#   ~/yue-studio/current/install.sh --rollback   вернуть предыдущую версию
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
while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-start) NO_START=1 ;;
    --rollback) ROLLBACK=1 ;;
    --hf-home) HF_HOME_DIR="${2:?--hf-home: нужен каталог}"; shift ;;
    -h|--help) sed -n '2,9p' "$0"; exit 0 ;;
    *) echo "неизвестный параметр: $1" >&2; exit 2 ;;
  esac
  shift
done

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
