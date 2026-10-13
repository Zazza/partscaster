// Задача internal-instruments-order: инструменты в три шага «семейство → подвид → инструмент».
// presetGroups.js: FAMILIES — семейства по порядку; PRESET_GROUPS[g].family — семейство группы (ru/en — подпись
// подвида); instrumentTree(list, locale, otherLabel) → [{family, label, subs:[{group, label, items}]}];
// familyOptions/subOptions — пункты списков; pathOf(tree, id) → {family, group}; groupPresets подписывает группу
// «<семейство> · <подвид>». Данные — реальные fxPresets.js. mergeInstruments: свой инструмент — в группе основы.
import { describe, it, expect } from 'vitest'
import {
  FAMILIES, PRESET_GROUPS, instrumentTree, familyOptions, subOptions, pathOf, groupPresets,
} from './presetGroups.js'
import { fxPresets } from './fxPresets.js'
import { mergeInstruments } from './myInstruments.js'

// клавиши и синтезаторы — одно семейство (решение человека 2026-10-13)
const FAMILY_ORDER = ['guitar', 'bass', 'drums', 'perc', 'keys', 'orch', 'fx']
const FAMILY_RU = ['Гитара', 'Бас', 'Барабаны', 'Перкуссия', 'Клавиши и синтезаторы', 'Оркестр', 'Обработка']

// Семейство пресета по его группе (или undefined)
const familyOf = (p) => PRESET_GROUPS[p.group]?.family
// Группы семейства из PRESET_GROUPS (порядок объявления)
const groupsOfFamily = (fam) => Object.keys(PRESET_GROUPS).filter((g) => PRESET_GROUPS[g].family === fam)
const famEn = (fam) => FAMILIES.find((f) => f.id === fam).en

const p = (id, group) => ({ id, ...(group === undefined ? {} : { group }), name: { ru: id, en: id } })
const shape = (tree) => tree.map((f) => [f.family, f.subs.map((s) => [s.group, s.items.map((x) => x.id)])])

describe('FAMILIES — семь семейств по порядку (условие 1)', () => {
  it('id и русские подписи — по порядку карточки', () => {
    expect(FAMILIES.map((f) => f.id)).toEqual(FAMILY_ORDER)
    expect(FAMILIES.map((f) => f.ru)).toEqual(FAMILY_RU)
  })

  it('у каждого семейства непустая английская подпись', () => {
    for (const f of FAMILIES) {
      expect(typeof f.en, f.id).toBe('string')
      expect(f.en.trim(), f.id).not.toBe('')
    }
  })
})

describe('PRESET_GROUPS — у каждой группы семейство (условия 1, 2)', () => {
  it('family каждой группы — id из FAMILIES', () => {
    for (const [g, v] of Object.entries(PRESET_GROUPS)) expect(FAMILY_ORDER, `группа ${g}`).toContain(v.family)
  })

  it('подписи подвида ru/en непустые', () => {
    for (const [g, v] of Object.entries(PRESET_GROUPS)) {
      expect(String(v.ru || '').trim(), `${g}.ru`).not.toBe('')
      expect(String(v.en || '').trim(), `${g}.en`).not.toBe('')
    }
  })
})

