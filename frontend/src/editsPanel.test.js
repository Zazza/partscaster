// Окно правок студии (карточка internal-studio-instant-apply, условие 11; тест-кейс ТК10). Чистый модуль:
//   editsCountLabel(n, lang) — подпись свёрнутого окна «N правок» (склонение ru; en — edit/edits);
//   loadPanelOpen() / savePanelOpen(bool) — состояние «свёрнуто/раскрыто» запоминается (localStorage);
//   по умолчанию — свёрнуто; сбой localStorage не роняет.
//
// Толкование: ключ localStorage — дело модуля; проверяется круговая запись/чтение и поведение при сбое.
import { describe, it, expect, afterEach } from 'vitest'
import { editsCountLabel, loadPanelOpen, savePanelOpen } from './editsPanel.js'

class MemStorage {
  constructor() { this.m = new Map() }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
  removeItem(k) { this.m.delete(k) }
  clear() { this.m.clear() }
}

// хранилище, которое бросает на каждом вызове (приватный режим, квота)
class BrokenStorage {
  getItem() { throw new Error('SecurityError') }
  setItem() { throw new Error('QuotaExceededError') }
  removeItem() { throw new Error('SecurityError') }
  clear() { throw new Error('SecurityError') }
}

const saved = globalThis.localStorage
afterEach(() => {
  globalThis.localStorage = saved
})

describe('editsCountLabel — ru', () => {
  for (const [n, want] of [
    [0, '0 правок'], [1, '1 правка'], [2, '2 правки'], [3, '3 правки'], [4, '4 правки'], [5, '5 правок'],
    [11, '11 правок'], [12, '12 правок'], [14, '14 правок'], [21, '21 правка'], [22, '22 правки'],
    [25, '25 правок'], [101, '101 правка'], [111, '111 правок'], [112, '112 правок'],
  ]) {
    it(`${n} → «${want}»`, () => {
      expect(editsCountLabel(n, 'ru')).toBe(want)
    })
  }
})

describe('editsCountLabel — en', () => {
  for (const [n, want] of [[0, '0 edits'], [1, '1 edit'], [2, '2 edits'], [5, '5 edits'], [11, '11 edits'], [21, '21 edits']]) {
    it(`${n} → «${want}»`, () => {
      expect(editsCountLabel(n, 'en')).toBe(want)
    })
  }
})

describe('loadPanelOpen / savePanelOpen', () => {
  it('ничего не сохранено → свёрнуто (false)', () => {
    globalThis.localStorage = new MemStorage()
    expect(loadPanelOpen()).toBe(false)
  })

  it('раскрыли → после «перезапуска» раскрыто; свернули → свёрнуто', () => {
    globalThis.localStorage = new MemStorage()
    savePanelOpen(true)
    expect(loadPanelOpen()).toBe(true)
    savePanelOpen(false)
    expect(loadPanelOpen()).toBe(false)
  })

  it('состояние живёт в localStorage, а не в памяти модуля: новое хранилище — снова свёрнуто', () => {
    globalThis.localStorage = new MemStorage()
    savePanelOpen(true)
    globalThis.localStorage = new MemStorage()
    expect(loadPanelOpen()).toBe(false)
  })

  it('мусор в хранилище → свёрнуто, без исключения', () => {
    const st = new MemStorage()
    globalThis.localStorage = st
    savePanelOpen(true)
    for (const k of st.m.keys()) st.m.set(k, '{не json')
    expect(loadPanelOpen()).toBe(false)
  })

  it('localStorage бросает → чтение даёт свёрнуто, запись не роняет', () => {
    globalThis.localStorage = new BrokenStorage()
    expect(() => savePanelOpen(true)).not.toThrow()
    expect(loadPanelOpen()).toBe(false)
  })

  it('localStorage нет вовсе → свёрнуто, запись не роняет', () => {
    globalThis.localStorage = undefined
    expect(() => savePanelOpen(true)).not.toThrow()
    expect(loadPanelOpen()).toBe(false)
  })
})
