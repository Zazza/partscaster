// Пресет звука в черновике (internal-studio-preset-draft, условия 10–14; ТК1–ТК7).
// Чистая логика без DOM:
//   presetRecord(result, {id, name}, childId) — запись черновика «пресет» из ответа PresetDraft
//     {sections, final, master, warnings};
//   withPreset(list, record) — пресет в черновике один и стоит первым;
//   draftSpecs(list) — спецификации пересборки: обычная запись → specOf, пресет раскрывается
//     на своём месте (sections минус перекрытые правками человека → final на весь микс → master);
//   specOf и editStep — из draftEdits.js.
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

// ---- ответ PresetDraft (sections — SectionSpec) ----
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
const rec = (over = {}, id = -900) => presetRecord(result(over), INFO, id)

// ожидаемые записи раскрытия final и master (условие 12)
const finalSpec = (steps) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems: ['mix'],
  fade_in: 0, fade_out: 0, keep_high_hz: 0, mix: true, steps })
const masterSpec = (engine) => ({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: 0, stems: [],
  fade_in: 0, fade_out: 0, keep_high_hz: 0, engine, master: true })

// sections пресета в выходе draftSpecs (без final/master)
const presetOnly = (sections, human) => {
  const out = draftSpecs([presetRecord({ sections }, INFO, -900), ...human])
  const own = human.filter((it) => !it.off).map(specOf)
  return out.slice(0, out.length - own.length)
}

describe('presetRecord (ТК1, условие 10)', () => {
  it('поля записи черновика', () => {
    const r = result()
    expect(presetRecord(r, INFO, -900)).toEqual({
      childId: -900, instId: 'preset', from: 0, to: 0, lead: 0, beat: 0, db: 0, stems: [], fadeIn: 0, fadeOut: 0,
      keepHighHz: 0, step: 'done', label: 'Пост-панк',
      preset: { id: 'post-punk', name: 'Пост-панк', sections: r.sections, final: FINAL, master: MASTER,
        warnings: r.warnings },
    })
  })

  it('нет final/master/warnings → пустые списки', () => {
    const p = presetRecord({ sections: [pVolume('bass')] }, INFO, -1).preset
    expect(p.final).toEqual([])
    expect(p.master).toEqual([])
    expect(p.warnings).toEqual([])
    expect(p.sections).toEqual([pVolume('bass')])
  })

  it('вход не мутируется и запись от него не зависит (глубокая копия)', () => {
    const r = result()
    const info = { ...INFO }
    const before = clone(r)
    const out = presetRecord(r, info, -900)
    expect(r).toEqual(before)
    expect(info).toEqual(INFO)
    r.sections[0].engine[0].lowpass_hz = 1
    r.final[0].params.drive = 9
    r.master.push({ type: 'x' })
    r.warnings.push('ещё')
    expect(out.preset.sections[0].engine[0].lowpass_hz).toBe(9000)
    expect(out.preset.final[0].params.drive).toBe(0.2)
    expect(out.preset.master).toEqual(MASTER)
    expect(out.preset.warnings).toEqual(['гитарист выкинут: нет плана'])
  })
})

describe('withPreset (ТК2, условие 11)', () => {
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
    const old2 = presetRecord(result(), { id: 'other', name: 'Другой' }, -801)
    const a = mute('drums')
    const b = stemFx('bass')
    const p = presetRecord(result(), { id: 'grunge', name: 'Гранж' }, -900)
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
    expect(specOf(mute('drums'))).toEqual({ child_id: 0, from: 0, to: 0, lead: 0, beat_sec: 0, db: -100,
      stems: ['drums'], fade_in: 0, fade_out: 0, keep_high_hz: 0 })
    expect(specOf(stemFx('bass'))).toMatchObject({ child_id: 0, chain: 'reverb', params: { mix: 0.3 } })
    expect(specOf(insert('other', 191))).toMatchObject({ child_id: 191, lead: 2, beat_sec: 0.5 })
    expect(specOf(mixFx())).toMatchObject({ mix: true, stems: ['mix'] })
    expect(specOf(humanMaster())).toMatchObject({ master: true, stems: [] })
  })

  it('editStep записи-пресета — done', () => {
    expect(editStep(rec())).toBe('done')
    expect(editStep(presetRecord({ sections: [] }, INFO, -1))).toBe('done')
  })
})