describe('instrumentTree — семейство → подвид → инструмент (условие 3)', () => {
  // Две группы баса и одна гитары — берём из PRESET_GROUPS, не из названий
  const [gB1, gB2] = groupsOfFamily('bass')
  const [gG] = groupsOfFamily('guitar')
  // бас идёт первым в списке, но семейство «Гитара» — раньше по FAMILIES
  const list = () => [p('b1', gB2), p('g1', gG), p('b2', gB1), p('b3', gB2), p('x'), p('y', 'weird-group')]

  it('в данных есть хотя бы два подвида баса и один гитары', () => {
    expect(gB1 && gB2 && gG).toBeTruthy()
  })

  it('семейства — в порядке FAMILIES, подвиды — по порядку PRESET_GROUPS (неизвестные — по появлению), пункты — исходный порядок', () => {
    const tree = instrumentTree(list(), 'ru', 'Другое')
    expect(shape(tree)).toEqual([
      ['guitar', [[gG, ['g1']]]],
      ['bass', [[gB1, ['b2']], [gB2, ['b1', 'b3']]]],
      ['other', [['', ['x']], ['weird-group', ['y']]]],
    ])
  })

  it('подписи: семейство — из FAMILIES, подвид — из PRESET_GROUPS; «другое» — otherLabel, подвиды без подписи', () => {
    const tree = instrumentTree(list(), 'ru', 'Другое')
    expect(tree.map((f) => f.label)).toEqual(['Гитара', 'Бас', 'Другое'])
    expect(tree[1].subs.map((s) => s.label)).toEqual([PRESET_GROUPS[gB1].ru, PRESET_GROUPS[gB2].ru])
    expect(tree[2].subs.map((s) => s.label)).toEqual(['', ''])
  })

  it('locale en → английские подписи', () => {
    const tree = instrumentTree(list(), 'en', 'Other')
    expect(tree.map((f) => f.label)).toEqual([famEn('guitar'), famEn('bass'), 'Other'])
    expect(tree[0].subs[0].label).toBe(PRESET_GROUPS[gG].en)
  })

  it('locale и otherLabel по умолчанию — ru и пустая подпись', () => {
    const tree = instrumentTree(list())
    expect(tree[0].label).toBe('Гитара')
    expect(tree.at(-1)).toMatchObject({ family: 'other', label: '' })
  })

  it('пустые семейства не выводятся; без «чужих» пресетов нет и «другого»', () => {
    const tree = instrumentTree([p('b1', gB1)], 'ru', 'Другое')
    expect(tree.map((f) => f.family)).toEqual(['bass'])
  })

  it('пункты — сами пресеты', () => {
    const l = list()
    expect(instrumentTree(l, 'ru')[0].subs[0].items[0]).toBe(l[1])
  })

  it('пустой список → []', () => {
    expect(instrumentTree([], 'ru', 'Другое')).toEqual([])
  })

  it('чистая: вход не мутируется', () => {
    const l = list()
    const before = JSON.parse(JSON.stringify(l))
    instrumentTree(l, 'ru', 'Другое')
    expect(l).toEqual(before)
  })
})

describe('familyOptions / subOptions — пункты списков', () => {
  const [gB1, gB2] = groupsOfFamily('bass')
  const [gG] = groupsOfFamily('guitar')
  const tree = () => instrumentTree([p('b1', gB1), p('g1', gG), p('b2', gB2), p('z')], 'ru', 'Другое')

  it('familyOptions: value = семейство, label — его подпись, порядок дерева', () => {
    expect(familyOptions(tree())).toEqual([
      { value: 'guitar', label: 'Гитара' },
      { value: 'bass', label: 'Бас' },
      { value: 'other', label: 'Другое' },
    ])
  })

  it('familyOptions([]) → []', () => {
    expect(familyOptions([])).toEqual([])
  })

  it('subOptions: подвиды узла семейства', () => {
    const bass = tree().find((f) => f.family === 'bass')
    expect(subOptions(bass)).toEqual([
      { value: gB1, label: PRESET_GROUPS[gB1].ru },
      { value: gB2, label: PRESET_GROUPS[gB2].ru },
    ])
  })

  it('subOptions(undefined) → []', () => {
    expect(subOptions(undefined)).toEqual([])
  })
})

describe('pathOf — где лежит инструмент (ТК6)', () => {
  const [gB1, gB2] = groupsOfFamily('bass')
  const [gG] = groupsOfFamily('guitar')
  const tree = () => instrumentTree([p('b1', gB2), p('g1', gG), p('b2', gB1), p('z', 'weird')], 'ru', 'Другое')

  it('найден → его семейство и подвид', () => {
    expect(pathOf(tree(), 'b2')).toEqual({ family: 'bass', group: gB1 })
    expect(pathOf(tree(), 'g1')).toEqual({ family: 'guitar', group: gG })
    expect(pathOf(tree(), 'z')).toEqual({ family: 'other', group: 'weird' })
  })

  it('не найден (или пустой id) → первое семейство и его первый подвид', () => {
    expect(pathOf(tree(), 'nope')).toEqual({ family: 'guitar', group: gG })
    expect(pathOf(tree(), '')).toEqual({ family: 'guitar', group: gG })
  })

  it('пустое дерево → пустой путь', () => {
    expect(pathOf([], 'b1')).toEqual({ family: '', group: '' })
  })
})

