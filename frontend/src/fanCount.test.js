// Тесты fanCount — сколько вариантов трека с разными seed ставить в очередь кнопкой «Варианты: N».
// Источник — карточка internal-window-frame, пачка 4, условие 21 и ТК9: целое 2..5, мусор → 3.
import { describe, it, expect } from 'vitest'
import { fanCount } from './fanCount.js'

describe('допустимые значения', () => {
  it('2, 3, 4, 5 возвращаются как есть', () => {
    for (const n of [2, 3, 4, 5]) expect(fanCount(n)).toBe(n)
  })

  it('строка с числом из хранилища — то же число', () => {
    expect(fanCount('4')).toBe(4)
  })
})

describe('выход за границы — к ближайшей границе', () => {
  it('1 → 2 (меньше двух вариантов не бывает)', () => {
    expect(fanCount(1)).toBe(2)
  })

  it('6 → 5 (больше пяти не бывает)', () => {
    expect(fanCount(6)).toBe(5)
  })
})

describe('дробное — округляется', () => {
  it('2.7 → 3', () => {
    expect(fanCount(2.7)).toBe(3)
  })
})

describe('мусор — значение по умолчанию 3', () => {
  it('null, undefined, не число, NaN → 3', () => {
    expect(fanCount(null)).toBe(3)
    expect(fanCount(undefined)).toBe(3)
    expect(fanCount('x')).toBe(3)
    expect(fanCount(NaN)).toBe(3)
  })
})

describe('результат всегда целое число 2..5', () => {
  it('на любом входе из карточки — целое в диапазоне', () => {
    for (const v of [2, 3, 4, 5, 1, 6, '4', null, undefined, 'x', NaN, 2.7]) {
      const r = fanCount(v)
      expect(Number.isInteger(r)).toBe(true)
      expect(r).toBeGreaterThanOrEqual(2)
      expect(r).toBeLessThanOrEqual(5)
    }
  })
})
