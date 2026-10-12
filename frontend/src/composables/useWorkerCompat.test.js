// Плашка совместимости воркера:
// ответ проверки версии, пришедший после recheck (запрос к старому адресу), игнорируется —
// плашку определяет последняя проверка, а не последний пришедший ответ.
// Модуль — синглтон: каждый тест импортирует его заново.
import { describe, it, expect, vi, beforeEach } from 'vitest'

const apiMock = vi.hoisted(() => ({}))
vi.mock('../api.js', () => ({ api: apiMock }))

function deferred() {
  let resolve
  const promise = new Promise((r) => { resolve = r })
  return { promise, resolve }
}

const flush = () => new Promise((r) => setTimeout(r, 0))

describe('useWorkerCompat: устаревший ответ после recheck', () => {
  let useWorkerCompat
  beforeEach(async () => {
    vi.resetModules()
    for (const k of Object.keys(apiMock)) delete apiMock[k]
    ;({ useWorkerCompat } = await import('./useWorkerCompat.js'))
  })

  it('ответ старого адреса, пришедший последним, не ставит плашку', async () => {
    const a = deferred()
    const b = deferred()
    const queue = [a.promise, b.promise]
    apiMock.workerVersion = vi.fn(() => queue.shift())
    const { warning, onHealth, recheck } = useWorkerCompat()

    onHealth({}) // запрос A висит
    await flush()
    recheck() // адрес сменился
    onHealth({}) // запрос B
    await flush()
    expect(apiMock.workerVersion).toHaveBeenCalledTimes(2)

    b.resolve({ warning: '' })
    await flush()
    a.resolve({ warning: 'обновите воркер' })
    await flush()

    expect(warning.value).toBe('')
  })
})
