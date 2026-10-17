// Черновик работы над треком (internal-studio-draft, условия 4, 5, 8; ТК5–ТК8).
// Чистая логика без DOM: шаг записи реестра, группировка по шагам, слияние
// локальной копии с черновиками воркера, состояние черновика трека.
//
// Формы данных (как в реестре useInserts):
//   applied — { [parentId]: [записи] }, pending — [спеки с полем parent],
//   localTs — { [parentId]: мс } (ключ yue_insert_ts), remote — [{job_id, state}],
//   state черновика — { v: 1, edits, queue (без parent), ts }.
import { describe, it, expect } from 'vitest'
import { editStep, editsBySteps, mergeDrafts, draftState, isEmptyDraft, isEmptyGen } from './draftEdits.js'

// записи реестра — в форме, в которой их пишет useInserts
const base = { from: 4, to: 8, lead: 0, beat: 0, db: 0, stems: ['drums'], fadeIn: 0, fadeOut: 0, keepHighHz: 0 }
const insert = (id = 191) => ({ ...base, childId: id, instId: 'guitar', lead: 2, beat: 0.5 })
const mute = (id = -1001) => ({ ...base, childId: id, instId: 'drums', db: -100 })
const fx = (id = -1002) => ({ ...base, childId: id, instId: 'fx-reverb', chain: 'reverb', params: { mix: 0.3 } })
const pedals = (id = -1003) => ({ ...base, childId: id, instId: 'pedals', steps: [{ chain: 'drive' }], label: 'p' })
const engine = (id = -1004) => ({ ...base, childId: id, instId: 'engine', engine: [{ block: 'eq' }], label: 'e' })
const envelope = (id = -1005) => ({ ...base, childId: id, instId: 'env', from: 0, to: 0, envelope: [{ t: 0, db: -3 }] })
const place = (id = -1006) => ({ ...engine(id), add: true, place: { kit: 'k' } })
const master = (id = -1007) => ({ ...base, childId: id, instId: 'master', from: 0, to: 0, stems: [], master: true,
  engine: [{ block: 'limiter' }], label: 'm' })

const clone = (x) => JSON.parse(JSON.stringify(x))
const ids = (list) => list.map(Number).sort((a, b) => a - b)

describe('editStep (ТК5)', () => {
  it('мастер → done (даже с цепочкой движка)', () => {
    expect(editStep(master())).toBe('done')
  })

  it('вклейка рендера (childId > 0) → edit', () => {
    expect(editStep(insert())).toBe('edit')
  })

  it('«громкость дорожек» (childId < 0, db, без chain/steps/engine/envelope/place) → edit', () => {
    expect(editStep(mute())).toBe('edit')
    expect(editStep({ ...mute(), db: 6 })).toBe('edit')
  })

  it('chain / steps / engine / envelope / place → sound', () => {
    expect(editStep(fx())).toBe('sound')
    expect(editStep(pedals())).toBe('sound')
    expect(editStep(engine())).toBe('sound')
    expect(editStep(envelope())).toBe('sound')
    expect(editStep(place())).toBe('sound')
  })

  it('явный step побеждает вид записи', () => {
    expect(editStep({ ...fx(), step: 'voice' })).toBe('voice')
    expect(editStep({ ...insert(), step: 'sound' })).toBe('sound')
    expect(editStep({ ...master(), step: 'edit' })).toBe('edit')
    expect(editStep({ ...mute(), step: 'done' })).toBe('done')
  })

  it('выключенная запись — тот же шаг, что у включённой', () => {
    expect(editStep({ ...fx(), off: true })).toBe('sound')
    expect(editStep({ ...insert(), off: true })).toBe('edit')
  })

  it('не мутирует запись', () => {
    const it0 = fx()
    const snap = clone(it0)
    editStep(it0)
    expect(it0).toEqual(snap)
  })
})

