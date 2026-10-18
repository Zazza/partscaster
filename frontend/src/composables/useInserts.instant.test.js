// «Применить» только пишет черновик; сборка — при «▶ микс» (карточка internal-studio-instant-apply,
// условия 1, 2, 4, 5, 6, 8, 9; тест-кейсы ТК1, ТК2, ТК3, ТК4 (useInserts.putPreset)).
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
const OTHER = 8
const clone = (x) => JSON.parse(JSON.stringify(x))
const flush = async () => { for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0) }

// записи черновика в форме useInserts
const base = { from: 0, to: 0, lead: 0, beat: 0, db: 0, fadeIn: 0, fadeOut: 0, keepHighHz: 0 }
const fxRec = (childId, over = {}) => ({ ...base, childId, instId: 'fx-soften', stems: ['vocals'],
  chain: 'soften', params: { strength: 0.6 }, ...over })
const E1 = fxRec(-11)
const E2 = fxRec(-12, { instId: 'fx-dewhistle', from: 10, to: 20, stems: ['drums'], chain: 'dewhistle', params: { freqs: [2638] } })
const A = { childId: 1, instId: 'i-a', from: 0, to: 10, lead: 0, beat: 0.5, db: -6, alts: [9] }
const ENG = { ...base, childId: -21, instId: 'engine', from: 20, to: 35, stems: ['other'],
  engine: [{ type: 'amp', model: 'JCM2000.nam', input_db: -6 }], label: 'Гитара через JCM2000' }
const ADD = { ...base, childId: -22, instId: 'engine', stems: ['mix'], add: true,
  engine: [{ type: 'synth', wave: 'saw' }], label: 'Синт', place: { pan: 0, width: 1 } }

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
const never = () => vi.fn(() => new Promise(() => {}))

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

// промис разрешился без ответа пересборки
async function settles(p) {
  let done = false
  let err = null
  Promise.resolve(p).then(() => { done = true }, (e) => { done = true; err = e })
  await flush()
  return { done, err }
}

afterEach(() => {
  vi.clearAllTimers()
  vi.useRealTimers()
})

// ---------- ТК1: каждая функция записи — только черновик ----------

const PEDALS = [{ chain: 'od-ts', params: { drive: 12 } }, { chain: 'reverb-hall', params: { wet: 0.3 } }]
const ENV = [{ t: 1, db: 0 }, { t: 3, db: -12 }]
const MASTER = [{ type: 'glue', threshold_db: -18 }, { type: 'limiter', target_lufs: -14 }]

