// «Готово» студии: версии трека одним списком — понятное короткое название вместо имени файла, полная подпись — в
// подсказке, новые сверху. Чистая логика без DOM.
import { useI18n } from './i18n/index.js'

const { t } = useI18n()

const PARTS = (label) => String(label || '').split(' + ').map((s) => s.trim()).filter(Boolean)

/** Название версии: {title, detail, kind}. chainName(id) — имя DSP-цепочки (нет — undefined). */
export function resultTitle(v, chainName = () => undefined) {
  const file = String((v && v.file) || '')
  const label = String((v && v.label) || '').trim()
  const detail = label
  let m
  if (/^dsp-preset-\d+-mix\.flac$/.test(file)) {
    return { title: t('results.presetPremix'), detail, kind: 'premix' }
  }
  if (/^dsp-preset-\d+\.flac$/.test(file)) {
    return { title: label ? t('results.preset', { name: label }) : t('results.presetNone'), detail, kind: 'preset' }
  }
  if (/^overdub-inst-/.test(file)) {
    const n = PARTS(label).length
    return { title: n ? t('results.mix') + ' · ' + n + ' ' + plural(n) : t('results.mix'), detail, kind: 'mix' }
  }
  if (/^overdub-\d+\.flac$/.test(file)) {
    return { title: label || t('results.layer'), detail, kind: 'layer' }
  }
  if (/^overdub-premaster-/.test(file)) {
    return { title: t('results.premix'), detail, kind: 'premix' }
  }
  if (/^dsp-fx-/.test(file)) {
    return { title: label || t('results.engine'), detail, kind: 'engine' }
  }
  if ((m = /^dsp-preview-(.+)\.flac$/.exec(file))) {
    return { title: `${chainName(m[1]) || label || m[1]} (${t('results.preview15')})`, detail, kind: 'effect' }
  }
  if ((m = /^dsp-(.+)\.flac$/.exec(file))) {
    return { title: chainName(m[1]) || label || m[1], detail, kind: 'effect' }
  }
  return { title: label || file, detail, kind: 'other' }
}

function plural(n) {
  const a = n % 10
  const b = n % 100
  if (a === 1 && b !== 11) return t('results.edits.one')
  if (a >= 2 && a <= 4 && (b < 12 || b > 14)) return t('results.edits.few')
  return t('results.edits.many')
}

/** Новые сверху: по created_at (ISO) убыв.; без даты — в конце, порядок среди них прежний. */
export function resultsOrder(list) {
  return (list || []).map((v, i) => ({ v, i })).sort((a, b) => {
    const x = a.v.created_at || ''
    const y = b.v.created_at || ''
    if (!x && !y) return a.i - b.i
    if (!x) return 1
    if (!y) return -1
    return x < y ? 1 : x > y ? -1 : a.i - b.i
  }).map((e) => e.v)
}

/** Дата версии «10.10 14:00» (год — если не текущий). */
export function resultDate(iso, now = new Date()) {
  const m = /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/.exec(String(iso || ''))
  if (!m) return ''
  return `${m[3]}.${m[2]}${Number(m[1]) !== now.getFullYear() ? '.' + m[1] : ''} ${m[4]}:${m[5]}`
}

/** С какой секунды играть готовый «кусок с правками»: за lead с до начала правки (окно — запомненное from или
 *  cont_from джобы); начало раньше lead или окна нет — с 0. */
export function fragmentStart(fj, lead = 4) {
  const from = Number((fj && (fj.from ?? fj.cont_from)) || 0)
  return from > lead ? from - lead : 0
}

/** Сыграть готовый кусок: его трек, затем перемотка на fragmentStart (за lead с до правки; раньше — с начала). */
export async function playFragment(api, fj, lead = 4) {
  await api.playAudio(fj.id)
  const at = fragmentStart(fj, lead)
  if (at > 0) await api.seekAudio(at)
}
