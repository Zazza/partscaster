// Реестр «Правки трека» (useInserts): записи «на весь трек» — карточка internal-studio-draft2,
// условие 2, тест-кейс ТК5. addMixFx(parent, {chain, params, step?, label?}), addMixPedals(parent,
// {steps, label}), setMixEnvelope(parent, envelope) — одна линия на весь трек, повторная заменяет,
// пустая убирает. Записи: childId < 0, mix: true, stems ['mix']; в api (specOf) уходит mix: true.
// editStep: mix → 'sound', если нет явного step.
// Модуль — синглтон: каждый тест импортирует его заново после vi.resetModules() (как в useInserts.test.js).
import { describe, it, expect, vi, afterEach } from 'vitest'
import { editStep } from '../draftEdits.js'

// Мок внешней границы — API воркера. Объект общий, поля подменяются в каждом тесте.
const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

// node-окружение vitest без DOM: минимальный localStorage
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

async function flush() {
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
}

const WAIT = { timeout: 3000, interval: 20 }

// обычная вклейка рендера — чтобы пересборке было из чего собирать и после удаления линии
const A = { childId: 1, instId: 'i-a', from: 0, to: 10, lead: 0, beat: 0.5, db: -6 }

async function load({ applied = {} } = {}) {
  vi.resetModules()
  globalThis.localStorage = new MemStorage()
  localStorage.setItem('yue_insert_queue', JSON.stringify([]))
  localStorage.setItem('yue_insert_applied', JSON.stringify(applied))
  vi.useFakeTimers()
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => [])
  apiMock.rebuildSections = vi.fn(async (_p, specs) => report(specs))
  apiMock.rebuildInserts = vi.fn(async (_p, specs) => report(specs))
  apiMock.drafts = vi.fn(async () => [])
  apiMock.draftPut = vi.fn(async () => undefined)
  apiMock.draftDelete = vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  current = ins
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  await flush()
  return ins
}
// «▶ микс» (internal-studio-instant-apply, условие 6): запись правки сама не собирает — сборка черновика
// трека идёт через ensureMix; ответ — спецификации последней пересборки.
let current = null
async function built(id = 'P') {
  await current.ensureMix(id)
  await flush()
  return apiMock.rebuildSections.mock.calls.at(-1)[1]
}


const lastCall = () => built()
const mixRecs = (ins, id = 'P') => ins.appliedFor(id).filter(x => x.mix === true)
const mixSpecs = specs => specs.filter(s => s.mix === true)

const levelFx = (over = {}) => ({ chain: 'level', params: { gain: -3, ceiling: -1 }, ...over })
const pedals = [{ chain: 'od-ts', params: { drive: 12 } }, { chain: 'reverb-hall', params: { wet: 0.3 } }]
const env1 = [{ t: 1, db: 0 }, { t: 3, db: -12 }]
const env2 = [{ t: 2, db: -6 }]

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('addMixFx — эффект на весь трек', () => {
  it('запись: childId < 0, mix: true, stems [mix], chain/params; вклейки на месте; без пересборки до «▶ микс»', async () => {
    const ins = await load({ applied: { P: [A] } })
    await ins.addMixFx('P', levelFx())
    await flush()
    const recs = mixRecs(ins)
    expect(recs.length).toBe(1)
    expect(recs[0].childId).toBeLessThan(0)
    expect(recs[0]).toMatchObject({ mix: true, stems: ['mix'], chain: 'level', params: { gain: -3, ceiling: -1 } })
    expect(ins.appliedFor('P').some(x => x.childId === 1)).toBe(true)
    expect(apiMock.rebuildSections).not.toHaveBeenCalled()
    await built()
    expect(apiMock.rebuildSections).toHaveBeenCalledTimes(1)
    expect(apiMock.rebuildSections.mock.calls.at(-1)[0]).toBe('P')
  })

  it('в api уходит mix: true, child_id 0, stems [mix], chain и params', async () => {
    const ins = await load({})
    await ins.addMixFx('P', levelFx())
    await flush()
    const specs = mixSpecs((await lastCall()))
    expect(specs.length).toBe(1)
    expect(specs[0]).toMatchObject({ mix: true, child_id: 0, stems: ['mix'], chain: 'level' })
    expect(specs[0].params).toEqual({ gain: -3, ceiling: -1 })
  })

  it('обычные записи в api — без mix: true', async () => {
    const ins = await load({ applied: { P: [A] } })
    await ins.addMixFx('P', levelFx())
    await flush()
    const other = (await lastCall()).filter(s => s.child_id === 1)
    expect(other.length).toBe(1)
    expect(other[0].mix === true).toBe(false)
  })

  it("step из аргумента ('done') — у записи; подпись label — у записи", async () => {
    const ins = await load({})
    await ins.addMixFx('P', levelFx({ chain: 'master', params: { amount: 0.5 }, step: 'done', label: 'Мастеринг (мягко)' }))
    await flush()
    const rec = mixRecs(ins)[0]
    expect(rec.step).toBe('done')
    expect(rec.label).toBe('Мастеринг (мягко)')
    expect(editStep(rec)).toBe('done')
  })

  it("без step — шаг записи 'sound'", async () => {
    const ins = await load({})
    await ins.addMixFx('P', levelFx())
    await flush()
    expect(editStep(mixRecs(ins)[0])).toBe('sound')
  })

  it('два эффекта подряд — две записи, childId уникальны и отрицательны, оба в api по порядку', async () => {
    const ins = await load({})
    await ins.addMixFx('P', levelFx())
    await ins.addMixFx('P', levelFx({ chain: 'breathe', params: { amount: 0.4 } }))
    await flush()
    const recs = mixRecs(ins)
    expect(recs.length).toBe(2)
    expect(new Set(recs.map(x => x.childId)).size).toBe(2)
    expect(recs.every(x => x.childId < 0)).toBe(true)
    expect(mixSpecs((await lastCall())).map(s => s.chain)).toEqual(['level', 'breathe'])
  })

  it('соседний трек не трогается', async () => {
    const ins = await load({ applied: { Q: [A] } })
    await ins.addMixFx('P', levelFx())
    await flush()
    expect(mixRecs(ins, 'Q').length).toBe(0)
    expect(ins.appliedFor('Q').length).toBe(1)
  })
})

