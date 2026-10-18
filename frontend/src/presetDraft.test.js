// Пресет звука в черновике. Чистая логика без DOM.
// internal-studio-instant-apply (условие 4, ТК4): presetRecord(preset, {id, name}, childId) — запись черновика
//   с рецептом пресета {preset: {id, name, recipe}}, без подготовки; draftSpecs раскрывает её в ОДНУ
//   спецификацию {child_id 0, …, stems [], id, preset: {preset: recipe, skip_sound, skip_place, skip_master}} —
//   перекрытия по тем же правилам, что у старой записи (звук/место/мастер человека); у каждой спецификации
//   id: String(childId) записи черновика.
// internal-studio-preset-draft (условия 10–14): withPreset(list, record) — пресет в черновике один и стоит
//   первым; старая запись (preset.sections, посчитанные воркером) раскрывается как раньше: sections минус
//   перекрытые правками человека → final на весь микс → master; specOf и editStep — из draftEdits.js.
//
// Толкования: порядок дорожек в skip_sound/skip_place не важен (сравнение множеств), пустой перечень = нет
// поля или []; skip_master false = нет поля или false; дорожка «mix» (правки на весь трек) звук дорожки
// пресета не перекрывает и в skip_sound не попадает (как final пресета остаётся при эффекте на весь трек).
//
// Записи человека — в форме, в которой их пишет useInserts (addMutes/addStemFx/addStemPedals/
// addStemEngine/mixItem) и mixDesk (withMaster/withStemPlace); sections пресета — в формате
// SectionSpec (Go), как их отдаёт воркер.
import { describe, it, expect } from 'vitest'
import { presetRecord, withPreset, draftSpecs } from './presetDraft.js'
import { specOf, editStep } from './draftEdits.js'
import { withMaster, withStemPlace } from './mixDesk.js'

const clone = (x) => JSON.parse(JSON.stringify(x))

// ---- записи человека ----
const base = { from: 0, to: 0, lead: 0, beat: 0, db: 0, fadeIn: 0, fadeOut: 0, keepHighHz: 0 }
const mute = (stem, id = -101, db = -100) => ({ ...base, childId: id, instId: stem, stems: [stem], db })
const stemFx = (stem, id = -102) => ({ ...base, childId: id, instId: 'fx-reverb', stems: [stem],
  chain: 'reverb', params: { mix: 0.3 } })
const stemPedals = (stem, id = -103) => ({ ...base, childId: id, instId: 'pedals', stems: [stem],
  steps: [{ chain: 'drive', params: { gain: 4 } }], label: 'педали' })
const stemEngine = (stem, id = -104, engine = [{ type: 'eq', highpass_hz: 80 }]) => ({ ...base, childId: id,
  instId: 'engine', stems: [stem], engine, label: 'движок' })
const stemAdd = (stem, id = -105) => ({ ...stemEngine(stem, id), add: true, place: { pan: 0.3, width: 1 } })
const stemEnv = (stem, id = -106) => ({ ...base, childId: id, instId: 'env', stems: [stem],
  envelope: [{ t: 0, db: -3 }, { t: 10, db: 0 }] })
const mixFx = (id = -107) => ({ ...base, childId: id, instId: 'fx-reverb', stems: ['mix'], mix: true,
  chain: 'reverb', params: { mix: 0.2 } })
const insert = (stem, id = 191) => ({ ...base, childId: id, instId: 'guitar', from: 4, to: 8, lead: 2, beat: 0.5,
  db: -6, stems: [stem], chain: 'reverb', params: {} })
const humanMaster = () => withMaster([], [{ type: 'glue', threshold_db: -20 }], 'мой мастер')[0]
const humanPlace = (stem, place = { pan: -0.5, width: 1 }) => withStemPlace([], stem, place)[0]

// ---- старая запись: посчитанные sections (SectionSpec) ----
const sec = (stems, extra = {}) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems,
  fade_in: 0, fade_out: 0, keep_high_hz: 0, ...extra })