describe('editsBySteps (ТК6)', () => {
  it('группы в порядке edit, sound, voice, done', () => {
    const voice = { ...fx(-2001), step: 'voice' }
    const list = [master(), voice, fx(), insert()]
    const groups = editsBySteps(list)
    expect(groups.map((g) => g.step)).toEqual(['edit', 'sound', 'voice', 'done'])
  })

  it('пустые группы не выводятся', () => {
    const groups = editsBySteps([fx(), master()])
    expect(groups.map((g) => g.step)).toEqual(['sound', 'done'])
  })

  it('пустой список → пусто', () => {
    expect(editsBySteps([])).toEqual([])
    expect(editsBySteps(undefined)).toEqual([])
  })

  it('порядок записей внутри группы — как во входе', () => {
    const a = fx(-1)
    const b = insert(5)
    const c = engine(-2)
    const d = mute(-3)
    const e = pedals(-4)
    const groups = editsBySteps([a, b, c, d, e])
    const by = Object.fromEntries(groups.map((g) => [g.step, g.items]))
    expect(by.sound.map((x) => x.childId)).toEqual([-1, -2, -4])
    expect(by.edit.map((x) => x.childId)).toEqual([5, -3])
  })

  it('выключенные — в своей группе и с off', () => {
    const groups = editsBySteps([{ ...fx(-1), off: true }, insert(5), { ...master(-9), off: true }])
    const by = Object.fromEntries(groups.map((g) => [g.step, g.items]))
    expect(by.sound).toHaveLength(1)
    expect(by.sound[0].off).toBe(true)
    expect(by.done).toHaveLength(1)
    expect(by.done[0].off).toBe(true)
    expect(by.edit[0].childId).toBe(5)
  })

  it('каждая запись попадает ровно в одну группу; вход не мутируется', () => {
    const list = [insert(1), mute(-1), fx(-2), { ...fx(-3), step: 'voice' }, master(-4)]
    const snap = clone(list)
    const groups = editsBySteps(list)
    expect(groups.reduce((n, g) => n + g.items.length, 0)).toBe(list.length)
    expect(list).toEqual(snap)
  })
})

describe('mergeDrafts (ТК7)', () => {
  const remoteOf = (jobId, edits, queue, ts) => ({ job_id: jobId, state: { v: 1, edits, queue, ts } })

  it('трек только локально → остаётся и уходит в push', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [] }
    const r = mergeDrafts(local, { 7: 500 }, [])
    expect(r.applied[7]).toEqual([fx(-1)])
    expect(ids(r.push)).toEqual([7])
  })

  it('трек только локально без ts → тоже в push (перенос старых правок)', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [] }
    const r = mergeDrafts(local, {}, [])
    expect(r.applied[7]).toEqual([fx(-1)])
    expect(ids(r.push)).toEqual([7])
  })

  it('трек только на воркере → берётся с воркера, не в push; parent очереди восстановлен', () => {
    const q = { childId: 300, from: 1, to: 2 }
    const r = mergeDrafts({ applied: {}, pending: [] }, {}, [remoteOf(9, [insert(5)], [q], 800)])
    expect(r.applied[9]).toEqual([insert(5)])
    expect(r.pending).toEqual([{ ...q, parent: 9 }])
    expect(r.ts[9]).toBe(800)
    expect(ids(r.push)).toEqual([])
  })

  it('есть оба, новее локальный → локальный, в push', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [{ childId: 40, parent: 7 }] }
    const r = mergeDrafts(local, { 7: 900 }, [remoteOf(7, [insert(5)], [], 100)])
    expect(r.applied[7]).toEqual([fx(-1)])
    expect(r.pending).toEqual([{ childId: 40, parent: 7 }])
    expect(r.ts[7]).toBe(900)
    expect(ids(r.push)).toEqual([7])
  })

  it('есть оба, новее воркер → воркер (правки и очередь трека), не в push', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [{ childId: 40, parent: 7 }] }
    const r = mergeDrafts(local, { 7: 100 }, [remoteOf(7, [insert(5)], [{ childId: 41 }], 900)])
    expect(r.applied[7]).toEqual([insert(5)])
    expect(r.pending).toEqual([{ childId: 41, parent: 7 }])
    expect(r.ts[7]).toBe(900)
    expect(ids(r.push)).toEqual([])
  })

  it('локальный без ts старше любого воркерного', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [] }
    const r = mergeDrafts(local, {}, [remoteOf(7, [insert(5)], [], 1)])
    expect(r.applied[7]).toEqual([insert(5)])
    expect(ids(r.push)).toEqual([])
  })

  it('равные ts → воркер', () => {
    const local = { applied: { 7: [fx(-1)] }, pending: [] }
    const r = mergeDrafts(local, { 7: 500 }, [remoteOf(7, [insert(5)], [], 500)])
    expect(r.applied[7]).toEqual([insert(5)])
    expect(ids(r.push)).toEqual([])
  })

  it('pending других треков не теряется', () => {
    const local = {
      applied: { 7: [fx(-1)], 8: [mute(-2)] },
      pending: [{ childId: 40, parent: 7 }, { childId: 50, parent: 8 }],
    }
    const r = mergeDrafts(local, { 7: 100, 8: 100 }, [remoteOf(7, [insert(5)], [{ childId: 41 }], 900)])
    expect(r.applied[8]).toEqual([mute(-2)])
    const byParent = (p) => r.pending.filter((s) => Number(s.parent) === p).map((s) => s.childId)
    expect(byParent(8)).toEqual([50])
    expect(byParent(7)).toEqual([41])
    expect(ids(r.push)).toEqual([8])
  })

  it('несколько треков: каждый решается отдельно', () => {
    const local = { applied: { 1: [fx(-1)], 2: [fx(-2)], 3: [fx(-3)] }, pending: [] }
    const remote = [remoteOf(1, [insert(11)], [], 50), remoteOf(2, [insert(12)], [], 500), remoteOf(4, [insert(14)], [], 5)]
    const r = mergeDrafts(local, { 1: 100, 2: 100, 3: 100 }, remote)
    expect(r.applied[1]).toEqual([fx(-1)])
    expect(r.applied[2]).toEqual([insert(12)])
    expect(r.applied[3]).toEqual([fx(-3)])
    expect(r.applied[4]).toEqual([insert(14)])
    expect(ids(r.push)).toEqual([1, 3])
  })

  it('пусто с обеих сторон → пусто, push пустой', () => {
    const r = mergeDrafts({ applied: {}, pending: [] }, {}, [])
    expect(Object.keys(r.applied)).toEqual([])
    expect(r.pending).toEqual([])
    expect(r.push).toEqual([])
  })

  it('вход не мутируется', () => {
    const local = { applied: { 7: [fx(-1)], 8: [mute(-2)] }, pending: [{ childId: 40, parent: 7 }] }
    const localTs = { 7: 100, 8: 999 }
    const remote = [remoteOf(7, [insert(5)], [{ childId: 41 }], 900), remoteOf(8, [], [], 1)]
    const snap = clone({ local, localTs, remote })
    mergeDrafts(local, localTs, remote)
    expect({ local, localTs, remote }).toEqual(snap)
  })
})

