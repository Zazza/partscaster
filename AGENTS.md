# AGENTS.md — rules for AI agents working in this repository

This is the working contract for any coding agent (Codex, Claude and others). Read it in full before
changing anything. Breaking these rules means a red CI or a broken release.

Partscaster has three parts: a desktop app (Go + Wails v2, Vue 3 UI), an MCP server for AI agents
(same Go code base) and a Python worker on the GPU machine (FastAPI around the YuE2 model).

## Hard rules

1. **Before handing off changes: `make lint && make test`, all green.**
   CI (`.github/workflows/ci.yml`) runs the same checks in three jobs:
   - `go` — builds the frontend (needed by `go:embed`), then gofmt, go vet, golangci-lint, go test;
   - `frontend` — eslint, vitest, production build, and the MCP data parity check (see rule 3);
   - `worker` — ruff and `unittest` discovery over `worker/test_*.py`.

   CI runs on pushes to `main` and on pull requests. Pushes to `develop` are **not** checked by CI,
   so on `develop` the local `make lint && make test` is the only gate.
2. **CHANGELOG and "What's new" go with every release.**
   - Every noticeable change gets an entry under `## [Unreleased]` in `CHANGELOG.md`, in the same commit.
   - When a human asks to cut a release, the agent renames `Unreleased` to `## [X.Y.Z] - YYYY-MM-DD`
     (Keep a Changelog groups: Added / Changed / Fixed / Removed) and, **in the same commit**, adds a new
     first entry to `frontend/src/releaseNotes.js` — the "What's new" window in the app. The source is the
     CHANGELOG section, but the text is written anew: 5–10 items, one plain line each about what changed for
     the user (no file names, endpoints or internal terms), in both `ru` and `en`. The entry's `version`
     equals the tag: the app uses the first entry to detect a new version. `releaseNotes.test.js` fails if
     CHANGELOG has a version newer than the first entry.
   - The tag `vX.Y.Z` is created and pushed by a human. The tag starts `.github/workflows/release.yml`,
     which builds and publishes the packages (see [CONTRIBUTING.md](CONTRIBUTING.md)). Agents do not tag,
     push tags or publish releases.
3. **Update dependents in the same change.** When you edit something, check what depends on it:
   - new or changed worker endpoint or response fields → method in the `yue.Service` interface
     (`internal/yue/types.go`) and its implementation in `internal/yue/<domain>.go` → `App` binding
     (`app*.go`) → wrapper in `frontend/src/api.js` → **MCP tool** (`internal/mcp/tools_*.go`). All API
     clients stay at parity;
   - slots, groups or presets in `frontend/src/` (`slotOptions.js`, `groups.js`, `presets.js`,
     `fxPresets.js`) or engine blocks in `worker/fx_blocks.json` → run `make mcp-data` and commit every
     regenerated file: `internal/mcp/*.json`, `frontend/src/fxBlocks.json` and `worker/fx_presets.json`.
     CI only diffs `internal/mcp/` and `frontend/src/fxBlocks.json`, so a stale `worker/fx_presets.json`
     slips through — commit it anyway;
   - architecture, data flow or deployment changes → `docs/{ru,en}/architecture.md`, `deployment.md`,
     `mcp.md`, `api.md` updated in the same change, not "later";
   - new or changed DSP chain (`internal/dsp`) → a row in `docs/ru/effects.md` and `docs/en/effects.md`
     with its ID written as (`id`), plus the updated chain count (`TestEffectsDocListsEveryChain` fails
     otherwise);
   - worker environment variables → `worker.env.example`, the variable table in `docs/{ru,en}/deployment.md`,
     and `docs/{ru,en}/mcp.md` if MCP is affected;
   - user-visible UI strings → both dictionaries `frontend/src/i18n/ru.js` and `en.js` (`i18n.test.js`
     checks that keys match);
   - noticeable user-facing changes → CHANGELOG `Unreleased` (rule 2).
4. **Documentation exists in two languages, kept in step.**
   - `docs/ru/` and `docs/en/` hold the same set of files with identical names; the root has `README.md`
     (English) and `README.ru.md` (Russian); `CONTRIBUTING.md` / `CONTRIBUTING.ru.md` likewise.
   - Any documentation change is made in **both** languages in the same commit. A fact added only to one
     language is a bug.
   - Tests read the docs: `docs/ru/effects.md` and `docs/en/effects.md` (every DSP chain as (`id`), the
     chain count, the `` | `level` | `` row of the engine block table) and `docs/ru/ui-guide.md` (the
     `## Пресеты звука` section). Keep these anchors when editing.
5. **Tests follow the specification of how things should work**, not a snapshot of current behaviour.
   If a test exposes a defect, fix the code, not the test (unless the test misread the actual data contract).
