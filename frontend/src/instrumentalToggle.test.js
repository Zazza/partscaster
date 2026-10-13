// Отметка «без слов» (internal-genres-order, ТК4) — по спецификации:
// вкл → слот голоса становится «instrumental, no vocals», прежний голос запоминается;
// выкл → прежний голос стиля возвращается; правку пользователя не затираем; вход не мутируется.
import { describe, it, expect } from 'vitest'
import * as sl from './styleLogic.js'

const INSTR = 'instrumental, no vocals'
const toggle = (...a) => sl.withInstrumental(...a)

describe('INSTRUMENTAL_VOCALS', () => {
  it('значение слота голоса для «без слов» — «instrumental, no vocals»', () => {
    expect(sl.INSTRUMENTAL_VOCALS).toBe(INSTR)
  })
})

describe('withInstrumental: включение', () => {
  it('голос стиля заменяется на «instrumental, no vocals», прежний — в kept', () => {
    const slots = { genre: 'punk', vocals: 'shouted male vocals' }
    const r = toggle(slots, true)
    expect(r.slots.vocals).toBe(INSTR)
    expect(r.slots.genre).toBe('punk')
    expect(r.kept).toBe('shouted male vocals')
  })

  it('пустой голос → kept пустой, слот всё равно «instrumental, no vocals»', () => {
    const r = toggle({ genre: 'punk', vocals: '' }, true)
    expect(r.slots.vocals).toBe(INSTR)
    expect(r.kept).toBe('')
  })

  it('голос уже «instrumental, no vocals» → kept не меняется (повторное включение не теряет прежний голос)', () => {
    const r = toggle({ vocals: INSTR }, true, 'soft female vocals')
    expect(r.slots.vocals).toBe(INSTR)
    expect(r.kept).toBe('soft female vocals')
  })

  it('вход не мутируется', () => {
    const slots = { genre: 'punk', vocals: 'shouted male vocals' }
    const copy = { ...slots }
    const r = toggle(slots, true)
    expect(slots).toEqual(copy)
    expect(r.slots).not.toBe(slots)
  })
})

describe('withInstrumental: выключение', () => {
  it('прежний голос стиля возвращается, kept очищается', () => {
    const r = toggle({ genre: 'punk', vocals: INSTR }, false, 'shouted male vocals')
    expect(r.slots.vocals).toBe('shouted male vocals')
    expect(r.slots.genre).toBe('punk')
    expect(r.kept).toBe('')
  })

  it('пользователь сам поменял голос, пока отметка стояла → голос не трогается, kept очищается', () => {
    const r = toggle({ vocals: 'whispered vocals' }, false, 'shouted male vocals')
    expect(r.slots.vocals).toBe('whispered vocals')
    expect(r.kept).toBe('')
  })

  it('без сохранённого голоса (kept по умолчанию) → слот голоса пустой', () => {
    const r = toggle({ vocals: INSTR }, false)
    expect(r.slots.vocals).toBe('')
    expect(r.kept).toBe('')
  })

  it('вход не мутируется', () => {
    const slots = { vocals: INSTR }
    const r = toggle(slots, false, 'baritone vocals')
    expect(slots).toEqual({ vocals: INSTR })
    expect(r.slots).not.toBe(slots)
  })
})

describe('withInstrumental: туда-обратно', () => {
  it('вкл → выкл возвращает исходные слоты', () => {
    const slots = { language: 'English', genre: 'post-punk', vocals: 'deadpan baritone', mood: 'cold' }
    const on = toggle(slots, true)
    const off = toggle(on.slots, false, on.kept)
    expect(off.slots).toEqual(slots)
    expect(off.kept).toBe('')
  })

  it('вкл → вкл → выкл тоже возвращает исходный голос', () => {
    const slots = { vocals: 'deadpan baritone' }
    const a = toggle(slots, true)
    const b = toggle(a.slots, true, a.kept)
    const c = toggle(b.slots, false, b.kept)
    expect(c.slots.vocals).toBe('deadpan baritone')
  })
})