const pChain = (stem) => sec([stem], { chain: 'comp', params: { ratio: 3 } })
const pSteps = (stem) => sec([stem], { steps: [{ chain: 'drive', params: {} }] })
const pEngine = (stem) => sec([stem], { engine: [{ type: 'eq', lowpass_hz: 9000 }], level_target: -18, level_total: -12 })
const pVolume = (stem) => sec([stem], { db: -3 })
const pPlace = (stem) => sec([stem], { place: { pan: 0.4, width: 1.2 } })
const pAdd = (stem) => sec([stem], { engine: [{ type: 'synth', wave: 'saw' }], add: true, place: { pan: -0.2, width: 1 } })

const FINAL = [{ chain: 'tape', params: { drive: 0.2 } }]
const MASTER = [{ type: 'glue', ratio: 2 }, { type: 'limiter', ceiling_db: -1, target_lufs: -14 }]
const result = (over = {}) => ({ sections: [pEngine('drums'), pAdd('mix')], final: clone(FINAL), master: clone(MASTER),
  warnings: ['гитарист выкинут: нет плана'], ...over })
const INFO = { id: 'post-punk', name: 'Пост-панк' }
// старая запись пресета — как её писал прежний presetRecord из ответа PresetDraft
const oldRecord = (r, info, childId) => ({
  childId, instId: 'preset', from: 0, to: 0, lead: 0, beat: 0, db: 0, stems: [], fadeIn: 0, fadeOut: 0,
  keepHighHz: 0, step: 'done', label: info.name,
  preset: { id: info.id, name: info.name, sections: clone(r.sections || []), final: clone(r.final || []),
    master: clone(r.master || []), warnings: clone(r.warnings || []) },
})
const rec = (over = {}, id = -900) => oldRecord(result(over), INFO, id)

// спецификация пересборки записи черновика: specOf + id записи
// specOf сам id может и не ставить — id обязателен в выходе draftSpecs
const noId = (x) => { const y = { ...x }; delete y.id; return y }
const own = (it) => ({ ...specOf(it), id: String(it.childId) })
// секции старой записи-пресета получают id записи-пресета
const withId = (id) => (sp) => ({ ...sp, id: String(id) })

// ---- рецепт пресета (yue.SoundPreset) ----
const RECIPE = {
  id: 71, name: 'Пост-панк', specs: [{ stems: ['bass'], engine: [{ type: 'eq', highpass_hz: 80 }], db: -2 },
    { stems: ['drums'], place: { pan: 0.3, width: 1 } }],
  final: [{ chain: 'width', params: { width: 1.1 } }], target_lufs: -13, master: [{ type: 'glue', ratio: 2 }],
  parts: [{ kind: 'synth', engine: [{ type: 'synth', osc1: 4 }], style: 'pad' }],
}
const recipeRec = (id = -900) => presetRecord(clone(RECIPE), { id: 71, name: 'Пост-панк' }, id)
const sorted = (x) => [...(x || [])].sort()
// единственная спецификация записи-пресета в выходе draftSpecs
const presetSpecOf = (list) => {
  const ps = draftSpecs(list).filter((s) => s.preset && 'preset' in s.preset)
  expect(ps.length).toBe(1)
  return ps[0]
}
const skips = (sp) => ({ sound: sorted(sp.preset.skip_sound), place: sorted(sp.preset.skip_place),
  master: !!sp.preset.skip_master })

// ожидаемые записи раскрытия final и master (условие 12)
const finalSpec = (steps) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems: ['mix'],
  fade_in: 0, fade_out: 0, keep_high_hz: 0, mix: true, steps })
const masterSpec = (engine) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems: [],
  fade_in: 0, fade_out: 0, keep_high_hz: 0, engine, master: true })

// sections пресета в выходе draftSpecs (без final/master)
const presetOnly = (sections, human) => {
  const out = draftSpecs([oldRecord({ sections }, INFO, -900), ...human])
  const mine = human.filter((it) => !it.off).map(own)
  return out.slice(0, out.length - mine.length)
}
// sections в выходе presetOnly — с id записи-пресета
const ids = (list) => list.map(withId(-900))

