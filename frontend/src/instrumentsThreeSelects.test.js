// Задача internal-instruments-order: страница «Инструменты» и пульт дорожек студии выбирают инструмент в три шага
// «семейство → подвид → инструмент»; второй выбор скрыт, если у семейства один подвид. Фраза на странице —
// кнопки с логикой radio (нажата ровно одна). Поведение — скриптом компонента (mountSetup, api подменён),
// раскладка — деревом разбора шаблона.
import { describe, it, expect, vi } from 'vitest'
import { nextTick } from 'vue'
import { templateAst, findElements, dir, shownIf, idents, mountSetup } from './testkit/sfc.js'
import { PRESET_GROUPS } from './presetGroups.js'
import { fxPresets } from './fxPresets.js'
import { api } from './api.js'

vi.hoisted(() => {
  globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} }
  globalThis.window = { addEventListener() {}, removeEventListener() {} }   // клавиши страницы — в onMounted
})
// api: любой вызов — null; состояние плеера — «стоит»
vi.mock('./api.js', () => ({
  api: new Proxy({ audioState: async () => ({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 }) }, { get: (o, k) => (k in o ? o[k] : (o[k] = vi.fn(async () => null))) }),
}))
const { default: InstrumentsPage } = await import('./components/InstrumentsPage.vue')
const { default: TrackDesk } = await import('./components/TrackDesk.vue')

const FAMILY_IDS = ['guitar', 'bass', 'drums', 'perc', 'keys', 'synth', 'orch', 'fx']   // порядок карточки
const presetById = (id) => fxPresets.find((x) => x.id === id)
const familyOfGroup = (g) => PRESET_GROUPS[g]?.family
const settle = async () => { await nextTick(); await nextTick() }

// Модель VSelect: v-model или :model-value
const modelOf = (n) => dir(n, 'model') ?? dir(n, 'bind', 'model-value') ?? dir(n, 'bind', 'modelValue')

describe('InstrumentsPage: три выбора — шаблон', () => {
  const root = templateAst('components/InstrumentsPage.vue')
  const selects = findElements(root, (n) => n.tag === 'VSelect')
  const byOptions = (name) => selects.filter((x) => dir(x.node, 'bind', 'options') === name)

  it('VSelect семейства, подвида и инструмента — подряд, в этом порядке', () => {
    const [f] = byOptions('familyOpts')
    const [g] = byOptions('groupOpts')
    const [i] = byOptions('itemOpts')
    expect(f && g && i).toBeTruthy()
    expect(selects.indexOf(f) + 1).toBe(selects.indexOf(g))
    expect(selects.indexOf(g) + 1).toBe(selects.indexOf(i))
    expect(idents(modelOf(f.node))).toContain('familyId')
    expect(idents(modelOf(g.node))).toContain('groupId')
  })

  it('подвид показан только при showGroup (ТК4)', () => {
    const [g] = byOptions('groupOpts')
    const cond = [g.node, ...g.ancestors].map(shownIf).filter(Boolean).join(' ')
    expect(idents(cond)).toContain('showGroup')
  })

  it('ТК7: фраза — не VSelect, а кнопки в v-for по phraseOpts с aria-pressed и кликом, выставляющим phraseId', () => {
    expect(byOptions('phraseOpts').length, 'фраза — не VSelect').toBe(0)
    const btns = findElements(root, (n) => n.tag === 'button' && idents(dir(n, 'for')).includes('phraseOpts'))
    expect(btns).toHaveLength(1)
    const b = btns[0].node
    expect(idents(dir(b, 'bind', 'aria-pressed'))).toContain('phraseId')
    expect(idents(dir(b, 'on', 'click'))).toContain('phraseId')
  })
})

