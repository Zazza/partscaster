// Тесты шага «Голос» студии: замена голоса в строке стиля, кусок для пробы,
// есть ли слова, запрос джобы «перепеть», разбор params карточки.
// Источник — карточка internal-studio-voice (ТК1–ТК12), не реализация.
import { describe, it, expect } from 'vitest'
import {
  swapVocals, hasSungWords, voiceProbe, voiceTrackJob, voiceCardParams,
  defaultVoiceParams, normalizeVoiceParams, voiceDescriptor,
} from './voiceLab.js'

describe('swapVocals', () => {
  it('ТК1: вокальный тег заменяется дескриптором на своём месте', () => {
    expect(swapVocals('English, post-punk, deadpan baritone, dry, 145 BPM', 'female vocals'))
      .toBe('English, post-punk, female vocals, dry, 145 BPM')
  })

  it('ТК2: несколько вокальных тегов убираются все, дескриптор один раз на месте первого', () => {
    expect(swapVocals('English, female vocals, rock, breathy voice, soaring singer, 120 BPM', 'male vocals'))
      .toBe('English, male vocals, rock, 120 BPM')
  })

  it('ТК2: каждое слово-признак голоса делает тег вокальным', () => {
    const words = ['vocals', 'vocal', 'voice', 'singer', 'singing', 'sung', 'sings', 'baritone',
      'tenor', 'soprano', 'alto', 'falsetto', 'crooner', 'rap', 'rapper', 'rapped']
    for (const w of words) {
      expect(swapVocals(`rock, smooth ${w}, dry`, 'X')).toBe('rock, X, dry')
    }
  })

  it('ТК3: вокальных тегов нет — дескриптор перед «N BPM» в конце', () => {
    expect(swapVocals('English, rock, dry, 120 BPM', 'male vocals'))
      .toBe('English, rock, dry, male vocals, 120 BPM')
  })

  it('ТК3: нет ни вокальных тегов, ни BPM — дескриптор в конец', () => {
    expect(swapVocals('English, rock, dry', 'male vocals'))
      .toBe('English, rock, dry, male vocals')
  })

  it('ТК4: без учёта регистра — «Raspy Voice» и «BARITONE» убираются', () => {
    expect(swapVocals('English, Raspy Voice, rock', 'female vocals'))
      .toBe('English, female vocals, rock')
    expect(swapVocals('English, BARITONE, rock', 'female vocals'))
      .toBe('English, female vocals, rock')
  })

  it('ТК4: только целым словом — «vocalist» не вокальный тег', () => {
    expect(swapVocals('jazz, vocalist harmonies, 90 BPM', 'female vocals'))
      .toBe('jazz, vocalist harmonies, female vocals, 90 BPM')
  })

  it('ТК4: регистровое слово перед инструментом — тег инструмента, остаётся', () => {
    const style = 'jazz, tenor sax, baritone guitar, alto saxophone, soprano horn'
    expect(swapVocals(style, 'female vocals')).toBe(style + ', female vocals')
  })

  it('ТК4: все инструменты из условия защищают регистровое слово', () => {
    for (const ins of ['sax', 'saxophone', 'horn', 'guitar', 'ukulele', 'clarinet', 'trombone', 'recorder']) {
      expect(swapVocals(`folk, alto ${ins}`, 'X')).toBe(`folk, alto ${ins}, X`)
    }
  })

  it('ТК4: инструмент и голос рядом — уходит только голос', () => {
    expect(swapVocals('jazz, tenor sax, smooth tenor, 100 BPM', 'female vocals'))
      .toBe('jazz, tenor sax, female vocals, 100 BPM')
  })

  it('ТК5: пустой дескриптор — строка без изменений', () => {
    const style = 'English, post-punk, deadpan baritone, dry, 145 BPM'
    expect(swapVocals(style, '')).toBe(style)
  })

  it('ТК5: пустой стиль — только дескриптор', () => {
    expect(swapVocals('', 'female vocals')).toBe('female vocals')
  })
})

