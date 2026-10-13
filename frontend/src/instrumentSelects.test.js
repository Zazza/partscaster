// Страница: раскладка — деревом разбора шаблона InstrumentsPage.vue, поведение списков — скриптом страницы
// (mountSetup, api подменён).
import { describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'
import { templateAst, findElements, dir, attr, mountSetup } from './testkit/sfc.js'
import { PRESET_GROUPS, groupPresets, itemOptions, subOptions, pathOf } from './presetGroups.js'
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

describe('itemOptions', () => {
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

vi.hoisted(() => {
  globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }
  globalThis.window = { addEventListener() {}, removeEventListener() {} }   // клавиши страницы — в onMounted
})
// api: любой вызов — null; состояние плеера — «стоит» (плеер опрашивает его сам)
vi.mock('./api.js', () => ({
  api: new Proxy({ audioState: async () => ({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 }) }, { get: (o, k) => (k in o ? o[k] : (o[k] = vi.fn(async () => null))) }),
}))
const { default: InstrumentsPage } = await import('./components/InstrumentsPage.vue')

describe('InstrumentsPage: списки вместо кнопок', () => {
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

  it('списки строятся из дерева пресетов: подвиды — семейства, инструменты — выбранного подвида', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    const tree = b.tree.value
    expect(tree.length).toBeGreaterThan(1)
    const fam = tree.find((f) => f.subs.length > 1)
    b.familyId.value = fam.family
    await nextTick()
    expect(b.groupOpts.value).toEqual(subOptions(fam))
    b.groupId.value = fam.subs[1].group
    expect(b.itemOpts.value).toEqual(itemOptions(fam.subs[1], 'ru'))
  })

  it('выбор инструмента ставит его цепочку и подсказку (note), группа — его', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    const target = b.tree.value[1].subs[0].items[0]
    b.applyPreset(target.id)
    await nextTick()
    expect(b.preset.value.id).toBe(target.id)
    expect(b.chain.value.map((x) => x.type)).toEqual(target.chain.map((x) => x.type))
    expect({ family: b.familyId.value, group: b.groupId.value }).toEqual(pathOf(b.tree.value, target.id))
    expect(b.tr(b.preset.value.note)).not.toBe('')
  })
})
