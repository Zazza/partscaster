# Contributing to Partscaster
**English** · [Русский](CONTRIBUTING.ru.md)

How to set up a development environment, run the checks, send changes and cut a release.
Bugs and ideas go to [GitHub Issues](https://github.com/Zazza/partscaster/issues).

## Environment

| Tool | Version | Needed for |
|---|---|---|
| Go | 1.24 (see `go.mod`) | desktop app, MCP server, Go tests |
| Node.js | 20 | frontend (Vue 3), vitest, eslint |
| [Wails CLI](https://wails.io) | v2.12.0, same as `go.mod`: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0` | `make build`, release packages |
| GTK 3 / WebKitGTK 4.1 dev packages (Linux) | `libgtk-3-dev libwebkit2gtk-4.1-dev` | building the Go part on Linux |
| Python | 3.12 | worker and its tests |
| [uv](https://docs.astral.sh/uv/) | any recent | installing the worker (`worker/install.sh`, `partscaster-worker.exe`) |
| ruff | any recent | `make lint` fails without it |
| golangci-lint | v2 (CI uses v2.14.0) | optional locally: `make lint` skips it, CI runs it |
| ffmpeg | any | DSP tests in `internal/dsp` and the worker; tests that need it are skipped without it |

On Ubuntu 24.04 only WebKitGTK 4.1 exists; build the app with `GOFLAGS=-tags=webkit2_41`.

Full deployment guide (GPU machine, worker, models): [docs/en/deployment.md](docs/en/deployment.md).

## Running the checks

The Go code embeds the built frontend, so build it once before running Go tests:

```bash
cd frontend && npm ci && npm run build && cd ..
pip install ruff fastapi httpx numpy scipy librosa soundfile   # worker test dependencies, as in CI
```

Then, from the repository root:

```bash
make lint     # gofmt check, go vet, golangci-lint, eslint (frontend), ruff (worker)
make test     # go test ./..., vitest, worker unittest
```

Separate parts: `make test-go`, `make test-front`, `make test-worker`, `make lint-go`, `make lint-front`,
`make lint-worker`. `make help` lists all targets.

## Sending changes

- Development happens on `develop`; `main` receives released versions. Open pull requests against `develop`.
- Before a pull request, `make lint && make test` must be green. CI (`.github/workflows/ci.yml`) runs the same
  checks on every pull request and on pushes to `main`.
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/): `feat:`, `fix:`, `refactor:`,
  `docs:`, `test:`, `chore:`, `build:`, optionally with a scope (`fix(worker): …`). Russian or English.
- One pull request — one task; no unrelated edits.

## Project rules

- Change behaviour deliberately: a user-visible change shows in the commit message and, if noticeable, in
  [CHANGELOG.md](CHANGELOG.md) under `Unreleased`.
- Write tests from the specification of how things should work, not as a snapshot of current behaviour.
- UI strings go through the i18n dictionaries `frontend/src/i18n/ru.js` and `en.js` — add every key to both.
  Code comments are in Russian, identifiers in English.
- Documentation exists in two languages: `docs/ru/` and `docs/en/` with the same file names, `README.md` and
  `README.ru.md`, this file and `CONTRIBUTING.ru.md`. Change both languages in the same commit.
- New worker environment variables go into `worker.env.example` and the variable table in
  `docs/ru/deployment.md` and `docs/en/deployment.md`.
- After editing slots, groups or presets in `frontend/src/` or `worker/fx_blocks.json`, run `make mcp-data` and
  commit the regenerated files.
- No personal data (home paths, LAN addresses, host names) and no secrets in code or docs.

The full rule set, including architecture constraints, is in [AGENTS.md](AGENTS.md).

## Releases

1. Rename `## [Unreleased]` in [CHANGELOG.md](CHANGELOG.md) to `## [X.Y.Z] - YYYY-MM-DD`.
2. In the same commit add a new first entry to `frontend/src/releaseNotes.js` — the "What's new" window:
   5–10 short plain lines in Russian and English, `version: 'X.Y.Z'`. `releaseNotes.test.js` fails if CHANGELOG
   has a newer version than this entry.
3. Merge into `main`, then a maintainer creates and pushes the tag `vX.Y.Z`.
4. The tag starts `.github/workflows/release.yml`. It builds on Linux, Windows and macOS runners and publishes to
   the GitHub Release of that tag:
   - `partscaster-<version>-<os>-<arch>.tar.gz` (Windows: `.zip`) — the app, `partscaster-mcp`, the worker
     archive and `INSTALL.txt`; the Windows package also contains `partscaster-worker.exe` (worker without WSL);
     on macOS the MCP server and worker archive are also inside `partscaster.app/Contents/MacOS/`;
   - `worker-<version>.tar.gz` — the worker archive from the Linux build, as a separate file.

   The second engine yue2.cpp (experimental) is built into the worker archive on Linux (in a CUDA 12.8
   container) and on Windows (with the CUDA toolkit installed in the job). The macOS worker archive comes
   without it. If that build fails, the package still ships, without yue2.cpp.

To build a package locally: `make release` (this OS), `make dist-linux`, `make dist-windows` (also works on
Linux), `make dist-mac` (on a Mac only). Results land in `dist/`.
