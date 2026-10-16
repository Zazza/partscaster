// Реестр «Правки трека» (useInserts) и черновик трека на воркере — карточка internal-studio-draft,
// условия 4, 5 (step у addStemFx), 6 (applyAll, clearDraft). Модуль — синглтон: каждый тест
// импортирует его заново после vi.resetModules() (как в useInserts.test.js).
import { describe, it, expect, vi, afterEach } from 'vitest'
import { unref } from 'vue'

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

// Отчёт воркера о пересборке для набора спек.
function report(specs) {
  return {
    variant: { file: 'out.wav' },
    inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
  }
}

// записи реестра в форме, как их кладёт addStemFx: childId < 0, в api — child_id 0
const fxRec = (childId, over = {}) => ({
  childId, instId: 'fx-soften', from: 0, to: 0, lead: 0, beat: 0, db: 0,
  stems: ['vocals'], fadeIn: 0, fadeOut: 0, keepHighHz: 0,
  chain: 'soften', params: { strength: 0.6 }, ...over,
})
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', from: 10, to: 20, stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })
const E3 = fxRec(-13, { instId: 'fx-soften-2', from: 30, to: 40, stems: ['other'], params: { strength: 0.3 } })
// обычная вклейка рендера
const A = { childId: 1, instId: 'i-a', from: 0, to: 10, lead: 0, beat: 0.5, db: -6 }
// ожидающий рендер (спека очереди) трека 7 и трека 8
const spec = (parent, childId) => ({ parent, childId, instId: 'i-' + childId, from: 5, to: 15, lead: 0.1, beat: 0.5, db: -6, srcJob: parent })

const T = 7      // трек, над которым идёт работа
const OTHER = 8  // соседний трек — его не трогаем

const fx = (over = {}) => ({ stem: 'vocals', chain: 'soften', params: { strength: 0.6, freq: 6000 }, from: 176, to: 0, ...over })

// Поднять модуль: localStorage (реестр, очередь, ts) + ответы воркера.
// drafts: массив черновиков воркера или Error (воркер недоступен / старый).
async function load({ applied = {}, queue = [], ts, drafts = [], jobs = [], draftPut, draftDelete, applyEdits, rebuildSections } = {}) {
  vi.resetModules()
  globalThis.localStorage = new MemStorage()
  localStorage.setItem('yue_insert_queue', JSON.stringify(queue))
  localStorage.setItem('yue_insert_applied', JSON.stringify(applied))
  if (ts !== undefined) localStorage.setItem('yue_insert_ts', JSON.stringify(ts))
  vi.useFakeTimers()
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => jobs)
  apiMock.rebuildSections = rebuildSections || vi.fn(async (_p, specs) => report(specs))
  apiMock.rebuildInserts = vi.fn(async (_p, specs) => report(specs))
  apiMock.drafts = drafts instanceof Error
    ? vi.fn(async () => { throw drafts })
    : vi.fn(async () => JSON.parse(JSON.stringify(drafts)))
  apiMock.draftPut = draftPut || vi.fn(async () => undefined)
  apiMock.draftDelete = draftDelete || vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = applyEdits || vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  // дать модулю дочитать черновики воркера (слияние при старте асинхронное)
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  return ins
}

const WAIT = { timeout: 3000, interval: 20 }
const remote = (jobId, edits, queue = [], ts = 1000) => ({ job_id: jobId, state: { v: 1, edits, queue, ts } })

