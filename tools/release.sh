#!/usr/bin/env bash
# Релизный пакет под текущую ОС: приложение + yue-mcp + архив воркера + INSTALL.txt.
# usage: tools/release.sh [VERSION]   (по умолчанию — git describe)
# Итог в dist/: yue-studio-<ver>-<os>-<arch>.tar.gz (на Windows — .zip) и worker-<ver>.tar.gz.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:-$(git describe --tags --always --dirty)}"
GOOS="$(go env GOOS)"
GOARCH="$(go env GOARCH)"
WAILS="${WAILS:-wails}"
EXE=""
[[ "$GOOS" == "windows" ]] && EXE=".exe"

mkdir -p dist
WORKER_TAR="$(tools/worker-tar.sh dist "$VERSION" | tail -1)"

LDFLAGS="-X main.version=$VERSION"
"$WAILS" build -clean -ldflags "$LDFLAGS"
# релизный MCP без srcDir: проверка «MCP старше исходников» в релизе не нужна
go build -ldflags "$LDFLAGS -X main.buildHash=$(go run ./cmd/srchash)" -o "build/bin/yue-mcp$EXE" ./cmd/yue-mcp

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
  (cd dist && rm -f "$NAME.zip" && 7z a -tzip "$NAME.zip" "$NAME" >/dev/null)
  echo "dist/$NAME.zip"
else
  tar -C dist -czf "dist/$NAME.tar.gz" "$NAME"
  echo "dist/$NAME.tar.gz"
fi
echo "$WORKER_TAR"
