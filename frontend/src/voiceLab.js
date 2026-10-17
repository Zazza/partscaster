// Примерочная голосов: чистая логика «ручки → дескриптор вокала».
// YuE2 кондиционируется только текстом, поэтому ручки — детерминированный
// маппинг в английские теги; дескриптор пересчитывается из params всегда
// на клиенте (на воркере хранятся только params и seed).
import { buildStyleLine } from './styleLogic.js'
import { planTimeline, sliceAbc, splitBars, expandMultiRests } from './abcEdit.js'

export const PARAMS_VERSION = 1

// Регистр-выбор для VSelect (лейблы — voicelab.register.* в i18n).
export const VOICE_REGISTERS = ['female', 'male', 'male-high', 'male-low']

// Слайдеры: key + максимум уровня. Подписи и названия уровней — voicelab.<key>.*
export const VOICE_KNOBS = [
  { key: 'rough', max: 3 },
  { key: 'creak', max: 2 },
  { key: 'delivery', max: 3 },
  { key: 'breath', max: 2 },
]

const REGISTER_TOKENS = {
  'female': 'female vocals',
  'male': 'male vocals',
  'male-high': 'high tenor male vocals',
  'male-low': 'deep resonant low male vocals',
}
const ROUGH_TOKENS = ['', 'raspy', 'gritty raspy', 'gravelly growled']
const CREAK_TOKENS = ['', 'creaky', 'vocal fry']
const DELIVERY_TOKENS = ['soft intimate delivery', 'even delivery', 'powerful delivery',
  'raw breaking delivery, on the verge of tears']
const BREATH_TOKENS = ['', 'breathy', 'whispered']

// Стенд прослушивания: фиксированный контекст, чтобы голоса были сравнимы
// между собой — меняется только дескриптор вокала. Свой ABC-план (механизм
// req_abc воркера): без интро, припев с первой доли — иначе draft-проба (~18 с,
// это начало трека) уходила на инструментальное интро и слов не было слышно.
// Формат подсмотрен в планах самой модели (score.abc): секции комментариями,
// Vocal/Ins по голосам, L:1/16 — длительности в 16-х.
export const AUDITION_BED = {
  slots: {
    language: 'English',
    genre: 'energetic pop rock',
    rhythm: 'driving mid-tempo beat',
    guitars: '',
    keys: '',
    mood: '',
    production: 'clean production, vocals up front',
    bpm: 110,
  },
  lyrics: '[Chorus]\nTake it all, take it now\nSay the only word out loud\nBurn it down and make it true\nNothing left but me and you\n',
  cot: 'full',
  abc: [
    'X:1',
    'T:voice audition',
    'M:4/4',
    'L:1/16',
    'Q:1/4=110',
    'V: Vocal clef=treble name="Vocal Melody" snm="Vocal"',
    'V: Ins clef=treble name="Ins Melody" snm="Inst."',
    'K:Dm',
    '% chorus',
    'V: Vocal',
    '"Dm"z4d2f2a2a2f2d2|"Bb"z4d2f2g2g2f2d2|"F"z4c2f2a2a2g2f2|"C"z4e2g2g2e2d2e2|',
    'V: Ins',
    '"Dm"d4f2d2f2g2gfd2|"Bb"d4f2d2f2g2gfd2|"F"c4a2f2a2g2gfc2|"C"e4g2e2g2a2age2|',
    'V: Vocal',
    '"Dm"z4d2f2a2a2f2d2|"Bb"z4d2f2g2g2f2d2|"F"z4c2f2a2a2g2f2|"C"z4e2g2g2e2d2e2|',
    'V: Ins',
    '"Dm"d4f2d2f2g2gfd2|"Bb"d4f2d2f2g2gfd2|"F"c4a2f2a2g2gfc2|"C"e4g2e2g2a2age2|',
    '',
  ].join('\n'),
}

export function defaultVoiceParams() {
  return { register: 'female', rough: 0, creak: 0, delivery: 1, breath: 0, extra: '' }
}