describe('presetRecord — запись с рецептом (ТК4, условие 4)', () => {
  it('поля записи: childId, preset {id, name, recipe — пресет как передан}; шаг «готово»', () => {
    const r = recipeRec()
    expect(r.childId).toBe(-900)
    expect(r.preset).toMatchObject({ id: 71, name: 'Пост-панк' })
    expect(r.preset.recipe).toEqual(RECIPE)
    expect(r.preset.sections).toBeUndefined()
    expect(editStep(r)).toBe('done')
  })

  it('вход не мутируется и запись от него не зависит (глубокая копия рецепта)', () => {
    const p = clone(RECIPE)
    const info = { id: 71, name: 'Пост-панк' }
    const out = presetRecord(p, info, -900)
    expect(p).toEqual(RECIPE)
    p.specs[0].db = 9
    p.master.push({ type: 'x' })
    expect(out.preset.recipe).toEqual(RECIPE)
    expect(info).toEqual({ id: 71, name: 'Пост-панк' })
  })

  it('withPreset: новая запись с рецептом заменяет старую (sections) — пресет в черновике один, первым', () => {
    const old = rec({}, -800)
    const a = mute('drums')
    const p = recipeRec(-901)
    expect(withPreset([a, old], p)).toEqual([p, a])
  })
})

describe('draftSpecs: запись с рецептом — одна спецификация (ТК4)', () => {
  it('без правок человека: {child_id 0, окно 0, stems [], id записи, preset {preset: рецепт}}, перекрытий нет', () => {
    const out = draftSpecs([recipeRec()])
    expect(out.length).toBe(1)
    expect(out[0]).toMatchObject({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems: [],
      fade_in: 0, fade_out: 0, keep_high_hz: 0, id: '-900' })
    expect(out[0].preset.preset).toEqual(RECIPE)
    expect(skips(out[0])).toEqual({ sound: [], place: [], master: false })
    for (const k of ['chain', 'engine', 'steps', 'master', 'mix', 'add']) expect(out[0][k] ?? undefined).toBeFalsy()
  })

  it('на своём месте в списке; правки человека — своими спецификациями с id', () => {
    const a = mute('drums')
    const b = stemFx('bass')
    const out = draftSpecs([a, recipeRec(), b])
    expect(out.length).toBe(3)
    expect(out[0]).toEqual(own(a))
    expect(out[1].id).toBe('-900')
    expect(out[1].preset.preset).toEqual(RECIPE)
    expect(out[2]).toEqual(own(b))
  })

  it('выключенный пресет не идёт', () => {
    const a = stemFx('bass')
    expect(draftSpecs([{ ...recipeRec(), off: true }, a])).toEqual([own(a)])
  })

  it('draftSpecs не мутирует запись с рецептом', () => {
    const list = [recipeRec(), stemFx('drums'), humanMaster()]
    const before = clone(list)
    draftSpecs(list)
    expect(list).toEqual(before)
  })

  describe('перекрытия — звук дорожки (skip_sound)', () => {
    for (const [name, mk] of [['эффект (chain)', stemFx], ['педали (steps)', stemPedals], ['движок (engine)', stemEngine]]) {
      it(`${name} человека на drums → skip_sound [drums]`, () => {
        expect(skips(presetSpecOf([recipeRec(), mk('drums')])).sound).toEqual(['drums'])
      })
    }

    it('звук на двух дорожках (bass, drums) → обе', () => {
      expect(skips(presetSpecOf([recipeRec(), stemFx('bass', -1), stemEngine('drums', -2)])).sound).toEqual(['bass', 'drums'])
    })

    it('правка человека стоит в списке до пресета — всё равно перекрывает', () => {
      expect(skips(presetSpecOf([stemFx('drums'), recipeRec()])).sound).toEqual(['drums'])
    })

    for (const [name, it2] of [
      ['пустой engine', stemEngine('drums', -104, [])],
      ['партия-добавление (add)', stemAdd('drums')],
      ['вклейка рендера (childId > 0)', insert('drums')],
      ['выключенная правка', { ...stemFx('drums'), off: true }],
      ['громкость (заглушить)', mute('drums')],
      ['линия громкости', stemEnv('drums')],
      ['место дорожки', humanPlace('drums')],
      ['эффект на весь трек', mixFx()],
    ]) {
      it(`${name} — звук пресета не перекрывает`, () => {
        expect(skips(presetSpecOf([recipeRec(), it2])).sound).toEqual([])
      })
    }
  })

  describe('перекрытия — место дорожки (skip_place)', () => {
    it('место человека на drums → skip_place [drums]; звук не перекрыт', () => {
      expect(skips(presetSpecOf([recipeRec(), humanPlace('drums')]))).toEqual({ sound: [], place: ['drums'], master: false })
    })

    it('выключенное место не перекрывает', () => {
      expect(skips(presetSpecOf([recipeRec(), { ...humanPlace('drums'), off: true }])).place).toEqual([])
    })

    it('звук человека место не перекрывает', () => {
      expect(skips(presetSpecOf([recipeRec(), stemFx('drums')])).place).toEqual([])
    })
  })

  describe('перекрытия — мастер (skip_master)', () => {
    it('мастер человека → skip_master; мастер человека — своей спецификацией', () => {
      const m = humanMaster()
      const out = draftSpecs([recipeRec(), m])
      expect(skips(out[0]).master).toBe(true)
      expect(out[1]).toEqual(own(m))
    })

    it('выключенный мастер человека → skip_master нет', () => {
      expect(skips(presetSpecOf([recipeRec(), { ...humanMaster(), off: true }])).master).toBe(false)
    })
  })
})

