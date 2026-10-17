// Счётчик включённых фильтров на кнопке «Фильтры» в узком окне (пачка 9, ТК21):
// поиск не в счёт — он виден всегда; неизвестные ключи не считаются.
import { describe, it, expect } from 'vitest'
import { activeFilterCount, defaultJobFilter } from './jobFilter.js'

const qf = (over = {}) => ({ ...defaultJobFilter(), ...over })

describe('activeFilterCount', () => {
  it('фильтр по умолчанию — 0', () => {
    expect(activeFilterCount(defaultJobFilter())).toBe(0)
  })

  it('включён статус — 1', () => {
    expect(activeFilterCount(qf({ status: 'done' }))).toBe(1)
  })

  it('статус и папка — 2', () => {
    expect(activeFilterCount(qf({ status: 'done', folder: 'Альбом' }))).toBe(2)
  })

  it('поиск не считается', () => {
    expect(activeFilterCount(qf({ q: 'x' }))).toBe(0)
    expect(activeFilterCount(qf({ q: 'x', status: 'done' }))).toBe(1)
  })

  it('неизвестные ключи не считаются', () => {
    expect(activeFilterCount(qf({ foo: 'bar', sort: 'name' }))).toBe(0)
  })
})
