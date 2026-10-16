// Реестр «Правки трека» (useInserts): изменения с рендером (gen) и «применить всё» через рендер —
// карточка internal-studio-draft3, условия 1, 4 (сброс), 6, 7; ТК3–ТК5. Модуль — синглтон: каждый тест
// поднимает его заново после vi.resetModules() (как в useInserts.draft.test.js).
import { describe, it, expect, vi, afterEach } from 'vitest'

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

// записи реестра в форме addStemFx: childId < 0
const fxRec = (childId, over = {}) => ({
  childId, instId: 'fx-soften', from: 0, to: 0, lead: 0, beat: 0, db: 0,
  stems: ['vocals'], fadeIn: 0, fadeOut: 0, keepHighHz: 0,
  chain: 'soften', params: { strength: 0.6 }, ...over,
})
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', from: 10, to: 20, stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })
const fx = (over = {}) => ({ stem: 'vocals', chain: 'soften', params: { strength: 0.6, freq: 6000 }, from: 176, to: 0, ...over })

const T = 7        // трек, над которым идёт работа
const OTHER = 8    // соседний трек
const NEW = 901    // джоба рендера «применить всё»
const TITLE = 'Песня · итог'

// изменения с рендером
const S1 = { kind: 'octave', dir: 'up', label: 'октава вверх', targets: [{ voice: 'Vocal', bar: 0 }] }
const S2 = { kind: 'rest', label: 'пауза', targets: [{ voice: 'Ins', bar: 1 }] }
const VOICE = { params: { register: 'female', rough: 0, creak: 0, delivery: 1, breath: 1, extra: '' }, name: 'Эфир' }
const GEN = { specs: [S1], voice: VOICE, instrumental: false }
// тело рендера — готовит студия (genRequest); реестр передаёт его в api.submit как есть
const REQUEST = { title: TITLE, style: 'post-punk, airy female vocals', lyrics: 'la', seed: 5, cot: 'full',
  abc: 'X:1', parent_id: T, role: 'rebuild' }

const WAIT = { timeout: 3000, interval: 20 }
const remote = (jobId, edits, extra = {}, ts = 1000) => ({ job_id: jobId, state: { v: 1, edits, queue: [], ts, ...extra } })

// Список джоб воркера — управляет тест: jobsNow.list
const jobsNow = { list: [] }
const setJob = (id, status, extra = {}) => {
  jobsNow.list = jobsNow.list.filter(j => j.id !== id).concat([{ id, status, parent_id: T, role: 'rebuild', title: TITLE, ...extra }])
}

function mockApi({ drafts = [], draftPut, applyEdits, submit, rebuildSections } = {}) {
  for (const k of Object.keys(apiMock)) delete apiMock[k]
  apiMock.jobs = vi.fn(async () => JSON.parse(JSON.stringify(jobsNow.list)))
  apiMock.rebuildSections = rebuildSections || vi.fn(async (_p, specs) => report(specs))
  apiMock.rebuildInserts = vi.fn(async (_p, specs) => report(specs))
  apiMock.drafts = typeof drafts === 'function'
    ? vi.fn(drafts)
    : drafts instanceof Error
      ? vi.fn(async () => { throw drafts })
      : vi.fn(async () => JSON.parse(JSON.stringify(drafts)))
  apiMock.draftPut = draftPut || vi.fn(async () => undefined)
  apiMock.draftDelete = vi.fn(async () => ({ deleted: true }))
  apiMock.applyEdits = applyEdits || vi.fn(async () => 501)
  apiMock.submit = submit || vi.fn(async () => NEW)
  apiMock.renameJob = vi.fn(async () => undefined)
}

async function boot() {
  const mod = await import('./useInserts.js')
  const ins = mod.useInserts()
  await vi.waitFor(() => expect(apiMock.drafts).toHaveBeenCalled(), WAIT)
  for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  return ins
}