6. **No personal data in code or docs:** machine addresses, home directories (`/home/<user>`), Windows
   drive mounts (`/mnt/<letter>`), LAN IPs, internal host nicknames. Use neutral examples
   (`~/partscaster`, `/data/hf-cache`, `user@gpu-host`, `192.0.2.10`) or environment variables.
7. **No secrets or tokens in the repository.** Machine-specific settings live only in
   `worker.env.example`, as commented-out examples.

## Architecture (do not break)

### Repository layout

| Path | What it is |
|---|---|
| `main.go`, `app*.go` | Wails entry point; `App` bindings split by area (`app.go`, `app_preview.go`, `app_recipe.go`, `app_setup.go`); `audioProxy` in `main.go` serves job artifacts to the webview |
| `player*.go`, `signals_*.go`, `open_*.go` | external audio player, pause signals, opening URLs — per-OS via build tags |
| `internal/yue` | HTTP client of the worker: `Service` interface, transport in `client.go`, methods per domain (`jobs`, `stems`, `llm`, `fx`, `presets`, `queue`, `recipe`, `voices`, `engine`, `instruments`, `journal`, `version`) |
| `internal/studio` | studio pipelines on the desktop side (ffmpeg + worker): preview cache, DSP apply, splice, sections, live layer, recipe |
| `internal/dsp` | ffmpeg filter chains (effects) |
| `internal/mcp`, `cmd/partscaster-mcp` | MCP server: tools, embedded library data |
| `internal/config` | worker address: `PARTSCASTER_URL` → app settings → `http://localhost:8091` |
| `internal/agentsetup` | the "For AI agents" settings section: ready-made MCP client config snippets |
| `internal/buildinfo`, `cmd/srchash` | source fingerprint baked into the MCP binary, so it can report being older than its sources |
| `internal/workerexe`, `cmd/partscaster-worker` | Windows worker without WSL (`partscaster-worker.exe`): installs uv, Python, packages and runs the worker |
| `cmd/partscaster-align` | developer tool: measures timing alignment of a rendered part against a track window |
| `frontend/` | Vue 3 UI |
| `worker/` | Python worker (FastAPI), its install scripts and data |
| `tools/` | `release.sh`, `worker-tar.sh`, `mcp-gendata.sh`, `yue2cpp-build.sh` / `.ps1` |
| `deploy/` | systemd user unit `partscaster-worker.service`, `INSTALL.txt` for release packages |

### Go host (desktop, Wails v2)

- `App` holds thin Wails bindings for the frontend. Keep business logic out of bindings: pipelines go to
  `internal/`, heavy work to the worker.
- Dependencies are injected through interfaces, **not global variables**:
  - `Player` (`player.go`) — passed to `NewApp(client, player)`. A global player variable is forbidden.
  - `yue.Service` (`internal/yue/types.go`) — `App` and the MCP server work with the interface.
    `yue.Journaled` wraps it so that mutating calls are written to the track's edit journal.
- Worker HTTP calls only through the shared helpers `Client.call` / `get` / `postJSON` / `postRaw` / `del`
  (`internal/yue/client.go`). Do not hand-roll marshal → request → status → decode.
- A new worker method: add it to `Service`, implement it in `internal/yue/<domain>.go`, add a binding in
  `app*.go`; if the webview needs a URL to the file, extend `audioProxy` in `main.go`.
- Platform-specific code only via build-tagged files (`_unix.go` / `_windows.go`): player, signals,
  `openExternal`. Do not mix it into shared code.
- ffmpeg chains live in `internal/dsp`; fragment processing goes through `dsp.Span`, not a separate function.
- Magic numbers become named constants (see the top of `app.go`).

### Frontend (Vue 3)

- `App.vue` is the orchestrator (pages, form, job list). Do not grow it: screens go to `src/components/`,
  state to `src/composables/`.
- `usePlayer` / `useConfirm` are singletons (state in module scope). Do not copy player state into
  components; call `usePlayer()` from anywhere.
- Pure logic (no DOM) lives in plain modules (e.g. `src/styleLogic.js`) and is covered by vitest. Logic in
  a component that you want to test — move it out first.
- Dictionary data (slots, part templates, durations) lives in `src/slotOptions.js`, not in components.
- Props/events between `App` and components; `defineModel` for `v-model`.
- UI strings go through `t('key')` with dictionaries `src/i18n/ru.js` and `src/i18n/en.js`; never hardcode a
  user-visible string in one language.
- `src/wailsjs/**` is generated by Wails — do not edit by hand.

### MCP server (`cmd/partscaster-mcp`, `internal/mcp`)