describe('draftState (ТК8)', () => {
  it('{v:1, edits трека, queue только этого трека без parent, ts}', () => {
    const applied = { 7: [fx(-1), insert(5)], 8: [mute(-2)] }
    const pending = [{ childId: 40, parent: 7, from: 1 }, { childId: 50, parent: 8 }, { childId: 41, parent: 7 }]
    const st = draftState(7, applied, pending, 1234)
    expect(st).toEqual({
      v: 1,
      edits: [fx(-1), insert(5)],
      queue: [{ childId: 40, from: 1 }, { childId: 41 }],
      ts: 1234,
    })
    expect(st.queue.every((q) => !('parent' in q))).toBe(true)
  })

  it('выключенные записи сохраняются в edits с off', () => {
    const st = draftState(7, { 7: [{ ...fx(-1), off: true }] }, [], 1)
    expect(st.edits).toEqual([{ ...fx(-1), off: true }])
  })

  it('не мутирует вход (parent в pending остаётся)', () => {
    const applied = { 7: [fx(-1)] }
    const pending = [{ childId: 40, parent: 7 }]
    const snap = clone({ applied, pending })
    draftState(7, applied, pending, 1)
    expect({ applied, pending }).toEqual(snap)
  })

  it('трека нет в реестре → пустой черновик, isEmptyDraft true', () => {
    const st = draftState(9, { 7: [fx(-1)] }, [{ childId: 40, parent: 7 }], 5)
    expect(st.edits).toEqual([])
    expect(st.queue).toEqual([])
    expect(isEmptyDraft(st)).toBe(true)
  })

  it('isEmptyDraft: есть правки или очередь → false', () => {
    expect(isEmptyDraft(draftState(7, { 7: [fx(-1)] }, [], 1))).toBe(false)
    expect(isEmptyDraft(draftState(7, {}, [{ childId: 40, parent: 7 }], 1))).toBe(false)
    expect(isEmptyDraft(draftState(7, { 7: [{ ...fx(-1), off: true }] }, [], 1))).toBe(false)
  })

  it('draftState → mergeDrafts: черновик воркера восстанавливает тот же реестр трека', () => {
    const applied = { 7: [fx(-1), insert(5)] }
    const pending = [{ childId: 40, parent: 7 }]
    const st = draftState(7, applied, pending, 77)
    const r = mergeDrafts({ applied: {}, pending: [] }, {}, [{ job_id: 7, state: st }])
    expect(r.applied[7]).toEqual(applied[7])
    expect(r.pending).toEqual(pending)
  })
})

