# Limitations
**English** · [Русский](../ru/limitations.md)

What the app cannot do now, or can do only with caveats — and what to do instead.

## Some features do not work without a plan

A plan is the note score of a track (score.abc). A generated track always has one; an imported track has one only
if transcription worked (otherwise the studio has a "Create score" button). Without a plan:

- the **guitarist** is removed from the preset: the version gets the mark "(YuE2 guitar — the track has no plan)"
  ("(гитара YuE2 — у трека нет плана)"); the rest of the processing runs;
- **drummer fills** and the cymbals of the "live sound" layer are skipped silently;
- **drums forward in choruses** are skipped — and also when the plan has no chorus;
- **synth from chords** does not play (there are no chords); percussion follows the beat grid, without song sections.

## Hardware

- **NVIDIA graphics card only** (the installer checks `nvidia-smi`). The main engine takes up to ~11.4 GB
  of video memory, yue2.cpp — from ~4.5 GB in economy mode.
- **Track length is limited** by the model budget (16000 tokens, variable `YUE2_MAX_TOKENS`). When memory runs out, the worker
  says what is to blame: the track length itself (reduce the duration) or other programs on the graphics card (Ollama,
  Stable Diffusion — free the memory).

## Security

The worker has **no authentication**: its API uploads and deletes files and controls the queue. Keep it only in the
local network; from outside — through a VPN, an SSH tunnel or a reverse proxy with authorization.

## Platforms

- The app is tested on Linux; it builds under Windows (its own player, without PipeWire). **The macOS build has not
  been tested.**
- The built-in player on Linux and macOS plays through `pw-play` (PipeWire): the Linux webview has no sound. Without PipeWire
  the player is silent — listen in a browser. macOS usually has no `pw-play` (not verified; how to verify — build and
  run it on a Mac).

## Experimental

- **Album voice** (Seed-VC): the voice is recognizable, but it shakes, and inside phrases the music suffers a little. An optional
  component of ~11 GB.
- **yue2.cpp**: the same seed gives a different song (only notes and continuations carry over); economy mode — a song up to
  ~3.5 minutes. The Windows package built on Linux comes without yue2.cpp.

## Model and sound

- **You cannot get the timbre of a specific singer with one button.** The "Voice" slot and the fitting room set the delivery character,
  not the timbre. What helps: the manner and era in the style, choosing a seed (×5) and processing the finished track.
- **The model takes stress and phrasing from the plan, not from the lyrics** — edit the melody: [lyrics-melody.md](lyrics-melody.md).
- **Instrument replacement works on RoFormer stems.** With Demucs the guitar leaks into the bass and sounds on top of
  the processed one.
- **SheetSage2 transcription** is accurate on a simple texture; on a dense mix the chords and sections are approximate.
- **A plan can be longer than the sound**: bars past the end of the track are dimmed in the roll.
- **Quick effects:** the reverb and delay tail past the end of the track is cut off (the length does not change); "Transpose" (Транспонирование) and
  "Octaver" (Октавер) need ffmpeg with `rubberband` (without it — an ffmpeg error); pedals process the guitar cut out of the
  mix — distortion on top of distortion can sound like mush.

## Licenses

Partscaster code — [MIT](../../LICENSE). Model weights are not part of the repository; they have their own terms:

| Component | License |
|---|---|
| YuE2 | CC BY-NC 4.0 **with an additional permission from the model authors**: musicians and content creators may monetize the generated songs (License section in the README [github.com/multimodal-art-projection/YuE](https://github.com/multimodal-art-projection/YuE)) |
| BS-RoFormer-SW, DrumSep (stem separation) | CC BY-NC / BY-NC-SA — non-commercial, without such a permission |
| SheetSage2 (transcription) | CC BY-NC — non-commercial, without such a permission |
| Demucs | MIT |
| Seed-VC | GPL-3.0 |

Check the terms on the model pages.