const writes = [
  ['addStemFx', (ins) => ins.addStemFx(T, { stem: 'vocals', chain: 'reverb', params: { mix: 0.3 }, from: 2, to: 4 }),
    (list) => list.some(x => x.childId < 0 && x.chain === 'reverb' && x.stems[0] === 'vocals')],
  ['addStemFx голоса (step voice)', (ins) => ins.addStemFx(T, { stem: 'vocals', chain: 'soften', params: { strength: 0.3 }, step: 'voice' }),
    (list) => list.some(x => x.step === 'voice' && x.chain === 'soften' && x.params.strength === 0.3)],
  ['addStemPedals', (ins) => ins.addStemPedals(T, { stem: 'other', steps: PEDALS, label: 'Педали' }),
    (list) => list.some(x => x.stems?.[0] === 'other' && JSON.stringify(x.steps) === JSON.stringify(PEDALS))],
  ['addStemEngine (с набором, которого может не быть)', (ins) => ins.addStemEngine(T, { stem: 'bass', chain: [{ type: 'bass', kit: 'growlybass/bass' }], label: 'Бас' }),
    (list) => list.some(x => x.instId === 'engine' && x.stems[0] === 'bass' && x.engine?.[0]?.kit === 'growlybass/bass')],
  ['addStemEngine синт (add)', (ins) => ins.addStemEngine(T, { stem: 'mix', chain: [{ type: 'synth', kit: 'pianokit/piano' }], add: true, label: 'Синт' }),
    (list) => list.some(x => x.add === true && x.engine?.[0]?.type === 'synth')],
  ['addStemEngines (ритм-секция)', (ins) => ins.addStemEngines(T, [
    { stem: 'kick', chain: [{ type: 'sampler', kit: 'osdk/kick' }], label: 'Бочка' },
    { stem: 'snare', chain: [{ type: 'sampler', kit: 'osdk/snare' }], label: 'Малый' },
  ]), (list) => ['kick', 'snare'].every(s => list.some(x => x.instId === 'engine' && x.stems[0] === s))],
  ['addStemEnvelope', (ins) => ins.addStemEnvelope(T, { stem: 'bass', envelope: ENV }),
    (list) => list.some(x => x.stems?.[0] === 'bass' && JSON.stringify(x.envelope) === JSON.stringify(ENV))],
  ['addMixFx (мастеринг одним кликом)', (ins) => ins.addMixFx(T, { chain: 'master', params: { amount: 0.5 }, step: 'done', label: 'Мастеринг' }),
    (list) => list.some(x => x.mix === true && x.chain === 'master')],
  ['addMixPedals', (ins) => ins.addMixPedals(T, { steps: PEDALS, label: 'Педали на трек' }),
    (list) => list.some(x => x.mix === true && JSON.stringify(x.steps) === JSON.stringify(PEDALS))],
  ['setMixEnvelope', (ins) => ins.setMixEnvelope(T, ENV),
    (list) => list.some(x => x.mix === true && JSON.stringify(x.envelope) === JSON.stringify(ENV))],
  ['setStemPlace', (ins) => ins.setStemPlace(T, 'other', { pan: -0.5, width: 1.2 }),
    (list) => list.some(x => x.stems?.[0] === 'other' && x.place?.pan === -0.5 && !x.engine)],
  ['setPlace', (ins) => ins.setPlace(T, -22, { pan: 0.7, width: 1 }),
    (list) => list.find(x => x.childId === -22)?.place?.pan === 0.7],
  ['setMaster', (ins) => ins.setMaster(T, { chain: MASTER, label: 'Мой мастер' }),
    (list) => list.some(x => x.master === true && JSON.stringify(x.engine) === JSON.stringify(MASTER))],
  ['addMute («убрать»)', (ins) => ins.addMute(T, { instId: 'mute-drums', from: 5, to: 9, stems: ['drums'], db: -100 }),
    (list) => list.some(x => x.childId < 0 && x.db === -100 && x.from === 5)],
  ['addMutes (куплеты реже)', (ins) => ins.addMutes(T, [
    { instId: 'm', from: 0, to: 4, stems: ['other'], db: -100 },
    { instId: 'm', from: 8, to: 12, stems: ['other'], db: -100 },
  ]), (list) => list.filter(x => x.db === -100 && x.stems?.[0] === 'other').length === 2],
  ['setDb', (ins) => ins.setDb(T, 1, -12),
    (list) => list.find(x => x.childId === 1)?.db === -12],
  ['setOff', (ins) => ins.setOff(T, -11, true),
    (list) => list.find(x => x.childId === -11)?.off === true],
  ['remove', (ins) => ins.remove(T, -12),
    (list) => !list.some(x => x.childId === -12)],
  ['selectAlt', (ins) => ins.selectAlt(T, 1, 9),
    (list) => list.some(x => x.childId === 9) && !list.some(x => x.childId === 1)],
  ['replaceEngine', (ins) => ins.replaceEngine(T, -21, { chain: [{ type: 'reverb', wet: 0.3 }], label: 'Реверб' }),
    (list) => list.find(x => x.childId === -21)?.engine?.[0]?.type === 'reverb'],
  ['putPreset', (ins) => ins.putPreset(T, clone(RECIPE), INFO),
    (list) => list.some(x => x.preset && x.preset.id === 71 && x.preset.recipe)],
]