- Protocol: JSON-RPC over stdio, no external dependencies. Tools live in `internal/mcp/tools_*.go`.
- Client logic goes through `yue.Service` — the same interface the app uses.
- **Parity:** new worker API method → `yue.Service` method → MCP tool.
- Library data `internal/mcp/*.json` is generated from `frontend/src/` and `worker/fx_blocks.json`
  (`make mcp-data`) and embedded with `go:embed`. **Never edit it by hand.**
- Destructive tools (delete / cancel / install and similar) require `confirm=true`; agents must ask the
  user first (the tool descriptions say so).

### Worker (Python, FastAPI)

- Entry point: `yue_worker.py` (API around the resident YuE2 pipeline, port 8091). Pure helpers are separate
  modules, for example: `opqueue.py` (shared queue of long GPU operations), `llm.py` (Ollama wrapper),
  `media.py` (audio conversion), `stems.py` + `sep_run.py` (stem separation), `engine_cpp.py` (second
  engine, yue2.cpp, experimental), `fx_engine.py` / `fx_nam.py` (sound engine), `presets.py` / `genres.py`
  (sound presets), `voice.py` + `seedvc_run.py` (voice timbre swap), `whisper_run.py` (lyrics
  transcription), `sheetsage.py` (melody to ABC), `instruments.py` / `phrases.py` ("Instruments" page),
  `recipe.py`, `plancheck.py`, `proclock.py`, `kits_install.py`, `waveform.py`.
- Ollama only through `llm.ollama_chat` (+ `llm.strip_md`); no duplicate urllib calls. `keep_alive=0` is
  mandatory: VRAM is shared with YuE2.
- Conversions through `media.encode_mp3` and soundfile; ffmpeg is not assumed on the GPU machine. The one
  exception is the studio spectrogram: `waveform.render_spectrum_png` uses ffmpeg if it is on PATH;
  otherwise the spectrum endpoint returns 503 and the loudness waveform still works.
- Environment variables are read in several modules: `yue_worker.py`, `stems.py`, `engine_cpp.py`,
  `voice.py`. Most are read once at import; some are read on every call (for example
  `PARTSCASTER_FX_ENGINE`, `PARTSCASTER_STEMS_MODEL`, `PARTSCASTER_SEP_PY`, and the yue2.cpp paths in
  `engine_cpp.py`). Do not assume a single place to look. A new variable goes into
  `worker.env.example` and the table in `docs/{ru,en}/deployment.md`.
- Packaging is by rule, not by list (`tools/worker-tar.sh`): the worker archive contains `worker/*.py`
  except `test_*`, `worker/*.json`, `worker/requirements*.txt`, `install.sh`, `seedvc_install.sh`,
  `worker/phrases/*`, the systemd unit and, if built, yue2.cpp. A new file of any other type or location
  will **not** ship — extend the script in the same change.

## Deliberate decisions (do not "fix" without asking)

- The worker has no authentication — a LAN tool by design (README, "Security").
- Player pause on Linux/macOS is SIGSTOP/SIGCONT of the external process; on Windows it is stop and restart.
- The webview (webkit2gtk) has no audio output — hence an external player, not `<audio>`.
- Transient studio state survives restarts in localStorage, nested per job (`yue_studio_state.jobs.<id>`).
  On a schema change old data is ignored; no migrations.
- localStorage keys and some paths keep historical `yue` names (`yue_studio_state`, `yue_server`,
  `yue_volume`, `~/yue/.venv`, `~/yue/hf-cache`). Renaming them would lose users' state or force a reinstall;
  leave them as they are.

## Commits and process

- Conventional Commits: `feat:` / `fix:` / `refactor:` / `docs:` / `test:` / `chore:` (+ `build:` for build
  changes), optional scope: `fix(worker): …`. Messages are written in Russian in this repository's history;
  either language is accepted.
- Code comments are in Russian; identifiers are in English.
- Change behaviour deliberately; noticeable user-facing changes go to CHANGELOG.
- Commit only what belongs to the task. `build/bin/`, `dist/`, `__pycache__/`, `.idea/`, `tmp/` and
  `cmd/partscaster-worker/payload/*.tar.gz` are not tracked (see `.gitignore`).
- Build: `make build` (Wails). MCP server: `make mcp` (and `make mcp-data` after the edits listed in rule 3).
  Worker deploy: `make worker` (host from `PARTSCASTER_DEPLOY_HOST`). Release package for this OS:
  `make release` (or `make dist-linux` / `dist-windows` / `dist-mac`).

## Common commands

```bash
make lint        # gofmt check, go vet, golangci-lint (skipped if absent), eslint, ruff
make test        # go test ./..., vitest, worker unittest
make fmt         # gofmt -w
make build       # wails build
make mcp-data    # regenerate embedded MCP data from frontend sources
cd frontend && npm test && npx eslint src && npm run build
cd worker && python3 -m unittest discover -p 'test_*.py' && ruff check .
go test ./... && go vet ./...
```
