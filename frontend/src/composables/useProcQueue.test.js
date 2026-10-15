// Тесты слежения за очередью обработки (useProcQueue): колбэк получает запись
// при каждой смене состояния, итог (done) — с result_job_id версии, отмена
// слежения прекращает опрос. Модуль — синглтон: каждый тест импортирует заново.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

async function load() {
  vi.resetModules()
  const mod = await import('./useProcQueue.js')
  return mod
}

beforeEach(() => { vi.useFakeTimers() })
afterEach(() => { vi.useRealTimers() })

describe('watchQueueItem — слежение до итога', () => {
  it('готовая запись (done) приходит колбэку сразу с итогом — версия-трек при ней', async () => {
    apiMock.queueList = vi.fn(async () => ({ items: [{ id: 5, state: 'done', result_job_id: 544 }] }))
    const { watchQueueItem } = await load()
    const got = new Promise((res) => watchQueueItem(5, res))
    await expect(got).resolves.toMatchObject({ id: 5, state: 'done', result_job_id: 544 })
  })

  it('смены состояния приходят по очереди: running, затем done', async () => {
    let state = 'running'
    apiMock.queueList = vi.fn(async () => ({ items: [{ id: 9, state }] }))
    const { watchQueueItem } = await load()
    const seen = []
    watchQueueItem(9, (it) => seen.push(it.state))
    await vi.advanceTimersByTimeAsync(100)   // первый опрос — running
    state = 'done'
    await vi.advanceTimersByTimeAsync(2500)
    expect(seen).toEqual(['running', 'done'])
  })

  it('повторные опросы без смены состояния колбэк не дёргают', async () => {
    apiMock.queueList = vi.fn(async () => ({ items: [{ id: 9, state: 'running' }] }))
    const { watchQueueItem } = await load()
    const cb = vi.fn()
    watchQueueItem(9, cb)
    await vi.advanceTimersByTimeAsync(7000)
    expect(cb).toHaveBeenCalledTimes(1)      // только первый звонок
    expect(cb).toHaveBeenCalledWith(expect.objectContaining({ state: 'running' }))
  })

  it('ошибка записи — колбэку запись с текстом ошибки', async () => {
    apiMock.queueList = vi.fn(async () => ({ items: [{ id: 3, state: 'error', err: 'воркер недоступен' }] }))
    const { watchQueueItem } = await load()
    const got = new Promise((res) => watchQueueItem(3, res))
    await expect(got).resolves.toMatchObject({ id: 3, state: 'error', err: 'воркер недоступен' })
  })

  it('отмена слежения: итог не приходит, опрос останавливается', async () => {
    let state = 'waiting'
    const queueList = vi.fn(async () => ({ items: [{ id: 4, state }] }))
    apiMock.queueList = queueList
    const { watchQueueItem } = await load()
    const cb = vi.fn()
    const un = watchQueueItem(4, cb)
    await vi.advanceTimersByTimeAsync(3000)
    expect(cb).toHaveBeenCalledTimes(1)      // первый звонок «waiting» был
    un()
    state = 'done'
    await vi.advanceTimersByTimeAsync(5000)
    expect(cb).toHaveBeenCalledTimes(1)
    const calls = queueList.mock.calls.length
    await vi.advanceTimersByTimeAsync(5000)
    expect(queueList.mock.calls.length).toBe(calls)   // таймер снят — очередь не опрашивается
  })

  it('сбой связи не роняет слежение: следующий опрос доносит итог', async () => {
    let fail = true
    apiMock.queueList = vi.fn(async () => {
      if (fail) throw new Error('mcp timeout')
      return { items: [{ id: 6, state: 'done', result_job_id: 100 }] }
    })
    const { watchQueueItem } = await load()
    const got = new Promise((res) => watchQueueItem(6, res))
    await vi.advanceTimersByTimeAsync(2500)
    fail = false
    await expect(vi.advanceTimersByTimeAsync(2500).then(() => got)).resolves.toMatchObject({ id: 6 })
  })
})
