// Тесты демо-эталонов пресетов (usePresetDemo) по контракту: на каждую семью жанровых пресетов —
// свой инструментальный черновик с фиксированным сидом и папкой «Демо», кеш id в localStorage
// переживает перезапуск, сломанная/удалённая джоба пересоздаётся, дорожки делаются лениво и один
// раз, параллельные ensure не дублируют submit. Модуль — синглтон: каждый тест импортирует заново.
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { PRESET_FAMILIES } from '../soundPresets.js'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

// node-окружение vitest без DOM: минимальный localStorage (как в useInserts.test.js)
class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}
const storage = new MemStorage()

const job = (over = {}) => ({ id: 1, title: 'Демо · Рок', status: 'done', duration_sec: 38, folder: 'Демо', ...over })

// свежий экземпляр синглтона + моки api; jobs — список джоб воркера
async function load({ jobs = [], stems } = {}) {
  vi.resetModules()
  globalThis.localStorage = storage
  apiMock.jobs = vi.fn(async () => jobs)
  apiMock.submit = vi.fn(async () => 777)
  apiMock.setJobFolder = vi.fn(async () => job())
  apiMock.jobStems = vi.fn(async () => stems || [])
  apiMock.makeStems = vi.fn(async () => ({}))
  const mod = await import('./usePresetDemo.js')
  return mod
}

beforeEach(() => { storage.clear() })

describe('DEMO_STYLES — по семьям пресетов', () => {
  it('ровно все PRESET_FAMILIES, у каждой сид и английский стиль (теги YuE)', async () => {
    const { DEMO_STYLES } = await load()
    expect(Object.keys(DEMO_STYLES)).toEqual(PRESET_FAMILIES)
    for (const [fam, s] of Object.entries(DEMO_STYLES)) {
      expect(s.seed, fam).toBeGreaterThan(0)
      expect(s.style, fam).toMatch(/^[ -~]+$/)          // только ASCII
      expect(s.style, fam).toMatch(/[Ii]nstrumental/)
    }
  })
})

describe('ensure — демо создаётся один раз', () => {
  it('без кеша: submit черновика-инструментала с фиксированным сидом, папка «Демо», кеш записан', async () => {
    const { usePresetDemo, DEMO_STYLES } = await load()
    const id = await usePresetDemo().ensure('Рок')
    expect(id).toBe(777)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    expect(apiMock.submit).toHaveBeenCalledWith(expect.objectContaining({
      title: 'Демо · Рок', lyrics: '[Instrumental]', seed: DEMO_STYLES['Рок'].seed,
      style: DEMO_STYLES['Рок'].style, draft: true, cot: 'off',
    }))
    expect(apiMock.setJobFolder).toHaveBeenCalledWith(777, 'Демо')
    expect(JSON.parse(storage.getItem('yue_preset_demo'))).toEqual({ 'Рок': 777 })
  })

  it('повторное ensure не дублирует submit (джоба уже в работе/готова)', async () => {
    const { usePresetDemo } = await load()
    const d = usePresetDemo()
    await d.ensure('Электроника')
    await d.ensure('Электроника')
    await d.ensure('Электроника')
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
  })

  it('кеш после перезапуска: готовая джоба узнаётся по списку, submit не зовётся', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12 })] })
    const id = await usePresetDemo().ensure('Рок')
    expect(id).toBe(12)
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(usePresetDemo().demos.value['Рок'].status).toBe('ready')
    expect(usePresetDemo().demos.value['Рок'].durationSec).toBe(38)
  })

  it('джоба в очереди — cooking; error/удалена — пересоздаётся', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12, status: 'running', duration_sec: 0 })] })
    const d = usePresetDemo()
    expect(await d.ensure('Рок')).toBe(12)
    expect(d.demos.value['Рок'].status).toBe('cooking')

    // сломалась — новая
    const { usePresetDemo: again } = await (async () => {
      vi.resetModules()
      globalThis.localStorage = storage
      apiMock.jobs = vi.fn(async () => [job({ id: 12, status: 'error', error: 'упала' })])
      return import('./usePresetDemo.js')
    })()
    const id2 = await again().ensure('Рок')
    expect(id2).toBe(777)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    expect(JSON.parse(storage.getItem('yue_preset_demo'))).toEqual({ 'Рок': 777 })
  })

  it('неизвестная семья (свой пресет, family "") — демо первой семьи', async () => {
    const { usePresetDemo } = await load()
    await usePresetDemo().ensure('')
    expect(apiMock.submit).toHaveBeenCalledWith(expect.objectContaining({ title: `Демо · ${PRESET_FAMILIES[0]}` }))
  })
})

