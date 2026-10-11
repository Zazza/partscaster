// ТК156 (127а–128в, ревью s16): сбой связи при проверке демо ≠ «демо удалили».
// ensure при закешированном id и падающем api.jobs — ошибка, без новой генерации и без сброса кеша;
// api.jobs отвечает, но этой джобы нет (удалили) — новое демо. Модуль — синглтон: импорт заново.
import { describe, it, expect, vi, beforeEach } from 'vitest'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}
const storage = new MemStorage()
const KEY = 'yue_preset_demo'

const job = (over = {}) => ({ id: 1, title: 'Демо · Рок', status: 'done', duration_sec: 38, folder: 'Демо', ...over })

async function load(jobsImpl) {
  vi.resetModules()
  globalThis.localStorage = storage
  apiMock.jobs = vi.fn(jobsImpl)
  apiMock.submit = vi.fn(async () => 777)
  apiMock.setJobFolder = vi.fn(async () => job())
  apiMock.jobStems = vi.fn(async () => [])
  apiMock.makeStems = vi.fn(async () => ({}))
  return import('./usePresetDemo.js')
}

// ensure «не удался»: либо промис отклонён, либо статус демо — error
async function ensureFails(d, fam) {
  let rejected = false
  let result
  try { result = await d.ensure(fam) } catch { rejected = true }
  return { failed: rejected || d.demos.value[fam]?.status === 'error', result }
}

beforeEach(() => { storage.clear() })

describe('ensure — сбой связи при проверке закешированного демо', () => {
  it('api.jobs падает: ensure не удался, submit не вызван, кеш id не стёрт', async () => {
    storage.setItem(KEY, JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load(async () => { throw new Error('network down') })
    const d = usePresetDemo()
    const { failed, result } = await ensureFails(d, 'Рок')
    expect(failed, 'сбой связи должен давать ошибку').toBe(true)
    expect(result).not.toBe(777)
    expect(apiMock.jobs).toHaveBeenCalled()
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(apiMock.setJobFolder).not.toHaveBeenCalled()
    expect(JSON.parse(storage.getItem(KEY))).toEqual({ 'Рок': 12 })
  })

  it('связь вернулась — то же демо узнаётся по кешу, без новой генерации', async () => {
    storage.setItem(KEY, JSON.stringify({ 'Рок': 12 }))
    let online = false
    const { usePresetDemo } = await load(async () => {
      if (!online) throw new Error('network down')
      return [job({ id: 12 })]
    })
    const d = usePresetDemo()
    await ensureFails(d, 'Рок')
    online = true
    expect(await d.ensure('Рок')).toBe(12)
    expect(apiMock.submit).not.toHaveBeenCalled()
    expect(JSON.parse(storage.getItem(KEY))).toEqual({ 'Рок': 12 })
  })

  it('сбой у одной семьи не трогает кеш другой', async () => {
    storage.setItem(KEY, JSON.stringify({ 'Рок': 12, 'Электроника': 34 }))
    const { usePresetDemo } = await load(async () => { throw new Error('timeout') })
    await ensureFails(usePresetDemo(), 'Рок')
    expect(JSON.parse(storage.getItem(KEY))).toEqual({ 'Рок': 12, 'Электроника': 34 })
  })
})

describe('ensure — демо удалили (api.jobs ответил, джобы нет)', () => {
  it('список без этой джобы — новое демо через submit, кеш обновлён', async () => {
    storage.setItem(KEY, JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load(async () => [job({ id: 99, title: 'чужой трек', folder: '' })])
    const id = await usePresetDemo().ensure('Рок')
    expect(id).toBe(777)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
    expect(JSON.parse(storage.getItem(KEY))).toEqual({ 'Рок': 777 })
  })

  it('пустой список джоб — тоже «удалили», новое демо', async () => {
    storage.setItem(KEY, JSON.stringify({ 'Рок': 12 }))
    const { usePresetDemo } = await load(async () => [])
    expect(await usePresetDemo().ensure('Рок')).toBe(777)
    expect(apiMock.submit).toHaveBeenCalledTimes(1)
  })
})
