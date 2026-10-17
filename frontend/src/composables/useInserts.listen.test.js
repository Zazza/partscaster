// «Пересобрать трек» берёт уже собранное прослушивание (internal-preset-verify-on-apply, условие 8; ТК10).
// applyAll(parentId, title) → api.applyEdits(parentId, specs, title, mixFile): mixFile — файл последней
// пересборки ЭТОГО трека (любой: rebuild — прослушивание, previewWith — «▶ стало», previewSpecs —
// предпрослушивание громкостей), только если она успешна и её записи совпадают с записями итога; иначе ''.
// Файл пересборки — из ответа api.rebuildSections (variant.file).
//
// Модуль — синглтон: каждый тест импортирует его заново после vi.resetModules() (как в useInserts.draft.test.js).
import { describe, it, expect, vi, afterEach } from 'vitest'

// Мок внешней границы — API воркера.
const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

const WAIT = { timeout: 3000, interval: 20 }

// записи реестра в форме, как их кладёт addStemFx: childId < 0
const fxRec = (childId, over = {}) => ({
  childId, instId: 'fx-soften', from: 0, to: 0, lead: 0, beat: 0, db: 0,
  stems: ['vocals'], fadeIn: 0, fadeOut: 0, keepHighHz: 0,
  chain: 'soften', params: { strength: 0.6 }, ...over,
})
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', from: 10, to: 20, stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })
const T = 7
const OTHER = 8
const remote = (jobId, edits, queue = [], ts = 1000) => ({ job_id: jobId, state: { v: 1, edits, queue, ts } })
const extraVol = { child_id: 0, from: 0, to: 0, stems: ['bass'], db: -2 }

// rebuildSections: n-й вызов отвечает файлом listen-n.flac; fail(n) — n-й вызов падает.
function rebuilds(fail = () => false) {
  let n = 0
  return vi.fn(async (_p, specs) => {
    n++
    if (fail(n)) throw new Error('rebuild failed')
    return {
      variant: { file: `listen-${n}.flac` },
      inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
    }
  })
}

async function load({ drafts = [], rebuildSections = rebuilds() } = {}) {
  vi.resetModules()
  globalThis.localStorage = new MemStorage()
  localStorage.setItem('yue_insert_queue', '[]')
  localStorage.setItem('yue_insert_applied', '{}')
  vi.useFakeTimers()
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => [])
  apiMock.rebuildSections = rebuildSections
  apiMock.rebuildInserts = vi.fn(async () => { throw new Error('старый контракт') })
  apiMock.drafts = vi.fn(async () => JSON.parse(JSON.stringify(drafts)))
  apiMock.draftPut = vi.fn(async () => undefined)
  apiMock.draftDelete = vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
  return ins
}

// последний вызов applyEdits: [parent, specs, title, mixFile]
const applyArgs = () => apiMock.applyEdits.mock.calls.at(-1)
// файл, которым ответил последний (успешный) вызов rebuildSections
const lastFile = async () => (await apiMock.rebuildSections.mock.results.at(-1).value).variant.file

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('applyAll передаёт файл прослушивания', () => {
  it('после успешного rebuild с теми же записями → mixFile = файл этой пересборки; записи итога = записи пересборки', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.rebuild(T)
    const file = await lastFile()
    const rebuiltSpecs = JSON.parse(JSON.stringify(apiMock.rebuildSections.mock.calls.at(-1)[1]))
    await ins.applyAll(T, 'итог')
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
    const [parent, specs, title, mixFile] = applyArgs()
    expect(Number(parent)).toBe(T)
    expect(specs).toEqual(rebuiltSpecs)
    expect(title).toBe('итог')
    expect(mixFile).toBe(file)
  })

  it('«▶ стало» (previewWith) без изменения записей, успешно → его файл', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.previewWith(T, (list) => list)
    const file = await lastFile()
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe(file)
  })

  it('предпрослушивание громкостей (previewSpecs) без добавочных записей → его файл', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    const r = await ins.previewSpecs(T, [])
    expect(apiMock.rebuildSections).toHaveBeenCalled()
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe(r.variant.file)
  })
})

