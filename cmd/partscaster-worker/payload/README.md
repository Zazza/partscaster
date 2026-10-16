# Worker payload for partscaster-worker.exe

`cmd/partscaster-worker` embeds this directory with `//go:embed payload`. When building the Windows package,
`tools/release.sh` copies the worker archive `worker-<version>.tar.gz` here, runs
`go build ./cmd/partscaster-worker`, and removes the archive again. The archive becomes part of
`partscaster-worker.exe`: on every start the program unpacks this worker code into `<home>\app\` and removes
older code versions there (data, packages and models live elsewhere and are kept).

The archive is never committed (`.gitignore`: `cmd/partscaster-worker/payload/*.tar.gz`). This README is what
keeps the directory in the repository, so `go build`, `go vet` and `go test` work without an archive. A binary built
that way starts but stops with "built without worker code"; build it with `make dist-windows` instead.