// Быстрые пресеты: задают стартовые ручки + «своё», дальше пользователь
// докручивает. Это данные, не поведение: страница применяет их через
// normalizeVoiceParams. Имена — voicelab.preset.<id> в i18n.
export const VOICE_PRESETS = [
  {
    id: 'operatic',
    params: { register: 'female', rough: 0, creak: 0, delivery: 2, breath: 0,
      extra: 'operatic soprano, symphonic' },
  },
  {
    id: 'dark-drama',
    params: { register: 'female', rough: 1, creak: 0, delivery: 3, breath: 0,
      extra: 'dramatic dark, soaring powerful' },
  },
  {
    id: 'scream',
    params: { register: 'female', rough: 2, creak: 0, delivery: 3, breath: 0,
      extra: 'screamed chorus, desperate' },
  },
  {
    id: 'whisper',
    params: { register: 'female', rough: 0, creak: 0, delivery: 0, breath: 2, extra: '' },
  },
  {
    id: 'velvet-low',
    params: { register: 'male-low', rough: 1, creak: 1, delivery: 2, breath: 0, extra: '' },
  },
  {
    id: 'reset',
    params: defaultVoiceParams(),
  },
]

// Вход из карточки/MCP может быть любым: клампим ручки, белый список регистров,
// недостающее добираем из дефолта.
export function normalizeVoiceParams(o) {
  const d = defaultVoiceParams()
  const src = (o && typeof o === 'object') ? o : {}
  // нет ручки или не число — значение по умолчанию (подача по умолчанию не 0)
  const clamp = (v, max, def) => {
    const n = Number.parseInt(v, 10)
    return Number.isFinite(n) ? Math.min(Math.max(n, 0), max) : def
  }
  const out = { ...d, register: VOICE_REGISTERS.includes(src.register) ? src.register : d.register }
  for (const k of VOICE_KNOBS) out[k.key] = clamp(src[k.key], k.max, d[k.key])
  out.extra = typeof src.extra === 'string' ? src.extra.trim() : ''
  return out
}

// Манеры голоса: подпись — описание манеры, теги — английские клише (имён исполнителей
// нет ни в подписях, ни в промпте: модель их не знает, а имя обещало бы «как у X»).
export const VOICE_ARCHETYPES = [
  {
    id: 'nasal-strain', params: { register: 'male', rough: 2, creak: 1, delivery: 3, breath: 0,
      extra: 'nasal, cracked, shouted, off-key' },
  },
  {
    id: 'fragile-falsetto', params: { register: 'male-high', rough: 0, creak: 0, delivery: 0, breath: 1,
      extra: 'nasal, frail falsetto, anxious, mournful' },
  },
  {
    id: 'piercing-wail', params: { register: 'male-high', rough: 1, creak: 0, delivery: 3, breath: 0,
      extra: 'piercing whiny wail, nasal, manic' },
  },
  {
    id: 'androgynous-sneer', params: { register: 'male-high', rough: 0, creak: 0, delivery: 2, breath: 0,
      extra: 'androgynous, sneering, seductive' },
  },
  {
    id: 'cold-baritone', params: { register: 'male', rough: 0, creak: 0, delivery: 1, breath: 0,
      extra: 'deadpan baritone, cold, cryptic, unsettling' },
  },
  {
    id: 'husky-laidback', params: { register: 'female', rough: 1, creak: 0, delivery: 2, breath: 0,
      extra: 'husky, laid-back phrasing, confident' },
  },
  {
    id: 'purring-mannered', params: { register: 'male-high', rough: 0, creak: 0, delivery: 1, breath: 1,
      extra: 'purring, mannered, drawling, playful' },
  },
  {
    id: 'grunge-strain', params: { register: 'male', rough: 2, creak: 1, delivery: 3, breath: 0,
      extra: 'grunge whine, cracked, desperate' },
  },
  {
    id: 'dark-preacher', params: { register: 'male-low', rough: 1, creak: 0, delivery: 2, breath: 0,
      extra: 'deep baritone, ominous, sermonizing' },
  },
  {
    id: 'smoky-soul', params: { register: 'female', rough: 1, creak: 0, delivery: 2, breath: 0,
      extra: 'smoky soul, jazzy phrasing, brazen' },
  },
  {
    id: 'eccentric-leaps', params: { register: 'female', rough: 0, creak: 0, delivery: 3, breath: 0,
      extra: 'eccentric, wild leaps, child-like' },
  },
  {
    id: 'talk-singing', params: { register: 'male', rough: 1, creak: 1, delivery: 1, breath: 0,
      extra: 'talk-singing, deadpan, dry' },
  },
  {
    id: 'rasping-allout', params: { register: 'male', rough: 3, creak: 2, delivery: 3, breath: 0,
      extra: 'rasping, straining, theatrical intensity' },
  },
  {
    id: 'detached-enigmatic', params: { register: 'male', rough: 0, creak: 0, delivery: 1, breath: 0,
      extra: 'cool, detached, enigmatic' },
  },
]

