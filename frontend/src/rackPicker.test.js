// Тесты стойки инструментов (пачка 8): добавление/удаление позиций без мутации входа (ТК18)
// и состав каталога позиций с новыми инструментами (ТК19).
import { describe, it, expect } from 'vitest'
import { rackAdd, rackRemove, allRackItems, rackGroups } from './rack.js'

describe('rackAdd', () => {
  it('добавление в пустой выбор → одна позиция с id и эффектом', () => {
    expect(rackAdd([], 'flute', '')).toEqual([{ id: 'flute', effect: '' }])
  })

  it('второй инструмент добавляется в конец', () => {
    const sel = [{ id: 'flute', effect: '' }]
    expect(rackAdd(sel, 'trumpet', 'reverb')).toEqual([
      { id: 'flute', effect: '' },
      { id: 'trumpet', effect: 'reverb' },
    ])
  })

  it('тот же id с другим эффектом — дубля нет, эффект заменён, место то же', () => {
    const sel = [
      { id: 'flute', effect: '' },
      { id: 'trumpet', effect: '' },
      { id: 'sax', effect: '' },
    ]
    const out = rackAdd(sel, 'trumpet', 'reverb')
    expect(out).toHaveLength(3)
    expect(out.map(x => x.id)).toEqual(['flute', 'trumpet', 'sax'])
    expect(out[1]).toEqual({ id: 'trumpet', effect: 'reverb' })
  })

  it('вход не мутируется: ни массив, ни его элементы', () => {
    const sel = [{ id: 'flute', effect: '' }]
    const snapshot = JSON.parse(JSON.stringify(sel))
    const first = sel[0]
    rackAdd(sel, 'trumpet', '')
    rackAdd(sel, 'flute', 'reverb')
    expect(sel).toEqual(snapshot)
    expect(sel[0]).toBe(first)
    expect(first).toEqual({ id: 'flute', effect: '' })
  })

  it('результат — новый массив, а не тот же объект', () => {
    const sel = [{ id: 'flute', effect: '' }]
    expect(rackAdd(sel, 'flute', 'reverb')).not.toBe(sel)
  })
})

describe('rackRemove', () => {
  it('убирает позицию по id, порядок остальных сохраняется', () => {
    const sel = [
      { id: 'flute', effect: '' },
      { id: 'trumpet', effect: 'reverb' },
      { id: 'sax', effect: '' },
    ]
    expect(rackRemove(sel, 'trumpet')).toEqual([
      { id: 'flute', effect: '' },
      { id: 'sax', effect: '' },
    ])
  })

  it('неизвестный id — выбор без изменений', () => {
    const sel = [{ id: 'flute', effect: '' }]
    expect(rackRemove(sel, 'no-such')).toEqual([{ id: 'flute', effect: '' }])
  })

  it('пустой выбор → пустой', () => {
    expect(rackRemove([], 'flute')).toEqual([])
  })

  it('вход не мутируется', () => {
    const sel = [{ id: 'flute', effect: '' }, { id: 'sax', effect: 'reverb' }]
    const snapshot = JSON.parse(JSON.stringify(sel))
    rackRemove(sel, 'flute')
    rackRemove(sel, 'no-such')
    expect(sel).toEqual(snapshot)
  })
})

describe('allRackItems — каталог позиций', () => {
  const items = allRackItems()

  it('id всех позиций уникальны', () => {
    const ids = items.map(i => i.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('у каждой позиции есть name и nameEn', () => {
    for (const it_ of items) {
      expect(typeof it_.name, it_.id).toBe('string')
      expect(it_.name.trim().length, it_.id).toBeGreaterThan(0)
      expect(typeof it_.nameEn, it_.id).toBe('string')
      expect(it_.nameEn.trim().length, it_.id).toBeGreaterThan(0)
    }
  })

  it('phrasing — непустая английская строка (только ASCII)', () => {
    for (const it_ of items) {
      expect(typeof it_.phrasing, it_.id).toBe('string')
      expect(it_.phrasing.trim().length, it_.id).toBeGreaterThan(0)
      expect(it_.phrasing, it_.id).toMatch(/^[\x20-\x7E]+$/)
    }
  })

  it('новые позиции из условия 37 присутствуют (с поправкой условия 40)', () => {
    const ids = new Set(items.map(i => i.id))
    const expected = [
      'trumpet', 'sax', 'trombone', 'horns',
      'strings-section', 'mandolin', 'banjo', 'gtr-12', 'sitar',
      'mellotron', 'wurlitzer', 'harpsichord', 'vibraphone', 'marimba',
      'harmonica', 'theremin', 'whistle',
    ]
    const missing = expected.filter(id => !ids.has(id))
    expect(missing).toEqual([])
  })

  // условие 40: по опыту #1099–#1144 перкуссия не слышна, 808 и брейкбит глушат барабаны — их нет;
  // 12-струнная и ситар звучат как гитара — они в группе «Гитары»
  it('убранных позиций и группы «Перкуссия» нет; 12-струнная и ситар — в «Гитарах»', () => {
    const ids = new Set(items.map(i => i.id))
    for (const id of ['tambourine', 'shaker', 'congas', 'cowbell', 'tr808', 'breakbeat']) {
      expect(ids.has(id), id).toBe(false)
    }
    expect(rackGroups.some(g => g.id === 'perc')).toBe(false)
    const guitars = rackGroups.find(g => g.id === 'guitars').items.map(i => i.id)
    expect(guitars).toEqual(expect.arrayContaining(['gtr-12', 'sitar']))
  })
})
