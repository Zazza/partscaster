// Английские названия и описания цепочек эффектов, наборов педалей и подписей ручек (Go-сторона,
// internal/dsp, отдаёт их по-русски). Ключ цепочки и набора — id; подпись ручки — по русскому тексту (у разных
// цепочек одинаковые ручки подписаны одинаково). Перевод — в одной точке: api.dspChains()/api.dspPresets().

export const CHAIN_EN = {
  'wall': ['Wall / noise / grit', 'A loudness monolith, grit in the highs, a noise sheet.'],
  'wall-lite': ['Light wall', 'The same chain, gentler: a bit denser and dirtier, without destruction.'],
  'tape': ['Cassette', 'Wow/flutter, rolled-off highs, pink hiss — home tape.'],
  'alive': ['Liveliness', 'Micro-detune, breathing loudness, compressor pumping — from plastic to “played by hand”.'],
  'grit': ['Dirt / overdrive', 'Saturation in the limiter, bitcrush, grit — garage and tube rasp.'],
  'breathe': ['Breathing', 'Expand the neural mix “brick”: quiet places a bit quieter, loud ones as they were — the track is less tiring.'],
  'master': ['Mastering', '“Like a real record”: expands the brick, raises loudness with saturation, adds grit to the highs — in one pass. The effects section has a one-click button for it.'],
  'gap': ['Silence', 'A full pause of a couple of seconds from the mark — a lull before a fill/chorus; soft edges.'],
  'tempo-from': ['Speed up from the mark', 'From the chosen second the track is faster without a pitch change — speed up the finale (the model does not perform tempo changes in the plan).'],
  'drop': ['Drop', 'From the mark the track slows down and fades, a pause, then it rises from silence at the normal tempo — a “drop → run-up” before the final chorus. The track gets longer by the stretch and the pause.'],
  'cresc': ['Louder towards the end', 'A smooth loudness rise from the chosen second — the finale sounds bigger; the limiter holds the peaks.'],
  'soften': ['Soften sibilance', 'De-esser: tames ringing and hissing parts of the voice, leaves the body of the voice alone. Better on the “vocals” stem than on the whole track (otherwise the cymbals get quieter too).'],
  'dewhistle': ['Remove whistle', 'A narrow notch at the whistle/squeal frequency (and its harmonics) in the chosen window — the music around barely changes. “Find whistle” suggests the frequency.'],
  'warp': ['Warped tape', 'Deep wow and flutter, a dull top — a tape defect as a device.'],
  'level': ['Album loudness', 'Only loudness by the set dB and a peak limiter at the ceiling — no saturation, grit or expansion: bring the album tracks to one loudness, same timbre and dynamics.'],
  'gate': ['Rhythm gate', 'Loudness opens and closes in time (bpm × hits per beat, grid from the offset): a sustained sound — a pad, a sustaining guitar — becomes a pulsing synth rhythm on the same chords. Put it on a stem (usually “other”) in a window.'],
  'megaphone': ['Megaphone', 'The voice through a horn: a narrow band, overdrive, a short room echo, like in garage rock. A chain for the “vocals” stem — the stem is picked automatically.'],
  'phone': ['Telephone', 'The voice from a phone receiver: an even narrower band, dry, a slight rasp. A chain for the “vocals” stem — the stem is picked automatically.'],
  'voice-drive': ['Vocal overdrive', 'Lo-fi vocal overdrive: clipping, bitcrush, a dull top, tape noise, like on 80s punk records. A chain for the “vocals” stem — the stem is picked automatically.'],
  'slapback': ['Slapback', 'A single 80–120 ms echo — rockabilly and garage rock: the voice with a repeat. A chain for the “vocals” stem — the stem is picked automatically.'],
  'reverb-room': ['Reverb: room', 'A small room: dense early reflections, a short tail — the instrument “in a room”, not in a vacuum. On the voice — as an effect on the “vocals” stem.'],
  'reverb-hall': ['Reverb: hall', 'A big hall: the tail builds up and fades for a long time — volume and depth for slow parts and the voice.'],
  'reverb-plate': ['Reverb: plate', 'A studio plate: an instantly dense bright tail without reflections — a classic for the voice and the snare.'],
  'reverb-spring': ['Reverb: spring', 'A guitar spring reverb: a narrow band and a metallic “boing” — surf, rockabilly, dub.'],
  'delay': ['Tempo delay', 'Repeats on a beat division of the track (1/4, 1/8, dotted 1/8…), fading with feedback, ping-pong between channels, the top of the repeats cut. “Find grid” suggests the tempo.'],
  'width': ['Stereo width', 'Wider or narrower stereo image via mid/side; the low end stays in the center. 0 — mono.'],
  'haas': ['Doubling (Haas)', 'One channel 10–30 ms later: one sound, but wide — a “double” voice or guitar.'],
  'chorus': ['Chorus', 'Several slightly “drifting” copies: the sound is wider and thicker, like several players.'],
  'flanger': ['Flanger', 'A comb that sweeps across the spectrum: a “jet” whoosh on guitars, cymbals, synths.'],
  'phaser': ['Phaser', 'A soft “sway” of spectral notches — 70s psychedelia on keys and guitar.'],
  'tremolo': ['Tremolo', 'A smooth sine loudness sway (no edges, unlike the Rhythm gate) — a vintage amp.'],
  'eq': ['Equalizer', 'Low and high shelves, a mid bell: adjust the timbre of a stem or the track. 0 dB — the band is off.'],
  'sweep': ['Filter sweep', 'From the mark the cutoff moves over N seconds — a “run-up before the chorus” (low cut going up) or “into the pillow” (high cut going down). After the window the sound is dry again or holds the final frequency.'],
  'autowah': ['Auto-wah', 'A band-pass filter swings between two frequencies — “wah-wah” on guitar or keys (a sweep, without following loudness).'],
  'fade': ['Fade in / fade out', 'A smooth fade-in from the start of the track and/or a fade-out from the mark to silence.'],
  'multiband': ['Multiband compressor', 'Three bands with their own compressor: denser and more even, a loud bass does not squash the voice and cymbals.'],
  'ducking': ['Ducking from drums', 'The stem ducks on every drum hit — a “pumping” mix, the kick cuts through. Only as an effect on a stem (usually “other” or bass): the key is the track’s drums.'],
  'pitch': ['Transpose', 'Higher or lower by semitones without a tempo change (rubberband), the voice formants are kept.'],
  'octaver': ['Octaver', 'Mixes in a copy an octave lower and/or higher — a thicker bass, an “organ” voice.'],
  'reverse': ['Reverse to the mark', 'Before the mark a piece plays backwards: a reverse cymbal “sucked” into the hit, or a reverse tail. The track length does not change.'],
  'stutter': ['Stutter', 'From the mark one grid beat repeats N times in a row — an electronic “stutter” before a fill.'],
  'tape-stop': ['Tape stop', 'From the mark the sound slows down and drops in pitch to a stop, like a switched-off tape deck; then the track goes on.'],
  'vinyl': ['Vinyl', 'Record crackle and clicks, quiet noise, a slightly closed top.'],
  'od-ts': ['Overdrive (Tube Screamer)', 'A warm overdrive with a mid hump: the guitar comes forward and does not boom in the lows — blues, classic rock, solos.'],
  'fuzz-muff': ['Fuzz (Big Muff)', 'A thick fuzz with huge sustain and scooped mids — a wall of guitars, shoegaze, stoner.'],
  'dist-rat': ['Distortion (RAT)', 'A hard harsh distortion, the “filter” cuts the top — punk, grunge, noise.'],
  'fuzz-octave': ['Octave fuzz', 'A fuzz with an octave up (like Hendrix’s Octavia): ringing, “synth-like” on solos.'],
  'boost': ['Booster', 'A clean loudness lift and a bit of top; before an overdrive — denser and meaner.'],
  'univibe': ['Uni-Vibe', 'A slow “viscous” phaser sway — Hendrix, Gilmour, psychedelia.'],
  'ringmod': ['Ring modulator', 'The sound is multiplied by a tone: instead of notes — metallic sums and differences, a “robot”, bells.'],
  'noise-gate': ['Noise gate', 'Silence between notes: removes hum and overdrive noise in the pauses, the notes stay intact.'],
  'comp-pedal': ['Compressor (pedal)', 'Quiet and loud notes are more even, notes sustain longer — country, funk, clean parts.'],
  'cab': ['Cabinet', 'A guitar speaker: cuts the lows and highs a cabinet does not have — the overdrive sounds like a guitar, not a buzz.'],
}