describe('mergeDrafts: трек есть только в localTs (ТК9)', () => {
  // правки и очередь трека сбросили офлайн: локально трека нет, но ts сброса остался
  const remoteOf = (jobId, edits, queue, ts) => ({ job_id: jobId, state: { v: 1, edits, queue, ts } })

  it('ts сброса новее воркерного → правок и очереди трека нет, трек в push (удалить черновик на воркере)', () => {
    const local = { applied: { 8: [mute(-2)] }, pending: [{ childId: 50, parent: 8 }] }
    const r = mergeDrafts(local, { 7: 900, 8: 900 }, [remoteOf(7, [insert(5)], [{ childId: 41 }], 100)])
    expect(r.applied[7] || []).toEqual([])
    expect(r.pending.filter((s) => Number(s.parent) === 7)).toEqual([])
    expect(r.push.map(Number)).toContain(7)
    // соседний трек не пострадал
    expect(r.applied[8]).toEqual([mute(-2)])
    expect(r.pending.filter((s) => Number(s.parent) === 8).map((s) => s.childId)).toEqual([50])
  })

  it('ts сброса старше воркерного → черновик воркера, не в push', () => {
    const r = mergeDrafts({ applied: {}, pending: [] }, { 7: 100 }, [remoteOf(7, [insert(5)], [{ childId: 41 }], 900)])
    expect(r.applied[7]).toEqual([insert(5)])
    expect(r.pending).toEqual([{ childId: 41, parent: 7 }])
    expect(r.ts[7]).toBe(900)
    expect(r.push.map(Number)).not.toContain(7)
  })

  it('трек только в localTs, на воркере черновика нет → в push', () => {
    const r = mergeDrafts({ applied: {}, pending: [] }, { 7: 500 }, [])
    expect(r.applied[7] || []).toEqual([])
    expect(r.pending).toEqual([])
    expect(ids(r.push)).toEqual([7])
  })
})

describe('mergeDrafts: ключ трека', () => {
  const remoteOf = (jobId, edits, queue, ts) => ({ job_id: jobId, state: { v: 1, edits, queue, ts } })

  it('нечисловой ключ трека (P) — правки и очередь не теряются', () => {
    const local = { applied: { P: [fx(-1)] }, pending: [{ childId: 40, parent: 'P' }] }
    const r = mergeDrafts(local, { P: 500 }, [])
    expect(r.applied.P).toEqual([fx(-1)])
    const q = r.pending.filter((s) => String(s.parent) === 'P').map((s) => s.childId)
    expect(q).toEqual([40])
  })

  it('нечисловой ключ без ts — тоже не теряется', () => {
    const local = { applied: { P: [fx(-1)] }, pending: [{ childId: 40, parent: 'P' }] }
    const r = mergeDrafts(local, {}, [])
    expect(r.applied.P).toEqual([fx(-1)])
    expect(r.pending.map((s) => s.childId)).toEqual([40])
  })

  it('job_id 7 (число) с воркера и локальный ключ "7" — один трек: новее воркер', () => {
    const local = { applied: { '7': [fx(-1)] }, pending: [{ childId: 40, parent: '7' }] }
    const r = mergeDrafts(local, { '7': 100 }, [remoteOf(7, [insert(5)], [{ childId: 41 }], 900)])
    expect(Object.keys(r.applied)).toEqual(['7'])
    expect(r.applied[7]).toEqual([insert(5)])
    expect(r.pending.map((s) => s.childId)).toEqual([41])
    expect(ids(r.push)).toEqual([])
  })

  it('job_id 7 (число) с воркера и локальный ключ "7" — один трек: новее локальный, один push', () => {
    const local = { applied: { '7': [fx(-1)] }, pending: [] }
    const r = mergeDrafts(local, { '7': 900 }, [remoteOf(7, [insert(5)], [], 100)])
    expect(Object.keys(r.applied)).toEqual(['7'])
    expect(r.applied[7]).toEqual([fx(-1)])
    expect(ids(r.push)).toEqual([7])
  })
})