describe('ТК1: «Применить» и правки в списке только пишут черновик', () => {
  for (const [name, write, check] of writes) {
    it(`${name}: запись в черновике, пересборки и запросов к воркеру нет, промис разрешён сразу`, async () => {
      const ins = await load({ drafts: [remote(T, [E1, E2, A, ENG, ADD])], rebuildSections: never() })
      const { done, err } = await settles(write(ins))
      expect(err).toBeNull()
      expect(done).toBe(true)
      expect(check(ins.appliedFor(T))).toBe(true)
      await flush()
      expect(apiMock.rebuildSections).not.toHaveBeenCalled()
      expect(mock.state.stray).toEqual([])
    })
  }

  it('соседний трек записью не трогается', async () => {
    const ins = await load({ drafts: [remote(T, [E1]), remote(OTHER, [E2])], rebuildSections: never() })
    await settles(ins.addStemFx(T, { stem: 'vocals', chain: 'reverb', params: { mix: 0.3 } }))
    expect(clone(ins.appliedFor(OTHER))).toEqual([E2])
  })
})

// ---------- ТК4 (useInserts): пресет звука — одна запись с рецептом ----------

describe('ТК4: putPreset — рецепт в черновике без подготовки', () => {
  it('запись: preset {id, name, recipe = пресет как передан}; прочие записи на месте; рецепт — копия', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: never() })
    const recipe = clone(RECIPE)
    await ins.putPreset(T, recipe, INFO)
    recipe.specs[0].db = 5
    const list = ins.appliedFor(T)
    const presets = list.filter(x => x.preset)
    expect(presets.length).toBe(1)
    expect(presets[0].preset).toMatchObject({ id: 71, name: 'Пост-панк' })
    expect(clone(presets[0].preset.recipe)).toEqual(RECIPE)
    expect(clone(list.filter(x => !x.preset))).toEqual([E1, E2])
  })

  it('второй пресет заменяет первый: в черновике один пресет', async () => {
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: never() })
    await ins.putPreset(T, clone(RECIPE), INFO)
    await ins.putPreset(T, { ...clone(RECIPE), id: 72, name: 'Гранж' }, { id: 72, name: 'Гранж' })
    const presets = ins.appliedFor(T).filter(x => x.preset)
    expect(presets.length).toBe(1)
    expect(presets[0].preset.id).toBe(72)
    expect(ins.appliedFor(T).some(x => x.childId === -11)).toBe(true)
  })

  it('старая запись пресета (sections) тоже заменяется новой', async () => {
    const OLD = { ...base, childId: -50, instId: 'preset', stems: [], step: 'done', label: 'Старый',
      preset: { id: 5, name: 'Старый', sections: [{ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: -3,
        stems: ['other'], fade_in: 0, fade_out: 0, keep_high_hz: 0 }], final: [], master: [], warnings: [] } }
    const ins = await load({ drafts: [remote(T, [OLD, E1])], rebuildSections: never() })
    await ins.putPreset(T, clone(RECIPE), INFO)
    const presets = ins.appliedFor(T).filter(x => x.preset)
    expect(presets.length).toBe(1)
    expect(presets[0].preset.id).toBe(71)
  })

  it('при сборке запись-пресет уходит одной спецификацией с рецептом и id записи', async () => {
    const ins = await load({ drafts: [remote(T, [])] })
    await ins.putPreset(T, clone(RECIPE), INFO)
    await ins.ensureMix(T)
    const [, specs] = callsFor(T).at(-1)
    const rec = ins.appliedFor(T).find(x => x.preset)
    const p = specs.filter(s => s.preset)
    expect(p.length).toBe(1)
    expect(p[0].id).toBe(String(rec.childId))
    expect(p[0].preset.preset).toEqual(RECIPE)
    expect(p[0]).toMatchObject({ child_id: 0, stems: [] })
  })
})

// ---------- ТК2: «▶ микс» — сборка только если черновик менялся ----------

