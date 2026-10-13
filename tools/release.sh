#!/usr/bin/env bash
# Релизный пакет под ОС: приложение + yue-mcp + архив воркера + INSTALL.txt.
# usage: [TARGET_OS=linux|windows|darwin] tools/release.sh [VERSION]   (версия по умолчанию — git describe)
# ОС по умолчанию — эта. С Linux собирается и Windows (Wails там без cgo); macOS — только на Mac.
# Итог в dist/: yue-studio-<ver>-<os>-<arch>.tar.gz (Windows — .zip) и worker-<ver>.tar.gz.
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
# второй движок yue2.cpp (эксперимент) — готовой сборкой в архиве воркера: собирается в контейнере на Linux с docker;
# без неё install.sh --engine cpp соберёт программу сам
if [[ "$HOST_OS" == "linux" && ! -x dist/yue2cpp-linux-x86_64/yue-server ]] && command -v docker >/dev/null; then
  tools/yue2cpp-build.sh || echo "!! сборка yue2.cpp не удалась — архив воркера без неё" >&2
fi
WORKER_TAR="$(tools/worker-tar.sh dist "$VERSION" | tail -1)"

LDFLAGS="-X main.version=$VERSION"
"$WAILS" build -clean ${WAILS_ARGS[@]+"${WAILS_ARGS[@]}"} -ldflags "$LDFLAGS"
# релизный MCP без srcDir: проверка «MCP старше исходников» в релизе не нужна
HASH="$(go run ./cmd/srchash)"
GOOS="$GOOS" GOARCH="$GOARCH" go build -ldflags "$LDFLAGS -X main.buildHash=$HASH" -o "build/bin/yue-mcp$EXE" ./cmd/yue-mcp

NAME="yue-studio-$VERSION-$GOOS-$GOARCH"
PKG="dist/$NAME"
rm -rf "$PKG"
mkdir -p "$PKG"
if [[ "$GOOS" == "darwin" ]]; then
  cp -R build/bin/yue-studio.app "$PKG/"
  # приложение ищет yue-mcp рядом со своим исполняемым файлом — он внутри .app
  cp "build/bin/yue-mcp" "$WORKER_TAR" "$PKG/yue-studio.app/Contents/MacOS/" # и архив: install_worker ищет его рядом с собой
else
  cp "build/bin/yue-studio$EXE" "$PKG/"
fi
cp "build/bin/yue-mcp$EXE" "$PKG/"
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