// вызовы api.draftPut / draftDelete для трека (id сравниваем числом: в api уходит job_id)
const putsFor = (id) => apiMock.draftPut.mock.calls.filter(c => Number(c[0]) === Number(id))
const lastPut = (id) => putsFor(id).at(-1)
const deletesFor = (id) => apiMock.draftDelete.mock.calls.filter(c => Number(c[0]) === Number(id))
const ids = (list) => (list || []).map(x => x.childId).sort((a, b) => a - b)
const recOf = (ins, id, childId) => ins.appliedFor(id).find(x => x.childId === childId)
const savedApplied = () => JSON.parse(localStorage.getItem('yue_insert_applied') || '{}')
const pendingOf = (ins) => unref(ins.pending) || []

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('старт: локальная копия сливается с черновиками воркера', () => {
  it('черновик только на воркере → его записи в appliedFor, ожидающие рендеры — в очереди с parent трека; на воркер не отправляется', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2], [{ childId: 3, instId: 'i-3', from: 1, to: 2 }])] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-12, -11]), WAIT)
    const q = pendingOf(ins).filter(s => Number(s.parent) === T)
    expect(q.map(s => s.childId)).toEqual([3])
    expect(putsFor(T)).toEqual([])
  })

  it('трек только в localStorage (перенос старых правок) → отправлен на воркер как {v:1, edits, queue, ts}', async () => {
    const ins = await load({ applied: { [T]: [E1] }, queue: [spec(T, 4)], drafts: [] })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    const [, state] = lastPut(T)
    expect(state.v).toBe(1)
    expect(state.edits).toEqual([E1])
    expect(state.queue.map(s => s.childId)).toEqual([4])
    expect(state.queue[0].parent).toBeUndefined()
    expect(typeof state.ts).toBe('number')
    expect(ids(ins.appliedFor(T))).toEqual([-11])
  })

  it('локальное без ts и черновик воркера по тому же треку → берётся воркерный, на воркер не отправляется', async () => {
    const ins = await load({ applied: { [T]: [E1] }, drafts: [remote(T, [E2], [], 5000)] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-12]), WAIT)
    expect(putsFor(T)).toEqual([])
  })

  it('локальное новее воркерного по ts (yue_insert_ts) → остаётся локальное и уходит на воркер', async () => {
    const ins = await load({ applied: { [T]: [E1] }, ts: { [T]: 9000 }, drafts: [remote(T, [E2], [], 5000)] })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(ids(ins.appliedFor(T))).toEqual([-11])
    expect(lastPut(T)[1].edits).toEqual([E1])
  })

  it('черновики двух треков с воркера: оба трека в реестре, ни один не уходит обратно', async () => {
    const ins = await load({ drafts: [remote(T, [E1]), remote(OTHER, [E2])] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(OTHER))).toEqual([-12]), WAIT)
    expect(ids(ins.appliedFor(T))).toEqual([-11])
    expect(apiMock.draftPut).not.toHaveBeenCalled()
  })

  it('api.drafts() бросает (старый воркер / нет связи) → работа по localStorage, appliedFor как прежде', async () => {
    const ins = await load({ applied: { [T]: [E1, E2] }, drafts: new Error('404 page not found') })
    expect(ids(ins.appliedFor(T))).toEqual([-12, -11])
    // реестр работает: правка проходит и сохраняется локально
    await ins.setOff(T, -11, true)
    expect(recOf(ins, T, -11).off).toBe(true)
    expect(savedApplied()[T].find(x => x.childId === -11).off).toBe(true)
  })

  it('пусто и там, и там → appliedFor пуст, на воркер ничего не отправляется и не удаляется', async () => {
    const ins = await load({})
    expect(ins.appliedFor(T) || []).toEqual([])
    expect(apiMock.draftPut).not.toHaveBeenCalled()
    expect(apiMock.draftDelete).not.toHaveBeenCalled()
  })
})

