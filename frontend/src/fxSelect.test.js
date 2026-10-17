import { describe, it, expect } from 'vitest'
import { CHAIN_GROUPS, groupOf, effectGroups, effectsIn } from './dspVoice.js'

// Два списка выбора эффекта: «Группа» (effectGroups) и «Эффект» внутри группы
// (effectsIn). Цепочки — как из биндинга YueDspChains: {id, name, group, voice?, pedal?}.

const ch = (id, group, over = {}) => ({ id, name: 'N-' + id, group, ...over })
const ids = list => list.map(c => c.id)

// вход намеренно перемешан по группам
const mixed = () => [
  ch('tape', 'lofi'),
  ch('megaphone', 'voice', { voice: true }),
  ch('reverb-hall', 'space'),
  ch('fuzz', 'pedals', { pedal: true }),
  ch('stutter', 'time'),
  ch('comp', 'dynamics'),
  ch('lowpass', 'filter'),
  ch('chorus', 'modulation'),
  ch('weird', 'nope'),
  ch('reverb-room', 'space'),
  ch('crush', 'lofi'),
]

describe('CHAIN_GROUPS — порядок групп', () => {
  it('space, pedals, modulation, filter, dynamics, time, lofi', () => {
    expect(CHAIN_GROUPS).toEqual(['space', 'pedals', 'modulation', 'filter', 'dynamics', 'time', 'lofi'])
  })
})

describe('groupOf — группа цепочки', () => {
  it('известная группа возвращается как есть', () => {
    for (const g of CHAIN_GROUPS) expect(groupOf(ch('x', g))).toBe(g)
  })

  it('неизвестная группа — other', () => {
    expect(groupOf(ch('x', 'nope'))).toBe('other')
  })

  it('группы нет (undefined, пустая строка) — other', () => {
    expect(groupOf(ch('x', undefined))).toBe('other')
    expect(groupOf({ id: 'x', name: 'X' })).toBe('other')
    expect(groupOf(ch('x', ''))).toBe('other')
  })

  it('voice не входит в CHAIN_GROUPS — other', () => {
    expect(groupOf(ch('megaphone', 'voice', { voice: true }))).toBe('other')
  })

  it('голосовая цепочка с группой space — space (флаг voice тут не влияет)', () => {
    expect(groupOf(ch('vrev', 'space', { voice: true }))).toBe('space')
  })

  it('педаль, помеченная pedal:true в группе space — space', () => {
    expect(groupOf(ch('spring', 'space', { pedal: true }))).toBe('space')
  })
})

describe('effectGroups — опции списка «Группа»', () => {
  it('порядок — CHAIN_GROUPS, затем other; педали скрыты', () => {
    expect(effectGroups(mixed())).toEqual(['space', 'modulation', 'filter', 'dynamics', 'time', 'lofi', 'other'])
  })

  it('пустые группы пропускаются', () => {
    expect(effectGroups([ch('tape', 'lofi'), ch('reverb', 'space')])).toEqual(['space', 'lofi'])
  })

  it('other — только если есть цепочки с неизвестной группой, и последней', () => {
    expect(effectGroups([ch('weird', 'nope'), ch('tape', 'lofi')])).toEqual(['lofi', 'other'])
    expect(effectGroups([ch('tape', 'lofi')])).not.toContain('other')
  })

  it('цепочка без группы попадает в other', () => {
    expect(effectGroups([ch('nogroup', undefined)])).toEqual(['other'])
  })

  it('педали исключены, даже если в группе есть цепочки', () => {
    expect(effectGroups([ch('fuzz', 'pedals'), ch('drive', 'pedals', { pedal: true })])).toEqual([])
  })

  it('педали исключены при currentId по умолчанию и при пустом currentId', () => {
    const chains = [ch('fuzz', 'pedals'), ch('tape', 'lofi')]
    expect(effectGroups(chains)).toEqual(['lofi'])
    expect(effectGroups(chains, '')).toEqual(['lofi'])
  })

  it('текущая цепочка — педаль: pedals показывается на своём месте', () => {
    expect(effectGroups(mixed(), 'fuzz'))
      .toEqual(['space', 'pedals', 'modulation', 'filter', 'dynamics', 'time', 'lofi', 'other'])
  })

  it('текущая цепочка — не педаль: pedals не добавляется', () => {
    expect(effectGroups(mixed(), 'tape')).not.toContain('pedals')
    expect(effectGroups(mixed(), 'reverb-hall')).not.toContain('pedals')
  })

  it('текущий id не найден: pedals не добавляется', () => {
    expect(effectGroups(mixed(), 'no-such-id')).not.toContain('pedals')
  })

  it('pedal:true в группе space не делает space педалью: space показан, pedals — нет', () => {
    const chains = [ch('spring', 'space', { pedal: true })]
    expect(effectGroups(chains)).toEqual(['space'])
    expect(effectGroups(chains, 'spring')).toEqual(['space'])
  })

  it('голосовые цепочки не дают группе появиться', () => {
    // группа voice (→ other) из одних голосовых — other не показывается
    expect(effectGroups([ch('megaphone', 'voice', { voice: true }), ch('tape', 'lofi')])).toEqual(['lofi'])
    // группа voice без флага — тоже голосовая
    expect(effectGroups([ch('phone', 'voice'), ch('tape', 'lofi')])).toEqual(['lofi'])
    // голосовая цепочка с группой space — space из одних голосовых не показан
    expect(effectGroups([ch('vrev', 'space', { voice: true }), ch('tape', 'lofi')])).toEqual(['lofi'])
  })

  it('группа с голосовой и обычной цепочкой показывается', () => {
    expect(effectGroups([ch('vrev', 'space', { voice: true }), ch('hall', 'space')])).toEqual(['space'])
  })

  it('пустой вход и только голосовые — пустой массив', () => {
    expect(effectGroups([])).toEqual([])
    expect(effectGroups([ch('megaphone', 'voice', { voice: true })])).toEqual([])
  })

  it('null/undefined — пустой массив', () => {
    expect(effectGroups(null)).toEqual([])
    expect(effectGroups(undefined)).toEqual([])
    expect(effectGroups(null, 'fuzz')).toEqual([])
  })

  it('вход не изменяется', () => {
    const chains = mixed()
    const snapshot = JSON.parse(JSON.stringify(chains))
    effectGroups(chains, 'fuzz')
    effectGroups(chains)
    expect(chains).toEqual(snapshot)
  })
})

