# Lyrics and melody
**English** · [Русский](../ru/lyrics-melody.md)

How to make the model sing a verse with the right stress and phrases: what it understands in the lyrics, where the melody
is edited and what rules the plan has.

## Why the model sings it "wrong"

YuE2 does not read the verse. First it writes a **plan** — a note score in ABC format: it places the syllables on notes by itself,
based on the statistics of modern pop music. This is where the pop phrasing and the badly guessed irregular meter come from. Then the model
sings the vocal melody from the plan note for note.

Conclusion: **stress and phrases are edited in the plan's melody, not in the lyrics.** A flat melody in the plan gives a flat voice.

## What the model understands in the lyrics

- **Section tags** — the borders of the song sections. The buttons above the lyrics field insert them at the
  cursor on their own line: "Verse" `[Verse]`, "Pre-chorus" `[Pre-Chorus]`, "Chorus" `[Chorus]`, "Bridge"
  `[Bridge]`, "Intro" `[Intro]`, "Outro" `[Outro]`, "Interlude" `[Interlude]`. In "no vocals" mode the buttons
  are hidden.
- **Tags are optional.** Without them YuE2 lays out the parts itself — separate stanzas with an empty line.
  Checked on one song: with and without tags it came out in the same form (intro, verse, chorus, interlude,
  verse, chorus, outro), but without tags the choruses were shorter (12 bars instead of 16). If you need exact
  section lengths, use tags.
- **Line = phrase.** One line is one short phrase.
- **Punctuation** — commas and dashes affect the pauses.
- **Lines with `#`** — your notes; they are cut out on submit:
  ```
  [Verse]
  # stress: сЕло; pause after line 2
  Село в тумане, дорога вниз
  ```
  Capital letters do not move the stress.
- **A regular meter** is the easiest: in an irregular verse, make the number of syllables in the lines equal.

## Finished track: voice by sections

The simplest way if the track already exists. In the studio, select bars on the roll or a stretch on the waveform and choose voice tricks:

| Trick | What it does |
|---|---|
| "voice higher" | the same melody a third higher, with more push — but not above the voice ceiling (see below) |
| "voice: variations" | the rhythm and main notes stay the same, phrase ends move to neighboring notes; pressing again — wider |
| "voice: hold phrase ends" | the end of a phrase stretches into the pause after it — the word sounds longer |
| "vocal ↑ oct." / "vocal ↓ oct." | the vocal part one octave higher or lower |
| "voice: stop" | the voice goes silent in the selected stretch |

Then **"↻ re-sing from here"**: the voice of the selected part is sung again (two takes); the music and the voice outside the part do not
change. The new versions appear among the track versions in a few minutes. End the selection in a pause of the voice —
if the part ends in the middle of singing, a seam is possible.

## New track: editing the plan

1. Paste the verse into the form, add `#` notes for yourself.
2. Press **"notes →"** — the "Track notation (ABC) — for advanced users" window opens with the plan (seconds, no sound).
3. Find the line `V: Vocal` — the vocal bars follow it:
   ```
   V: Vocal
   E,4G,4"E"f4e2|d4"c"B,4z8|...
   ```
   `|` is a bar border, chords are in quotes, `z` is a rest, the digit after a note is the duration in `L` units from
   the header (usually `L:1/32`: `f4` = 4/32 = an eighth).
4. Edit for the stress:
   - a stressed syllable — onto a longer note and onto a strong beat: `f2` → `f4`;
   - an extra note — remove it or replace it with a rest;
   - the phrase starts in the wrong place — move the notes across `|` or put a rest at the start of the bar.
5. **"→ to the form"** attaches the plan to the form; start with the main "Queue" button. Bars, chords and melody come from the
   plan; genre, instruments, delivery and seed come from the form.

You can also attach a ready score (for example, from another track) with the **"Import ABC"** button in the form.

## Plan rules

Breaking these rules does not always give an error — the model just sings the wrong thing.

- **The voice ceiling is the top of the plan's melody + 2 steps.** Above it the model squeaks or breaks into falsetto. The "voice higher" trick
  keeps the ceiling by itself; for manual edits an AI agent checks it with the MCP tool `plan_check`.
- **The voice has as many notes as the line has syllables.** Stressed syllables go on strong beats.
- **The sum of durations in a bar equals the time signature**: with `M:4/4` and `L:1/32` — 32 units. If there are more or
  fewer bars, everything after the edit shifts.
- **No empty lines inside the plan** — neither after `K:` nor between sections: an empty line cuts off the melody. This was noticed
  in generations; the app code does not check it.
- **Accidentals on notes** — in a minor key with flats in the key signature, it is safer to put an explicit sign on every note (`=`, `^`, `_`).

## How it connects to the rest

- If you do not like the vocal delivery, describe the character in the "Voice" slot ("low male, half-spoken", "rough, straining") and
  the era in "Sound (recording)" ("1960s garage", "raw grunge 1991"); you can pick a voice in the voice fitting room.
- **The melody of an existing song** — "From a track…" in the plan window: SheetSage2 turns the track into a plan, then you edit it.
- **Lyrics for a cover** — you paste them yourself: the app does not recognize or translate lyrics.
- **A plan longer than the sound** happens: such bars are dimmed in the roll.