// Ручки → английский дескриптор вокального слота. Детерминирован: одинаковые
// params → одинаковая строка. Порядок фиксирован: регистр, скрип, грубость,
// дыхание, подача; extra — последним, своим текстом.
export function voiceDescriptor(p) {
  const n = normalizeVoiceParams(p)
  const parts = [REGISTER_TOKENS[n.register]]
  if (n.creak > 0) parts.push(CREAK_TOKENS[n.creak])
  if (n.rough > 0) parts.push(ROUGH_TOKENS[n.rough])
  // шёпот побеждает экстремальную подачу: «на разрыв» шёпотом не бывает
  if (n.breath === 2 && n.delivery >= 3) {
    parts.push('hushed whispered delivery')
  } else {
    if (n.breath > 0) parts.push(BREATH_TOKENS[n.breath])
    parts.push(DELIVERY_TOKENS[n.delivery])
  }
  if (n.extra) parts.push(n.extra)
  return parts.join(', ')
}

// Строка стиля для джобы-прослушивания: стенд + дескриптор вокала.
export function auditionStyle(vocalsDescriptor) {
  return buildStyleLine({ ...AUDITION_BED.slots, vocals: vocalsDescriptor || '' })
}

// Полный payload для api.submit: draft-превью на стенде с конкретным seed
// и фиксированным планом без интро. Seed обязан быть конкретным числом (не 0):
// воркер не пишет обратно использованный случайный seed, воспроизводимость
// голоса — только через seed.
export function auditionJob(p, seed) {
  return {
    title: 'voice audition',
    style: auditionStyle(voiceDescriptor(p)),
    lyrics: AUDITION_BED.lyrics,
    seed,
    cot: AUDITION_BED.cot,
    abc: AUDITION_BED.abc,
    draft: true,
  }
}

// Params карточки (JSON-строка с воркера) → ручки; битое/пустое — дефолт.
export function voiceCardParams(card) {
  try { return normalizeVoiceParams(JSON.parse((card && card.params) || '{}')) } catch { return defaultVoiceParams() }
}

// ---- шаг «Голос» студии: тот же трек другим голосом ----

// тег стиля о голосе: слово-признак целиком; регистровое слово перед инструментом
// («tenor sax», «baritone guitar») — инструмент, не голос
const VOCAL_WORD_RE = /\b(vocals?|voice|singer|singing|sung|sings|falsetto|crooner|rap|rapper|rapped)\b/i
const VOCAL_RANGE_RE = /\b(baritone|tenor|soprano|alto)\b(?!\s+(sax|saxophone|horn|guitar|ukulele|clarinet|trombone|recorder)\b)/i
const BPM_TAG_RE = /^\d+\s*bpm$/i
const isVocalTag = (tag) => VOCAL_WORD_RE.test(tag) || VOCAL_RANGE_RE.test(tag)

// У трека хранится только готовая строка стиля (слотов нет): теги о голосе убираются,
// дескриптор встаёт на место первого из них; тегов о голосе нет — перед «N BPM» в конце,
// нет и его — в конец. Пустой дескриптор — строка как есть.
export function swapVocals(style, descriptor) {
  const d = String(descriptor || '').trim()
  if (!d) return style
  const tags = String(style || '').split(',').map((x) => x.trim()).filter(Boolean)
  const at = tags.findIndex(isVocalTag)
  const out = tags.filter((x) => !isVocalTag(x))
  if (at >= 0) {
    // место первого вокального тега среди оставшихся: до него убранных нет
    out.splice(at, 0, d)
  } else if (out.length && BPM_TAG_RE.test(out[out.length - 1])) {
    out.splice(out.length - 1, 0, d)
  } else {
    out.push(d)
  }
  return out.join(', ')
}