// internal-studio-draft3, условие 1, ТК2: изменения с рендером (gen) — часть состояния черновика трека.
// gen = {specs: [приёмы плана], voice: {params, name} | null, instrumental: bool};
// gens — { [parentId]: gen }, как applied. applying — {job_id, title} идущего «применить всё».
describe('gen в черновике трека (ТК2)', () => {
  const remoteOf = (jobId, state) => ({ job_id: jobId, state: { v: 1, edits: [], queue: [], ts: 1, ...state } })
  const oct = { kind: 'octave', dir: 'up', label: 'октава вверх', targets: [{ voice: 'Vocal', bar: 0 }] }
  const voice = { params: { register: 'female', delivery: 1 }, name: 'Эфир' }
  const G = { specs: [oct], voice: null, instrumental: false }
  const GV = { specs: [], voice, instrumental: true }
  const EMPTY = { specs: [], voice: null, instrumental: false }

  describe('isEmptyGen', () => {
    it('нет gen / пустой gen (нет specs, нет voice, instrumental false) → true', () => {
      expect(isEmptyGen(undefined)).toBe(true)
      expect(isEmptyGen(null)).toBe(true)
      expect(isEmptyGen({})).toBe(true)
      expect(isEmptyGen(EMPTY)).toBe(true)
    })

    it('есть приёмы, или голос, или «без голоса» → false', () => {
      expect(isEmptyGen({ ...EMPTY, specs: [oct] })).toBe(false)
      expect(isEmptyGen({ ...EMPTY, voice })).toBe(false)
      expect(isEmptyGen({ ...EMPTY, instrumental: true })).toBe(false)
    })
  })

  describe('draftState', () => {
    it('gen трека переносится в состояние; правки и очередь — как прежде', () => {
      const st = draftState(7, { 7: [fx(-1)] }, [{ childId: 40, parent: 7 }], 55, { 7: G, 8: GV })
      expect(st.gen).toEqual(G)
      expect(st.edits).toEqual([fx(-1)])
      expect(st.queue).toEqual([{ childId: 40 }])
      expect(st.ts).toBe(55)
      expect(st.v).toBe(1)
    })

    it('gen другого трека не попадает', () => {
      const st = draftState(7, {}, [], 1, { 8: GV })
      expect(isEmptyGen(st.gen)).toBe(true)
    })

    it('без gens (старый вызов) → состояние без gen, как в этапе 2', () => {
      const st = draftState(7, { 7: [fx(-1)] }, [], 1)
      expect(isEmptyGen(st.gen)).toBe(true)
      expect(st.edits).toEqual([fx(-1)])
    })

    it('не мутирует gens', () => {
      const gens = { 7: clone(G) }
      const snap = clone(gens)
      draftState(7, {}, [], 1, gens)
      expect(gens).toEqual(snap)
    })
  })

  describe('isEmptyDraft с gen и applying', () => {
    it('трек только с gen (без правок и очереди) — не пуст', () => {
      expect(isEmptyDraft(draftState(7, {}, [], 1, { 7: G }))).toBe(false)
      expect(isEmptyDraft(draftState(7, {}, [], 1, { 7: { ...EMPTY, instrumental: true } }))).toBe(false)
      expect(isEmptyDraft(draftState(7, {}, [], 1, { 7: { ...EMPTY, voice } }))).toBe(false)
    })

    it('пустой gen и нет правок/очереди → пусто', () => {
      expect(isEmptyDraft(draftState(7, {}, [], 1, { 7: EMPTY }))).toBe(true)
      expect(isEmptyDraft({ v: 1, edits: [], queue: [], ts: 1, gen: EMPTY })).toBe(true)
    })

    it('идёт «применить всё» (applying) → не пусто', () => {
      expect(isEmptyDraft({ v: 1, edits: [], queue: [], ts: 1, applying: { job_id: 901, title: 'итог' } })).toBe(false)
    })
  })

  describe('mergeDrafts', () => {
    it('трек только на воркере с gen → gens[трек] = gen воркера', () => {
      const r = mergeDrafts({ applied: {}, pending: [], gens: {} }, {}, [remoteOf(9, { gen: G, ts: 800 })])
      expect(r.gens[9]).toEqual(G)
      expect(ids(r.push)).toEqual([])
    })

    it('трек только локально и только с gen → gen остаётся, трек в push', () => {
      const r = mergeDrafts({ applied: {}, pending: [], gens: { 7: GV } }, { 7: 500 }, [])
      expect(r.gens[7]).toEqual(GV)
      expect(ids(r.push)).toEqual([7])
    })

    it('новее локальный → gen локальный (в т.ч. отсутствие gen побеждает старый gen воркера)', () => {
      const r1 = mergeDrafts({ applied: { 7: [fx(-1)] }, pending: [], gens: { 7: G } }, { 7: 900 },
        [remoteOf(7, { edits: [insert(5)], gen: GV, ts: 100 })])
      expect(r1.gens[7]).toEqual(G)
      const r2 = mergeDrafts({ applied: { 7: [fx(-1)] }, pending: [], gens: {} }, { 7: 900 },
        [remoteOf(7, { edits: [insert(5)], gen: GV, ts: 100 })])
      expect(isEmptyGen(r2.gens[7])).toBe(true)
    })

    it('новее воркер → gen воркера (локальный gen трека заменяется)', () => {
      const r = mergeDrafts({ applied: { 7: [fx(-1)] }, pending: [], gens: { 7: G } }, { 7: 100 },
        [remoteOf(7, { edits: [insert(5)], gen: GV, ts: 900 })])
      expect(r.gens[7]).toEqual(GV)
      expect(r.applied[7]).toEqual([insert(5)])
    })

    it('gen соседнего трека не теряется', () => {
      const r = mergeDrafts({ applied: { 8: [mute(-2)] }, pending: [], gens: { 8: G } }, { 7: 100, 8: 100 },
        [remoteOf(7, { gen: GV, ts: 900 })])
      expect(r.gens[8]).toEqual(G)
      expect(r.gens[7]).toEqual(GV)
    })

    it('applying из черновика воркера переносится (ход «применить всё» переживает перезапуск)', () => {
      const applying = { job_id: 901, title: 'Песня · итог' }
      const r = mergeDrafts({ applied: {}, pending: [], gens: {}, applying: {} }, {},
        [remoteOf(7, { edits: [fx(-1)], gen: G, applying, ts: 800 })])
      expect(r.applying[7]).toEqual(applying)
      expect(r.gens[7]).toEqual(G)
    })

    it('draftState → mergeDrafts: gen трека восстанавливается', () => {
      const st = draftState(7, { 7: [fx(-1)] }, [], 77, { 7: GV })
      const r = mergeDrafts({ applied: {}, pending: [], gens: {} }, {}, [{ job_id: 7, state: st }])
      expect(r.gens[7]).toEqual(GV)
      expect(r.applied[7]).toEqual([fx(-1)])
    })

    it('старый вход без gens/applying — работает как прежде', () => {
      const r = mergeDrafts({ applied: { 7: [fx(-1)] }, pending: [] }, { 7: 500 }, [])
      expect(r.applied[7]).toEqual([fx(-1)])
      expect(isEmptyGen((r.gens || {})[7])).toBe(true)
    })

    it('вход не мутируется', () => {
      const local = { applied: { 7: [fx(-1)] }, pending: [], gens: { 7: clone(G) }, applying: {} }
      const remote = [remoteOf(7, { gen: GV, ts: 900 }), remoteOf(8, { gen: G, applying: { job_id: 1, title: 't' }, ts: 5 })]
      const snap = clone({ local, remote })
      mergeDrafts(local, { 7: 100 }, remote)
      expect({ local, remote }).toEqual(snap)
    })
  })
})

