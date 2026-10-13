// Чистая логика сборки строки стиля и подготовки стиха к отправке.
import { slotDict, durOptions } from './slotOptions.js'

// порядок слотов в строке стиля — язык/жанр первыми, продакшн последним
export const SLOT_ORDER = [
  'language', 'genre', 'rhythm', 'guitars', 'keys', 'vocals', 'mood', 'production',
]

// строка стиля из слотов (+ строка стойки, + BPM): пустые слоты пропускаются
export function buildStyleLine(slots, rackStr = '') {
  const parts = SLOT_ORDER
    .map(k => (slots[k] || '').trim())
    .filter(Boolean)
  if (rackStr) parts.push(rackStr)
  if (slots.bpm) parts.push(slots.bpm + ' BPM')
  return parts.join(', ')
}

// значение слота → английский тег: известное русское подставляется словарём,
// неизвестное (в т.ч. самописное) проходит как есть
export function dictStyle(s) {
  const t = (s || '').trim()
  return slotDict[t.toLowerCase()] || t
}

// стих к отправке: строки-пометки (начинаются с #) вырезаются — это заметки
// для себя (ударения, произношение), модель их не поёт
export function cleanLyrics(text) {
  return text.split('\n').filter(l => !l.trim().startsWith('#')).join('\n')
}

// «без слов»: вместо стиха — n секций [Instrumental] (n из режима длительности)
export function instrumentalLyrics(durMode) {
  const opt = durOptions.find(o => o.id === durMode) || durOptions[0]
  return Array(Math.max(1, opt.n)).fill('[Instrumental]').join('\n\n')
}

export function effectiveLyrics(lyrics, noLyrics, durMode) {
  return noLyrics ? instrumentalLyrics(durMode) : cleanLyrics(lyrics)
}

// «без слов» для слотов стиля: включили — голос стиля → «без голоса» (прежний запоминается в kept),
// выключили — прежний голос возвращается, если пользователь не вписал свой. Вход не меняется.
export const INSTRUMENTAL_VOCALS = 'instrumental, no vocals'

export function withInstrumental(slots, on, kept = '') {
  const out = { ...slots }
  if (on) {
    if (out.vocals === INSTRUMENTAL_VOCALS) return { slots: out, kept }
    return { slots: { ...out, vocals: INSTRUMENTAL_VOCALS }, kept: out.vocals || '' }
  }
  if (out.vocals === INSTRUMENTAL_VOCALS) out.vocals = kept
  return { slots: out, kept: '' }
}

// «без слов» для стиля точной строкой (1:1): строка уходит как есть, но с «Instrumental, no vocals» в начале
// (если ещё нет) — иначе модель получала бы заглушку стиха при описании голоса в строке
export function instrumentalLine(line, on) {
  if (!on) return line
  const s = (line || '').trim()
  if (!s) return ''
  if (/^instrumental/i.test(s)) return line
  return `Instrumental, no vocals, ${s}`
}