describe('effectsIn — опции списка «Эффект» внутри группы', () => {
  it('цепочки своей группы в порядке входа', () => {
    expect(ids(effectsIn(mixed(), 'space'))).toEqual(['reverb-hall', 'reverb-room'])
    expect(ids(effectsIn(mixed(), 'lofi'))).toEqual(['tape', 'crush'])
  })

  it('возвращает сами объекты цепочек', () => {
    const chains = mixed()
    const res = effectsIn(chains, 'lofi')
    expect(res[0]).toBe(chains[0])
    expect(res[1]).toBe(chains[10])
  })

  it('педали доступны в группе pedals', () => {
    expect(ids(effectsIn(mixed(), 'pedals'))).toEqual(['fuzz'])
  })

  it('pedal:true в группе space — в space, не в pedals', () => {
    const chains = [ch('spring', 'space', { pedal: true }), ch('fuzz', 'pedals')]
    expect(ids(effectsIn(chains, 'space'))).toEqual(['spring'])
    expect(ids(effectsIn(chains, 'pedals'))).toEqual(['fuzz'])
  })

  it('other — цепочки с неизвестной группой и без группы', () => {
    const chains = [ch('weird', 'nope'), ch('tape', 'lofi'), ch('nogroup', undefined)]
    expect(ids(effectsIn(chains, 'other'))).toEqual(['weird', 'nogroup'])
  })

  it('голосовые цепочки исключены', () => {
    const chains = [
      ch('megaphone', 'voice', { voice: true }),
      ch('phone', 'voice'),
      ch('vrev', 'space', { voice: true }),
      ch('hall', 'space'),
      ch('weird', 'nope'),
    ]
    expect(ids(effectsIn(chains, 'space'))).toEqual(['hall'])
    expect(ids(effectsIn(chains, 'other'))).toEqual(['weird'])
    expect(effectsIn(chains, 'voice')).toEqual([])
  })

  it('пустая группа — пустой массив', () => {
    expect(effectsIn(mixed(), 'nope-there')).toEqual([])
    expect(effectsIn([ch('tape', 'lofi')], 'space')).toEqual([])
  })

  it('неизвестная группа — пустой массив (даже если у цепочки такое поле group)', () => {
    expect(effectsIn([ch('weird', 'nope')], 'nope')).toEqual([])
  })

  it('пустой вход, null и undefined — пустой массив', () => {
    expect(effectsIn([], 'space')).toEqual([])
    expect(effectsIn(null, 'space')).toEqual([])
    expect(effectsIn(undefined, 'space')).toEqual([])
  })

  it('вход не изменяется', () => {
    const chains = mixed()
    const snapshot = JSON.parse(JSON.stringify(chains))
    effectsIn(chains, 'space')
    effectsIn(chains, 'other')
    expect(chains).toEqual(snapshot)
  })
})
