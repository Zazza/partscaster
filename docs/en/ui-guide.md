# Interface guide
**English** · [Русский](../ru/ui-guide.md)

What is where in the Partscaster window and what each button does. The order follows the work: create a track,
pick a good take in the list, finish it in the studio, mix the sound.

## App window

On the left is a fixed panel: the section menu and the job queue. On the right is the open section or the track
studio. At the bottom is the player. There is no header: sections open from the menu on the left, and you also
leave the studio with a menu item.

<!-- screenshot: the whole window — left panel with menu and queue, track list, player at the bottom -->

### Left panel

From top to bottom:

- **Logo and name** — a click opens the [“What it is / What's new”](#what-it-is--whats-new) window.
- **Worker address** (the GPU machine that runs the neural network) with a status dot. If the worker does not
  respond — a red dot and "worker unavailable".
- **Video memory** — a bar and "used/total GB". Next to it, if present: "queued: N", "GPU queue: N"
  and the number of the running generation with a percentage.
- **Section menu**: "New track", "track list", "voices", "instruments", "sound presets", "settings".
  The open section is highlighted. A menu item closes the studio if it is open.
- **Queue** — takes the rest of the panel, described below.
- **Bottom row**: " / " — collapse the panel into a narrow strip with icons, sun/moon — theme,
  EN/RU — interface language.

When collapsed, you see the menu icons, the percentage of used video memory and a counter of unfinished jobs on
the "track list" icon.

### Queue

Everything long that the worker does, in one list in order of execution: generations, presets, rebuilds, stem
separation, effects, notes, lyrics recognition, voice change, auditions. It includes jobs started from the app
and jobs started by an AI agent through MCP — these are marked "AI".

- At the top is the running job (a generation shows a percentage), then the waiting ones, below — the last
  5 results.
- Results: "done → #N" (a click opens this version in the studio), "done", "error" (the text is in the
  tooltip), "canceled", "interrupted" (whoever started it stopped responding, or the worker restarted).
- ✕ on any unfinished job — cancel, with confirmation. A waiting job is simply removed from the queue. A running
  job stops: stem separation, voice change and generation — at once, everything else — after the current step.
- ▲ / ▼ on a waiting job — earlier / later.
- The pause button in the "Queue" header — no new jobs start for anyone (neither the app nor the AI),
  the running one finishes. Press again — "Resume the queue".
- An empty queue shows "empty — all quiet".

### Player at the bottom of the window

One for the whole app, whatever is open: ⏮ — previous finished track, ▶/⏸ — play/pause,
■ — stop, ⏭ — next finished track, the title (a long one scrolls), a seek bar with minute marks
(on hover you see where a click will jump), time, volume. ▶ in a track list row and the listen buttons in the
studio play through this same player.

### Version banner

If the app and worker versions do not match, a warning appears at the top of the window. ✕ — hide it.

## New track

The "New track" menu item opens the "New composition" page. After "Queue", "×5 seeds" or "✦ draft"
the track list opens, and the track is already in the queue. The filled-in form stays as it is while the app is
open. ✕ in the header or Esc — back to the track list.

<!-- screenshot: the "New composition" form — style library row, slots, lyrics, buttons at the bottom -->

### Style library

The row above the form:

- **"— group —"** and **"— style —"** — two lists. Ready styles are grouped by movement (post-punk, punk,
  shoegaze, techno…); a movement has several variants that differ in substance: rhythm, guitars, voice,
  recording. The selected style is spread into the form slots at once; after that you can edit it.
- **"1:1"** — apply the style as an exact line, without spreading it into slots.
- **"save"** — the "Style name" window: the current form style (slots and line as they are) goes into
  "my styles".
- **"my styles"** — a window with all your styles: click — apply to the form, ✕ — delete (built-in ones
  are not affected).

### Form fields

- **"Title"** — your own track title; if empty, the track is shown by its number.
- **"Import ABC"** — paste a ready note plan (ABC is a text notation for music) as the track skeleton: bars,
  chords and structure come from it, while genre, instruments, rhythm and delivery come from the form. An
  attached plan is shown as the line "plan attached · N lines", "detach" — remove it.
- **Style slots** — "Song language", "Style", "Rhythm (drums and bass)", "Guitars", "Keys and more", "Voice",
  "Mood", "Sound (recording)" and **BPM**. You type freely; hints appear under the field.
  On submit, known options are replaced with English wording (the model was trained on English tags);
  write your own words in English — Russian ones are not translated.
- **"saved voice — descriptor + seed"** — a voice from the [voice lab](#voice-lab): it fills the "Voice" slot
  and sets the card's seed.
- **"Style as a single line"** (collapsed) — the line built from the slots and the rack. If you type your own,
  it wins over the slots ("· overridden manually"); "follow the slots" — return to automatic building.
- **"Instrument rack (N)"** (collapsed) — instrument groups with checkboxes; a checked one has an effect choice.
  Checked items are added to the style automatically.
- **Lyrics** — your own song lyrics; lines with `#` are your notes, they are not sung. Above the field:
  - "no vocals" — instrumental: no lyrics needed, the voice in the slots changes to "no vocal" (turn it off and
    the style's voice comes back, unless you typed your own). For a "1:1" style, "Instrumental, no vocals" is
    added to the start of the line. A length option for the instrumental appears next to it: "by lyrics",
    "1–2 min", "2–5 min", "5–10 min", "10–30 min (needs more than 16 GB VRAM)"; the length is approximate;
  - section tag buttons: "Verse", "Pre-chorus", "Chorus", "Bridge", "Intro", "Outro", "Interlude" (the
    tooltip shows what will be inserted). A click inserts `[Verse]`, `[Pre-Chorus]`, `[Chorus]`, `[Bridge]`,
    `[Intro]`, `[Outro]` or `[Interlude]` at the cursor on its own line; selected text is replaced. In
    "no vocals" mode the buttons are hidden;
  - next to them, a hint: "Tags are optional: without them YuE2 lays out the parts itself — separate stanzas
    with an empty line." Checked: with and without tags the song came out in the same form (intro, verse,
    chorus, interlude, verse, chorus, outro); only without tags the choruses were shorter.
- **"style for the model"** — the final line that goes to the model. If Russian words remain in it, a red line
  appears under it: "The style has Russian words: YuE2 understands tags in English — write them in English."
  This does not block submitting: the line is sent as is.

### Advanced

A collapsed block; its header shows a summary of what is selected:

- **seed** — empty: a random seed (it is saved in the track, "seed N" in the details); a number — repeat: the
  same seed, style and lyrics give the same track.
- **"plan before sound"** — what the model writes before the sound: "melody with chords" (default, the harmony
  is tighter) or "melody only". The studio needs the plan: notes, tricks, drama.
- **"drama"** — "flat", "build-up" (quiet start, growth towards the final chorus), "waves" (calm
  verses, powerful choruses), "burst" (heavy slow start, a collapse before the finale, the final chorus an octave
  higher).
- **"character"** — two performance knobs: "boldness" (model temperature: higher — the playing is more original
  but dirtier; default 1.0) and "precision" (cfg: higher — clearer, closer to the notes, cleaner sound and voice;
  default 1.5). "default" — reset. This is not "better/worse", it is a different character; a stretch for an
  insert and a continuation take the parent's character.

### Sound presets and launch

- **"Sound presets:"** — choose up to three [sound presets](#sound-presets). When the track is ready and the
  GPU queue is free, the app itself makes versions with these presets; the raw version stays the main one.
  This works while the app is open. Presets are not applied to a draft.
- **"Queue"** — one track.
- **"×5 seeds"** — five tracks with neighboring seeds: pick the best one.
- **"notes →"** — the "Track notation (ABC)" window (below): the attached plan or a new plan from the style.
- **"✦ draft"** — a short version of ~15–20 s: hear the style before the full render.

### The "Track notation (ABC)" window

For advanced users: the usual path is styles and the studio, you do not need to come here. The window opens with
the "notes →" button in the form and with the "notes (ABC)" item in the track menu.

- you edit the ABC text right in the window; "ABC cheat sheet" — a short guide to notes, bars, chords and rests;
- "→ to the form" — attach the plan to the form: the skeleton comes from ABC, everything else from the form;
  launch with the main form button;
- "New plan" — generate a plan from the current style and lyrics;
- "From a track…" — a note transcription of your audio file: a cover based on someone else's melody or your own.

## Track list

The main page: all tracks in a table of compact rows. Derived items (stretches for inserts, rebuilds, variants)
are hidden under their track. While there are no tracks — the "New track" and "import track" buttons.

<!-- screenshot: track list with one expanded row — details, versions, buttons -->

### Filters

The panel above the list sticks to the top: status ("in progress", "finished", "errors and canceled"), date
("today", "7 days", "30 days"), length ("sample <1 min", "1–3 min", "3–6 min", ">6 min"), drafts ("drafts only",
"no drafts"), folder ("Album" (Альбом), "Basics" (Основы), "Experiments" (Эксперименты), your own, "no folder"),
search "title · style · number", the **"import recipe"** and **"import track"** buttons. Under the panel — "showing N of M", the page
number and "Collapse all" if several rows are expanded. A long list is paged with the ← → arrows at the bottom.

### Track row

Number, ▶/⏸ for a finished track (an unfinished one has a status dot), the title with marks ("draft", "queued",
"error", ⚠ — a voice is heard in an instrumental; details in the tooltip), a progress bar during generation,
✎ — rename (your own title instead of the number: the number changes when you move to another machine), a counter
of versions and attachments, duration, creation date ("01.10 17:26", the year if it is not the current one),
folder, ▸/▾. A click on the row expands the details.

### Track details

- **Info line**: "seed N", the yue2.cpp engine badge (experiment), "plan: melody only", character,
  "custom ABC", "main: #N" (if another version is chosen as the main one).
- **Folder** — an icon; click — a list: "no folder", folders, "new folder…" (name and Enter). The song's versions
  move together with it.
- A track that is queued or in progress has "cancel". An unfinished one has ✕ "Delete the result" and ⟲ "Repeat
  with these parameters". An error shows the error text and "↻ retry".
- A finished track has:
  - **"studio →"** — open the main version in the [studio](#studio);
  - **"⤓ download ▾"** — flac, mp3 (320 kbps), "notes (abc)";
  - **"⋯"** — "in browser" (a listening page with seeking), "notes (ABC)" (the notes window), "new track
    like this" (the form with the track's parameters), "replay in another style…", "Export recipe", "delete"
    (with confirmation).
- **Style** — by parts: language, genre, rhythm, guitars, voice, mood, sound and so on.
- **Sound preset rows**, if they were ordered in the form: ⏳ waiting for the queue, ⟳ being applied, ✓ → version
  #N, ✕ reason. ↻ — retry.

### Versions and attachments

Under the details is a flat list: the original, versions (rebuilds, continuations, re-sings, "album voice",
variant tracks), mixes with inserts and material (stretches for inserts, stretch checks). Each row has: ▶,
☆ "make main" (you keep working with the main version: ▶ in the track row plays it and "studio →" opens it),
number, title, role, "studio →", "↻ retry" (for failed ones), ✕.

### Replay in another style

The "replay in another style…" item in the "⋯" menu. The neural network plays the track again from its notes and
lyrics in a new style — a quick cover without the studio (a new generation, a few minutes).

- "genre" — a list with search ("— as it was —" — do not change), "parts" — part buttons, "in your own words";
- "vocal" — "track lyrics", "no vocal" or "own lyrics";
- Russian words in "in your own words" are not translated: the same warning as in the form appears under the
  field ("The style has Russian words…");
- "result" — "cover — a new version of the song" (a separate version in the song's group) or "over the original"
  (the new performance is quieter, on top of the source; "layer volume"; the version appears in the studio's
  "Done" step);
- "performance" — "closer to the original" (the track's seed) or "new interpretation";
- "replay" — launch. The new version is named "<track> · replayed: <genre>".

### Track recipe: export and import

A recipe is a small file with the rules of the song, without audio: with it, the song can be made on another
installation.

- **"Export recipe"** in the "⋯" menu of a finished track — save the file. The app lists what was not included.
- **"import recipe"** above the list — the "Import track recipe" window: choose a file ("Another file…" — a
  different one), look at the title and the number of steps, "Import". The performance will be very close, but not
  identical; sound edits are repeated exactly. If the model on this worker is different, the window offers a mode:
  "by seed — like the original" or "by note plan" (the same notes, a new performance).
- Import progress is shown as a line above the list; on error — "Retry" and "Dismiss".

### Importing your own track

A track made elsewhere (another generator, your own recording) can be worked on in the studio. The
**"import track"** button is above the list next to "import recipe", and in an empty list — next to "New track".
You can also import a track through MCP (the `import_track` tool).

- Choose a file (flac, mp3, wav, ogg, m4a) — the track is uploaded to the worker, the app takes the score right
  away, and the track opens in the studio. While the import runs, the button shows "importing…".
- Stems, minus, effects and notes work with the track. The track itself does not change, everything goes into new
  versions.
- If there is not enough video memory for the score, the track is still imported and the app says so. Stems,
  minus and effects work; you can make the score later with the "Create score" button in the studio.

## Studio

The studio opens with the "studio →" button on a track or version and with the "done → #N" result in the queue.
It is one page, and the sections follow the work: **1 Listen · 2 Edit · 3 Sound · 4 Done**. The step bar sticks
to the top and highlights the current section; on its right are "#N track title", the active track badge and the
sample kit download progress ("downloading kit…"). To leave — any menu item on the left.

Above the steps is the line "BPM · key · meter · N bars (тактов) · ~duration" and the selected stretch
("a–b s selected" (выделено a–b с)).

For a track without a score (note transcription) the studio works without the piano roll and bar tricks: the
waveform, stems, sound and "Done" work as usual. At the top there is a message and a **"Create score"** button
(transcription, up to a few minutes); after it the roll and tricks appear. An imported track has no internal model data,
so "continue from here" and "re-sing from here" are not available for it.

<!-- screenshot: studio — step bar, wave with a selection, plan roll -->

**Active track** — one for the whole studio. You choose it by clicking a track name in the "Stems" desk or in
any "what to apply to" list (volume line, effects, pedals); the wave and spectrum show it, and the volume line,
effects and pedals work on it. The step bar shows "Track: bass" and ✕ (or click again) — back to the whole track.
For a drum part (kick, snare…) the wave shows the part, but edits apply to all the drums.

### 1 Listen

**Wave** ("sound · amplitude / spectrum"):

- click — listen from this spot, drag — select a stretch, wheel — zoom at the cursor,
  shift+wheel or horizontal scroll — move, "⟲" — fit the whole track in the window;
- the file list — "main track", track variants, stems, "minus";
- "amplitude" (volume: you see hits, dips, entries) or "spectrum" (a frequency picture from the worker, needs
  ffmpeg on the GPU machine);
- "snap to bars" (on) — selection edges snap to the plan's bars;
- "wave on top" (on) — the wave sticks under the step bar and stays visible in "Sound" and "Done";
- "color ▾" — wave color;
- the plan's bars and sections are drawn over the wave.

**"volume"** (pencil icon) — a volume line over the wave for the whole track or one stem. This shows a
"what to apply to" list, "reset", "section ▲ / ▼" (with a selection) and "apply line". Click — a point on the
line, drag a point — bend the line, double or right click — delete a point. Dragging across the wave selects a
stretch; "section ▲/▼" raises or lowers it by 2 dB with smooth edges, the rest of the line does not change.
Limits: +12 dB, −30 dB, the dashed line is 0 dB. Whole track — a new track variant; a stem — an entry in "Track
edits" (a new line for the same stem replaces the old one, an empty one removes it).

**Plan** ("plan · bars × voices, chords") — one scrollable row of bars: voice and instruments, note density,
chords, sections. "− / +" — bar width. Dragging the mouse across bars selects them (the same as selecting on the
wave). Bars with accumulated tricks are marked in orange.

### 2 Edit

Tricks change the plan of the selected stretch. They accumulate without generation; the sound changes only in new
versions. The hint under the tricks says what to do next: select a stretch → pick a trick → rebuild. The original
never changes.

Trick groups:

- **listen** — "▶ original selection": how this stretch sounds now.
- **harmony** — "darker", "brighter", "tenser": a different chord in the selected bars.
- **arrangement** — "drop-out (silence)" (the bars go silent for all voices), "cut bars", "sparser verses" (in
  all verses guitars and keys −6 dB, drums −3 dB — the chorus opens up more).
- **tempo** — "↑ faster" / "↓ slower": from the selected spot ±10 %, it accumulates.
- **voice** (if the selection has singing):
  - "vocal ↑ oct." / "vocal ↓ oct.";
  - "voice higher" — the same melody a third higher, with more push;
  - "voice: variations" — the rhythm and anchor notes stay, phrase ends move to neighboring notes (press again —
    wider);
  - "voice: hold phrase ends" — the last note of a phrase is held into the rest after it;
  - "↻ re-sing from here" — the voice of the selected part is sung again with these tricks (2 takes), the music
    and the voice outside the part do not change; in a few minutes new ones appear among the versions — make the
    best one the main one. It is better to end the part in a vocal rest, otherwise you get a seam warning. Voice
    notes are not raised above the ceiling (the top of the plan melody + 2 steps): above that the model squeaks in
    falsetto. The model barely hears delivery notes in the style ("strain", "rasp") — a lively voice is made with
    the melody.
- **instrument** — a list and "add". Instruments (electric guitar, strings, piano, organ, synth lead, flute,
  saxophone, bells, bass, drum fill, build-up, harder): the selected stretch is re-rendered in the track's style,
  and only the stem of this instrument is inserted into the track. Volume without generation: "drums only",
  "drums louder", "guitars quieter", "bass quieter", "voice: stop" (the voice in the selection is silent, the music
  goes on).
- **continue from here** — the field "what to change in the sound" (in English, optional) and "↻ redo from here":
  the track before the selection stays the same, after it the model plays anew. Volume edits and effects of this
  version do not carry over to the continuation.
- **build**:
  - "▶ stretch with edits" — the selected spot with edits in the context of the track: the model continues the
    track from the start of the selection following the edited plan and plays to the end of the selection (+2 s);
    playback starts 4 s before the edit and begins automatically;
  - "⟳ rebuild the track" — one generation with all edits (the same style, lyrics and seed); when ready it plays
    in the studio;
  - "✦ draft" — a quick test of the track's start with edits (~18 s);
  - "⟳ instrumental (no vocals)" — the same music without the vocal part;
  - "remove in selection" and "reset edits" — remove the marks and plan edits.

Trick marks carry over to new versions.

**Track edits** — everything that accumulates into a rebuild without generation: inserts, mutes, effects and
pedals on a stem, volume lines, sound chains, parts, stereo placement, master. Each edit has:

- a checkbox — turn on/off (a turned-off edit stays in the list, the track is rebuilt without it);
- the edit's "volume" slider;
- "✓ in time" (the part is aligned to the track's kick) or "by plan" (alignment failed, a shift of up to ~0.1 s is
  possible);
- render variants 1…N and "↻ another take" — the same stretch with a different seed;
- stereo placement — for parts;
- "edit" on a sound chain — opens it in "Stems", where the button becomes "replace";
- ✕ — delete (with confirmation).

The block header has "▶ mix with edits". With no edits — "No edits — the original plays."

**Album voice** (collapsed, "experiment") — the voice of this track is sung with the timbre of another song's
voice, the music and melody stay the same: choose a "reference song" (search by title or number) → "sing". In a
few minutes — a new version "album voice (exp.)". The voice is recognizable, but it wobbles, and the music
suffers a little inside phrases. It needs the optional Seed-VC component on the worker
([deployment.md](deployment.md)); without it the block says it is unavailable.

### 3 Sound

**Stems** — a desk: one row for each stem of the track (voice, drums and their parts, bass, guitar, keys,
"other").

- No stems — "make stems" (the separation model comes from the settings). If the track is split into only 4
  stems — "split in more detail": again with the model from the settings (RoFormer gives drum parts, guitar and
  keys), edits stay.
- **"rhythm section from kits"** — all drum parts and the bass with samples from one kit, in one rebuild (toms —
  by pitch); the window is the selection, with no selection — the whole track. Next to it are the kit ("Live
  acoustic", "808 drum machine", "909 drum machine", "80s sample machine (Linn)", "Early rhythm box (CR-78)",
  "80s electronic drums (Simmons)") and the drum part treatment ("dry", "room", "80s gated reverb", "tight",
  "lo-fi").
- In a stem row: ▶ — the stem alone; the name (click — make it active, tooltip — what this sound is:
  hi-hat — small cymbals "ts-ts", ride — a big ringing cymbal…); the loudness of the loud parts relative to the
  track, marked "quiet" or "barely audible" (moving or processing such a stem is useless — listen to it first);
  icons of the stem's edits (a turned-off one is crossed out); **stereo placement** — "pan", "width", "center"
  (when you release the slider, the track is rebuilt); **"sound ▾"**.
- "sound ▾" opens the sound choice: "Instrument" → "Kind" → "Sound (type to search)" (extra lists are hidden),
  "fine-tune" — chain blocks with knobs (as on the ["Instruments"](#instruments) page), "stem only",
  "▶ after" / "▶ before" on the selection (no selection — 15 s from the cursor) and **"into track"** — an entry in
  "Track edits", the track is rebuilt without a shift.
- **Minus**: "in the minus:" — turn off the stems you do not need (drums, bass, other, voice) → "minus track":
  the rest are mixed into a new file without regeneration.

<!-- screenshot: the "Stems" desk — stem rows, "sound ▾" open on the bass -->

**Synth over chords** (collapsed) — a new part played by a vintage synth on top of the track, following the plan's
chords in time with the sound. "read chords" → "parts:" (song parts as buttons), synth (Solina, Juno, Moog, CS-80,
Farfisa, Vox Continental, VL-Tone and your own), style ("pad (chords)", "arpeggio", "pulse (eighth bass)", "drone
(held)"), "octave", "level", stereo placement, "synth sound and effects" (blocks), "▶ after", "▶ part alone" (hear
what to look for in the mix), "▶ before", "into track". Stem separation is not needed. If you cannot hear the
synth — raise the octave: in the guitar register the part blends with them.

**Percussion on the grid** (collapsed) — hi-hat, shaker, tambourine, claps, cowbell or a doubled kick on top of the
track. "find the grid" (with a plan — bars and song parts; without a plan — bars from the tempo, and then "bar
starts at beat": if "on 2 and 4" landed on 1 and 3 — shift it), song parts, sound, pattern ("every beat",
"eighths", "sixteenths", "on 2 and 4", "off-beats"), "swing", "accent", "level", stereo placement, "percussion
sound and effects", "▶ after / ▶ part alone / ▶ before", "into track". Several parts — several edits.

**Quick effects (ffmpeg)** — a different tool: an effect on the whole track or a stretch, preview in place. It is
easier to change the sound of a single stem in the "Stems" desk. Two collapsed blocks:

- **Audio effects** — 50+ effects. "effects chain…" → "what to apply to" (whole track, vocals, drums, bass,
  guitars/synths, and with detailed separation — guitar, keys, kick, snare, toms, hi-hat, ride, crash) → knobs →
  "apply". On a stem the effect applies only in the selected spot (no selection — to the whole stem) and
  accumulates in "Track edits"; on the whole track — a new variant in "Done". "preview spot" — the effect on the
  selected stretch (no selection — 15 s from the cursor), it is not added to the variants; "▶ after ⇄ / ▶ before ⇄"
  — the same stretch with and without the effect, from the same second; "stem only" — the stem without the rest of
  the mix. "De-whistle" (Убрать свист) has "find whistle" (looks for narrow tones in the selection and fills in the
  frequency), the delay, stutter and rhythm gate have "find grid" (tempo and downbeat). "track metrics" — a window
  with sound measurements. All effects and their knobs — [effects.md](effects.md).
- **Pedals** — guitar effects in order: "what to apply to" (usually the guitar — chosen automatically), "preset…"
  ("Hendrix" (Хендрикс), "Shoegaze" (Шугейз), "Surf" (Сёрф)…), "+ pedal", ●/○ — on/off, ← → — order, ✕; a click
  on a pedal — its knobs. "preview spot", "▶ after ⇄ / before ⇄", "apply". "compare presets" — the current board and
  up to three presets on one stretch, switching from the same second. Details —
  [effects.md — pedals](effects.md#pedals-and-pedalboard).

### 4 Done

**Master** — glue and loudness of the whole mix as the last step. "presets:" — "Master: streaming −14 LUFS",
"Master: loud −11 LUFS", "Master: soft glue"; "loudness" and "to target" (LUFS is the perceived loudness of the
track: −14 — like streaming services, −11 — loud), "limiter", peak "ceiling" (dBTP), "master chain" (blocks),
"▶ after / ▶ before" on the mix with edits, "into track" — one "Master" edit (a new one replaces the old one,
"now:" shows the current one). After the rebuild — the line "After master: −14.0 LUFS, peak −1.0 dBTP".

**Sound presets** — a recipe for processing the whole track. Choose a preset (a list with search) → **"apply to
track"**: no stems — the track is split (drum parts — with the RoFormer model), the stems are rebuilt, the final
result is a new version "track · preset" (a few minutes, the job goes through the queue).

- Under the selected preset — a description and a volume slider for each of its edits (bass louder, voice
  quieter…): the preset is applied with them, the preset itself does not change. If you changed them — "as in
  preset" (reset) and "save as mine" (a new preset of your own with these levels).
- **"preview"** — a rebuild of the stem volumes from the sliders, with the track edits but without the preset's
  processing: seconds.
- **"save edits as preset"** — the preset includes "whole track" edits with a sound chain, an effect or
  pedals, and the track master ("master included"). A counter shows how many will be included and how many are
  skipped (edits with a window, inserts, mutes and volume lines are not included). Title, description, "final
  chain on the mix:" — "none" or the same as another preset.

What is inside presets and how to edit them — the ["Sound presets"](#sound-presets) section.

**results** — one-click buttons at the top, all track versions below.

- **"one-click mastering"** — the "Mastering" (Мастеринг) chain (unsquash the "brick", loudness through
  saturation, top-end grit) on the whole track, saved at once as a separate version "title · Мастеринг (medium)".
- **"one-click breathing"** — unsquash the "brick" of a neural mix: quiet parts a little quieter, loud parts as
  they were.
- The level **"light / medium / strong"** for both buttons: "light" — less top-end grit (if the voice rings or
  squeaks), "strong" — louder and denser; the level is written into the version name.
- **"album loudness…"** — opens the "Album loudness" (Громкость альбома) effect in "Audio effects": bring the
  album's tracks to one loudness.
- The version list — all of them, intermediate ones too: newest at the top, a clear name and date, the full
  caption in the tooltip. Each has: ▶, ⤓ (save flac), **"→ to tracks"** (→ в треки; make it a separate track in
  the list), ✕.
- "details: metric numbers" — changes in sound measurements for each version and "full delta" (полная дельта).

## Sound presets

The "sound presets" menu item. A sound preset is a recipe for processing the whole track: stem processing, drum
replacement, balance, parts and a final chain → a new version of the track. Presets are applied not here but in
the new track form and in the studio (the "Done" step).

<!-- screenshot: the "Sound presets" page — an expanded family and an expanded preset -->

**Library.** Presets are grouped by family — "Rock" (Рок), "Heavy" (Тяжёлое), "Electronic" (Электроника), "Pop
and more" (Поп и другое), "Mine" (Мои); a family expands on click, and the number of movements is shown next to
it. There are 109 built-in presets: 34 movements (post-punk, cold wave, goth, indie, shoegaze, metal, synth-pop,
trip-hop, house, soul…) with 3 variants each, plus 7 more. Each built-in preset replaces the drums with a kit,
changes the bass and guitar, sets the stem balance by measurement, like in real recordings, adds parts and a
master; it does not touch the voice. Except for electronic music, the guitar is played by the guitarist following
the plan's chords, with its own style for each variant (without a track plan — the neural network's guitar). The
preset sets the loudness of the result itself: most built-in presets target −14 or −11 LUFS, some −16.

A preset row has:

- ▸ — what is inside: stem edits, "final:", "result loudness: N LUFS", "tuned on track #N";
- ▶ "listen on the demo" — apply the preset to the demo and play the result at once (a few minutes; progress is
  shown as a line at the top of the page);
- a built-in preset has "Save as my own": it opens the editor with a copy (the built-in preset itself does not
  change);
- your own preset has "preset editor", a pencil (rename and edit the description), ✕ (delete, with
  confirmation).

**"new preset"** — the editor with an empty recipe.

**Preset editor** — a window for full editing of the recipe:

- title and description ("what the sound is like");
- **"reference:"** — what to listen on: the family demo ("Rock", "Heavy", "Electronic", "Pop and more") or your own
  track from the list; "stem only", "▶ before" and "window a–b s" — the stretch used for listening;
- **"Track processing"** — the recipe entries. Each has: ▶ "after", ✕, which stems it applies to (buttons),
  volume, "loudness to the track" (the stem's loudness target by measurement when applied), stereo placement and
  "edit the chain" (blocks with knobs);
- **catalog** — stem tabs (vocals, drums, bass, guitar, keys, guitars/synths): each processing has ▶ (listen on
  the reference) and "+" (add). Drums have **"Rhythm section as a kit"**: kit, treatment and "add the rhythm
  section";
- **"Master — the whole track"** — ready masters, "loudness" with a LUFS target, "▶ after", "master chain";
- "final chain on the mix:" — the same as another preset;
- "Parts (N) — carried over unchanged" — synth and percussion parts from the source preset;
- drum parts are heard only with the RoFormer stem model — without it the editor warns you, and the entry stays
  in the preset;
- at the bottom — "Save" (for a copy — "Save as my own"), **"Apply to the reference"** (a full run of the preset
  on the reference, a few minutes; the finished version plays at once) and "Cancel". Closing with unsaved edits
  asks for confirmation.

**Demo reference** — a short genre draft for each family. The app creates it the first time you listen
("preparing the demo track…", then "splitting the demo into stems…") and keeps it in the hidden folder "Demo"
(Демо) — it is not visible in the general track list. If it failed — "create again".

## Instruments

The "instruments" menu item. A short phrase plays in a loop through the worker's sound engine (pedals → amp →
cabinet → space): turn the controls and hear at once what changes. No track is needed here — track processing is
done in the studio, in the "Stems" desk.

- **"Instrument", "Kind", "Sound (type to search)"** — three lists. Instruments: guitar, bass, drums, percussion,
  keys and synths, orchestra, processing. Kind — how the sounds differ: for guitar — clean, overdrive, space,
  through an amp; for bass — a different sound for the same notes, a sound for the genre, replacing notes with
  samples, a synth as a new part; for drums — a kit (live, 808, 909, Linn, CR-78, Simmons); for orchestra —
  strings, woodwinds, brass, bells and marimba, "Bass → orchestra", "Melody → orchestra"; for keys and synths —
  piano and electric piano, organs, pads and strings, leads, toys. If there is only one kind, the list is hidden.
  A hint about the sound appears on hover and under the lists. Voice and master are not shown here — they belong
  in the studio.
- Guitar has the kind "Guitarist fits in by chords" — a nylon, semi-acoustic or another electric guitar plays its
  own part: chords from the track plan, strokes like the neural network's guitar; the style (strumming,
  fingerpicking, bass + chord, fifths or auto by song part) is different for each sound. The track needs a plan.
- **"phrase"** — what plays: for guitar, recordings of clean guitar (fingerpicking, strumming, long notes, a
  shred, funk, jazz chords; GuitarSet, CC BY 4.0), for bass — eighths and long notes (a real bass guitar if the
  Growlybass kit is on the worker, otherwise synth bass), for drums — a rock beat with a fill and a slow one with
  ride, for synth — a playing style over the chords Em C G D, percussion — on top of a drum beat.
- **"tempo"** — ×0.5…×1.5; guitar recordings are stretched without changing pitch, artifacts are heard at the
  extremes.
- **"play in a loop"** — the phrase plays endlessly; any edit is recalculated a quarter of a second after the last
  movement and plays from the same point in the loop (with an amp, the first calculation takes up to 15 s).
  "stop" — stop it; closing the page also stops it.
- **"processing"** (checkbox) — turn off the chain and hear the same phrase dry. The loudness is matched:
  you compare the sound, not "louder means better".
- **Blocks** are edited with knobs: the checkbox turns a block off, ↑/↓ — order, ✕ — remove, "+ block" — add.
  The equalizer has bands ("+ band").
- **Amp (NAM)** and the cabinet or room **IR** — from uploaded files: "upload…" — your own `.nam` or
  `.wav` (none are included). Without an IR the cabinet is "light" (top end cut), the reverb is built-in. Drum
  blocks have "download kit" for samples.
- **Your own instruments** — "save as mine…" → title → "save": the current settings become your own instrument
  (stored on the worker). It is at the bottom of its group, marked "(mine)"; when your own is selected —
  "save" (replace its settings with the current ones), "rename", "delete". In the studio (the "Stems" desk,
  "Synth over chords", "Percussion on the grid") your own instruments are offered together with the ready ones.

## Voice lab

The "voices" menu item. Choosing the character of a voice: an audition of ~18 s on the same neutral backing track,
so that voices are compared under equal conditions. A voice you like is saved as a card.

<!-- screenshot: voice lab — voice knobs, "in the vein of:", voice cards -->

- **"presets:"** — "operatic", "dark drama", "scream", "whisper", "velvet low", "reset".
- **"in the vein of:"** — Letov, Thom Yorke, Jack White, Placebo, Interpol, Zemfira, Mumiy Troll, Cobain, Nick
  Cave, Amy Winehouse, Björk, Lou Reed, Vysotsky, BG: they set the knobs for that manner.
- **Knobs**: "register" (female, male, high male, low male), "roughness", "creak", "delivery", "breath".
- **"custom"** — timbre and manner tags in English: "whiskey-soaked", "nasal", "slurred", "child-like".
  Russian words are not translated: under the line for the "Voice" slot, the same warning as in the form
  appears ("The style has Russian words…"). The model does not reproduce subtle speech defects (a burr, a lisp).
- **seed** — empty: a new one for each audition; a number — lock the voice (the same seed and knobs — the same
  voice).
- Under the knobs — the line that goes into the "Voice" slot.
- **"audition"** — a ~18 s sample (rendering on the GPU takes 1–2 minutes), it plays automatically. While it
  renders you can turn the knobs — the sample will not change.
- After the sample — a name and **"save voice"**.

**Voice cards** — saved voices: description, seed, ▶ (if the sample still exists, otherwise "source deleted"),
"re-audition" (a new sample with these settings), ✕.
**"clear probes"** — delete all audition samples from the track list; the cards stay.

A saved voice is chosen in the new track form — with the "saved voice — descriptor + seed" list: it fills the
"Voice" slot and sets the card seed.

## Settings

The "settings" menu item. Everything is applied with the "Save" button at the bottom of the page.

- **"Worker connection"** — "Worker address". "Check" — check the address in the field without saving.
  You can enter just an IP: port 8091 is added. Both the app and the MCP server use this address.
- **"Checks"**:
  - "Split instrumental tracks into stems right away" — an instrumental is split into stems after generation
    (~7 s of GPU), and if something sounds in the voice stem, the track gets ⚠ "voice in instrumental";
  - "stem separation" — "fast — Demucs (~20 s per track)" or "cleaner — RoFormer (~1.5 min per track)".
    RoFormer cuts out voice, guitars and keys more cleanly and splits drums into parts; its weights are
    non-commercial (CC BY-NC-SA). It applies to new separations. If it is not installed on the worker, the page
    tells you where to read ([deployment.md](deployment.md)).
- **"Generation engine"** (if the worker supports it) — "Main (PyTorch)" or "yue2.cpp — experimental". yue2.cpp
  has "model", "Download" (download progress as a line; after it — "Save") and "Economy mode (8 GB)": ~4.5 GB of
  video memory, songs up to ~3.5 min. A different engine with the same seed gives a different song; notes and
  continuations carry over. Tracks from this engine are marked with a badge in the track details.
- **"Worker (information only)"** — data folder, whisper (what a finished track sings — for an AI agent, MCP `job_lyrics`), separation model;
  below — "App version".
- **"For AI agents (Claude Code, Claude Desktop, Codex)"** — the MCP server partscaster-mcp gives an agent the
  app's tools: generation, studio, effects; it takes the worker address from these settings. The path to
  partscaster-mcp is shown (if not found — a hint to build it: `make mcp`). **"Connect to Claude Code"** — after
  confirmation it runs `claude mcp add` for all of the user's projects. Below are ready configuration snippets for
  Claude Code (a terminal command), Claude Desktop (`claude_desktop_config.json`) and Codex
  (`~/.codex/config.toml`) with a "Copy" button. Details — [mcp.md](mcp.md).

<!-- screenshot: settings — the "For AI agents" block -->

## What it is / What's new

The window opens with a click on the logo. It appears by itself on the first launch (the "What it is" tab) and
after an update (the "What's new · version" tab) — the news of a new version is shown even if the checkbox is set.

- **"What it is"** — the idea of the app in five points: notes first, editing in place, nothing is lost, sound
  ready for release, an AI assistant.
- **"What's new"** — the list of changes in the current version.
- **"Don't show on start"** and **"Got it"** — close.
