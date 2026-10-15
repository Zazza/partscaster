#!/usr/bin/env bash
# Установка распознавания нот гитары (блок движка «Гитара по нотам») — необязательная часть воркера.
# basic-pitch (Spotify, Apache-2.0) и YourMT3+ (GPL-3.0, вызывается отдельной программой). Всё — код, venv, веса и
# кэши — в одном каталоге: его можно перенести на другой диск целиком.
#
# usage: notes_install.sh [каталог] [--basic-pitch-only]
#   каталог по умолчанию — $PARTSCASTER_NOTES_DIR или ~/partscaster/notes;
#   не по умолчанию — пропиши PARTSCASTER_NOTES_DIR=<каталог> в ~/partscaster/worker.env
#   --basic-pitch-only — без YourMT3+ (без видеокарты; ноты беднее, на перегрузе хуже)
# Нужно: git, uv (https://docs.astral.sh/uv/), ~8 ГБ на время установки, ~5 ГБ после.
set -euo pipefail

ONLY_BP=0
ARGS=()
for a in "$@"; do
  if [ "$a" = "--basic-pitch-only" ]; then ONLY_BP=1; else ARGS+=("$a"); fi
done
DIR="${ARGS[0]:-${PARTSCASTER_NOTES_DIR:-$HOME/partscaster/notes}}"
YMT3_REPO=https://huggingface.co/spaces/mimbres/YourMT3
YMT3_REV=5e66c1ea173a8186e0d20432b841d3180cc015b5   # проверенная ревизия кода и весов
YMT3_CKPT=amt/logs/2024/mc13_256_g4_all_v7_mt3f_sqr_rms_moe_wf4_n8k2_silu_rope_rp_b36_nops/checkpoints/last.ckpt
TORCH_INDEX="${PARTSCASTER_TORCH_INDEX:-https://download.pytorch.org/whl/cu128}"
NEED_GB=8    # на пике: venv ~5 (из них CUDA ~4) + кэш пакетов ~2 + веса 0,6
HERE="$(cd "$(dirname "$0")" && pwd)"

mkdir -p "$DIR"
DIR="$(cd "$DIR" && pwd)"
free=$(df -Pk "$DIR" | awk 'NR==2 {print int($4 / 1048576)}')
if (( free < NEED_GB )); then
  echo "мало места в $DIR: свободно ${free} ГБ, нужно ~${NEED_GB} ГБ — укажи другой каталог: $0 /путь" >&2
  exit 1
fi
command -v git >/dev/null || { echo "нужен git" >&2; exit 1; }
command -v uv >/dev/null || { echo "нужен uv: https://docs.astral.sh/uv/" >&2; exit 1; }
export UV_CACHE_DIR="$DIR/uv-cache" HF_HOME="$DIR/hf"

uv venv -q --allow-existing --python 3.12 "$DIR/venv"
PY="$DIR/venv/bin/python"
uv pip install -q --python "$PY" torch==2.10.0 torchaudio==2.10.0 --index-url "$TORCH_INDEX"
uv pip install -q --python "$PY" -r "$HERE/requirements-notes.txt"
uv pip install -q --python "$PY" --no-deps basic-pitch==0.4.0

if (( ONLY_BP == 0 )); then
  if [ ! -d "$DIR/ymt3/.git" ]; then
    GIT_LFS_SKIP_SMUDGE=1 git clone -q "$YMT3_REPO" "$DIR/ymt3"
  fi
  git -C "$DIR/ymt3" fetch -q origin "$YMT3_REV" 2>/dev/null || true
  GIT_LFS_SKIP_SMUDGE=1 git -C "$DIR/ymt3" checkout -q "$YMT3_REV"
  ck="$DIR/ymt3/$YMT3_CKPT"
  # чекпоинт в репозитории — указатель LFS (~130 байт): настоящий файл (560 МБ) — по ссылке ревизии
  if [ ! -s "$ck" ] || [ "$(stat -c %s "$ck")" -lt 1000000 ]; then
    echo "веса YourMT3+ (560 МБ)…"
    "$PY" -c "import sys,urllib.request; urllib.request.urlretrieve(sys.argv[1], sys.argv[2])" \
      "$YMT3_REPO/resolve/$YMT3_REV/$YMT3_CKPT" "$ck.part"
    mv "$ck.part" "$ck"
  fi
fi

echo "пробный запуск…"
"$PY" "$HERE/notes_run.py" --dir "$DIR" --warmup
uv cache clean -q 2>/dev/null || true
du -sh "$DIR"
echo "распознавание нот готово: $DIR"
if [ "$DIR" != "$HOME/partscaster/notes" ]; then
  echo "пропиши в ~/partscaster/worker.env: PARTSCASTER_NOTES_DIR=$DIR  (и перезапусти воркер)"
fi