describe('withPreset (условие 11 прежней карточки)', () => {
  it('пустой список → только пресет', () => {
    const p = rec()
    expect(withPreset([], p)).toEqual([p])
  })

  it('новый пресет первым, остальные записи в прежнем порядке', () => {
    const list = [mute('drums'), stemFx('bass'), humanMaster()]
    const p = rec()
    const out = withPreset(list, p)
    expect(out[0]).toEqual(p)
    expect(out.slice(1)).toEqual(list)
  })

  it('прежний пресет (где бы ни стоял) убран — пресет в черновике один', () => {
    const old1 = rec({}, -800)
    const old2 = oldRecord(result(), { id: 'other', name: 'Другой' }, -801)
    const a = mute('drums')
    const b = stemFx('bass')
    const p = oldRecord(result(), { id: 'grunge', name: 'Гранж' }, -900)
    const out = withPreset([a, old1, b, old2], p)
    expect(out).toEqual([p, a, b])
  })

  it('выключенный прежний пресет тоже убран', () => {
    const old = { ...rec({}, -800), off: true }
    const p = rec({}, -900)
    expect(withPreset([old, mute('drums')], p)).toEqual([p, mute('drums')])
  })

  it('вход не мутируется', () => {
    const list = [mute('drums'), rec({}, -800), stemFx('bass')]
    const before = clone(list)
    const p = rec({}, -900)
    const pBefore = clone(p)
    withPreset(list, p)
    expect(list).toEqual(before)
    expect(p).toEqual(pBefore)
  })
})

describe('specOf и editStep в draftEdits (ТК7, условие 14)', () => {
  it('specOf экспортирован: формат записи пересборки как прежде', () => {
    expect(noId(specOf(mute('drums')))).toEqual({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: -100,
      stems: ['drums'], fade_in: 0, fade_out: 0, keep_high_hz: 0 })
    expect(specOf(stemFx('bass'))).toMatchObject({ child_id: 0, chain: 'reverb', params: { mix: 0.3 } })
    expect(specOf(insert('other', 191))).toMatchObject({ child_id: 191, lead: 2, beat_sec: 0.5 })
    expect(specOf(mixFx())).toMatchObject({ mix: true, stems: ['mix'] })
    expect(specOf(humanMaster())).toMatchObject({ master: true, stems: [] })
  })

  it('editStep старой записи-пресета — done', () => {
    expect(editStep(rec())).toBe('done')
    expect(editStep(oldRecord({ sections: [] }, INFO, -1))).toBe('done')
  })
})

describe('draftSpecs без пресета (ТК3, условие 12)', () => {
  it('пустой список → пусто', () => {
    expect(draftSpecs([])).toEqual([])
  })

  it('= записи без выключенных, каждая через specOf с id записи, порядок сохранён', () => {
    const list = [mute('drums'), { ...stemFx('bass'), off: true }, stemPedals('vocals'), stemEngine('other'),
      stemAdd('mix'), stemEnv('bass'), mixFx(), insert('drums'), humanPlace('other'), humanMaster()]
    expect(draftSpecs(list)).toEqual(list.filter((it) => !it.off).map(own))
  })

  it('id — строка childId записи (и у вклейки рендера)', () => {
    expect(draftSpecs([insert('drums', 191), mute('bass', -101)]).map((s) => s.id)).toEqual(['191', '-101'])
  })

  it('все записи выключены → пусто', () => {
    expect(draftSpecs([{ ...mute('drums'), off: true }])).toEqual([])
  })
})