const LYRIC_TAG_RE = /^\s*\[[^\]]*\]\s*$/
const TAG_ANY_RE = /\[[^\]]*\]/g
// буква или цифра любого алфавита: \w в JS без флага u — только латиница (кириллица, CJK — «без слов»)
const WORD_CHAR_RE = /[\p{L}\p{N}]/u

// В тексте есть что петь: что-то кроме тегов [..] и пробелов (как _has_words воркера).
export function hasSungWords(lyrics) {
  return WORD_CHAR_RE.test(String(lyrics || '').replace(TAG_ANY_RE, ' '))
}

// такт голоса с нотой: аккорды "Dm", украшения !..! и поля [K:..] — не ноты; паузы z/x — не ноты
const hasNote = (bar) => /[A-Ga-g]/.test(bar.replace(/"[^"]*"/g, '').replace(/![^!]*!/g, '').replace(/\[[^\]]*\]/g, ''))

// номера тактов голоса (сквозные, после разворота мультипауз) с нотами
function noteBars(abc, voice) {
  const out = []
  let n = 0
  for (const p of splitBars(abc)) {
    if (p.kind !== 'body' || p.voice !== voice) continue
    const closed = p.raw.trim().endsWith('|')
    const m = closed ? p.chunks.length - 1 : p.chunks.length
    for (let j = 0; j < m; j++, n++) if (hasNote(p.chunks[j])) out.push(n)
  }
  return out
}

// первый блок текста со словами: тег [..] и его строки до следующего тега; без тегов — первый абзац
function firstSungBlock(lyrics) {
  const lines = String(lyrics || '').split('\n')
  if (!lines.some((l) => LYRIC_TAG_RE.test(l))) {
    const para = String(lyrics || '').split(/\n\s*\n/).find((b) => WORD_CHAR_RE.test(b))
    return (para || '').trim()
  }
  const blocks = []
  for (const l of lines) {
    if (LYRIC_TAG_RE.test(l) || !blocks.length) blocks.push([])
    blocks[blocks.length - 1].push(l)
  }
  const b = blocks.find((x) => hasSungWords(x.join('\n')))
  return b ? b.join('\n').trim() : ''
}

// Проба голоса: draft рендерит начало плана, а у трека с интро в первых ~18 с не поют —
// кусок плана с первого такта, где у Vocal есть ноты, не больше maxBars тактов (все голоса,
// заголовок сохраняется) и первый блок текста со словами. Нот у голоса нет — null.
export function voiceProbe(abc, lyrics, maxBars = 8) {
  const text = expandMultiRests(String(abc || ''))
  const tl = planTimeline(text)
  const voice = Object.keys(tl).find((v) => /^vocal/i.test(v))
  if (!voice) return null
  const first = noteBars(text, voice)[0]
  if (first === undefined) return null
  const bars = tl[voice]
  const last = Math.min(first + Math.max(1, maxBars), bars.length) - 1
  return {
    abc: sliceAbc(text, bars[first].start, bars[last].end, 0),
    lyrics: firstSungBlock(lyrics),
  }
}

// Запрос рендера трека другим голосом: стиль трека с заменённым голосом, seed и режим
// плана трека (как у пересборки: off → melody, abc требует full|melody). Полный — rebuild
// (своя версия со своим голосом: 'variant' значил бы «голос родителя»), проба — fragment.
export function voiceTrackJob(job, params, { abc, lyrics, title, draft }) {
  return {
    title,
    style: swapVocals(job.style || '', voiceDescriptor(params)),
    lyrics,
    seed: job.seed || 0,   // 0 — воркер возьмёт сид родителя
    cot: job.cot === 'off' || !job.cot ? 'melody' : job.cot,
    abc,
    draft: !!draft,
    parent_id: job.id,
    role: draft ? 'fragment' : 'rebuild',
  }
}