describe('hasSungWords', () => {
  it('ТК6: тег и строка слов — есть слова', () => {
    expect(hasSungWords('[Verse]\nla la')).toBe(true)
  })

  it('ТК6: только теги и пустые строки — слов нет', () => {
    expect(hasSungWords('[Instrumental]\n\n[Instrumental]')).toBe(false)
  })

  it('ТК6: пустой текст — слов нет', () => {
    expect(hasSungWords('')).toBe(false)
    expect(hasSungWords('  \n \n')).toBe(false)
  })

  it('ТК13: слова не латиницей (кириллица, CJK) — есть слова', () => {
    expect(hasSungWords('[Verse]\nпривет')).toBe(true)
    expect(hasSungWords('[Verse]\n你好')).toBe(true)
    expect(hasSungWords('[Verse]\n안녕')).toBe(true)
  })

  it('слова без тегов — есть слова', () => {
    expect(hasSungWords('just words here')).toBe(true)
  })
})

// --- план для пробы -------------------------------------------------------

const HEAD = [
  'X:1',
  'T:probe test',
  'M:4/4',
  'L:1/16',
  'Q:1/4=120',
  'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
  'V: Ins clef=treble name="Ins Melody" snm="Inst."',
  'K:Dm',
]

// Такты Vocal: 0–1 интро (только паузы под аккордами — буквы аккордов не ноты),
// 2 — первый такт куплета, тоже пауза; 3 — первая нота голоса; дальше ноты.
// Всего 14 тактов, от первой ноты остаётся 11 — больше лимита 8.
const VOCAL_BARS = [
  '"Dm"z16', '"Bb"z16',
  '"F"z16', '"C"z8a8', '"Dm"d4f4a4f4', '"Bb"d4f4g4f4',
  '"F"c4f4a4g4', '"C"e4g4g4e4', '"Dm"d4f4a4f4', '"Bb"d4f4g4d4',
  '"F"c4a4f4c4', '"C"e4g4e4c4', '"Dm"f4a4d4a4', '"A"e4^c4e4a4',
]
// Ins: у каждого такта своё содержимое, чтобы отличать номера
const INS_BARS = VOCAL_BARS.map((_, i) => `"Dm"D${'ABCDEFG'[i % 7]}${i + 1}`)

function plan(vocal = VOCAL_BARS, ins = INS_BARS) {
  const row = (bars, a, b) => bars.slice(a, b).join('|') + '|'
  return [
    ...HEAD,
    '% intro',
    'V: Vocal', row(vocal, 0, 2),
    'V: Ins', row(ins, 0, 2),
    '% verse',
    'V: Vocal', row(vocal, 2, 6),
    'V: Ins', row(ins, 2, 6),
    'V: Vocal', row(vocal, 6, 10),
    'V: Ins', row(ins, 6, 10),
    'V: Vocal', row(vocal, 10, 14),
    'V: Ins', row(ins, 10, 14),
    '',
  ].join('\n')
}

// такты по голосам в порядке появления: { Vocal: [...], Ins: [...] }
function barsByVoice(abc) {
  const out = {}
  let voice = null
  for (const raw of abc.split('\n')) {
    const line = raw.trim()
    if (!line || line.startsWith('%')) continue
    const vm = line.match(/^V:\s*(\S+)/)
    if (vm) { voice = vm[1]; out[voice] = out[voice] || []; continue }
    if (/^[A-Za-z]:/.test(line) || !voice) continue
    for (const b of line.split('|')) if (b.trim()) out[voice].push(b.trim())
  }
  return out
}

const LYRICS = '[Intro]\n\n[Verse]\nfirst line of verse\nsecond line of verse\n\n[Chorus]\nchorus line\n'

