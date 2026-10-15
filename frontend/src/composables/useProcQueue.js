// Слежение за очередью обработки (Go): запись применения пресета — от постановки
// до итога. Колбэку приходит запись при каждой смене состояния (waiting →
// running → done/error/canceled; у done — result_job_id версии-трека), компонент
// сам решает, что показывать. Пока есть интересующиеся, опрашиваем список очереди.
import { api } from '../api.js'

const POLL_MS = 2000
const FINAL = ['done', 'error', 'canceled']

const watchers = new Map()   // id записи → Map<колбэк, последнее состояние>
let timer = null
let polling = false          // асинхронный опрос не накладывается на интервальный

async function poll() {
  if (polling) return
  polling = true
  try {
    let items = null
    try { items = ((await api.queueList()) || {}).items || null } catch { /* связь моргнула — на следующем тике */ }
    if (items) {
      for (const it of items) {
        const cbs = watchers.get(it.id)
        if (!cbs || !cbs.size) continue
        for (const [cb, last] of cbs) {
          if (last === it.state) continue        // состояние не сменилось — молчим
          cbs.set(cb, it.state)
          cb(it)
        }
        if (FINAL.includes(it.state)) watchers.delete(it.id) // итог доставлен
      }
    }
  } finally {
    polling = false
    if (!watchers.size && timer) { clearInterval(timer); timer = null }
  }
}

/** Следить за записью очереди: колбэк получает запись при каждой смене состояния,
 *  последняя — done | error | canceled (у done — result_job_id версии-трека).
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
