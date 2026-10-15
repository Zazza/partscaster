// Слежение за записью общей очереди воркера (применение пресета) — от постановки
// до итога. Колбэку приходит запись при каждой смене состояния (waiting →
// running → done/error/canceled/interrupted; у done — result_job_id версии-трека), компонент
// сам решает, что показывать. Пока есть интересующиеся, опрашиваем список очереди.
import { api } from '../api.js'
import { FINAL_STATES } from '../queueView.js'

const POLL_MS = 2000

const watchers = new Map()   // id записи → Map<колбэк, последнее состояние>
let timer = null
let polling = false          // асинхронный опрос не накладывается на интервальный

async function poll() {
  if (polling) return
  polling = true
  try {
    // пропавшими считаем только тех, за кем следили ДО запроса: запись, поставленная во время опроса,
    // в его ответ попасть не могла
    const before = new Set(watchers.keys())
    let items = null
    try { items = ((await api.queueList()) || {}).items || null } catch { /* связь моргнула — на следующем тике */ }
    if (items) {
      // запись пропала из очереди (перезапуск воркера: операции клиентов живут в его памяти) — итог «прервано»,
      // иначе окно вечно ждало бы
      const seen = new Set(items.map((it) => it.id))
      for (const [id, cbs] of watchers) {
        if (seen.has(id) || !before.has(id)) continue
        for (const cb of cbs.keys()) cb({ id, state: 'interrupted', error: '', result_job_id: 0 })
        watchers.delete(id)
      }
      for (const it of items) {
        const cbs = watchers.get(it.id)
        if (!cbs || !cbs.size) continue
        for (const [cb, last] of cbs) {
          if (last === it.state) continue        // состояние не сменилось — молчим
          cbs.set(cb, it.state)
          cb(it)
        }
        if (FINAL_STATES.includes(it.state)) watchers.delete(it.id) // итог доставлен
      }
    }
  } finally {
    polling = false
    if (!watchers.size && timer) { clearInterval(timer); timer = null }
  }
}

/** Следить за записью очереди: колбэк получает запись при каждой смене состояния,
 *  последняя — done | error | canceled | interrupted (у done — result_job_id версии-трека).
 *  Возвращает функцию прекращения слежения (компонент закрылся). */
export function watchQueueItem(id, cb) {
  if (!id) return () => {}
  let cbs = watchers.get(id)
  if (!cbs) {
    cbs = new Map()
    watchers.set(id, cbs)
  }
  cbs.set(cb, null)          // null: даже текущее состояние достойно первого звонка
  if (!timer) timer = setInterval(poll, POLL_MS)
  poll() // сразу: запись могла уже завершиться (в отчёте очереди)
  return () => {
    const s = watchers.get(id)
    if (!s) return
    s.delete(cb)
    if (!s.size) {
      watchers.delete(id)
      if (!watchers.size && timer) { clearInterval(timer); timer = null }
    }
  }
}
