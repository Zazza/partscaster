// Логика витрины общей очереди (internal-shared-queue, условия 17–19; ТК19, ТК20; контракт «Фронт»):
// queueRows(snapshot, stats) -> rows[] в порядке snapshot, FINAL_STATES.
import { describe, it, expect } from 'vitest'
import { queueRows, FINAL_STATES } from './queueView.js'

// запись очереди воркера (поля json как в контракте)
const op = (over = {}) => ({
  id: 1, kind: 'preset', title: 'трек — пресет', job_id: 0, source: 'app', state: 'waiting', pos: 1,
  result_job_id: 0, error: '', cancel_requested: false, ...over,
})
const snap = (items, paused = false) => ({ paused, items })
const byId = (rows) => Object.fromEntries(rows.map((r) => [r.id, r]))

describe('FINAL_STATES', () => {
  it('завершённые состояния — done, error, canceled, interrupted', () => {
    expect([...FINAL_STATES].sort()).toEqual(['canceled', 'done', 'error', 'interrupted'])
  })
})

describe('краевые: пусто и воркер недоступен', () => {
  it('пустой snapshot — пустой список', () => {
    expect(queueRows(snap([]), null)).toEqual([])
  })
  it('null/undefined snapshot или без items — пустой список, не падает', () => {
    expect(queueRows(null, null)).toEqual([])
    expect(queueRows(undefined, undefined)).toEqual([])
    expect(queueRows({ paused: true }, {})).toEqual([])
    expect(queueRows({ paused: false, items: null }, null)).toEqual([])
  })
})

describe('строки: поля и порядок', () => {
  it('порядок строк = порядок snapshot, генерации и операции вместе (ТК19)', () => {
    const items = [
      op({ id: 5, kind: 'generate', state: 'running', job_id: 502, pos: 1 }),
      op({ id: 3, kind: 'preset', state: 'waiting', pos: 2 }),
      op({ id: 9, kind: 'generate', state: 'waiting', job_id: 503, pos: 3 }),
      op({ id: 1, kind: 'stems', state: 'done', pos: 0, result_job_id: 700 }),
    ]
    expect(queueRows(snap(items), null).map((r) => r.id)).toEqual([5, 3, 9, 1])
  })

  it('поля строки из записи: id, kind, title, jobId, state', () => {
    const [r] = queueRows(snap([op({ id: 7, kind: 'generate', title: 'Gone', job_id: 502, state: 'waiting' })]), null)
    expect(r).toMatchObject({ id: 7, kind: 'generate', title: 'Gone', jobId: 502, state: 'waiting' })
  })

  it('пометка ИИ — только у источника mcp (ТК19)', () => {
    const rows = byId(queueRows(snap([
      op({ id: 1, source: 'mcp' }), op({ id: 2, source: 'app', pos: 2 }), op({ id: 3, source: 'worker', pos: 3 }),
    ]), null))
    expect(rows[1].ai).toBe(true)
    expect(rows[2].ai).toBe(false)
    expect(rows[3].ai).toBe(false)
  })
})

describe('кнопки (ТК19, условие 18)', () => {
  const items = [
    op({ id: 10, state: 'running', pos: 1 }),
    op({ id: 11, state: 'waiting', pos: 2 }),
    op({ id: 12, state: 'waiting', pos: 3 }),
    op({ id: 13, state: 'waiting', pos: 4 }),
    op({ id: 20, state: 'done', result_job_id: 812 }),
    op({ id: 21, state: 'error', error: 'нет стемов' }),
    op({ id: 22, state: 'canceled' }),
    op({ id: 23, state: 'interrupted' }),
  ]
  const rows = byId(queueRows(snap(items), null))

  it('верхняя ждущая — только ▼ (идущая над ней не считается ждущей)', () => {
    expect(rows[11]).toMatchObject({ canUp: false, canDown: true, canCancel: true })
  })
  it('средняя ждущая — ▲ и ▼', () => {
    expect(rows[12]).toMatchObject({ canUp: true, canDown: true, canCancel: true })
  })
  it('нижняя ждущая — только ▲', () => {
    expect(rows[13]).toMatchObject({ canUp: true, canDown: false, canCancel: true })
  })
  it('идущая — только ✕', () => {
    expect(rows[10]).toMatchObject({ canUp: false, canDown: false, canCancel: true })
  })
  it('у завершённых кнопок нет', () => {
    for (const id of [20, 21, 22, 23]) {
      expect(rows[id], String(id)).toMatchObject({ canUp: false, canDown: false, canCancel: false })
    }
  })
  it('единственная ждущая — без ▲▼, но с ✕', () => {
    const [r] = queueRows(snap([op({ id: 1, state: 'waiting' })]), null)
    expect(r).toMatchObject({ canUp: false, canDown: false, canCancel: true })
  })
  it('ждущие, разделённые завершённой, всё равно соседи по ▲▼', () => {
    const r = byId(queueRows(snap([
      op({ id: 1, state: 'waiting', pos: 1 }), op({ id: 2, state: 'done', pos: 2 }), op({ id: 3, state: 'waiting', pos: 3 }),
    ]), null))
    expect(r[1]).toMatchObject({ canUp: false, canDown: true })
    expect(r[3]).toMatchObject({ canUp: true, canDown: false })
  })
})

describe('итог завершённой (ТК20, условие 19)', () => {
  const rows = byId(queueRows(snap([
    op({ id: 1, state: 'done', result_job_id: 812 }),
    op({ id: 2, state: 'error', error: 'нет стемов' }),
    op({ id: 3, state: 'canceled' }),
    op({ id: 4, state: 'interrupted' }),
    op({ id: 5, state: 'waiting' }),
    op({ id: 6, state: 'running' }),
  ]), null))

  it('готово → версия-трек', () => {
    expect(rows[1].result).toMatchObject({ type: 'version', jobId: 812 })
  })
  it('ошибка → текст', () => {
    expect(rows[2].result).toMatchObject({ type: 'error', text: 'нет стемов' })
  })
  it('отменено и прервано — свои типы', () => {
    expect(rows[3].result).toMatchObject({ type: 'canceled' })
    expect(rows[4].result).toMatchObject({ type: 'interrupted' })
  })
  it('у незавершённых итога нет', () => {
    expect(rows[5].result.type).toBeNull()
    expect(rows[6].result.type).toBeNull()
  })
})

describe('процент идущей генерации', () => {
  const running = op({ id: 5, kind: 'generate', state: 'running', job_id: 502 })

  it('из stats.running.progress_pct, если job_id совпал', () => {
    const [r] = queueRows(snap([running]), { running: { job_id: 502, progress_pct: 37 } })
    expect(r.pct).toBe(37)
  })
  it('job_id не совпал — null', () => {
    const [r] = queueRows(snap([running]), { running: { job_id: 999, progress_pct: 37 } })
    expect(r.pct).toBeNull()
  })
  it('stats без running или null — null', () => {
    expect(queueRows(snap([running]), { running: null })[0].pct).toBeNull()
    expect(queueRows(snap([running]), null)[0].pct).toBeNull()
  })
  it('у ждущей генерации процента нет, даже при совпадении job_id', () => {
    const [r] = queueRows(snap([op({ id: 7, kind: 'generate', state: 'waiting', job_id: 503 })]),
      { running: { job_id: 503, progress_pct: 10 } })
    expect(r.pct).toBeNull()
  })
})
