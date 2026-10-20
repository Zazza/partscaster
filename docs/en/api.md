# Worker HTTP API

**English** · [Русский](../ru/api.md)

The worker is a service on the machine with the graphics card. It generates tracks with the YuE2 model, splits
them into stems, processes audio and stores the library. The Partscaster app and the MCP server ([mcp.md](mcp.md))
work only through this API. This file is for people who write their own client or script, or who need to find out
what exactly went wrong.

## General

- Port: **8091**. By default the app connects to `http://localhost:8091`. If the worker is on a separate
  machine, set its address in the app settings (for example, `http://gpu-host:8091`).
- There is no authentication. Do not expose the worker to the internet (see "Security" in
  [README](../../README.md)).
- Responses are JSON unless stated otherwise. An error is an HTTP code and `{"detail": "reason"}`: 404 — no such
  track or file, 409 — the current state does not allow it (running, already exists, canceled by a person),
  422 — invalid request, 502 — an external service did not respond (GitHub), 503 — a component is not
  installed or is turned off.
- File upload: raw bytes in the request body, the file name in the `X-Filename` header.
- In the API a track is called a **job** (`job`). `{id}` in paths is its number. Track files live in the track
  folder on the worker and are served via `GET /audio/{id}/{file}`.

### Shared queue

Everything that uses the graphics card runs one at a time, in order of place in the shared queue: generations,
`/plan`, previews, stem splitting, lyrics recognition (whisper), note transcription (SheetSage), "Album voice",
and the effects engine with an amp. The person sees the same list in the app.

Queue entry: `{id, kind, title, job_id, source, state, pos, result_job_id, error, cancel_requested,
client, created_at, started_at, finished_at}`; `source` is `app` / `mcp` / `other` (who queued it),
`state` is `waiting` / `running` / `done` / `error` / `canceled` / `interrupted`.

Request headers:

| Header | Meaning |
|---|---|
| `X-Partscaster-Source` | `app` or `mcp` — who queues the operation (otherwise `other`) |
| `X-Partscaster-Op` | id of the client operation (`POST /queue/ops`) that this request belongs to: a step of the chain does not enter the queue a second time. Unknown id or another client's operation → 409 |

If a person canceled the operation, a heavy step responds with 409 "canceled by a person" ("отменено человеком").

## Service and settings

| Method | Path | What it does |
|---|---|---|
| GET | `/health` | `{status, model_loaded}` — is the worker alive, is the yue2.cpp server with the model running |
| GET | `/version` | `{version, api, source_hash, model, engine, components: {whisper, seedvc, roformer}, stems_model}`: build version (the `VERSION` file of the archive, without it `dev`), API version (clients compare it with their own), code fingerprint, generation model (GGUF file and yue2.cpp commit), the label of the current model `cpp-<model>` (`cpp-bf16`, `cpp-q8` etc.), what is installed; `stems_model` — always `bs-roformer-sw` |
| GET | `/config` | Current settings: `data_dir`, `whisper_py`, `whisper_available`, `seedvc_dir`, `seedvc_available`, `stems_auto`, `auto_stems_instrumental`, `roformer_available` (whether stem separation is installed), `fx_engine`, `fx_preview`, `fx_phrases`, `cpp_model`, `cpp_low_vram`, `cpp_available` (the yue2.cpp program runs), `cpp_models` `{name: {file, size, installed}}`, `cpp_recommended` `{model, low_vram, vram_mib}` — the recommended model for the first GPU (`vram_mib` 0 — amount unknown); only a hint, it selects nothing by itself |
| POST | `/config` | Changes the given fields, responds like GET. Fields: `stems_auto` — when to make stems: `studio` (when the studio opens, default; the client splits), `always` (right away for every finished full track and import — a separate queue operation after the generation; drafts and insert pieces — no), `manual` (other values — 422); `auto_stems_instrumental` — for tracks without vocals, make stems right away and check whether a voice slipped in (yes by default); `fx_engine` — effects engine on/off; `cpp_model` — generation model `bf16` / `q8` / `q6` / `q5` (any downloaded one; not downloaded or unknown — 422), `cpp_low_vram` — low-VRAM mode. A model change applies from the next generation. The `engine` and `stems_model` fields from old clients are ignored |
| GET | `/stats` | Summary for the status bar: `{model_loaded, vram_total, vram_used (MB), queue: {queued, running}, running: {job_id, title, stage, progress_pct, tok_per_s}, gpu_waiting}`; `model_loaded` — the yue2.cpp server is running, VRAM — of the first card by `nvidia-smi` |
| POST | `/engine/models/install?name=` | Download a yue2.cpp model and the VAE (`bf16` / `q8` / `q6` / `q5`), pinned files with a sha256 check; an already downloaded one — no network. Waits for the download to finish → `{name, installed, cpp_models}`. Unknown name — 422, checksum mismatch or network failure — 502 |
| GET | `/engine/models/progress` | The model download in progress `{name, file, done, total}` or `{}` |

