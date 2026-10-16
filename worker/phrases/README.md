# Guitar phrases for the "Instruments" page

Short clean guitar recordings (pickup signal, no amp or effects). On the "Instruments" page a phrase loops while
you turn the knobs of a sound-engine chain, so you hear the chain on a real guitar. Each file is exactly 4 bars
at its own tempo, mono, 16-bit FLAC, loudness matched to −20 dBFS RMS.

**Source:** GuitarSet — Q. Xi, R. M. Bittner, J. Pauwels, X. Ye, J. P. Bello, "GuitarSet: A Dataset for Guitar
Transcription", ISMIR 2018. Subset `audio_mono-pickup_mix`, <https://zenodo.org/records/3371780>.
**License:** [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/). Changes: excerpts cut, DC offset removed,
loudness matched.

| File | GuitarSet recording | Start, s | Length, s | Tempo, BPM |
|---|---|---|---|---|
| gtr-arpeggio.flac | 05_SS3-98-C_comp_mix.wav | 0.000 | 9.796 | 98 |
| gtr-strum.flac | 00_Rock3-148-C_comp_mix.wav | 6.486 | 6.486 | 148 |
| gtr-sustain.flac | 00_BN3-119-G_solo_mix.wav | 2.017 | 8.067 | 119 |
| gtr-shred.flac | 00_Rock1-90-C#_solo_mix.wav | 0.000 | 10.667 | 90 |
| gtr-funk.flac | 00_Funk2-119-G_comp_mix.wav | 22.185 | 8.067 | 119 |
| gtr-jazz.flac | 02_Jazz3-137-Eb_comp_mix.wav | 15.766 | 7.007 | 137 |

Next to each recording lies `<name>.notes.json`: the phrase's notes (start, end, MIDI pitch, velocity), beat
times and tuning offset in cents. The `gtrnotes` block of the sound engine plays from these notes
(`worker/phrases.py`, `notes_file` and `gtrnotes_chain`). Names and tempos of the phrases are listed in
`GUITAR` in `worker/phrases.py`; a new phrase needs a line there and a FLAC file of whole bars; without a notes file the `gtrnotes` block
has nothing to play on that phrase.

All files in this directory ship with the worker archive (`tools/worker-tar.sh` copies `worker/phrases/*`).