describe('voiceProbe — план', () => {
  it('ТК7: кусок начинается с первого такта Vocal с нотой, не больше 8 тактов', () => {
    const r = voiceProbe(plan(), LYRICS)
    expect(r).not.toBeNull()
    const v = barsByVoice(r.abc).Vocal
    expect(v.length).toBeGreaterThan(0)
    expect(v.length).toBeLessThanOrEqual(8)
    expect(v[0]).toBe('"C"z8a8')
    // сплошной кусок исходных тактов подряд
    expect(v).toEqual(VOCAL_BARS.slice(3, 3 + v.length))
  })

  it('ТК7: у Ins те же такты, что у Vocal', () => {
    const r = voiceProbe(plan(), LYRICS)
    const { Vocal, Ins } = barsByVoice(r.abc)
    expect(Ins).toEqual(INS_BARS.slice(3, 3 + Vocal.length))
  })

  it('ТК7: строки заголовка плана сохранены', () => {
    const r = voiceProbe(plan(), LYRICS)
    const lines = r.abc.split('\n').map((s) => s.trim())
    for (const h of HEAD) expect(lines).toContain(h)
  })

  it('ТК7: лимит тактов из параметра', () => {
    const r = voiceProbe(plan(), LYRICS, 4)
    const { Vocal, Ins } = barsByVoice(r.abc)
    expect(Vocal.length).toBeGreaterThan(0)
    expect(Vocal.length).toBeLessThanOrEqual(4)
    expect(Vocal[0]).toBe('"C"z8a8')
    expect(Ins).toEqual(INS_BARS.slice(3, 3 + Vocal.length))
  })

  it('ТК7: такт с нотой в конце плана — кусок до конца, без выхода за план', () => {
    const vocal = VOCAL_BARS.map((b, i) => (i < 12 ? '"Dm"z16' : b))
    const r = voiceProbe(plan(vocal), LYRICS)
    const { Vocal, Ins } = barsByVoice(r.abc)
    expect(Vocal[0]).toBe(VOCAL_BARS[12])
    expect(Vocal).toEqual(VOCAL_BARS.slice(12, 12 + Vocal.length))
    expect(Vocal.length).toBeLessThanOrEqual(2)
    expect(Ins).toEqual(INS_BARS.slice(12, 12 + Vocal.length))
  })

  it('ТК9: у Vocal нет нот (только паузы под аккордами) — null', () => {
    const vocal = VOCAL_BARS.map(() => '"Dm"z16')
    expect(voiceProbe(plan(vocal), LYRICS)).toBeNull()
  })

  it('ТК9: в плане нет голоса Vocal — null', () => {
    const abc = [...HEAD.filter((h) => !h.startsWith('V: Vocal')), '% verse', 'V: Ins',
      INS_BARS.join('|') + '|', ''].join('\n')
    expect(voiceProbe(abc, LYRICS)).toBeNull()
  })

  it('ТК9: пустой план — null', () => {
    expect(voiceProbe('', LYRICS)).toBeNull()
  })
})

describe('voiceProbe — текст', () => {
  it('ТК8: первый блок со словами вместе с его тегом; пустое интро пропущено', () => {
    const { lyrics } = voiceProbe(plan(), LYRICS)
    expect(lyrics).toContain('[Verse]')
    expect(lyrics).toContain('first line of verse')
    expect(lyrics).toContain('second line of verse')
    expect(lyrics).not.toContain('[Intro]')
    expect(lyrics).not.toContain('[Chorus]')
    expect(lyrics).not.toContain('chorus line')
    expect(lyrics.trim().startsWith('[Verse]')).toBe(true)
  })

  it('ТК8: блок длится до следующего тега, даже без пустой строки между ними', () => {
    const { lyrics } = voiceProbe(plan(), '[Intro]\n[Verse 1]\nalpha\nbeta\n[Chorus]\ngamma')
    expect(lyrics).toContain('[Verse 1]')
    expect(lyrics).toContain('alpha')
    expect(lyrics).toContain('beta')
    expect(lyrics).not.toContain('gamma')
    expect(lyrics).not.toContain('[Intro]')
  })

  it('ТК13: блок со словами кириллицей находится', () => {
    const { lyrics } = voiceProbe(plan(), '[Intro]\n\n[Куплет]\nпервая строка\n\n[Припев]\nприпев')
    expect(lyrics).toContain('первая строка')
    expect(lyrics).not.toContain('припев\n')
    expect(lyrics.trim().startsWith('[Куплет]')).toBe(true)
  })

  it('ТК8: текст без тегов — первый абзац', () => {
    const { lyrics } = voiceProbe(plan(), 'first a\nfirst b\n\nsecond c\nsecond d')
    expect(lyrics).toContain('first a')
    expect(lyrics).toContain('first b')
    expect(lyrics).not.toContain('second c')
  })
})

// --- запрос джобы --------------------------------------------------------

function track(over = {}) {
  return {
    id: 42,
    title: 'Warm Wire',
    style: 'English, post-punk, deadpan baritone, dry, 145 BPM',
    lyrics: '[Verse]\nla la',
    seed: 777,
    cot: 'full',
    ...over,
  }
}

