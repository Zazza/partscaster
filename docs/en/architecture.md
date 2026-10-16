# Partscaster architecture
**English** · [Русский](../ru/architecture.md)

Partscaster is three programs: a desktop app (a window), an MCP server for AI agents, and a worker
(a server on a machine with an NVIDIA GPU that generates music with the YuE2 model). The app and MCP
do not generate anything themselves: they call the worker over HTTP. This page shows what they consist of and how they connect;
installation is in [deployment.md](deployment.md), the worker API is in [api.md](api.md), the MCP tools are in [mcp.md](mcp.md).

## Overview

```
 PC (Linux / Windows)                              Machine with NVIDIA (often the same PC)
┌──────────────────────────────┐   HTTP :8091    ┌─────────────────────────────────────┐
│ partscaster — app            │ ──────────────▶ │ worker (Python, FastAPI, :8091)     │
│ Wails v2: Go + Vue 3         │                 │  • shared GPU queue                 │
│  • sound — system player     │                 │  • YuE2 (PyTorch) — main engine     │
│  • ffmpeg effects — locally  │                 │  • yue2.cpp (yue-server on          │
│  • sound presets, rebuild    │                 │    127.0.0.1) — second, experiment  │
├──────────────────────────────┤                 │  • SheetSage2, demucs / RoFormer    │
│ partscaster-mcp — MCP server │ ──────────────▶ │  • sound engine, NAM, kits          │
│ stdio, for AI agents         │                 │  • whisper, Seed-VC — separate      │
└──────────────────────────────┘                 │    environments (optional)          │
                                                 │  SQLite + track files               │
                                                 └──────────────┬──────────────────────┘
                                                                │ HTTP (optional)
                                                          Ollama :11434
```

- The app and the worker are separate processes: closing the app does not stop generations; the queue and
  tracks live on the worker (`<data>/yue.db`; by default, data is in `~/partscaster/data`).
- On Linux, the worker runs as a systemd service (`partscaster-worker`); on Windows without WSL — as the program
  `partscaster-worker.exe` (see below).
- The app and MCP use one worker address: the `PARTSCASTER_URL` variable → the address saved in the app's "Settings"
  → `http://localhost:8091` (`internal/config`).

## App (repository root, Go)

| File/directory | Role |
|---|---|
| `main.go` | starts Wails, embeds the built frontend, proxies `/audio/<id>/<file>` to the worker |
| `app.go`, `app_preview.go`, `app_recipe.go`, `app_setup.go` | bindings for Vue (`Yue*` methods): thin wrappers over the worker client and the `internal/studio` pipelines; effect previews; export/import of a track recipe; version, worker check, the "For AI agents" section. Also here: background run of the sound presets chosen when a track was created (every 10 s) |
| `player.go`, `player_unix.go`, `player_windows.go`, `player_volume_*.go` | built-in player: the webview in Wails has no audio output. Linux — `pw-play` (PipeWire), pause by stopping the process; Windows — WPF MediaPlayer through PowerShell (pause does not keep the position). A non-Windows build uses the same `pw-play` |
| `loopwav.go` | a phrase loop from the "Instruments" page, laid out into a long WAV for the player |
| `internal/config` | worker address and app settings (`settings.json` in the user settings directory, `partscaster/`) |
| `internal/yue` | HTTP client of the worker. `client.go` — transport only; methods are split by domain: `jobs.go`, `stems.go`, `llm.go`, `fx.go`, `presets.go`, `queue.go`, `recipe.go`, `voices.go`, `engine.go`, `instruments.go`, `journal.go`, `version.go`; the `Service` interface is in `types.go` |
| `internal/dsp` | ffmpeg effects on a finished file: chains (dynamics, modulation, space, pedals, glitch), inserts and fitting them to the rhythm, stereo position, volume line, level measurement |
| `internal/studio` | studio and sound preset pipelines on the PC side (below) |
| `internal/agentsetup` | the "For AI agents" section: ready-made config snippets for Claude Desktop and Codex, connecting to Claude Code with the `claude mcp add` command |
| `cmd/partscaster-align` | helper utility: measures how well an insert is in sync (offset, tempo, error in ms) |

