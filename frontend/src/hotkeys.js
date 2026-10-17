// Горячие клавиши приложения (вне полей ввода): пробел — играть/пауза, ←/→ — перемотка,
// Ctrl/Cmd+F — к поиску треков. Чистая функция: событие → действие, без DOM-побочек.

// поля, куда печатают: в них пробел, стрелки и Ctrl+F принадлежат полю
const TEXT_INPUTS = new Set(['', 'text', 'search', 'number', 'email', 'url', 'tel', 'password'])
function isTextField(t) {
  if (!t) return false
  if (t.isContentEditable) return true
  const tag = String(t.tagName || '').toUpperCase()
  if (tag === 'TEXTAREA' || tag === 'SELECT') return true
  return tag === 'INPUT' && TEXT_INPUTS.has(String(t.type || '').toLowerCase())
}
// ползунок и флажок: пробел и стрелки у них свои (сдвинуть, отметить)
function isOwnKeysControl(t) {
  const tag = String(t?.tagName || '').toUpperCase()
  return tag === 'INPUT' && ['range', 'checkbox', 'radio'].includes(String(t.type || '').toLowerCase())
}

export function keyAction(e) {
  if (!e || isTextField(e.target)) return null
  const mod = e.ctrlKey || e.metaKey
  // по коду клавиши, а не символу: на русской раскладке F — это «а»
  if (mod && !e.altKey && e.code === 'KeyF') return 'search'
  if (mod || e.altKey || isOwnKeysControl(e.target)) return null
  if (e.code === 'Space' || e.key === ' ') return e.repeat ? null : 'toggle'
  if (e.key === 'ArrowLeft') return 'back'
  if (e.key === 'ArrowRight') return 'fwd'
  return null
}
