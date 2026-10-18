// Уточнения контракта после ревью (internal-studio-instant-apply, условия 6, 8, 9, 10): свежесть внутри очереди
// сборок, ensureMix во время «применить всё», currentMix (файл «было»), warnings только у собранного пресета,
// ошибка с отметкой {edit:id} наружу — без отметки. Общая часть мока и записей — как в useInserts.instant.test.js.
//
// Контракт useInserts:
//   - функции записи (addStemFx, addStemPedals, addStemEngine, addStemEngines, addStemEnvelope, addMixFx,
//     addMixPedals, setMixEnvelope, setStemPlace, setPlace, setMaster, addMute, addMutes, setDb, setOff, remove,
//     selectAlt, replaceEngine, putPreset(parentId, preset, info)) пишут черновик, api.rebuildSections не зовут;
//   - ensureMix(parentId): записи черновика отличаются от последней удачной сборки — одна api.rebuildSections,
//     ответ — её результат; совпадают — без вызова, ответ {cached: true, file}; mixStale(parentId) → boolean;
//   - ошибка api.rebuildSections с отметкой {edit:<id>} → у записи с этим id (String(childId)) поле error = текст
//     без отметки; удачная сборка снимает error у всех записей трека; warnings записи-пресета = r.warnings
//     удачной сборки;
//   - у каждой спецификации сборки id: String(childId).
//
// Мок — только внешняя граница api.js. Любой метод api, кроме заданных тестом, записывается как «лишний
// запрос к воркеру» (ответ — undefined). Пересборка в тестах ТК1 не отвечает никогда: запись, которая её
// ждёт, повиснет — это и проверяется.
//
// Толкования:
//   - «запрос к воркеру» при записи — любой вызов api, кроме сохранения самого черновика (drafts, draftPut,
//     draftDelete) и списка джоб очереди рендеров (jobs);
//   - «кнопка свободна сразу» — промис функции записи разрешается без ответа пересборки (пересборка в тесте
//     не отвечает никогда);
//   - Wails отклоняет промис строкой, поэтому ошибка сборки проверяется и строкой, и объектом Error;
//   - отклоняет ли сам ensureMix при сбое — не проверяется (контракт не задаёт), проверяются отметки записей.
import { describe, it, expect, vi, afterEach } from 'vitest'

const mock = vi.hoisted(() => {
  const state = { stray: [] }
  const target = {}
  const api = new Proxy(target, {
    get(t, k) {
      if (k in t) return t[k]
      if (typeof k !== 'string' || k === 'then' || k === 'toJSON') return undefined
      return (...args) => {
        state.stray.push(k)
        void args
        return Promise.resolve(undefined)
      }
    },
  })
  return { state, target, api }
})
vi.mock('../api.js', () => ({ api: mock.api }))
const apiMock = mock.target

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

const WAIT = { timeout: 3000, interval: 20 }
const T = 7
const clone = (x) => JSON.parse(JSON.stringify(x))
const flush = async () => { for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0) }

// записи черновика в форме useInserts
const base = { from: 0, to: 0, lead: 0, beat: 0, db: 0, fadeIn: 0, fadeOut: 0, keepHighHz: 0 }
const fxRec = (childId, over = {}) => ({ ...base, childId, instId: 'fx-soften', stems: ['vocals'],
  chain: 'soften', params: { strength: 0.6 }, ...over })
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', from: 10, to: 20, stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })

// рецепт пресета — объект пресета звука (yue.SoundPreset)
const RECIPE = {
  id: 71, name: 'Пост-панк', specs: [{ stems: ['bass'], engine: [{ type: 'eq', highpass_hz: 80 }], db: -2 }],
  final: [{ chain: 'width', params: { width: 1.1 } }], target_lufs: -13, master: [{ type: 'glue', ratio: 2 }],
}
const INFO = { id: 71, name: 'Пост-панк' }

const remote = (jobId, edits) => ({ job_id: jobId, state: { v: 1, edits, queue: [], ts: 1000 } })