describe('draftSpecs: старая запись (sections) раскрывается как раньше (условие 4)', () => {
  it('sections → final на весь микс → master', () => {
    const r = result()
    expect(draftSpecs([rec()])).toEqual([...r.sections, finalSpec(FINAL), masterSpec(MASTER)].map(withId(-900)))
  })

  it('пресет раскрывается на своём месте в списке', () => {
    const a = mute('drums')
    const b = stemFx('bass')
    const r = result({ sections: [pEngine('vocals')] })
    expect(draftSpecs([a, oldRecord(r, INFO, -900), b])).toEqual(
      [own(a), ...[pEngine('vocals'), finalSpec(FINAL), masterSpec(MASTER)].map(withId(-900)), own(b)])
  })

  it('пустой final — записи на весь микс нет', () => {
    expect(draftSpecs([rec({ final: [] })])).toEqual(ids([...result().sections, masterSpec(MASTER)]))
    expect(draftSpecs([rec({ final: undefined })])).toEqual(ids([...result().sections, masterSpec(MASTER)]))
  })

  it('пустой master — записи мастера нет', () => {
    expect(draftSpecs([rec({ master: [] })])).toEqual(ids([...result().sections, finalSpec(FINAL)]))
    expect(draftSpecs([rec({ master: undefined })])).toEqual(ids([...result().sections, finalSpec(FINAL)]))
  })

  it('пресет только с final/master (без sections)', () => {
    expect(draftSpecs([rec({ sections: [] })])).toEqual(ids([finalSpec(FINAL), masterSpec(MASTER)]))
  })

  it('пустой пресет → ничего', () => {
    expect(draftSpecs([oldRecord({ sections: [] }, INFO, -1)])).toEqual([])
  })

  it('level_target/level_total sections проходят как есть', () => {
    const out = draftSpecs([oldRecord({ sections: [pEngine('drums')] }, INFO, -1)])
    expect(out[0]).toMatchObject({ level_target: -18, level_total: -12 })
  })

  it('verify_db sections (цель сверки при итоге) проходит как есть; у записи без цели поля нет', () => {
    const target = sec(['bass'], { db: 4.9, verify_db: -10 })
    const out = draftSpecs([oldRecord({ sections: [target, pVolume('other')] }, INFO, -1)])
    expect(out[0]).toMatchObject({ stems: ['bass'], db: 4.9, verify_db: -10 })
    expect('verify_db' in out[1]).toBe(false)
  })

  it('draftSpecs не мутирует список и запись пресета', () => {
    const list = [rec(), stemFx('drums'), humanMaster()]
    const before = clone(list)
    draftSpecs(list)
    expect(list).toEqual(before)
  })
})