### Studio and sound presets (`internal/studio`)

This is code shared by the app and MCP. It turns track edits into a new version. The heavy work (model,
separation, sound engine, master) is done by the worker; ffmpeg processing and assembly are done by the PC.

- **Rebuild** (`sections.go`, `splice.go`, `dspapply.go`, `envelope.go`): effects on stems, inserts of
  pieces re-rendered by the model, volume line, stem position in stereo. A rebuild always starts from the
  clean `audio.flac` with all replacements; the vocal is not replaced when instruments are inserted; the master is the last
  step on the worker, and a "mix without master" is saved next to it.
- **Sound presets** (`presets.go`): a recipe for processing a finished track. `ApplySoundPreset` gets the stems
  (if there are none, it separates; drum parts — RoFormer), installs the needed sample kits, rebuilds the edits for
  "whole track", runs the final ffmpeg chain, the master and loudness to the target on the worker, and saves the result as a track
  version; levels are checked on the result. `RunGraph` is a shared step "ffmpeg graph on a track file → variant".
  `PresetRunner` applies the presets chosen when a track was created, when the GPU queue is empty.
- **"Live sound" layer** (`livelayer.go`): added to any preset — air above 16 kHz and cymbals
  for presets with live kits, a drummer in the choruses ([effects.md](effects.md)). It is not part of the preset recipe.
- **Parts** (`parts.go`): synth notes by chords and percussion hits by bars.
- **Track recipe** (`recipe.go`): import of a song on another installation — the root by generation, then the entries of
  the edit journal in order.
- **Queue operation** (`queueop.go`): `RunOp` / `StartOp` — a chain (preset, rebuild) takes one
  place in the worker's shared queue until it finishes (see "Shared queue").

The worker stores the preset recipes and the "preset on track" statuses (table `sound_presets`); the PC applies them.

## Interface (`frontend/`, Vue 3 + Vite)

| Directory/file | Role |
|---|---|
| `src/main.js`, `src/App.vue` | entry and shell: sidebar, track list, "New track" form, page switching |
| `src/components/` | screens and large blocks (31 components): `SideBar` (sections and the queue panel), `StudioPage` (track studio) with `StudioPresets`/`StudioMaster`/`StudioSynth`/`StudioPerc`/`TrackDesk`/`WaveView`/`PedalBoard`, `PresetsPage` and the preset editor, `InstrumentsPage`, `VoicesPage`, `SettingsPage`, modal windows |
| `src/composables/` | shared state and behavior: `usePlayer`, `useProcQueue` (tracking a queue operation), `useWorkerCompat` (version compatibility), `useInserts`, `useRevoice`, `useWindowPlay` and others |
| `src/*.js` | pure logic without DOM (tests next to it — `*.test.js`, vitest): style library (`presets.js`, `slotOptions.js`, `groups.js`), effect chains (`fxPresets.js`, `fxBlocks.json`, `fxChain.js`), sound presets, queue (`queueView.js`), recipe and others |
| `src/api.js` | wrapper over the generated Wails bindings (`src/wailsjs/`) |
| `src/i18n/` | interface strings: `ru.js`, `en.js` |

Sidebar sections: "New track", "track list", "voices", "instruments", "sound presets",
"settings" and the "Queue" block.

## MCP server (`cmd/partscaster-mcp`, `internal/mcp`)

`partscaster-mcp` is an MCP server (Model Context Protocol): it gives an AI agent (Claude Code, Claude Desktop,
Codex) the same features as the app. Transport — stdio, protocol — JSON-RPC 2.0, one message per line.

- It calls the worker with the same `internal/yue` client and uses the same `internal/studio` pipelines
  (presets, rebuild, effects) — the result matches the app.