## Queue

| Method | Path | What it does |
|---|---|---|
| GET | `/queue` | `{paused, items}`: the running operation, the waiting ones in order, the last finished ones (up to 20) |
| POST | `/queue/ops` | `{kind, title, job_id}` → entry. A client operation is a chain of steps of the app or MCP (preset, rebuild). The client must check in: without `/beat` for longer than 180 s the operation becomes `interrupted` |
| POST | `/queue/ops/{id}/beat` | "Alive" check-in → entry (shows `state` and `cancel_requested`); unknown — 404. Optional body `{stage, pass, eta_s}` — the operation's progress: `stage` — `prepare`, `parts`, `levels`, `render`, `verify` or `fix` (anything else — 422), `pass` — build number ≥ 1, `eta_s` — estimate of the time left, s (≥ 0 or `null`). Without a body the last progress stays. The entry returns `stage`, `pass`, `eta_s` and `stage_since` (s since the step began); an entry without progress has `""`/`null` |
| POST | `/queue/ops/{id}/finish` | `{state: done/error/canceled, result_job_id, error}` → entry; not a client operation or already finished — 409 |
| POST | `/queue/ops/{id}/move` | `{dir: -1 or 1}` → `{moved}`: one place earlier or later; at the edge `moved=false`; not waiting — 409 |
| POST | `/queue/ops/{id}/cancel` | → `{canceled, running}`: a waiting one is removed at once; a running one gets a cancel mark, its external processes (RoFormer, Seed-VC, whisper) stop, a step inside the worker is finished |
| POST | `/queue/pause` | `{paused}` → `{paused}`: nothing new starts, the running one is finished. The pause lasts until the worker restarts |

