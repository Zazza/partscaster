// Тесты по карточке internal-evening-fixes, условия 1–2 (ТК1, ТК2) — от условий, не от реализации.
import { describe, it, expect } from 'vitest'
import {
  SLOT_ORDER, pickMultiSlotTag, applyParsedSlots, styleExtra, parseStyleLine,
} from './styleLogic.js'
import { formStyle } from './requests.js'
import { slotOptions } from './slotOptions.js'

// ТК1: текст после последней запятой (или всё поле без запятых) — черновик поиска, если он не
// совпадает целиком с готовым значением подсказок (ru или en, любой слот); совпадающий сохраняется.
describe('pickMultiSlotTag — выбор заменяет черновик поиска (условие 1)', () => {
  it('черновик без запятой заменяется выбранным', () => {
    expect(pickMultiSlotTag('мет', 'метал')).toBe('метал, ')
  })

  it('пустое поле — выбранное значение и запятая', () => {
    expect(pickMultiSlotTag('', 'метал')).toBe('метал, ')
  })

  it('черновик после запятой заменяется, выбранное раньше остаётся', () => {
    expect(pickMultiSlotTag('рок, мет', 'метал')).toBe('рок, метал, ')
  })

  it('поле — готовое значение подсказок без запятой: выбор другого даёт оба значения', () => {
    expect(pickMultiSlotTag('блюз', 'дум-метал')).toBe('блюз, дум-метал, ')
  })

  it('готовое значение другого слота тоже сохраняется (любой слот)', () => {
    // «Русский» — подсказка слота language, «рояль» — keys; выбор идёт в мультиполе жанра
    expect(slotOptions.language.some(([ru]) => ru === 'Русский')).toBe(true)
    expect(slotOptions.keys.some(([ru]) => ru === 'рояль')).toBe(true)
    expect(pickMultiSlotTag('Русский', 'блюз')).toBe('Русский, блюз, ')
    expect(pickMultiSlotTag('рояль', 'блюз')).toBe('рояль, блюз, ')
  })

  it("('рок, <готовое>', X) → 'рок, <готовое>, X, '", () => {
    expect(pickMultiSlotTag('рок, блюз', 'дум-метал')).toBe('рок, блюз, дум-метал, ')
  })

  it('самописный обрывок без запятой (не значение подсказок) заменяется выбранным', () => {
    expect(pickMultiSlotTag('тяжеляк', 'дум-метал')).toBe('дум-метал, ')
  })

  it('выбор уже имеющегося значения — без дубля', () => {
    expect(pickMultiSlotTag('блюз', 'блюз')).toBe('блюз, ')
    expect(pickMultiSlotTag('рок, блюз, ', 'блюз')).toBe('рок, блюз, ')
    expect(pickMultiSlotTag('рок, блюз', 'блюз')).toBe('рок, блюз, ')
  })

  it('выбор уже имеющегося при черновике в хвосте — черновик уходит, дубля нет', () => {
    expect(pickMultiSlotTag('блюз, бл', 'блюз')).toBe('блюз, ')
  })

  it('en-подписи: черновик заменяется', () => {
    expect(pickMultiSlotTag('doo', 'doom metal')).toBe('doom metal, ')
    expect(pickMultiSlotTag('blues, doo', 'doom metal')).toBe('blues, doom metal, ')
  })

  it('en-подписи: готовое значение без запятой сохраняется', () => {
    expect(slotOptions.genre.some(([, en]) => en === 'blues')).toBe(true)
    expect(pickMultiSlotTag('blues', 'doom metal')).toBe('blues, doom metal, ')
    expect(pickMultiSlotTag('grand piano', 'blues')).toBe('grand piano, blues, ')
  })

  it('en-подписи: самописный обрывок заменяется, повтор без дубля', () => {
    expect(pickMultiSlotTag('heavyish', 'doom metal')).toBe('doom metal, ')
    expect(pickMultiSlotTag('blues', 'blues')).toBe('blues, ')
  })
})