- The tools (77) are split into files: `tools_workflow.go` (generation and tracks), `tools_studio.go`,
  `tools_revoice.go`, `tools_fx.go`, `tools_presets.go`, `tools_library.go` (style library),
  `tools_queue.go`, `tools_recipe.go`, `tools_install.go` (`doctor`, `install_worker`, `install_app`).
- Library data (`internal/mcp/*.json`) are copies of frontend sources embedded in the binary;
  `make mcp-data` regenerates them (`tools/mcp-gendata.sh`, which also writes `worker/fx_presets.json`).
- Downloaded files go to `PARTSCASTER_MCP_DIR` (default `~/partscaster-mcp-downloads`).
- Calls in the shared queue are marked with the source "mcp" (in the app — the "AI" mark); calls that change something
  are written to the track's edit journal.
- Warnings in responses: a worker of another API generation (`compat.go`) and an MCP built earlier than its
  sources (`stale.go` + `internal/buildinfo`; `make mcp` embeds the fingerprint through `cmd/srchash`;
  in a release build this check is off).

## Worker (`worker/`, Python)

The worker is the FastAPI app `yue_worker:app` on port 8091 without authentication. All modules are flat
in `worker/`.

| Module | Role |
|---|---|
| `yue_worker.py` | all endpoints, SQLite, generation loop, model loading, settings (`<data>/settings.json`), overdub mix |
| `opqueue.py` | shared queue of long operations (below); a pure module without FastAPI and torch |
| `proclock.py` | inter-process file lock on Linux and Windows |
| `engine_cpp.py` | second engine **yue2.cpp**: models (pinned files and checksums), starting and stopping `yue-server`, translating requests, parsing the response and the log; from the command line — `python engine_cpp.py download <model>` |
| `sheetsage.py` | SheetSage2: track → ABC score (for the piano roll and overdub) |
| `abcparse.py`, `plancheck.py`, `arc.py`, `chordgrid.py` | parsing ABC into a timeline; checking an edited plan; dramaturgy (build-up, explosion) on top of the plan; chord grid by bars |
| `stems.py`, `sep_run.py` | stem separation: demucs in the worker process, or BS-Roformer-SW + DrumSep as a subprocess in a separate environment |
| `fx_engine.py`, `fx_nam.py` | sound engine (a chain of blocks on numpy/scipy, without time shift); NAM amplifier |
| `drumsynth.py`, `kits_install.py` | synthesis of drum machines and synth basses; installing sample kits in advance |
| `guitarpart.py`, `guitarnotes.py` | a guitarist part from the plan chords, and splitting it into hits for the `gtrnotes` block |
| `presets.py`, `genres.py` | sound presets: recipe check, built-in presets, statuses on a track; genre presets from ready-made chains (`fx_presets.json`) |
| `instruments.py`, `phrases.py`, `phrases/` | the "Instruments" page: your own instruments and short phrases for listening |
| `recipe.py` | track recipe from `jobs` rows and the `job_ops` journal |
| `voice.py`, `seedvc_run.py` | "album voice": Seed-VC as a separate program from its own environment |
| `whisper_run.py` | song lyrics recognition; runs with the interpreter of the separate whisper environment |
| `llm.py` | Ollama call (`keep_alive: 0`) |
| `dsp.py`, `waveform.py`, `media.py` | track measurement (librosa); loudness waveform and spectrogram; audio conversion |

SQLite tables: `jobs`, `sound_presets`, `job_ops` (edit journal), `recipe_imports`,
`fx_instruments`, `voices`.

Data (`PARTSCASTER_DATA_DIR`): `jobs/<id>/` — `audio.flac`/`.mp3`/`.wav`, `score.abc`, `request.abc`,
`latent.npy` (25 frames/s), `semantic.npy`, `metrics.json`, variants `dsp-*`/`overdub-*`/`preview-*`/`stem-*.flac`;
`voices/`, `references/`, `transcribes/`; `fx/amps`, `fx/irs`, `fx/kits` —
NAM captures, impulse responses, sample kits; `yue2cpp-models/` — yue2.cpp models; `settings.json`.