describe('изменение реестра трека → черновик на воркере', () => {
  it('addStemFx → draftPut этого трека с новой записью; localStorage тоже обновлён', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.addStemFx(T, fx())
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    const st = lastPut(T)[1]
    expect(st.v).toBe(1)
    expect(st.edits.length).toBe(2)
    expect(st.edits.find(x => x.chain === 'soften' && x.from === 176)).toBeTruthy()
    expect(st.edits).toEqual(JSON.parse(JSON.stringify(ins.appliedFor(T))))
    expect(savedApplied()[T].length).toBe(2)
  })

  it('setOff → draftPut с выключенной записью', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.setOff(T, -11, true)
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    const st = lastPut(T)[1]
    expect(st.edits.find(x => x.childId === -11).off).toBe(true)
    expect(st.edits.find(x => x.childId === -12).off || false).toBe(false)
  })

  it('setDb → draftPut с новой громкостью', async () => {
    const ins = await load({ drafts: [remote(T, [A])] })
    await ins.setDb(T, 1, -12)
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(lastPut(T)[1].edits.find(x => x.childId === 1).db).toBe(-12)
  })

  it('remove одной из двух → draftPut с оставшейся, без удаления черновика', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.remove(T, -11)
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(ids(lastPut(T)[1].edits)).toEqual([-12])
    expect(deletesFor(T)).toEqual([])
  })

  it('remove последней записи (очередь трека пуста) → api.draftDelete трека', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.remove(T, -11)
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
    // пустой черновик не отправляется PUT-ом после удаления
    const lastDel = Math.max(...apiMock.draftDelete.mock.invocationCallOrder)
    const putOrders = apiMock.draftPut.mock.calls
      .map((c, i) => [c, apiMock.draftPut.mock.invocationCallOrder[i]])
      .filter(([c, order]) => Number(c[0]) === T && order > lastDel)
    expect(putOrders).toEqual([])
  })

  it('изменение трека 7 не отправляет неизменённый трек 8', async () => {
    const ins = await load({ drafts: [remote(T, [E1]), remote(OTHER, [E2])] })
    await ins.addStemFx(T, fx())
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(putsFor(OTHER)).toEqual([])
    expect(deletesFor(OTHER)).toEqual([])
  })

  it('воркер недоступен (draftPut бросает) → localStorage сохранён; при следующем изменении отправка повторяется с актуальным состоянием', async () => {
    let fail = true
    const draftPut = vi.fn(async () => { if (fail) throw new Error('connection refused') })
    const ins = await load({ drafts: [], draftPut })
    await ins.addStemFx(T, fx())
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(savedApplied()[T].length).toBe(1)
    expect(ids(ins.appliedFor(T)).length).toBe(1)

    fail = false
    const before = putsFor(T).length
    await ins.addStemFx(T, fx({ chain: 'dewhistle', params: { freqs: [2638] } }))
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(before), WAIT)
    const st = lastPut(T)[1]
    expect(st.edits.map(x => x.chain).sort()).toEqual(['dewhistle', 'soften'])
    expect(savedApplied()[T].length).toBe(2)
  })
})

describe('addStemFx: шаг записи (step)', () => {
  it("step: 'voice' → у записи реестра и в черновике step 'voice'", async () => {
    const ins = await load({})
    await ins.addStemFx(T, fx({ step: 'voice' }))
    const rec = ins.appliedFor(T).find(x => x.chain === 'soften')
    expect(rec.step).toBe('voice')
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(lastPut(T)[1].edits.find(x => x.chain === 'soften').step).toBe('voice')
  })

  it('без step → у записи нет поля step', async () => {
    const ins = await load({})
    await ins.addStemFx(T, fx())
    const rec = ins.appliedFor(T).find(x => x.chain === 'soften')
    expect('step' in rec).toBe(false)
  })
})