## Generation and tracks

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs` | Queue a track → `{id}`. The body is in the table below |
| POST | `/plan` | `{style, lyrics, seed, cot}` → `{abc, truncated, seconds, seed}` — only the plan (ABC score) without audio. Waits for its turn at the graphics card. `cot=off` — 422 |
| GET | `/jobs` | List of tracks, newest first, no more than 2000. No filters |
| GET | `/jobs/{id}` | One track. Fields are below |
| PATCH | `/jobs/{id}` | `{title?, folder?, parent_id?}` → track: title (not empty, up to 200 characters), folder (up to 60; `""` — remove from the folder) and parent (`parent_id` — an existing track, not this one; otherwise 422). A field that is not passed does not change |
| DELETE | `/jobs/{id}` | Delete the track and its folder → `{deleted}`; a waiting generation is removed from the queue; a running track — 409 (cancel it first) |
| POST | `/jobs/{id}/cancel` | Queued — remove; running — stop the generation at the nearest step → `{canceled, running?}` |
| POST | `/jobs/{id}/retry` | A failed or canceled track goes back to the queue with the same parameters → `{retried}`; another status — 409 |
| POST | `/jobs/{id}/continue` | "redo from here" → `{id}`. Body `{from_sec, seed?, abc?, style_add?, to_sec?, title?}`: new track = the source's model steps up to `from_sec` + continuation (another seed — another take; `abc` — a changed plan; `style_add` — an addition to the style: what to change in the sound). With `to_sec` — a "part" (`role=fragment`): the model plays only up to `to_sec` (not past the end of the track) + 2 s of tail. The new track is attached to the source (`parent_id`, `role=continue`, `cont_from`). 422 — the track has no model steps (import) or `from_sec` is past the end |
| POST | `/jobs/{id}/head` | `{head_id?}` — the main version of the song: `{id}` is the root, `head_id` is the root itself or its descendant (otherwise 422); `null`/0 — the track itself is main again |
| POST | `/jobs/{id}/overdub` | `{style, lyrics?, seed?, gain?, abc?}` → `{id}` — a part over the track: a new render from the track's score (or from your own plan `abc` — then the instrument sounds only in the selected part) with a new style. The finished part is mixed into the original at volume `gain` (0.5 by default) into the file `overdub-<id>.flac`; `gain=0` — no mixing (a cover as a new track). The parent's seed gives the same interpretation. Without `lyrics` the model sings a vocalise. No score — 404 |
| POST | `/jobs/{id}/variant_track` | `{file, title?, voice_src?}` → `{id}` — an effect variant (`dsp-*.flac` and others) as a separate finished track: a copy of the audio and the source's score, after that stems and effects work (`parent_id` = source, `role=variant`). `voice_src` — the track whose voice is put into the version (after "re-sing"); a non-existent one — 422 |
| POST | `/jobs/{id}/voice` | EXPERIMENT "Album voice": `{ref_job_id, ref_from?, ref_dur?, steps?, title?}` → `{id}` — a new version where the voice is sung with the timbre of a sample (Seed-VC), the music and melody stay the same. The sample is a window of the vocal stem of track `ref_job_id`: `ref_from` (not set — the densest part of singing), `ref_dur` 3–30 s (25), `steps` 10–100 (50). The voice is recognizable but it wobbles. Seed-VC not installed — 503 |
| POST | `/jobs/{id}/voice_preview` | EXPERIMENT "Album voice", listen to a piece: `{ref_job_id, from, to, ref_from?, ref_dur?, steps?}` → `{file, from, to}` — the same voice conversion with the sample's timbre (Seed-VC), but only on the `[from, to)` piece of the track (3–30 s, otherwise 422); the file `preview-vc-*.flac` in the track folder (the last 3 are kept). It goes through the GPU queue and answers when the piece is ready. No Seed-VC — 503, track or sample not done — 404 |
| POST | `/tracks/import?transcribe=&title=` | Import your own track (bytes + `X-Filename`) → `{id, duration_sec, abc_file, transcribe_error}`. `transcribe` (yes by default) — take the score right away for the piano roll and overdub; its failure does not cancel the track |
| POST | `/jobs/{id}/preview` | `{from_sec, to_sec}` (not shorter than 1 s) → `{file, from_sec, to_s, duration_sec}` — a quick part of the track from the saved latents (no new generation), file `preview-<from>-<to>.flac` |
| GET | `/jobs/{id}/previews` | Names of the track's `preview-*.flac`, newest first |
| POST | `/jobs/{id}/mp3` | Make `audio.mp3` (320 kbit/s) if it does not exist (imported tracks) → `{file}` |

### POST /jobs body

| Field | Meaning |
|---|---|
| `style` | Style string — English YuE tags (required) |
| `lyrics` | Lyrics with sections `[Verse]`, `[Chorus]`… or `[Instrumental]` (required) |
| `title` | Title; empty — "Untitled HH:MM" |
| `seed` | Seed; empty or 0 — random, it is written into the track. With `parent_id` — the parent's seed (for an old parent without a seed — 831001) |
| `cot` | Plan mode: `full` (default) / `melody` / `off` |
| `abc` | Your own plan (ABC). With `cot=off` — 422 |
| `draft` | Draft of ~15–20 s: hear the style before the full render |
| `arc` | Dramatic shape over the plan: `build` (rise), `wave` (wave), `burst` (burst, voice an octave higher in the finale). Works with your own `abc` too; with `cot=off` — 422 |
| `max_tokens` | Length ceiling (~25 tokens/s), not the length itself: at least 6000 for a full track (no-lyrics too); the model decides the song length by its plan (3–5 min) and ignores the number of sections; 0 — the worker's budget |
| `temperature` | Boldness of playing: 0 (default 1.0) or 0.5–1.5 |
| `cfg` | Precision to style and notes: 0 (default 1.5, variable `YUE2_CFG_SCALE`) or 1–4 |
| `parent_id`, `role` | A derived track: from which track and why — `section` (a part for splicing in), `rebuild`, `fragment`, `variant`. In the app it is hidden under the parent. Zero `temperature`/`cfg` and an empty seed are taken from the parent. A continuation is created only via `/continue` |
| `sound_preset_ids` | Up to 3 different sound presets: the app applies them when the track is ready. With `draft` — 422 |

### Track fields (GET /jobs)

Main: `id`, `title`, `status` (`queued` / `running` / `done` / `error` / `canceled`), `style`, `lyrics`,
`seed`, `cot`, `draft`, `arc`, `temperature`, `cfg`, `duration_sec`, `audio_file`, `mp3_file`, `wav_file`,
`abc_file`, `error`, `folder`, `created_at`, `finished_at`.

| Field | Meaning |
|---|---|
| `parent_id`, `role`, `cont_from` | Derived track and the continuation point |
| `head_id` | Main version of the song (on the root) |
| `voice_src` | The track whose voice is put into the version; empty — its own voice |
| `req_abc` | The track was rendered from its own plan (`request.abc`) |
| `engine` | Which model made it: `cpp-bf16` / `cpp-q8` / `cpp-q6` / `cpp-q5`; old tracks of the former PyTorch engine have `torch` or empty. A seed repeats the performance only on the same model |
| `has_draft` | The track has a studio draft — unfinished work (see "Studio draft") |
| `mixes` | Number of finished studio mixes (`overdub-inst-*.flac`: splices, effects on stems) |
| `sound_presets` | `[{id, status: pending/running/done/error, child_id, error}]` |
| `queue_pos`, `source` | Place in the queue and who queued it |
| `vocal_leak` | Flag "a voice is heard in a track without vocals" |
| `truncated` | Generation hit the budget even after one automatic retry (×1.6): the ending may be missing |
| `stage`, `tokens`, `tok_per_s`, `elapsed_s`, `progress_pct` | Live progress — only on a running track |

## Track files

| Method | Path | What it does |
|---|---|---|
| GET | `/audio/{id}/{file}` | Any file from the track folder: `audio.flac` / `.mp3` / `.wav`, `score.abc`, `request.abc`, `stem-*.flac`, `dsp-*.flac`, `overdub-*.flac`, `preview-*.flac`, `metrics.json` etc. |
| GET | `/listen/{id}?f=` | HTML listening page; `f` — a variant (`dsp-*`, `overdub-*`, `preview-*`, `stem-*`) |

## Score and audio analysis

| Method | Path | What it does |
|---|---|---|
| GET | `/jobs/{id}/score` | Timeline from `score.abc`: bars × voices, chords, sections, loudness (RMS) per section |
| POST | `/jobs/{id}/plan_check` | `{abc, from_sec?}` — a changed plan against the track's plan: bars per voice before/after, length, changed bars (time, text before and after), voice ceiling (top of the melody + 2 steps), warnings. Empty `abc` — 422 |
| GET | `/jobs/{id}/chord_grid` | Chords and sections of the plan by the bars of the audio: `{bpm, bars: [{start, end, chord, section}]}` in track seconds (beats from the drums, if none — from the mix). No plan — 404, no beats — 422. For a synth part by chords |
| POST | `/jobs/{id}/grid` | `{from_sec, to_sec}` (0 — to the end) → `{bpm, offset, strength, source}` — tempo and the downbeat time in the window, from the drum stem (if there is one) or the mix; `strength` < 0.1 — in fact there is no grid. Window shorter than 4 s — 422 |
| POST | `/jobs/{id}/analyze?fresh=` | Track measurements: tempo, crest factor, dynamics, spectrum; cache `metrics.json`, `fresh=true` — recompute |
| GET | `/jobs/{id}/tones?from=&to=&stem=` | Narrow steady tones ("whistle", squeal) in the window — in the mix or in the stem `stem` (`vocals` / `drums` / `bass` / `other` / `guitar` / `piano`): `[{hz, prominence_db}]`, the most noticeable first, ≥ 12 dB above the surrounding ±300 Hz, no more than 5; `to=0` — to the end. No stem — 409, window outside the track — 422 |
| GET | `/jobs/{id}/vocal_contour?from=&to=` | Voice pitch by the bars of the plan (from the `vocals` stem): `{bars: [{index, start, end, notes[4]}], median_hz, low_hz, high_hz}`, notes per quarter of a bar ("D4", "·" — no voice). No vocal stem — 409 |
| GET | `/jobs/{id}/peaks?file=&bins=` | Loudness envelope (the studio waveform): `{file, bins, duration_sec, peaks: [[min, max], …]}`, amplitudes −1…1. `file` empty — the main track; `bins` empty — ~60 windows/s (500…20000 windows, cache `<file>.peaks.json`), explicit — 100…20000 without cache |
| GET | `/jobs/{id}/spectrum.png?file=` | Spectrogram PNG 1600×320, X axis — 0…duration (the same scale as the waveform); cache next to the file. Needs ffmpeg on the worker, none — 503 |
| GET | `/jobs/{id}/guitar_part?style=` | What the guitarist will play "from the plan chords" (block `gtrnotes`): `{style, bars, strikes: [{t, midi[]}], beats, tune_cents}`. `style` — 0 auto, 1 strumming, 2 fingerpicking, 3 bass + chord, 4 fifths; other values — 422. Needs the plan and a guitar stem; cache `guitar-part.json` |
| POST | `/jobs/{id}/transcribe` | Take the score (SheetSage2) from a finished track that has none → `{id, abc_file}`. The score already exists or the track is not ready — 409 |

## Lyrics (whisper)

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs/{id}/lyrics?language=` | What a finished track sings (faster-whisper) — a check for an AI agent (MCP `job_lyrics`). If there is a vocal stem, it is recognized (more precise than the mix). `language` — `en` / `ru` / … / `auto`; without it — by the first tag of the style ("English, …" → `en`) |