describe('groupPresets — подпись «семейство · подвид» (условие 6)', () => {
  const [gB1] = groupsOfFamily('bass')
  const [gG] = groupsOfFamily('guitar')

  it('ru и en', () => {
    const list = [p('b', gB1), p('g', gG)]
    expect(groupPresets(list, 'ru').map((g) => g.label))
      .toEqual([`Бас · ${PRESET_GROUPS[gB1].ru}`, `Гитара · ${PRESET_GROUPS[gG].ru}`])
    expect(groupPresets(list, 'en').map((g) => g.label))
      .toEqual([`${famEn('bass')} · ${PRESET_GROUPS[gB1].en}`, `${famEn('guitar')} · ${PRESET_GROUPS[gG].en}`])
  })

  it('порядок групп и состав — как раньше (первое появление)', () => {
    const out = groupPresets([p('g', gG), p('b', gB1), p('g2', gG)], 'ru')
    expect(out.map((g) => [g.group, g.items.map((x) => x.id)])).toEqual([[gG, ['g', 'g2']], [gB1, ['b']]])
  })
})

describe('данные fxPresets: семейства готовых инструментов', () => {
  it('ТК1: у каждой готовой цепочки группа из PRESET_GROUPS и семейство из списка, ни одной без группы', () => {
    for (const x of fxPresets) {
      expect(x.group, `${x.id}: нет группы`).toBeTruthy()
      expect(PRESET_GROUPS[x.group], `${x.id}: группа ${x.group} не объявлена`).toBeTruthy()
      expect(FAMILY_ORDER, `${x.id}: семейство`).toContain(familyOf(x))
    }
  })

  it('ТК1: каждое из восьми семейств непустое', () => {
    const used = new Set(fxPresets.map(familyOf))
    for (const f of FAMILY_ORDER) expect(used.has(f), `семейство ${f} пустое`).toBe(true)
  })

  it('ТК1: в дереве реальных пресетов нет семейства «другое»', () => {
    expect(instrumentTree(fxPresets, 'ru', 'Другое').map((f) => f.family)).not.toContain('other')
  })

  it('ТК2: басовые цепочки (звук / жанр / сэмплы) и синт-бас — все в семействе «Бас»', () => {
    const bass = fxPresets.filter((x) => x.id.startsWith('bass-'))
    expect(bass.length).toBeGreaterThan(0)
    for (const x of bass) expect(familyOf(x), x.id).toBe('bass')
    expect(familyOf(fxPresets.find((x) => x.id === 'synth-moog-bass'))).toBe('bass')
  })

  it('ТК2: у баса подвиды различаются — «другой звук тех же нот», «под жанр», «замена нот сэмплами» — разные группы', () => {
    const g = (id) => fxPresets.find((x) => x.id === id).group
    // bass-melodic — звук, bass-vintage — жанр, bass-kit — сэмплы
    expect(new Set([g('bass-melodic'), g('bass-vintage'), g('bass-kit')]).size).toBe(3)
    expect(groupsOfFamily('bass').length).toBeGreaterThanOrEqual(3)
  })

  it('ТК3: барабанные наборы — подвид на набор: 6 наборов, части одного набора в одной группе, семейство «Барабаны»', () => {
    const kits = fxPresets.filter((x) => x.chain?.[0]?.type === 'sampler')
    const kitOf = (x) => String(x.chain[0].kit || '').split('/')[0]
    const byKit = new Map()
    for (const x of kits) {
      expect(familyOf(x), x.id).toBe('drums')
      if (!byKit.has(kitOf(x))) byKit.set(kitOf(x), new Set())
      byKit.get(kitOf(x)).add(x.group)
    }
    expect([...byKit.keys()].sort()).toEqual(['cr78', 'linn', 'osdk', 'simmons', 'tr808', 'tr909'])
    for (const [kit, groups] of byKit) expect(groups.size, `набор ${kit} разнесён по группам`).toBe(1)
    const all = [...byKit.values()].map((s) => [...s][0])
    expect(new Set(all).size, 'разные наборы — разные подвиды').toBe(6)
  })

  it('перкуссия (дорожка perc) — семейство «Перкуссия»', () => {
    const perc = fxPresets.filter((x) => (x.stems || []).length === 1 && x.stems[0] === 'perc')
    expect(perc.length).toBeGreaterThan(0)
    for (const x of perc) expect(familyOf(x), x.id).toBe('perc')
  })

  it('обработка: мастер, голос, комната, synth-delay — семейство «Обработка»', () => {
    const ids = fxPresets.filter((x) => x.id.startsWith('master-')).map((x) => x.id)
    expect(ids.length).toBeGreaterThan(0)
    for (const id of [...ids, 'vocal-plate', 'drums-room', 'synth-delay']) {
      expect(familyOf(fxPresets.find((x) => x.id === id)), id).toBe('fx')
    }
  })

  it('гитара: все guitar-* в «Гитаре», подвиды — чистые / перегруз / пространство / через усилитель', () => {
    const gtr = fxPresets.filter((x) => x.id.startsWith('guitar-'))
    for (const x of gtr) expect(familyOf(x), x.id).toBe('guitar')
    const g = (id) => fxPresets.find((x) => x.id === id).group
    expect(new Set([g('guitar-clean'), g('guitar-fuzz'), g('guitar-shoegaze'), g('guitar-amp-clean')]).size).toBe(4)
  })

  it('оркестр: все orch-* в «Оркестре»; струнные / духовые / медь / колокольчики / бас → оркестр / мелодия → оркестр — разные подвиды', () => {
    for (const x of fxPresets.filter((y) => y.id.startsWith('orch-'))) expect(familyOf(x), x.id).toBe('orch')
    const g = (id) => fxPresets.find((x) => x.id === id).group
    const subs = ['orch-violins-pad', 'orch-flutes', 'orch-horns', 'orch-glock', 'orch-bass-tuba', 'orch-lead-violin'].map(g)
    expect(new Set(subs).size).toBe(6)
  })

  it('клавиши и синтезаторы — одно семейство: фортепиано, органы, пэды, лиды, игрушки — разными подвидами, в этом порядке', () => {
    const g = (id) => fxPresets.find((x) => x.id === id)
    const ids = ['synth-piano', 'synth-hammond', 'synth-solina', 'synth-moog-lead', 'synth-vltone']
    for (const id of [...ids, 'synth-rhodes', 'synth-farfisa']) expect(familyOf(g(id)), id).toBe('keys')
    const keys = instrumentTree(fxPresets).find((f) => f.family === 'keys')
    expect(keys.subs.map((s) => s.group)).toEqual(ids.map((id) => g(id).group))
  })
})