// rebuildSections по умолчанию: n-й вызов отвечает файлом listen-n.flac
function rebuilds(answer = () => ({})) {
  let n = 0
  return vi.fn(async (_p, specs) => {
    n++
    const extra = await answer(n, specs)
    return {
      variant: { file: `listen-${n}.flac` },
      inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
      ...extra,
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
  apiMock.drafts = vi.fn(async () => clone(drafts))
  apiMock.draftPut = vi.fn(async () => undefined)
  apiMock.draftDelete = vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  await flush()
  for (const d of drafts) {
    await vi.waitFor(() => expect((ins.appliedFor(d.job_id) || []).length).toBe(d.state.edits.length), WAIT)
  }
  mock.state.stray.length = 0
  return ins
}

const recOf = (ins, id, childId) => (ins.appliedFor(id) || []).find(x => x.childId === childId)
const callsFor = (id) => apiMock.rebuildSections.mock.calls.filter(c => Number(c[0]) === Number(id))
const specIds = (specs) => specs.map(s => s.id).sort()

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})


// управляемая пересборка: n-й вызов ждёт release(n-1)
function held() {
  const res = []
  const fn = vi.fn((_p, specs) => new Promise((r) => res.push((extra = {}) => r({
    variant: { file: `held-${res.length}.flac` },
    inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
    ...extra,
  }))))
  return { fn, res }
}

describe('1: свежесть проверяется и в очереди сборок', () => {
  it('два ensureMix во время сборки, черновик тот же → одна пересборка; второй — {cached, file} этой сборки', async () => {
    const h = held()
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: h.fn })
    const p1 = ins.ensureMix(T)
    await flush()
    const p2 = ins.ensureMix(T)
    await flush()
    expect(h.fn).toHaveBeenCalledTimes(1)
    h.res[0]()
    const r1 = await p1
    const r2 = await p2
    await flush()
    expect(h.fn).toHaveBeenCalledTimes(1)
    expect(r1.variant.file).toBe('held-1.flac')
    expect(r2).toMatchObject({ cached: true, file: 'held-1.flac' })
  })

  it('контроль: правка между двумя ensureMix → вторая сборка после первой', async () => {
    const h = held()
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: h.fn })
    const p1 = ins.ensureMix(T)
    await flush()
    await ins.setOff(T, -11, true)
    await ins.addStemFx(T, { stem: 'bass', chain: 'reverb', params: { mix: 0.2 } })
    const p2 = ins.ensureMix(T)
    await flush()
    expect(h.fn).toHaveBeenCalledTimes(1)
    h.res[0]()
    await p1
    await flush()
    expect(h.fn).toHaveBeenCalledTimes(2)
    h.res[1]()
    await p2
  })
})

describe('2: ensureMix во время «применить всё» не собирает', () => {
  it('applyAll идёт → ни одной пересборки, ответ {cached: true, file: mixFile(трек)}', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.ensureMix(T)
    let done
    apiMock.applyEdits = vi.fn(() => new Promise((r) => { done = r }))
    const pa = ins.applyAll(T, 'итог')
    await flush()
    const n = apiMock.rebuildSections.mock.calls.length
    const file = ins.mixFile(T)
    const r = await ins.ensureMix(T)
    await flush()
    expect(apiMock.rebuildSections.mock.calls.length).toBe(n)
    expect(r).toEqual({ cached: true, file })
    done(501)
    await pa
  })

  it('applyAll идёт, черновик ни разу не собирался → всё равно без пересборки', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    let done
    apiMock.applyEdits = vi.fn(() => new Promise((r) => { done = r }))
    const pa = ins.applyAll(T, 'итог')
    await flush()
    const r = await ins.ensureMix(T)
    expect(apiMock.rebuildSections).not.toHaveBeenCalled()
    expect(r).toMatchObject({ cached: true })
    done(501)
    await pa
  })
})

describe('3: currentMix — файл «было» (трек со всеми правками черновика)', () => {
  it('правок нет → null без сборки', async () => {
    const ins = await load({ drafts: [] })
    expect(await ins.currentMix(T)).toBeNull()
    expect(apiMock.rebuildSections).not.toHaveBeenCalled()
  })

  it('все правки выключены → null без сборки', async () => {
    const ins = await load({ drafts: [remote(T, [{ ...E1, off: true }])] })
    expect(await ins.currentMix(T)).toBeNull()
    expect(apiMock.rebuildSections).not.toHaveBeenCalled()
  })

  it('черновик устарел → одна сборка, ответ — её файл', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    expect(await ins.currentMix(T)).toBe('listen-1.flac')
    expect(callsFor(T).length).toBe(1)
    expect(specIds(callsFor(T)[0][1])).toEqual(['-11', '-12'])
  })

  it('свежий → файл без сборки', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.ensureMix(T)
    expect(await ins.currentMix(T)).toBe('listen-1.flac')
    expect(callsFor(T).length).toBe(1)
  })

  it('после правки снова собирает и отдаёт новый файл', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.currentMix(T)
    await ins.addStemFx(T, { stem: 'bass', chain: 'reverb', params: { mix: 0.2 } })
    expect(await ins.currentMix(T)).toBe('listen-2.flac')
    expect(callsFor(T).length).toBe(2)
  })
})