describe('applyAll — «применить всё»', () => {
  // какие спеки уходят в пересборку при том же реестре — эталон для applyEdits
  async function rebuildSpecsFor(edits) {
    const ins = await load({ drafts: [remote(T, edits)] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(edits.length), WAIT)
    await ins.rebuild(T)
    await vi.waitFor(() => expect(apiMock.rebuildSections).toHaveBeenCalled(), WAIT)
    return JSON.parse(JSON.stringify(apiMock.rebuildSections.mock.calls.at(-1)[1]))
  }

  it('api.applyEdits(parentId, спеки только включённых записей — как у пересборки, title); возвращает id нового трека', async () => {
    const edits = [E1, { ...E2, off: true }, E3]
    const expected = await rebuildSpecsFor(edits)
    expect(expected.map(s => s.chain).sort()).toEqual(['soften', 'soften'])  // выключенная в эталон не попала

    const ins = await load({ drafts: [remote(T, edits)] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(3), WAIT)
    const id = await ins.applyAll(T, 'Песня · итог')
    expect(id).toBe(501)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
    const [parent, specs, title] = apiMock.applyEdits.mock.calls[0]
    expect(Number(parent)).toBe(T)
    expect(specs).toEqual(expected)
    expect(specs.some(s => s.chain === 'dewhistle')).toBe(false)
    expect(title).toBe('Песня · итог')
  })

  it('после успеха реестр трека пуст и черновик удалён (api.draftDelete)', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    await ins.applyAll(T, 'итог')
    expect(ins.appliedFor(T) || []).toEqual([])
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
    expect(savedApplied()[T] || []).toEqual([])
  })

  it('соседний трек после applyAll не тронут', async () => {
    const ins = await load({ drafts: [remote(T, [E1]), remote(OTHER, [E2])] })
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(1), WAIT)
    await ins.applyAll(T, 'итог')
    expect(ids(ins.appliedFor(OTHER))).toEqual([-12])
    expect(deletesFor(OTHER)).toEqual([])
  })

  it('ошибка applyEdits → та же ошибка наружу, реестр не тронут, черновик не удалён', async () => {
    const err = new Error('rebuild failed')
    const applyEdits = vi.fn(async () => { throw err })
    const ins = await load({ drafts: [remote(T, [E1, E2])], applyEdits })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    await expect(ins.applyAll(T, 'итог')).rejects.toBe(err)
    expect(ids(ins.appliedFor(T))).toEqual([-12, -11])
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
    expect(deletesFor(T)).toEqual([])
  })
})

describe('clearDraft — «сбросить черновик»', () => {
  it('реестр и ожидающие рендеры трека пусты, api.draftDelete(трек); соседний трек не тронут', async () => {
    const ins = await load({
      drafts: [
        remote(T, [E1, E2], [{ childId: 3, instId: 'i-3', from: 1, to: 2 }]),
        remote(OTHER, [E3], [{ childId: 4, instId: 'i-4', from: 1, to: 2 }]),
      ],
    })
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(1), WAIT)
    await ins.clearDraft(T)
    expect(ins.appliedFor(T) || []).toEqual([])
    expect(pendingOf(ins).filter(s => Number(s.parent) === T)).toEqual([])
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)

    expect(ids(ins.appliedFor(OTHER))).toEqual([-13])
    expect(pendingOf(ins).filter(s => Number(s.parent) === OTHER).map(s => s.childId)).toEqual([4])
    expect(deletesFor(OTHER)).toEqual([])
    expect(putsFor(OTHER)).toEqual([])
  })

  it('после clearDraft в localStorage нет записей и очереди трека', async () => {
    const ins = await load({ applied: { [T]: [E1] }, queue: [spec(T, 4), spec(OTHER, 5)], drafts: new Error('offline') })
    await ins.clearDraft(T)
    expect(savedApplied()[T] || []).toEqual([])
    const q = JSON.parse(localStorage.getItem('yue_insert_queue') || '[]')
    expect(q.filter(s => Number(s.parent) === T)).toEqual([])
    expect(q.filter(s => Number(s.parent) === OTHER).map(s => s.childId)).toEqual([5])
  })

  it('clearDraft трека без правок — без ошибки, черновик на воркере всё равно удаляется', async () => {
    const ins = await load({})
    await ins.clearDraft(T)
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
  })
})

