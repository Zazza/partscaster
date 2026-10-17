// «Применить» пресета звука в черновик — useInserts.preparePreset (карточка internal-studio-preset-draft,
// условие 21; ТК14). Модуль — синглтон: каждый тест импортирует его заново после vi.resetModules()
// (как в useInserts.draft.test.js).
//
// preparePreset(parentId, run, info): run() — запрос PresetDraft (ответ {sections, final, master, warnings}),
// info — {id, name} пресета. Пока run() идёт — isPresetPreparing(parentId) = true, «применить всё» и второй
// preparePreset того же трека отклоняются; готово — запись-пресет в черновике (как withPreset: прежний пресет
// убран, новый первым), флаг снят; run() упал — черновик не тронут, флаг снят, ошибка наружу. Во время
// «применить всё» preparePreset отклоняется.
//
// Толкование: «отклоняется ошибкой» — вызов бросает или возвращает отклонённый промис (проверяется оба);
// отклонённый preparePreset не зовёт свой run().
import { describe, it, expect, vi, afterEach } from 'vitest'

// Мок внешней границы — API воркера. Объект общий, поля подменяются в каждом тесте.
const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

function report(specs) {
  return {
    variant: { file: 'out.wav' },
    inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
  }
}

const base = { from: 0, to: 0, lead: 0, beat: 0, db: 0, fadeIn: 0, fadeOut: 0, keepHighHz: 0 }
const fxRec = (childId, over = {}) => ({ ...base, childId, instId: 'fx-soften', stems: ['vocals'],
  chain: 'soften', params: { strength: 0.6 }, ...over })
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })

// ответ PresetDraft
const sec = (stems, extra = {}) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems,
  fade_in: 0, fade_out: 0, keep_high_hz: 0, ...extra })
const RESULT = {
  sections: [sec(['vocals'], { chain: 'eq', params: { high: -12 } }), sec(['bass'], { db: -2 })],
  final: [{ chain: 'width', params: { width: 1.1 } }],
  master: [{ type: 'limiter', target_lufs: -13 }],
  warnings: ['гитарист: у трека нет плана'],
}
const INFO = { id: 71, name: 'Пост-панк' }
// прежний пресет в черновике (как его пишет presetRecord)
const OLD_PRESET = { ...base, childId: -50, instId: 'preset', stems: [], step: 'done', label: 'Старый',
  preset: { id: 5, name: 'Старый', sections: [sec(['other'], { db: -3 })], final: [], master: [], warnings: [] } }

const T = 7

async function load({ drafts = [], applyEdits } = {}) {
  vi.resetModules()
  globalThis.localStorage = new MemStorage()
  localStorage.setItem('yue_insert_queue', '[]')
  localStorage.setItem('yue_insert_applied', '{}')
  vi.useFakeTimers()
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => [])
  apiMock.rebuildSections = vi.fn(async (_p, specs) => report(specs))
  apiMock.rebuildInserts = vi.fn(async (_p, specs) => report(specs))
  apiMock.drafts = vi.fn(async () => JSON.parse(JSON.stringify(drafts)))
  apiMock.draftPut = vi.fn(async () => undefined)
  apiMock.draftDelete = vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = applyEdits || vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  return ins
}

const WAIT = { timeout: 3000, interval: 20 }
const remote = (jobId, edits) => ({ job_id: jobId, state: { v: 1, edits, queue: [], ts: 1000 } })
const clone = (x) => JSON.parse(JSON.stringify(x))
const attempt = (fn) => Promise.resolve().then(fn)
const flush = async () => { for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0) }

// run(), который отвечает, когда тест скажет
function deferredRun() {
  let resolve, reject
  const run = vi.fn(() => new Promise((res, rej) => { resolve = res; reject = rej }))
  return { run, resolve: (v) => resolve(v), reject: (e) => reject(e) }
}

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('ТК14: preparePreset — пока готовится', () => {
  it('пока run() идёт — isPresetPreparing = true; до и после — false', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(1), WAIT)
    expect(ins.isPresetPreparing(T)).toBe(false)

    const d = deferredRun()
    const p = ins.preparePreset(T, d.run, INFO)
    await vi.waitFor(() => expect(d.run).toHaveBeenCalled(), WAIT)
    expect(ins.isPresetPreparing(T)).toBe(true)

    d.resolve(clone(RESULT))
    await p
    expect(ins.isPresetPreparing(T)).toBe(false)
  })

  it('пока run() идёт — applyAll того же трека отклоняется, applyEdits не зовётся', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(1), WAIT)
    const d = deferredRun()
    const p = ins.preparePreset(T, d.run, INFO)
    await vi.waitFor(() => expect(d.run).toHaveBeenCalled(), WAIT)

    await expect(attempt(() => ins.applyAll(T, 'итог'))).rejects.toThrow()
    await flush()
    expect(apiMock.applyEdits).not.toHaveBeenCalled()

    d.resolve(clone(RESULT))
    await p
  })

  it('пока run() идёт — второй preparePreset того же трека отклоняется, его run() не зовётся', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(1), WAIT)
    const d = deferredRun()
    const p = ins.preparePreset(T, d.run, INFO)
    await vi.waitFor(() => expect(d.run).toHaveBeenCalled(), WAIT)

    const second = vi.fn(async () => clone(RESULT))
    await expect(attempt(() => ins.preparePreset(T, second, { id: 72, name: 'Другой' }))).rejects.toThrow()
    expect(second).not.toHaveBeenCalled()
    expect(ins.isPresetPreparing(T)).toBe(true)

    d.resolve(clone(RESULT))
    await p
    const presets = ins.appliedFor(T).filter(x => x.preset)
    expect(presets.length).toBe(1)
    expect(presets[0].preset.id).toBe(71)
  })

  it('после завершения applyAll снова проходит', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(1), WAIT)
    await ins.preparePreset(T, vi.fn(async () => clone(RESULT)), INFO)
    await expect(attempt(() => ins.applyAll(T, 'итог'))).resolves.toBe(501)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
  })
})

