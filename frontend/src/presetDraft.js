// Пресет звука в черновике: одна запись «Пресет: <имя>» в «Изменениях трека». Go (PresetDraft) готовит её
// содержимое — записи пересборки с посчитанными громкостями и поправками сверки, финал и мастер; черновик
// раскрывает запись при каждой пересборке. Правка человека на той же дорожке заменяет правку пресета:
// выключил свою — правка пресета снова звучит. Чистая логика без DOM.
import { specOf } from './draftEdits.js'
import { isPlaceRecord } from './mixDesk.js'

const copy = (x) => JSON.parse(JSON.stringify(x))

// запись черновика из ответа PresetDraft
export function presetRecord(result, { id, name }, childId) {
  const r = result || {}
  return {
    childId, instId: 'preset', from: 0, to: 0, lead: 0, beat: 0, db: 0, stems: [], fadeIn: 0, fadeOut: 0,
    keepHighHz: 0, step: 'done', label: name,
    preset: { id, name, sections: copy(r.sections || []), final: copy(r.final || []), master: copy(r.master || []),
      warnings: copy(r.warnings || []) },
  }
}

// новый пресет — первым (основа, правки человека — поверх), прежний пресет уходит
export function withPreset(list, record) {
  return [copy(record), ...(list || []).filter((it) => !it.preset)]
}

const hasSound = (x) => !!(x.chain || x.steps || (x.engine && x.engine.length))

// что перекрывают включённые правки человека: дорожки со звуком, дорожки с местом, мастер
function overrides(list) {
  const sound = new Set()
  const place = new Set()
  let master = false
  for (const it of list) {
    if (it.preset) continue
    if (it.master) { master = true; continue }
    if (isPlaceRecord(it)) { place.add(it.stems[0]); continue }
    if (!it.add && !it.mix && !(it.childId > 0) && hasSound(it)) for (const s of it.stems || []) sound.add(s)
  }
  return { sound, place, master }
}

// «место»-запись пресета: место одной дорожки без обработки
const isPlaceSection = (s) => !!s.place && !s.add && !hasSound(s) && !(s.envelope && s.envelope.length)
  && (s.stems || []).length === 1

const whole = { child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, fade_in: 0, fade_out: 0, keep_high_hz: 0 }

function expand(p, ov) {
  const out = []
  for (const s of p.sections || []) {
    if (!s.add && hasSound(s) && (s.stems || []).some((n) => ov.sound.has(n))) continue
    if (isPlaceSection(s) && ov.place.has(s.stems[0])) continue
    out.push(copy(s))
  }
  if ((p.final || []).length) out.push({ ...whole, stems: ['mix'], mix: true, steps: copy(p.final) })
  if ((p.master || []).length && !ov.master) out.push({ ...whole, stems: [], engine: copy(p.master), master: true })
  return out
}

// спецификации пересборки из записей черновика: выключенные не идут, пресет раскрывается на своём месте
export function draftSpecs(list) {
  const on = (list || []).filter((it) => !it.off)
  const ov = overrides(on)
  return on.flatMap((it) => (it.preset ? expand(it.preset, ov) : [specOf(it)]))
}
