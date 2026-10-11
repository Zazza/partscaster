// Тесты карточки internal-own-track, этап 12, условия 90–91 (тест-кейс ТК130): страница «Инструменты» —
// два списка (группа → инструмент) вместо стены кнопок пресетов.
// presetGroups.js: groupOptions(groups, firstLabel) → [{value, label}], itemOptions(group, locale) →
// [{value, label, title}], groupOf(groups, id) → group. groups — выход groupPresets.
// Страница: раскладка — деревом разбора шаблона InstrumentsPage.vue, поведение списков — скриптом страницы
// (mountSetup, api подменён). Написаны по карточке, без чтения реализации.
import { describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'
import { templateAst, findElements, dir, attr, mountSetup } from './testkit/sfc.js'
import { PRESET_GROUPS, groupPresets, groupOptions, itemOptions, groupOf } from './presetGroups.js'
import { fxPresets } from './fxPresets.js'

const [G1, G2] = Object.keys(PRESET_GROUPS)
const p = (id, group, extra = {}) => ({
  id,
  ...(group === undefined ? {} : { group }),
  name: { ru: `имя ${id}`, en: `name ${id}` },
  note: { ru: `заметка ${id}`, en: `note ${id}` },
  ...extra,
})
// [a{G1}, b{G2}, c{G1}, d{без группы}] → G1:[a,c], G2:[b], '':[d]
const list = () => [p('a', G1), p('b', G2), p('c', G1), p('d')]

describe('groupOptions (условие 90, ТК130)', () => {
  it('value — group по порядку groupPresets, label — подпись группы', () => {
    const groups = groupPresets(list(), 'en')
    expect(groupOptions(groups, 'First')).toEqual([
      { value: G1, label: PRESET_GROUPS[G1].en },
      { value: G2, label: PRESET_GROUPS[G2].en },
      { value: '', label: 'First' },
    ])
  })

  it('подписи — на языке groups (ru)', () => {
    const groups = groupPresets(list(), 'ru')
    expect(groupOptions(groups, 'Первая').map((o) => o.label))
      .toEqual([PRESET_GROUPS[G1].ru, PRESET_GROUPS[G2].ru, 'Первая'])
  })

  it('группа без подписи первой по порядку → firstLabel', () => {
    const groups = groupPresets([p('x'), p('y', G2)], 'en')
    expect(groupOptions(groups, 'Presets')).toEqual([
      { value: '', label: 'Presets' },
      { value: G2, label: PRESET_GROUPS[G2].en },
    ])
  })

  it('пусто → []', () => {
    expect(groupOptions([], 'Presets')).toEqual([])
  })

  it('на реальных fxPresets: по одному пункту на группу, порядок groupPresets, подписи непустые', () => {
    const groups = groupPresets(fxPresets, 'ru')
    const out = groupOptions(groups, 'Пресеты')
    expect(out.map((o) => o.value)).toEqual(groups.map((g) => g.group))
    for (const o of out) expect(String(o.label).trim(), `подпись группы ${o.value}`).not.toBe('')
  })
})

describe('itemOptions (условие 90, ТК130)', () => {
  it('value = id, label = name[locale], title = note[locale] — по порядку пунктов группы (en)', () => {
    const g1 = groupPresets(list(), 'en')[0]
    expect(itemOptions(g1, 'en')).toEqual([
      { value: 'a', label: 'name a', title: 'note a' },
      { value: 'c', label: 'name c', title: 'note c' },
    ])
  })

  it('locale ru → русские имя и заметка', () => {
    const g1 = groupPresets(list(), 'ru')[0]
    expect(itemOptions(g1, 'ru')).toEqual([
      { value: 'a', label: 'имя a', title: 'заметка a' },
      { value: 'c', label: 'имя c', title: 'заметка c' },
    ])
  })

  it('чужие группы не попадают', () => {
    const groups = groupPresets(list(), 'en')
    const g2 = groups.find((g) => g.group === G2)
    expect(itemOptions(g2, 'en').map((o) => o.value)).toEqual(['b'])
    const none = groups.find((g) => g.group === '')
    expect(itemOptions(none, 'en').map((o) => o.value)).toEqual(['d'])
  })

  it('нет группы (undefined/null) → []', () => {
    expect(itemOptions(undefined, 'en')).toEqual([])
    expect(itemOptions(null, 'ru')).toEqual([])
  })

  it('на реальных fxPresets: пункты группы = её пресеты, подписи из name', () => {
    const groups = groupPresets(fxPresets, 'en')
    for (const g of groups) {
      const out = itemOptions(g, 'en')
      expect(out.map((o) => o.value)).toEqual(g.items.map((x) => x.id))
      expect(out.map((o) => o.label)).toEqual(g.items.map((x) => x.name.en))
    }
  })
})

describe('groupOf (условие 90, ТК130)', () => {
  const groups = () => groupPresets(list(), 'en')

  it('нашёл → группа, где лежит пресет', () => {
    expect(groupOf(groups(), 'a')).toBe(G1)
    expect(groupOf(groups(), 'c')).toBe(G1)
    expect(groupOf(groups(), 'b')).toBe(G2)
    expect(groupOf(groups(), 'd')).toBe('')
  })

  it('не нашёл → группа первой', () => {
    expect(groupOf(groups(), 'nope')).toBe(G1)
    expect(groupOf(groupPresets([p('x', G2), p('y', G1)], 'en'), 'nope')).toBe(G2)
  })

  it('пустой id → группа первой', () => {
    expect(groupOf(groups(), '')).toBe(G1)
  })

  it('пусто (нет групп) → ""', () => {
    expect(groupOf([], 'a')).toBe('')
  })
})

// ---- страница по исходнику (условие 91) ----

vi.hoisted(() => {
  globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }
  globalThis.window = { addEventListener() {}, removeEventListener() {} }   // клавиши страницы — в onMounted
})
// api: любой вызов — null; состояние плеера — «стоит» (плеер опрашивает его сам)
vi.mock('./api.js', () => ({
  api: new Proxy({ audioState: async () => ({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 }) }, { get: (o, k) => (k in o ? o[k] : (o[k] = vi.fn(async () => null))) }),
}))
const { default: InstrumentsPage } = await import('./components/InstrumentsPage.vue')

