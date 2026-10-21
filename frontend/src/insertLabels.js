// Понятные подписи результатов вклеек (файл микса overdub-inst-<N>.flac назван по номеру рендера куска N).
// Подпись берём из реестра вклеек (инструмент + окно), для старых файлов вне
// реестра — из названия рендера («… · drumfill» / «… · сбивка»).
import { TRICK_INSTRUMENTS } from './abcEdit.js'
import { isPlaceRecord, placeLabel } from './mixDesk.js'
import { useI18n } from './i18n/index.js'
import { localizeWarning, pedalPresetName } from './dspI18n.js'

const { t, locale } = useI18n()

// роли производных треков: результаты слушают, материал — сырьё для вклеек
export const RESULT_ROLES = ['variant', 'rebuild', 'continue', 'voice']

export function isResultJob(j) {
  return RESULT_ROLES.includes(j.role)
}

// id инструмента по названию рендера куска: хвост после « · » — id или подпись
export function instIdFromTitle(title, labelOf) {
  for (const part of String(title || '').split(' · ').slice(1).map((x) => x.trim())) {
    const hit = TRICK_INSTRUMENTS.find((i) => i.id === part || labelOf(i.id) === part)
    if (hit) return hit.id
  }
  return null
}

// номер рендера из имени файла микса; не вклейка — null
export function mixChildId(file) {
  const m = String(file || '').match(/^overdub-inst-(\d+)\.flac$/)
  return m ? Number(m[1]) : null
}

// подпись микса: «сбивка 3:24–3:28 + электрогитара 4:00–4:16».
// applied — вклейки родителя из реестра; jobs — список треков (для старых файлов)
export function mixLabel(file, { applied = [], jobs = [], labelOf, fmt }) {
  const id = mixChildId(file)
  if (id == null) return null
  const inReg = applied.some((it) => it.childId === id || (it.alts || []).includes(id))
  if (inReg) {
    return applied.map((it) => `${labelOf(it.instId)} ${fmt(it.from)}–${fmt(it.to)}`).join(' + ')
  }
  const job = jobs.find((j) => j.id === id)
  const inst = job && instIdFromTitle(job.title, labelOf)
  return inst ? labelOf(inst) : null
}

// строка реестра вклеек: у эффекта на дорожку — «эффект · дорожка», у вклейки/
// заглушки — название приёма
export function insertTitle(it, { chainName, stemName, instName, placeName }) {
  // пресет звука — одной строкой: название и пометки (гитарист пропущен…)
  if (it.preset) return [t('insert.preset', { name: it.preset.name }), ...(it.preset.warnings || []).map((w) => localizeWarning(w, locale.value))].join(' · ')
  // мастер — на весь микс; место дорожки — «место 30 % вправо · гитары»
  if (it.master && it.engine) return it.label || t('insert.master', { name: it.engine.map((b) => b.type).join(' → ') })
  if (isPlaceRecord(it)) return `${placeName ? placeName(it.place) : t('mix.place') + ' ' + placeLabel(it.place)} · ${stemName(it.stems[0])}`
  // эффект: своя подпись записи («один клик» с уровнем) или название цепочки
  if (it.chain) {
    // «один клик» — название цепочки на языке показа и уровень; старые записи — своя подпись
    const fxName = it.oneClick ? `${chainName(it.chain)} (${t('studio.dsp.level.' + it.oneClick)})` : it.label || chainName(it.chain)
    return `${fxName} · ${(it.stems || []).map(stemName).join(', ')}`
  }
  // линия громкости — на дорожку или на весь трек
  if (it.envelope) return `${t('insert.envelope')} · ${(it.stems || []).map(stemName).join(', ')}`
  // цепочка звукового движка: подпись записи или типы блоков по порядку
  if (it.engine) {
    const what = it.label || t('engine.label', { name: it.engine.map((b) => b.type).join(' → ') })
    return `${what} · ${(it.stems || []).map(stemName).join(', ')}`
  }
  // доска педалей: подпись набора/доски или названия педалей по порядку
  if (it.steps) {
    const what = (it.pedalPreset && pedalPresetName(it.pedalPreset, locale.value)) || it.label ||
      it.steps.filter((s) => !s.off).map((s) => chainName(s.chain)).join(' → ')
    return `${what} · ${(it.stems || []).map(stemName).join(', ')}`
  }
  // «убрать» из пульта «Дорожки» — какая дорожка
  if (it.instId === 'remove') return `${t('insert.remove')} · ${(it.stems || []).map(stemName).join(', ')}`
  return instName(it.instId)
}

// окно записи: to ≤ 0 — «до конца», весь трек — отдельной подписью
export function insertWindow(it, { fmt, toEnd, whole }) {
  if (!(it.to > 0)) return it.from > 0 ? `${fmt(it.from)}–${toEnd}` : whole
  return `${fmt(it.from)}–${fmt(it.to)}`
}