## Transcription of another track

| Method | Path | What it does |
|---|---|---|
| POST | `/transcribe` | A track (bytes + `X-Filename`) → `{id, abc, …}` — an ABC score (SheetSage2) for a cover. The result is saved |
| GET | `/transcribes` | The last 50 transcriptions |
| GET | `/transcribes/{tid}/{file}` | A transcription file |

## References

| Method | Path | What it does |
|---|---|---|
| POST | `/references` | A reference recording (bytes + `X-Filename`) → `{id, metrics}` — measurements to compare with tracks. Cannot be decoded — 422 |
| GET | `/references` | List of references `[{id, created_at, metrics, size}]`, newest first |

## Stems

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs/{id}/stems` | Split the track (BS-Roformer-SW + DrumSep) → `{stems, seconds, model}`, `model` — `bs-roformer-sw`. The main stems `stem-drums/bass/other/vocals.flac` add up to the track. Detailed ones — `guitar`, `piano` (inside "other") and drum parts `kick`, `snare`, `toms`, `hh`, `ride`, `crash` — are not part of the sum; a DrumSep failure — the main stems are there, the parts are not. The `?model=` parameter of old clients is ignored. If there is no separation environment or it crashed — 500 «разделение на дорожки не удалось: …» ("stem separation failed"), no stems. A repeated request while the track is already being split waits for that split and returns its result |
| GET | `/jobs/{id}/stems` | The track's stems `[{file, name, created_at, metrics}]` |

## Effect variants

A variant is a processed copy of the track in its folder (`dsp-*.flac`, `overdub-*.flac`), with measurements and
a label.

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs/{id}/dsp?label=` | Upload a variant from the app (bytes, `X-Filename` like `dsp-*.flac`); `label` up to 300 characters — what was done ("Перегруз голоса · голос", that is "Voice drive · vocals") → `{file, created_at, metrics, label}` |
| GET | `/jobs/{id}/dsp` | Variants `[{file, created_at, metrics, label}]`, newest first |
| DELETE | `/jobs/{id}/dsp/{file}` | Delete a variant and its measurements, the waveform and spectrum cache → `{deleted}`. `stem-*` stems cannot be deleted this way — 422 |