describe('InstrumentsPage: два списка вместо кнопок (условие 91, ТК130)', () => {
  const root = templateAst('components/InstrumentsPage.vue')
  const selects = findElements(root, (n) => n.tag === 'VSelect')
  const byOptions = (name) => selects.filter((x) => dir(x.node, 'bind', 'options') === name)

  it('VSelect групп без поиска и VSelect инструментов с поиском — рядом, группа первой; выбор — applyPreset', () => {
    const [g] = byOptions('groupOpts')
    const [i] = byOptions('itemOpts')
    expect(g && i).toBeTruthy()
    expect(attr(g.node, 'searchable')).toBeNull()
    expect(attr(i.node, 'searchable')).not.toBeNull()
    expect(selects.indexOf(g) + 1).toBe(selects.indexOf(i))
    expect(dir(i.node, 'on', 'update:model-value')).toBe('applyPreset')
  })

  it('списки строятся из групп пресетов: группы — groupOptions, инструменты — выбранной группы', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    const groups = b.presetGroups.value
    expect(groups.length).toBeGreaterThan(1)
    expect(b.groupOpts.value).toEqual(groupOptions(groups, b.t('instr.group.other')))
    b.groupId.value = groups[1].group
    expect(b.itemOpts.value).toEqual(itemOptions(groups[1], 'ru'))
  })

  it('выбор инструмента ставит его цепочку и подсказку (note), группа — его', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    const target = b.presetGroups.value[1].items[0]
    b.applyPreset(target.id)
    await nextTick()
    expect(b.preset.value.id).toBe(target.id)
    expect(b.chain.value.map((x) => x.type)).toEqual(target.chain.map((x) => x.type))
    expect(b.groupId.value).toBe(groupOf(b.presetGroups.value, target.id))
    expect(b.tr(b.preset.value.note)).not.toBe('')
  })
})
