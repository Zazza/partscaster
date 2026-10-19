# Partscaster features
**English** · [Русский](../ru/features.md)

What the app can do, in short and in the order of work: generate a song → fix it → finish the sound → hand it off.
How to use it — [ui-guide.md](ui-guide.md), all effects — [effects.md](effects.md), control through an
AI agent — [mcp.md](mcp.md), what cannot be done — [limitations.md](limitations.md).

## 1. Generate

- **New track form** — hints in slots: genre, rhythm, guitars, keys, voice, mood, sound. On
  submit they become English tags for the YuE2 model; write your own words in English — Russian ones are not
  translated, and the form warns about it. The result is shown in the
  "Style as a single line" field — you can rewrite it by hand.
- **Style library** — 37 groups, 112 ready styles; **instruments** — group, instrument, effect and "Add";
  the added ones go into the style.
- **Your own lyrics**; section tags (`[Verse]`, `[Chorus]`…) are inserted with buttons above the field and are optional:
  without them YuE2 lays out the parts itself if stanzas are separated by an empty line. Lines with `#` are your notes and
  are not sung; the "no vocals" option gives an instrumental. Details — [lyrics-melody.md](lyrics-melody.md).
- **Character** (how bold and how precise the model is) and **dramaturgy** — flat, build-up, wave or burst (the final
  chorus one octave higher).
- **Plan (ABC)** — the song's note score (melody, chords, sections) that the model writes before the sound: "notes →"
  shows it in seconds; you can fix it and attach it to the form. "Import ABC" — a ready score as the skeleton of the track.
- **Queue** — one generation; **×5 seeds** — five variants of one song (a seed is the random number); you pick the best one
  by ear.
- **Sound presets right in the form** — up to three; when the track is ready, the "track · preset" versions appear by themselves.
- **A generation model to fit your graphics card** — generation runs on yue2.cpp, from 8 GB of video memory. "Settings"
  suggest a model for your card (BF16, Q8 or Q8 in economy mode); you can choose any. The same seed on another model gives
  a different performance; a track made by another model has a model badge in the list.

## 2. Edit

The track studio opens with the "studio" button on the track card. The source does not change when you edit: every edit is a new
version. Details — [ui-guide.md](ui-guide.md).

- **Listen** — waveform, spectrum and the plan's piano roll; a click plays from that point, a drag selects a stretch.
- **Tricks on a selection** — "wrong chord" (darker / brighter / more tense), "drop-out (silence)", "cut bars",
  "vocal ↑/↓ oct.", tempo. The tricks collect in the plan, "Apply" puts them into the draft, they sound after "apply all" in "Done"; for a quick listen —
  "stretch with edits".
- **Voice by sections** — "voice higher", "voice: variations", "voice: hold phrase ends", "voice: stop", then
  "↻ re-sing from here": two new vocal takes only in the selected part; the music does not change.
- **Rebuilding sections by continuation** — the model re-renders the selected stretch in the context of the track, and only the
  needed stem goes into the track (guitars/keys, bass or drums); "continue from here" — the same performance up to the mark,
  new after it.
- **Without vocals** — "play without vocals": the track is played anew without the vocal part; the same music without vocals — "remove" on the vocals in "Stems".
- **Replay in another style…** (track menu) — the model plays the track again from its notes in a new genre: a cover as a new
  version or a layer over the original.

## 3. Sound

- **Stems** — the track is split into vocals, drums, bass, guitar, keys and "other": Demucs (fast) or
  BS-RoFormer-SW (cleaner, about 4.5 times slower); the drums are also split into parts (kick, snare, toms, hi-hat, ride, crash).
  The "Stems" desk: volume, pan, width, its own sound for each stem, "before / after" on a selection.
- **Replacing instruments with real samples from the track's notes** — the "replace" button in the desk: live drums and
  drum machines (808, 909, Linn, CR-78, Simmons), bass guitars, orchestra (violins, cellos, brass), keyboards and synths.
  The hits and notes are taken from the stem itself.