describe('ТК2: ensureMix / mixStale', () => {
  it('черновик ещё не собирался → одна пересборка всеми записями (id у каждой), ответ — её результат', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    expect(ins.mixStale(T)).toBe(true)
    const r = await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(1)
    const [, specs] = callsFor(T)[0]
    expect(specIds(specs)).toEqual(['-11', '-12'])
    expect(r.variant.file).toBe('listen-1.flac')
    expect(ins.mixStale(T)).toBe(false)
  })

  it('не менялся после удачной сборки → без вызова, {cached: true, file} последней сборки', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.ensureMix(T)
    const r = await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(1)
    expect(r).toMatchObject({ cached: true, file: 'listen-1.flac' })
  })

  it('две правки подряд → одна сборка при «▶ микс», в ней обе', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.ensureMix(T)
    await ins.addStemFx(T, { stem: 'bass', chain: 'reverb', params: { mix: 0.2 } })
    await ins.setOff(T, -11, true)
    expect(ins.mixStale(T)).toBe(true)
    await flush()
    expect(callsFor(T).length).toBe(1)
    await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(2)
    const [, specs] = callsFor(T)[1]
    expect(specs.some(s => s.chain === 'reverb' && s.stems[0] === 'bass')).toBe(true)
    expect(specs.some(s => s.id === '-11')).toBe(false)
    expect(ins.mixStale(T)).toBe(false)
  })

  it('правка вернула черновик к собранному (выкл → вкл) → без новой сборки', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.ensureMix(T)
    await ins.setOff(T, -11, true)
    await ins.setOff(T, -11, false)
    expect(ins.mixStale(T)).toBe(false)
    const r = await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(1)
    expect(r.cached).toBe(true)
  })

  it('сборка упала → черновик всё ещё «не собран», следующий «▶ микс» собирает снова', async () => {
    const rs = rebuilds((n) => { if (n === 1) throw new Error('воркер недоступен') })
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: rs })
    await ins.ensureMix(T).catch(() => {})
    expect(ins.mixStale(T)).toBe(true)
    const r = await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(2)
    expect(r.variant.file).toBe('listen-2.flac')
    expect(ins.mixStale(T)).toBe(false)
  })

  it('пока собирается — правки принимаются сразу; после сборки черновик снова «не собран»', async () => {
    let release
    const hold = new Promise((r) => { release = r })
    const rs = rebuilds(async (n) => { if (n === 1) await hold; return {} })
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: rs })
    const play = ins.ensureMix(T)
    await flush()
    expect(callsFor(T).length).toBe(1)
    const { done } = await settles(ins.addStemFx(T, { stem: 'bass', chain: 'reverb', params: { mix: 0.2 } }))
    expect(done).toBe(true)
    expect(ins.appliedFor(T).some(x => x.chain === 'reverb')).toBe(true)
    release()
    await play
    await flush()
    expect(ins.mixStale(T)).toBe(true)
    await ins.ensureMix(T)
    expect(callsFor(T).length).toBe(2)
    expect(callsFor(T)[1][1].some(s => s.chain === 'reverb')).toBe(true)
  })

  it('сборка трека 7 не делает «собранным» трек 8', async () => {
    const ins = await load({ drafts: [remote(T, [E1]), remote(OTHER, [E1])] })
    await ins.ensureMix(T)
    expect(ins.mixStale(OTHER)).toBe(true)
    await ins.ensureMix(OTHER)
    expect(callsFor(OTHER).length).toBe(1)
  })

  it('«применить всё» после «▶ микс» без правок — файл этой сборки (как сейчас)', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.ensureMix(T)
    await ins.applyAll(T, 'итог')
    const [parent, specs, title, mixFile] = apiMock.applyEdits.mock.calls.at(-1)
    expect(Number(parent)).toBe(T)
    expect(specs).toEqual(callsFor(T)[0][1])
    expect(title).toBe('итог')
    expect(mixFile).toBe('listen-1.flac')
  })

  it('«применить всё» после правки без «▶ микс» — файла нет (записи другие)', async () => {
    const ins = await load({ drafts: [remote(T, [E1, E2])] })
    await ins.ensureMix(T)
    await ins.setOff(T, -12, true)
    await ins.applyAll(T, 'итог')
    expect(apiMock.applyEdits.mock.calls.at(-1)[3]).toBe('')
  })
})

// ---------- ТК3: ошибка сборки с отметкой правки → красная строка; удачная сборка снимает ----------

