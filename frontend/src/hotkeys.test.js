// ТК7 (internal-window-frame, пачка 3, условие 12): горячие клавиши вне полей ввода.
// keyAction(e) — чистая функция: событие клавиатуры → 'toggle' | 'back' | 'fwd' | 'search' | null.
import { describe, it, expect } from 'vitest'
import { keyAction } from './hotkeys.js'

const BODY = { tagName: 'BODY', isContentEditable: false }

// Событие-заготовка: по умолчанию без модификаторов, без повтора, цель — body.
const ev = (key, code, over = {}) => ({
  key, code,
  ctrlKey: false, metaKey: false, altKey: false, shiftKey: false,
  repeat: false, target: BODY,
  ...over,
})
const space = (over = {}) => ev(' ', 'Space', over)
const left = (over = {}) => ev('ArrowLeft', 'ArrowLeft', over)
const right = (over = {}) => ev('ArrowRight', 'ArrowRight', over)
const ctrlF = (over = {}) => ev('f', 'KeyF', { ctrlKey: true, ...over })
const metaF = (over = {}) => ev('f', 'KeyF', { metaKey: true, ...over })

describe('клавиши вне полей ввода', () => {
  it('пробел без модификаторов — играть/пауза', () => {
    expect(keyAction(space())).toBe('toggle')
  })

  it('← — перемотка назад, → — вперёд', () => {
    expect(keyAction(left())).toBe('back')
    expect(keyAction(right())).toBe('fwd')
  })

  it('Ctrl+F и Cmd+F — к поиску треков', () => {
    expect(keyAction(ctrlF())).toBe('search')
    expect(keyAction(metaF())).toBe('search')
  })

  it('Ctrl+F с заглавной F (Caps Lock) — тоже поиск', () => {
    expect(keyAction(ev('F', 'KeyF', { ctrlKey: true }))).toBe('search')
  })

  it('Ctrl+F на русской раскладке (клавиша «а») — тоже поиск', () => {
    expect(keyAction(ev('а', 'KeyF', { ctrlKey: true }))).toBe('search')
    expect(keyAction(ev('а', 'KeyF', { metaKey: true }))).toBe('search')
  })
})

describe('модификаторы и повтор', () => {
  it('пробел с Ctrl — не играть/пауза', () => {
    expect(keyAction(space({ ctrlKey: true }))).toBeNull()
  })

  it('удержание пробела (повтор) — не переключает снова', () => {
    expect(keyAction(space({ repeat: true }))).toBeNull()
  })

  it('F без Ctrl/Cmd — ничего', () => {
    expect(keyAction(ev('f', 'KeyF'))).toBeNull()
    expect(keyAction(ev('а', 'KeyF'))).toBeNull()
  })
})

describe('другие клавиши', () => {
  it('буквы, Enter, Escape, ↑/↓ — ничего', () => {
    expect(keyAction(ev('a', 'KeyA'))).toBeNull()
    expect(keyAction(ev('Enter', 'Enter'))).toBeNull()
    expect(keyAction(ev('Escape', 'Escape'))).toBeNull()
    expect(keyAction(ev('ArrowUp', 'ArrowUp'))).toBeNull()
    expect(keyAction(ev('ArrowDown', 'ArrowDown'))).toBeNull()
  })

  it('Ctrl с другой буквой — ничего', () => {
    expect(keyAction(ev('s', 'KeyS', { ctrlKey: true }))).toBeNull()
  })
})

describe('в полях ввода горячих клавиш нет', () => {
  // Текстоподобные поля: туда печатают, пробел и стрелки принадлежат полю.
  const fields = {
    'input text': { tagName: 'INPUT', type: 'text', isContentEditable: false },
    'input search': { tagName: 'INPUT', type: 'search', isContentEditable: false },
    'input number': { tagName: 'INPUT', type: 'number', isContentEditable: false },
    'input без type': { tagName: 'INPUT', type: '', isContentEditable: false },
    textarea: { tagName: 'TEXTAREA', isContentEditable: false },
    select: { tagName: 'SELECT', isContentEditable: false },
    contenteditable: { tagName: 'DIV', isContentEditable: true },
  }

  for (const [name, target] of Object.entries(fields)) {
    it(`${name} — null для всех клавиш`, () => {
      expect(keyAction(space({ target }))).toBeNull()
      expect(keyAction(left({ target }))).toBeNull()
      expect(keyAction(right({ target }))).toBeNull()
      expect(keyAction(ctrlF({ target }))).toBeNull()
      expect(keyAction(metaF({ target }))).toBeNull()
    })
  }

  // range/checkbox — не поле для текста: поиск по Ctrl+F из них работает.
  // Пробел и стрелки здесь не проверяются — у ползунка и флажка они свои в браузере.
  it('ползунок и флажок не считаются полем ввода для Ctrl+F', () => {
    const range = { tagName: 'INPUT', type: 'range', isContentEditable: false }
    const checkbox = { tagName: 'INPUT', type: 'checkbox', isContentEditable: false }
    expect(keyAction(ctrlF({ target: range }))).toBe('search')
    expect(keyAction(ctrlF({ target: checkbox }))).toBe('search')
  })
})