export const DSP_PRESET_EN = {
  'hendrix': ['Hendrix', 'Octave fuzz → Uni-Vibe → cabinet → a bit of room: viscous psychedelic rock.'],
  'shoegaze': ['Shoegaze', 'Big Muff → chorus → long delay → huge hall: a wall of guitars the voice drowns in.'],
  'surf': ['Surf', 'Tremolo → lots of spring: Dick Dale, Tarantino soundtracks.'],
  'grunge': ['Grunge', 'RAT → light chorus → cabinet: dirty, heavy, the early 90s.'],
  'dub': ['Dub', 'A thick low end → a delay with long dark feedback → spring: Jamaican dub.'],
  'postpunk': ['Post-punk', 'Compressor → chorus → short delay → plate: cold ringing 80s guitars.'],
  'clean-space': ['Clean + space', 'Compressor → delay → hall: a clean guitar in a big space — ambient, ballads.'],
}

export const LABEL_EN = {
  'эксайтер (песок верхов)': 'exciter (grit in the highs)',
  'стена (ниже = монолитнее)': 'wall (lower = more monolithic)',
  'шумовое полотно 0.5–9к': 'noise sheet 0.5–9k',
  'с какой секунды (0 — весь трек)': 'from which second (0 — whole track)',
  'wow (завывание ленты)': 'wow (tape wobble)',
  'шипение': 'hiss',
  'срез верхов, кГц': 'high cut, kHz',
  'микро-детюн (живость высоты)': 'micro-detune (pitch liveliness)',
  'дыхание громкости': 'loudness breathing',
  'насос компрессора': 'compressor pumping',
  'зерно верхов': 'grain in the highs',
  'перегруз': 'overdrive',
  'биткраш (ломкость)': 'bitcrush (brittleness)',
  'песок верхов': 'grit in the highs',
  'сила (0 — выкл, 1 — заметно)': 'strength (0 — off, 1 — noticeable)',
  'перегруз (громкость)': 'overdrive (loudness)',
  'разжатие тишины (0 — выкл)': 'expand the quiet parts (0 — off)',
  'с какой секунды тишина': 'silence from which second',
  'длина паузы, с': 'pause length, s',
  'мягкость краёв, мс': 'edge softness, ms',
  'с какой секунды быстрее': 'faster from which second',
  'во сколько раз быстрее': 'how many times faster',
  'с какой секунды тормозить': 'slow down from which second',
  'сколько секунд тормозить (исходного звука)': 'slow down for how many seconds (of the source)',
  'темп в конце торможения (1 — без замедления)': 'tempo at the end of the slowdown (1 — no slowdown)',
  'тишина после, с': 'silence after, s',
  'нарастание после тишины, с (0 — сразу полный звук)': 'rise after the silence, s (0 — full sound at once)',
  'откуда нарастать, дБ': 'rise from, dB',
  'с какой секунды расти': 'rise from which second',
  'за сколько секунд': 'over how many seconds',
  'на сколько дБ громче': 'how many dB louder',
  'сила (0 — выкл, 1 — сильно)': 'strength (0 — off, 1 — strong)',
  'предел ослабления (0–1)': 'attenuation limit (0–1)',
  'с каких частот (0 — ниже, 1 — только верх)': 'from which frequencies (0 — lower, 1 — top only)',
  'частота свиста, Гц (найти — «найти свист»)': 'whistle frequency, Hz (find — “find whistle”)',
  'ещё тон, Гц (0 — нет)': 'another tone, Hz (0 — none)',
  'глубина выреза, дБ': 'notch depth, dB',
  'ширина выреза, Гц': 'notch width, Hz',
  'гармоники (1 — только сам тон)': 'harmonics (1 — the tone only)',
  'с какой секунды': 'from which second',
  'по какую секунду (0 — до конца)': 'until which second (0 — to the end)',
  'варп (завывание)': 'warp (wobble)',
  'флаттер (дрожь)': 'flutter (tremble)',
  'громкость, дБ (+ громче, − тише)': 'loudness, dB (+ louder, − quieter)',
  'потолок пиков, дБ': 'peak ceiling, dB',
  'темп трека, BPM': 'track tempo, BPM',
  'ударов на долю (2 — восьмые, 4 — шестнадцатые)': 'hits per beat (2 — eighths, 4 — sixteenths)',
  'доля открытого звука в ударе': 'gate length (share of each hit)',
  'глубина (1 — полная тишина между ударами)': 'depth (1 — full silence between hits)',
  'сетка: время любой сильной доли, с («найти сетку»)': 'grid: time of any downbeat, s (“find grid”)',
  'низ полосы, Гц': 'band low, Hz',
  'верх полосы, кГц': 'band high, kHz',
  'насыщение': 'saturation',
  'звон рупора': 'horn ring',
  'короткое эхо': 'short echo',
  'сухой/обработанный (доля эффекта)': 'dry/wet (effect share)',
  'хрип трубки': 'receiver rasp',
  'перегруз, дБ': 'overdrive, dB',
  'плотность середины (1.5 кГц), дБ': 'mid density (1.5 kHz), dB',
  'срез низа, Гц': 'low cut, Hz',
  'верх плёнки, кГц': 'tape top, kHz',
  'шум ленты': 'tape noise',
  'задержка повтора, мс': 'repeat delay, ms',
  'громкость повтора': 'repeat level',
  'хвост, с': 'tail, s',
  'предзадержка, мс': 'pre-delay, ms',
  'верх хвоста, кГц': 'tail top, kHz',
  'громкость хвоста (сухой не убавляется)': 'tail level (dry is not reduced)',
  'ширина хвоста (0 — моно)': 'tail width (0 — mono)',
  'темп трека, BPM («найти сетку»)': 'track tempo, BPM (“find grid”)',
  'доля: 1 — 1/4, 2 — 1/8, 3 — 1/8 с точкой, 4 — 1/16, 5 — 1/4 с точкой': 'division: 1 — 1/4, 2 — 1/8, 3 — dotted 1/8, 4 — 1/16, 5 — dotted 1/4',
  'обратная связь (громкость следующего повтора)': 'feedback (level of the next repeat)',
  'пинг-понг L/R (0 — нет, 1 — да)': 'ping-pong L/R (0 — no, 1 — yes)',
  'верх повторов, кГц': 'repeats top, kHz',
  'громкость повторов': 'repeats level',
  'ширина (0 — моно, 1 — как есть, 2 — шире)': 'width (0 — mono, 1 — as is, 2 — wider)',
  'моно ниже, Гц (0 — нет)': 'mono below, Hz (0 — none)',
  'задержка второго канала, мс': 'second channel delay, ms',
  'какой канал позже (0 — правый, 1 — левый)': 'which channel is later (0 — right, 1 — left)',
  'сухой/удвоенный (доля эффекта)': 'dry/doubled (effect share)',
  'глубина, мс': 'depth, ms',
  'скорость, Гц': 'rate, Hz',
  'базовая задержка, мс': 'base delay, ms',
  'обратная связь, %': 'feedback, %',
  'глубина': 'depth',
  'глубина (1 — до тишины)': 'depth (1 — down to silence)',
  'низ, дБ': 'low, dB',
  'низ: частота полки, Гц': 'low: shelf frequency, Hz',
  'середина, дБ': 'mid, dB',
  'середина: частота, Гц': 'mid: frequency, Hz',
  'середина: добротность (больше — уже)': 'mid: Q (higher — narrower)',
  'верх, дБ': 'high, dB',
  'верх: частота полки, Гц': 'high: shelf frequency, Hz',
  '0 — срез верха (НЧ-фильтр), 1 — срез низа (ВЧ-фильтр)': '0 — high cut (low-pass), 1 — low cut (high-pass)',
  'частота в начале, Гц': 'start frequency, Hz',
  'частота в конце, Гц': 'end frequency, Hz',
  'после свипа: 0 — вернуть сухой звук, 1 — держать конечную частоту': 'after the sweep: 0 — back to dry, 1 — hold the end frequency',
  'скорость качания, Гц': 'sweep rate, Hz',
  'нижняя точка, Гц': 'low point, Hz',
  'верхняя точка, Гц': 'high point, Hz',
  'нарастание с начала, с (0 — нет)': 'fade-in from the start, s (0 — none)',
  'затухание с секунды (0 — нет)': 'fade-out from second (0 — none)',
  'длина затухания, с': 'fade-out length, s',
  'плотность (0 — выкл)': 'density (0 — off)',
  'раздел низ/середина, Гц': 'low/mid split, Hz',
  'раздел середина/верх, Гц': 'mid/high split, Hz',
  'порог ключа (ниже — чаще приседает)': 'key threshold (lower — ducks more often)',
  'глубина (степень сжатия)': 'depth (compression amount)',
  'атака, мс': 'attack, ms',
  'восстановление, мс': 'release, ms',
  'полутоны (+ выше, − ниже)': 'semitones (+ higher, − lower)',
  'октава вниз': 'octave down',
  'октава вверх': 'octave up',
  'отметка (реверс заканчивается здесь), с': 'mark (the reverse ends here), s',
  'длина реверса, с': 'reverse length, s',
  'что реверсировать: 0 — кусок до отметки, 1 — кусок после (реверс-тарелка)': 'what to reverse: 0 — the piece before the mark, 1 — the piece after (reverse cymbal)',
  'громкость реверса': 'reverse level',
  '0 — поверх звука, 1 — вместо': '0 — over the sound, 1 — instead',
  'с какой секунды (на сильную долю)': 'from which second (on a downbeat)',
  'сколько раз повторить кусок': 'how many times to repeat the piece',
  'за сколько секунд остановиться': 'stop over how many seconds',
  'треск': 'crackle',
  'шум': 'noise',
  'верх, кГц': 'top, kHz',
  'тон (верх), кГц': 'tone (top), kHz',
  'громкость, дБ': 'level, dB',
  'сустейн (перегруз), дБ': 'sustain (drive), dB',
  'тон (0 — бас, 1 — верх)': 'tone (0 — bass, 1 — treble)',
  'фузз, дБ': 'fuzz, dB',
  'октава вверх (доля)': 'octave up (share)',
  'яркость (верх), дБ': 'brightness (top), dB',
  'сухой/обработанный (1 — вибрато, 0.5 — хорус)': 'dry/wet (1 — vibrato, 0.5 — chorus)',
  'частота модуляции, Гц': 'modulation frequency, Hz',
  'порог, дБ (тише — глушится)': 'threshold, dB (quieter — muted)',
  'закрытие, мс': 'release, ms',
  'сустейн (сила сжатия)': 'sustain (compression strength)',
  'атака, мс (больше — щелчок медиатора проходит)': 'attack, ms (more — the pick click gets through)',
  'верх динамика, кГц': 'speaker top, kHz',
  'середина «коробки», дБ': '“box” mid, dB',
}