describe('mergeInstruments: свой инструмент — в семействе и подвиде основы (ТК5)', () => {
  const base = fxPresets.find((x) => x.id === 'bass-melodic')
  const mine = (over = {}) => ({
    id: 11, name: 'Мой бас', base: 'bass-melodic', group: 'drum-kits', stems: ['bass'],
    chain: [{ type: 'eq', highpass_hz: 40 }], extra: {}, ...over,
  })

  it('основа — басовая цепочка → группа основы, а не сохранённая (старая «drum-kits»)', () => {
    const out = mergeInstruments(fxPresets, [mine()], 'ru')
    const m = out.find((x) => x.id === 'my-11')
    expect(m.group).toBe(base.group)
    const tree = instrumentTree(out, 'ru', 'Другое')
    expect(pathOf(tree, 'my-11')).toEqual({ family: 'bass', group: base.group })
  })

  it('свой — в конце подвида основы', () => {
    const tree = instrumentTree(mergeInstruments(fxPresets, [mine()], 'ru'), 'ru', 'Другое')
    const sub = tree.find((f) => f.family === 'bass').subs.find((s) => s.group === base.group)
    expect(sub.items.at(-1).id).toBe('my-11')
  })

  it('без основы и с неизвестной группой → группа как есть, в дереве — «Другое» последним (не теряется)', () => {
    const out = mergeInstruments(fxPresets, [mine({ base: undefined, group: 'my-own-group' })], 'ru')
    expect(out.find((x) => x.id === 'my-11').group).toBe('my-own-group')
    const tree = instrumentTree(out, 'ru', 'Другое')
    expect(tree.at(-1)).toMatchObject({ family: 'other', label: 'Другое' })
    expect(tree.at(-1).subs.flatMap((s) => s.items.map((x) => x.id))).toEqual(['my-11'])
  })

  it('основы нет среди готовых → группа своего как есть', () => {
    const out = mergeInstruments(fxPresets, [mine({ base: 'no-such-preset', group: 'guitar-x' })], 'ru')
    expect(out.find((x) => x.id === 'my-11').group).toBe('guitar-x')
  })
})
