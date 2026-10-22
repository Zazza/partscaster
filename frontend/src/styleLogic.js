// Чистая логика сборки строки стиля и подготовки стиха к отправке.
import { slotDict, slotOptions, multiSlots } from './slotOptions.js'
import { allRackItems, effectPhrase, rackAdd } from './rack.js'

// порядок слотов в строке стиля — язык/жанр первыми, продакшн последним
export const SLOT_ORDER = [
  'language', 'genre', 'rhythm', 'guitars', 'keys', 'vocals', 'mood', 'production',
]

// строка стиля из слотов (+ строка стойки, + BPM): пустые слоты пропускаются,
// значения нормализуются как списки тегов (мультивыбор слотов: хвостовые запятые выпадают)
export function buildStyleLine(slots, rackStr = '') {
  const parts = SLOT_ORDER
    .map(k => splitSlotTags(slots[k]).join(', '))
    .filter(Boolean)
  if (rackStr) parts.push(rackStr)
  if (slots.bpm) parts.push(slots.bpm + ' BPM')
  return parts.join(', ')
}

// значения слота — список тегов через запятую или точку с запятой (мультивыбор):
// пустые элементы и хвостовые разделители выпадают
export function splitSlotTags(s) {
  return (s || '').split(/\s*[;,]\s*/).map(x => x.trim()).filter(Boolean)
}

// все готовые значения подсказок (ru и en, любой слот) — в нижнем регистре
let knownLabels = null
function isKnownLabel(s) {
  if (!knownLabels) {
    knownLabels = new Set()
    for (const opts of Object.values(slotOptions)) {
      for (const [ru, en] of opts) { knownLabels.add(ru.toLowerCase()); knownLabels.add(en.toLowerCase()) }
    }
  }
  return knownLabels.has(s.toLowerCase())
}

// логические теги слота: соседние куски склеиваются, если вместе образуют словарное
// значение — русские подписи бывают из нескольких частей через запятую
// («1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски»), и при наивном
// разрезании их перевод бы терялся; английские составные подписи подсказок («melancholy, sadness») — так же.
// Кусок без словарной склейки — самописный тег
export function slotTagList(s) {
  const items = splitSlotTags(s)
  const out = []
  let i = 0
  while (i < items.length) {
    let len = 0
    for (let j = items.length; j > i; j--) {
      const chunk = items.slice(i, j).join(', ').toLowerCase()
      if (slotDict[chunk] !== undefined || isKnownLabel(chunk)) { len = j - i; break }
    }
    if (!len) len = 1
    out.push(items.slice(i, i + len).join(', '))
    i += len
  }
  return out
}

// значение слота → английские теги: каждый логический тег переводится отдельно
// (составное «блюз, дум-метал» словарю не принадлежит — переводить надо поштучно),
// словарное русское подставляется словарём, самописное (в т.ч. английское) проходит как есть
export function dictStyle(s) {
  return slotTagList(s)
    .map(x => slotDict[x.toLowerCase()] || x)
    .join(', ')
}

// выбор тега из подсказок в мультислоте: хвост (последний логический тег поля, если поле не кончается запятой)
// — черновик поиска, им фильтровались подсказки, и выбранный тег его заменяет. Хвост, равный целиком готовому
// значению подсказок, — уже выбранный тег (загруженный стиль без запятой на конце) — он остаётся; составное
// значение с запятой внутри («грустно, тоска») — один логический тег.
// Новый тег добавляется без дубликата (сравнение по логическим тегам — составные словарные значения равны
// себе целиком). Запятая на конце — приглашение продолжать список: после выбора хвост пуст — список снова полный
export function pickMultiSlotTag(current, tag) {
  const cur = current || ''
  const t = (tag || '').trim()
  const items = slotTagList(cur)
  const open = /[;,]\s*$/.test(cur)   // запятая на конце — хвоста-черновика нет
  if (!open && items.length && !isKnownLabel(items[items.length - 1])) items.pop()
  if (t && !items.some(x => x.toLowerCase() === t.toLowerCase())) items.push(t)
  return items.join(', ') + ', '
}

