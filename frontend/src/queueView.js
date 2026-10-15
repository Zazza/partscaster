// Витрина общей очереди воркера (левый блок): генерации и долгие операции
// (пресеты, пересборка, стемы, эффекты…) одним списком в порядке исполнения —
// идущая, ждущие по месту, последние завершённые. Чистая логика без DOM.

/** Состояния, после которых запись не меняется (итог доставлен). */
export const FINAL_STATES = ['done', 'error', 'canceled', 'interrupted']

function resultOf(it) {
  switch (it.state) {
    case 'done': return { type: 'version', jobId: it.result_job_id || 0, text: '' }
    case 'error': return { type: 'error', jobId: 0, text: it.error || '' }
    case 'canceled': return { type: 'canceled', jobId: 0, text: '' }
    case 'interrupted': return { type: 'interrupted', jobId: 0, text: it.error || '' }
    default: return { type: null, jobId: 0, text: '' }
  }
}

/** Строки витрины из снимка очереди (GET /queue) и статистики воркера (процент генерации).
 *  ▲▼ — между ждущими (идущая и завершённые не считаются), ✕ — у всего незавершённого. */
export function queueRows(snapshot, stats) {
  const items = (snapshot && Array.isArray(snapshot.items)) ? snapshot.items : []
  const waiting = items.filter((it) => it.state === 'waiting').map((it) => it.id)
  const run = stats && stats.running
  return items.map((it) => {
    const w = waiting.indexOf(it.id)
    const live = it.state === 'waiting' || it.state === 'running'
    return {
      id: it.id,
      kind: it.kind,
      title: it.title,
      jobId: it.job_id || 0,
      state: it.state,
      ai: it.source === 'mcp',
      canCancel: live,
      canUp: w > 0,
      canDown: w >= 0 && w < waiting.length - 1,
      result: resultOf(it),
      pct: it.kind === 'generate' && it.state === 'running' && run && run.job_id === it.job_id &&
        run.progress_pct != null ? run.progress_pct : null,
    }
  })
}