## Sound engine

Processing on the worker with a chain of blocks: pedals, NAM amp, cabinet, space, samplers, synth. The blocks
and their parameters are in [effects.md](effects.md) and in the MCP tool `fx_blocks`. There are 24 blocks: `gate`,
`eq`, `comp`, `drive`, `amp`, `cab`, `reverb`, `delay`, `gain`, `sampler`, `bass`, `synth`, `chorus`, `phaser`,
`flanger`, `tape`, `spring`, `tremolo`, `perc`, `glue`, `limiter`, `air`, `gtrnotes`, `level`.

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs/{id}/fx` | A chain on a track or stem → variant `dsp-fx-<stem>-<hash>.flac` (as for `/dsp`, plus `clipped`). The body is below. The audio is not shifted in time; the `amp` block goes through the queue. Invalid chain — 422 with a reason; engine turned off (`fx_engine` or `PARTSCASTER_FX_ENGINE=0`) — 503 |
| GET | `/fx/assets` | `{amps: [{name, latency}], irs: [{name, sr, seconds}], kits: [{name: "osdk/kick", samples}]}` — NAM amp captures, impulse responses (IR) and installed sample kits |
| POST | `/fx/assets?kind=amp/ir&name=` | Upload `.nam` (`kind=amp`) or `.wav` (`kind=ir`), bytes in the body, up to 50 MB → `{name, kind}`. Larger — 413, wrong format or name — 422 |
| POST | `/fx/kits/install?name=` | Install a sample kit from the catalog below → `{name, parts, downloaded}`. A repeat — no network, missing parts are downloaded. Unknown name — 422, network failure — 502 (no half-kit is left) |
| GET | `/fx/kits/progress` | The kit installation in progress `{name, part, done, total, bytes}` or `{}` |
| GET | `/fx/phrases` | Phrases of the "Instruments" page: `[{id, family: guitar/bass/drums/synth, name: {ru, en}, bpm, beats, chords, style, parts, cycle_sec}]`. Guitar — as recordings (GuitarSet, CC BY 4.0), bass and drums — as notes; `synth` — silence, the client places notes of `synth`/`perc` blocks by the bars of the loop |
| POST | `/fx/phrase` | `{phrase, tempo 0.5…1.5 (1), chain, stems (parts for the chain, empty — all), bypass}` → `{file, cycle_sec, clipped}` — a loop of the phrase through the chain, it loops without a seam (the tails of the loop end are at its start), loudness — by the dry phrase. `bypass` keeps only `synth`/`perc` blocks. The last 40 loops are cached. No phrase — 404, invalid tempo, chain or part — 422, engine turned off — 503 |
| GET | `/fx/phrase/files/{file}` | WAV 16-bit stereo of a loop (the name is from the `/fx/phrase` response) |
| GET | `/fx/instruments` | Your own instruments `[{id, name, base, group, stems, chain, extra, created_at}]` |
| POST | `/fx/instruments` | `{name 1…80, base (id of a ready chain), group ≤ 40, stems (not empty: stems, `synth`, `perc`), chain, extra {style, octave, pattern, swing, accent, amp_hint, place}}` → the created one; invalid — 422 |
| PUT | `/fx/instruments/{id}` | The passed fields are replaced → the updated one; no such one — 404 |
| DELETE | `/fx/instruments/{id}` | → `{ok}`; no such one — 404 |

### POST /jobs/{id}/fx body

| Field | Meaning |
|---|---|
| `source` | `mix` (default) or a stem: `vocals`, `drums`, `bass`, `other`, `guitar`, `piano`, `kick`, `snare`, `toms`, `hh`, `ride`, `crash`. If there are no stems, they are made |
| `chain` | Array of blocks `[{type, …parameters}]` in order; missing parameters — defaults |
| `from`, `to` | Window, s; outside the window the track does not change |
| `output` | `mix` — the track where the stem is replaced with the processed one; `solo` — only the processed stem |
| `label` | Variant label |
| `preview` | Only listen to a part (needs `from` and `to`): file `preview-fx-<hash>.flac` — the window plus a reverb/delay tail up to 3 s, without measurements, it does not go into variants; the same request — the same file without recomputing; up to 8 such files are kept per track → `{file, duration_sec, clipped}`. With `limiter` the loudness is computed over the whole input, then the window is cut |
| `fade` | Preview only: smooth edges of the window input, 0…0.5 s |
| `pad` | Preview only: the file starts from the track start (silence before `from`), so it can be inserted without a shift |
| `add` | Addition (synth part): `output=mix` — the track plus the processed audio, nothing is subtracted; `synth` notes are in track seconds, the worker shifts them by the window |
| `file` | The input is a track variant (`dsp-*.flac` or `overdub-*.flac`), not the source audio; only with `source=mix`. Invalid name — 422, no file — 404 |
| `in_place` | With `file`, not a preview: write the result into the same variant (" · мастер" is added to the label) — mastering on a finished studio mix |

### Sample kit catalog

Kits are not included in the package: the worker downloads them from GitHub at a pinned version. Drum machines
and synth basses are synthesized by the worker itself, without the network.

| Kit | What it is | Block |
|---|---|---|
| `osdk` | The Open Source Drum Kit: kick, snare, hi-hat (closed, half-closed, half-open), ride, crash, three toms | `sampler` |
| `tr808`, `tr909`, `linn`, `cr78`, `simmons` | Drum machines, synthesis: kick, snare, hh-closed, hh-open, ride, crash, tom-small/medium/large, clap, rim, cowbell | `sampler`, `perc` |
| `growlybass`, `swagbass`, `blackblue`, `meatbass`, `pastabass` | Bass guitars and double bass by Karoryfer (CC0) | `bass` |
| `synthbass` | Synth basses, synthesis: moog, sub808, acid | `bass` |
| `salamander` | Piano Salamander Grand Piano (CC-BY 3.0): `piano`, `piano-soft` | `synth` with `kit` |
| `vsco-violin`, `vsco-viola`, `vsco-cello`, `vsco-contrabass`, `vsco-harp`, `vsco-flute`, `vsco-oboe`, `vsco-clarinet`, `vsco-bassoon`, `vsco-trumpet`, `vsco-horn`, `vsco-trombone`, `vsco-tuba`, `vsco-mallets` | Orchestra VSCO-2 CE (CC0) | `synth` with `kit`, `bass` |
| `emilyguitar`, `shinyguitar`, `fsbsguitar`, `nylonguitar` | Guitars (electric direct, semi-acoustic, Fender, nylon) | `gtrnotes` |

## Sound presets

A preset is a recipe for processing the whole track: stem edits, a final stage on the mix, mastering. There are
109 built-in presets: three tuned on tracks (`transmission`, `sex-on-fire`, `live-rhythm`) and genre presets; the
`family` field is Рок (Rock), Тяжёлое (Heavy), Электроника (Electronic), Поп и другое (Pop and other). Built-in
presets come first, are updated from the code when the worker starts (the id stays the same) and cannot be
changed through the API.

| Method | Path | What it does |
|---|---|---|
| GET | `/sound-presets` | `[{id, slug, name, note, family, specs, final, master, parts, reference_job_id, target_lufs, builtin}]` |
| POST | `/sound-presets` | Create your own → preset with `id`. The body is below; an empty preset and other errors — 422 with a reason |
| PUT | `/sound-presets/{id}` | Replace your own completely (body as for POST). Built-in — 409, no such one — 404 |
| DELETE | `/sound-presets/{id}` | Delete your own → `{deleted}`. Built-in — 409, no such one — 404 |
| POST | `/jobs/{id}/sound-presets/{pid}/state` | `{status, child_id?, error?}` — the status of a preset on the track (the app applies it): `pending→running` (claim; already not `pending` — 409), `running→done` (`child_id` — the version), `running→error` (`error` up to 500 characters), `error`/`running→pending` (retry); other transitions — 409, no field — 422, the track has no such preset — 404. The response is the track's `sound_presets` |

### Preset body

| Field | Meaning |
|---|---|
| `name` | Name, 1–80 characters |
| `note` | Description of the sound |
| `specs` | Up to 16 stem edits for the whole track: `{stems: [stems], db −24…24}` and exactly one of: `engine` (an engine chain, checked as for `/fx`), `chain` + `params` (an ffmpeg effect), `steps: [{chain, params, off}]` (up to 12 pedals). Stems: `vocals`, `drums`, `bass`, `other`, `guitar`, `piano`, `kick`, `snare`, `toms`, `hh`, `ride`, `crash`. An entry `{stems: [one], level_db}` — volume only; `{stems: [one], place: {pan −1…1, width 0…2}}` — position in stereo |
| `level_db` | On an entry with one stem, −40…+6 — target loudness of the stem relative to the track: when applying, the app adjusts the volume by measurement (±12 dB; with processing — by measuring the processed stem; after mastering it checks the result and makes up to two correcting rebuilds) |
| `final` | Up to 12 steps `{chain, params, off}` on the whole mix after the edits (ffmpeg) |
| `master` | An engine chain of up to 16 blocks on the whole mix on the worker after the final stage |
| `target_lufs` | −24…−6 or `null` — loudness of the result: it goes into the master `limiter` (true peak −1 dBTP; no `limiter` — one is added) |
| `parts` | Up to 8 parts over the track: `{kind: synth/perc, engine (first block synth or perc, without notes), style: pad/arp/pulse/drone, octave −2…2, pattern: fours/eighths/sixteenths/backbeat/offbeat, swing 0…0.5, accent 0…1, sections, place}`. The app builds the notes from the chords (`chord_grid`) and the track grid |
| `reference_job_id` | The reference track on which the preset was tuned |

## Voice lab voices

| Method | Path | What it does |
|---|---|---|
| POST | `/voices` | `{name, job_id, params (JSON string of knobs), seed}` → `{id}` — a voice card from a finished audition track; the audio is copied and survives deletion of the track |
| GET | `/voices` | Cards, newest first; `job_alive` — whether the source track still exists, `has_audio` |
| DELETE | `/voices/{id}` | Delete the card and the audio copy → `{deleted}` |

## Studio draft

Unfinished work on a track: studio edits ("Track changes": splices, effects on stems, pedals, engine, volumes,
placement, master) and pending piece renders. The app writes it; the worker does not parse `state` (a JSON object
up to 1 MB). The draft is deleted with the track and after "apply all" in the studio.

| Method | Path | What it does |
|---|---|---|
| GET | `/drafts` | All drafts `[{job_id, state, updated_at}]` |
| GET | `/jobs/{id}/draft` | The track's draft `{job_id, state, updated_at}`; none — 404 |
| PUT | `/jobs/{id}/draft` | `{state}` → the draft; no track — 404, `state` not an object or over 1 MB — 422 |
| DELETE | `/jobs/{id}/draft` | → `{deleted}` |

## Edit log and recipes

The log is what was done to a track. A recipe is a song as rules without audio, to repeat it on another
installation. The app and MCP write the log after a successful edit, and they also run a recipe import step by
step; the worker only stores the state.

| Method | Path | What it does |
|---|---|---|
| POST | `/jobs/{id}/ops` | `{op, params, result, refs}` → `{id}` — a log entry. `op`: `submit`, `continue`, `fragment`, `overdub`, `variant_track`, `fx`, `minus` (old recipes: skipped on import), `update`, `set_head`, `rebuild_sections`, `envelope`, `splice`, `dsp_apply`, `pedals`, `sound_preset`; other values — 422, no track — 404 |
| GET | `/jobs/{id}/ops` | The track's log in order `[{id, job_id, op, params, result, refs, created_at}]` |
| GET | `/jobs/{id}/recipe` | Recipe of the whole song of the track (from the root). 422 — the chain of versions is broken by a deleted version, or the song has something a recipe cannot carry (a draft, an import of someone else's audio, "Album voice"). The `worker_model` field is the model of the song's root; tracks of the former PyTorch engine have `m-a-p/YuE2-3B@pytorch`: the model will not match the current one, and the import will suggest the "by note plan" mode |
| POST | `/recipes/import` | `{recipe, mode: seed/plan}` → `{id}` — a new import session; a foreign format — 422 |
| GET | `/recipes/imports` | Import sessions `[{id, title, status, progress, error, mode, root_job_id, created_at, finished_at}]` |
| GET | `/recipes/imports/{id}` | The same plus `recipe` and the leader's `state` |
| POST | `/recipes/imports/{id}/claim` | `{owner}` → `{id}` — take the lead for 10 min; someone else holds it — 409 |
| PATCH | `/recipes/imports/{id}` | `{owner?, state?, progress?, status? (running/done/error/dismissed), error?, root_job_id?}`; someone else leads the import — 409 |