describe('addMixPedals — педали на весь трек', () => {
  it('запись: childId < 0, mix: true, stems [mix], steps, label; в api — mix: true и steps по порядку', async () => {
    const ins = await load({})
    await ins.addMixPedals('P', { steps: pedals, label: 'Овердрайв → Реверб' })
    await flush()
    const recs = mixRecs(ins)
    expect(recs.length).toBe(1)
    expect(recs[0].childId).toBeLessThan(0)
    expect(recs[0]).toMatchObject({ mix: true, stems: ['mix'], label: 'Овердрайв → Реверб' })
    expect(recs[0].steps).toEqual(pedals)
    expect(editStep(recs[0])).toBe('sound')
    const specs = mixSpecs((await lastCall()))
    expect(specs.length).toBe(1)
    expect(specs[0]).toMatchObject({ mix: true, child_id: 0, stems: ['mix'] })
    expect(specs[0].steps).toEqual(pedals)
  })
})

describe('setMixEnvelope — одна линия громкости на весь трек', () => {
  const envSpecs = specs => mixSpecs(specs).filter(s => Array.isArray(s.envelope) && s.envelope.length)

  it('запись: childId < 0, mix: true, stems [mix], envelope; в api — mix: true и линия как передана', async () => {
    const ins = await load({})
    await ins.setMixEnvelope('P', env1)
    await flush()
    const recs = mixRecs(ins)
    expect(recs.length).toBe(1)
    expect(recs[0].childId).toBeLessThan(0)
    expect(recs[0]).toMatchObject({ mix: true, stems: ['mix'] })
    expect(recs[0].envelope).toEqual(env1)
    expect(editStep(recs[0])).toBe('sound')
    const specs = envSpecs((await lastCall()))
    expect(specs.length).toBe(1)
    expect(specs[0]).toMatchObject({ mix: true, child_id: 0, stems: ['mix'] })
    expect(specs[0].envelope).toEqual(env1)
  })

  it('повторная заменяет прежнюю: одна запись, в api новая линия', async () => {
    const ins = await load({})
    await ins.setMixEnvelope('P', env1)
    await flush()
    await ins.setMixEnvelope('P', env2)
    await flush()
    const recs = mixRecs(ins).filter(x => x.envelope)
    expect(recs.length).toBe(1)
    expect(recs[0].envelope).toEqual(env2)
    const specs = envSpecs((await lastCall()))
    expect(specs.length).toBe(1)
    expect(specs[0].envelope).toEqual(env2)
  })

  it('пустая убирает линию; остальные записи на месте', async () => {
    const ins = await load({ applied: { P: [A] } })
    await ins.addMixFx('P', levelFx())
    await ins.setMixEnvelope('P', env1)
    await flush()
    await ins.setMixEnvelope('P', [])
    await flush()
    expect(mixRecs(ins).filter(x => x.envelope && x.envelope.length).length).toBe(0)
    expect(mixRecs(ins).filter(x => x.chain === 'level').length).toBe(1)
    expect(ins.appliedFor('P').some(x => x.childId === 1)).toBe(true)
    expect(envSpecs((await lastCall())).length).toBe(0)
  })

  it('пустая при отсутствии линии — записей не добавляет', async () => {
    const ins = await load({ applied: { P: [A] } })
    await ins.setMixEnvelope('P', [])
    await flush()
    expect(mixRecs(ins).length).toBe(0)
    expect(ins.appliedFor('P').length).toBe(1)
  })

  it('линия на весь трек не заменяет линию дорожки и наоборот', async () => {
    const ins = await load({})
    await ins.addStemEnvelope('P', { stem: 'vocals', envelope: env1 })
    await flush()
    await ins.setMixEnvelope('P', env2)
    await flush()
    const call = (await lastCall())
    const stemEnv = call.filter(s => s.mix !== true && Array.isArray(s.envelope) && s.envelope.length)
    expect(stemEnv.length).toBe(1)
    expect(stemEnv[0].stems).toEqual(['vocals'])
    expect(envSpecs(call).length).toBe(1)
    expect(envSpecs(call)[0].envelope).toEqual(env2)
  })
})

describe('editStep для записей на весь трек', () => {
  it("mix без явного step → 'sound' (даже без chain/steps/envelope)", () => {
    expect(editStep({ childId: -5, mix: true, stems: ['mix'] })).toBe('sound')
    expect(editStep({ childId: -5, mix: true, stems: ['mix'], chain: 'level' })).toBe('sound')
  })

  it('явный step побеждает', () => {
    expect(editStep({ childId: -5, mix: true, stems: ['mix'], chain: 'master', step: 'done' })).toBe('done')
  })
})
