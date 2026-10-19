# MCP server (partscaster-mcp)

**English** · [Русский](../ru/mcp.md)

`partscaster-mcp` lets an AI agent (Claude Code, Claude Desktop, Codex and others) control Partscaster the same
way a person does in the app: queue tracks, edit them in the studio, apply effects and presets, configure and
install the worker. It works over the [Model Context Protocol](https://modelcontextprotocol.io) via stdio and
uses the same worker HTTP API ([api.md](api.md)). This file is for people who connect an agent and for people
who add tools.

## Build

```bash
make mcp          # → build/bin/partscaster-mcp
make mcp-data     # regenerate the library data from frontend/src
```

The MCP server is a separate program: changes in `internal/` or `cmd/partscaster-mcp` start to work only
after `make mcp` and reconnecting the client (in Claude Code — `/mcp`).

`make mcp` embeds a fingerprint of the sources into the program. On the first call and then once every 30 minutes
MCP compares it with the repository. If the code is already different, the tool response gets the note
"⚠ MCP server is out of date… make mcp and reconnect" ("⚠ MCP-сервер устарел… make mcp и переподключите").
Without the repository nearby (the program is installed separately) there is no check.

`make mcp` and `make release` also embed the build version (`git describe`). It is visible in
`serverInfo.version` when connecting. MCP also compares the worker version (`GET /version`) with its own once
every 30 minutes: if the worker is older, responses get the note "⚠ … update the worker" ("⚠ … обновите
воркер"); if newer, "update the app" ("обновите приложение"). The worker version is in the output of `status`
and `doctor`.

## Connecting

The simplest way is from the app: "Settings" → "For AI agents" shows ready snippets for each client with the
real path to `partscaster-mcp` (it lies next to the app) and the current worker address. The button
"Connect to Claude Code" runs `claude mcp add partscaster --scope user …` (the server is visible in all
projects) and also removes the old `yue-studio` registration, if there was one.

Environment variables:

| Variable | Meaning |
|---|---|
| `PARTSCASTER_URL` | Worker address; not set — the address from the app settings, without them `http://localhost:8091` |
| `PARTSCASTER_MCP_DIR` | Where to download files (tracks, scores, spectrograms); default `~/partscaster-mcp-downloads` |

### Claude Code

```bash
claude mcp add partscaster --scope user -e PARTSCASTER_URL=http://gpu-host:8091 -- /path/to/partscaster-mcp
```

For a clone of the repository, so that the worker address does not get into shared settings, use `--scope local`:

```bash
make mcp
claude mcp add partscaster --scope local -e PARTSCASTER_URL=http://gpu-host:8091 -- "$PWD/build/bin/partscaster-mcp"
```

### Claude Desktop

`claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "partscaster": {
      "command": "/path/to/partscaster-mcp",
      "env": { "PARTSCASTER_URL": "http://gpu-host:8091" }
    }
  }
}
```

### Codex (`~/.codex/config.toml`)

```toml
[mcp_servers.partscaster]
command = "/path/to/partscaster-mcp"
env = { PARTSCASTER_URL = "http://gpu-host:8091" }
```

## Confirmation by a person

Tools marked **†** cannot be undone: they require `confirm=true`, and the agent must ask the user first. These
are `cancel`, `delete_job`, `queue_cancel`, `dsp_variant_delete`, `sound_preset_delete`, `voice_delete`.

Three more tools without `confirm` only show a plan, and act with `confirm=true` — also after asking the user:
`recipe_import` (without `confirm` — an import preview), `install_worker` and `install_app` (a dry run by
default; installation — `dry_run=false` and `confirm=true`).

## Tools

76 in total. In the "Parameters" column, required ones are in **bold**.

### Worker queue

The same queue that a person sees in the app. Long tools (generations, `plan`, `job_preview`, `make_stems`,
`rebuild_sections`, `sound_preset_apply`, `revoice_apply`, lyrics and note recognition, the
effects engine with an amp, etc.) enter it with the "AI" mark and wait for their turn. If a person canceled the
operation, the tool returns the error "canceled by a person" ("отменено человеком").

| Tool | Parameters | What it does |
|---|---|---|
| `queue_list` | — | The running operation, the waiting ones in order, the last finished ones, pause. An entry has: `id`, `kind` (generate, preset, rebuild, stems, fx, transcribe, whisper, voice, plan, preview), `title`, `job_id`, `source` (`app` — a person, `mcp` — AI), `state`, `result_job_id`, `error`; preset progress: `stage` (prepare, parts, levels, render, verify, fix), `pass` (build number), `eta_s` (estimate of the time left, s; `null` — not yet), `stage_since` (s since the step began) |
| `queue_move` | **id**, **dir** | Move a waiting entry: `dir` −1 — earlier, 1 — later |
| `queue_cancel` † | **id**, **confirm** | A waiting one — remove, a running one — stop |
| `queue_pause` | **paused** | Pause: nothing new starts, the running one is finished; `false` — remove the pause |

### Tracks and generation

| Tool | Parameters | What it does |
|---|---|---|
| `status` | — | Is the worker available, is the model loaded, worker version |
| `jobs` | folder, limit, brief | List of tracks. `folder` — a folder ("Album"…; `"-"` — without a folder), `limit` — the last N, `brief` — short lines without style, lyrics and plan |
| `job_update` | **job_id**, title, folder, parent_id | Track title and folder (versions follow the root); `folder=""` — remove from the folder; `parent_id` — move under another track (a derived version, folded under it) |
| `submit` | **style**, **lyrics**, title, seed, n, cot, draft, arc, max_tokens, temperature, cfg, parent_id, role, sound_preset_ids | Queue a track. `n` — a fan of 1–10 tracks with seeds base+0…n−1; `temperature` 0.5–1.5 (0 — default 1.0); `cfg` 1–4 (0 — default 1.5); the rest — as for `POST /jobs` ([api.md](api.md)). Instrumental — `lyrics='[Instrumental]'` |
| `plan` | **style**, **lyrics**, seed | The track plan (ABC) without audio — look at and fix the notes before generation. Slow |
| `render_abc` | **abc**, **style**, lyrics, title, seed, parent_id, role | A track from your own score. With `parent_id` and `role=section` — a part for splicing in under the parent; `seed=0` — the parent's seed |
| `cancel` † | **job_id**, **confirm** | Remove a track from the queue or stop the generation |
| `retry_job` | **job_id** | A failed or canceled track goes back to the queue with the same parameters |
| `delete_job` † | **job_id**, **confirm** | Delete a track with all its files |
| `artifacts` | **job_id**, file | Download a track file to the downloads directory and return the path. `file` — a name in the track folder (`audio.flac`, `audio.mp3`, `audio.wav`, `score.abc`, `request.abc`…); empty — mp3 if there is one, otherwise the main audio |
| `job_score` | **job_id** | Score timeline: bars × voices, chords, sections, seconds |
| `job_preview` | **job_id**, **from_sec**, **to_sec** | A quick part of the track from the saved latents (no new generation), path to the file |

### Lyrics, transcription, import

| Tool | Parameters | What it does |
|---|---|---|
| `transcribe` | **path** | An audio file from the PC → ABC score (SheetSage2) for a cover |
| `transcribe_job` | **job_id** | Take the score from a finished track that has none (an import without transcription, an effect variant). Does not overwrite an existing one |
| `job_lyrics` | **job_id**, language | What a finished track sings (faster-whisper) — a check for the agent: whether the given lyrics were sung; if there is a vocal stem, it is recognized. `language` — `en`, `ru`, … or `auto`; empty — by the track style |
| `import_track` | **path**, title, folder, transcribe | Your own track from the PC as a library track; `transcribe=true` — take the score right away (slow) |

### Layer over the model

What the YuE model itself does not have: the same mechanisms as in the app studio.

| Tool | Parameters | What it does |
|---|---|---|
| `continue_job` | **job_id**, **from_sec**, seed, abc, style_add, to_sec | "redo from here": the track up to `from_sec` + a continuation by the model, without a splice. `abc` — a changed plan, `style_add` — what to change in the sound, `to_sec` — only a part up to this point (to check an edit in context) |
| `set_head` | **job_id**, **head_id** | Main version of the song: `job_id` — the root, `head_id` — a descendant version or 0 (the track itself) |
| `variant_track` | **job_id**, **file**, title, voice_src | Effect variant → a separate version track; `voice_src` — whose voice is put in (after "re-sing") |
| `rebuild_sections` | **job_id**, **specs**, as_track, title, voice_src | Rebuild the track's stems from the clean original, as in the studio. A `specs` entry: `{child_id, from, to, stems, …}` — splice in a part from the render `child_id`; `child_id 0` + `db` — stem volume (−100 — mute); `revoice` — "re-sing"; `chain`/`params`/`steps` — an ffmpeg effect on the stems in the window; `envelope` — a volume line; `engine` — an engine chain; `add` — a part over the track; `place` — position in stereo; `master` — mastering; `mix` with `chain`/`steps`/`envelope` — an effect on the whole assembled mix (after the stem edits, before the master); `preset` — a preset record from `sound_preset_draft` (the build prepares and expands it, `skip_sound`/`skip_place`/`skip_master` — what your own records override); `id` — a record tag: a build failure on it comes back with " {edit:<id>}" in the error, the preset's warnings — in `warnings` of the response. `as_track` — make it a version track right away |
| `revoice_start` | **job_id**, **from**, **to**, abc, takes | "re-sing from here", step 1: the voice of a part is sung again from the version's voice source; `takes` — 1–4 takes (2); `abc` — a changed melody (not above the voice ceiling). The response is the take ids |
| `revoice_apply` | **job_id**, **take_id**, **from**, **to**, beat_sec, title | Step 2: the voice of a finished take is put only into the window `from`–`to` → a new version with `voice_src` = the take |
| `splice` | **job_id**, **parts**, crossfade, title | Join parts of versions (`{job_id, from, to, gain_db?}`) into a new version with a `crossfade` transition (0.05 s): bring back a cut instrumental break, assemble the best takes. Cut at bar boundaries of one performance |
| `plan_check` | **job_id**, **abc**, from | What changed in the plan: bars, time, voice ceiling (top of the melody + 2 steps), edits before the `from` point (the continuation will not play them). `continue_job` and `revoice_start` with `abc` do the same |
| `vocal_contour` | **job_id**, from, to | Voice pitch by the bars of the plan — is it sung as planned, did the voice go into a squeal. Needs `make_stems` |
| `find_tones` | **job_id**, from, to, stem | A narrow "whistle" in the window — frequencies for the `dewhistle` chain ("Remove whistle", "Убрать свист") |
| `chord_grid` | **job_id** | Chords and sections of the plan by the bars of the audio `{bpm, bars}` — the base for a synth part. No plan — run `transcribe_job` first |
| `beat_grid` | **job_id**, from, to | Tempo and downbeat in the window — `bpm` and `offset` for effects in time (the `gate` chain, "Rhythm gate", "Ритм-гейт") and percussion |

### Track studio

| Tool | Parameters | What it does |
|---|---|---|
| `make_stems` | **job_id** | Split the track (BS-Roformer-SW + DrumSep, ~1.5 min per track): main stems drums/bass/other/vocals, detailed guitar/piano and drum parts kick/snare/toms/hh/ride/crash |
| `overdub` | **job_id**, **style**, lyrics, seed, gain, abc | A part over the track from its score with a new style. `gain` 0.1–1 (0.5); 0 — a cover as a new track without mixing. `abc` — your own plan for the part. Without `lyrics` — a vocalise |
| `guitar_part` | **job_id**, style | What the guitarist will play "from the plan chords" (block `gtrnotes`): bars, strikes with notes. `style` 0–4: auto, strumming, fingerpicking, bass + chord, fifths |
| `voice_convert` | **job_id**, **ref_job_id**, ref_from, ref_dur, steps, title | EXPERIMENT "Album voice": the track's voice with the timbre of a sample (Seed-VC on the worker). The voice is recognizable but it wobbles — see [deployment.md](deployment.md). Without Seed-VC — error 503 |
| `voice_preview` | **job_id**, **ref_job_id**, **from**, **to**, ref_from, ref_dur, steps | EXPERIMENT "Album voice", listen to a piece: the voice of the `[from, to)` piece of the track (3–30 s) with the sample's timbre → the file `preview-vc-*.flac` in the track folder; it does not create a version |
| `analyze_job` | **job_id** | Track measurements: tempo, crest factor, dynamics, spectrum — to compare variants |
| `job_peaks` | **job_id**, file, bins | Loudness envelope as numbers: find a drop, an entry, a place to edit |
| `job_spectrum` | **job_id**, file | Spectrogram PNG to the downloads directory. Needs ffmpeg on the worker |
| `draft_list` | — | Studio drafts (unfinished work) of all tracks: `[{job_id, state, updated_at}]` |
| `draft_get` | **job_id** | The track's draft: `state` — `{v, edits, queue, ts, gen?, applying?}` ("Track changes", pending renders, changes that need a render — voice/plan tricks/no vocals, the "apply all" progress); none — empty |
| `draft_set` | **job_id**, **state** | Write the track's draft (a JSON object). The studio picks it up on its next start |
| `draft_clear` | **job_id**, **confirm** | Delete the track's draft — the studio edits are lost. Only with `confirm=true` after the human agrees |

### ffmpeg effects (on the PC)

What each chain does — [effects.md](effects.md).

| Tool | Parameters | What it does |
|---|---|---|
| `dsp_chains` | — | Effect chains, their knobs and ranges. `group` — group (as the effects.md sections: space, pedals, modulation, filter, dynamics, time, lofi, voice); `voice=true` — voice effects (megaphone, telephone, drive, slapback); `for_voice=true` — a general effect that also suits a voice; chains with `key` (`ducking`, `key=drums`) — only on a stem |
| `dsp_presets` | — | Ready pedal sets: `steps` for preview and applying |
| `dsp_apply` | **job_id**, chain, params, steps, stem, from, to, db | Apply a chain (`chain` + `params`) or pedals in order (`steps`). Without `stem` — a variant for the whole track (pedals — `dsp-pedals.flac`); with `stem` — only this stem in the window `from`–`to` through a rebuild, `db` — on top of the leveling. For voice chains the volume is leveled to the source; reverb and delay sound as a tail after `to` |
| `dsp_preview` | **job_id**, chain, params, steps, stem, solo, from, to | Listen to an effect on a part (default 20–35 s + a tail up to 3 s) → variant `dsp-preview-*.flac`; `solo` — only the processed stem |
| `dsp_variants` | **job_id** | Track variants with measurements and label |
| `dsp_variant_delete` † | **job_id**, **file**, **confirm** | Delete a variant (`dsp-*` / `overdub-*`); stems are not deleted |
| `volume_envelope` | **job_id**, **points**, stem | A volume line `[{t, db}]` (−30…+12 dB, up to 200 points) on the whole track (`dsp-envelope.flac`) or a stem |

### Sound engine (on the worker)

A chain of blocks: pedals → NAM amp → cabinet → space, plus samplers, synth, percussion, mastering — 24 blocks,
described in [effects.md](effects.md). Processing does not shift the audio — the result is fit for splicing in
note for note.

| Tool | Parameters | What it does |
|---|---|---|
| `fx_apply` | **job_id**, **chain**, source, from, to, output, label, preview, fade, add, file, in_place | A chain on the track (`source=mix`) or a stem → variant `dsp-fx-*.flac`. `output` — `mix` or `solo`; `preview` — only listen to the part `from`–`to`; `add` — a part over the track; `file` — input from a mix variant (this is how mastering is applied), `in_place` — write into the same file. Fields — as for `POST /jobs/{id}/fx` ([api.md](api.md)) |
| `fx_blocks` | — | Parameters of each block: defaults, bounds, step, labels |
| `fx_presets` | — | Ready chains of the "Instruments" page `{id, name, note, stems, group, amp_hint, chain}`. Empty `amp.model` — put in the first capture from `fx_assets` whose name contains a substring from `amp_hint` |
| `fx_assets` | — | Uploaded NAM captures, IRs and sample kits |
| `fx_asset_upload` | **kind**, **path** | Upload your own `.nam` (`kind=amp`) or `.wav` (`kind=ir`) from the PC |
| `fx_kit_install` | **name** | Install a sample kit on the worker (catalog — in [api.md](api.md#sample-kit-catalog)): drums `osdk`, Karoryfer basses, orchestra `vsco-*`, piano `salamander`, guitars for `gtrnotes`; drum machines `tr808`/`tr909`/`linn`/`cr78`/`simmons` and `synthbass` are synthesized without the network |
| `fx_kit_progress` | — | Progress of the kit installation in progress |
| `fx_phrase` | phrase, tempo, chain, stems, bypass | Phrases without a track: without `phrase` — the catalog (guitar as recordings, bass and drums as notes); with `phrase` — a loop through the chain, file `phrase-*.wav`. `tempo` 0.5…1.5 |
| `fx_instruments` | action, id, name, base, group, stems, chain, extra | Your own instruments based on ready ones: `action` — `list` (default), `create`, `update`, `delete`. In the app — next to the ready ones |

### Sound presets

A preset is a recipe for processing the whole track: stem edits, a final stage on the mix, mastering → a version
"track · preset".

| Tool | Parameters | What it does |
|---|---|---|
| `sound_presets` | id | Without `id` — a short list (more than a hundred built-in, `family`: Рок (Rock), Тяжёлое (Heavy), Электроника (Electronic), Поп и другое (Pop and other); the subgenre is the name before " · "); with `id` — the full recipe |
| `sound_preset_create` | **name**, note, specs, final, master, parts, target_lufs, reference_job_id | Your own preset. Fields — as in the preset body ([api.md](api.md#preset-body)); at least one of `specs`, `final`, `master`, `target_lufs` is needed |
| `sound_preset_update` | **preset_id**, **name**, note, specs, final, master, parts, target_lufs, reference_job_id | Replace your own preset; `master`, `parts`, `target_lufs` that are not passed stay the same (`[]` / `null` — remove). A built-in one — not allowed |
| `sound_preset_delete` † | **preset_id**, **confirm** | Delete your own preset |
| `sound_preset_draft` | **preset_id**, db | Preset into the studio draft at once: a "Preset" record carrying the whole recipe, with no preparation and no builds; the track is not changed → `{"edit": record}`. Preparation (stems, parts, sample kits, measurement) and the balance check happen when the draft is built (`rebuild_sections` / "▶ mix" and "apply all" in the studio). Put the record first in the draft's `state.edits` (`draft_get` → `draft_set`, drop the earlier record with `preset`) |
| `sound_preset_apply` | **job_id**, **preset_id**, db | Apply now (synchronous, minutes): splits the track, installs missing kits, rebuilds, the final stage — on the result → `{version_id, file, warnings?}`. `db` — volume of entries for this time: `{"index": dB}` |

A preset can also be ordered when creating a track — `submit` with `sound_preset_ids` (up to 3): the open app
applies them when the track is ready. Not allowed for a draft.

### Track recipe

Moving a song to another installation without audio.

| Tool | Parameters | What it does |
|---|---|---|
| `recipe_export` | job_id, path | The track's song (from the root) → file `*.partscaster.json` to the downloads directory or `path` (the older `*.yue-track.json` is also accepted); `warnings` — what was not included |
| `recipe_import` | path, mode, confirm | Without `confirm` — a preview (title, steps, model, warnings); `confirm=true` — start. `mode`: `seed` (the same seed, default) or `plan` (by notes — if the worker model is a different version) |
| `recipe_imports` | retry, dismiss | Progress of imports; `retry=<id>` — continue a failed one from the same step, `dismiss=<id>` — remove from the list |

MCP itself runs the import while it is running (or the open app does): generations again, edits in order. The
performance comes out very close, but not the same; sound edits are repeated exactly. The app and MCP write the
edit log after each successful change.

### Voice lab voices

| Tool | Parameters | What it does |
|---|---|---|
| `voices_list` | — | Voice cards: character knobs, audition seed, whether the source track still exists |
| `voice_create` | **name**, **job_id**, params, seed | Save a voice card from an audition track; `params` — JSON of knobs `{register, rough, creak, delivery, breath, extra}` |
| `voice_delete` † | **voice_id**, **confirm** | Delete the card and the audio copy |

### Style library

| Tool | Parameters | What it does |
|---|---|---|
| `styles` | group, search | Style groups (built-in and your own) and search — ready style strings for `submit` |
| `slot_options` | slot, search | Russian phrase → English YuE tag by field: genre, rhythm, guitars, keys, vocals, mood, production, language |

### Settings

| Tool | Parameters | What it does |
|---|---|---|
| `config_get` | — | Worker address, worker data paths, generation model (chosen and recommended for the GPU) and worker engines |
| `config_set` | server_url, stems_auto, fx_engine, cpp_model, cpp_low_vram | `server_url` is also saved to the app settings; `stems_auto` — when to make stems: `studio` (when the studio opens) / `always` (right away for every finished track) / `manual`; `fx_engine` — effects engine on/off; `cpp_model` — generation model (any downloaded one: `bf16`, `q8`, `q6`, `q5`; the one recommended for the card is in `config_get`; the same seed on another model gives a different performance), `cpp_low_vram` — low-VRAM mode; model and mode — from the next generation |
| `engine_model_install` | **name** | Download a yue2.cpp model: `bf16` (7.2 GB), `q8` (3.8 GB), `q6` (2.9 GB), `q5` (2.6 GB). Waits for the download to finish; then — `config_set cpp_model=<name>` |

### Installation and diagnostics

| Tool | Parameters | What it does |
|---|---|---|
| `doctor` | host | Environment check: is the worker available and which components it has (including whether stem separation is installed — `roformer_available`), ffmpeg, Go, Node; on the machine with the graphics card (`host` — over ssh) — GPU and video memory, disk space. Worker version |
| `install_worker` | host, archive, dry_run, confirm, whisper, seedvc, cpp_model, hf_home, seedvc_dir, sep_dir | Install the worker (Ubuntu + NVIDIA/CUDA). A dry run by default: the agent shows the plan and asks whether whisper (~3.2 GB) and Seed-VC (~11 GB) are needed. RoFormer stem separation (~7.3 GB, non-commercial weights) is always installed, together with the worker. Installation — `dry_run=false`, `confirm=true` and explicit `whisper`/`seedvc`. GPU and disk space are checked before installation. Generation model — `cpp_model=auto|bf16|q8|q6|q5` (without it — the one chosen earlier, if downloaded, otherwise the recommended one by VRAM). Default directories: `hf_home` — `~/yue/hf-cache`, `seedvc_dir` — `~/partscaster/seedvc`, `sep_dir` — `~/sep-venv` and `~/sep-models` (or the paths already written in `worker.env`); `sep_dir` is passed to `install.sh --sep-dir` |
| `install_app` | dry_run, confirm | Build the app on this PC (`wails build`; needs Go, Node, wails CLI). A dry run by default |

## Work recipes

**Minus without vocals.** "Queue a track in the style "70s analog reggae, 75 BPM" as an instrumental" → `submit`;
the agent waits with `jobs` until the track becomes `done`; "download the mp3" → `artifacts` → path to the file;
"make a minus without vocals" → `make_stems` + `rebuild_sections` with the `vocals` stem muted → `artifacts`.

**Synth by chords.** `chord_grid` (chords by the bars of the audio) → a `synth` block with notes in track seconds →
`rebuild_sections` with the entry `{child_id: 0, stems: ["mix"], add: true, engine: [synth…, effects…]}`.
To listen in advance — `fx_apply` with `source=mix`, `preview`, `add`. Ready synths — `fx_presets` with
`stems: ["synth"]`.

**Percussion by the grid.** Bars — `chord_grid` (if there is a plan) or `beat_grid` → a `perc` block with hits
`notes: [{t, d, vel}]` → `rebuild_sections` with an `add` entry, as for the synth. Ready ones — `fx_presets` with
`stems: ["perc"]` (each has `pattern` and `swing`).

**Effect on one stem.** `dsp_apply` with `stem` (and `from`/`to`, `db`) or a `rebuild_sections` entry with
`chain`/`params` — the other stems do not change. Chains with `key` (`ducking`) — only with `stem`. Into the track
through the engine — a `rebuild_sections` entry `{child_id: 0, stems, from, to, engine: [blocks]}`; edits
accumulate together with splices.

**Mixing and mastering.** Position in stereo — `place: {pan, width}`: on an entry with `add` — the position of the
part; as a separate entry `{child_id: 0, stems: [stem], place}` — the position of the stem. Mastering — the entry
`{child_id: 0, master: true, engine: [glue…, limiter {target_lufs, ceiling_db}]}` on the whole assembled mix;
the response is the loudness (LUFS) and the true peak. To listen to mastering on a finished mix — `fx_apply` with
`file` and `preview`, to write into the same file — `in_place`. Ready masters — `fx_presets` with
`stems: ["master"]`.

## Installing the worker through the agent

The worker is installed from the archive `worker-<version>.tar.gz`; its `install.sh` installs the base together with stem separation (see
[deployment.md](deployment.md)). The archive is the `archive` parameter; without it, MCP built from sources builds
it itself (`make worker-tar`), and a release MCP takes the archive that lies next to `partscaster-mcp`. MCP
installs optional components (whisper, Seed-VC) after the base.

- **Everything on one PC**: `install_worker` without `host` — unpacking and `install.sh` locally.
- **Graphics card on another machine**: `install_worker` with `host=user@gpu-host` — the archive is copied over
  scp, the rest goes over ssh (configured ssh access is needed).

## Development

- The server is `internal/mcp/` (JSON-RPC over stdio, no external dependencies), the entry point is
  `cmd/partscaster-mcp`.
- The client part reuses `internal/yue.Service` (stubs for tests are in `server_test.go`).
- Library data (`internal/mcp/*.json`) is generated from the frontend (`tools/mcp-gendata.sh`) and embedded
  via `go:embed`. **Do not edit it by hand** — edit the sources in `frontend/src/` and run `make mcp-data`.
- A new worker feature = a new `yue.Service` method + an MCP tool (parity of the app and MCP, see
  [AGENTS.md](../../AGENTS.md)).