describe('draftSpecs без пресета (ТК3, условие 12)', () => {
  it('пустой список → пусто', () => {
    expect(draftSpecs([])).toEqual([])
  })

  it('= записи без выключенных, каждая через specOf, порядок сохранён', () => {
    const list = [mute('drums'), { ...stemFx('bass'), off: true }, stemPedals('vocals'), stemEngine('other'),
      stemAdd('mix'), stemEnv('bass'), mixFx(), insert('drums'), humanPlace('other'), humanMaster()]
    expect(draftSpecs(list)).toEqual(list.filter((it) => !it.off).map(specOf))
  })

  it('все записи выключены → пусто', () => {
    expect(draftSpecs([{ ...mute('drums'), off: true }])).toEqual([])
  })
})

describe('draftSpecs: раскрытие пресета (ТК4, условие 12)', () => {
  it('sections → final на весь микс → master', () => {
    const r = result()
    expect(draftSpecs([rec()])).toEqual([...r.sections, finalSpec(FINAL), masterSpec(MASTER)])
  })

  it('пресет раскрывается на своём месте в списке', () => {
    const a = mute('drums')
    const b = stemFx('bass')
    const r = result({ sections: [pEngine('vocals')] })
    expect(draftSpecs([a, presetRecord(r, INFO, -900), b])).toEqual(
      [specOf(a), pEngine('vocals'), finalSpec(FINAL), masterSpec(MASTER), specOf(b)])
  })

  it('пустой final — записи на весь микс нет', () => {
    expect(draftSpecs([rec({ final: [] })])).toEqual([...result().sections, masterSpec(MASTER)])
    expect(draftSpecs([rec({ final: undefined })])).toEqual([...result().sections, masterSpec(MASTER)])
  })

  it('пустой master — записи мастера нет', () => {
    expect(draftSpecs([rec({ master: [] })])).toEqual([...result().sections, finalSpec(FINAL)])
    expect(draftSpecs([rec({ master: undefined })])).toEqual([...result().sections, finalSpec(FINAL)])
  })

  it('пресет только с final/master (без sections)', () => {
    expect(draftSpecs([rec({ sections: [] })])).toEqual([finalSpec(FINAL), masterSpec(MASTER)])
  })

  it('пустой пресет → ничего', () => {
    expect(draftSpecs([presetRecord({ sections: [] }, INFO, -1)])).toEqual([])
  })

  it('level_target/level_total sections проходят как есть', () => {
    const out = draftSpecs([presetRecord({ sections: [pEngine('drums')] }, INFO, -1)])
    expect(out[0]).toMatchObject({ level_target: -18, level_total: -12 })
  })

  it('draftSpecs не мутирует список и запись пресета', () => {
    const list = [rec(), stemFx('drums'), humanMaster()]
    const before = clone(list)
    draftSpecs(list)
    expect(list).toEqual(before)
  })
})

