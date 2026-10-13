#!/usr/bin/env bash
# Архив воркера: OUTDIR/worker-<VERSION>.tar.gz с одним каталогом worker-<VERSION>/.
# usage: tools/worker-tar.sh OUTDIR [VERSION]   (по умолчанию — git describe)
# Состав — правилом, а не списком файлов: весь код воркера без тестов, данные (*.json),
# requirements, скрипты установки, фразы, юнит systemd и (если собрана) программа yue2.cpp. Путь архива — последней строкой.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT="${1:?usage: tools/worker-tar.sh OUTDIR [VERSION]}"
VERSION="${2:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
NAME="worker-$VERSION"

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
D="$STAGE/$NAME"
mkdir -p "$D/units" "$D/phrases"
for f in worker/*.py; do
  [[ "$(basename "$f")" == test_* ]] || cp "$f" "$D/"
done
cp worker/*.json worker/requirements*.txt worker/seedvc_install.sh worker/install.sh "$D/"
cp worker/phrases/* "$D/phrases/"
cp deploy/units/yue-worker.service "$D/units/"
# готовая сборка yue2.cpp (tools/yue2cpp-build.sh) — второй движок генерации, ЭКСПЕРИМЕНТ; нет её — install.sh
# соберёт сам при --engine cpp
if [[ -x dist/yue2cpp-linux-x86_64/yue-server ]]; then
  mkdir -p "$D/yue2cpp"
  cp -P dist/yue2cpp-linux-x86_64/* "$D/yue2cpp/"
fi
chmod +x "$D/install.sh" "$D/seedvc_install.sh"
printf '%s\n' "$VERSION" > "$D/VERSION"

mkdir -p "$OUT"
ARCHIVE="$(cd "$OUT" && pwd)/$NAME.tar.gz"
tar -C "$STAGE" -czf "$ARCHIVE" "$NAME"
echo "$ARCHIVE"
