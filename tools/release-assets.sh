#!/usr/bin/env bash
# Постоянные имена пакетов для GitHub Release: DIR/partscaster-<версия>-<ос>-<арх>.(zip|tar.gz) →
# DIR/release/partscaster-<ос>-<арх>.(zip|tar.gz), чтобы ссылка releases/latest/download/<имя> не менялась от
# версии к версии. Каталог с версией внутри архива остаётся; архив воркера worker-<версия>.tar.gz не трогается.
# usage: tools/release-assets.sh DIR   (печатает пути созданных файлов)
set -euo pipefail
dir="${1:?usage: tools/release-assets.sh DIR}"
mkdir -p "$dir/release"
shopt -s nullglob
for f in "$dir"/partscaster-*.zip "$dir"/partscaster-*.tar.gz; do
  base="$(basename "$f")"
  case "$base" in *.zip) ext=zip ;; *) ext=tar.gz ;; esac
  stem="${base%."$ext"}"
  # ОС и архитектура — два последних поля через дефис; версия (бывает с дефисами: 1.0.0-rc1) — всё между
  arch="${stem##*-}"
  rest="${stem%-*}"
  os="${rest##*-}"
  out="$dir/release/partscaster-$os-$arch.$ext"
  cp "$f" "$out"
  echo "$out"
done
