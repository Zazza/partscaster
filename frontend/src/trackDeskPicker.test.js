// Тесты карточки internal-own-track, условие 122 «Пульт: „звук ▾“ — выбор инструмента» (тест-кейс ТК148),
// components/TrackDesk.vue:
//   - выбор «группа → инструмент» — два VSelect в раскрытой дорожке: группа и инструмент (второй — с поиском,
//     пункты — инструменты выбранной группы, itemOptions из presetGroups.js);
//   - раскрытие дорожки (toggleRow) не выбирает первый пресет и не открывает редактор блоков;
//   - редактор блоков ChainEditor — только по флагу «тонко настроить»; «изменить» из «Правок трека» (editRecord)
//     открывает его сразу;
//   - цепочки без дорожек (stems пусто — мастер-склейка) в пульте не показываются.
// Поведение — скриптом компонента (mountSetup: setup в живом экземпляре, api подменён), раскладка — деревом
// разбора шаблона. Подписи desk.* в ru/en — общий тест i18n.test.js.
import { describe, it, expect, vi } from 'vitest'
import { templateAst, findElements, dir, attr, shownIf, idents, mountSetup } from './testkit/sfc.js'
import { fxPresets } from './fxPresets.js'
import { itemOptions } from './presetGroups.js'
import { presetsFor } from './trackDesk.js'

vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
// api: любой вызов — null; состояние плеера — «стоит» (плеер опрашивает его сам)
vi.mock('./api.js', () => ({
  api: new Proxy({ audioState: async () => ({ playing: false, position_sec: 0, duration_sec: 0, job_id: 0 }) }, { get: (o, k) => (k in o ? o[k] : (o[k] = vi.fn(async () => null))) }),
}))
const { default: TrackDesk } = await import('./components/TrackDesk.vue')

const STEMS = [{ name: 'bass', file: 'stem-bass.flac' }, { name: 'guitar', file: 'stem-guitar.flac' }]
const desk = () => mountSetup(TrackDesk, { job: { id: 3, duration_sec: 60 }, stems: STEMS, active: '', sel: null, cursor: 0 })

describe('TrackDesk: выбор инструмента (ТК148)', () => {
  it('раскрытие дорожки: ничего не выбрано, редактор блоков закрыт', async () => {
    const { b } = await desk()
    b.toggleRow('guitar')
    expect(b.open.value).toBe('guitar')
    expect(b.presetId.value).toBe('')
    expect(b.chain.value).toEqual([])
    expect(b.fine.value).toBe(false)
    b.toggleRow('guitar')
    expect(b.open.value).toBe('')
  })

  it('группы — инструменты дорожки, без цепочек без дорожек (мастер-склейка)', async () => {
    const { b } = await desk()
    b.toggleRow('guitar')
    const ids = b.rowGroups.value.flatMap((g) => g.items.map((x) => x.id))
    expect(ids.length).toBeGreaterThan(0)
    expect(fxPresets.find((p) => p.id === 'master-glue')).toBeTruthy()
    expect(ids).not.toContain('master-glue')
    const want = presetsFor('guitar', fxPresets).filter((p) => (p.stems || []).length).map((p) => p.id)
    expect([...ids].sort()).toEqual([...want].sort())
  })

  it('пункты инструмента — инструменты выбранной группы; выбор ставит цепочку, редактор не открывает', async () => {
    const { b } = await desk()
    b.toggleRow('guitar')
    const g = b.rowGroups.value.find((x) => x.items.length)
    b.groupId.value = g.group
    expect(b.itemOpts.value).toEqual(itemOptions(g, 'ru'))
    b.pickById(g.items[0].id)
    expect(b.presetId.value).toBe(g.items[0].id)
    expect(b.chain.value.length).toBeGreaterThan(0)
    expect(b.fine.value).toBe(false)
  })

  it('«изменить» из «Правок трека» открывает дорожку записи сразу с редактором блоков', async () => {
    const { b } = await desk()
    b.editRecord({ stems: ['bass'], engine: [{ type: 'comp' }], childId: 12, from: 5, to: 9, label: 'бас' })
    expect(b.open.value).toBe('bass')
    expect(b.fine.value).toBe(true)
    expect(b.chain.value.map((x) => x.type)).toEqual(['comp'])
    expect(b.editing.value).toMatchObject({ childId: 12, from: 5, to: 9 })
  })

  it('«изменить» запись дорожки, которой у трека нет, — ошибка, редактор не открывается', async () => {
    const { b } = await desk()
    b.editRecord({ stems: ['vocals'], engine: [{ type: 'comp' }] })
    expect(b.open.value).toBe('')
    expect(b.err.value).not.toBe('')
  })
})

describe('TrackDesk: раскладка шаблона (ТК148)', () => {
  const root = templateAst('components/TrackDesk.vue')
  const selects = findElements(root, (n) => n.tag === 'VSelect')
  const inOpenRow = (s) => s.ancestors.some((a) => idents(shownIf(a)).includes('open'))

  it('VSelect группы и VSelect инструмента (с поиском) — в раскрытой дорожке, группа раньше', () => {
    const group = selects.find((s) => dir(s.node, 'bind', 'options') === 'groupOpts')
    const item = selects.find((s) => dir(s.node, 'bind', 'options') === 'itemOpts')
    expect(group && item).toBeTruthy()
    expect(attr(item.node, 'searchable') !== null || dir(item.node, 'bind', 'searchable') === 'true').toBe(true)
    expect(inOpenRow(group) && inOpenRow(item)).toBe(true)
    expect(selects.indexOf(group)).toBeLessThan(selects.indexOf(item))
  })

  it('ChainEditor — под флагом «тонко настроить» (fine), который переключает кнопка', () => {
    const [ed] = findElements(root, (n) => n.tag === 'ChainEditor')
    expect(shownIf(ed.node)).toBe('fine')
    const toggles = findElements(root, (n) => n.tag === 'button' && /\bfine\s*=\s*!\s*fine\b/.test(dir(n, 'on', 'click') || ''))
    expect(toggles).toHaveLength(1)
  })
})