// «Перезапуск приложения»: модуль поднимается заново с тем же localStorage
// (в отличие от load(), которая начинает с чистой копии на ПК).
async function restart({ drafts = [], draftPut, draftDelete, applyEdits } = {}) {
  vi.resetModules()
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => [])
  apiMock.rebuildSections = vi.fn(async (_p, specs) => report(specs))
  apiMock.rebuildInserts = vi.fn(async (_p, specs) => report(specs))
  apiMock.drafts = typeof drafts === 'function'
    ? vi.fn(drafts)
    : drafts instanceof Error
      ? vi.fn(async () => { throw drafts })
      : vi.fn(async () => JSON.parse(JSON.stringify(drafts)))
  apiMock.draftPut = draftPut || vi.fn(async () => undefined)
  apiMock.draftDelete = draftDelete || vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = applyEdits || vi.fn(async () => 501)
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  return ins
}

// воркер перестал отвечать на запись черновиков
function workerDown() {
  apiMock.draftPut.mockImplementation(async () => { throw new Error('connection refused') })
  apiMock.draftDelete.mockImplementation(async () => { throw new Error('connection refused') })
}

// sync-throw и rejected promise сводятся к одному: «правка отклонена»
const attempt = (fn) => Promise.resolve().then(fn)

describe('ТК10: сброс офлайн не воскресает после перезапуска', () => {
  it('clearDraft при недоступном воркере → после перезапуска старый черновик воркера не возвращается и удаляется', async () => {
    const oldTs = Date.now() - 60_000
    const ins = await load({ drafts: [remote(T, [E1], [], oldTs)] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-11]), WAIT)

    workerDown()
    await vi.advanceTimersByTimeAsync(1000)
    await attempt(() => ins.clearDraft(T)).catch(() => {})  // офлайн-ошибка сброса не предмет теста
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
    expect(ins.appliedFor(T) || []).toEqual([])

    const again = await restart({ drafts: [remote(T, [E1], [], oldTs)] })
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
    expect(again.appliedFor(T) || []).toEqual([])
    expect(pendingOf(again).filter(s => Number(s.parent) === T)).toEqual([])
  })

  it('remove последней записи при недоступном воркере → после перезапуска правка не воскресает, черновик удаляется', async () => {
    const oldTs = Date.now() - 60_000
    const ins = await load({ drafts: [remote(T, [E1], [], oldTs)] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-11]), WAIT)

    workerDown()
    await vi.advanceTimersByTimeAsync(1000)
    await attempt(() => ins.remove(T, -11)).catch(() => {})
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
    expect(ins.appliedFor(T) || []).toEqual([])

    const again = await restart({ drafts: [remote(T, [E1], [], oldTs)] })
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
    expect(again.appliedFor(T) || []).toEqual([])
  })
})

describe('ТК11: старт без воркера не затирает чужие черновики', () => {
  it('drafts() бросает → правка трека 8 уходит на воркер, трек 7 — нет; когда воркер ответил, его более новый черновик 7 побеждает', async () => {
    // первый запуск: трек 7 синхронизирован с рабочим воркером
    const syncedTs = Date.now() - 60_000
    const first = await load({ drafts: [remote(T, [E1], [], syncedTs)] })
    await vi.waitFor(() => expect(ids(first.appliedFor(T))).toEqual([-11]), WAIT)

    // перезапуск без связи с воркером
    let online = false
    const newer = Date.now() + 120_000  // черновик 7 правили в другом месте позже
    const ins = await restart({
      drafts: async () => {
        if (!online) throw new Error('connection refused')
        return JSON.parse(JSON.stringify([remote(T, [E2], [], newer)]))
      },
    })
    expect(ids(ins.appliedFor(T))).toEqual([-11])  // работа по локальной копии

    await ins.addStemFx(OTHER, fx())
    await vi.waitFor(() => expect(putsFor(OTHER).length).toBeGreaterThan(0), WAIT)
    expect(putsFor(T)).toEqual([])
    expect(deletesFor(T)).toEqual([])

    // воркер вернулся: повтор чтения черновиков (~15 с)
    online = true
    await vi.advanceTimersByTimeAsync(16_000)
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-12]), WAIT)
    expect(putsFor(T)).toEqual([])
    expect(deletesFor(T)).toEqual([])
    // правка трека 8 не потерялась
    expect(ins.appliedFor(OTHER).some(x => x.chain === 'soften' && x.from === 176)).toBe(true)
  })
})

