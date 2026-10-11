#!/usr/bin/env bash
# Архив воркера: OUTDIR/worker-<VERSION>.tar.gz с одним каталогом worker-<VERSION>/.
# usage: tools/worker-tar.sh OUTDIR [VERSION]   (по умолчанию — git describe)
# Состав — правилом, а не списком файлов: весь код воркера без тестов, данные (*.json),
# requirements, скрипты установки, фразы и юнит systemd. Путь архива — последней строкой.
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
chmod +x "$D/install.sh" "$D/seedvc_install.sh"
printf '%s\n' "$VERSION" > "$D/VERSION"

mkdir -p "$OUT"
ARCHIVE="$(cd "$OUT" && pwd)/$NAME.tar.gz"
tar -C "$STAGE" -czf "$ARCHIVE" "$NAME"
echo "$ARCHIVE"
