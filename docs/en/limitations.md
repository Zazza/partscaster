# Limitations
**English** · [Русский](../ru/limitations.md)

What the app cannot do now, or can do only with caveats — and what to do instead.

## Some features do not work without a plan

A plan is the note score of a track (score.abc). A generated track always has one; an imported track has one only
if transcription worked (otherwise the studio has a "Create score" button). Without a plan:

- the **guitarist** is removed from your own preset that has one (built-in genre presets do not use it): the version
  gets the mark "(YuE2 guitar — the track has no plan)"
  ("(гитара YuE2 — у трека нет плана)"); the rest of the processing runs;
- **drummer fills** and the cymbals of the "live sound" layer are skipped silently;
- **drums forward in choruses** are skipped — and also when the plan has no chorus;
- **synth from chords** does not play (there are no chords); percussion follows the beat grid, without song sections.

## Hardware

- **NVIDIA graphics card only** (the installer checks `nvidia-smi`), from 8 GB of video memory. The generation model
  takes at peak: BF16 ~10 GB, Q8 ~8 GB, economy mode sizes memory to the card (~6.5 GB on an 8 GB card, a track with continuations up to ~10 min).
- **The model decides the song length**: it writes a plan for 3–5 minutes, the length cannot be chosen (the model
  ignores the number of `[Instrumental]` parts). With continuations a track is limited by the model context: up to
  ~14.5 min, in economy mode — by the card's video memory (~10 min on 8 GB); `YUE2_MAX_TOKENS` caps it from above.
  When memory runs out, the worker says what is to blame: the model itself (turn on economy mode or choose a smaller
  model) or other programs on the graphics card (for example, Stable Diffusion — free the memory).

## Security

The worker has **no authentication**: its API uploads and deletes files and controls the queue. Keep it only in the
local network; from outside — through a VPN, an SSH tunnel or a reverse proxy with authorization.

## Platforms

- The app is tested on Linux; for Windows there is a package (the app with its own player without PipeWire and the
  `partscaster-worker.exe` worker without WSL); **macOS has not been tested.**
- The built-in player on Linux plays through `pw-play` (PipeWire): the Linux webview has no sound. Without PipeWire the
  player is silent — listen in a browser. On macOS the player is the system `afplay`: volume changes take effect from the
  next start (a seek or the next track), not while playing. Not tested on a Mac.

## Experimental

- **Album voice** (Seed-VC): the voice is recognizable, but it shakes, and inside phrases the music suffers a little. An optional
  component of ~11 GB.

## Model and sound

- **You cannot get the timbre of a specific singer with one button.** The "Voice" slot and the fitting room set the delivery character,
  not the timbre. What helps: the manner and era in the style, choosing a seed (the "Variants" button) and processing the finished track.
- **The model takes stress and phrasing from the plan, not from the lyrics** — edit the melody: [lyrics-melody.md](lyrics-melody.md).
- **Old tracks separated before the switch to RoFormer (Demucs) are poor for instrument replacement:** the guitar
  leaks into the bass there and sounds on top of the processed one, and there are no drum parts. In the studio — "split in more detail".
- **SheetSage2 transcription** is accurate on a simple texture; on a dense mix the chords and sections are approximate.
- **A plan can be longer than the sound**: bars past the end of the track are dimmed in the roll.
- **A seed repeats the performance only on the same generation model.** Another model (BF16, Q8…) with the same seed
  sings differently; old tracks made by the former PyTorch engine cannot be repeated by seed — continuations and
  rebuilds from their notes work.
- **Quick effects:** the reverb and delay tail past the end of the track is cut off (the length does not change); "Transpose" (Транспонирование) and
  "Octaver" (Октавер) need ffmpeg with `rubberband` (without it — an ffmpeg error); pedals process the guitar cut out of the
  mix — distortion on top of distortion can sound like mush.

## Licenses

Partscaster code — [MIT](../../LICENSE). Model weights are not part of the repository; they have their own terms:

| Component | License |
|---|---|
| YuE2 | CC BY-NC 4.0 **with an additional permission from the model authors**: musicians and content creators may monetize the generated songs (License section in the README [github.com/multimodal-art-projection/YuE](https://github.com/multimodal-art-projection/YuE)) |
| BS-RoFormer-SW, DrumSep (stem separation, installed together with the worker) | CC BY-NC / BY-NC-SA — non-commercial, without such a permission |
| SheetSage2 (transcription) | CC BY-NC — non-commercial, without such a permission |
| yue2.cpp (the generation program; the GGUF models are the same YuE2 weights, YuE2 terms) | MIT |
| Seed-VC | GPL-3.0 |

Check the terms on the model pages.
