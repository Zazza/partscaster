// Голосовые цепочки эффектов (мегафон, телефон, перегруз, слэпбэк): примочки
// «на голос» приходят из Go с флагом voice. Такой цепочке эффект почти всегда
// нужен на дорожке «голос», а не на весь микс — дорожка выбирается сама.
// Чистая логика без DOM — покрывается vitest.
import { previewWindow } from './fxPreview.js'

// Дорожка эффекта при выборе цепочки: голосовая цепочка без выбранной дорожки
// сама переключается на «голос», цепочка с ключом (ducking от барабанов) — на
// «прочее»; осознанный выбор пользователя не перекрывает.
export function voiceTarget(chain, cur) {
  if (chain && chain.voice && !cur) return 'vocals'
  if (needsStem(chain) && !cur) return 'other'
  return cur || ''
}

// Цепочка с дорожкой-ключом (key) работает только эффектом на дорожку: у
// всего микса ключа нет.
export function needsStem(chain) {
  return !!(chain && chain.key)
}

// Цепочке нужна сетка темпа (Ритм-гейт, дилей, статтер) — показать «найти сетку».
export function hasGrid(chain) {
  return !!(chain && (chain.params || []).some((p) => p.id === 'bpm'))
}

// Значения крутилок цепочки по умолчанию (у голосовых обязательна доля
// эффекта mix — сухой/обработанный).
export function chainDefaults(chain) {
  const p = {}
  if (chain) for (const prm of chain.params || []) p[prm.id] = prm.default
  return p
}

// Группы общего списка эффектов — в порядке разделов docs/effects.md; голосовые (voice)
// в общем списке не показываются: они в «Обработке голоса» шага «Голос»
export const CHAIN_GROUPS = ['space', 'pedals', 'modulation', 'filter', 'dynamics', 'time', 'lofi']

// Опции селекта эффектов по группам: заголовок группы (не выбирается) и её цепочки в
// порядке входа; цепочки без известной группы — в «прочее» последними. label(group) — подпись.
export function chainGroupOptions(chains, label) {
  const order = [...CHAIN_GROUPS, 'other']
  const by = Object.fromEntries(order.map((g) => [g, []]))
  for (const c of chains || []) {
    if (c.group === 'voice') continue
    by[CHAIN_GROUPS.includes(c.group) ? c.group : 'other'].push(c)
  }
  const out = []
  for (const g of order) {
    if (!by[g].length) continue
    // value заголовка — с префиксом, которого нет у id цепочек (id — латиница и дефис)
    out.push({ value: '#group:' + g, label: label(g), header: true, disabled: true })
    for (const c of by[g]) out.push({ value: c.id, label: c.name })
  }
  return out
}

// Эффекты блока «Обработка голоса»: сначала только голосовые, потом общие, уместные на голосе
export function voiceFxChains(chains) {
  const list = chains || []
  return [...list.filter((c) => c.voice), ...list.filter((c) => !c.voice && c.for_voice)]
}

// Начало первого спетого такта по разбору плана (bars из GET /jobs/{id}/score):
// бар голоса Vocal с нотами (voices.Vocal > 0); никто не поёт или плана нет — null
export function firstSungSec(bars) {
  let best = null
  for (const b of bars || []) {
    const n = Object.entries(b.voices || {}).find(([v]) => /^vocal$/i.test(v))
    if (!n || !(n[1] > 0)) continue
    if (best === null || b.start_sec < best) best = b.start_sec
  }
  return best
}

// Окно превью обработки голоса: выделение; без него — от первого спетого такта
// (голосовой эффект на вступлении не слышно); без плана — как в «Эффектах звука», от курсора
export function voicePreviewWindow(sel, sungAt, cursorSec, durSec) {
  if (sel && sel.to > sel.from) return previewWindow(sel, cursorSec, durSec)
  return previewWindow(null, sungAt != null ? sungAt : cursorSec, durSec)
}
