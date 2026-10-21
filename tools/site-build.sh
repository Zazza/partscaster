#!/usr/bin/env bash
# Сайт для GitHub Pages: site/index.html + логотип + примеры «было/стало» окна знакомства
# (frontend/src/welcomeClips.json → clips.json, клипы из internal/welcome/clips) в OUT (по умолчанию _site).
# Клипы в site/ не копируются в репозитории — берутся из окна знакомства при сборке, один источник.
# usage: tools/site-build.sh [OUT]   (локально посмотреть: python3 -m http.server -d _site)
set -euo pipefail
cd "$(dirname "$0")/.."
out="${1:-_site}"
mkdir -p "$out/clips"
cp site/index.html "$out/"
cp frontend/src/assets/logo.png "$out/logo.png"
cp frontend/src/welcomeClips.json "$out/clips.json"
cp internal/welcome/clips/*.mp3 "$out/clips/"
echo "$out"