describe('draftSpecs: правка человека перекрывает старую запись пресета (условие 7)', () => {
  describe('звук дорожки', () => {
    for (const [name, mk] of [['эффект (chain)', stemFx], ['педали (steps)', stemPedals], ['движок (engine)', stemEngine]]) {
      it(`${name} человека на drums → выпадают chain/steps/engine-sections пресета на drums`, () => {
        const sections = [pChain('drums'), pSteps('drums'), pEngine('drums'), pEngine('bass')]
        expect(presetOnly(sections, [mk('drums')])).toEqual(ids([pEngine('bass')]))
      })
    }

    it('section пресета на нескольких дорожках, среди них X — выпадает', () => {
      const multi = sec(['drums', 'bass'], { engine: [{ type: 'eq' }] })
      expect(presetOnly([multi, pChain('vocals')], [stemFx('bass')])).toEqual(ids([pChain('vocals')]))
    })

    it('add-section пресета на той же дорожке остаётся', () => {
      expect(presetOnly([pAdd('drums'), pEngine('drums')], [stemEngine('drums')])).toEqual(ids([pAdd('drums')]))
    })

    it('громкость пресета (db без chain/steps/engine) на той же дорожке остаётся', () => {
      expect(presetOnly([pVolume('drums'), pChain('drums')], [stemFx('drums')])).toEqual(ids([pVolume('drums')]))
    })

    it('«место» пресета на той же дорожке звуком человека не перекрывается', () => {
      expect(presetOnly([pPlace('drums'), pChain('drums')], [stemFx('drums')])).toEqual(ids([pPlace('drums')]))
    })

    it('пустой engine человека не перекрывает', () => {
      expect(presetOnly([pEngine('drums')], [stemEngine('drums', -104, [])])).toEqual(ids([pEngine('drums')]))
    })

    it('партия-добавление человека (add) не перекрывает', () => {
      expect(presetOnly([pEngine('drums')], [stemAdd('drums')])).toEqual(ids([pEngine('drums')]))
    })

    it('вклейка рендера (childId > 0) не перекрывает', () => {
      expect(presetOnly([pChain('drums')], [insert('drums')])).toEqual(ids([pChain('drums')]))
    })

    it('выключенная правка человека не перекрывает — правка пресета снова идёт', () => {
      expect(presetOnly([pChain('drums')], [{ ...stemFx('drums'), off: true }])).toEqual(ids([pChain('drums')]))
    })

    it('правка человека стоит в списке до пресета — всё равно перекрывает', () => {
      const out = draftSpecs([stemFx('drums'), oldRecord({ sections: [pChain('drums'), pChain('bass')] }, INFO, -1)])
      expect(out).toEqual([own(stemFx('drums')), withId(-1)(pChain('bass'))])
    })
  })

  describe('место дорожки', () => {
    it('место человека на X → «место» пресета на X выпадает, на Y остаётся', () => {
      expect(presetOnly([pPlace('drums'), pPlace('bass')], [humanPlace('drums')])).toEqual(ids([pPlace('bass')]))
    })

    it('место человека не трогает звук пресета на той же дорожке', () => {
      expect(presetOnly([pPlace('drums'), pEngine('drums')], [humanPlace('drums')])).toEqual(ids([pEngine('drums')]))
    })

    it('место партии-добавления пресета (add + place) остаётся', () => {
      expect(presetOnly([pAdd('drums')], [humanPlace('drums')])).toEqual(ids([pAdd('drums')]))
    })

    it('place пресета на нескольких дорожках — не «место» одной дорожки, остаётся', () => {
      const multi = sec(['drums', 'bass'], { place: { pan: 0.2, width: 1 } })
      expect(presetOnly([multi], [humanPlace('drums')])).toEqual(ids([multi]))
    })

    it('выключенное место человека не перекрывает', () => {
      expect(presetOnly([pPlace('drums')], [{ ...humanPlace('drums'), off: true }])).toEqual(ids([pPlace('drums')]))
    })
  })

  describe('мастер', () => {
    it('мастер человека → мастер пресета не идёт (final пресета идёт)', () => {
      const m = humanMaster()
      expect(draftSpecs([rec(), m])).toEqual([...ids([...result().sections, finalSpec(FINAL)]), own(m)])
    })

    it('выключенный мастер человека → мастер пресета идёт', () => {
      expect(draftSpecs([rec(), { ...humanMaster(), off: true }]))
        .toEqual(ids([...result().sections, finalSpec(FINAL), masterSpec(MASTER)]))
    })
  })

  describe('не перекрывают — идут вместе', () => {
    it('громкость человека (заглушить/тише) на той же дорожке', () => {
      const all = [pChain('drums'), pVolume('drums'), pPlace('drums')]
      expect(presetOnly(all, [mute('drums')])).toEqual(ids(all))
      expect(presetOnly(all, [mute('drums', -101, -6)])).toEqual(ids(all))
    })

    it('линия громкости человека на той же дорожке', () => {
      const all = [pChain('drums'), pVolume('drums')]
      expect(presetOnly(all, [stemEnv('drums')])).toEqual(ids(all))
    })

    it('эффект человека на весь трек — final пресета остаётся', () => {
      const m = mixFx()
      expect(draftSpecs([rec(), m]))
        .toEqual([...ids([...result().sections, finalSpec(FINAL), masterSpec(MASTER)]), own(m)])
    })

    it('правка человека на другой дорожке', () => {
      expect(presetOnly([pChain('drums')], [stemFx('bass'), humanPlace('bass')])).toEqual(ids([pChain('drums')]))
    })
  })
})

describe('draftSpecs: выключенный пресет (ТК6)', () => {
  it('не раскрывается: ни sections, ни final, ни master', () => {
    const a = stemFx('bass')
    expect(draftSpecs([{ ...rec(), off: true }, a])).toEqual([own(a)])
  })
})
