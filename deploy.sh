#!/usr/bin/env bash
# Partscaster: сборка desktop-приложения (ПК) и деплой воркера на GPU-машину.
# usage: ./deploy.sh [worker|build]; хост воркера — PARTSCASTER_DEPLOY_HOST (user@host)
set -euo pipefail
cd "$(dirname "$0")"

CMD="${1:-build}"

if [[ "$CMD" == "worker" ]]; then
  HOST="${PARTSCASTER_DEPLOY_HOST:?задайте PARTSCASTER_DEPLOY_HOST=user@gpu-host для деплоя воркера}"
  # воркер едет одним архивом с версией; install.sh на хосте раскладывает его рядом с прежними
  # версиями, переключает ~/partscaster/current, ставит пакеты (если requirements менялись) и перезапускает
  ARCHIVE="$(./tools/worker-tar.sh dist | tail -1)"
  NAME="$(basename "$ARCHIVE" .tar.gz)"
  echo "== deploy $NAME to $HOST =="
  ssh "$HOST" 'mkdir -p ~/partscaster/incoming'
  scp -q "$ARCHIVE" "$HOST:partscaster/incoming/$NAME.tar.gz"
  ssh "$HOST" "set -e
export XDG_RUNTIME_DIR=/run/user/\$(id -u)
cd ~/partscaster/incoming
rm -rf '$NAME'
tar xzf '$NAME.tar.gz'
bash '$NAME/install.sh'
rm -rf '$NAME' '$NAME.tar.gz'"
  echo "worker OK: http://$(echo "$HOST" | cut -d@ -f2):8091/version"
  exit 0
fi

echo "== build desktop app =="
make build