describe('ТК12: правки трека во время «применить всё» отклоняются', () => {
  it('пока applyEdits не завершён: addStemFx/remove трека 7 отклонены, реестр 7 не меняется; трек 8 правится; после завершения правки 7 снова принимаются', async () => {
    let finish
    const applyEdits = vi.fn(() => new Promise((resolve) => { finish = resolve }))
    const ins = await load({ drafts: [remote(T, [E1, E2]), remote(OTHER, [E3])], applyEdits })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(1), WAIT)

    const applying = ins.applyAll(T, 'итог')
    await vi.waitFor(() => expect(applyEdits).toHaveBeenCalled(), WAIT)

    await expect(attempt(() => ins.addStemFx(T, fx()))).rejects.toThrow()
    await expect(attempt(() => ins.remove(T, -11))).rejects.toThrow()
    expect(ids(ins.appliedFor(T))).toEqual([-12, -11])
    expect(ins.appliedFor(T).some(x => x.from === 176)).toBe(false)

    // правка соседнего трека завершается, не дожидаясь «применить всё» трека 7
    let done8 = false
    const edit8 = attempt(() => ins.addStemFx(OTHER, fx())).then(() => { done8 = true })
    await vi.advanceTimersByTimeAsync(5_000)
    expect(ins.appliedFor(OTHER).length).toBe(2)
    expect(done8).toBe(true)

    finish(501)
    await expect(applying).resolves.toBe(501)
    await edit8

    await ins.addStemFx(T, fx())
    expect(ins.appliedFor(T).some(x => x.chain === 'soften' && x.from === 176)).toBe(true)
  })
})

describe('ТК13: превью и пересборка трека во время «применить всё» отклоняются', () => {
  it('пока applyEdits не завершён: previewSpecs/rebuild трека 7 отклонены без запроса к воркеру; трек 8 слушается; после завершения превью 7 снова проходит', async () => {
    const gains = [{ child_id: 0, from: 0, to: 0, stems: ['drums'], db: 3 }]
    const sectionsFor = (id) => apiMock.rebuildSections.mock.calls.filter(c => Number(c[0]) === id)
    let finish
    const applyEdits = vi.fn(() => new Promise((resolve) => { finish = resolve }))
    const ins = await load({ drafts: [remote(T, [E1, E2]), remote(OTHER, [E3])], applyEdits })
    await vi.waitFor(() => expect(ins.appliedFor(T).length).toBe(2), WAIT)
    await vi.waitFor(() => expect(ins.appliedFor(OTHER).length).toBe(1), WAIT)

    const applying = ins.applyAll(T, 'итог')
    await vi.waitFor(() => expect(applyEdits).toHaveBeenCalled(), WAIT)
    const before = sectionsFor(T).length

    await expect(attempt(() => ins.previewSpecs(T, gains))).rejects.toThrow()
    await expect(attempt(() => ins.rebuild(T))).rejects.toThrow()
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
    expect(sectionsFor(T).length).toBe(before)

    const r8 = await ins.previewSpecs(OTHER, gains)
    expect(r8.variant.file).toBe('out.wav')
    expect(sectionsFor(OTHER).length).toBeGreaterThan(0)

    finish(501)
    await expect(applying).resolves.toBe(501)

    const r7 = await ins.previewSpecs(T, gains)
    expect(r7.variant.file).toBe('out.wav')
    expect(sectionsFor(T).length).toBeGreaterThan(before)
  })
})