describe('InstrumentsPage: три выбора — поведение', () => {
  it('семейства — из списка FAMILIES, в его порядке, больше одного', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    const vals = b.familyOpts.value.map((o) => o.value)
    expect(vals.length).toBeGreaterThan(1)
    for (const v of vals) expect(FAMILY_IDS).toContain(v)
    expect([...vals].sort((x, y) => FAMILY_IDS.indexOf(x) - FAMILY_IDS.indexOf(y))).toEqual(vals)
    for (const o of b.familyOpts.value) expect(String(o.label).trim(), o.value).not.toBe('')
  })

  it('ТК6: смена семейства → подвид и инструмент — первые из нового семейства', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    for (const fam of ['bass', 'drums', 'guitar']) {
      b.familyId.value = fam
      await settle()
      expect(b.groupOpts.value.length, fam).toBeGreaterThan(0)
      for (const o of b.groupOpts.value) expect(familyOfGroup(o.value), `${fam}: подвид ${o.value}`).toBe(fam)
      expect(b.groupId.value, fam).toBe(b.groupOpts.value[0].value)
      expect(b.presetId.value, fam).toBe(b.itemOpts.value[0].value)
      expect(presetById(b.presetId.value).group, fam).toBe(b.groupId.value)
    }
  })

  it('ТК6: инструменты в списке — только выбранного подвида', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    b.familyId.value = 'drums'
    await settle()
    const second = b.groupOpts.value[1]
    expect(second).toBeTruthy()
    b.groupId.value = second.value
    await settle()
    expect(b.itemOpts.value.length).toBeGreaterThan(0)
    for (const o of b.itemOpts.value) expect(presetById(o.value).group, o.value).toBe(second.value)
  })

  it('ТК6: выбор инструмента из другого семейства → семейство и подвид выставляются по нему', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    b.familyId.value = 'guitar'
    await settle()
    const target = presetById('drums-kick-tr808')
    b.presetId.value = target.id
    await settle()
    expect(b.familyId.value).toBe('drums')
    expect(b.groupId.value).toBe(target.group)
  })

  it('ТК4: showGroup — ровно когда у семейства больше одного подвида; гитара — показан', async () => {
    const { b } = await mountSetup(InstrumentsPage, {})
    for (const o of b.familyOpts.value) {
      b.familyId.value = o.value
      await settle()
      expect(b.showGroup.value, o.value).toBe(b.groupOpts.value.length > 1)
    }
    b.familyId.value = 'guitar'
    await settle()
    expect(b.showGroup.value).toBe(true)
  })

  it('ТК7: фраза — одна выбранная из phraseOpts; выбор другой переключает', async () => {
    // воркер отдаёт фразы гитары (по умолчанию подменённый api — null, фраз нет)
    const name = (x) => ({ ru: x, en: x })
    api.fxPhrases.mockResolvedValueOnce([
      { id: 'g1', family: 'guitar', name: name('аккорды') }, { id: 'g2', family: 'guitar', name: name('перебор') },
    ])
    const { b } = await mountSetup(InstrumentsPage, {})
    b.familyId.value = 'guitar'
    await settle()
    const vals = b.phraseOpts.value.map((o) => o.value)
    expect(vals.length).toBeGreaterThan(0)
    // нажата ровно одна: текущее значение — одно из пунктов
    expect(vals.filter((v) => v === b.phraseId.value)).toHaveLength(1)
    if (vals.length > 1) {
      const other = vals.find((v) => v !== b.phraseId.value)
      b.phraseId.value = other
      await settle()
      expect(vals.filter((v) => v === b.phraseId.value)).toEqual([other])
    }
  })
})

describe('TrackDesk: три выбора на пульте — шаблон', () => {
  const root = templateAst('components/TrackDesk.vue')
  const selects = findElements(root, (n) => n.tag === 'VSelect')
  const byOptions = (name) => selects.filter((x) => dir(x.node, 'bind', 'options') === name)
  const cond = (x) => [x.node, ...x.ancestors].map(shownIf).filter(Boolean).join(' ')

  it('VSelect семейства (при showFamily), подвида (при showGroup), инструмента — подряд', () => {
    const [f] = byOptions('familyOpts')
    const [g] = byOptions('groupOpts')
    const [i] = byOptions('itemOpts')
    expect(f && g && i).toBeTruthy()
    expect(idents(cond(f))).toContain('showFamily')
    expect(idents(cond(g))).toContain('showGroup')
    expect(idents(modelOf(f.node))).toContain('familyId')
    expect(selects.indexOf(f) < selects.indexOf(g) && selects.indexOf(g) < selects.indexOf(i)).toBe(true)
  })
})

describe('TrackDesk: три выбора на пульте — поведение', () => {
  const STEMS = [{ name: 'bass', file: 'stem-bass.flac' }, { name: 'guitar', file: 'stem-guitar.flac' }]
  const desk = () => mountSetup(TrackDesk, { job: { id: 3, duration_sec: 60 }, stems: STEMS, active: '', sel: null, cursor: 0 })

  it('раскрытие дорожки: семейство и подвид — первые; showFamily/showGroup — по числу пунктов', async () => {
    const { b } = await desk()
    b.toggleRow('bass')
    await settle()
    expect(b.familyOpts.value.length).toBeGreaterThan(0)
    expect(b.familyId.value).toBe(b.familyOpts.value[0].value)
    expect(b.groupId.value).toBe(b.groupOpts.value[0].value)
    expect(b.showFamily.value).toBe(b.familyOpts.value.length > 1)
    expect(b.showGroup.value).toBe(b.groupOpts.value.length > 1)
  })

  it('дорожка баса: первое семейство — «Бас», в нём несколько подвидов', async () => {
    const { b } = await desk()
    b.toggleRow('bass')
    await settle()
    expect(b.familyId.value).toBe('bass')
    expect(b.showGroup.value).toBe(true)
  })

  it('ТК6: смена семейства на пульте → подвид — первый нового семейства', async () => {
    const { b } = await desk()
    b.toggleRow('bass')
    await settle()
    const other = b.familyOpts.value.find((o) => o.value !== b.familyId.value)
    expect(other, 'у дорожки баса и «Оркестр» (бас → оркестр)').toBeTruthy()
    b.familyId.value = other.value
    await settle()
    for (const o of b.groupOpts.value) expect(familyOfGroup(o.value), o.value).toBe(other.value)
    expect(b.groupId.value).toBe(b.groupOpts.value[0].value)
  })
})