// Поднять модуль с чистой копией на ПК
async function load(opts = {}) {
  vi.clearAllTimers()   // часы прежнего экземпляра модуля (если был) не должны тикать
  vi.resetModules()
  globalThis.localStorage = new MemStorage()
  vi.useFakeTimers()
  jobsNow.list = [{ id: T, status: 'done' }, { id: OTHER, status: 'done' }]
  mockApi(opts)
  return boot()
}

// «Перезапуск приложения»: модуль заново, localStorage тот же, список джоб тот же
async function restart(opts = {}) {
  vi.clearAllTimers()   // прежний экземпляр «закрыт»: его опрос джоб не продолжается
  vi.resetModules()
  mockApi(opts)
  return boot()
}

// крутить часы модуля (опрос джоб), пока условие не выполнится
async function until(check, ms = 60_000) {
  for (let t = 0; t < ms; t += 1000) {
    try { check(); return } catch { /* ещё не выполнено — ждём следующий тик */ }
    await vi.advanceTimersByTimeAsync(1000)
  }
  check()
}

const putsFor = (id) => apiMock.draftPut.mock.calls.filter(c => Number(c[0]) === Number(id))
const lastPut = (id) => putsFor(id).at(-1)
const deletesFor = (id) => apiMock.draftDelete.mock.calls.filter(c => Number(c[0]) === Number(id))
const ids = (list) => (list || []).map(x => x.childId).sort((a, b) => a - b)
const chains = (list) => (list || []).map(x => x.chain).sort()
const attempt = (fn) => Promise.resolve().then(fn)

// пустой gen: нет приёмов, голоса и «без голоса»
const genEmpty = (g) => !g || (!(g.specs || []).length && !g.voice && !g.instrumental)

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

