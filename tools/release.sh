#!/usr/bin/env bash
# Релизный пакет под ОС: приложение + partscaster-mcp + архив воркера + INSTALL.txt.
# usage: [TARGET_OS=linux|windows|darwin] tools/release.sh [VERSION]   (версия по умолчанию — git describe)
# ОС по умолчанию — эта. С Linux собирается и Windows (Wails там без cgo); macOS — только на Mac.
# Итог в dist/: partscaster-<ver>-<os>-<arch>.tar.gz (Windows — .zip) и worker-<ver>.tar.gz.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty)}"
HOST_OS="$(go env GOHOSTOS)"
GOOS="${TARGET_OS:-$HOST_OS}"
case "$GOOS" in
  linux | windows | darwin) ;;
  *) echo "TARGET_OS=$GOOS: неизвестная ОС (linux, windows или darwin)" >&2; exit 2 ;;
esac
if [[ "$GOOS" != "$HOST_OS" && "$GOOS" != "windows" ]]; then
  case "$GOOS" in
    darwin) echo "пакет macOS собирается только на Mac (make dist-mac) или в CI по тегу" >&2 ;;
    linux) echo "пакет Linux собирается только на Linux (make dist-linux) или в CI по тегу" >&2 ;;
  esac
  exit 2
fi
GOARCH="$(go env GOHOSTARCH)"
[[ "$GOOS" != "$HOST_OS" ]] && GOARCH="amd64"
WAILS="${WAILS:-wails}"
WAILS_ARGS=()
[[ "$GOOS" != "$HOST_OS" ]] && WAILS_ARGS=(-platform "$GOOS/$GOARCH")
EXE=""
[[ "$GOOS" == "windows" ]] && EXE=".exe"

mkdir -p dist
# движок генерации yue2.cpp — готовой сборкой в архиве воркера: на Linux — в контейнере CUDA 12.8 (нужен docker),
# на Windows — локально (нужны VS Build Tools C++, CUDA Toolkit, cmake, ninja). Linux без сборки — архив с
# предупреждением (install.sh соберёт программу сам); пакет Windows без сборки worker-tar.sh не соберёт
if [[ "$HOST_OS" == "linux" && ! -x dist/yue2cpp-linux-x86_64/yue-server ]] && command -v docker >/dev/null; then
  tools/yue2cpp-build.sh || echo "!! сборка yue2.cpp не удалась — архив воркера без неё" >&2
fi
if [[ "$HOST_OS" == "windows" && ! -f dist/yue2cpp-windows-x86_64/yue-server.exe ]] && command -v nvcc >/dev/null; then
  powershell -NoProfile -ExecutionPolicy Bypass -File tools/yue2cpp-build.ps1 \
    || echo "!! сборка yue2.cpp не удалась — пакет Windows без неё не собирается" >&2
fi
export TARGET_OS="$GOOS"   # worker-tar берёт каталог yue2cpp своей ОС
WORKER_TAR="$(tools/worker-tar.sh dist "$VERSION" | tail -1)"

LDFLAGS="-X main.version=$VERSION"
"$WAILS" build -clean ${WAILS_ARGS[@]+"${WAILS_ARGS[@]}"} -ldflags "$LDFLAGS"
# релизный MCP без srcDir: проверка «MCP старше исходников» в релизе не нужна
HASH="$(go run ./cmd/srchash)"
GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags "$LDFLAGS -X main.buildHash=$HASH" -o "build/bin/partscaster-mcp$EXE" ./cmd/partscaster-mcp
if [[ "$GOOS" == "windows" ]]; then
  # partscaster-worker.exe — воркер без WSL: код воркера вшивается в программу (go:embed из payload/); окно установки
  # (Wails — теги desktop,production) и значок в трее, без консоли (-H windowsgui); cgo не нужен.
  # Иконка exe и сведения о файле — ресурсом .syso (go-winres): go build подхватывает его сам; перегенерация —
  # из cmd/partscaster-worker/icon.ico (рисует tools/make-worker-icon.py)
  (cd cmd/partscaster-worker && go tool go-winres simply --icon icon.ico --manifest none \
    --product-name "Partscaster Worker" --file-description "Partscaster — воркер (установка и трей)" \
    --product-version "$VERSION" --arch "$GOARCH")
  PAYLOAD=cmd/partscaster-worker/payload
  mkdir -p "$PAYLOAD"
  rm -f "$PAYLOAD"/worker-*.tar.gz
  cp "$WORKER_TAR" "$PAYLOAD/"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -tags desktop,production -ldflags "$LDFLAGS -H windowsgui" \
    -o build/bin/partscaster-worker.exe ./cmd/partscaster-worker
  rm -f "$PAYLOAD"/worker-*.tar.gz
fi

NAME="partscaster-$VERSION-$GOOS-$GOARCH"
PKG="dist/$NAME"
rm -rf "$PKG"
mkdir -p "$PKG"
if [[ "$GOOS" == "darwin" ]]; then
  cp -R build/bin/partscaster.app "$PKG/"
  # приложение ищет partscaster-mcp рядом со своим исполняемым файлом — он внутри .app
  cp "build/bin/partscaster-mcp" "$WORKER_TAR" "$PKG/partscaster.app/Contents/MacOS/" # и архив: install_worker ищет его рядом с собой
else
  cp "build/bin/partscaster$EXE" "$PKG/"
fi
[[ "$GOOS" == "windows" ]] && cp build/bin/partscaster-worker.exe "$PKG/"
cp "build/bin/partscaster-mcp$EXE" "$PKG/"
cp "$WORKER_TAR" "$PKG/"
sed "s/@VERSION@/$VERSION/g" deploy/INSTALL.txt > "$PKG/INSTALL.txt"

if [[ "$GOOS" == "windows" ]]; then
  rm -f "dist/$NAME.zip"
  if command -v 7z >/dev/null; then
    (cd dist && 7z a -tzip "$NAME.zip" "$NAME" >/dev/null)
  else
    (cd dist && zip -qr "$NAME.zip" "$NAME")
  fi
  echo "dist/$NAME.zip"
else
  tar -C dist -czf "dist/$NAME.tar.gz" "$NAME"
  echo "dist/$NAME.tar.gz"
fi
echo "$WORKER_TAR"
