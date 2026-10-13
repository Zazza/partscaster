Сюда сборка (`tools/release.sh`, пакет Windows) кладёт архив воркера `worker-<версия>.tar.gz` перед
`go build ./cmd/yue-worker` — он вшивается в программу. В репозиторий архив не попадает.
