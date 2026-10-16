# Effects and sound engine

**English** · [Русский](../ru/effects.md)

How you change the sound of a finished track in Partscaster: effects on the whole track or on one stem, guitar pedals,
replacing instruments with samples of real instruments, new parts on top of the track, and genre presets ("the whole
style"). The source never changes: every processing step gives a new variant or a new track version.

First, how to use it. Then a reference: all chains, blocks, ready-made kits and sample licenses.

## How to use

The app has four ways, from the simplest to the most detailed:

| Way | Where | What it does | Where it runs |
|---|---|---|---|
| **Sound preset** | track studio, "Sound presets" ([ui-guide.md](ui-guide.md)) | the whole track style with one button: drums, bass, guitar, parts, master | GPU machine (worker) |
| **Effects (DSP)** | track studio, the "Sound" step, the **"Quick effects (ffmpeg)"** section, the **"Audio effects"** box | one ffmpeg chain on the whole track or on a stem: reverb, delay, mastering, lo-fi… | PC |
| **Pedals** | track studio, the "Sound" step, the **"Quick effects (ffmpeg)"** section, the **"Pedals"** box | a board of guitar pedals in signal order | PC |
| **Sound engine** | the "Stems" desk in the studio and the "Instruments" page (the "instruments" menu item) | processing without a time shift, replacing drums, bass and guitar with samples, synth, orchestra, percussion, master | GPU machine (worker) |

If you do not know where to start: apply a genre preset ([overview](#genre-presets)), listen, and then
edit single stems with effects or with the engine.

General rules for all ways:

- **Nothing gets damaged.** An effect on the whole track is a variant in the list of variants. An effect on a stem is
  an edit. It stacks with inserts and mutes and is built in one rebuild from the clean original. A preset is a new
  track version.
- **Preview first.** Almost everything has a "preview spot": a piece of the track with the effect. The
  **"▶ after ⇄ / ▶ before ⇄"** button switches the same piece with and without the effect, from the same second.
- **Stems are made automatically.** To process one stem, the track must be split into stems. If there are no stems,
  the app makes them itself. For instrument replacement, RoFormer separation is better: it has drum parts
  (kick, snare, toms, hi-hat, ride, crash) and less leakage of one stem into another.
- **The default values work** — it is best to start with them.

## Effects (DSP chains)

Effects are ffmpeg filter chains that run on the PC. Total: 53 chains. Below: how to apply them, what
each one does, and what to combine it with. Guitar pedals are built into a chain in their order in the "Pedals"
box — see [Pedals and pedalboard](#pedals-and-pedalboard).

### How to apply

In the track studio — the "Sound" step, the **"Quick effects (ffmpeg)"** section, the **"Audio effects"** box (the
interface in detail — [ui-guide.md](ui-guide.md)).

Whole-track variants are collected in the "Done" step, the "results" box.

1. **Pick a chain** in the "effects chain…" list. Under the list is a short description, below it are knobs with default values.
2. **Pick the target**: the whole track or one stem — vocals, drums, bass, "guitars/synths".
   - **Whole track** — processes the whole mix. The result is a variant `dsp-<chain>.flac`.
   - **Stem** — processes only this stem. The difference "processed − original" goes into the track. The other
     stems do not change.
   - **Window** — a selection on the roll or on the wave: the effect on the stem sounds only there. No selection — the
     whole track.
3. **Listen to the preview spot** — both for the whole track and for a stem. The piece is the selection on the wave or
   roll. No selection — 15 seconds from the playback cursor. No cursor either — from second 20. For a stem you hear it
   in the mix with the others. The reverb/delay tail sounds up to 3 seconds after the piece. The **"stem only"**
   checkbox plays the same processed and original stem without the rest of the mix: in a dense mix (a wall of
   guitars) the effect is masked, solo you hear it. The preview is stored on the PC and does not go into the variant
   list. The first preview of a track downloads the audio and the stems; the next ones take a couple of seconds.
4. **Apply** — the variant appears in the list with the metric difference from the source (crest factor,
   dynamics, top end, flatness). "full delta" (полная дельта) shows all metrics.
5. You like it — **"→ to tracks"** (→ в треки) in the variant row (the variant becomes a separate song version), then
   the "make main" star at that version in the track list. You do not like it — "✕".

**Common knobs:**

- **"from which second"** (с какой секунды) (`from`) — on all chains without their own mark. Before it the track is
  untouched. At the mark the processed sound smoothly (0.1 s) replaces the dry one. This is how you make "harder from
  the third chorus".
- **"dry/processed"** (сухой/обработанный) (`mix`) — the effect share: 0 — dry sound, 1 — only processed. Vocal
  chains, chorus, flanger, phaser, auto-wah, doubling, Uni-Vibe and ring modulator have it.
- Reverb and delay have **"tail / repeats volume"** (громкость хвоста / повторов) (`wet`) instead of `mix`: the dry
  sound is not reduced.

**One click:** in the "Done" step, the "results" box — the "one-click mastering" and "one-click breathing" buttons
with the level light / medium / strong — a chain on the whole track with tested settings. The result is a separate version right away.

**"find grid"** — on chains with a tempo (Rhythm gate, delay, stutter): the tempo and the downbeat are detected from
the drums and filled into the knobs.

**Through an AI agent (MCP):** `dsp_chains` — the list with knob ranges, `dsp_presets` — pedal presets,
`dsp_preview` — preview of a piece (`stem`, `from`/`to`, the effect chain in order — `steps`), `dsp_apply` —
apply (with `stem` and `from`/`to` — on the stem in a window; `steps` — a chain in order, without `stem` — the variant
`dsp-pedals.flac`), several effects on stems at once — `rebuild_sections`. Details — [mcp.md](mcp.md).

### Space

Against the "flat" neural mix: YuE has almost no space, the track sounds "in a vacuum".

| Effect | What it does | Main knobs (defaults) |
|---|---|---|
| Reverb: room (`reverb-room`) | A small room, dense early reflections, a short tail | tail 0.6 s · pre-delay 10 ms · tail top 8 kHz · tail volume 0.3 · width 1 |
| Reverb: hall (`reverb-hall`) | A large hall: the tail grows and fades for a long time | tail 2.5 s · pre-delay 30 ms · top 6 kHz |
| Reverb: plate (`reverb-plate`) | A studio plate: a dense bright tail at once — vocals, snare drum | tail 1.6 s · pre-delay 5 ms · top 12 kHz |
| Reverb: spring (`reverb-spring`) | A guitar spring reverb, a metallic "boing" — surf, rockabilly, dub | tail 1.2 s · top 4.5 kHz |
| Tempo delay (`delay`) | Repeats on the track beat, they fade, ping-pong between channels | BPM 120 · beat 1/8 dotted · feedback 0.45 · ping-pong on · repeats top 6 kHz · volume 0.5 |
| Stereo width (`width`) | A wider or narrower stereo image; the low end stays in the center | width 1.4 (0 — mono, 1 — as is, 2 — wider) · mono below 120 Hz |
| Doubling (Haas) (`haas`) | One channel 10–30 ms later: one sound, but wide | delay 18 ms · channel (right) · share 0.7 |

How to use:

- **Reverb on vocals** — as an effect on the "vocals" stem, not on the whole track: this keeps the drums dry and tight.
  The tail on a stem in a window keeps sounding after the window ends — it is not cut off.
- **Pre-delay** of 20–40 ms separates the voice from the tail: the words stay clear even with a long hall.
- **"Tail top"** (верх хвоста) lower (4–6 kHz) — a warm, "distant" tail; higher — a bright, "close" one.
- **Tail width** 0 — a mono tail (old records), 1 — a wide one.
- **Delay**: the tempo comes from "find grid". 1/8 dotted is the classic for vocals and guitar. Feedback 0.3–0.5 —
  a few repeats, 0.7+ — a long "space".
- **Stereo width** works with what is there: it widens an almost mono track only a little. For such a track, reverb
  or doubling is better.
- **Doubling** gives width even to a mono source, but lowers the volume a bit (about −1 dB): the dry and delayed
  signals partly cancel each other.

### Pedals and pedalboard

The **"Pedals"** box in the studio (the "Sound" step, the "Quick effects (ffmpeg)" section) is a board of guitar effects **in signal order**, like pedals on the floor: the
output of one is the input of the next. Separate effects on one stem do not work this way: each one is computed from
the original stem, and their differences add up.

1. **Stem** — "guitar" by default (6-stem separation makes it automatically on the first preview). If the track has no
   separate guitar, the pedals switch to "guitars/synths" themselves and say so. You can pick "guitars/synths",
   keys, vocals, bass or the whole track.
2. **Preset** — a starting point: "Hendrix" (Хендрикс), "Shoegaze" (Шугейз), "Surf" (Сёрф), "Grunge" (Гранж),
   "Dub" (Даб), "Post-punk" (Пост-панк), "Clean + space" (Чисто + простор). The preset is laid out on the board,
   then you can edit everything.
3. **"+ pedal"** — the palette: all pedals below plus suitable effects (chorus, flanger, phaser, tremolo, auto-wah,
   octaver, delay, reverbs, equalizer).
4. On each pedal: **●/○** — on/off (a switched-off pedal stays on the board), **← →** — move, **✕** — remove.
   Click a pedal to see its knobs (the delay has "find grid").
5. **"preview spot"** and **"▶ after ⇄ / ▶ before ⇄"** — as in "Audio effects": the selected spot, from the same second.
6. **"compare presets"** — tick up to three presets → "render slices": A — the current board, B–D — the presets, all
   on the same piece. The A/B/C/D and "▶ before" buttons switch from the same second.
7. **"apply"** — on a stem: one rebuild edit (in the selected spot, no selection — the whole track), it stacks with
   inserts. On the whole track — the "pedals" variant (`dsp-pedals.flac`).

The board is remembered for each track and survives a restart.

**Honestly about the sound:** the pedals process a guitar that the model already recorded and a neural network cut out
of the mix. Modulation, delay, reverbs and cabinet sit well. Overdrives on top of an already distorted guitar and the
leakage of other instruments can give mush — choose by ear in the preview. If you want the found sound to be "native",
describe it in the style (the guitar field, "rack") and regenerate, or replace the guitar completely with the
[guitarist by chords](#guitarist-by-chords).

| Pedal | What it does | Main knobs (defaults) |
|---|---|---|
| Overdrive (Tube Screamer) (`od-ts`) | Warm overdrive with a mid hump, the low end does not boom — blues, solos | drive 18 dB · tone 3.5 kHz · volume 0 dB |
| Fuzz (Big Muff) (`fuzz-muff`) | Thick fuzz, huge sustain, scooped mids — a wall of guitars | sustain 30 dB · tone 0.5 (bass ↔ top) · volume 0 |
| Distortion (RAT) (`dist-rat`) | Hard, harsh distortion — punk, grunge | drive 28 dB · tone (top) 4 kHz · volume 0 |
| Octave fuzz (`fuzz-octave`) | Fuzz with an octave up (Octavia) — a ringing solo | fuzz 24 dB · octave 0.7 · tone 5 kHz |
| Booster (`boost`) | A clean boost of volume and top; before an overdrive — angrier | volume 8 dB · brightness 2 dB |
| Uni-Vibe (`univibe`) | A thick phaser sway — Hendrix, Gilmour | rate 2 Hz · depth 0.6 · share 0.6 |
| Ring modulator (`ringmod`) | Sound × tone: metallic sums and differences, "robot" | frequency 440 Hz · share 0.8 |
| Noise gate (`noise-gate`) | Silence between notes: removes the overdrive hum in pauses | threshold −50 dB · close 100 ms |
| Compressor (pedal) (`comp-pedal`) | Notes more even and longer — country, funk | sustain 0.5 · attack 10 ms · volume 0 dB |
| Cabinet (`cab`) | A guitar speaker: no extra low and top end — the overdrive sounds "like a guitar" | low cut 90 Hz · top 5 kHz · mids 0 dB |

For the overdrives (overdrive, fuzz, distortion, octave fuzz) on a stem, the volume of the processed guitar is matched to
the original. Otherwise clipping would bring any quiet stem to full scale, and the guitar would cover the mix. The
pedal "volume" is a correction in dB on top.

The order, as on the floor: gate/compressor → booster/overdrive/fuzz → modulation → cabinet → delay → reverbs.
Changing the order changes the sound (reverb before fuzz — a "dirty cloud").

### Modulation

| Effect | What it does | Main knobs |
|---|---|---|
| Chorus (`chorus`) | Several "floating" copies: wider and thicker, "several players" | depth 3 ms · rate 0.6 Hz · share 0.5 |
| Flanger (`flanger`) | A comb moves across the spectrum: a "jet" whoosh | delay 2 ms · depth 2 ms · rate 0.3 Hz · feedback 30 % · share 0.5 |
| Phaser (`phaser`) | A soft sway of spectrum notches — 70s psychedelia | rate 0.5 Hz · depth 0.5 · share 0.6 |
| Tremolo (`tremolo`) | A smooth sine sway of volume (no edges, unlike the Rhythm gate) | rate 5 Hz · depth 0.5 |

Best on the "guitars/synths" stem. Chorus on vocals — a "double" vocal. Flanger on the whole track is a noticeable
trick, it goes into a window (a transition, a bridge).

### Filters and tone

| Effect | What it does | Main knobs |
|---|---|---|
| Equalizer (`eq`) | Low and high shelves, a mid bell. 0 dB — the band is off | low 0 dB at 120 Hz · mid 0 dB at 1 kHz, Q 1 · high 0 dB at 8 kHz |
| Filter sweep (`sweep`) | From the mark, the cutoff frequency moves over N seconds | from which second · over 8 s · type (low cut) · 20 → 2000 Hz · after the sweep — dry |
| Auto-wah (`autowah`) | A band-pass filter sways between two frequencies | rate 2 Hz · 400–2200 Hz · share 0.8 |
| De-whistle (`dewhistle`) | A narrow notch at the squeal frequency (and harmonics) in a window | frequency ("find whistle" helps) · up to three tones · depth 30 dB · width 60 Hz |
| Soften ringing (`soften`) | De-esser: pushes down ringing and hissing spots | strength 0.5 · limit 0.5 · from which frequencies 0.5 |

- **Sweep** — a "run-up before the chorus": low cut 20 → 2000 Hz over 4–8 seconds before the chorus, at the chorus the
  sound is full again. "Into the pillow" — high cut (type 0) 20000 → 500 Hz.
- **Auto-wah** — a sway over time. ffmpeg cannot follow the playing volume (a real "auto").
- **De-whistle and soften ringing** — repair of generation artifacts. Better on the stem with the artifact (often
  vocals), otherwise the cymbals get quieter too.

### Loudness and dynamics

| Effect | What it does | Main knobs |
|---|---|---|
| Mastering (`master`) | "Like a real record": unsquash the brick, loudness through saturation, top-end grit | drive 1.5 · grit 1.3 · unsquash 1.5 |
| Breathe (`breathe`) | Unsquash the "brick": quiet spots a bit quieter, loud ones as they were | strength 1 |
| Album loudness (`level`) | Only volume and a peak limiter — the tone and dynamics stay the same | volume 0 dB · ceiling −1.5 dB |
| Multiband compressor (`multiband`) | Three bands, each with its own compressor: a loud bass does not push down the vocals | density 0.5 · splits 200 and 3000 Hz |
| Ducking from drums (`ducking`) | The stem dips on every drum hit — a "pumping" mix | threshold 0.08 · ratio 6 · attack 5 ms · release 200 ms |
| Fade in/out (`fade`) | A fade-in from the start and/or a fade-out from the mark to silence | fade-in 0 s · fade-out from second 0 (off) · length 8 s |
| Louder to the end (`cresc`) | A smooth volume rise from the mark — a bigger finale | from second 120 · over 30 s · +3 dB |
| Silence (`gap`) | A full pause from the mark — a lull before a fill | from second 120 · 2 s · edges 40 ms |

- **Ducking** — only as an effect on a stem (usually "guitars/synths" or bass): the key is the drums of the same track.
  The stem is picked automatically. On the whole track it is an error: the mix has no key.
- **Album loudness** — matching to one loudness (for example, −14 LUFS): how many dB to add — see LUFS in the
  track metrics.
- The chain does not know the track length: the fade-out is set with the "from which second" mark, not "N seconds
  before the end".

### Tempo, pitch and glitch

| Effect | What it does | Main knobs |
|---|---|---|
| Speed up from the mark (`tempo-from`) | From the mark faster without a pitch change — push the finale | from second 120 · ×1.1 |
| Drop (`drop`) | Slow-down and fade, a pause, then a fade-in — "drop → run-up". The track gets longer | slow down 3 s to tempo 0.7 · silence 2 s · fade-in 6 s from −30 dB |
| Transpose (`pitch`) | Higher or lower by semitones without a tempo change, vocal formants are kept | +1 semitone (−12…+12) |
| Octaver (`octaver`) | Mixes in a copy an octave lower and/or higher | down 0.5 · up 0 |
| Reverse to the mark (`reverse`) | Before the mark — a piece played backwards: a reverse cymbal or a reverse tail | mark · length 1.5 s · source (after the mark) · volume 0.8 · on top |
| Stutter (`stutter`) | From the mark one grid hit repeats N times — a "stutter" before a fill | from second (on the downbeat) · BPM · hits per beat 2 · repeats 4 |
| Tape stop (`tape-stop`) | The sound slows down and drops to a stop, then the track goes on | from second · over 1.5 s |
| Rhythm gate (`gate`) | The volume opens and closes in time: a sustained sound → a pulsing rhythm | BPM · hits per beat 4 · open share 0.5 · depth 0.9 · grid shift |

- **Put the marks on the downbeat** (the start of a bar): the joint sounds exactly at the mark. The beat time comes
  from "find grid" or from the wave with the plan bars.
- **Transpose and octaver** need ffmpeg with the `rubberband` filter (see [limitations.md](limitations.md)).
- **Rhythm gate** — on the "guitars/synths" stem in a window: a pad or a sustained guitar becomes a synth rhythm on the
  same chords.

### Tape, dirt and lo-fi

| Effect | What it does | Main knobs |
|---|---|---|
| Liveliness (`alive`) | Micro-detune, volume breathing, compressor pumping — "played by hand" | detune 0.06 · breathing 0.12 · pumping 2.5 · grain 1 |
| Cassette (`tape`) | Wow/flutter, top cut, pink hiss — home tape | wow 0.1 · hiss 0.018 · top 9.5 kHz |
| Warp tape (`warp`) | Deep wow and flutter, dull top — tape defects as a trick | warp 0.22 · flutter 0.12 · top 7 kHz |
| Vinyl (`vinyl`) | Record crackle and clicks, quiet noise, a slightly closed top | crackle 0.4 · noise 0.008 · top 14 kHz |
| Dirt/overdrive (`grit`) | Saturation, bitcrush, grit — garage and tube rasp | drive 2.4 · bitcrush 0.35 · grit 2.2 |
| Wall/noise/grit (`wall`) | A monolith of loudness, top-end grit, a noise layer | exciter 2.5 · wall 0.5 · noise 0.09 |
| Light wall (`wall-lite`) | The same chain, softer | exciter 1.2 · wall 0.7 · noise 0.05 |

### Vocal effects

In the studio they live in the "4 Voice" step → "Voice processing" (not in the general effects list), next to
the general effects that suit a voice (`for_voice`). MCP and the API offer them as before. They are marked as vocal: the "vocals" stem is picked automatically. The volume of the processed voice is matched to
the original (the overdrive does not break the mix), "dry/processed" (сухой/обработанный) is the effect share.

| Effect | What it does | Main knobs |
|---|---|---|
| Megaphone (`megaphone`) | A voice through a horn: narrow band, overdrive, short echo (Jack White) | band 400 Hz–3 kHz · saturation 5 · ring 6 · echo 0.5 |
| Telephone (`phone`) | A handset: an even narrower band, dry, a light rasp | band 420 Hz–2.6 kHz · rasp 3 |
| Vocal overdrive (`voice-drive`) | A dense overdriven mid, a top closed as if by tape (Letov) | drive 22 dB · mid +5 dB · low from 200 Hz · top 8.5 kHz |
| Slapback (`slapback`) | A single echo of 80–120 ms — rockabilly | delay 100 ms · volume 0.6 |

### Recipes against a "flat" sound

Measured on eight neural tracks (stereo correlation, 1 = mono; raw neural tracks have 0.83–0.98):

- **Space for the whole track:** "Reverb: hall" (Реверб: зал), tail volume 0.3 — the correlation drops to 0.67–0.76,
  the track "stands in a room". "Tempo delay" (Дилей в темп) (volume 0.35) — to 0.78–0.91, softer.

Starting points (by ear, without measurements):

- **Vocals forward, but in space:** on the "vocals" stem — "Reverb: plate" (Реверб: плейт) (volume 0.25, pre-delay
  30 ms) and "Tempo delay" (Дилей в темп) (1/8 dotted, feedback 0.3, volume 0.2).
- **Wide guitars:** on "guitars/synths" — "Doubling (Haas)" (Удвоение (Хаас)) or "Chorus" (Хорус); on an almost mono
  track this is wider than "Stereo width" (Стерео-ширина).
- **Pumping synth rock:** "Ducking from drums" (Ducking от барабанов) on "guitars/synths" — the kick punches through
  the wall of guitars.
- **Transition into the chorus:** "Filter sweep" (Свип фильтра) (low cut) 4–8 seconds before the chorus, on the last
  beat — "Reverse to the mark" (Реверс к отметке) (a reverse cymbal).
- **Finale:** "Louder to the end" (Громкость к концу) +2–3 dB or "Speed up from the mark" (Ускорить с отметки)
  ×1.05–1.1; the very end — "Fade in/out" (Нарастание/затухание).
- **The last step** — "album loudness…" to a common loudness (for example, −14 LUFS).

The numbers are a direction, not a guarantee: the final decision is by ear.

## Sound engine (on the worker)

A second processing path, independent of the chains above: the blocks run on the GPU machine (numpy/scipy + NAM amps —
neural "captures" of real amplifiers), not in ffmpeg on the PC. The main difference: **processing does not shift the
sound**. The output of each block has the same length, and a hit stays on the same sample (linear filters are
zero-phase, the delay of a NAM capture is measured and removed; with a real model at most 1 sample remains). So you can
process a stem and put it back into the track without "comb filtering" against the remains of the same part in other
stems.

**Where to listen and how to put it back into the track:**

- **The "Instruments" page** (the "instruments" menu item; details — [ui-guide.md](ui-guide.md)) — build a chain and
  listen to it on a short phrase in a loop; no track is needed here. The guitar phrases are recordings of a clean guitar (a cut of GuitarSet, see
  [licenses](#sample-kits-and-licenses)); the bass and drum phrases are synthesized. There is no button to send the chain
  to the studio: save it as your own instrument ("save as mine…"), and in the studio pick it in the "Stems" desk
  along with the ready-made ones.
- **Studio, the "Stems" desk** — a ready-made or your own chain on a stem in the selection. The entry stacks with
  inserts and ffmpeg effects and is built by the same rebuild from the clean original. The worker computes the stem as
  a piece of the window with smooth edges. The processed piece goes into the track instead of the original stem; the
  reverb/delay tail sounds after the window.
- **MCP** — `fx_apply` (listen), `rebuild_sections` with the `engine` field (into the track), `fx_blocks`, `fx_presets`,
  `fx_assets`, `fx_kit_install` ([mcp.md](mcp.md)).

The result of `fx_apply` is a variant `dsp-fx-<stem>-<hash>.flac` in the common variant list: `output=mix` — the track
where the stem is replaced with the processed one, `solo` — only the stem; `from`/`to` — the window, outside the window
the track is bit-identical.

**Instrument replacement depends on separation.** The engine replaces the stem exactly, but what the separator put into
other stems keeps sounding with the original sound. With Demucs, the guitar leaks into the bass a lot (measured on one
track: 72 % of the 0.4–4 kHz band of the bass is the same guitar), and the processed guitar lies on top of it as a quiet
copy. With RoFormer the trace in the bass is small (16 %), and the replacement is audible — for the engine, make stems
with RoFormer (settings → "stem separation" or `make_stems model=roformer`).

### Drums: hit replacement

**Kick and snare.** The `sampler` block on the "kick" and "snare" stems replaces the hits with kit samples: the
ready-made chains "Kick: kit" and "Snare: kit" (the live kit `osdk` — The Open Source Drum Kit). The worker downloads
the kit on demand (`fx_kit_install osdk`). In the studio — the "Stems" desk, the "kick"/"snare" row, the window is the
selection.

**Hi-hat and cymbals.** The same block on the "hi-hat", "ride", "crash" stems: the ready-made "Hi-hat: kit", "Ride: kit",
"Crash: kit" (missing kit parts are downloaded automatically). "Hi-hat: kit" has an equalizer after the samples: the
osdk hi-hat samples are dull (the sound center is about 5 kHz) and have kick and snare leakage, while the hi-hat of real
records is 6–9 kHz without low end. A low cut at 1 kHz, −6 dB at 1 kHz and +8 dB at 10 kHz even it out. Drum machines
have a bright hi-hat anyway — without an equalizer. The hi-hat has two kits: `kit` — closed, `kit_open` — half-open.
The hit type depends on how long the hit sounds in the stem (until it falls by −20 dB, not past the next hit); the
border is in the middle between the sample lengths of both kits. `choke` — a new hit mutes the previous one, like a
hi-hat pedal. A preview with cymbals has a tail of up to 4 s. Limit: the block hears a hit in the last ~160 ms of the
window only up to the window end and gives it a closed hi-hat.

**Toms by pitch.** The `kit_mid` and `kit_low` rows split the hits of the "toms" stem by pitch: the hit pitch is the
spectrum peak at 50–500 Hz in the first 60 ms. The hits are grouped by clusters of pitches; groups closer than ~2
semitones or with less than 5 % of the hits are merged. The high ones are played by `kit` (small tom), the middle ones
by `kit_mid`, the low ones by `kit_low`. One tone for the whole part — the middle tom. The ready-made chain is
"Toms: kit by pitch". `kit_open` cannot be used together with them.

**Hit dynamics.** The `dynamics` parameter: 1 — hit strength as played, 0 — all the same strength (machine), 2 — quiet
(ghost) hits even quieter, accents louder. The overall stem volume does not change.

**Drum machines.** Besides the live kit, there are five machines that the worker synthesizes itself (without the
network and without third-party samples): 808 (a booming kick, a metallic hi-hat), 909 (a dense kick, a noisy snare),
an 80s sampling machine in the spirit of the LinnDrum (28 kHz, 8 bit with companding), an early rhythm machine in the
spirit of the CR-78, electronic drums in the spirit of Simmons (toms with a "pew"). Each one has a kick, a snare, a
closed/open hi-hat, a ride, a crash, three toms, a clap, a rimshot, a cowbell, with 8 velocity layers each; the kick
and toms have an emphasized attack, so that the sampler places the hit exactly. The ready-made chains per part are
`drums-<part>-<machine>` ("Kick: 808 drum machine" and so on).

**Rhythm section with one button.** In the "Stems" desk — "rhythm section from kits": all drum parts and the bass that
the track has, with ready-made chains in one rebuild (each part is a separate edit). You pick the kit and the
treatment: dry, room (a short quiet reverb of 0.5 s, wet 0.12 — the dry samples sound in one room), 80s gated reverb
(a bright hall cut off 280 ms after the hit — `reverb gate_ms`, not a level gate), tight (a compressor and parallel
dirt), lo-fi (tape and a top cut); the treatment does not go to the bass. The same for the whole track is the built-in
sound preset "Live rhythm section" (Живая ритм-секция).

### Bass: part replacement

The `bass` block on the "bass" stem plays the bass notes with samples of a real instrument. The rhythm is the beats of
the stem itself (`division` notes per beat, 2 — eighths) and its hits; the pitch is the bass frequency in each beat. A
new pluck happens on a hit or on a note change, otherwise the note is held. The volume and the low/mid balance follow
the original bass over time: the samples have more mids, and one common volume gave a bass that gets in the way under
the vocals. The notes are not recognized by a separate model — the block takes the pitch per beat itself. The block
cannot repeat a complex pattern (slap, fast runs).

Bass kits (ready-made chains of the "Bass: kits" section):

| Ready-made chain | Kit | Sound |
|---|---|---|
| Bass: bass guitar (kit) | `growlybass/bass` | Squier Jazz Bass |
| Bass: flatwounds (kit) | `swagbass/bass` | Ibanez BTB with old flatwound strings — a dull "Motown" |
| Bass: 5-string fingerstyle (kit) | `blackblue/darkblack` | warm, dense, deep low end |
| Bass: picked, bright (kit) | `blackblue/babyblue` | a bright attack for punk and rock |
| Upright bass, pizzicato (kit) | `meatbass/pizz` | a 1958 double bass — jazz, rockabilly, acoustic |
| Bass VI, picked (kit) | `pastabass/linguine` | the Squier Bass VI baritone bass — clicky post-punk and surf |
| Bass: Moog synth (kit) | `synthbass/moog` | a saw with a square through a filter with a click |
| Bass: 808 sub (kit) | `synthbass/sub808` | a low sine with dirt, it fades |
| Bass: acid 303 (kit) | `synthbass/acid` | a resonant filter, it "quacks" on every note |

The worker synthesizes the synth basses itself: notes by semitones E1…G3, files `m<MIDI>.wav`. The block takes the
pitch from the name, and folds the stem notes outside the kit range by octaves into the kit range.

**Bass treatments without note replacement** (different sound, same notes):

- the "Bass: tone" section — melodic high with chorus, fuzz bass (parallel overdrive), pick/punk, deep dub;
- the "Bass: genres" section — warm vintage (flatwound strings, tape), tight fingerstyle, bright slap, funk with a
  compressor, rock with half overdrive, metal (overdrive and click), post-punk with chorus, soft "fretless".

Replacement limits: a legato note change right after the previous note — the pluck comes up to 10 ms after the note
start. A note that enters in the first 20–40 ms of the window over the tail of another one sometimes sounds up to 8 ms
early. A legato note change in the first ~100 ms of the window gives no separate pluck (in the studio rebuild the smooth
window entry covers it, in the preview you hear it). The block works best on a RoFormer stem: on a Demucs "bass" with
leaked guitar the pitch is detected worse.

### Guitarist by chords

The `gtrnotes` block on the "guitar" stem plays a **new** part with samples of another guitar — like a guitarist who was
told "fit in by ear". The hits are the hit moments of the old guitar (the attack peak of the stem); the chord of a hit
is the plan chord in that bar (the same grid as `chord_grid`). The notes of the old guitar are not recognized: the
guitarist plays the plan chords, so the track plan (transcription) is needed, otherwise you get a clear error. The old
guitar does not go to the output — it sets the hit strength and the volume over time.

The playing manner is the `style` parameter:

| `style` | Manner |
|---|---|
| 0 | auto by song parts: verse — fingerpicking, chorus/pre-chorus/bridge — strumming, the rest — bass + chord |
| 1 | strumming |
| 2 | fingerpicking (bass → up → down the chord strings) |
| 3 | bass + chord (on beats 1 and 3 — root/fifth, in between — the top of the chord) |
| 4 | fifths (root + fifth + octave) |

Fingerings are open for common chords, barre for the others. A hit sounds equally loud with any number of strings; the
tuning is the root of the bass and the vocals. Strumming (`strum` 1): a hit on the beat is a downstroke (strings from
bottom to top with a `spread_ms` spread, a quiet hit is wider), between beats an upstroke (quieter). An intro without
drums follows the beat grid extended past the edge. A muted hit is a sample of the `mute_kit` kit. A new hit mutes the
ringing strings of the previous one.

The part is computed without a GPU in seconds and cached with the track (`guitar-part.json`; a new guitar stem or a new
plan — recalculation). To see it: `GET /jobs/{id}/guitar_part`, MCP `guitar_part`. Limit: if the YuE2 guitar went into
another stem during separation (Demucs on metal — into the bass), the new guitar sounds only where the guitar stem has
hits.

Ready-made guitarists (the "Guitarist fits in by chords" section):

| Ready-made chain | Guitar (kit) | Manner |
|---|---|---|
| Nylon (classical guitar) | `nylonguitar/guitar` (FreePats) | fingerpicking |
| Hollow-body (microphone) | `shinyguitar/acoustic` (Karoryfer Shinyguitar) — closest to acoustic | auto |
| Electric: clean (Epiphone) | `emilyguitar/notes` (Karoryfer Emilyguitar) | bass + chord |
| Electric: clean (Fender, bridge) | `fsbsguitar/guitar` (FreePats FSBS) | strumming |
| Electric through a fuzz | `fsbsguitar/guitar` + overdrive | fifths |

### Synth, keys and orchestra over the track

**Synth over chords.** In the studio (the "Sound" step, "Synth over chords") — a new part on top of the track: "read
chords" (chords and song parts from the plan, locked to the audio bars: beats from the drums, the plan offset from the
guitar and bass notes), a ready-made instrument, a style (pad — a chord per bar, arpeggio — eighths over the triad,
pulse — the root in eighths, drone — the root and the fifth), octave, song parts, volume relative to the track
(`rel_db`, default −8 — 8 dB quieter than the track where the synth plays; the volume "by ear" uses the A curve, at the
output of the whole synth chain). "▶ part alone" — hear what to look for in the mix. The part is added to the track and
subtracts nothing — no stem separation is needed.

You hear a synth not so much because of its volume but because of its register: a pad in the guitar register merges
with them even when louder than the track; an octave higher it is clearly audible. So the ready-made pads, organs and
keys with the "pad" style play an octave above the guitars.

**The `synth` block** plays the part notes: two oscillators (saw, square, pulse, triangle, sine, FM), unison with
detune, sub-octave, noise, a filter with resonance and its own envelope, a volume envelope, vibrato and an LFO on the
filter. The FM oscillator is a sine whose phase is modulated by a sine `fm_ratio` times higher with depth `fm_index`,
fading over `fm_decay_s` (electric piano, bells, music box). The `kit` parameter — play with kit samples instead of
oscillators (the pitch comes from the file name: "A0v10", "C#4", "m28"; the nearest sample is shifted by resampling):
the Salamander piano and the VSCO-2 orchestra. Effects for synths are the `chorus`, `phaser`, `flanger`, `tape`,
`spring` blocks.

Ready-made synths and keys (25):

| Section | Instruments |
|---|---|
| Pads and strings | Solina string ensemble, Juno pad with chorus, CS-80 brass, Mellotron tape strings, "aah" choir |
| Synth bass | Moog synth bass (pulse in eighths) |
| Leads | Moog lead, theremin, stylophone |
| Organs | Farfisa, Vox Continental, Hammond with rotary speaker, church organ |
| Keys | grand piano and soft piano (Salamander samples), Rhodes, Wurlitzer, DX7, clavinet, harpsichord |
| Toys and bells | VL-Tone, music box, celesta, omnichord, Casio |

**Orchestra (VSCO-2).** Samples of real orchestral instruments. The worker downloads an instrument kit when you pick it
for the first time (~5…100 MB). 33 ready-made chains in three roles:

| Role | How it works | Ready-made |
|---|---|---|
| Part on top of the track (like a synth) | the `synth` block with a VSCO `kit` plays the plan chords | strings (8): violins, violas, cellos in chords, solo violin, violins pizzicato (arpeggio), cellos pizzicato (pulse), double bass drone, harp; woodwinds (5): flutes, flute staccato, oboe, clarinets, bassoon; brass (5): French horns, trumpets, short brass stabs, trombones, tuba; pitched percussion (3): glockenspiel, marimba, xylophone |
| Bass → orchestra | the `bass` block on the "bass" stem: an instrument plays the bass notes | cellos pizzicato, bowed cellos, violas pizzicato, double bass pizzicato, bassoon, tuba (6) |
| Melody → orchestra | the `bass` block with pitch search at 80…1500 Hz on the guitar, "other" or vocal stem: a single-line melody (guitar solo, vocals) | violin, flute, oboe, clarinet, trumpet, French horn (6); two notes per beat, fast runs and bends are simplified |

### Percussion over the track

In the studio ("Percussion on the grid", next to the synth) — hi-hat, shaker, tambourine, claps, cowbell, rimshot,
ride or kick doubling on top of the track. "find the grid": with a plan — bars and song parts (as for the synth),
without a plan — bars from the tempo and downbeat of the whole track. Pattern (every beat, eighths, sixteenths, on 2
and 4, between beats), swing (the second eighths/sixteenths later: 33 % — triplets), accent (0 — all hits even, 100 % —
beats louder, in-betweens quieter), volume relative to the track. This is an addition, not a replacement: replacing the
drum pattern sounded bad — the rest of the old hits argued with the new ones.

The `perc` block plays hits with a drum machine voice (synthesis) or with kit samples (`voice` 0, `kit`; the hit
strength picks the layer). The volume is relative to the track over the whole part. `humanize_ms` shifts the hits
"like a human" the same way in the preview and in the rebuild.

Ready-made (9): hi-hat eighths (808), hi-hat sixteenths (808), shaker sixteenths, tambourine on 2 and 4, claps on 2
and 4 (909), cowbell quarters, rimshot on 2 and 4, kick doubling (osdk, "four on the floor"), ride eighths (osdk).

### Mixing: place in the stereo field

Each stem in the "Stems" desk has pan (left…right) and width (0 — mono, 1 — as is, 2 — wider); synth and percussion
parts have their own (ready-made: pads wider, hi-hat/shaker/tambourine/cowbell/ride to the side). The rebuild computes
the place (not an engine block): width — through mid/side, pan — with power preserved (at the edge the sound is not
louder than in the center). The place applies to all stem edits, so the result of a stem is its sound after all edits
in the new place. A place on "drums" and on one of their parts (kick…) at the same time adds up approximately. The
first lever against "the part merges with the guitars" is still the register (synth an octave higher); the place is
the second one.

### Master

The "Done" step, the "Master" box: an engine chain on the whole mix with edits — the last step of the rebuild, on the
worker, into the same mix file (next to it is the "mix without master" `overdub-premaster-N.flac` for "before/after").
Blocks: `glue` — bus glue (a stereo-linked compressor with a soft knee and a parallel share `mix`); `limiter` — a
true-peak limiter (×4 between samples, look-ahead without a time shift): peaks not above `ceiling_db` dBTP; with
`target_lufs` the loudness of the whole track is brought to the target (±0.3 LU, adjustment ±12 dB). Ready-made masters:
"streaming −14 LUFS", "loud −11 LUFS", "soft glue" (no loudness). The `master` and `target_lufs` fields of a sound
preset also run on the worker.

### Ready-made guitar chains

They work on a bare worker — only engine blocks, without captures and IR — except the "Guitar: amp" section:

- **Clean:** clean with a short reverb, post-punk chorus and delay, a bright compressed "jangle", surf (spring and
  tremolo).
- **Drive:** garage fuzz, industrial metal (a hard gate and scooped mids), symphonic metal (high gain and hall),
  alt-rock with phaser.
- **Space:** shoegaze wall with flanger, ambient (long delay and hall), stereo tremolo pulse, lo-fi cassette.
- **Amp (NAM):** clean amp, hotter drive, Vox and short reverb, high gain. These chains carry `amp_hint` — substrings of
  the capture name by priority (clean — twin/fender, drive — jcm900/jcm, Vox — ac15, high gain —
  od2/jvm-od/5150/recto/bug): the studio and "Instruments" take the first loaded capture with the hint in its name,
  otherwise the first loaded one. Captures and IR are not shipped — upload your own (`fx_asset_upload`).

The names describe the sound; the "in the vein of …" reference is in the tooltip. The engine drive (`drive`) is tanh,
not an amp: on a YuE guitar that is already distorted it adds dirt and compression; a "real" amp is the chains with a
capture.

A working chain for a YuE guitar: `gate` → `eq` (highpass 100, lowpass 5500) → `amp` with a capture of **the amp only,
without a cabinet** → `cab` without IR. A capture with a cabinet gives "two cabinets" — it sounds like a telephone.

### All ready-made engine chains

152 ready-made chains in 29 sections ("Instruments" page, "Stems" desk, MCP `fx_presets`; the source is
`frontend/src/fxPresets.js`, the copy for the worker is `worker/fx_presets.json`):

| Section | Count | What is inside |
|---|---|---|
| Guitar: amp | 4 | NAM: clean, hotter, Vox, high gain |
| Guitar: clean | 4 | clean, post-punk, jangle, surf |
| Guitar: drive | 4 | fuzz, industrial, symphonic metal, alt-rock |
| Guitar: space | 4 | shoegaze, ambient, tremolo, lo-fi |
| Guitarist by chords | 5 | nylon, hollow-body, two clean electrics, through a fuzz |
| Bass: tone | 4 | melodic, fuzz, pick, dub |
| Bass: genres | 8 | vintage, fingerstyle, slap, funk, rock, metal, post-punk, fretless |
| Bass: kits | 9 | 6 bass guitars/double bass and 3 synth basses (table above) |
| Mix | 3 | vocal: plate; drums: room; synth: delay and hall |
| Drums: live kit | 6 | kick, snare, toms by pitch, hi-hat, ride, crash (osdk) |
| Drums: 808, 909, Linn, CR-78, Simmons | 5 × 6 | the same six parts for each machine |
| Master | 4 | glue (on the whole track), streaming −14 LUFS, loud −11 LUFS, soft glue |
| Synths and keys | 25 in 6 sections | pads, synth bass, leads, organs, keys, toys |
| Orchestra | 33 in 6 sections | strings, woodwinds, brass, pitched percussion, bass → orchestra, melody → orchestra |
| Percussion | 9 | see above |

### Engine blocks

A chain is a list of blocks in order (JSON, missing parameters use defaults). A chain has at most 16 blocks, an
equalizer has at most 12 bands (more → error 422 with the reason): a long chain would occupy the worker and the GPU
queue for a long time. The blocks are described in one file, `worker/fx_blocks.json` (the worker checks the chain
against it; the page and MCP get a copy through `make mcp-data`).

| Block | What it does | Parameters (defaults) |
|---|---|---|
| `gate` | noise gate: below the threshold — attenuated | threshold_db −50 · range_db −40 · attack_ms 1 · release_ms 100 |
| `eq` | low/high cut and bands | highpass_hz 0 (off) · lowpass_hz 0 (off) · bands [{freq_hz, gain_db, q}] |
| `comp` | compressor | threshold_db −20 · ratio 4 · attack_ms 10 · release_ms 100 · makeup_db 0 |
| `drive` | overdrive (tanh, ×4 oversampling); volume — as at the input | gain_db 12 · mix 1 · output_db 0 |
| `amp` | amp — a NAM capture (`fx_assets`); the input is brought to −20 dBFS RMS, the output to the input volume | model (req.) · input_db 0 · output_db 0 |
| `cab` | cabinet — convolution with an IR (the IR peak at zero); without IR — a "light cabinet": a top cut | ir "" · cutoff_hz 7000 · mix 1 |
| `reverb` | reverb — convolution with a hall IR or the built-in one (decaying noise); the dry sound is untouched; `gate_ms` — the response is cut off this many ms after the hit (80s gated reverb, does not depend on level) | ir "" · decay_s 1.5 · predelay_ms 10 · lowpass_hz 8000 · wet 0.3 · gate_ms 0 (off; 50…1000) |
| `delay` | repeats, each next one darker | time_ms 375 · feedback 0.35 · lowpass_hz 6000 · wet 0.3 |
| `gain` | volume: overdrive and amp match the output to the input — after a narrow band the sound is quiet, raise it with this block | gain_db 0 (−24…+24) |
| `sampler` | hit replacement: on a drum part stem the hits are detected, the hit strength picks a kit sample, the sample is placed peak to the hit peak; the output has only the new hits; `kit_open` — a kit for long-sounding hits (open hi-hat), `choke` — mute the previous hit; `kit_mid`/`kit_low` — toms by pitch | kit (req.) · kit_open "" · kit_mid "" · kit_low "" · floor_db −18 (quieter — leakage) · dynamics 1 (0 even … 2 sharper) · output_db 0 · choke 0 |
| `bass` | part replacement: the stem notes per beat are played with kit samples — bass guitar, cello, violin, flute… (the sample pitch comes from the name m&lt;MIDI&gt;/note, otherwise from the sound; the nearest one is shifted; notes are folded by octaves into the kit range), the attack is on the input hit; volume and low/high balance follow the input | kit (req.) · division 2 (1…4) · fmin_hz 30 · fmax_hz 400 (where to look for the input pitch: bass 30…400, melody 80…1500) · floor_db −20 · output_db 0 |
| `synth` | synthesizer: plays the `notes` [{t, d, midi[], vel}]; `kit` — play with kit samples (piano, orchestra); the volume "by ear" (A curve) is relative to the input track over all part notes (`rel_db`), at the output of the part chain; a quiet input — peak −6 dBFS | osc1 0 / osc2 0 (0 saw, 1 square, 2 pulse, 3 triangle, 4 sine, 5 FM) · osc2_semi 0 · osc_mix 0 · unison 1 · detune_cents 0 · sub 0 · noise 0 · pwm 0.5 · fm_ratio 1 · fm_index 2 · fm_decay_s 0.5 · cutoff_hz 4000 · resonance 0 · env_amount 0 · f_attack_s 0.01 · f_decay_s 0.3 · attack_s 0.01 · decay_s 0.2 · sustain 0.8 · release_s 0.3 · vib_rate 5 · vib_cents 0 · lfo_cutoff 0 · rel_db −8 · output_db 0 · kit "" |
| `chorus` | chorus/ensemble: copies through modulated delays, L/R with different phase | voices 2 · depth_ms 3 · rate_hz 0.6 · mix 0.5 |
| `phaser` | phaser: an all-pass chain with a modulated frequency | stages 4 · rate_hz 0.4 · depth 0.7 · feedback 0.3 · mix 0.5 |
| `flanger` | flanger: a short modulated delay with feedback | delay_ms 2 · depth_ms 2 · rate_hz 0.25 · feedback 0.4 · mix 0.5 |
| `tape` | tape: pitch drift (wow) and flutter, saturation (quiet signal — level unchanged, peaks are compressed), top cut, hiss | wow 0.2 · flutter 0.1 · saturation 0.2 · lowpass_hz 10000 · hiss 0 |
| `spring` | spring reverb: dispersion and reflections of the spring, the dry sound is untouched | decay_s 1.8 · tone 0.5 · wet 0.25 |
| `tremolo` | tremolo: the volume sways with an LFO; shape 0 — sine, 1 — "on/off"; stereo 1 — the right channel in opposite phase (the sound moves left-right); the sound is not shifted | rate_hz 5 · depth 0.5 · shape 0 · stereo 0 |
| `perc` | percussion: hits `notes` [{t, d, vel}] with a drum machine voice or kit samples; the volume is relative to the input track in the hit cells of the whole part (`rel_db`); a quiet input — peak −6 dBFS | voice 1 (0 kit, 1 hi-hat, 2 shaker, 3 clap, 4 cowbell, 5 rimshot, 6 tambourine) · kit "" · tone 1 · decay 1 · humanize_ms 0 · rel_db −14 · output_db 0 |
| `glue` | bus glue (master): a stereo-linked RMS detector, a 6 dB soft knee, a parallel share | threshold_db −18 · ratio 2 · attack_ms 10 · release_ms 250 · makeup_db 0 · mix 1 |
| `limiter` | true-peak master limiter; with `target_lufs` — loudness to the target over the whole input | ceiling_db −1 (dBTP) · release_ms 100 · target_lufs 0 (off; −24…−6) |
| `air` | air: rebuilds the top end above 16 kHz that YuE2 cuts — second harmonics of the 8–11.5 kHz band, only above 16 kHz; the level is `tilt_db` below the neighboring 12–16 kHz band (as in live records). Where the top end already exists (or the band itself is missing), the sound does not change; below 15 kHz it does not change. `only` 1 — the output is only the air (to add on top of the track) | tilt_db −10 (−30…0) · only 0 |
| `gtrnotes` | guitarist by chords: a part (the worker sets `notes` — the plan chords in the hits of the old guitar; on the "Instruments" page — the phrase notes) with samples of another guitar; the input is the old guitar: it does not go to the output, it sets the hit strength and the volume over time | kit (req.) · mute_kit "" · style 0 · strum 1 · spread_ms 8 · output_db 0 |
| `level` | volume by a line: points `points` [{t, db}] in track seconds (up to 200, dB −30…+12, ascending t), linear in dB between them, before the first and after the last — their dB; a window preview uses the place in the track; the sound is not shifted. It is not shown in the block palette — the "live sound" layer adds it ("drums forward") | points (req.) |

"Req." — a parameter without a default value: without it the worker answers 422 "parameter … is required" (нужен
параметр …). A kit (`kit`) is set as `<kit>/<part>`, for example `osdk/kick`.

## Genre presets

A sound preset is the whole track style with one button. There are 109 built-in presets: 106 genre variants and 3
older ones ("Post-punk · vocals and bass up front" (Пост-панк · голос и бас впереди), "Indie rock · wide and loud"
(Инди-рок · широкий и громкий), "Live rhythm section" (Живая ритм-секция)). The names describe the genre and the
sound, without band names.

**What one genre preset does** (it does not touch the vocals):

1. **Drums** — all six parts (kick, snare, toms, hi-hat, ride, crash) are replaced with a kit — the live osdk or one of
   the drum machines — with a rhythm section treatment: dry, room, gated reverb, tight or lo-fi. The cymbals (hi-hat,
   ride, crash) get no compression and overdrive from "tight", and their room is brighter (top 12 kHz): compression and
   a dark room muffled the ring.
2. **Bass** — a ready-made chain: a sound treatment or note replacement with a kit (bass guitar, synth bass).
3. **Guitar** — the guitarist by the plan chords (see below) and the variant treatment.
4. **Parts on top of the track** — some variants have a synth, keys or percussion in all song parts or only in
   some (for example, strings in the choruses).
5. **Master** — bus glue and, where the style asks for it, tape; loudness — as a target:

| Master | What is in it | Loudness |
|---|---|---|
| streaming | glue | −14 LUFS |
| loud | glue | −11 LUFS |
| tape | tape (top 12 kHz) + glue | −14 LUFS |
| cassette | cassette (top 9 kHz) + glue | −14 LUFS |
| soft | glue with share 0.4 | −16 LUFS |

The ["live sound" layer](#the-live-sound-layer) is applied together with any preset.

**List of genres** — 4 families, 34 genres, 106 variants (30 genres have 3 variants each, four have 4 each: the fourth
variant comes with the guitarist by notes right away — acoustic, fuzz, clean guitar, nylon):

| Family | Genres |
|---|---|
| Rock (15) | Post-punk, Cold wave, New wave, Gothic rock, Indie rock (4), Garage rock (4), Punk, 90s alt-rock, 70s classic rock, Blues rock, Surf, 60s Psychedelia, Shoegaze & dream pop, Post-rock, Stoner |
| Heavy (5) | Heavy metal, Alt- and nu-metal, Industrial metal, Symphonic metal, Doom |
| Electronic (9) | Synthpop, Synthwave, Chiptune & 8-bit, EBM & electro, Techno & acid, House, Trip-hop, Lo-fi hip-hop, Ambient |
| Pop and other (5) | 80s pop, Indie pop (4), Funk and disco, Soul, Jazz lounge (4) |

### Loudness balance

The preset sets a loudness target relative to the track for the kick, snare, hi-hat, bass and guitar: the "loud spots"
of the stem (the 95th percentile of RMS) relative to the sum of the main stems. The drum targets are based on
measurements of real records split by RoFormer:

| Family | Kick | Snare | Hi-hat |
|---|---|---|---|
| Rock, pop and other | −7.5 dB | −9.5 dB | −23 dB |
| Heavy | −8 dB | −10 dB | −23 dB |
| Electronic | −6 dB | −11 dB | −25 dB |

With the "soft" master, the kick and snare go 2 dB deeper. The bass target is set by the variant (usually −6…−8 dB). The
guitar target is the variant level plus the family offset (rock −2.5 dB, heavy −4.5, pop and other −1, electronic 0)
and another −3 dB if the guitarist plays: clean samples with an attack are more audible at the same loudness.

An edit with processing is brought to the target by measuring the already processed stem (overdrive and samples change
the loudness). After the master, the preset splits the finished version, checks the balance and, if it misses by more
than 1 dB, rebuilds — at most two correcting passes: the master to LUFS loudness and the remains of old stems in other
stems shift the balance.
Builds before a correction are not deleted: they are labeled (in Russian) «промежуточная: сборка 1 — до сверки баланса»
(before the balance check) / «промежуточная: сборка N — после поправки N−1» (after correction N−1) and fold under the
final version "track · preset".

### Guitarist in genre presets

In all genre presets except electronic, the YuE2 guitar is replaced by the [guitarist by chords](#guitarist-by-chords)
(80 of 106 variants), followed by the variant treatment. The guitar and the manner are chosen by the treatment:

| Variant treatment | Guitar | Manner |
|---|---|---|
| post-punk chorus, surf | FSBS (Fender) | fingerpicking |
| tremolo, ambient echo | Emily (Epiphone) | fingerpicking |
| "jangle", shoegaze wall | FSBS | strumming |
| clean (blues, funk, soul, pop) | Emily | bass + chord |
| alt-rock | FSBS | auto by song parts |
| fuzz, industrial, metal | FSBS through the variant overdrive | fifths |
| cassette treatment | FSBS through a fuzz, softer tape | fifths |

Exceptions: "Post-punk · angular dry" (Пост-панк · угловатый сухой) — fingerpicking; "Symphonic metal" (Симфо-метал)
and "Doom · mournful" (Дум · траурный) — clean Emily fifths with echo and hall instead of high gain. The tape on clean
samples is softened (wow ≤ 0.15, flutter ≤ 0.1): strong tape on them "shakes and floats". In electronic presets the
guitar is in the background and stays the YuE2 guitar, except "Chiptune and 8-bit · 8-bit and guitars" (Чиптюн и
8-бит · 8-бит и гитары).

For a track without a plan, the guitarist is skipped (the YuE2 guitar goes through the rest of the processing), and the
version name gets the mark "YuE2 guitar — the track has no plan" (гитара YuE2 — у трека нет плана).

### The "live sound" layer

Any sound preset is applied together with the "live sound" layer (the track itself without a preset is the YuE2 output
as is). The layer fixes what is wrong with YuE2 when measured against live records:

- **Top end above 16 kHz.** YuE2 cuts it, and the voice sounds dull. The `air` block rebuilds it on the "vocals" and
  "other" stems, which the preset does not process itself (the air is computed from the original stem and would break
  through the top cut that the preset puts on the guitar on purpose). For a preset with only a final chain on the whole
  mix — air on the whole mix, without stem separation.
- **Too few cymbals** (crash 6–30 dB quieter than in live records, ride up to 10 dB). For a preset with the live osdk kit
  — a crash on the first beat of every song part change and of every chorus bar, 18 dB quieter than the track (louder
  rare hits stick out); a ride on the hi-hat hits in the choruses — if the preset also replaces the hi-hat. The added
  cymbals have a top cut at 14 kHz: the samples are brighter than the cymbals of mixed records. A preset with a drum
  machine (808, 909 and others) keeps its own drums.
- **Drummer.** For a preset with the live kit, before every song part change there is a fill: half a bar, into a
  chorus — one bar, into the last chorus — two bars; sixteenths, the first third is a snare roll, then the toms from
  high to low, the strength grows (`perc` blocks with osdk kits, 7 dB quieter than the track). YuE2 almost never plays
  toms and fills — the layer adds them.
- **Drums forward in choruses.** If the plan has a chorus, the `level` block is put at the end of the chain of the drum
  parts — a volume line by song parts: chorus +4.5 dB, pre-chorus +1, verse −1, the rest 0; the rise starts with the
  fill. The drum loudness target is raised by the same 4.5 dB: the "loud spots" measurement falls on the chorus.

Cymbals, fills and "drums forward" need the track plan (song parts): a track without a plan gets only air. The layer is
not written into the preset recipe or into the edit log — when a recipe is imported, it is added again.

## Sample kits and licenses

The kits are not shipped: the worker downloads them on demand from GitHub (`fx_kit_install <kit>` or the studio itself
when a kit is needed) — the version of each one is pinned. To download all at once — `kits_install.py`
([deployment.md](deployment.md)). The worker synthesizes the drum machines and synth basses itself — they contain no
third-party samples. The user uploads NAM captures and IRs themselves — their licenses are the user's responsibility.

**Attribution required (CC BY):**

| What | Where it is used | License | How to credit |
|---|---|---|---|
| Salamander Grand Piano V3 | the `salamander` kit (parts `piano`, `piano-soft`, 30 notes each, ~47 MB): "Grand piano", "Soft piano" | CC BY 3.0 | Alexander Holm, Salamander Grand Piano (sfzinstruments/SalamanderGrandPiano) |
| GuitarSet (cut) | guitar phrases of the "Instruments" page, `worker/phrases/` — **stored in the repository** ([phrases README](../../worker/phrases/README.md)) | CC BY 4.0 | Q. Xi, R. M. Bittner, J. Pauwels, X. Ye, J. P. Bello, "GuitarSet: A Dataset for Guitar Transcription", ISMIR 2018 |

**No attribution required:**

| Kit | Parts | What it is | License |
|---|---|---|---|
| `osdk` | kick, snare, hh-closed, hh-half, hh-open, ride, crash, tom-small, tom-medium, tom-large | The Open Source Drum Kit (Real Music Media), live drums | stated as public domain in the code; the kit repository does not name a license |
| `growlybass` | bass | Karoryfer Growlybass — Squier Jazz Bass | CC0 |
| `swagbass` | bass | Karoryfer Swagbass — Ibanez BTB, flatwound strings | CC0 |
| `blackblue` | darkblack, babyblue | Karoryfer Black And Blue Basses — 5-string, fingers / pick | CC0 |
| `meatbass` | pizz | Karoryfer Meatbass — 1958 double bass, pizzicato | CC0 |
| `pastabass` | linguine | Karoryfer Pastabass — Squier Bass VI, picked | CC0 |
| `emilyguitar` | notes, mute | Karoryfer Emilyguitar — Epiphone, pickups direct | CC0 |
| `shinyguitar` | acoustic, mute | Karoryfer Shinyguitar — hollow-body, microphone | CC0 |
| `fsbsguitar` | guitar | FreePats Electric Guitar FSBS — Fender, bridge pickup | CC0 |
| `nylonguitar` | guitar | FreePats Spanish Classical Guitar — nylon | CC0 |
| `vsco-violin` | solo, ens, pizz, spic | VSCO-2 Community Edition — solo violin and section | CC0 |
| `vsco-viola`, `vsco-cello`, `vsco-contrabass` | ens/sus, pizz, spic | VSCO-2 CE — violas, cellos, double bass | CC0 |
| `vsco-harp` | harp | VSCO-2 CE — harp | CC0 |
| `vsco-flute`, `vsco-oboe`, `vsco-clarinet`, `vsco-bassoon` | sus, stac | VSCO-2 CE — woodwinds | CC0 |
| `vsco-trumpet`, `vsco-horn`, `vsco-trombone`, `vsco-tuba` | sus, stac | VSCO-2 CE — brass | CC0 |
| `vsco-mallets` | glock, marimba, xylo | VSCO-2 CE — glockenspiel, marimba, xylophone | CC0 |
| `tr808`, `tr909`, `linn`, `cr78`, `simmons`, `synthbass` | — | synthesized by the worker | — |

The parts `pizz` and `spic` are pizzicato and a short bow stroke, `sus` — sustained, `stac` — short. In VSCO, the note
pitch in the file name almost everywhere counts from C3 = 60 (solo violin and harp — from C4 = 60); the worker corrects
the octave at install time and stores the samples under the exact pitch `m<MIDI>.wav`. The sample length is trimmed:
guitars — 3 s, basses — 6 s, piano and orchestra — 8 s (worker memory).

## Limitations

- The "live sound" layer, air: it is not added to stems that the preset processes itself (otherwise it would break
  through the intentional top cut). All built-in genre presets process the guitar — so they have air only on the
  vocals; strings, acoustic and synths on "other" (it and the guitar are one group) get no air. The air level is taken
  from the 12–16 kHz band of the stem itself: where YuE2 already dims that band (strings, acoustic), the boost is
  small. For presets with tape or cassette in the master (a 12/9 kHz cut is the style), the tape kills the air.
- Cymbals, fills and "drums forward" are placed by the track plan: a track without a plan gets only air.
- The reverb and delay tail past the end of the track is cut off: the result length equals the track length.
- There is no preview with the "Ducking from drums" (Ducking от барабанов) chain (it needs a second key stem) — you
  listen to it after applying.
- More details — [limitations.md](limitations.md), the section on DSP chains.