// «Разобрать по полям» точно: слоты и BPM — ровно из разбора строки (чего в строке нет — пусто, а не старое)
export function applyParsedSlots(slots, res) {
  const out = { ...slots }
  for (const k of SLOT_ORDER) out[k] = (res.slots && res.slots[k]) || ''
  out.bpm = res.bpm ?? null
  return out
}

// хвост строки стиля после слотов: стойка и нераспознанные при разборе куски (уходят в отправку как есть)
export function styleExtra(rackStr, unknown = []) {
  return [rackStr, ...unknown].map(x => (x || '').trim()).filter(Boolean).join(', ')
}

// Обратный разбор строки стиля в поля формы: слоты, BPM и инструментальная стойка.
// Словарные значения (русские и английские ключи) расходятся по своим слотам — мультислоты
// копятся без дублей, одиночные берут первое (второе — в «не распознано»); «N BPM» — в BPM;
// фразы стойки (phrasing инструмента, при эффекте — с припиской) — карточками в стойку.
// Совпадающее значение слота и фразы стойки («fuzz wall of guitars») — слот: словарные
// значения первичны, фраза с эффектом («… drenched in reverb») стойке всё равно уникальна.
// Нераспознанные куски возвращаются списком — их показываем под полем, а не теряем
export function parseStyleLine(line, locale = 'ru') {
  const bySlot = new Map()
  for (const [key, opts] of Object.entries(slotOptions)) {
    for (const [ru, en] of opts) {
      const value = locale === 'en' ? en : ru
      bySlot.set(ru.toLowerCase(), { key, value })
      bySlot.set(en.toLowerCase(), { key, value })
    }
  }
  const byRack = new Map()
  for (const it of allRackItems()) {
    byRack.set(it.phrasing.toLowerCase(), { id: it.id, effect: '' })
    for (const [eff, phr] of Object.entries(effectPhrase)) {
      byRack.set((it.phrasing + ' ' + phr).toLowerCase(), { id: it.id, effect: eff })
    }
  }

  const slots = {}
  let bpm = null
  let rack = []
  const unknown = []
  const items = splitSlotTags(line)
  let i = 0
  while (i < items.length) {
    // жадно: самый длинный чанк из оставшихся кусков — словарные значения и фразы стойки
    // бывают из нескольких частей через запятую («1980-е …, панк-энергия…», «instrumental, no vocals»)
    let hit = null
    let len = 0
    for (let j = items.length; j > i; j--) {
      const chunk = items.slice(i, j).join(', ').toLowerCase()
      const s = bySlot.get(chunk)
      if (s) { hit = { kind: 'slot', ...s }; len = j - i; break }
      const r = byRack.get(chunk)
      if (r) { hit = { kind: 'rack', ...r }; len = j - i; break }
      const m = chunk.match(/^(\d{2,3}) bpm$/)
      if (m) { hit = { kind: 'bpm', value: Number(m[1]) }; len = j - i; break }
    }
    if (!hit) { unknown.push(items[i]); i += 1; continue }
    const raw = items.slice(i, i + len).join(', ')
    if (hit.kind === 'slot') {
      if (multiSlots.has(hit.key)) {
        const cur = splitSlotTags(slots[hit.key])
        if (!cur.some(x => x.toLowerCase() === hit.value.toLowerCase())) cur.push(hit.value)
        slots[hit.key] = cur.join(', ')
      } else if (!(hit.key in slots)) slots[hit.key] = hit.value
      else unknown.push(raw)
    } else if (hit.kind === 'rack') {
      rack = rackAdd(rack, hit.id, hit.effect)
    } else if (bpm === null) {
      bpm = hit.value
    } else {
      unknown.push(raw)
    }
    i += len
  }
  return { slots, bpm, rack, unknown }
}

// стих к отправке: строки-пометки (начинаются с #) вырезаются — это заметки
// для себя (ударения, произношение), модель их не поёт
export function cleanLyrics(text) {
  return text.split('\n').filter(l => !l.trim().startsWith('#')).join('\n')
}

// «без слов»: вместо стиха — одна секция [Instrumental]. Длину решает модель планом (3–5 мин),
// число секций она не слушает (замер 2026-10-20) — выбора длительности нет
export function instrumentalLyrics() {
  return '[Instrumental]'
}

export function effectiveLyrics(lyrics, noLyrics) {
  return noLyrics ? instrumentalLyrics() : cleanLyrics(lyrics)
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