describe('ТК3: genFor / setGen', () => {
  it('трек без изменений с рендером → genFor пуст', async () => {
    const ins = await load({})
    expect(genEmpty(ins.genFor(T))).toBe(true)
  })

  it('setGen сливает поля: приёмы, затем «без голоса», затем голос — всё вместе', async () => {
    const ins = await load({})
    await ins.setGen(T, { specs: [S1] })
    expect(ins.genFor(T).specs).toEqual([S1])
    await ins.setGen(T, { instrumental: true })
    expect(ins.genFor(T).specs).toEqual([S1])
    expect(ins.genFor(T).instrumental).toBe(true)
    await ins.setGen(T, { voice: VOICE })
    expect(ins.genFor(T)).toMatchObject({ specs: [S1], instrumental: true, voice: VOICE })
  })

  it('specs заменяются целиком, не дописываются', async () => {
    const ins = await load({})
    await ins.setGen(T, { specs: [S1] })
    await ins.setGen(T, { specs: [S2] })
    expect(ins.genFor(T).specs).toEqual([S2])
  })

  it('«без голоса» снимается повторным setGen instrumental false; прочие поля остаются', async () => {
    const ins = await load({})
    await ins.setGen(T, { specs: [S1], instrumental: true })
    await ins.setGen(T, { instrumental: false })
    expect(ins.genFor(T).instrumental || false).toBe(false)
    expect(ins.genFor(T).specs).toEqual([S1])
  })

  it('setGen → draftPut трека со state.gen = genFor; правки трека в черновике на месте', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-11]), WAIT)
    await ins.setGen(T, { specs: [S1], voice: VOICE })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    const st = lastPut(T)[1]
    expect(st.gen).toMatchObject({ specs: [S1], voice: VOICE })
    expect(JSON.parse(JSON.stringify(st.gen))).toEqual(JSON.parse(JSON.stringify(ins.genFor(T))))
    expect(ids(st.edits)).toEqual([-11])
  })

  it('gen стал пустым, правок нет → gen удалён, черновик трека удаляется на воркере', async () => {
    const ins = await load({})
    await ins.setGen(T, { specs: [S1] })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    await ins.setGen(T, { specs: [] })
    expect(genEmpty(ins.genFor(T))).toBe(true)
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
  })

  it('gen стал пустым, правки есть → черновик остаётся, в нём нет gen', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-11]), WAIT)
    await ins.setGen(T, { instrumental: true })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    const before = putsFor(T).length
    await ins.setGen(T, { instrumental: false })
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(before), WAIT)
    expect(genEmpty(lastPut(T)[1].gen)).toBe(true)
    expect(ids(lastPut(T)[1].edits)).toEqual([-11])
    expect(deletesFor(T)).toEqual([])
  })

  it('gen из черновика воркера при старте → genFor его отдаёт, обратно не отправляется', async () => {
    const ins = await load({ drafts: [remote(T, [], { gen: GEN })] })
    await vi.waitFor(() => expect(ins.genFor(T)).toMatchObject({ specs: [S1], voice: VOICE }), WAIT)
    expect(putsFor(T)).toEqual([])
  })

  it('gen хранится на ПК: воркер недоступен → после перезапуска gen на месте', async () => {
    const ins = await load({ drafts: new Error('offline') })
    await ins.setGen(T, { specs: [S1], instrumental: true })
    const again = await restart({ drafts: new Error('offline') })
    expect(again.genFor(T)).toMatchObject({ specs: [S1], instrumental: true })
  })

  it('setGen трека 7 не трогает трек 8', async () => {
    const ins = await load({ drafts: [remote(OTHER, [], { gen: { specs: [S2], voice: null, instrumental: false } })] })
    await vi.waitFor(() => expect(ins.genFor(OTHER).specs).toEqual([S2]), WAIT)
    await ins.setGen(T, { instrumental: true })
    expect(ins.genFor(OTHER).specs).toEqual([S2])
    expect(ins.genFor(OTHER).instrumental || false).toBe(false)
    await vi.waitFor(() => expect(putsFor(T).length).toBeGreaterThan(0), WAIT)
    expect(putsFor(OTHER)).toEqual([])
  })

  it('«сбросить черновик» (clearDraft) снимает и gen трека', async () => {
    const ins = await load({ drafts: [remote(T, [E1], { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    await ins.clearDraft(T)
    expect(genEmpty(ins.genFor(T))).toBe(true)
    await vi.waitFor(() => expect(deletesFor(T).length).toBeGreaterThan(0), WAIT)
  })
})

describe('ТК4: applyAll с gen — один рендер, потом правки на новой версии', () => {
  async function startApply({ edits = [E1, E2], applyEdits } = {}) {
    const ins = await load({ drafts: [remote(T, edits, { gen: GEN })], applyEdits })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    await vi.waitFor(() => expect(ins.appliedFor(T)?.length || 0).toBe(edits.length), WAIT)
    setJob(NEW, 'queued')
    const p = attempt(() => ins.applyAll(T, TITLE, { request: REQUEST }))
    await vi.waitFor(() => expect(apiMock.submit).toHaveBeenCalled(), WAIT)
    await p
    return ins
  }

  it('api.submit один раз с телом от студии; ход в черновике трека (applying.job_id); applyEdits пока не зовётся', async () => {
    const ins = await startApply()
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    expect(apiMock.submit.mock.calls[0][0]).toEqual(REQUEST)
    expect(ins.applyingFor(T)).toMatchObject({ job_id: NEW, title: TITLE })
    await vi.waitFor(() => expect(lastPut(T)?.[1]?.applying?.job_id).toBe(NEW), WAIT)
    expect(apiMock.applyEdits).not.toHaveBeenCalled()
    expect(apiMock.renameJob).not.toHaveBeenCalled()
  })

  it('пока рендер идёт: правки, gen и повторное «применить всё» трека 7 отклоняются; реестр не меняется', async () => {
    const ins = await startApply()
    await expect(attempt(() => ins.addStemFx(T, fx()))).rejects.toThrow()
    await expect(attempt(() => ins.remove(T, -11))).rejects.toThrow()
    await expect(attempt(() => ins.setGen(T, { instrumental: true }))).rejects.toThrow()
    await expect(attempt(() => ins.applyAll(T, TITLE, { request: REQUEST }))).rejects.toThrow()
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    expect(ids(ins.appliedFor(T))).toEqual([-12, -11])
    expect(ins.genFor(T)).toMatchObject({ specs: [S1], voice: VOICE })
    expect(ins.genFor(T).instrumental || false).toBe(false)
  })

  it('пока рендер трека 7 идёт, трек 8 правится', async () => {
    const ins = await startApply()
    await ins.addStemFx(OTHER, fx())
    expect(ins.appliedFor(OTHER).some(x => x.chain === 'soften' && x.from === 176)).toBe(true)
  })

  it('рендер готов → правки трека 7 на новой версии, у 7 правки и gen очищены, applying снят; затем applyEdits(новая, спеки, title)', async () => {
    let finish
    const applyEdits = vi.fn(() => new Promise((resolve) => { finish = resolve }))
    const ins = await startApply({ applyEdits })
    // пока джоба в очереди — ничего не переносится
    await vi.advanceTimersByTimeAsync(10_000)
    expect(applyEdits).not.toHaveBeenCalled()
    expect(ids(ins.appliedFor(T))).toEqual([-12, -11])

    setJob(NEW, 'done')
    await until(() => expect(applyEdits).toHaveBeenCalled())
    // правки перенесены на новую версию (пока applyEdits не завершён)
    expect(chains(ins.appliedFor(NEW))).toEqual(['dewhistle', 'soften'])
    expect(ins.appliedFor(NEW).find(x => x.chain === 'dewhistle')).toMatchObject({ from: 10, to: 20, params: { freqs: [2638] } })
    expect(ins.appliedFor(T) || []).toEqual([])
    expect(genEmpty(ins.genFor(T))).toBe(true)
    expect(ins.applyingFor(T) || null).toBe(null)

    const [parent, specs, title] = applyEdits.mock.calls[0]
    expect(Number(parent)).toBe(NEW)
    expect(specs.map(s => s.chain).sort()).toEqual(['dewhistle', 'soften'])
    expect(title).toBe(TITLE)
    expect(apiMock.renameJob).not.toHaveBeenCalled()
    expect(apiMock.submit).toHaveBeenCalledTimes(1)

    finish(501)
    await until(() => expect(deletesFor(T).length).toBeGreaterThan(0))
  })

  it('правок у трека нет (только gen) → новая версия и есть итог: renameJob(новая, title), applyEdits не зовётся', async () => {
    const ins = await startApply({ edits: [] })
    setJob(NEW, 'done')
    await until(() => expect(apiMock.renameJob).toHaveBeenCalled())
    const [id, title] = apiMock.renameJob.mock.calls[0]
    expect(Number(id)).toBe(NEW)
    expect(title).toBe(TITLE)
    expect(apiMock.applyEdits).not.toHaveBeenCalled()
    expect(genEmpty(ins.genFor(T))).toBe(true)
    expect(ins.applyingFor(T) || null).toBe(null)
  })

  for (const status of ['error', 'canceled']) {
    it(`рендер ${status} → applying снят (правки снова принимаются), gen и правки целы, причина доступна`, async () => {
      const ins = await startApply()
      setJob(NEW, status, status === 'error' ? { error: 'CUDA out of memory' } : {})
      await until(() => expect(ins.applyingFor(T)?.error).toBeTruthy())
      if (status === 'error') expect(String(ins.applyingFor(T).error)).toContain('CUDA out of memory')
      expect(ins.genFor(T)).toMatchObject({ specs: [S1], voice: VOICE })
      expect(ids(ins.appliedFor(T))).toEqual([-12, -11])
      expect(apiMock.applyEdits).not.toHaveBeenCalled()
      expect(apiMock.renameJob).not.toHaveBeenCalled()
      // трек больше не заблокирован
      await ins.addStemFx(T, fx())
      expect(ins.appliedFor(T).some(x => x.from === 176)).toBe(true)
      // в черновике воркера хода больше нет, gen остался
      await vi.waitFor(() => expect(lastPut(T)[1].applying?.job_id).not.toBe(NEW), WAIT)
      expect(lastPut(T)[1].gen).toMatchObject({ specs: [S1] })
    })
  }

  it('без gen — как этап 2: applyEdits(трек 7) сразу, рендер не запускается', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-11]), WAIT)
    const id = await ins.applyAll(T, TITLE)
    expect(id).toBe(501)
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(Number(apiMock.applyEdits.mock.calls[0][0])).toBe(T)
    expect(ins.applyingFor(T) || null).toBe(null)
  })

  it('ошибка api.submit → наружу, applying не ставится, gen и правки целы', async () => {
    const err = new Error('queue full')
    const ins = await load({ drafts: [remote(T, [E1], { gen: GEN })], submit: vi.fn(async () => { throw err }) })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    await expect(attempt(() => ins.applyAll(T, TITLE, { request: REQUEST }))).rejects.toBe(err)
    expect(ins.applyingFor(T)?.job_id).toBeUndefined()
    expect(ins.genFor(T)).toMatchObject({ specs: [S1] })
    expect(ids(ins.appliedFor(T))).toEqual([-11])
    await ins.addStemFx(T, fx())   // трек не заблокирован
  })
})