describe('draftSpecs: правка человека перекрывает пресет (ТК5, условие 13)', () => {
  describe('звук дорожки', () => {
    for (const [name, mk] of [['эффект (chain)', stemFx], ['педали (steps)', stemPedals], ['движок (engine)', stemEngine]]) {
      it(`${name} человека на drums → выпадают chain/steps/engine-sections пресета на drums`, () => {
        const sections = [pChain('drums'), pSteps('drums'), pEngine('drums'), pEngine('bass')]
        expect(presetOnly(sections, [mk('drums')])).toEqual([pEngine('bass')])
      })
    }

    it('section пресета на нескольких дорожках, среди них X — выпадает', () => {
      const multi = sec(['drums', 'bass'], { engine: [{ type: 'eq' }] })
      expect(presetOnly([multi, pChain('vocals')], [stemFx('bass')])).toEqual([pChain('vocals')])
    })

    it('add-section пресета на той же дорожке остаётся', () => {
      expect(presetOnly([pAdd('drums'), pEngine('drums')], [stemEngine('drums')])).toEqual([pAdd('drums')])
    })

    it('громкость пресета (db без chain/steps/engine) на той же дорожке остаётся', () => {
      expect(presetOnly([pVolume('drums'), pChain('drums')], [stemFx('drums')])).toEqual([pVolume('drums')])
    })

    it('«место» пресета на той же дорожке звуком человека не перекрывается', () => {
      expect(presetOnly([pPlace('drums'), pChain('drums')], [stemFx('drums')])).toEqual([pPlace('drums')])
    })

    it('пустой engine человека не перекрывает', () => {
      expect(presetOnly([pEngine('drums')], [stemEngine('drums', -104, [])])).toEqual([pEngine('drums')])
    })

    it('партия-добавление человека (add) не перекрывает', () => {
      expect(presetOnly([pEngine('drums')], [stemAdd('drums')])).toEqual([pEngine('drums')])
    })

    it('вклейка рендера (childId > 0) не перекрывает', () => {
      expect(presetOnly([pChain('drums')], [insert('drums')])).toEqual([pChain('drums')])
    })

    it('выключенная правка человека не перекрывает — правка пресета снова идёт', () => {
      expect(presetOnly([pChain('drums')], [{ ...stemFx('drums'), off: true }])).toEqual([pChain('drums')])
    })

    it('правка человека стоит в списке до пресета — всё равно перекрывает', () => {
      const out = draftSpecs([stemFx('drums'), presetRecord({ sections: [pChain('drums'), pChain('bass')] }, INFO, -1)])
      expect(out).toEqual([specOf(stemFx('drums')), pChain('bass')])
    })
  })

  describe('место дорожки', () => {
    it('место человека на X → «место» пресета на X выпадает, на Y остаётся', () => {
      expect(presetOnly([pPlace('drums'), pPlace('bass')], [humanPlace('drums')])).toEqual([pPlace('bass')])
    })

    it('место человека не трогает звук пресета на той же дорожке', () => {
      expect(presetOnly([pPlace('drums'), pEngine('drums')], [humanPlace('drums')])).toEqual([pEngine('drums')])
    })

    it('место партии-добавления пресета (add + place) остаётся', () => {
      expect(presetOnly([pAdd('drums')], [humanPlace('drums')])).toEqual([pAdd('drums')])
    })

    it('place пресета на нескольких дорожках — не «место» одной дорожки, остаётся', () => {
      const multi = sec(['drums', 'bass'], { place: { pan: 0.2, width: 1 } })
      expect(presetOnly([multi], [humanPlace('drums')])).toEqual([multi])
    })

    it('выключенное место человека не перекрывает', () => {
      expect(presetOnly([pPlace('drums')], [{ ...humanPlace('drums'), off: true }])).toEqual([pPlace('drums')])
    })
  })

  describe('мастер', () => {
    it('мастер человека → мастер пресета не идёт (final пресета идёт)', () => {
      const m = humanMaster()
      expect(draftSpecs([rec(), m])).toEqual([...result().sections, finalSpec(FINAL), specOf(m)])
    })

    it('выключенный мастер человека → мастер пресета идёт', () => {
      expect(draftSpecs([rec(), { ...humanMaster(), off: true }]))
        .toEqual([...result().sections, finalSpec(FINAL), masterSpec(MASTER)])
    })
  })

  describe('не перекрывают — идут вместе', () => {
    it('громкость человека (заглушить/тише) на той же дорожке', () => {
      const all = [pChain('drums'), pVolume('drums'), pPlace('drums')]
      expect(presetOnly(all, [mute('drums')])).toEqual(all)
      expect(presetOnly(all, [mute('drums', -101, -6)])).toEqual(all)
    })

    it('линия громкости человека на той же дорожке', () => {
      const all = [pChain('drums'), pVolume('drums')]
      expect(presetOnly(all, [stemEnv('drums')])).toEqual(all)
    })

    it('эффект человека на весь трек — final пресета остаётся', () => {
      const m = mixFx()
      expect(draftSpecs([rec(), m]))
        .toEqual([...result().sections, finalSpec(FINAL), masterSpec(MASTER), specOf(m)])
    })

    it('правка человека на другой дорожке', () => {
      expect(presetOnly([pChain('drums')], [stemFx('bass'), humanPlace('bass')])).toEqual([pChain('drums')])
    })
  })
})

describe('draftSpecs: выключенный пресет (ТК6)', () => {
  it('не раскрывается: ни sections, ни final, ни master', () => {
    const a = stemFx('bass')
    expect(draftSpecs([{ ...rec(), off: true }, a])).toEqual([specOf(a)])
  })
})
