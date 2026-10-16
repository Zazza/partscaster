# Deployment
**English** · [Русский](../ru/deployment.md)

How to install Partscaster: the worker goes on a machine with an NVIDIA GPU, the app goes on a PC (often this is
the same machine), and, if you want, an AI agent connects through MCP. How the parts work: [architecture.md](architecture.md).

## Where to start

| Your case | Path |
|---|---|
| Linux with NVIDIA, ready-made build | [Release package](#release-package) → `install.sh` |
| Let an AI agent install it (Claude Code and others) | [Via an AI agent](#via-an-ai-agent) — MCP tool `install_worker` |
| Windows with NVIDIA, no WSL | [partscaster-worker.exe](#worker-on-windows-without-wsl--partscaster-workerexe) from the Windows package |
| Development, your own changes | [From source](#from-source) — `make worker`, `make build` |
| Windows, but you want a Linux worker | [WSL2](#worker-in-wsl2) |

After you install the worker: start the app, open "Settings", enter the worker address (a plain IP or machine
name is enough — port 8091 is added automatically) and press "Check". On the same machine, use `127.0.0.1`.

## Requirements

- **NVIDIA GPU with a driver** (`nvidia-smi` must work). Other GPUs are not supported:
  the installers check `nvidia-smi` and stop without it.
- **VRAM** (generation peak): main engine ~11 GB (measured ~11.4); second engine yue2.cpp — Q8 ~8 GB,
  low-VRAM mode ~4.5 GB (song up to ~3.5 min). More: [yue2.cpp](#second-generation-engine--yue2cpp-experiment).
- **Linux worker:** Linux with systemd (tested on Ubuntu), [`uv`](https://docs.astral.sh/uv/) —
  `install.sh` uses it to create the environment and install packages; it does not install `uv` itself. It looks for it
  in `PATH`, then in `~/.local/bin` and `~/.cargo/bin` (where the uv installer puts it; ssh without a login shell does not
  add them to `PATH`). uv downloads
  Python 3.12 by itself. Internet access — for packages and weights.
- **Disk:** worker base ~17.5 GB (environment ~7.4 + weights ~10) + sample kits ~1.2 GB + room for tracks.
  Optional components — in the [table below](#components-and-disk-space).
- **The YuE2 model is open:** weights download from Hugging Face without a token or login.

## Release package

A release is the GitHub Release of tag `vX.Y.Z` (built by `.github/workflows/release.yml`). It contains:

| File | What is inside |
|---|---|
| `partscaster-<version>-linux-<arch>.tar.gz` | app, `partscaster-mcp`, worker archive (with yue2.cpp for Linux, if built), `INSTALL.txt` |
| `partscaster-<version>-windows-amd64.zip` | the same for Windows + `partscaster-worker.exe`; the worker archive inside has yue2.cpp for Windows |
| `partscaster-<version>-darwin-<arch>.tar.gz` | app `partscaster.app` (`partscaster-mcp` and the worker archive are inside `.app/Contents/MacOS/`); no yue2.cpp |
| `worker-<version>.tar.gz` | worker archive for a Linux machine with a GPU (the same as in the Linux package) |

The worker archives in packages for different OSes differ only in the yue2.cpp program. For a Linux machine with a GPU, take
`worker-<version>.tar.gz` or the archive from the Linux package: the archive from the Windows package has no Linux program,
and with `--engine cpp` `install.sh` will start to build it from source.

On the GPU machine:

```bash
tar xzf worker-<version>.tar.gz
./worker-<version>/install.sh
```

Then — the app ("Settings" → worker address → "Check") and, if you want, AI agents:
"Settings" → "For AI agents" (ready-made configs and the "Connect to Claude Code" button).

**Versions.** The worker answers `GET /version` (build, API version, code fingerprint, model, what is installed).
The app and MCP compare the API version: if the worker is older, a banner says "update the worker" (обновите воркер);
if it is newer — "update the app" (обновите приложение). Update **the worker first, then the app**.

## Via an AI agent

The MCP tool `install_worker` installs the worker and optional components: empty `host` — on this PC,
`host=user@gpu-host` — on another machine over ssh. The archive is taken from next to `partscaster-mcp` (release package)
or built with `make worker-tar` (MCP built from source).

- By default it is a dry run: **nothing is executed**; it shows components, disk space and steps.
- To install: `dry_run=false`, `confirm=true` and an explicit choice of `whisper`, `seedvc`, `roformer` (true/false);
  with `roformer=true` — also `stems_model=htdemucs|roformer` (what to use for stems by default).
- Paths: `hf_home` (weights cache, default `~/yue/hf-cache`), `sep_dir` (RoFormer), `seedvc_dir`;
  engine — `engine=torch|cpp`, `cpp_model=auto|bf16|q8|q6|q5`.
- Before installing, it checks the GPU and free space in the home directory; if there is not enough, the installation
  does not start.
- `doctor` — what is installed and working now (worker, components, GPU, disk space).

Inside, this is the same `install.sh` from the archive, plus steps for the components (see below) and starting the service.

## Components and disk space

| Component | Needed | Disk space | Without it |
|---|---|---|---|
| Worker: YuE2 generation, SheetSage2 score, demucs stems | required | ~17.5 GB (environment ~7.4 + weights ~10, downloaded on the first generation) | — |
| Sample kits (drums, basses, orchestra) | installed together with the worker | ~1.2 GB | downloaded on first selection |
| RoFormer: cleaner stem separation + drums split into parts | optional | ~7.3 GB (environment 6.1 + weights 1.1) | demucs makes the stems, no drum parts |
| whisper: lyrics recognition for tracks | optional | ~3.2 GB | recognition buttons are inactive with a "not installed" note |
| Seed-VC: "Album voice" (experiment) | optional | ~10–11 GB after installation, ~14 GB during it | the "Album voice" block with a "not installed" note |
| NAM: amplifiers in the sound engine | optional | a package folder without torch | only the `amp` block is unavailable |
| yue2.cpp: second engine | optional | model 2.6–7.2 GB + VAE 0.5 GB | the main engine works |

Hugging Face weights (`HF_HOME`), RoFormer and Seed-VC can live on another disk — then you need less space on the
home disk.

## Linux worker: install.sh

The worker ships as a versioned archive. Its `install.sh` does both installation and update:

1. checks the GPU (`nvidia-smi`) before installing the new code (the move from the old name, if needed, runs
   even earlier — see below);
2. puts the code into `~/partscaster/releases/<time>-<version>/`;
3. creates the environment `~/yue/.venv` (`uv venv --python 3.12`) if it does not exist, and installs the packages from
   `requirements.txt` from the PyTorch cu128 index — only for a new environment or a changed `requirements.txt`
   (fingerprint — `~/partscaster/.requirements.sha256`, without comments and blank lines: editing the text does not
   reinstall packages);
4. creates `~/partscaster/worker.env` if it does not exist, with one line `HF_HOME=` (default
   `~/yue/hf-cache`, `--hf-home DIR` — another directory; the tilde is expanded — systemd does not understand it);
5. atomically switches the link `~/partscaster/current` to the new version and installs the unit
   `~/.config/systemd/user/partscaster-worker.service`;
6. installs sample kits (`kits_install.py`; if it fails, the next run or the app downloads them);
7. with `--engine` — sets up the generation engine (see yue2.cpp);
8. keeps 3 previous versions and deletes the rest;
9. restarts the service and waits up to ~30 s for an answer from `/health` (`--no-start` — without starting).

The model loads not at startup, but on the first generation (the weights download at that time too).

| Parameter | What it does |
|---|---|
| `--no-start` | install without starting the service |
| `--hf-home DIR` | weights cache directory for a new `worker.env` |
| `--engine torch\|cpp` | generation engine; without the parameter the engine does not change (new installation — main engine) |
| `--cpp-model auto\|bf16\|q8\|q6\|q5` | yue2.cpp model (only with `--engine cpp`), default `auto` |
| `--cpp-low-vram` | yue2.cpp low-VRAM mode (only with `--engine cpp`) |
| `--rollback` | `~/partscaster/current/install.sh --rollback` — go back to the previous version |

Paths are changed with installer variables: `PARTSCASTER_HOME` (default `~/partscaster`) and
`PARTSCASTER_VENV` (`~/yue/.venv`). Data (`~/partscaster/data`) and `worker.env` are shared by all versions,
and the installer does not touch them. The environment `~/yue/.venv` and the cache `~/yue/hf-cache` have these names for historical reasons.

**Autostart after reboot.** The service is a user service (`systemctl --user`) and starts together with the user
session. To make the worker start after a reboot without a login (usually needed on a separate
GPU machine), enable this once:

```bash
sudo loginctl enable-linger $USER
```

Control: `systemctl --user status|restart|stop partscaster-worker`, log —
`journalctl --user -u partscaster-worker -n 50`.

**Without systemd:**

```bash
cd ~/partscaster/current && ~/yue/.venv/bin/python -m uvicorn yue_worker:app --host 0.0.0.0 --port 8091
```

In this case, set the variables from `worker.env` and `PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True` yourself.

**Migration from the old name.** If the home directory has `~/yue-studio` but no `~/partscaster` yet
(and `PARTSCASTER_HOME` is not set), `install.sh` moves the contents to `~/partscaster` by itself, disables and
removes the old `yue-worker` unit, renames the `YUE_*` keys in `worker.env` to `PARTSCASTER_*` and
leaves a link `~/yue-studio` → `partscaster`. The environment `~/yue/.venv` and the cache `~/yue/hf-cache` stay
in place. The app also copies its settings from the old folder once in the same way.

**ffmpeg** on the GPU machine is needed only for the spectrogram in the studio (`sudo apt install ffmpeg`); without it
the "spectrum" mode answers 503; the loudness waveform works.

### Updating

Order: **the worker first, then the app**. A new app with an old worker works, but without
the new features.

- From a release — `install.sh` of the new archive (as for installation).
- From source — `make worker` (or `./deploy.sh worker`) with `PARTSCASTER_DEPLOY_HOST=user@gpu-host`:
  the archive is copied to `~/partscaster/incoming` and installed by its `install.sh`.
- Restarting the worker interrupts running app and MCP operations (sound preset, rebuild — "interrupted")
  and removes the queue pause. A generation that was running at the moment of the restart is marked as an error; waiting generations
  stay in the queue.

### Worker variables

Set them in `~/partscaster/worker.env` (example — `worker.env.example` in the repository root); after editing —
`systemctl --user restart partscaster-worker`.

| Variable | Default | Meaning |
|---|---|---|
| `PARTSCASTER_DATA_DIR` | `~/partscaster/data` (the unit sets `<PARTSCASTER_HOME>/data`) | SQLite database, tracks, worker settings, kits, yue2.cpp models |
| `PARTSCASTER_HOME` | `~/partscaster` (set by the unit) | installation directory; the worker looks there for the yue2.cpp program built by `install.sh` (`yue2cpp-src`) |
| `HF_HOME` | `~/yue/hf-cache` (`install.sh` writes it into a new `worker.env`); without the line — `~/.cache/huggingface` | Hugging Face weights cache (YuE2, SheetSage2, demucs) |
| `PYTORCH_CUDA_ALLOC_CONF` | `expandable_segments:True` (set by the unit) | PyTorch VRAM allocation mode |
| `YUE2_MAX_TOKENS` | `16000` | semantic token budget per song ≈ maximum length (16000 — up to ~10 min; VRAM peak grows with length) |
| `YUE2_CFG_SCALE` | `1.5` | how closely to follow the style (classifier-free guidance): higher — more exact, but drier |
| `YUE2_BACKEND` | `torch` | main engine mode: `torch`, `torch-eager` (without CUDA graphs; the Windows worker sets it), `vllm` |
| `PARTSCASTER_OLLAMA_URL` | `http://127.0.0.1:11434/api/chat` | Ollama for the lyrics copywriter, style translation, translating lyrics to fit the syllables |
| `PARTSCASTER_OLLAMA_MODEL` | `qwen2.5-chat-ru:latest` | Ollama model |
| `PARTSCASTER_WHISPER_PY` | `~/whisper-venv/bin/python` | interpreter of the whisper environment |
| `PARTSCASTER_SEP_PY` | `~/sep-venv/bin/python` | interpreter of the RoFormer environment |
| `PARTSCASTER_SEP_MODELS` | `~/sep-models` | RoFormer and DrumSep weights (~1.1 GB, downloaded on the first separation) |
| `PARTSCASTER_STEMS_MODEL` | — | `demucs` — always separate with demucs, even if RoFormer is selected and installed |
| `PARTSCASTER_SEEDVC_DIR` | `~/partscaster/seedvc` | Seed-VC directory |
| `PARTSCASTER_NAM_DEPS` | — | folder with NAM packages for the `amp` block |
| `PARTSCASTER_FX_ENGINE` | — | `0` — sound engine turned off (`/jobs/{id}/fx` → 503), whatever the settings say |
| `PARTSCASTER_CPP_BIN` | — | your own `yue-server` program (otherwise — from the archive, then the one built by `install.sh`) |
| `PARTSCASTER_CPP_MODELS` | `<PARTSCASTER_DATA_DIR>/yue2cpp-models` | yue2.cpp model directory (GGUF) |

Variables of other programs:

| Variable | Where | Meaning |
|---|---|---|
| `PARTSCASTER_URL` | app, `partscaster-mcp` | worker address (takes priority over the one saved in "Settings"; default `http://localhost:8091`). An app started from a desktop shortcut does not see shell variables — set the address in "Settings" |
| `PARTSCASTER_MCP_DIR` | `partscaster-mcp` | where to download files (default `~/partscaster-mcp-downloads`) |
| `PARTSCASTER_HOME`, `PARTSCASTER_VENV` | `install.sh` | installation directory and worker environment |
| `PARTSCASTER_DEPLOY_HOST` | `deploy.sh`, `make worker` | `user@gpu-host` for deployment |
| `PARTSCASTER_TORCH_INDEX` | `seedvc_install.sh` | PyTorch index for Seed-VC (default CUDA 12.8) |

You can change the Ollama address and model on the fly on the app's "Settings" page; this lasts until the
worker restarts, then the values from `worker.env` are used again.

## Optional components

They are installed separately — by an agent (`install_worker`) or by hand with the commands below. The worker works without them.

### Stem separation — BS-Roformer-SW and DrumSep

By default, demucs makes the stems (fast, ~20 s for a 4-minute track). BS-Roformer-SW
(python-audio-separator) is cleaner (median SDR on MUSDB18: vocals 11.7 vs 9.1 dB, "other" 7.6 vs 4.8),
but ~4.5 times slower; DrumSep also splits the drums into kick, snare, toms, hi-hat, ride,
crash. It runs in a **separate environment** (audio-separator has its own torch and onnxruntime); the worker starts
it as a subprocess:

```bash
uv venv ~/sep-venv --python 3.12
uv pip install --python ~/sep-venv/bin/python "audio-separator[gpu]" audioread
```

- In another location — set `PARTSCASTER_SEP_PY` and `PARTSCASTER_SEP_MODELS` in `worker.env`. Agent:
  `install_worker roformer=true stems_model=roformer sep_dir=/data/sep` — the environment goes into
  `<sep_dir>/venv`, the weights into `<sep_dir>/models`, and the paths are added to `worker.env`.
- To turn it on: "Settings" → "stem separation", MCP `config_set stems_model=roformer`, one time —
  `make_stems model=roformer`. If there is no environment or it crashed, demucs separates without an error.
- **Weights license:** RoFormer-SW and DrumSep — CC BY-NC-SA (non-commercial), downloaded on the first separation.
  Commercially clean mode — `PARTSCASTER_STEMS_MODEL=demucs` (demucs weights — MIT).

### whisper — track lyrics

```bash
uv venv ~/whisper-venv && uv pip install --python ~/whisper-venv/bin/python faster-whisper
```

Path to the interpreter — `PARTSCASTER_WHISPER_PY`.

### "Album voice" — Seed-VC (experiment)

Replaces the voice timbre of a track with the timbre of a sample (`POST /jobs/{id}/voice`, MCP `voice_convert`); the music and
the melody do not change. The voice is recognizable, but it wobbles — this is a property of Seed-VC; inside phrases you can hear bits of music
that got into the vocal stem. It is installed into its own directory with its own environment; the worker calls it as a separate
program:

```bash
~/partscaster/current/seedvc_install.sh                 # into ~/partscaster/seedvc
~/partscaster/current/seedvc_install.sh /data/seedvc    # or into your own directory
```

- Needed: `git`, `uv`, ~14 GB during installation, ~10–11 GB after it (environment ~7.5 + weights ~2.4). The script
  checks disk space in advance. Everything — code of the pinned version, environment, weights, caches — is inside one directory.
- Non-default directory — `PARTSCASTER_SEEDVC_DIR=<directory>` in `worker.env` and a worker restart.
  Check: MCP `doctor` or `GET /config` (`seedvc_available`).
- Another CUDA version — `PARTSCASTER_TORCH_INDEX=<index>` before running the script.
- Resources (RTX 4070 Ti SUPER 16 GB): +3.8 GB VRAM during the conversion; 50 steps — ~0.6 of the vocal duration.
- Seed-VC license — GPL-3.0: the user installs it explicitly, and it runs as a separate program.

### NAM amplifiers

The sound engine (`POST /jobs/{id}/fx`, MCP `fx_apply`) runs on numpy/scipy and needs nothing. Only
the `amp` block needs neural-amp-modeler **0.12.2** (MIT; reads `.nam` v0.5 captures). It is installed into a separate
folder without its own torch (torch comes from the worker):

```bash
uv pip install --python ~/yue/.venv/bin/python --target /data/nam-deps --no-deps \
  neural-amp-modeler==0.12.2 wavio pytorch_lightning lightning_utilities torchmetrics auraloss \
  matplotlib contourpy cycler fonttools kiwisolver pillow pyparsing python-dateutil
```

Then `PARTSCASTER_NAM_DEPS=/data/nam-deps` in `worker.env` and a worker restart.

- Captures (`.nam`) and impulse responses (`.wav`) are not included — the user uploads them
  (`fx_asset_upload`) to `<data>/fx/amps` and `fx/irs`. The capture latency is measured on the first use.
- Rollback: `PARTSCASTER_FX_ENGINE=0` or `config_set fx_engine=false` — the endpoint answers 503; the app's ffmpeg
  effects work.

### Sample kits

Kits for the engine blocks (drums, bass, guitars, orchestra, piano) are installed in advance: `install.sh`
runs `kits_install.py` (~1.2 GB the first time, then no network needed). If a kit is missing, the app downloads it
on first selection with progress (`GET /fx/kits/progress`); by hand — MCP `fx_kit_install` or:

```bash
cd ~/partscaster/current && ~/yue/.venv/bin/python kits_install.py vsco-violin osdk
```

Run it with the variables from `worker.env` (`install.sh` does this), otherwise the kits go to the default data
directory. Files download through direct links from raw.githubusercontent.com (the lists are pinned in
`worker/fx_kit_files.json`); each file has 3 attempts, and an interrupted download resumes. Drum machines (`tr808`, `tr909`,
`linn`, `cr78`, `simmons`) and `synthbass` are synthesized by the worker itself, without network. Kit contents and licenses —
in [effects.md](effects.md).

### Ollama

A separate service on `127.0.0.1:11434`. Not required: generation works without it; the copywriter,
style auto-translation and translating lyrics to fit the syllables are unavailable. Model — `PARTSCASTER_OLLAMA_MODEL` or the "Settings" page
(the model list comes from Ollama).

## Second generation engine — yue2.cpp (experiment)

Less VRAM — for 8–12 GB cards or a GPU shared with other programs. The same seed
on another engine gives a different song; notes and continuations carry over.

- **Installation:** `install.sh --engine cpp [--cpp-model auto|bf16|q8|q6|q5] [--cpp-low-vram]`. `auto` — by
  the VRAM of the first card: ≥ 11.5 GB → Q8; 7.5–11.5 GB → Q8 in low-VRAM mode; less → Q5 in low-VRAM mode.
  `--engine torch` — go back to the main engine.
- **VRAM at peak** (song 2:47): BF16 ~10 GB, Q8 ~8, Q6 and Q5 ~7.7; low-VRAM mode — Q8 ~4.5,
  Q6 and Q5 ~4.2. Main engine — ~11.4.
- **Program:** a ready-made build from the worker archive (`worker-<version>/yue2cpp/`, CUDA 12.8; the CUDA libraries
  come from the worker environment, the CUDA development package is not needed). If it is missing or does not start, `install.sh`
  builds the pinned commit in `$PARTSCASTER_HOME/yue2cpp-src` (needs git, cmake, nvcc, ~10 min).
  If that fails, the worker is installed and works on the main engine.
- **Models** — in `<data>/yue2cpp-models` (`PARTSCASTER_CPP_MODELS`), downloaded with a checksum check:
  during installation, from the app ("Settings" → "Generation engine" → "Download") or with MCP `engine_model_install`.
- **Switching** — the app's "Settings" or MCP `config_set engine=cpp|torch`; takes effect from the next
  generation. The main engine stays installed.

## Worker on Windows without WSL — partscaster-worker.exe

The Windows package has `partscaster-worker.exe` next to the app. A double click opens a console window:
while the window is open, the worker works; close it, and the worker stops (no Python process stays in the background). There is no autostart.

- **First run:** checks the GPU (`nvidia-smi`) and disk space (~25 GB needed), lets you choose a folder (default
  `%LOCALAPPDATA%\partscaster-worker`, the choice is remembered; another one — `--home D:\partscaster-worker`). Into the folder
  it installs uv (pinned version, sha256 check), Python 3.12, the `venv` environment and packages:
  `requirements.txt`, `torch`/`torchaudio` `2.10.0+cu128`, the `yue2` package (archive of a pinned YuE commit),
  `truststore`. The YuE2 model (~10 GB) downloads on the first generation.
- **After that:** starts in seconds. Packages are reinstalled only if `requirements.txt` or
  the pinned versions changed (fingerprint — `packages.sha256`). A new program version puts new code into `app\`;
  data (`data\`), models (`hf-cache\`) and packages stay.
- **Address:** `127.0.0.1:8091` — in the app, enter `127.0.0.1`. For other machines on the network — `--lan` (the worker
  has no authentication, so by default it is reachable only from this PC).
- **Certificates:** uv takes them from the Windows store (`UV_SYSTEM_CERTS`), Python — through `truststore`;
  downloads work even where an antivirus or proxy replaces HTTPS certificates.
- **Engine:** on the first installation the program asks — main engine (PyTorch in
  `YUE2_BACKEND=torch-eager` mode: PyTorch for Windows is built without flash-attention) or yue2.cpp. The yue2.cpp option
  is there only if the package has `yue2cpp\yue-server.exe`. Model by card memory: less than 10 GB — q6 in
  low-VRAM mode, otherwise q8. The weights download right away, the choice is written to `data\settings.json` and is not
  asked again. Without questions: `--engine cpp --cpp-model q8 [--cpp-low-vram]` or `--engine torch`.
  To change it later — in the app's "Settings".
- The program does not install optional components (whisper, RoFormer, Seed-VC); it does not install sample kits
  in advance — they download on first selection.

## Worker in WSL2

For Windows where you need a normal Linux worker (for example, dual-boot with a shared disk). CUDA in WSL2 works
through the NVIDIA driver installed in Windows:

```powershell
wsl --install -d Ubuntu
```

Inside WSL, install `uv` and the worker the same way as on Linux (`install.sh`). If the weights and data are on a Windows disk
shared with the Linux boot (in WSL it is visible as `/mnt/<drive letter>`), set them in `~/partscaster/worker.env`:

```bash
HF_HOME=/mnt/<drive>/hf-cache                       # do not download the weights again
PARTSCASTER_DATA_DIR=/mnt/<drive>/partscaster-data  # shared tracks (or keep your own)
PARTSCASTER_OLLAMA_URL=http://<Windows IP>:11434/api/chat   # Ollama on Windows
```

Windows IP from WSL — `ip route show default | awk '{print $3}'`; in WSL with mirrored networking,
`127.0.0.1` also works. In WSL without systemd, start it with the command from the "Without systemd" section.

The app on Windows connects to `http://localhost:8091` (or to the WSL IP — `wsl hostname -I`).
If the data is shared, do not run the workers of both boots at the same time — there is one SQLite database.

## From source

Requirements: Go 1.24+, Node.js 20+, [Wails CLI v2](https://wails.io) of the same version as in `go.mod`
(v2.12.0: `go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0`). For the worker — `uv`; for
`make lint` — `ruff` (golangci-lint optional); for `make yue2cpp` — docker.

| Command | What it does |
|---|---|
| `make worker` | worker archive to the GPU machine (`PARTSCASTER_DEPLOY_HOST=user@gpu-host`) and its `install.sh` |
| `make worker-tar` | only the archive `dist/worker-<version>.tar.gz` |
| `make yue2cpp` | ready-made yue2.cpp build for the worker archive (in a CUDA 12.8 container) |
| `make build` | app `build/bin/partscaster` with the version from git |
| `make mcp` | MCP server `build/bin/partscaster-mcp` (connecting — [mcp.md](mcp.md)) |
| `make mcp-data` | regenerate the MCP library data from the frontend |
| `make release` | package for this OS in `dist/` |
| `make dist-linux` / `dist-windows` / `dist-mac` | package for an OS |
| `make install-desktop` / `uninstall-desktop` | shortcut in the Linux applications menu |
| `make test`, `make lint`, `make fmt` | tests, linters, Go formatting |

### Packages by OS

| Target | Where it builds | Result |
|---|---|---|
| `make dist-linux` | Linux only | `partscaster-<version>-linux-<arch>.tar.gz` |
| `make dist-windows` | Linux or Windows | `partscaster-<version>-windows-amd64.zip` (with `partscaster-worker.exe`) |
| `make dist-mac` | Mac only | `partscaster-<version>-darwin-<arch>.tar.gz` |

Before the package, yue2.cpp is built if it is not in `dist/` yet: on Linux — if docker is available, on Windows —
if CUDA Toolkit (`nvcc`) is available. If that fails, the worker archive ships without the second engine; this is not an error.
The release in CI starts from a `v*` tag, which a human sets and pushes; each OS package is built on its own
runner.

### App by OS

**Linux (main platform):**

```bash
sudo apt install libgtk-3-dev libwebkit2gtk-4.1-dev
make build              # build/bin/partscaster
make install-desktop    # shortcut in ~/.local/share/applications
```

The shortcut is also needed for the icon: GNOME on Wayland takes it only from a `.desktop` file. Sound — through `pw-play`
(PipeWire): the webview in webkit2gtk has no audio output.

**Windows:** `wails build` (WebView2) → `build\bin\partscaster.exe`; the whole package — `make dist-windows`.
Sound — through WPF MediaPlayer (PowerShell). Worker — `partscaster-worker.exe`, WSL2 or a separate machine:
the app needs only HTTP access to it.

**macOS:** the package builds (`make dist-mac`, WKWebView), but it was not tested. The app player on macOS is
the same `pw-play` as on Linux; *not verified: on a Mac without PipeWire, sound in the app most likely
will not work.* The worker needs NVIDIA, so you cannot install it on a Mac — only use the address of a worker on another machine.