describe('ТК14: preparePreset — готово', () => {
  it('запись-пресет первой (instId preset, данные ответа), прочие записи в прежнем порядке, флаг снят', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    const before = clone(ins.appliedFor(T))

    await ins.preparePreset(T, vi.fn(async () => clone(RESULT)), INFO)

    const list = ins.appliedFor(T)
    expect(list.length).toBe(3)
    const rec = list[0]
    expect(rec.instId).toBe('preset')
    expect(rec.label).toBe('Пост-панк')
    expect(rec.step).toBe('done')
    expect(rec.preset).toEqual({ id: 71, name: 'Пост-панк', ...clone(RESULT) })
    expect(clone(list.slice(1))).toEqual(before)
    expect(ins.isPresetPreparing(T)).toBe(false)
  })

  it('прежний пресет убран, новый — первым, остальные в порядке', async () => {
    const ins = await load({ drafts: [remote(T, [E1, OLD_PRESET, E2])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(3), WAIT)
    const others = clone(ins.appliedFor(T).filter(x => !x.preset))

    await ins.preparePreset(T, vi.fn(async () => clone(RESULT)), INFO)

    const list = ins.appliedFor(T)
    const presets = list.filter(x => x.preset)
    expect(presets.length).toBe(1)
    expect(list[0].preset.id).toBe(71)
    expect(clone(list.slice(1))).toEqual(others)
  })

  it('пустые final/master/warnings в ответе — в записи []', async () => {
    const ins = await load({ drafts: [remote(T, [])] })
    await ins.preparePreset(T, vi.fn(async () => ({ sections: RESULT.sections })), INFO)
    const rec = ins.appliedFor(T)[0]
    expect(rec.preset.final).toEqual([])
    expect(rec.preset.master).toEqual([])
    expect(rec.preset.warnings).toEqual([])
  })
})

describe('ТК14: preparePreset — run() упал', () => {
  it('ошибка наружу, черновик не тронут, флаг снят; затем можно снова', async () => {
    const ins = await load({ drafts: [remote(T, [E1, OLD_PRESET])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    const before = clone(ins.appliedFor(T))
    const putsBefore = apiMock.draftPut.mock.calls.length

    const err = new Error('воркер: сверка упала')
    await expect(ins.preparePreset(T, vi.fn(async () => { throw err }), INFO)).rejects.toBe(err)

    expect(clone(ins.appliedFor(T))).toEqual(before)
    expect(ins.isPresetPreparing(T)).toBe(false)
    await flush()
    expect(apiMock.draftPut.mock.calls.length).toBe(putsBefore)

    // флаг снят — следующая попытка и «применить всё» проходят
    await ins.preparePreset(T, vi.fn(async () => clone(RESULT)), INFO)
    expect(ins.appliedFor(T)[0].preset.id).toBe(71)
    await expect(attempt(() => ins.applyAll(T, 'итог'))).resolves.toBe(501)
  })
})

describe('ТК14: preparePreset во время «применить всё»', () => {
  it('отклоняется, run() не зовётся, черновик не тронут', async () => {
    let finish
    const applyEdits = vi.fn(() => new Promise((resolve) => { finish = resolve }))
    const ins = await load({ drafts: [remote(T, [E1])], applyEdits })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(1), WAIT)
    const before = clone(ins.appliedFor(T))

    const applying = ins.applyAll(T, 'итог')
    await vi.waitFor(() => expect(applyEdits).toHaveBeenCalled(), WAIT)

    const run = vi.fn(async () => clone(RESULT))
    await expect(attempt(() => ins.preparePreset(T, run, INFO))).rejects.toThrow()
    expect(run).not.toHaveBeenCalled()
    expect(clone(ins.appliedFor(T))).toEqual(before)
    expect(ins.isPresetPreparing(T)).toBe(false)

    finish(501)
    await expect(applying).resolves.toBe(501)
  })
})