// русские имена наборов педалей (как в internal/dsp/presets.go) — для подписи правки по id на русском интерфейсе
export const DSP_PRESET_RU = {
  'hendrix': 'Хендрикс', 'shoegaze': 'Шугейз', 'surf': 'Сёрф', 'grunge': 'Гранж', 'dub': 'Даб',
  'postpunk': 'Пост-панк', 'clean-space': 'Чисто + простор',
}

// pedalPresetName — имя набора педалей по id на языке интерфейса ('' — неизвестный id).
export function pedalPresetName(id, locale) {
  if (locale === 'en') return (DSP_PRESET_EN[id] || [])[0] || ''
  return DSP_PRESET_RU[id] || ''
}

// предупреждения, которые Go-сторона пишет по-русски (internal/studio)
export const WARNING_EN = {
  'гитара YuE2 — у трека нет плана': 'YuE2 guitar — the track has no plan',
}

const en = (locale) => locale === 'en'

// localizeChains — на английском цепочки и подписи их ручек получают английский текст; неизвестное — как пришло.
export function localizeChains(list, locale) {
  if (!en(locale) || !Array.isArray(list)) return list
  return list.map((c) => {
    const t = c && CHAIN_EN[c.id]
    const params = Array.isArray(c?.params) ? c.params.map((p) => (LABEL_EN[p.label] ? { ...p, label: LABEL_EN[p.label] } : p)) : c?.params
    return t ? { ...c, name: t[0], note: t[1], params } : { ...c, params }
  })
}

// localizeDspPresets — наборы педалей (internal/dsp/presets.go).
export function localizeDspPresets(list, locale) {
  if (!en(locale) || !Array.isArray(list)) return list
  return list.map((p) => {
    const t = p && DSP_PRESET_EN[p.id]
    return t ? { ...p, name: t[0], note: t[1] } : p
  })
}

// localizeWarning — известные русские фразы предупреждений Go-стороны по-английски, в том числе внутри строки
// («<пресет>: гитара YuE2 — у трека нет плана» у пресета из рецепта); прочее — как есть.
export function localizeWarning(w, locale) {
  if (!en(locale) || typeof w !== 'string') return w
  let out = w
  for (const [ru, enText] of Object.entries(WARNING_EN)) out = out.split(ru).join(enText)
  return out
}