The worker computes the waveform and the spectrogram and caches them next to the audio: `<file>.peaks.json`,
`<file>.spectrum.png` + `.spectrum.json`. The waveform does not need ffmpeg; the spectrogram does
(without it, the spectrum endpoint answers 503).

## YuE2 generation pipeline

Model stages: plan (ABC score) → semantic tokens (autoregression from left to right) →
latents (flow-matching) → VAE decoding in pieces.

Consequences:
- **an edit in the middle changes the tail**: there are no point edits; each variant is a full recomputation;
- **a piece preview is cheap**: the latents are saved, so a piece decodes in seconds without generation;
- **the instruments are set by the style string**, not by the score: ABC has only the Vocal and Ins voices.

## Two generation engines

| | Main | yue2.cpp (experiment) |
|---|---|---|
| What it is | PyTorch, package `yue2` (YuE2-3B, BF16) | [yue2.cpp](https://github.com/ServeurpersoCom/yue2.cpp) on GGML: the same weights in GGUF (bf16/q8/q6/q5) |
| VRAM at peak | ~11 GB (measured ~11.4) | BF16 ~10 GB, Q8 ~8, Q6/Q5 ~7.7; low-VRAM mode (`--max-seq 8192`, song up to ~3.5 min) — ~4.5 |
| How it works | the model loads on the first operation and stays loaded | the worker starts `yue-server` on 127.0.0.1 itself and shuts it down |

- The choice is the worker setting `engine` (app, MCP `config_set`); it takes effect from the next generation,
  and a track remembers its engine (`jobs.engine`). Only one engine is in VRAM at a time: before starting
  yue2.cpp, the worker unloads PyTorch; before loading PyTorch, it stops yue2.cpp.
- **The seed does not carry over between engines or between yue2.cpp models** — you get a different performance.
  **Notes carry over**: continuation and preview use `semantic.npy` / `latent.npy`, the format is shared.
- Both engines have the model operations: track, custom plan, dramaturgy, draft, fragment, continuation,
  plan only, piece preview, cancel.
- The yue2.cpp program and the model files are pinned (repository commit, GGUF revision and sha256): an update of
  the other project does not change the sound of tracks already made.

## Shared queue (`worker/opqueue.py`)

There is one GPU, so one long operation runs (the "slot"); the others wait by their place in the queue.

- One queue holds: generations (place — `jobs.queue_pos`, survives a restart), heavy sections of endpoints
  (`gpu_queue` → `QUEUE.slot`: render, plan, preview, stems, voice, whisper, SheetSage, effects with
  an amplifier) and **client operations** — app and MCP chains (sound preset, rebuild) that hold the
  slot for all their steps.
- Chain steps carry the `X-Partscaster-Op` header and do not enter the queue again; nested worker sections
  (auto-stems after a render, stems inside effects) do not either. Acquisition order: the slot, then
  the model lock `_pipe_lock`.
- A client operation lives on "alive" heartbeats (`POST /queue/ops/{id}/beat`): if the client is gone for 180 s,
  the operation is "interrupted" and the slot goes to the next one.
- Cancel: a waiting operation is removed at once; for a running one, external processes (RoFormer, Seed-VC, whisper, yue2.cpp)
  are stopped, and work inside the worker process (demucs, SheetSage, NAM, plan) finishes the current step;
  the result is "canceled", and the request gets 409.
- The pause and client operations are kept in the worker's memory: after a restart the pause is removed, and the client gets
  404 and shows "interrupted".
- Endpoints: `GET /queue`, `POST /queue/ops`, `/queue/ops/{id}/beat|finish|move|cancel`, `POST /queue/pause`.
  In the app, the queue is shown in the sidebar (`queueView.js`); in MCP — `queue_list`, `queue_move`,
  `queue_cancel`, `queue_pause`. The client side is `internal/yue/queue.go` and `internal/studio/queueop.go`.

## VRAM

- The main engine keeps the model loaded between requests: ~7.7 GB at rest, ~11 GB at the generation peak.
- SheetSage2 is small (hundreds of MB) and also stays loaded.
- Separation: demucs (~0.7 GB) loads per call and unloads; RoFormer is a separate process, and its memory
  is freed when it exits.
- Sound engine: a NAM capture (tens of MB) loads per call through the queue; the other blocks run on the CPU.
- Ollama is called with `keep_alive: 0` and unloads right after the answer. If Ollama keeps a model
  in VRAM, generation can fail with an out-of-memory error.

## Track stems (contract)

Files `stem-*.flac` in the track directory:

- **main** — `drums`, `bass`, `other`, `vocals`: together they make the track. They are used for the backing track, inserts,
  rebuild, vocal check (`vocal_leak`);
- **detailed inside "other"** — `guitar`, `piano`: a refinement of `other`, not part of the sum;
- **detailed inside "drums"** — `kick`, `snare`, `toms`, `hh`, `ride`, `crash` (DrumSep, only with
  RoFormer): a refinement of `drums`, not part of the sum.

The model is the worker setting `stems_model`: `htdemucs` (default, fast) or `roformer`
(BS-Roformer-SW, cleaner, ~4.5 times slower; one time — `POST /jobs/{id}/stems?model=roformer`). In RoFormer,
"other" does not include guitar and keys, so the app's `other` = the model's other + guitar + piano, and the remainder
"track − sum of the main stems" is distributed to the stems by their contribution at each frequency — the sum is exact. If there is no RoFormer environment,
it crashed, or `PARTSCASTER_STEMS_MODEL=demucs` is set — demucs works (htdemucs + htdemucs_6s), and there are no drum
parts. Which model ran — the `model` field of the response; in `GET /config` — `stems_pref` (the choice),
`stems_model` (actually used), `roformer_available`.

## Worker on Windows without WSL (`cmd/partscaster-worker`, `internal/workerexe`)

`partscaster-worker.exe` is one program with the worker archive embedded into it when the Windows package is built
(`//go:embed payload`). The first run installs into the user folder (default
`%LOCALAPPDATA%\partscaster-worker`) uv, Python 3.12, the environment and packages, unpacks the worker code
into `app\` and starts uvicorn; after that it starts right away. While the window is open, the worker works: the Python process
is tied to the program by a Windows job object. Models are in `hf-cache\`, data is in `data\`; the PyTorch
mode is `YUE2_BACKEND=torch-eager`. Details: [deployment.md](deployment.md).

## Build and release (`tools/`, `.github/workflows/`)

| File | What it does |
|---|---|
| `tools/worker-tar.sh` | worker archive `worker-<version>.tar.gz`: code without tests, `*.json`, `requirements*.txt`, installation scripts, phrases, the systemd unit and the ready-made yue2.cpp program for its own OS, if built |
| `tools/release.sh` | package for an OS: app, `partscaster-mcp`, worker archive, `INSTALL.txt`; for Windows — also `partscaster-worker.exe` |
| `tools/yue2cpp-build.sh`, `tools/yue2cpp-build.ps1` | yue2.cpp build: Linux — in a CUDA 12.8 container, Windows — locally |
| `tools/mcp-gendata.sh` | MCP library data from frontend sources |
| `deploy.sh` | `worker` — the archive to the GPU machine and its `install.sh`; `build` — app build |
| `.github/workflows/ci.yml` | tests and linters for Go, frontend, worker; check of MCP data against the frontend (push to `main` and pull request) |
| `.github/workflows/release.yml` | on a `v*` tag — Linux, Windows, macOS packages and upload to GitHub Release |

The `make` commands and the build for each OS are in [deployment.md](deployment.md).