describe('ТК5: перезапуск во время «применить всё» — ждёт ту же джобу', () => {
  // первый запуск: «применить всё» отправило рендер 901, состояние черновика ушло на воркер
  async function firstRun(edits = [E1]) {
    const ins = await load({ drafts: [remote(T, edits, { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    await vi.waitFor(() => expect(lastPut(T)?.[1]?.applying?.job_id).toBe(NEW), WAIT)
    return JSON.parse(JSON.stringify(lastPut(T)[1]))
  }

  it('тот же localStorage + черновик воркера с applying → submit не повторяется, трек заблокирован, по готовности — applyEdits(901)', async () => {
    const state = await firstRun()
    const ins = await restart({ drafts: [{ job_id: T, state }] })
    expect(ins.applyingFor(T)).toMatchObject({ job_id: NEW, title: TITLE })
    await expect(attempt(() => ins.addStemFx(T, fx()))).rejects.toThrow()
    await vi.advanceTimersByTimeAsync(10_000)
    expect(apiMock.submit).not.toHaveBeenCalled()

    setJob(NEW, 'done')
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    expect(Number(apiMock.applyEdits.mock.calls[0][0])).toBe(NEW)
    expect(apiMock.applyEdits.mock.calls[0][2]).toBe(TITLE)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('другой ПК (пустой localStorage), черновик воркера с applying → продолжает ждать ту же джобу', async () => {
    const state = await firstRun([])
    const ins = await load({ drafts: [{ job_id: T, state }] })
    await vi.waitFor(() => expect(ins.applyingFor(T)?.job_id).toBe(NEW), WAIT)
    setJob(NEW, 'done')
    await until(() => expect(apiMock.renameJob).toHaveBeenCalled())
    expect(Number(apiMock.renameJob.mock.calls[0][0])).toBe(NEW)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('рендер успел закончиться, пока приложение было закрыто → после перезапуска доводится без нового submit', async () => {
    const state = await firstRun()
    setJob(NEW, 'done')
    await restart({ drafts: [{ job_id: T, state }] })
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    expect(Number(apiMock.applyEdits.mock.calls[0][0])).toBe(NEW)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })
})

describe('ТК6: ожидающий рендер куска переезжает на новую версию; «применить всё» ждёт его', () => {
  const PIECE = 3   // джоба рендера куска трека 7
  const QSPEC = { childId: PIECE, instId: 'i-3', from: 5, to: 15, lead: 0.1, beat: 0.5, db: -6, srcJob: T }
  const pendingOf = (ins) => (ins.pending && 'value' in ins.pending ? ins.pending.value : ins.pending) || []

  it('рендер 901 готов → правки и кусок на 901; applyEdits ждёт кусок; кусок готов → вклейка в 901 и applyEdits(901, …с вклейкой, title)', async () => {
    let finish
    const applyEdits = vi.fn(() => new Promise((resolve) => { finish = resolve }))   // висит, пока тест не отпустит
    const ins = await load({ applyEdits, drafts: [{ job_id: T, state: { v: 1, edits: [E1], queue: [QSPEC], ts: 1000, gen: GEN } }] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    await vi.waitFor(() => expect(pendingOf(ins).filter(s => Number(s.parent) === T).map(s => s.childId)).toEqual([PIECE]), WAIT)
    setJob(PIECE, 'queued')
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    expect(apiMock.submit).toHaveBeenCalledTimes(1)

    setJob(NEW, 'done')
    await until(() => expect(chains(ins.appliedFor(NEW))).toEqual(['soften']))
    // кусок — в очереди новой версии, у трека 7 ничего не осталось
    await until(() => expect(pendingOf(ins).filter(s => Number(s.parent) === NEW).map(s => s.childId)).toEqual([PIECE]))
    expect(pendingOf(ins).filter(s => Number(s.parent) === T)).toEqual([])
    expect(ins.appliedFor(T) || []).toEqual([])
    expect(genEmpty(ins.genFor(T))).toBe(true)

    // кусок ещё рендерится — применять рано
    await vi.advanceTimersByTimeAsync(15_000)
    expect(apiMock.applyEdits).not.toHaveBeenCalled()
    expect(apiMock.renameJob).not.toHaveBeenCalled()

    setJob(PIECE, 'done')
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    expect(ids(ins.appliedFor(NEW))).toContain(PIECE)   // вклейка куска в реестре новой версии
    const [parent, specs, title] = apiMock.applyEdits.mock.calls[0]
    expect(Number(parent)).toBe(NEW)
    expect(title).toBe(TITLE)
    expect(specs.some(s => Number(s.child_id) === PIECE)).toBe(true)
    expect(specs.some(s => s.chain === 'soften')).toBe(true)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    finish(501)
    await vi.advanceTimersByTimeAsync(5_000)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
  })
})

describe('ТК6: «сбросить черновик» после ошибки рендера убирает и причину', () => {
  it('рендер error → applyingFor(7).error; clearDraft(7) → applyingFor(7) null, gen пуст', async () => {
    const ins = await load({ drafts: [remote(T, [E1], { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    setJob(NEW, 'error', { error: 'CUDA out of memory' })
    await until(() => expect(ins.applyingFor(T)?.error).toBeTruthy())
    await ins.clearDraft(T)
    expect(ins.applyingFor(T) ?? null).toBe(null)
    expect(genEmpty(ins.genFor(T))).toBe(true)
  })
})

describe('ТК7: шаг итога (applyEdits новой версии) — ошибка и перезапуск', () => {
  // черновики воркера, как их оставили draftPut/draftDelete модуля (по порядку вызовов)
  function workerDrafts() {
    const ops = [
      ...apiMock.draftPut.mock.calls.map((c, i) => [apiMock.draftPut.mock.invocationCallOrder[i], 'put', c]),
      ...apiMock.draftDelete.mock.calls.map((c, i) => [apiMock.draftDelete.mock.invocationCallOrder[i], 'del', c]),
    ].sort((a, b) => a[0] - b[0])
    const store = new Map()
    for (const [, op, c] of ops) {
      if (op === 'put') store.set(Number(c[0]), JSON.parse(JSON.stringify(c[1])))
      else store.delete(Number(c[0]))
    }
    return [...store].map(([job_id, state]) => ({ job_id, state }))
  }

  async function toFinalize(applyEdits) {
    const ins = await load({ applyEdits, drafts: [remote(T, [E1, E2], { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    await vi.waitFor(() => expect(ids(ins.appliedFor(T))).toEqual([-12, -11]), WAIT)
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    setJob(NEW, 'done')
    await until(() => expect(applyEdits).toHaveBeenCalled())
    return ins
  }

  it('applyEdits(901) падает → applyingFor(901) с причиной, правки 901 на месте (итог можно собрать вручную)', async () => {
    const applyEdits = vi.fn(async () => { throw new Error('rebuild failed: disk full') })
    const ins = await toFinalize(applyEdits)
    await until(() => expect(ins.applyingFor(NEW)?.error).toBeTruthy())
    expect(String(ins.applyingFor(NEW).error)).toContain('disk full')
    expect(chains(ins.appliedFor(NEW))).toEqual(['dewhistle', 'soften'])
    expect(apiMock.renameJob).not.toHaveBeenCalled()
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
  })

  it('перезапуск, пока applyEdits(901) не завершён → итог 901 доводится заново, submit не повторяется', async () => {
    const applyEdits = vi.fn(() => new Promise(() => {}))   // приложение закрыли посреди шага итога
    await toFinalize(applyEdits)
    await vi.waitFor(() => expect(lastPut(NEW)?.[1]?.applying).toMatchObject({ finalize: true, title: TITLE }), WAIT)
    const drafts = workerDrafts()

    const ins = await restart({ drafts })
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    const [parent, specs, title] = apiMock.applyEdits.mock.calls[0]
    expect(Number(parent)).toBe(NEW)
    expect(specs.map(s => s.chain).sort()).toEqual(['dewhistle', 'soften'])
    expect(title).toBe(TITLE)
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(ins.applyingFor(T) ?? null).toBe(null)
  })
})

describe('ТК8: новая версия ждёт итога — правки человека отклоняются, внутренняя вклейка идёт', () => {
  const PIECE = 3
  const QSPEC = { childId: PIECE, instId: 'i-3', from: 5, to: 15, lead: 0.1, beat: 0.5, db: -6, srcJob: T }

  it('setGen/addStemFx(901) отклонены; кусок done → вклейка в 901; applyEdits(901) один раз', async () => {
    const applyEdits = vi.fn(() => new Promise(() => {}))   // итог висит — смотрим реестр 901 до его конца
    const ins = await load({ applyEdits, drafts: [{ job_id: T, state: { v: 1, edits: [E1], queue: [QSPEC], ts: 1000, gen: GEN } }] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(PIECE, 'queued')
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    setJob(NEW, 'done')
    await until(() => expect(chains(ins.appliedFor(NEW))).toEqual(['soften']))

    await expect(attempt(() => ins.setGen(NEW, { voice: VOICE }))).rejects.toThrow()
    await expect(attempt(() => ins.addStemFx(NEW, fx()))).rejects.toThrow()
    expect(genEmpty(ins.genFor(NEW))).toBe(true)
    expect(ins.appliedFor(NEW).some(x => x.from === 176)).toBe(false)
    expect(applyEdits).not.toHaveBeenCalled()

    setJob(PIECE, 'done')
    await until(() => expect(ids(ins.appliedFor(NEW))).toContain(PIECE))
    await until(() => expect(applyEdits).toHaveBeenCalled())
    await vi.advanceTimersByTimeAsync(20_000)
    expect(applyEdits).toHaveBeenCalledTimes(1)
    expect(Number(applyEdits.mock.calls[0][0])).toBe(NEW)
    expect(applyEdits.mock.calls[0][1].some(s => Number(s.child_id) === PIECE)).toBe(true)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
  })
})

describe('ТК9–ТК10: перезапуск, рендер 901 уже готов', () => {
  // первый запуск: «применить всё» отправило рендер; на воркере — черновик 7 с applying
  async function firstRun() {
    const ins = await load({ drafts: [remote(T, [E1, E2], { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    await vi.waitFor(() => expect(lastPut(T)?.[1]?.applying?.job_id).toBe(NEW), WAIT)
    const old = JSON.parse(JSON.stringify(lastPut(T)[1]))
    setJob(NEW, 'done')   // рендер закончился, пока приложение было закрыто
    return old
  }

  it('ТК9: drafts() отвечает с задержкой старым черновиком 7 с тем же applying → applyEdits(901) ровно один раз', async () => {
    const old = await firstRun()
    let answer
    const ins = await restart({ drafts: () => new Promise((resolve) => { answer = resolve }) })
    await vi.advanceTimersByTimeAsync(10_000)   // несколько тиков без ответа воркера
    expect(apiMock.applyEdits).not.toHaveBeenCalled()

    answer([{ job_id: T, state: old }])
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    await vi.advanceTimersByTimeAsync(30_000)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
    expect(Number(apiMock.applyEdits.mock.calls[0][0])).toBe(NEW)
    expect(apiMock.applyEdits.mock.calls[0][2]).toBe(TITLE)
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(ins.applyingFor(T) ?? null).toBe(null)
  })

  it('ТК10: drafts() бросает «404» (старый воркер) → ход по localStorage доводится: applyEdits(901) один раз', async () => {
    await firstRun()
    await restart({ drafts: new Error('404 page not found') })
    await until(() => expect(apiMock.applyEdits).toHaveBeenCalled())
    await vi.advanceTimersByTimeAsync(30_000)
    expect(apiMock.applyEdits).toHaveBeenCalledTimes(1)
    expect(Number(apiMock.applyEdits.mock.calls[0][0])).toBe(NEW)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })
})

describe('ТК11: «сбросить черновик» снимает застрявший итог новой версии', () => {
  const PIECE = 3
  const QSPEC = { childId: PIECE, instId: 'i-3', from: 5, to: 15, lead: 0.1, beat: 0.5, db: -6, srcJob: T }
  const pendingOf = (ins) => (ins.pending && 'value' in ins.pending ? ins.pending.value : ins.pending) || []

  it('пересборка куска 901 всё время падает → итога нет; clearDraft(901) проходит, ход и черновик 901 сняты, правки снова принимаются', async () => {
    const rebuildSections = vi.fn(async () => { throw new Error('rebuild sections failed') })
    const ins = await load({ rebuildSections, drafts: [{ job_id: T, state: { v: 1, edits: [E1], queue: [QSPEC], ts: 1000, gen: GEN } }] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(PIECE, 'queued')
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    setJob(NEW, 'done')
    await until(() => expect(chains(ins.appliedFor(NEW))).toEqual(['soften']))
    await until(() => expect(ins.applyingFor(NEW)).toMatchObject({ finalize: true, title: TITLE }))

    setJob(PIECE, 'done')
    await until(() => expect(rebuildSections).toHaveBeenCalled())
    await vi.advanceTimersByTimeAsync(20_000)
    expect(apiMock.applyEdits).not.toHaveBeenCalled()

    await ins.clearDraft(NEW)
    expect(ins.applyingFor(NEW) ?? null).toBe(null)
    expect(ins.appliedFor(NEW) || []).toEqual([])
    expect(pendingOf(ins).filter(s => Number(s.parent) === NEW)).toEqual([])
    await vi.waitFor(() => expect(deletesFor(NEW).length).toBeGreaterThan(0), WAIT)

    // воркер снова собирает куски (сбой пересборки был условием застревания, а не предметом проверки);
    // правка трека 901 принимается — блокировки «применить всё» больше нет
    rebuildSections.mockImplementation(async (_p, specs) => report(specs))
    const err = await attempt(() => ins.addStemFx(NEW, fx())).then(() => null, (e) => e)
    expect(String(err && err.message)).not.toMatch(/применить всё/i)
    expect(ins.appliedFor(NEW).some(x => x.chain === 'soften' && x.from === 176)).toBe(true)
  })

  it('контроль: пока у трека 7 идёт рендер, clearDraft(7) отклоняется, ход и gen на месте', async () => {
    const ins = await load({ drafts: [remote(T, [E1], { gen: GEN })] })
    await vi.waitFor(() => expect(genEmpty(ins.genFor(T))).toBe(false), WAIT)
    setJob(NEW, 'queued')
    await ins.applyAll(T, TITLE, { request: REQUEST })
    await expect(attempt(() => ins.clearDraft(T))).rejects.toThrow()
    expect(ins.applyingFor(T)).toMatchObject({ job_id: NEW })
    expect(ins.genFor(T)).toMatchObject({ specs: [S1] })
    expect(ids(ins.appliedFor(T))).toEqual([-11])
  })
})