// ТК2: «Разобрать по полям» точный
const filled = () => ({
  language: 'English', genre: 'панк', rhythm: 'медленно', guitars: 'слайд-гитара',
  keys: 'рояль', vocals: 'мужской фальцет', mood: 'романтично', production: 'узкое моно', bpm: 140,
})

describe('applyParsedSlots — поля, которых нет в строке, очищаются (условие 2)', () => {
  it('слоты SLOT_ORDER берутся из res.slots, отсутствующие — пустые; bpm из res', () => {
    const res = { slots: { genre: 'блюз', guitars: 'слайд-гитара' }, bpm: 120, rack: [], unknown: [] }
    const out = applyParsedSlots(filled(), res)
    for (const k of SLOT_ORDER) {
      expect(out[k], k).toBe(res.slots[k] ?? '')
    }
    expect(out.bpm).toBe(120)
  })

  it('BPM нет в строке — bpm становится null, старый не остаётся', () => {
    const out = applyParsedSlots(filled(), { slots: { genre: 'блюз' }, bpm: null, rack: [], unknown: [] })
    expect(out.bpm).toBeNull()
  })

  it('пустой разбор — все слоты пустые, bpm null', () => {
    const out = applyParsedSlots(filled(), { slots: {}, bpm: null, rack: [], unknown: [] })
    for (const k of SLOT_ORDER) expect(out[k], k).toBe('')
    expect(out.bpm).toBeNull()
  })

  it('исходный объект слотов не мутирует', () => {
    const src = filled()
    const snapshot = JSON.parse(JSON.stringify(src))
    const out = applyParsedSlots(src, { slots: { genre: 'блюз' }, bpm: null, rack: [], unknown: [] })
    expect(src).toEqual(snapshot)
    expect(out).not.toBe(src)
  })

  it('с настоящим разбором parseStyleLine: старые значения не остаются', () => {
    const res = parseStyleLine('Russian, блюз, 120 BPM')
    const out = applyParsedSlots(filled(), res)
    expect(out.language).toBe('Русский')
    expect(out.genre).toBe('блюз')
    expect(out.bpm).toBe(120)
    for (const k of ['rhythm', 'guitars', 'keys', 'vocals', 'mood', 'production']) {
      expect(out[k], k).toBe('')
    }
  })
})

describe('styleExtra — стойка и нераспознанные куски одной строкой (условие 2)', () => {
  it('стойка и куски через запятую', () => {
    expect(styleExtra('tape echo', ['странный тег', 'ещё кусок'])).toBe('tape echo, странный тег, ещё кусок')
  })

  it('пустая стойка выпадает', () => {
    expect(styleExtra('', ['странный тег'])).toBe('странный тег')
  })

  it('нет нераспознанных — только стойка', () => {
    expect(styleExtra('tape echo', [])).toBe('tape echo')
  })

  it('пустые куски выпадают', () => {
    expect(styleExtra('tape echo', ['', 'странный тег', ''])).toBe('tape echo, странный тег')
  })

  it("('', []) → ''", () => {
    expect(styleExtra('', [])).toBe('')
  })
})

describe('нераспознанные куски уходят в отправку (условие 2)', () => {
  it("formStyle('', slots, styleExtra(...)) — нераспознанный кусок в конце строки", () => {
    const slots = applyParsedSlots(filled(), { slots: { genre: 'блюз' }, bpm: null, rack: [], unknown: [] })
    const s = formStyle('', slots, styleExtra('', ['странный тег']))
    expect(s.endsWith('странный тег')).toBe(true)
  })

  it('со стойкой: кусок дописан после стойки, в конце', () => {
    const slots = applyParsedSlots(filled(), { slots: { genre: 'блюз' }, bpm: null, rack: [], unknown: [] })
    const s = formStyle('', slots, styleExtra('tape echo', ['странный тег']))
    expect(s).toContain('tape echo')
    expect(s.endsWith('странный тег')).toBe(true)
    expect(s.indexOf('tape echo')).toBeLessThan(s.indexOf('странный тег'))
  })
})