const MALE = { ...defaultVoiceParams(), register: 'male', rough: 1 }

describe('voiceTrackJob', () => {
  it('ТК10: «перепеть весь трек» — поля пересборки с заменённым голосом', () => {
    const job = track()
    const p = voiceTrackJob(job, MALE, { abc: 'X:1\nPLAN', lyrics: job.lyrics, title: 'Warm Wire · голос Тест', draft: false })
    expect(p).toMatchObject({
      title: 'Warm Wire · голос Тест',
      style: `English, post-punk, ${voiceDescriptor(MALE)}, dry, 145 BPM`,
      lyrics: '[Verse]\nla la',
      seed: 777,
      cot: 'full',
      abc: 'X:1\nPLAN',
      draft: false,
      parent_id: 42,
      role: 'rebuild',
    })
  })

  it('ТК10: трек не меняется', () => {
    const job = track()
    const copy = JSON.parse(JSON.stringify(job))
    voiceTrackJob(job, MALE, { abc: 'X:1', lyrics: job.lyrics, title: 't', draft: false })
    expect(job).toEqual(copy)
  })

  it('ТК10: без сида у трека — seed 0', () => {
    for (const seed of [undefined, null, 0]) {
      const p = voiceTrackJob(track({ seed }), MALE, { abc: 'X:1', lyrics: 'x', title: 't', draft: false })
      expect(p.seed).toBe(0)
    }
  })

  it('ТК10: cot off и пустой → melody', () => {
    for (const cot of ['off', '', undefined, null]) {
      const p = voiceTrackJob(track({ cot }), MALE, { abc: 'X:1', lyrics: 'x', title: 't', draft: false })
      expect(p.cot).toBe('melody')
    }
    const m = voiceTrackJob(track({ cot: 'melody' }), MALE, { abc: 'X:1', lyrics: 'x', title: 't', draft: false })
    expect(m.cot).toBe('melody')
  })

  it('ТК10: в стиле нет голоса — дескриптор перед BPM', () => {
    const p = voiceTrackJob(track({ style: 'English, rock, 120 BPM' }), MALE,
      { abc: 'X:1', lyrics: 'x', title: 't', draft: false })
    expect(p.style).toBe(`English, rock, ${voiceDescriptor(MALE)}, 120 BPM`)
  })

  it('ТК11: проба — role fragment, draft true, переданные план и текст', () => {
    const job = track()
    const p = voiceTrackJob(job, MALE, { abc: 'X:1\nPIECE', lyrics: '[Verse]\nonly piece', title: 'проба голоса · свой', draft: true })
    expect(p).toMatchObject({
      title: 'проба голоса · свой',
      style: `English, post-punk, ${voiceDescriptor(MALE)}, dry, 145 BPM`,
      lyrics: '[Verse]\nonly piece',
      abc: 'X:1\nPIECE',
      seed: 777,
      cot: 'full',
      draft: true,
      parent_id: 42,
      role: 'fragment',
    })
  })
})

describe('voiceCardParams', () => {
  it('ТК12: битый JSON — дефолтные params', () => {
    expect(voiceCardParams({ params: '{not json' })).toEqual(defaultVoiceParams())
  })

  it('ТК12: пусто — дефолтные params', () => {
    expect(voiceCardParams({ params: '' })).toEqual(defaultVoiceParams())
    expect(voiceCardParams({})).toEqual(defaultVoiceParams())
    expect(voiceCardParams({ params: null })).toEqual(defaultVoiceParams())
  })

  it('ТК12: поля клампятся как в normalizeVoiceParams', () => {
    const raw = { v: 1, register: 'alien', rough: 99, creak: -3, delivery: 2, breath: 1, extra: '  husky  ' }
    expect(voiceCardParams({ params: JSON.stringify(raw) })).toEqual(normalizeVoiceParams(raw))
  })

  it('ТК12: сохранённая карточка читается обратно в те же ручки', () => {
    const p = { register: 'male-low', rough: 2, creak: 1, delivery: 3, breath: 0, extra: 'dark' }
    expect(voiceCardParams({ name: 'x', params: JSON.stringify({ v: 1, ...p }) })).toEqual(p)
  })
})