describe('refresh — поллинг открытого редактора', () => {
  it('cooking → done: статус ready и длительность', async () => {
    const { usePresetDemo } = await load()
    const d = usePresetDemo()
    await d.ensure('Рок')
    expect(d.demos.value['Рок'].status).toBe('cooking')
    apiMock.jobs = vi.fn(async () => [job({ id: 777, status: 'done' })])
    await d.refresh()
    expect(d.demos.value['Рок'].status).toBe('ready')
    expect(d.demos.value['Рок'].durationSec).toBe(38)
  })

  it('готовая/ошибочная не трогаются, без готовящихся jobs не зовётся', async () => {
    const { usePresetDemo } = await load()
    const d = usePresetDemo()
    await d.refresh()
    expect(apiMock.jobs).not.toHaveBeenCalled()
  })
})

describe('ensureStems — дорожки лениво', () => {
  it('готовое демо: стемов нет — makeStems один раз, имена запомнились', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12 })] })
    const d = usePresetDemo()
    await d.ensure('Рок')
    apiMock.jobStems = vi.fn(async () => [{ name: 'vocals' }, { name: 'drums' }, { name: 'bass' }, { name: 'other' }])
    let names = await d.ensureStems('Рок')
    expect(names).toEqual(['vocals', 'drums', 'bass', 'other'])
    expect(apiMock.makeStems).not.toHaveBeenCalled()      // дорожки уже были
    expect([...d.available('Рок')]).toEqual(['vocals', 'drums', 'bass', 'other'])

    // повторно — из кеша, воркер не спрашивается
    names = await d.ensureStems('Рок')
    expect(apiMock.jobStems).toHaveBeenCalledTimes(1)
    expect(names).toEqual(['vocals', 'drums', 'bass', 'other'])
  })

  it('стемов нет: сначала makeStems, потом чтение', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12 })] })
    const d = usePresetDemo()
    await d.ensure('Рок')
    let first = true
    apiMock.jobStems = vi.fn(async () => (first ? (first = false, []) : [{ name: 'bass' }]))
    const names = await d.ensureStems('Рок')
    expect(apiMock.makeStems).toHaveBeenCalledWith(12)
    expect(names).toEqual(['bass'])
  })

  it('неготовое демо — без разделения', async () => {
    const { usePresetDemo } = await load()
    const d = usePresetDemo()
    expect(await d.ensureStems('Рок')).toEqual([])
    expect(apiMock.makeStems).not.toHaveBeenCalled()
  })
})

describe('waitReady — дождаться готовности (для применения пресета)', () => {
  it('джоба уже done — сразу ready, без submit', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12 })] })
    expect(await usePresetDemo().waitReady('Рок')).toBe('ready')
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('демо сломалось — error', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12, status: 'error', error: 'нет' })] })
    // error в списке — ensure пересоздаёт джобу (777), она тоже error → waitReady вернёт error
    apiMock.jobs = vi.fn(async () => [job({ id: 777, status: 'error', error: 'снова нет' })])
    expect(await usePresetDemo().waitReady('Рок')).toBe('error')
  })
})

describe('reset — забыть демо', () => {
  it('после reset ensure создаёт новую джобу', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12 })] })
    const d = usePresetDemo()
    await d.ensure('Рок')
    d.reset('Рок')
    expect(JSON.parse(storage.getItem('yue_preset_demo'))).toEqual({})
    await d.ensure('Рок')
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
  })
})

