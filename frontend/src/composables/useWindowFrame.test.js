// Тесты заголовка своей полосы окна (useWindowFrame): до вызова показанный
// заголовок пуст (полоса тогда показывает название приложения из i18n),
// setWindowTitle меняет показанный заголовок и ставит системный — для панели
// задач и alt-tab. Привязки Wails в тестах не работают — мок на границе.
// Модуль — синглтон: каждый тест импортирует заново.
import { describe, it, expect, vi, beforeEach } from 'vitest'

const appMock = vi.hoisted(() => ({ YueSetWindowTitle: null }))
vi.mock('../wailsjs/go/main/App', () => ({
  YueSetWindowTitle: (...args) => appMock.YueSetWindowTitle(...args),
}))

async function load() {
  vi.resetModules()
  return import('./useWindowFrame.js')
}

beforeEach(() => {
  appMock.YueSetWindowTitle = vi.fn(async () => {})
})

describe('заголовок окна (ТК5)', () => {
  it('до вызова показанный заголовок пуст, системный не ставился', async () => {
    const { windowTitle } = await load()
    expect(windowTitle.value).toBe('')
    expect(appMock.YueSetWindowTitle).not.toHaveBeenCalled()
  })

  it("setWindowTitle('X') — показан 'X', системный вызван с 'X'", async () => {
    const { windowTitle, setWindowTitle } = await load()
    setWindowTitle('X')
    expect(windowTitle.value).toBe('X')
    expect(appMock.YueSetWindowTitle).toHaveBeenCalledTimes(1)
    expect(appMock.YueSetWindowTitle).toHaveBeenCalledWith('X')
  })

  it('повторный вызов — показан последний заголовок, системный получает каждый', async () => {
    const { windowTitle, setWindowTitle } = await load()
    setWindowTitle('Partscaster')
    setWindowTitle('Студия · #42 — Gone')
    expect(windowTitle.value).toBe('Студия · #42 — Gone')
    expect(appMock.YueSetWindowTitle.mock.calls).toEqual([['Partscaster'], ['Студия · #42 — Gone']])
  })
})