describe('4: warnings — только пресету, который был в собранных записях', () => {
  it('пресет заменили во время сборки → у нового пресета warnings старой сборки нет', async () => {
    const h = held()
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: h.fn })
    await ins.putPreset(T, clone(RECIPE), INFO)
    const p = ins.ensureMix(T)
    await flush()
    await ins.putPreset(T, { ...clone(RECIPE), id: 72, name: 'Гранж' }, { id: 72, name: 'Гранж' })
    h.res[0]({ warnings: ['предупреждение пресета A'] })
    await p
    await flush()
    const rec = ins.appliedFor(T).find(x => x.preset)
    expect(rec.preset.id).toBe(72)
    expect(rec.warnings || []).toEqual([])
    expect(ins.mixStale(T)).toBe(true)
  })

  it('пресет добавили во время сборки без пресета → warnings не получает', async () => {
    const h = held()
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: h.fn })
    const p = ins.ensureMix(T)
    await flush()
    await ins.putPreset(T, clone(RECIPE), INFO)
    h.res[0]({ warnings: ['чьё-то предупреждение'] })
    await p
    await flush()
    expect(ins.appliedFor(T).find(x => x.preset).warnings || []).toEqual([])
  })

  it('контроль: пресет был в сборке и остался → warnings у него', async () => {
    const h = held()
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: h.fn })
    await ins.putPreset(T, clone(RECIPE), INFO)
    const p = ins.ensureMix(T)
    await flush()
    await ins.addStemFx(T, { stem: 'bass', chain: 'reverb', params: { mix: 0.2 } })
    h.res[0]({ warnings: ['нет плана'] })
    await p
    await flush()
    expect(ins.appliedFor(T).find(x => x.preset).warnings).toEqual(['нет плана'])
  })
})

describe('5: ошибка с отметкой наружу — без отметки', () => {
  const MARKED = 'движок: нет памяти {edit:-12}'
  const CLEAN = 'движок: нет памяти'
  const reason = async (p) => { try { await p } catch (e) { return e } throw new Error('промис не отклонён') }

  for (const [name, run] of [
    ['ensureMix', (ins) => ins.ensureMix(T)],
    ['rebuild', (ins) => ins.rebuild(T)],
  ]) {
    it(`${name}: строка → строка без отметки; у записи error`, async () => {
      const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: vi.fn(async () => { throw MARKED }) })
      const e = await reason(run(ins))
      expect(typeof e).toBe('string')
      expect(e.trim()).toBe(CLEAN)
      await flush()
      expect(recOf(ins, T, -12).error).toContain(CLEAN)
      expect(recOf(ins, T, -12).error).not.toContain('{edit:')
    })

    it(`${name}: Error → Error, message без отметки; у записи error`, async () => {
      const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: vi.fn(async () => { throw new Error(MARKED) }) })
      const e = await reason(run(ins))
      expect(e).toBeInstanceOf(Error)
      expect(e.message.trim()).toBe(CLEAN)
      await flush()
      expect(recOf(ins, T, -12).error).toContain(CLEAN)
    })
  }

  it('applyAll: строка → строка без отметки; у записи error; черновик цел', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    apiMock.applyEdits = vi.fn(async () => { throw MARKED })
    const e = await reason(ins.applyAll(T, 'итог'))
    expect(typeof e).toBe('string')
    expect(e.trim()).toBe(CLEAN)
    await flush()
    expect(recOf(ins, T, -12).error).toContain(CLEAN)
    expect(ins.appliedFor(T).length).toBe(2)
  })

  it('applyAll: Error → Error без отметки', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    apiMock.applyEdits = vi.fn(async () => { throw new Error(MARKED) })
    const e = await reason(ins.applyAll(T, 'итог'))
    expect(e).toBeInstanceOf(Error)
    expect(e.message.trim()).toBe(CLEAN)
    await flush()
    expect(recOf(ins, T, -12).error).toContain(CLEAN)
  })

  it('ошибка без отметки — наружу как есть', async () => {
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: vi.fn(async () => { throw 'воркер недоступен' }) })
    expect(await reason(ins.ensureMix(T))).toBe('воркер недоступен')
  })
})