describe('ТК3: отметка правки в ошибке сборки', () => {
  for (const [how, fail] of [
    ['строкой (как Wails)', () => 'движок: нет памяти {edit:-12}'],
    ['объектом Error', () => new Error('движок: нет памяти {edit:-12}')],
  ]) {
    it(`ошибка ${how} → у записи -12 error с причиной без отметки; у прочих — нет`, async () => {
      const rs = vi.fn(async () => { throw fail() })
      const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rs })
      await ins.ensureMix(T).catch(() => {})
      await flush()
      const e = recOf(ins, T, -12).error
      expect(typeof e).toBe('string')
      expect(e).toContain('движок: нет памяти')
      expect(e).not.toContain('{edit:')
      expect(recOf(ins, T, -11).error).toBeFalsy()
      expect(ins.mixStale(T)).toBe(true)
    })
  }

  it('удачная сборка снимает отметки со всех записей трека', async () => {
    const rs = rebuilds((n) => { if (n === 1) throw 'движок: нет памяти {edit:-12}' })
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rs })
    await ins.ensureMix(T).catch(() => {})
    await flush()
    expect(recOf(ins, T, -12).error).toBeTruthy()
    await ins.ensureMix(T)
    await flush()
    expect(recOf(ins, T, -12).error).toBeFalsy()
    expect(recOf(ins, T, -11).error).toBeFalsy()
  })

  it('отметка записи-пресета → error у пресета', async () => {
    const ins = await load({ drafts: [remote(T, [E1])] })
    await ins.putPreset(T, clone(RECIPE), INFO)
    const id = String(ins.appliedFor(T).find(x => x.preset).childId)
    apiMock.rebuildSections = vi.fn(async () => { throw `гитарист: нет памяти {edit:${id}}` })
    await ins.ensureMix(T).catch(() => {})
    await flush()
    const rec = ins.appliedFor(T).find(x => x.preset)
    expect(rec.error).toContain('гитарист: нет памяти')
    expect(rec.error).not.toContain('{edit:')
    expect(recOf(ins, T, -11).error).toBeFalsy()
  })

  it('ошибка без отметки → ни одна запись не красная', async () => {
    const rs = vi.fn(async () => { throw 'воркер недоступен' })
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rs })
    await ins.ensureMix(T).catch(() => {})
    await flush()
    expect(ins.appliedFor(T).some(x => x.error)).toBe(false)
  })

  it('отметка несуществующей записи → ни одна запись не красная', async () => {
    const rs = vi.fn(async () => { throw 'сбой {edit:-999}' })
    const ins = await load({ drafts: [remote(T, [E1, E2])], rebuildSections: rs })
    await ins.ensureMix(T).catch(() => {})
    await flush()
    expect(ins.appliedFor(T).some(x => x.error)).toBe(false)
  })

  it('ошибка трека 7 не красит записи трека 8 с тем же id', async () => {
    const rs = vi.fn(async (p) => { if (Number(p) === T) throw 'сбой {edit:-12}'; return { variant: { file: 'x.flac' }, inserts: [] } })
    const ins = await load({ drafts: [remote(T, [E1, E2]), remote(OTHER, [E1, E2])], rebuildSections: rs })
    await ins.ensureMix(T).catch(() => {})
    await flush()
    expect(recOf(ins, OTHER, -12).error).toBeFalsy()
  })
})

// ---------- ТК3/условие 9: предупреждения пресета — у строки пресета после сборки ----------

describe('условие 9: warnings записи-пресета', () => {
  it('удачная сборка с warnings → у записи-пресета те же warnings; у прочих — нет', async () => {
    const rs = rebuilds(() => ({ warnings: ['гитара YuE2 — у трека нет плана'] }))
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: rs })
    await ins.putPreset(T, clone(RECIPE), INFO)
    await ins.ensureMix(T)
    await flush()
    expect(ins.appliedFor(T).find(x => x.preset).warnings).toEqual(['гитара YuE2 — у трека нет плана'])
    expect(recOf(ins, T, -11).warnings || []).toEqual([])
  })

  it('следующая удачная сборка без warnings → предупреждений у пресета нет', async () => {
    const rs = rebuilds((n) => (n === 1 ? { warnings: ['нет плана'] } : {}))
    const ins = await load({ drafts: [remote(T, [E1])], rebuildSections: rs })
    await ins.putPreset(T, clone(RECIPE), INFO)
    await ins.ensureMix(T)
    await ins.setOff(T, -11, true)
    await ins.ensureMix(T)
    await flush()
    expect(ins.appliedFor(T).find(x => x.preset).warnings || []).toEqual([])
  })
})
