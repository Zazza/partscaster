// Ход операции клиента в очереди воркера (применение пресета): шаг, номер сборки, время шага,
// оценка оставшегося — одной строкой для записи очереди.

// у этих шагов есть номер сборки (сборка N, сверка N, исправляющая сборка N)
const WITH_PASS = new Set(['render', 'verify', 'fix'])

/** Секунды → M:SS (секунды вниз до целого). */
function mmss(sec) {
  const s = Math.max(0, Math.floor(Number(sec) || 0))
  return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0')
}

/** Строка хода записи очереди: «<шаг>[ N] · M:SS[ · осталось ≈ M:SS]»; без шага — пустая строка.
 *  op — запись очереди воркера ({stage, pass, eta_s, stage_since}), t — перевод. */
export function opProgressText(op, t) {
  if (!op || !op.stage) return ''
  let head = t('queue.op.stage.' + op.stage)
  if (WITH_PASS.has(op.stage) && op.pass) head += ' ' + op.pass
  const parts = [head]
  if (op.stage_since != null) parts.push(mmss(op.stage_since))
  if (op.eta_s != null) parts.push(t('queue.op.eta', { eta: mmss(op.eta_s) }))
  return parts.join(' · ')
}