// waitReady(fam, timeout, cancelled) — закрытый редактор прекращает опрос воркера:
// отмена → 'cancelled', после отмены ни опроса списка джоб, ни генерации нового демо.
describe('waitReady — отмена (редактор закрыт)', () => {
  const TIMEOUT = 600   // секунд
  const flush = async () => { for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0) }
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })

  // результат промиса без ожидания (undefined — ещё не решён)
  const track = (p) => { const st = { done: false, value: undefined }; p.then((v) => { st.done = true; st.value = v }); return st }

  it('демо готовится, редактор закрыли посреди опроса — cancelled, опрос остановлен, submit не вызван', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12, status: 'running', duration_sec: 0 })] })
    let closed = false
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => closed))
    await flush()
    await vi.advanceTimersByTimeAsync(30_000)
    expect(st.done, 'демо ещё готовится — waitReady не должен решиться').toBe(false)
    expect(apiMock.jobs.mock.calls.length, 'до закрытия воркер опрашивается').toBeGreaterThan(0)

    closed = true
    await vi.advanceTimersByTimeAsync(30_000)
    await flush()
    expect(st.done).toBe(true)
    expect(st.value).toBe('cancelled')
    const calls = apiMock.jobs.mock.calls.length
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000)
    await flush()
    expect(apiMock.jobs.mock.calls.length, 'после отмены воркер опрашивается дальше').toBe(calls)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('отменено с самого начала (cancelled () => true) — cancelled, без опроса по таймеру', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12, status: 'running', duration_sec: 0 })] })
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => true))
    await flush()
    expect(st.done).toBe(true)
    expect(st.value).toBe('cancelled')
    const calls = apiMock.jobs.mock.calls.length
    expect(calls, 'не больше одной проверки списка').toBeLessThanOrEqual(1)
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000)
    await flush()
    expect(apiMock.jobs.mock.calls.length).toBe(calls)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('отменено с самого начала и демо нет — новое демо не генерируется', async () => {
    const { usePresetDemo } = await load()
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => true))
    await flush()
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000)
    await flush()
    expect(st.value).toBe('cancelled')
    expect(apiMock.submit).not.toHaveBeenCalled()
  })

  it('без отмены (cancelled () => false) готовность по-прежнему дожидается: running → done = ready', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load({ jobs: [job({ id: 12, status: 'running', duration_sec: 0 })] })
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => false))
    await flush()
    await vi.advanceTimersByTimeAsync(20_000)
    expect(st.done).toBe(false)
    apiMock.jobs = vi.fn(async () => [job({ id: 12, status: 'done' })])
    await vi.advanceTimersByTimeAsync(60_000)
    await flush()
    expect(st.value).toBe('ready')
  })
})

// отмена проверяется и между ensure и refresh — редактор закрыли, пока ensure ждал
// ответа воркера (submit нового демо или проверка списка джоб), — дальше воркер не опрашивается.
describe('waitReady — отмена во время ensure', () => {
  const TIMEOUT = 600
  const flush = async () => { for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0) }
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.clearAllTimers(); vi.useRealTimers() })
  const track = (p) => { const st = { done: false, value: undefined }; p.then((v) => { st.done = true; st.value = v }); return st }
  const deferred = () => { let resolve; const promise = new Promise((r) => { resolve = r }); return { promise, resolve } }

  it('закрыли во время submit нового демо — cancelled, список джоб после этого не запрашивается', async () => {
    const { usePresetDemo } = await load()
    const sub = deferred()
    apiMock.submit = vi.fn(() => sub.promise)
    let closed = false
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => closed))
    await flush()
    expect(apiMock.submit, 'демо нет — ensure генерирует').toHaveBeenCalledTimes(1)
    closed = true
    const jobsAtClose = apiMock.jobs.mock.calls.length
    sub.resolve(777)
    await flush()
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000)
    await flush()
    expect(st.done).toBe(true)
    expect(st.value).toBe('cancelled')
    expect(apiMock.jobs.mock.calls.length, 'после отмены вызван refresh/jobs').toBe(jobsAtClose)
  })

  it('закрыли во время проверки кешированного демо (jobs в ensure) — cancelled, повторного jobs нет', async () => {
    storage.setItem('yue_preset_demo', JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load()
    const list = deferred()
    apiMock.jobs = vi.fn(() => list.promise)
    let closed = false
    const st = track(usePresetDemo().waitReady('Рок', TIMEOUT, () => closed))
    await flush()
    expect(apiMock.jobs, 'ensure проверяет кеш по списку джоб').toHaveBeenCalledTimes(1)
    closed = true
    list.resolve([job({ id: 12, status: 'running', duration_sec: 0 })])
    await flush()
    await vi.advanceTimersByTimeAsync(5 * 60 * 1000)
    await flush()
    expect(st.done).toBe(true)
    expect(st.value).toBe('cancelled')
    expect(apiMock.jobs, 'после отмены вызван refresh/jobs').toHaveBeenCalledTimes(1)
    expect(apiMock.submit).not.toHaveBeenCalled()
  })
})