- **Guitarist** — a part played from the plan's chords replaces the YuE2 guitar: strumming, fingerpicking, bass + chord, fifths or "auto" by
  song section. The track needs a plan.
- **Synth from chords** and **percussion on the grid** — a new part over the track, in time: old synths (pad, arpeggio,
  pulse, drone) and hi-hat, shaker, tambourine, claps, cowbell, kick doubling.
- **Genre sound presets** — a recipe for processing the whole track; the result is a new version "track · preset". 109
  built-in: 106 genre presets (34 styles in the families "Rock" (Рок), "Heavy" (Тяжёлое), "Electronic" (Электроника), "Pop and other" (Поп и другое)) and 3 general ones. One
  preset changes:
  - drums — all parts as a kit (live or drum machine) with rhythm section processing;
  - bass and guitar — processing and volume (the guitar stays the YuE2 guitar);
  - parts on top — synth or percussion in the right sections;
  - master — glue, loudness target, peak limiter;
  - balance — levels of kick, snare, hi-hat and guitar measured on real recordings; after the master the app checks
    the balance and corrects it up to two times. Presets do not touch the voice.
- **"Live sound" layer** — is added to any preset: it rebuilds the top above 16 kHz, which YuE2 cuts off; for
  presets with live drums — a crash on section changes and in choruses, a ride in choruses, **drummer fills** before
  a section change and **drums forward in choruses**. Details — [effects.md](effects.md).
- **Quick effects** — 53 ffmpeg chains: space, modulation, filters, dynamics, tempo and pitch, lo-fi,
  voice effects, guitar pedals; on the whole track or a stretch, with an in-place preview. All of them — [effects.md](effects.md).
- **Pedals** — a board of guitar effects: ready sets, order, on/off, comparing variants on one stretch.
- **Volume line** on the waveform — for the whole track or one stem.
- **"Instruments" page** — a phrase (guitar, bass, drums) in a loop through pedals, a NAM amp, a cabinet and
  space; tempo ×0.5…×1.5; you can save a good chain.
- **"Sound presets" page** — what is inside each preset, search by families and styles, an editor for your own preset
  (stem processing, volume, master), listening on a demo. You can also save your own preset from the studio —
  "save edits as preset". Details — [ui-guide.md — Sound presets](ui-guide.md#sound-presets).

## 4. Done

- **Master** — the final chain on the whole mix: glue, equalizer, true-peak limiter, loudness target
  (LUFS); ready variants for streaming. The quick option — "one-click mastering".
- **All track versions** — listen, compare, make one the track; metrics (loudness, dynamics, stereo, spectrum).
- **Download** a version or a stem as a file; "Album loudness" matches the album tracks to each other.
- **Track recipe** — "Export recipe": the song's rules as a small file without audio; "Import recipe" reproduces it
  on another installation (by seed or by the note plan).

## 5. Around the track

- **Queue** — all long worker operations in the left panel: progress, cancel, order "Earlier / Later", pause;
  an "AI" mark on what an agent started.
- **MCP for AI agents** — an agent (Claude, Codex) runs the whole flow: generation, studio, sound, installation and diagnostics.
  Tools and setup — [mcp.md](mcp.md).
- **Voice fitting room** ("voices" menu) — knobs for the voice character, a ~18 s preview, a voice card with a seed;
  a saved voice is chosen in the new track form.
- **Covers** — "From a track…" in the plan editor: SheetSage2 turns someone else's song or your own into a plan; you paste
  the lyrics yourself.
- **Album voice** (experiment) — the track's voice with the timbre of the voice from another song (Seed-VC).
- **Importing your own track** (flac/mp3/wav/ogg/m4a) — with the "import track" button in the track list; with stems, effects, presets and, if transcription worked,
  a plan.

## Under the hood

- The app (Wails: Go + Vue) runs on the PC; generation, stems and the sound engine run on a worker with an NVIDIA graphics card.
  The worker can be on the same PC or on another machine in the network. Quick effects are computed by ffmpeg on the PC.
  Diagram — [architecture.md](architecture.md).
- Installation and system requirements — [deployment.md](deployment.md), the worker's HTTP API — [api.md](api.md).
