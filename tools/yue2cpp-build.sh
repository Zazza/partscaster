#!/usr/bin/env bash
# Готовая сборка yue2.cpp (второй движок генерации, ЭКСПЕРИМЕНТ) для архива воркера: dist/yue2cpp-linux-x86_64/
# (yue-server + libggml*, поиск библиотек — свой каталог). Собирается в контейнере CUDA 12.8 на ubuntu 22.04
# (glibc 2.35 — идёт и на системах новее): библиотеки CUDA 12 (cudart, cublas) у пользователя уже есть в venv
# воркера (пакеты nvidia-* от PyTorch cu128), пакет разработки CUDA ему не нужен. Нужен docker.
# usage: tools/yue2cpp-build.sh   (архитектуры — YUE2CPP_ARCHS, по умолчанию все карты от Turing до Blackwell)
set -euo pipefail
cd "$(dirname "$0")/.."

COMMIT="c893629" # = worker/engine_cpp.py CPP_COMMIT
REPO="https://github.com/ServeurpersoCom/yue2.cpp.git"
ARCHS="${YUE2CPP_ARCHS:-75;80;86;89;90;120}"
IMAGE="nvidia/cuda:12.8.1-devel-ubuntu22.04"
SRC="dist/yue2cpp-src"
OUT="dist/yue2cpp-linux-x86_64"

mkdir -p dist
if [[ ! -d "$SRC/.git" ]]; then
  git clone --quiet --recurse-submodules "$REPO" "$SRC"
fi
git -C "$SRC" fetch --quiet origin || true
git -C "$SRC" checkout --quiet "$COMMIT"
git -C "$SRC" submodule update --quiet --init --recursive

# в контейнере нет драйвера: линковка с заглушкой libcuda (настоящую даёт драйвер у пользователя);
# NCCL (несколько видеокарт) выключен — у пользователя его может не быть
docker run --rm -e ARCHS="$ARCHS" -e OWNER="$(id -u):$(id -g)" -v "$PWD/$SRC:/src" "$IMAGE" bash -euc '
  STUBS=/tmp/cuda-stubs
  mkdir -p "$STUBS"
  ln -sf /usr/local/cuda/lib64/stubs/libcuda.so "$STUBS/libcuda.so"
  ln -sf /usr/local/cuda/lib64/stubs/libcuda.so "$STUBS/libcuda.so.1"
  apt-get update -qq >/dev/null && apt-get install -y -qq cmake >/dev/null
  cmake -S /src -B /src/build-rel -DGGML_CUDA=ON -DGGML_CUDA_NCCL=OFF -DGGML_NATIVE=OFF \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_CUDA_ARCHITECTURES="$ARCHS" \
    -DCMAKE_EXE_LINKER_FLAGS="-L$STUBS -Wl,-rpath-link,$STUBS" \
    -DCMAKE_SHARED_LINKER_FLAGS="-L$STUBS -Wl,-rpath-link,$STUBS" \
    -DCMAKE_BUILD_RPATH_USE_ORIGIN=ON -DCMAKE_BUILD_RPATH="\$ORIGIN" >/dev/null
  cmake --build /src/build-rel --config Release -j "$(nproc)" --target yue-server >/dev/null
  chown -R "$OWNER" /src   # сборка от root в контейнере — каталог снова свой
'

rm -rf "$OUT"
mkdir -p "$OUT"
cp -P "$SRC/build-rel/yue-server" "$SRC"/build-rel/libggml*.so* "$OUT/"
echo "$COMMIT" > "$OUT/COMMIT"
echo "$OUT"
