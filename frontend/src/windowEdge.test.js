// Тесты поправки края растягивания окна без системной рамки (frameEdge):
// верх — полоса заголовка, за неё не растягиваем; верхние углы тянут только
// по горизонтали; левый/правый/нижний края и нижние углы — как есть;
// нет края — нет растягивания; в развёрнутом окне края не растягивают вовсе.
import { describe, it, expect } from 'vitest'
import { frameEdge } from './windowEdge.js'

const ALL_EDGES = ['n-resize', 'nw-resize', 'ne-resize', 'w-resize', 'e-resize', 's-resize', 'sw-resize', 'se-resize']

describe('верхний край и верхние углы (ТК1)', () => {
  it('верхний край не растягивает — там заголовок', () => {
    expect(frameEdge('n-resize', false)).toBeUndefined()
  })

  it('угол сверху слева растягивает только влево', () => {
    expect(frameEdge('nw-resize', false)).toBe('w-resize')
  })

  it('угол сверху справа растягивает только вправо', () => {
    expect(frameEdge('ne-resize', false)).toBe('e-resize')
  })
})

describe('боковые, нижний край и нижние углы (ТК2)', () => {
  for (const edge of ['w-resize', 'e-resize', 's-resize', 'sw-resize', 'se-resize']) {
    it(`${edge} — без изменений`, () => {
      expect(frameEdge(edge, false)).toBe(edge)
    })
  }
})

describe('нет края (ТК3)', () => {
  it('undefined — растягивания нет', () => {
    expect(frameEdge(undefined, false)).toBeUndefined()
  })

  it('пустая строка — растягивания нет', () => {
    expect(frameEdge('', false)).toBeUndefined()
  })
})

describe('развёрнутое окно (ТК4)', () => {
  for (const edge of ALL_EDGES) {
    it(`${edge} в развёрнутом окне — растягивания нет`, () => {
      expect(frameEdge(edge, true)).toBeUndefined()
    })
  }

  it('нет края в развёрнутом окне — тоже нет', () => {
    expect(frameEdge(undefined, true)).toBeUndefined()
    expect(frameEdge('', true)).toBeUndefined()
  })
})
