// Группы стилей для двухуровневого комбобокса «группа → стиль»: группа — течение, стиль — его вариант.
// Варианты одного течения различаются по сути (ритм, гитары, голос, запись) и уводят YuE2 от его клише.
// Без названий групп и артистов; «фоновых» и попсовых жанров для быстрого нейрослопа нет — их можно
// собрать вручную в полях стиля. «Без слов» — отметка формы, а не отдельный стиль.
// Откалиброванные пресеты переиспользуются из presets.js по id.
import { presets } from './presets.js'

const byId = (id) => presets.find((p) => p.id === id)

// Общие слоты для инструментальных жанров: вокал не нужен.
const instr = (over = {}) => ({
  language: 'English',
  vocals: 'instrumental, no vocals',
  ...over,
})


export const groups = [
  {
    id: 'postpunk', name: 'Пост-панк', nameEn: 'Post-punk',
    items: [
      {
        id: 'cold-bass-postpunk', name: 'Холодный: мелодичный бас ведёт', nameEn: 'Cold: melodic bass leads',
        style: 'English, dark post-punk, cold 1979 minimalism, melodic high-register bass guitar playing the lead melody, mechanical tight drums with cavernous reverb, sparse thin trebly guitar, icy string synth pads, deep deadpan baritone male vocals, bleak isolation, 140 BPM',
        slots: { language: 'English', genre: 'post-punk, cold wave', rhythm: 'mechanical tight, cavernous', guitars: 'sparse thin trebly, bass carries the melody', keys: 'icy string synth pads', vocals: 'deep deadpan baritone male', mood: 'bleak isolation, despair', production: 'cavernous reverb, spacious raw 1979' }, bpm: 140,
      },
      {
        id: 'dark-arpeggio-postpunk', name: 'Тёмный: сплетённые арпеджио гитар', nameEn: 'Dark: interlocking guitar arpeggios',
        style: 'English, dark post-punk, two interlocking minor-key arpeggiated guitars, deep propulsive bass, metronomic drums, deadpan baritone male vocals, doomed urban romance, 130 BPM',
        slots: { language: 'English', genre: 'post-punk, post-punk revival', rhythm: 'metronomic driving', guitars: 'interlocking minor arpeggios, staccato chords', keys: '', vocals: 'deadpan baritone male', mood: 'doomed romance, night city', production: 'dry tight, punchy modern' }, bpm: 130,
      },
      {
        id: 'angular-postpunk', name: 'Угловатый: сухие рваные гитары, танцевальный бас', nameEn: 'Angular: dry jagged guitars, danceable bass',
        style: 'English, angular post-punk, art punk, jerky disco-punk drums, dry funky bassline, scratchy angular guitar stabs, choppy and dry, nervous speak-sung male, ironic, twitchy tension, sardonic, dry close room, no reverb, 1980 independent label, 150 BPM',
        slots: { language: 'English', genre: 'angular post-punk, art punk', rhythm: 'jerky disco-punk drums, dry funky bassline', guitars: 'scratchy angular guitar stabs, choppy and dry', keys: '', vocals: 'nervous speak-sung male, ironic', mood: 'twitchy tension, sardonic', production: 'dry close room, no reverb, 1980 independent label' }, bpm: 150,
      },
    ],
  },
  {
    id: 'coldwave', name: 'Холодная волна', nameEn: 'Cold wave',
    items: [
      {
        id: 'coldwave-machine', name: 'Синтетическая: драм-машина и синт-бас', nameEn: 'Synthetic: drum machine and synth bass',
        style: 'English, coldwave, minimal synth, cheap drum machine, steady mechanical pulse, thin chorus guitar far behind, monophonic synth bass, icy synth lead, distant detached baritone, monotone, cold loneliness, nocturnal, home 4-track, tape hiss, small cold reverb, 125 BPM',
        slots: { language: 'English', genre: 'coldwave, minimal synth', rhythm: 'cheap drum machine, steady mechanical pulse', guitars: 'thin chorus guitar far behind', keys: 'monophonic synth bass, icy synth lead', vocals: 'distant detached baritone, monotone', mood: 'cold loneliness, nocturnal', production: 'home 4-track, tape hiss, small cold reverb' }, bpm: 125,
      },
      {
        id: 'coldwave-tape', name: 'Плёночная: советский магнитофонный андеграунд', nameEn: 'Tape: Soviet underground recording',
        style: 'Russian, soviet coldwave, 1980s tape underground, stiff drum machine, plodding bass, out-of-tune clean guitar, flanger, cheap organ synth drone, flat gloomy male, half-spoken, grey apathy, kitchen melancholy, worn cassette dub, muffled lo-fi, 118 BPM',
        slots: { language: 'Russian', genre: 'soviet coldwave, 1980s tape underground', rhythm: 'stiff drum machine, plodding bass', guitars: 'out-of-tune clean guitar, flanger', keys: 'cheap organ synth drone', vocals: 'flat gloomy male, half-spoken', mood: 'grey apathy, kitchen melancholy', production: 'worn cassette dub, muffled lo-fi' }, bpm: 118,
      },
    ],
  },
  {
    id: 'goth', name: 'Готик-рок', nameEn: 'Gothic rock',
    items: [
      {
        id: 'gothic-rock', name: 'Плотный: катящийся бас и живые барабаны', nameEn: 'Dense: rolling bass and live drums',
        style: 'English, gothic rock, chorus-drenched rolling bass, dark brooding atmosphere, dramatic deep male vocals, cavernous reverb, 120 BPM',
        slots: { language: 'English', genre: 'gothic rock, darkwave', rhythm: 'rolling processional', guitars: 'chorus-drenched, shimmering dark', keys: '', vocals: 'dramatic deep male', mood: 'brooding romantic darkness', production: 'cavernous 1985' }, bpm: 120,
      },
      {
        id: 'goth-machine', name: 'Машинный: драм-машина и флэнжер', nameEn: 'Machine: drum machine and flanger',
        style: 'English, gothic rock, deathrock, drum machine tom patterns, galloping bass, flanger and chorus guitar, sharp treble, sepulchral baritone, dramatic, morbid romance, nocturnal ritual, cavernous gated reverb, 1983, 135 BPM',
        slots: { language: 'English', genre: 'gothic rock, deathrock', rhythm: 'drum machine tom patterns, galloping bass', guitars: 'flanger and chorus guitar, sharp treble', keys: '', vocals: 'sepulchral baritone, dramatic', mood: 'morbid romance, nocturnal ritual', production: 'cavernous gated reverb, 1983' }, bpm: 135,
      },
    ],
  },
  {
    id: 'punk', name: 'Панк', nameEn: 'Punk',
    items: [
      {
        id: 'punk-77', name: 'Британский 77-го: средний темп, издёвка', nameEn: 'British \'77: mid-tempo, sneering',
        style: 'English, 1976 London pub rock turning punk, not pop-punk, mid-tempo 4/4, loose live drums, root-note bass, 1950s rock and roll riffs played rough, raw overdriven chords, no solos, sneering snarled male, London accent, sarcastic, bored contempt, garage live room, dry, no polish, 135 BPM',
        slots: { language: 'English', genre: '1976 London pub rock turning punk, not pop-punk', rhythm: 'mid-tempo 4/4, loose live drums, root-note bass', guitars: '1950s rock and roll riffs played rough, raw overdriven chords, no solos', keys: '', vocals: 'sneering snarled male, London accent, sarcastic', mood: 'bored contempt', production: 'garage live room, dry, no polish' }, bpm: 135,
      },
      {
        id: 'punk-modern', name: 'Современный злой: давящие барабаны, грязный бас ведёт', nameEn: 'Modern angry: pounding drums, dirty bass leads',
        style: 'English, modern noisy punk, post-hardcore, not pop-punk, pounding tom-heavy drums, distorted overdriven bass driving the riff, abrasive noisy guitar stabs, feedback, barked shouted male, thick accent, spoken-shouted, furious, cathartic, defiant, loud dry room, bass-heavy, raw, 140 BPM',
        slots: { language: 'English', genre: 'modern noisy punk, post-hardcore, not pop-punk', rhythm: 'pounding tom-heavy drums, distorted overdriven bass driving the riff', guitars: 'abrasive noisy guitar stabs, feedback', keys: '', vocals: 'barked shouted male, thick accent, spoken-shouted', mood: 'furious, cathartic, defiant', production: 'loud dry room, bass-heavy, raw' }, bpm: 140,
      },
      {
        id: 'punk-lofi-hc', name: 'Лоу-фай хардкор: быстро, с органом, как в подвале', nameEn: 'Lo-fi hardcore: fast, with organ, basement-style',
        style: 'English, lo-fi hardcore punk, garage punk, fast d-beat, sloppy pounding, buzzsaw fuzz guitar, blown-out, cheap combo organ riff, hoarse yelled male, buried in the mix, unhinged sweaty chaos, blown-out 4-track tape, basement recording, 175 BPM',
        slots: { language: 'English', genre: 'lo-fi hardcore punk, garage punk', rhythm: 'fast d-beat, sloppy pounding', guitars: 'buzzsaw fuzz guitar, blown-out', keys: 'cheap combo organ riff', vocals: 'hoarse yelled male, buried in the mix', mood: 'unhinged sweaty chaos', production: 'blown-out 4-track tape, basement recording' }, bpm: 175,
      },
      {
        id: 'ska-punk', name: 'Ска-панк: гитара на слабую долю и духовые', nameEn: 'Ska punk: offbeat guitar and horns',
        style: 'English, two-tone ska punk, not pop-punk, fast skank beat, offbeat upstroke guitar chops, walking bass, brass section riffs with trumpet and trombone, rough energetic male vocals, sweaty live club, raw horns up front, 165 BPM',
        slots: { language: 'English', genre: 'two-tone ska punk, not pop-punk', rhythm: 'fast skank beat, walking bass', guitars: 'offbeat upstroke guitar chops', keys: 'brass section riffs with trumpet and trombone', vocals: 'rough energetic male vocals', mood: 'sweaty live club', production: 'raw horns up front' }, bpm: 165,
      },
      {
        id: 'horror-punk', name: 'Хоррор-панк: мрачно и с хором', nameEn: 'Horror punk: dark with gang chorus',
        style: 'English, horror punk, spooky catchy punk, galloping horror riff, ghoul-chorus backing vocals, campy monster-movie imagery, 160 BPM',
        slots: { language: 'English', genre: 'horror punk', rhythm: 'galloping fast', guitars: 'spooky horror riffs', keys: '', vocals: 'snarling male with ghoul choir', mood: 'campy spooky fun', production: 'raw 1980' }, bpm: 160,
      },
    ],
  },
  {
    id: 'garage', name: 'Гаражный рок', nameEn: 'Garage rock',
    items: [
      {
        id: 'garage-rock', name: 'Сырой: хруст и крик', nameEn: 'Raw: crunch and screams',
        style: 'English, garage rock, raw crunchy guitars, simple fast energetic beat, shouted sloppy vocals, two-minute song energy, 145 BPM',
        slots: { language: 'English', genre: 'garage rock', rhythm: 'simple fast energetic', guitars: 'raw crunchy chords', keys: 'cheap organ stabs', vocals: 'shouted, sloppy, urgent', mood: 'reckless energy', production: 'raw garage' }, bpm: 145,
      },
      {
        id: 'garage-blues-duo', name: 'Блюзовый дуэт: слайд и барабаны, без баса', nameEn: 'Blues duo: slide and drums, no bass',
        style: 'English, garage blues rock duo, slide guitar in open tuning, no bass guitar, pounding heavy drums, stomping fuzz riffs, raw passionate male vocals, frantic swagger, 120 BPM',
        slots: { language: 'English', genre: 'garage blues', rhythm: 'pounding stomping', guitars: 'open-tuning slide, fuzz riffs', keys: '', vocals: 'raw passionate male, howling', mood: 'frantic swagger', production: 'raw analog two-piece' }, bpm: 120,
      },
      {
        id: 'garage-60s', name: '60-х: фузз и орган', nameEn: '60s: fuzz and organ',
        style: 'English, 1960s garage rock, proto-punk, stomping beat, tambourine, fuzz box riff, twangy reverb, Farfisa combo organ, snotty howling male, teen rebellion, wild, mono 1966 garage, tape saturation, 140 BPM',
        slots: { language: 'English', genre: '1960s garage rock, proto-punk', rhythm: 'stomping beat, tambourine', guitars: 'fuzz box riff, twangy reverb', keys: 'Farfisa combo organ', vocals: 'snotty howling male', mood: 'teen rebellion, wild', production: 'mono 1966 garage, tape saturation' }, bpm: 140,
      },
    ],
  },
  {
    id: 'altrock', name: 'Альт-рок и гранж', nameEn: 'Alt-rock & grunge',
    items: [
      {
        id: 'grunge', name: 'Гранж: тихо-громко', nameEn: 'Grunge: quiet-loud',
        style: 'English, 1990s grunge, detuned distorted guitars, loud-quiet-loud dynamics, heavy fuzz bass, apathetic angsty male vocals, raw unpolished production, 110 BPM',
        slots: { language: 'English', genre: 'grunge', rhythm: 'loud-quiet dynamics, plodding to explosive', guitars: 'detuned distorted, thick fuzz', keys: '', vocals: 'apathetic angsty male, cracked', mood: 'angst, resignation', production: 'raw unpolished 1991' }, bpm: 110,
      },
      {
        id: 'chorus-wall-alt', name: 'Стена гитар с хорусом', nameEn: 'Wall of chorused guitars',
        style: 'English, alternative rock, chorus-drenched buzzing guitar walls, glam edge, driving straight beat, ringing melodic riffs, tense high androgynous male vocals, bittersweet intoxicating romance, 125 BPM',
        slots: { language: 'English', genre: 'alternative rock, glam-tinged', rhythm: 'driving straight', guitars: 'chorus-drenched walls, ringing riffs', keys: '', vocals: 'tense high androgynous male', mood: 'bittersweet intoxicating romance', production: 'polished 1998 alt-rock' }, bpm: 125,
      },
      {
        id: 'art-rock-chords', name: 'Арт-рок: необычные аккорды', nameEn: 'Art rock: unusual chords',
        style: 'English, art rock, unusual guitar harmony with sus2 and add9 colors, clean delayed arpeggios, paranoid melancholy, quiet-loud dynamics, occasional 5/4 passage, anxious high male vocals, 110 BPM',
        slots: { language: 'English', genre: 'art rock, alternative', rhythm: 'shifting, breathing, dynamic', guitars: 'clean sus/add9 chords, delay arpeggios', keys: '', vocals: 'anxious high male, falsetto leaps', mood: 'paranoid melancholy, alienation', production: 'wide modern alt-rock' }, bpm: 110,
      },
    ],
  },
  {
    id: 'indie', name: 'Инди-рок', nameEn: 'Indie rock',
    items: [
      {
        id: 'indie-rock', name: 'Звонкий: чистые гитары', nameEn: 'Jangly: clean guitars',
        style: 'English, indie rock, jangly clean guitars, driving upbeat drums, earnest youthful male vocals, bittersweet bright mood, 135 BPM',
        slots: { language: 'English', genre: 'indie rock', rhythm: 'driving upbeat', guitars: 'jangly clean arpeggios', keys: '', vocals: 'earnest youthful male', mood: 'bittersweet bright', production: 'clean modern indie' }, bpm: 135,
      },
      {
        id: 'indie-slacker', name: 'Небрежный: шумные гитары, ленивый голос', nameEn: 'Slacker: noisy guitars, lazy vocals',
        style: 'English, lo-fi indie rock, slacker rock, loose mid-tempo drums, sloppy, detuned noisy fuzz, off-kilter leads, lazy deadpan male, half-mumbled, ironic apathy, bittersweet, lo-fi home recording, tape hiss, 112 BPM',
        slots: { language: 'English', genre: 'lo-fi indie rock, slacker rock', rhythm: 'loose mid-tempo drums, sloppy', guitars: 'detuned noisy fuzz, off-kilter leads', keys: '', vocals: 'lazy deadpan male, half-mumbled', mood: 'ironic apathy, bittersweet', production: 'lo-fi home recording, tape hiss' }, bpm: 112,
      },
    ],
  },
  {
    id: 'shoegaze', name: 'Шугейз и дрим-поп', nameEn: 'Shoegaze & dream pop',
    items: [
      {
        id: 'shoegaze-wall', name: 'Стена: гитары плывут, голос тонет', nameEn: 'Wall: guitars swim, vocals drown',
        style: 'English, shoegaze, steady driving drums buried in the wash, walls of reverse reverb, tremolo-bar warble, dense fuzz layers, breathy buried vocals, more texture than words, blissful haze, overwhelmed, loud blurry wall of sound, no clear edges, 120 BPM',
        slots: { language: 'English', genre: 'shoegaze', rhythm: 'steady driving drums buried in the wash', guitars: 'walls of reverse reverb, tremolo-bar warble, dense fuzz layers', keys: '', vocals: 'breathy buried vocals, more texture than words', mood: 'blissful haze, overwhelmed', production: 'loud blurry wall of sound, no clear edges' }, bpm: 120,
      },
      {
        id: 'dreampop', name: 'Дрим-поп: прозрачно и медленно', nameEn: 'Dream pop: transparent and slow',
        style: 'English, dream pop, slow drum machine, soft pulse, chiming chorus guitar, long delays, glassy synth pad, airy ethereal female, weightless longing, huge reverb, soft focus, 92 BPM',
        slots: { language: 'English', genre: 'dream pop', rhythm: 'slow drum machine, soft pulse', guitars: 'chiming chorus guitar, long delays', keys: 'glassy synth pad', vocals: 'airy ethereal female', mood: 'weightless longing', production: 'huge reverb, soft focus' }, bpm: 92,
      },
    ],
  },
  {
    id: 'postrock', name: 'Пост-рок', nameEn: 'Post-rock',
    items: [
      {
        id: 'postrock-crescendo', name: 'Нарастание: от тишины до стены', nameEn: 'Crescendo: from silence to a wall',
        style: 'Instrumental, no vocals, post-rock, slow build from silence to pounding climax, tremolo-picked guitars, delay swells, bowed strings, glockenspiel, cathartic, vast, wide live room, dynamic, no compression, 100 BPM',
        slots: { language: 'English', genre: 'post-rock', rhythm: 'slow build from silence to pounding climax', guitars: 'tremolo-picked guitars, delay swells', keys: 'bowed strings, glockenspiel', vocals: 'instrumental, no vocals', mood: 'cathartic, vast', production: 'wide live room, dynamic, no compression' }, bpm: 100, lyrics: '[Instrumental]',
      },
      {
        id: 'postrock-math', name: 'Математичный: сложный счёт, чистые гитары', nameEn: 'Math-y: complex time, clean guitars',
        style: 'Instrumental, no vocals, math rock, instrumental post-rock, odd time signatures, tight stop-start drums, clean tapped interlocking guitars, restless, precise, playful, dry and clear, every instrument separate, 140 BPM',
        slots: { language: 'English', genre: 'math rock, instrumental post-rock', rhythm: 'odd time signatures, tight stop-start drums', guitars: 'clean tapped interlocking guitars', keys: '', vocals: 'instrumental, no vocals', mood: 'restless, precise, playful', production: 'dry and clear, every instrument separate' }, bpm: 140, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'surf', name: 'Сёрф', nameEn: 'Surf',
    items: [
      {
        id: 'surf', name: 'Солнечный: пружинный ревер и погоня', nameEn: 'Sunny: spring reverb and a chase',
        style: 'Instrumental, no vocals, 1960s surf rock, twangy wet-reverb guitar lead, driving beat, hand claps, sunny beach chase energy, 150 BPM',
        slots: instr({ genre: 'surf rock', rhythm: 'driving dance beat', guitars: 'twangy wet-reverb lead', keys: 'sax bursts', mood: 'sunny beach fun', production: '1960s vintage' }), bpm: 150, lyrics: '[Instrumental]',
      },
      {
        id: 'surf-dark', name: 'Тёмный: минор и тремоло', nameEn: 'Dark: minor key and tremolo',
        style: 'Instrumental, no vocals, dark surf rock, spy surf, galloping tom beat, minor key reverb lead, heavy tremolo picking, menacing, noir chase, drenched spring reverb, 1963 mono, 165 BPM',
        slots: { language: 'English', genre: 'dark surf rock, spy surf', rhythm: 'galloping tom beat', guitars: 'minor key reverb lead, heavy tremolo picking', keys: '', vocals: 'instrumental, no vocals', mood: 'menacing, noir chase', production: 'drenched spring reverb, 1963 mono' }, bpm: 165, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'psych', name: 'Психоделия 60-х', nameEn: '60s Psychedelia',
    items: [
      {
        id: 'psych-tape', name: 'Плёночная: меллотрон и обратные звуки', nameEn: 'Tape: mellotron and reversed sounds',
        style: 'English, 1967 psychedelic rock, loose swinging drums, melodic bass, fuzz leads, backwards guitar, Mellotron flutes, harpsichord, dreamy double-tracked male, kaleidoscopic wonder, tape phasing, hard-panned stereo, 4-track, 105 BPM',
        slots: { language: 'English', genre: '1967 psychedelic rock', rhythm: 'loose swinging drums, melodic bass', guitars: 'fuzz leads, backwards guitar', keys: 'Mellotron flutes, harpsichord', vocals: 'dreamy double-tracked male', mood: 'kaleidoscopic wonder', production: 'tape phasing, hard-panned stereo, 4-track' }, bpm: 105,
      },
      {
        id: 'psych-jam', name: 'Джем: орган, эхо и длинные соло', nameEn: 'Jam: organ, echo and long solos',
        style: 'English, acid rock, psychedelic jam, hypnotic groove, congas, long wah-fuzz solos, swirling organ, tape echo, chanting male, reverb-soaked, trippy, expansive, live loose, warm valve, 112 BPM',
        slots: { language: 'English', genre: 'acid rock, psychedelic jam', rhythm: 'hypnotic groove, congas', guitars: 'long wah-fuzz solos', keys: 'swirling organ, tape echo', vocals: 'chanting male, reverb-soaked', mood: 'trippy, expansive', production: 'live loose, warm valve' }, bpm: 112,
      },
    ],
  },
  {
    id: 'classic', name: 'Классический рок и блюз-рок', nameEn: 'Classic rock & blues rock',
    items: [
      {
        id: 'classic-rock', name: 'Рок 70-х: орган и гимн', nameEn: '70s rock: organ and anthem',
        style: 'English, classic 1970s rock, bluesy guitar riffs, powerful live drums, Hammond organ, raspy male vocals, anthemic sing-along chorus, vintage analog production, 120 BPM',
        slots: { language: 'English', genre: 'classic 1970s rock', rhythm: 'powerful live drums, steady', guitars: 'bluesy riffs', keys: 'Hammond organ', vocals: 'raspy male, anthemic', mood: 'confident, celebratory', production: 'vintage analog' }, bpm: 120,
      },
      {
        id: 'blues-rock', name: 'Блюз-рок: шаффл и жгучее соло', nameEn: 'Blues rock: shuffle and burning solo',
        style: 'English, blues rock, driving shuffle-rock beat, searing blues guitar leads, Hammond organ, powerful gritty male vocals, swagger, 118 BPM',
        slots: { language: 'English', genre: 'blues rock', rhythm: 'driving shuffle rock', guitars: 'searing blues leads', keys: 'Hammond organ', vocals: 'powerful gritty male', mood: 'swagger, confident', production: 'loud live band' }, bpm: 118,
      },
      {
        id: 'prog-rock', name: 'Прог-рок: меняющийся размер', nameEn: 'Prog rock: shifting time signatures',
        style: 'English, progressive rock, shifting time signatures, Mellotron flutes, long instrumental passages, dynamic quiet-loud shifts, melodic high male vocals, 110 BPM',
        slots: { language: 'English', genre: 'progressive rock', rhythm: 'shifting signatures, dynamic', guitars: 'alternating clean and heavy', keys: 'Mellotron, Moog', vocals: 'melodic high male', mood: 'epic, adventurous', production: '1970s elaborate' }, bpm: 110,
      },
    ],
  },
  {
    id: 'metal', name: 'Хэви-метал', nameEn: 'Heavy metal',
    items: [
      {
        id: 'heavy-metal', name: 'Классика 80-х: две гитары', nameEn: '80s classic: twin guitars',
        style: 'English, classic 1980s heavy metal, galloping bass, twin guitar harmonies, soaring high male vocals, punchy drums, 135 BPM',
        slots: { language: 'English', genre: 'heavy metal', rhythm: 'galloping, punchy', guitars: 'twin harmonic leads', keys: '', vocals: 'soaring high male', mood: 'heroic defiance', production: '1983 punchy' }, bpm: 135,
      },
      {
        id: 'thrash', name: 'Трэш: быстро и рублено', nameEn: 'Thrash: fast and chopping',
        style: 'English, thrash metal, aggressive downpicked riffs, fast double-kick drums, barked rhythmic vocals, breakneck tempo, 185 BPM',
        slots: { language: 'English', genre: 'thrash metal', rhythm: 'breakneck double-kick', guitars: 'aggressive downpicked riffs', keys: '', vocals: 'barked rhythmic', mood: 'aggression, urgency', production: 'sharp loud' }, bpm: 185,
      },
      {
        id: 'black-metal', name: 'Блэк-метал: сырой классический', nameEn: 'Black metal: raw classic',
        style: 'English, raw classic black metal, fast tremolo-picked guitar wall, relentless blast beats, lo-fi cold grim production, raspy shrieked male vocals, cold dark atmosphere, 180 BPM',
        slots: { language: 'English', genre: 'black metal', rhythm: 'relentless blast beats', guitars: 'fast tremolo-picked wall', keys: '', vocals: 'raspy shrieked male', mood: 'cold grim darkness', production: 'lo-fi raw cold' }, bpm: 180,
      },
      {
        id: 'death-metal', name: 'Дэт-метал: старая школа', nameEn: 'Death metal: old school',
        style: 'English, old school death metal, down-tuned grinding guitar riffs, fast double-kick with blast sections, guttural growled male vocals, raw dark production, brutal and tight, 160 BPM',
        slots: { language: 'English', genre: 'death metal', rhythm: 'fast double-kick with blasts', guitars: 'down-tuned grinding riffs', keys: '', vocals: 'guttural death growls', mood: 'brutal, claustrophobic', production: 'raw dark heavy' }, bpm: 160,
      },
      {
        id: 'melodic-death', name: 'Мелодичный дэт: гётеборгский', nameEn: 'Melodic death: Gothenburg',
        style: 'English, melodic death metal, Gothenburg style twin guitar harmonies over aggressive riffs, tight double-kick drums, harsh growled male vocals, dark driving melancholic energy, 150 BPM',
        slots: { language: 'English', genre: 'melodic death metal', rhythm: 'tight driving double-kick', guitars: 'twin harmonies over aggressive riffs', keys: '', vocals: 'harsh growled male', mood: 'dark driving melancholy', production: 'dense tight modern' }, bpm: 150,
      },
      {
        id: 'power-metal', name: 'Пауэр-метал: эпика и скорость', nameEn: 'Power metal: epic and fast',
        style: 'English, power metal, fast galloping twin guitar harmonies, big drums with constant double-kick, soaring clear male vocals, epic triumphant fantasy mood, bright polished production, 165 BPM',
        slots: { language: 'English', genre: 'power metal', rhythm: 'galloping double-kick', guitars: 'fast twin harmonic leads', keys: 'symphonic synth pads', vocals: 'soaring clear male', mood: 'epic triumphant', production: 'bright polished big' }, bpm: 165,
      },
      {
        id: 'prog-metal', name: 'Прог-метал: рваный счёт', nameEn: 'Prog metal: shifting meters',
        style: 'English, progressive metal, heavy down-tuned riffs in shifting time signatures, syncopated precise drumming, clean melodic male vocals with harsh accents, dense modern production, cerebral and heavy, 140 BPM',
        slots: { language: 'English', genre: 'progressive metal', rhythm: 'syncopated shifting meters', guitars: 'heavy down-tuned technical riffs', keys: 'atmospheric synth layers', vocals: 'clean melodic with harsh accents', mood: 'cerebral intensity', production: 'dense modern precise' }, bpm: 140,
      },
    ],
  },
  {
    id: 'symphonic', name: 'Симфо-метал', nameEn: 'Symphonic metal',
    items: [
      {
        id: 'symphonic-choir', name: 'Хор и орган', nameEn: 'Choir and organ',
        style: 'English, symphonic metal, heavy down-tuned guitar riffs with full choir and church organ, orchestral string section, dramatic operatic female soprano vocals, double-kick drums, epic gothic grandeur, 140 BPM',
        slots: { language: 'English', genre: 'symphonic metal, gothic metal', rhythm: 'steady double-kick, processional', guitars: 'heavy down-tuned riffs under the orchestra', keys: 'choir, church organ, string section', vocals: 'dramatic operatic female soprano', mood: 'epic gothic grandeur', production: 'orchestral, wide, powerful' }, bpm: 140,
      },
      {
        id: 'symphonic-piano', name: 'С роялем и струнными', nameEn: 'With piano and strings',
        style: 'English, symphonic metal, intimate piano intro growing into heavy riffs with string section, clear expressive female vocals, half-time drums with orchestral crashes, melancholic cinematic drama, 120 BPM',
        slots: { language: 'English', genre: 'symphonic metal', rhythm: 'half-time with orchestral crashes', guitars: 'riffs entering after piano intros', keys: 'grand piano, string section', vocals: 'clear expressive female', mood: 'melancholic cinematic drama', production: 'cinematic, detailed, wide' }, bpm: 120,
      },
    ],
  },
  {
    id: 'doom', name: 'Дум и стоунер', nameEn: 'Doom & stoner',
    items: [
      {
        id: 'doom', name: 'Дум: очень медленно и тяжело', nameEn: 'Doom: very slow and heavy',
        style: 'English, doom metal, crushingly slow riffs, thick dark distortion, mournful wailing vocals, funeral pace, cavernous heaviness, 70 BPM',
        slots: { language: 'English', genre: 'doom metal', rhythm: 'crushingly slow', guitars: 'thick dark riffs', keys: '', vocals: 'mournful wailing', mood: 'grief, dread', production: 'thick cavernous' }, bpm: 70,
      },
      {
        id: 'stoner', name: 'Стоунер: фузз и пустыня', nameEn: 'Stoner: fuzz and desert',
        style: 'English, stoner rock, desert rock, groovy mid-tempo, heavy swing, thick fuzz riffs, tube amp, laid-back hazy male, sun-baked hypnotic, warm vintage, live jam, 100 BPM',
        slots: { language: 'English', genre: 'stoner rock, desert rock', rhythm: 'groovy mid-tempo, heavy swing', guitars: 'thick fuzz riffs, tube amp', keys: '', vocals: 'laid-back hazy male', mood: 'sun-baked hypnotic', production: 'warm vintage, live jam' }, bpm: 100,
      },
      {
        id: 'sludge-metal', name: 'Сладж: злой и вязкий', nameEn: 'Sludge: mean and thick',
        style: 'English, sludge metal, slow mean down-tuned guitar riffs, bashed heavy drums, shouted hardcore-style male vocals, dusty raw production, hostile and thick, 90 BPM',
        slots: { language: 'English', genre: 'sludge metal', rhythm: 'slow heavy plod', guitars: 'down-tuned mean riffs', keys: '', vocals: 'shouted hardcore male', mood: 'hostile, oppressive', production: 'dusty raw heavy' }, bpm: 90,
      },
    ],
  },
  {
    id: 'synthwave', name: 'Синтвейв', nameEn: 'Synthwave',
    items: [
      {
        id: 'outrun', name: 'Аутран: шоссе на закате', nameEn: 'Outrun: highway at sunset',
        style: 'Instrumental, no vocals, synthwave outrun, driving 80s beat, arpeggiated analog bass, gated reverb drums, neon sax or lead synth, sunset highway nostalgia, 110 BPM',
        slots: instr({ genre: 'synthwave, outrun', rhythm: 'driving 80s beat, gated reverb snare', keys: 'arpeggio analog bass, neon lead synth', mood: 'neon nostalgia, sunset highway', production: '1985 analog sheen' }), bpm: 110, lyrics: '[Instrumental]',
      },
      {
        id: 'darksynth', name: 'Дарк-синт: хоррор 80-х', nameEn: 'Darksynth: 80s horror',
        style: 'Instrumental, no vocals, darksynth, menacing slow pounding beat, distorted bass pulse, horror-movie staccato strings synth, cold fear, midnight slasher chase, 100 BPM',
        slots: instr({ genre: 'darksynth, horror synth', rhythm: 'pounding slow beat', keys: 'distorted bass, staccato string synth', mood: 'cold fear, menacing', production: 'dark analog grit' }), bpm: 100, lyrics: '[Instrumental]',
      },
      {
        id: 'vaporwave', name: 'Вейпорвейв: замедленные сэмплы', nameEn: 'Vaporwave: slowed samples',
        style: 'Instrumental, no vocals, vaporwave, slowed chopped easy-listening samples, pitch-dropped sax and muzak piano, tape slow-downs, ironic nostalgic haze, 70 BPM',
        slots: instr({ genre: 'vaporwave', rhythm: 'slowed loose beat', keys: 'pitch-dropped sax, muzak piano', mood: 'ironic nostalgic haze', production: 'tape slow-down, lo-fi sheen' }), bpm: 70, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'chiptune', name: 'Чиптюн и 8-бит', nameEn: 'Chiptune & 8-bit',
    items: [
      {
        id: 'chiptune', name: 'Чиптюн: игровая приставка', nameEn: 'Chiptune: game console',
        style: 'Instrumental, no vocals, chiptune, 8-bit square-wave lead melodies, fast pulse-wave arpeggios, 8-bit noise-channel drums, playful retro video game energy, 150 BPM',
        slots: instr({ genre: 'chiptune', rhythm: '8-bit noise-channel drums', keys: '8-bit square-wave lead melodies, fast pulse-wave arpeggios', mood: 'playful retro video game energy', production: 'lo-fi 8-bit bitcrushed chip sound' }), bpm: 150, lyrics: '[Instrumental]',
      },
      {
        id: 'chip-punk', name: '8-бит панк: пищалки и гитары', nameEn: '8-bit punk: bleeps and guitars',
        style: 'English, chiptune punk, electropunk, fast punk drums, distorted power chords, 8-bit square-wave lead melodies, soft melodic male in front, snotty playful fun, raw loud, bitcrushed, 170 BPM',
        slots: { language: 'English', genre: 'chiptune punk, electropunk', rhythm: 'fast punk drums', guitars: 'distorted power chords', keys: '8-bit square-wave lead melodies', vocals: 'soft melodic male in front', mood: 'snotty playful fun', production: 'raw loud, bitcrushed' }, bpm: 170,
      },
      {
        id: 'chip-metal', name: '8-бит метал: пищалки и рубка', nameEn: '8-bit metal: bleeps and chugging',
        style: 'English, chiptune metal, double kick blast drums, palm-muted metal chug, 8-bit arpeggios, screamed and clean male, frantic boss fight energy, modern loud, 180 BPM',
        slots: { language: 'English', genre: 'chiptune metal', rhythm: 'double kick blast drums', guitars: 'palm-muted metal chug', keys: '8-bit arpeggios', vocals: 'screamed and clean male', mood: 'frantic boss fight energy', production: 'modern loud' }, bpm: 180,
      },
    ],
  },
  {
    id: 'industrial', name: 'EBM и электро', nameEn: 'EBM & electro',
    items: [
      {
        id: 'ebm', name: 'EBM: марш машин', nameEn: 'EBM: march of machines',
        style: 'Instrumental, no vocals, EBM electronic body music, pumping industrial bass sequencer, metallic percussion, hard steady beat, cold machine discipline, 130 BPM',
        slots: instr({ genre: 'EBM, industrial', rhythm: 'hard steady machine beat', keys: 'pumping sequencer bass', mood: 'cold machine discipline, marching', production: 'metallic, punchy' }), bpm: 130, lyrics: '[Instrumental]',
      },
      {
        id: 'dark-electro', name: 'Тёмное электро', nameEn: 'Dark electro',
        style: 'Instrumental, no vocals, dark electro, distorted beat, gritty bass growls, ominous bell tones, gothic night factory mood, 125 BPM',
        slots: instr({ genre: 'dark electro', rhythm: 'distorted steady beat', keys: 'gritty bass growls, bells', mood: 'gothic ominous', production: 'gritty dark' }), bpm: 125, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'techno', name: 'Техно и эйсид', nameEn: 'Techno & acid',
    items: [
      byId('dark-techno'),
      {
        id: 'melodic-techno', name: 'Мелодичное', nameEn: 'Melodic',
        style: 'Instrumental, no vocals, melodic techno, driving four-on-the-floor, hypnotic arpeggio bassline, cinematic pad swells, build-ups and drops, euphoric dark energy, 124 BPM',
        slots: instr({ genre: 'melodic techno', rhythm: 'four-on-the-floor, driving', keys: 'arpeggio bass, cinematic pads', mood: 'euphoric dark energy, hypnotic', production: 'tight modern club mix' }), bpm: 124, lyrics: '[Instrumental]',
      },
      {
        id: 'minimal-techno', name: 'Минимал', nameEn: 'Minimal',
        style: 'Instrumental, no vocals, minimal techno, sparse clicking percussion, one dry bassline loop, tiny blips and glitches, tons of space, clinical precision, 128 BPM',
        slots: instr({ genre: 'minimal techno', rhythm: 'sparse exact groove', keys: 'dry bass loop, glitch blips', mood: 'clinical, hypnotic restraint', production: 'dry, wide space' }), bpm: 128, lyrics: '[Instrumental]',
      },
      {
        id: 'acid', name: 'Эйсид: 303', nameEn: 'Acid: 303',
        style: 'Instrumental, no vocals, acid techno, squelchy 303 acid bassline, relentless straight kick, sharp hi-hats, tweaky filter sweeps, sweaty warehouse trance, 132 BPM',
        slots: instr({ genre: 'acid techno', rhythm: 'relentless straight kick', keys: 'squelchy 303 acid line', mood: 'sweaty hypnotic intensity', production: 'raw analog' }), bpm: 132, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'house', name: 'Хаус', nameEn: 'House',
    items: [
      {
        id: 'deep-house', name: 'Дип', nameEn: 'Deep',
        style: 'Instrumental, no vocals, deep house, warm bubbling bassline, four-on-the-floor with soft clap, jazzy chords through filter, late-night groove, sophisticated ease, 120 BPM',
        slots: instr({ genre: 'deep house', rhythm: 'four-on-the-floor, groovy', keys: 'bubbling bass, jazzy filtered chords', mood: 'late-night sophisticated ease', production: 'deep warm club mix' }), bpm: 120, lyrics: '[Instrumental]',
      },
      {
        id: 'funky-house', name: 'Диско-хаус', nameEn: 'Disco house',
        style: 'Instrumental, no vocals, funky disco house, chopped disco guitar loops, bouncing bass, filter sweeps, hands-in-the-air piano stabs, party euphoria, 124 BPM',
        slots: instr({ genre: 'funky house, disco house', rhythm: 'bouncing four-on-the-floor', guitars: 'chopped disco guitar loops', keys: 'piano stabs', mood: 'party euphoria, glossy fun', production: 'punchy bright club' }), bpm: 124, lyrics: '[Instrumental]',
      },
      {
        id: 'prog-house', name: 'Прогрессив', nameEn: 'Progressive',
        style: 'Instrumental, no vocals, progressive house, long smooth build-ups, layered synth arpeggios, wide atmospherics, emotional breakdown into drop, uplifting journey, 126 BPM',
        slots: instr({ genre: 'progressive house', rhythm: 'driving four-on-the-floor', keys: 'layered arpeggios, wide pads', mood: 'uplifting journey, emotional', production: 'wide cinematic club' }), bpm: 126, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'trance', name: 'Транс', nameEn: 'Trance',
    items: [
      {
        id: 'uplifting', name: 'Аплифтинг', nameEn: 'Uplifting',
        style: 'Instrumental, no vocals, uplifting trance, fast driving beat, supersaw leads, epic breakdown with plucked melody, big euphoric drop, hands-up euphoria, 138 BPM',
        slots: instr({ genre: 'uplifting trance', rhythm: 'fast driving four-on-the-floor', keys: 'supersaw leads, plucked melody', mood: 'euphoric, epic, radiant', production: 'huge bright festival' }), bpm: 138, lyrics: '[Instrumental]',
      },
      {
        id: 'psytrance', name: 'Пси-транс', nameEn: 'Psytrance',
        style: 'Instrumental, no vocals, psychedelic trance, rolling bassline, zapping alien synths, dense layered percussion, hypnotic acid squelches, trippy fractal energy, 145 BPM',
        slots: instr({ genre: 'psytrance', rhythm: 'rolling fast beat', keys: 'zapping synths, acid squelches', mood: 'trippy hypnotic intensity', production: 'dense psychedelic' }), bpm: 145, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'dnb', name: "Drum'n'bass и джангл", nameEn: "Drum'n'bass & jungle",
    items: [
      {
        id: 'liquid-dnb', name: 'Ликвид', nameEn: 'Liquid',
        style: 'Instrumental, no vocals, liquid drum and bass, rolling breakbeats, warm sub bass, lush pad chords, soulful vocal-chop echoes (no words), sunrise euphoria, 174 BPM',
        slots: instr({ genre: 'liquid drum and bass', rhythm: 'rolling breakbeats', keys: 'warm sub bass, lush pads', mood: 'sunrise euphoria, soulful', production: 'polished warm' }), bpm: 174, lyrics: '[Instrumental]',
      },
      {
        id: 'jungle', name: 'Джангл: рубленые брейки', nameEn: 'Jungle: chopped breaks',
        style: 'Instrumental, no vocals, jungle, chopped amen breaks, deep dub bassline, reggae skank chords, sirens and dub delays, raw rude-bwoy energy, 165 BPM',
        slots: instr({ genre: 'jungle', rhythm: 'chopped amen breaks', keys: 'dub bass, reggae skank chords', mood: 'raw energetic rave', production: 'dub delays, raw' }), bpm: 165, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'breaks', name: 'Брейки и биг-бит', nameEn: 'Breaks & big beat',
    items: [
      byId('bigbeat-guitars'),
      {
        id: 'electro-breaks', name: 'Электро-брейки', nameEn: 'Electro breaks',
        style: 'Instrumental, no vocals, electro breaks, funky syncopated beat, robotic talkbox bass, 808 claps, scratching, retro-futuristic b-boy energy, 128 BPM',
        slots: instr({ genre: 'electro, breaks', rhythm: 'syncopated funky breaks', keys: 'talkbox bass, 808 claps', mood: 'retro-futuristic swagger', production: 'punchy analog' }), bpm: 128, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'triphop', name: 'Трип-хоп и даунтемпо', nameEn: 'Trip-hop & downtempo',
    items: [
      {
        id: 'trip-hop', name: 'Трип-хоп: нуар и пыльные сэмплы', nameEn: 'Trip-hop: noir and dusty samples',
        style: 'Instrumental, no vocals, trip-hop, slow heavy hip-hop breakbeat, moody bassline, dusty vinyl samples, theremin-like synth wails, noir atmosphere, melancholy paranoia, 88 BPM',
        slots: instr({ genre: 'trip-hop', rhythm: 'slow heavy breakbeat, head-nodding', keys: 'moody bassline, dusty samples, theremin-like synth', mood: 'noir melancholy, paranoid', production: 'vinyl hiss, dark wide mix' }), bpm: 88, lyrics: '[Instrumental]',
      },
      {
        id: 'balearic', name: 'Балеарик: нейлон и маримба', nameEn: 'Balearic: nylon guitar and marimba',
        style: 'Instrumental, no vocals, balearic downtempo, gentle tropical percussion, nylon guitar licks, warm analog synth pad, marimba, ocean-wide reverb, sunny serenity, 96 BPM',
        slots: instr({ genre: 'balearic, downtempo', rhythm: 'gentle shuffling groove', guitars: 'nylon guitar licks', keys: 'warm analog synth pad, marimba', mood: 'sunny serenity, seaside morning', production: 'ocean-wide reverb, smooth' }), bpm: 96, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'ambient', name: 'Эмбиент', nameEn: 'Ambient',
    items: [
      {
        id: 'space-ambient', name: 'Космический', nameEn: 'Space',
        style: 'Instrumental, no vocals, space ambient, no percussion, deep sub drones, shimmering high harmonics, slow chord clouds, NASA radio chatter feel far away, immense scale, weightless floating, 50 BPM',
        slots: instr({ genre: 'space ambient, drone', rhythm: 'no percussion, drifting', keys: 'sub drones, shimmering pads', mood: 'weightless, immense cosmic calm', production: 'huge stereo field, endless reverb' }), bpm: 50, lyrics: '[Instrumental]',
      },
      {
        id: 'dark-ambient', name: 'Тёмный', nameEn: 'Dark',
        style: 'Instrumental, no vocals, dark ambient, no beat, low cavernous drones, metallic resonances, distant industrial echoes, unsettling tension, abyssal darkness, 45 BPM',
        slots: instr({ genre: 'dark ambient', rhythm: 'no beat, pulseless drift', keys: 'low drones, metallic resonances', mood: 'unease, abyssal darkness', production: 'cavernous, isolated' }), bpm: 45, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'blues', name: 'Блюз', nameEn: 'Blues',
    items: [
      {
        id: 'chicago-blues', name: 'Чикаго: электро', nameEn: 'Chicago: electric',
        style: 'English, Chicago electric blues, shuffle groove, gritty tube guitar solos, harmonica wails, walking bass, barroom piano, raspy male vocals, whiskey and smoke, 96 BPM',
        slots: { language: 'English', genre: 'Chicago blues', rhythm: 'blues shuffle', guitars: 'gritty tube electric solos', keys: 'barroom piano, harmonica', vocals: 'raspy male, worn', mood: 'whiskey melancholy, smoky', production: 'live club, tube warmth' }, bpm: 96,
      },
      {
        id: 'delta-blues', name: 'Дельта: акустика', nameEn: 'Delta: acoustic',
        style: 'English, delta blues, fingerpicked acoustic slide guitar, stomping foot and harmonica, gravel male voice, 1930s porch recording, lonely road story, 78 BPM',
        slots: { language: 'English', genre: 'delta blues, country blues', rhythm: 'stomping foot, free', guitars: 'acoustic slide, fingerpicked', keys: 'harmonica', vocals: 'gravel male, raw', mood: 'lonely road, haunted', production: '1930s mono field recording' }, bpm: 78,
      },
      {
        id: 'slow-blues', name: 'Медленный: полночь', nameEn: 'Slow: midnight',
        style: 'English, slow blues 12/8, mournful bends guitar, sparse piano chords, brushed drums, world-weary male vocals, last-call bar sadness, 58 BPM',
        slots: { language: 'English', genre: 'slow blues', rhythm: '12/8 slow drag', guitars: 'mournful slow bends', keys: 'sparse piano', vocals: 'world-weary male', mood: 'last-call sadness', production: 'smoky close' }, bpm: 58,
      },
    ],
  },
  {
    id: 'funk', name: 'Фанк и афробит', nameEn: 'Funk & afrobeat',
    items: [
      {
        id: 'funk', name: 'Фанк 70-х', nameEn: '70s funk',
        style: 'English, funk, syncopated slap bass, wah rhythm guitar, punchy horns, tight drums with ghost notes, swaggering groove, 105 BPM',
        slots: { language: 'English', genre: 'funk', rhythm: 'syncopated tight groove', guitars: 'wah rhythm chops', keys: 'clavinet, punchy horns', vocals: 'confident, call-and-response', mood: 'swagger, party', production: 'punchy 1975' }, bpm: 105,
      },
      {
        id: 'afrobeat', name: 'Афробит: полиритмия и медь', nameEn: 'Afrobeat: polyrhythms and brass',
        style: 'English, afrobeat, polyrhythmic drum groove, shekere, hypnotic bass ostinato, interlocking clean guitar riffs, Farfisa organ, big horn section riffs, chanted call-and-response, male lead, defiant joyful trance, live band, long jam, 1973 Lagos, 112 BPM',
        slots: { language: 'English', genre: 'afrobeat', rhythm: 'polyrhythmic drum groove, shekere, hypnotic bass ostinato', guitars: 'interlocking clean guitar riffs', keys: 'Farfisa organ, big horn section riffs', vocals: 'chanted call-and-response, male lead', mood: 'defiant joyful trance', production: 'live band, long jam, 1973 Lagos' }, bpm: 112,
      },
    ],
  },
  {
    id: 'reggae', name: 'Регги и даб', nameEn: 'Reggae & dub',
    items: [
      {
        id: 'roots-reggae', name: 'Рутс: one drop и орган', nameEn: 'Roots: one drop and organ',
        style: 'English, roots reggae, one drop groove, warm round bass, offbeat skank guitar, Hammond organ bubbles, laid-back soulful male, sunny morning, conscious, warm 1970s Kingston studio, 78 BPM',
        slots: { language: 'English', genre: 'roots reggae', rhythm: 'one drop groove, warm round bass', guitars: 'offbeat skank guitar', keys: 'Hammond organ bubbles', vocals: 'laid-back soulful male', mood: 'sunny morning, conscious', production: 'warm 1970s Kingston studio' }, bpm: 78,
      },
      {
        id: 'dub', name: 'Даб: эхо и пружинный ревер', nameEn: 'Dub: echo and spring reverb',
        style: 'Instrumental, no vocals, roots dub, heavy one-drop bass and drums, spring reverb explosions, tape delay echoes, skanking guitar offbeats, sparse melodica, deep bass meditation, 75 BPM',
        slots: instr({ genre: 'dub', rhythm: 'one-drop, heavy and slow', guitars: 'skanking offbeat chords', keys: 'melodica, tape delay echoes', mood: 'deep meditative groove', production: 'spring reverb, tape delay throws' }), bpm: 75, lyrics: '[Instrumental]',
      },
      {
        id: 'dancehall', name: 'Дэнсхолл', nameEn: 'Dancehall',
        style: 'Jamaican patois, dancehall, riddim bounce, punchy synth stabs, energetic toasting male vocals, party crowd energy, 102 BPM',
        slots: { language: 'Jamaican patois', genre: 'dancehall', rhythm: 'bouncing riddim', guitars: '', keys: 'punchy synth stabs', vocals: 'energetic male toasting, rhythmic', mood: 'party crowd energy', production: 'punchy modern' }, bpm: 102,
      },
    ],
  },
  {
    id: 'jazz', name: 'Джаз', nameEn: 'Jazz',
    items: [
      {
        id: 'noir-jazz', name: 'Нуар: дымное трио', nameEn: 'Noir: smoky trio',
        style: 'Instrumental, no vocals, jazz noir, sparse smoky trio, walking bass, brushed snare, lonely muted trumpet, dark alley atmosphere, rain and neon reflections, 76 BPM',
        slots: instr({ genre: 'jazz noir', rhythm: 'slow walking swing', guitars: '', keys: 'muted trumpet, piano', mood: 'dark rain-soaked loneliness', production: 'smoky close-mic' }), bpm: 76, lyrics: '[Instrumental]',
      },
      {
        id: 'swing-bigband', name: 'Свинг: биг-бэнд', nameEn: 'Swing: big band',
        style: 'English, big band swing, punchy brass sections, walking bass, ride cymbal swing, joyful shout vocals with band answers, dancehall 1940s celebration, 140 BPM',
        slots: { language: 'English', genre: 'big band swing', rhythm: 'fast swing, ride cymbal', guitars: 'rhythm guitar chunk', keys: 'brass sections, piano', vocals: 'joyful male shout with band answers', mood: '1940s celebration', production: 'vintage ballroom' }, bpm: 140,
      },
    ],
  },
  {
    id: 'hiphop', name: 'Хип-хоп', nameEn: 'Hip-hop',
    items: [
      {
        id: 'boombap', name: 'Бум-бэп: золотая эра', nameEn: 'Boom bap: golden era',
        style: 'English, boom bap hip-hop, dusty sampled beat, fat kick and snappy snare, jazzy sample loop, confident rap flow, 92 BPM',
        slots: { language: 'English', genre: 'boom bap hip-hop', rhythm: 'dusty head-nod beat', guitars: '', keys: 'jazzy sample loop', vocals: 'confident rap flow', mood: 'golden-era cool', production: 'sampled, warm dusty' }, bpm: 92,
      },
      {
        id: 'ghetto-funk', name: 'Фанк-брейк с рэпом', nameEn: 'Funk break with rap',
        style: 'English, funk-rap, live funk breakbeat, slap bass, horn stabs, old-school party rap with crowd responses, 105 BPM',
        slots: { language: 'English', genre: 'funk rap, old school', rhythm: 'live funk breakbeat', guitars: 'wah chops', keys: 'horn stabs, clavinet', vocals: 'old-school rap, crowd responses', mood: 'block party', production: 'live band' }, bpm: 105,
      },
      {
        id: 'rus-hiphop', name: 'Русский: лирика', nameEn: 'Russian: lyrical',
        style: 'Russian, melodic russian hip-hop, melancholic piano loop, crisp boom-bap-leaning beat, sincere rap verses and sung hook, rainy city mood, 90 BPM',
        slots: { language: 'Russian', genre: 'melodic hip-hop', rhythm: 'crisp mid-tempo beat', guitars: '', keys: 'melancholic piano loop', vocals: 'sincere rap + sung hook', mood: 'rainy city melancholy', production: 'clean modern' }, bpm: 90,
      },
    ],
  },
  {
    id: 'folk', name: 'Фолк', nameEn: 'Folk',
    items: [
      {
        id: 'singersongwriter', name: 'Авторская: гитара и голос', nameEn: 'Singer-songwriter: guitar and voice',
        style: 'Russian, singer-songwriter, one acoustic guitar and voice, intimate storytelling verses, sincere conversational vocals, live room, campfire honesty, 95 BPM',
        slots: { language: 'Russian', genre: 'singer-songwriter, bard song', rhythm: 'free-flowing, rubato', guitars: 'fingerpicked acoustic guitar', keys: '', vocals: 'sincere male, conversational storytelling', mood: 'intimate, sincere', production: 'live room, close and dry' }, bpm: 95,
      },
      {
        id: 'folk-acoustic', name: 'Акустика без слов', nameEn: 'Acoustic, wordless',
        style: 'Instrumental, no vocals, acoustic folk, fingerpicked guitar interludes, wooden flute, gentle frame drum, meadow-and-forest serenity, 90 BPM',
        slots: instr({ genre: 'acoustic folk', rhythm: 'gentle pulsing', guitars: 'fingerpicked melodies', keys: 'wooden flute', mood: 'pastoral serenity', production: 'warm natural' }), bpm: 90, lyrics: '[Instrumental]',
      },
      {
        id: 'slavic-folk', name: 'Славянский', nameEn: 'Slavic',
        style: 'Russian, slavic folk, gusli and zhaleyka motifs, female folk vocals with ornaments, circle-dance rhythm, ancient field and river imagery, 105 BPM',
        slots: { language: 'Russian', genre: 'slavic folk', rhythm: 'circle-dance pulse', guitars: '', keys: 'gusli, zhaleyka, whistle', vocals: 'female folk, ornamented', mood: 'ancient, earthy', production: 'natural live' }, bpm: 105,
      },
      {
        id: 'celtic', name: 'Кельтика', nameEn: 'Celtic',
        style: 'Instrumental, no vocals, celtic folk, tin whistle and fiddle dance melody, acoustic guitar strum, bodhran pulse, tavern cheer to misty hills, 115 BPM',
        slots: instr({ genre: 'celtic folk', rhythm: 'bodhran dance pulse', guitars: 'acoustic strum', keys: 'tin whistle, fiddle', mood: 'tavern cheer, misty nostalgia', production: 'warm live' }), bpm: 115, lyrics: '[Instrumental]',
      },
      {
        id: 'bluegrass', name: 'Блюграсс', nameEn: 'Bluegrass',
        style: 'English, bluegrass, breakneck banjo rolls, fiddle and mandolin breaks, upright slap bass, high lonesome harmonies, porch energy, 150 BPM',
        slots: { language: 'English', genre: 'bluegrass', rhythm: 'breakneck roll', guitars: 'flatpicking runs', keys: 'banjo, fiddle, mandolin', vocals: 'high lonesome harmonies', mood: 'joyous porch energy', production: 'live around one mic' }, bpm: 150,
      },
      {
        id: 'folk-rock', name: 'Фолк-рок', nameEn: 'Folk rock',
        style: 'English, folk rock, strummed acoustic guitars with electric leads, steady drums, storytelling male vocals, anthemic choruses, 125 BPM',
        slots: { language: 'English', genre: 'folk rock', rhythm: 'steady driving', guitars: 'strummed acoustic + electric leads', keys: 'mandolin', vocals: 'storytelling male', mood: 'anthemic, road-trip', production: 'warm full band' }, bpm: 125,
      },
    ],
  },
  {
    id: 'oriental', name: 'Восток и этника', nameEn: 'Oriental & ethnic',
    items: [
      byId('ethnic-oriental'),
      {
        id: 'turkish-psych', name: 'Анатолийская психоделия', nameEn: 'Anatolian psychedelia',
        style: 'Turkish, anatolian psychedelic folk, saz electric baglama riffs, funky groove, hypnotic east modes, passionate male vocals, vintage 70s fuzz, 110 BPM',
        slots: { language: 'Turkish', genre: 'anatolian psych folk', rhythm: 'funky hypnotic groove', guitars: 'electric saz fuzz riffs', keys: '', vocals: 'passionate male, melismatic', mood: 'hypnotic fiery', production: '70s vintage fuzz' }, bpm: 110,
      },
      {
        id: 'arabesque', name: 'Арабеска', nameEn: 'Arabesque',
        style: 'Instrumental, no vocals, arabesque orchestral, violin and qanun unison bends, darbuka groove, dramatic minor modes, oud solos, bazaar-to-moonlight romance, 100 BPM',
        slots: instr({ genre: 'arabesque, middle-eastern', rhythm: 'darbuka groove', guitars: 'oud solos', keys: 'violin, qanun', mood: 'dramatic romantic longing', production: 'orchestral warm' }), bpm: 100, lyrics: '[Instrumental]',
      },
      {
        id: 'sitar-raga', name: 'Ситар-рага', nameEn: 'Sitar raga',
        style: 'Instrumental, no vocals, indian classical raga, sitar alaps and fast meend runs, tanpura drone, tabla accelerating tala, exotic microtonal beauty, hypnotic, 90 BPM',
        slots: instr({ genre: 'indian classical, raga', rhythm: 'tabla tala, accelerating', keys: 'sitar, tanpura drone', mood: 'hypnotic exotic serenity', production: 'intimate live room' }), bpm: 90, lyrics: '[Instrumental]',
      },
      {
        id: 'shamanic', name: 'Шаманские бубны', nameEn: 'Shamanic drums',
        style: 'Instrumental, no vocals, shamanic drumming, frame drum heart-beat pulse, rattle and throat-hum overtones, ceremonial fire circle, trance possession rhythm, 130 BPM',
        slots: instr({ genre: 'shamanic, ritual', rhythm: 'frame drum trance pulse', guitars: '', keys: 'rattles, throat overtones', mood: 'ceremonial trance, primal', production: 'raw live circle' }), bpm: 130, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'latino', name: 'Латино', nameEn: 'Latin',
    items: [
      {
        id: 'flamenco', name: 'Фламенко', nameEn: 'Flamenco',
        style: 'Spanish, flamenco, fiery rasgueado guitar, palmas hand-claps, cajón, passionate wailing male vocals with melisma, duende intensity, 120 BPM',
        slots: { language: 'Spanish', genre: 'flamenco', rhythm: 'palmas, compás pulse', guitars: 'fiery rasgueado falsetas', keys: 'cajón', vocals: 'passionate wailing, melismatic', mood: 'duende, fierce sorrow', production: 'intimate live' }, bpm: 120,
      },
      {
        id: 'salsa', name: 'Сальса', nameEn: 'Salsa',
        style: 'Spanish, salsa, montuno piano, brass section, congas and timbales, swinging dance groove, joyful male vocals with coro, 95 BPM',
        slots: { language: 'Spanish', genre: 'salsa', rhythm: 'clave, congas, timbales', guitars: '', keys: 'montuno piano, brass', vocals: 'joyful male with coro responses', mood: 'dancefloor joy', production: 'big band latin' }, bpm: 95,
      },
      {
        id: 'tango', name: 'Танго', nameEn: 'Tango',
        style: 'Instrumental, no vocals, argentine tango, bandoneón laments, dramatic staccato strings, piano stabs, sharp stops and lunges, fatal romance, 110 BPM',
        slots: instr({ genre: 'argentine tango', rhythm: 'sharp staccato, sudden stops', guitars: '', keys: 'bandoneón, dramatic strings', mood: 'fatal romance, dusk drama', production: 'vintage ballroom' }), bpm: 110, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'cinematic', name: 'Кино и неоклассика', nameEn: 'Cinematic & neoclassical',
    items: [
      {
        id: 'noir-score', name: 'Нуар-саундтрек', nameEn: 'Noir score',
        style: 'Instrumental, no vocals, film noir score, lonely sax over dark strings, slow jazz-noir drum shadows, muted trumpet lament, crime-city 2am, 72 BPM',
        slots: instr({ genre: 'noir score, jazz', rhythm: 'slow shadowed beat', guitars: '', keys: 'dark strings, lonely sax, muted trumpet', mood: 'crime-city 2am fatalism', production: 'dark analog film mix' }), bpm: 72, lyrics: '[Instrumental]',
      },
      {
        id: 'neoclassical', name: 'Неоклассика: фортепиано', nameEn: 'Neoclassical: piano',
        style: 'Instrumental, no vocals, neoclassical piano, intimate felt piano melody, subtle string swells, pedaled resonances, bittersweet introspection, 70 BPM',
        slots: instr({ genre: 'neoclassical, modern classical', rhythm: 'rubato, breathing', guitars: '', keys: 'felt piano, subtle strings', mood: 'bittersweet introspection', production: 'intimate close piano' }), bpm: 70, lyrics: '[Instrumental]',
      },
      {
        id: 'western', name: 'Вестерн', nameEn: 'Western',
        style: 'Instrumental, no vocals, spaghetti western, twanging reverb guitar, whistling melody, mariachi trumpets, galloping rhythm, dusty showdown tension, 95 BPM',
        slots: instr({ genre: 'spaghetti western', rhythm: 'galloping', guitars: 'twang reverb lead', keys: 'mariachi trumpet, whistle', mood: 'dusty showdown tension', production: '60s cinema analog' }), bpm: 95, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'classical', name: 'Классика', nameEn: 'Classical',
    items: [
      {
        id: 'baroque', name: 'Барокко', nameEn: 'Baroque',
        style: 'Instrumental, no vocals, baroque, harpsichord and recorder sonata, walking cello bass, ornate trills, courtly elegance, 100 BPM',
        slots: instr({ genre: 'baroque chamber', rhythm: 'steady walking bass', guitars: 'lute', keys: 'harpsichord, recorder', mood: 'courtly elegance', production: 'intimate chamber' }), bpm: 100, lyrics: '[Instrumental]',
      },
      {
        id: 'string-quartet', name: 'Струнный квартет', nameEn: 'String quartet',
        style: 'Instrumental, no vocals, string quartet, four voices in conversation, warm vibrato, gentle andante with dramatic middle section, 80 BPM',
        slots: instr({ genre: 'chamber, string quartet', rhythm: 'andante, breathing', guitars: '', keys: 'two violins, viola, cello', mood: 'conversational warmth with drama', production: 'close hall' }), bpm: 80, lyrics: '[Instrumental]',
      },
      {
        id: 'waltz', name: 'Вальс', nameEn: 'Waltz',
        style: 'Instrumental, no vocals, waltz, oom-pah-pah bass, swirling melody line, old ballroom nostalgia, 3/4, 140 BPM',
        slots: instr({ genre: 'waltz', rhythm: '3/4 oom-pah-pah', guitars: '', keys: 'piano, strings, accordion', mood: 'ballroom nostalgia', production: 'vintage hall' }), bpm: 140, lyrics: '[Instrumental]',
      },
      {
        id: 'organ-dark', name: 'Орган: готика', nameEn: 'Organ: gothic',
        style: 'Instrumental, no vocals, dark organ toccata, cathedral echoes, ominous minor cascades, gothic dread and grandeur, 75 BPM',
        slots: instr({ genre: 'organ, gothic classical', rhythm: 'pulsing toccata', guitars: '', keys: 'pipe organ', mood: 'gothic dread, grandeur', production: 'huge cathedral' }), bpm: 75, lyrics: '[Instrumental]',
      },
    ],
  },
  {
    id: 'russian', name: 'Русское', nameEn: 'Russian',
    items: [
      {
        id: 'ru-rock', name: 'Русский рок', nameEn: 'Russian rock',
        style: 'Russian, classic russian rock, anthemic minor-key guitars, earnest hoarse male vocals, live drums, sing-along chorus, kitchen-poetry sincerity, 120 BPM',
        slots: { language: 'Russian', genre: 'russian rock', rhythm: 'steady anthemic', guitars: 'clean-to-distort riffs', keys: '', vocals: 'earnest hoarse male', mood: 'sincere, defiant hope', production: 'live band, 1990s' }, bpm: 120,
      },
      {
        id: 'ru-romance', name: 'Городской романс', nameEn: 'Urban romance',
        style: 'Russian, urban romance, solo guitar, sorrowful gypsy-tinged vocals with tremble, minor key, candle-lit nostalgia, 70 BPM',
        slots: { language: 'Russian', genre: 'russian romance', rhythm: 'free, rubato', guitars: 'fingerpicked classical guitar', keys: 'accordion touches', vocals: 'sorrowful, trembling', mood: 'candle-lit sorrow', production: 'intimate vintage' }, bpm: 70,
      },
    ],
  },
]
// Пользовательские группы (страница «Библиотека») хранятся в localStorage.
export function loadCustomGroups() {
  try {
    const raw = JSON.parse(localStorage.getItem('yue_custom_groups') || '[]')
    return Array.isArray(raw) ? raw : []
  } catch {
    return []
  }
}

export function saveCustomGroups(gs) {
  localStorage.setItem('yue_custom_groups', JSON.stringify(gs))
}