// «Голос альбома» в черновике: тембр песни (internal-album-voice-draft, условие 1; ТК1).
// Новые функции берутся через пространство имён: пока их нет, падают только тесты ниже, а не весь файл.
import * as draftMod from './draftEdits.js'

describe('тембр песни в gen (internal-album-voice-draft, ТК1)', () => {
  const TB = { ref_job_id: 9, name: 'X' }
  const oct = { kind: 'octave', dir: 'up', label: 'октава вверх', targets: [{ voice: 'Vocal', bar: 0 }] }
  const voice = { params: { register: 'female', delivery: 1 }, name: 'Эфир' }
  const EMPTY = { specs: [], voice: null, instrumental: false }
  const remoteOf = (jobId, state) => ({ job_id: jobId, state: { v: 1, edits: [], queue: [], ts: 1, ...state } })

  describe('isEmptyGen', () => {
    it('gen только с тембром — не пустой', () => {
      expect(isEmptyGen({ timbre: TB })).toBe(false)
      expect(isEmptyGen({ ...EMPTY, timbre: TB })).toBe(false)
    })

    it('тембр null при прочих пустых полях — пусто', () => {
      expect(isEmptyGen({ ...EMPTY, timbre: null })).toBe(true)
    })
  })

  describe('genNeedsRender', () => {
    it('экспортируется функцией', () => {
      expect(typeof draftMod.genNeedsRender).toBe('function')
    })

    it('приёмы плана, новый голос или «без голоса» → нужен рендер (с тембром и без)', () => {
      const f = draftMod.genNeedsRender
      expect(f({ ...EMPTY, specs: [oct] })).toBe(true)
      expect(f({ ...EMPTY, voice })).toBe(true)
      expect(f({ ...EMPTY, instrumental: true })).toBe(true)
      expect(f({ ...EMPTY, specs: [oct], timbre: TB })).toBe(true)
      expect(f({ ...EMPTY, voice, timbre: TB })).toBe(true)
      expect(f({ ...EMPTY, instrumental: true, timbre: TB })).toBe(true)
    })

    it('только тембр → рендер не нужен', () => {
      const f = draftMod.genNeedsRender
      expect(f({ timbre: TB })).toBe(false)
      expect(f({ ...EMPTY, timbre: TB })).toBe(false)
    })

    it('нет gen / пустой gen → рендер не нужен', () => {
      const f = draftMod.genNeedsRender
      expect(f(null)).toBe(false)
      expect(f(undefined)).toBe(false)
      expect(f({})).toBe(false)
      expect(f(EMPTY)).toBe(false)
    })
  })

  describe('genTimbre', () => {
    it('экспортируется функцией', () => {
      expect(typeof draftMod.genTimbre).toBe('function')
    })

    it('есть тембр → он и отдаётся (с приёмами и голосом тоже)', () => {
      const f = draftMod.genTimbre
      expect(f({ timbre: TB })).toEqual(TB)
      expect(f({ ...EMPTY, specs: [oct], voice, timbre: TB })).toEqual(TB)
    })

    it('«без голоса» → тембра к применению нет', () => {
      expect(draftMod.genTimbre({ ...EMPTY, instrumental: true, timbre: TB }) ?? null).toBe(null)
    })

    it('тембра нет / gen нет → null', () => {
      const f = draftMod.genTimbre
      expect(f({ ...EMPTY, specs: [oct] }) ?? null).toBe(null)
      expect(f({ ...EMPTY, timbre: null }) ?? null).toBe(null)
      expect(f(null) ?? null).toBe(null)
      expect(f(undefined) ?? null).toBe(null)
    })
  })

  describe('draftState / mergeDrafts переносят gen с тембром', () => {
    it('draftState: gen трека с тембром попадает в состояние как есть; черновик не пуст', () => {
      const G = { ...EMPTY, timbre: TB }
      const st = draftState(7, {}, [], 1, { 7: G })
      expect(st.gen).toEqual(G)
      expect(isEmptyDraft(st)).toBe(false)
    })

    it('mergeDrafts: gen с тембром с воркера восстанавливается', () => {
      const G = { ...EMPTY, specs: [oct], timbre: TB }
      const r = mergeDrafts({ applied: {}, pending: [], gens: {} }, {}, [remoteOf(9, { gen: G, ts: 800 })])
      expect(r.gens[9]).toEqual(G)
    })

    it('draftState → mergeDrafts: тембр переживает круг через воркер', () => {
      const G = { ...EMPTY, timbre: TB }
      const st = draftState(7, {}, [], 77, { 7: G })
      const r = mergeDrafts({ applied: {}, pending: [], gens: {} }, {}, [{ job_id: 7, state: st }])
      expect(r.gens[7]).toEqual(G)
    })

    it('локальный трек только с тембром → остаётся и уходит на воркер (push)', () => {
      const G = { ...EMPTY, timbre: TB }
      const r = mergeDrafts({ applied: {}, pending: [], gens: { 7: G } }, { 7: 500 }, [])
      expect(r.gens[7]).toEqual(G)
      expect(r.push.map(Number)).toEqual([7])
    })
  })
})