describe('applyAll не передаёт файл (пусто)', () => {
  it('пересборок трека не было → mixFile пуст', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    const before = apiMock.rebuildSections.mock.calls.filter(c => Number(c[0]) === T).length
    await ins.applyAll(T, 'итог')
    expect(before).toBe(0)
    expect(applyArgs()[3]).toBe('')
  })

  it('после rebuild — «▶ стало» с другими записями (успешно) → последняя пересборка не та → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.rebuild(T)
    await ins.previewWith(T, (list) => list.filter((it) => it.childId !== -12))
    expect(apiMock.rebuildSections).toHaveBeenCalledTimes(2)
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe('')
  })

  it('после rebuild — предпрослушивание с добавочной громкостью → записи другие → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.rebuild(T)
    await ins.previewSpecs(T, [extraVol])
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe('')
  })

  it('пересборка с теми же записями упала → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rebuilds(() => true) })
    await ins.rebuild(T).catch(() => {})
    expect(apiMock.rebuildSections).toHaveBeenCalled()
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe('')
  })

  it('успешный rebuild, затем такой же rebuild упал → последняя неудачна → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rebuilds((n) => n === 2) })
    await ins.rebuild(T)
    await ins.rebuild(T).catch(() => {})
    expect(apiMock.rebuildSections).toHaveBeenCalledTimes(2)
    await ins.applyAll(T, 'итог')
    expect(applyArgs()[3]).toBe('')
  })

  it('пересборка соседнего трека с теми же записями не считается → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2]), remote(OTHER, [E1, E2])] })
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(2), WAIT)
    await ins.rebuild(OTHER)
    await ins.applyAll(T, 'итог')
    expect(Number(applyArgs()[0])).toBe(T)
    expect(applyArgs()[3]).toBe('')
  })

  it('запись выключили после прослушивания (записи итога другие) → пусто', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.rebuild(T)
    const n = apiMock.rebuildSections.mock.calls.length
    await ins.setOff(T, -12, true)
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
    // если выключение само пересобрало трек — последняя пересборка уже с новыми записями, файл законен
    const rebuiltAgain = apiMock.rebuildSections.mock.calls.length > n
    await ins.applyAll(T, 'итог')
    if (rebuiltAgain) {
      expect(applyArgs()[3]).toBe(await lastFile())
    } else {
      expect(applyArgs()[3]).toBe('')
    }
  })
})

describe('«применить всё» не вышло (api.applyEdits отклонён)', () => {
  // итог мог успеть перезаписать файл микса сборкой с поправками: прежнее прослушивание больше не «последняя пересборка»
  it('повторный applyAll без новой готовой пересборки → mixFile пуст', async () => {
    // пересборки соседнего трека держат общую очередь: пересборка T после неудачи до повторного итога не начнётся
    let release
    const hold = new Promise((r) => { release = r })
    let n = 0
    const rs = vi.fn(async (p, specs) => {
      n++
      const file = `listen-${n}.flac`
      if (Number(p) === OTHER) await hold
      return { variant: { file }, inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })) }
    })
    const ins = await load({ drafts: [remote(T, [E1, E2]), remote(OTHER, [E1, E2])], rebuildSections: rs })
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(2), WAIT)
    await ins.rebuild(T)
    const file = await lastFile()
    const busy = ins.rebuild(OTHER)
    for (let i = 0; i < 5; i++) await vi.advanceTimersByTimeAsync(0)
    apiMock.applyEdits.mockRejectedValueOnce(new Error('отменено'))
    await expect(ins.applyAll(T, 'итог')).rejects.toThrow()
    expect(apiMock.applyEdits.mock.calls[0][3]).toBe(file)
    for (let i = 0; i < 5; i++) await vi.advanceTimersByTimeAsync(0)
    // новых готовых пересборок T не было
    expect(apiMock.rebuildSections.mock.calls.filter(c => Number(c[0]) === T).length).toBe(1)
    await ins.applyAll(T, 'итог-2')
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(2)
    expect(applyArgs()[3]).toBe('')
    release()
    await busy
  })

  it('после неудачи прослушивание пересобирается записями черновика, и следующий applyAll берёт её файл', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    apiMock.applyEdits.mockRejectedValueOnce(new Error('rebuild failed on worker'))
    await expect(ins.applyAll(T, 'итог')).rejects.toThrow()
    const draftSpecs = JSON.parse(JSON.stringify(apiMock.applyEdits.mock.calls[0][1]))
    await vi.waitFor(() => expect(apiMock.rebuildSections.mock.calls.some(c => Number(c[0]) === T)).toBe(true), WAIT)
    const call = apiMock.rebuildSections.mock.calls.at(-1)
    expect(Number(call[0])).toBe(T)
    expect(call[1]).toEqual(draftSpecs)
    const file = await lastFile()
    for (let i = 0; i < 5; i++) await vi.advanceTimersByTimeAsync(0)
    await ins.applyAll(T, 'итог')
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(2)
    expect(applyArgs()[1]).toEqual(draftSpecs)
    expect(applyArgs()[3]).toBe(file)
  })
})
