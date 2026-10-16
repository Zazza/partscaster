// Кнопки тегов секций над полем стихов: набор тегов и чистая вставка тега в место курсора.
// Подписи кнопок — i18n `lyrics.tag.<id>`, всплывающая подсказка — сам тег.

export const SECTION_TAGS = [
  { id: 'verse', tag: '[Verse]' },
  { id: 'prechorus', tag: '[Pre-Chorus]' },
  { id: 'chorus', tag: '[Chorus]' },
  { id: 'bridge', tag: '[Bridge]' },
  { id: 'intro', tag: '[Intro]' },
  { id: 'outro', tag: '[Outro]' },
  { id: 'interlude', tag: '[Interlude]' },
]

// Тег встаёт отдельной строкой вместо выделения [selStart, selEnd): перенос до — если курсор
// не в начале строки, перенос после — если за вставкой сразу не идёт перевод строки.
// Курсор — сразу после вставленного (тег и добавленные переносы).
export function insertSectionTag(text, selStart, selEnd, tag) {
  const src = text || ''
  const a = Math.max(0, Math.min(selStart, src.length))
  const b = Math.max(a, Math.min(selEnd, src.length))
  const before = src.slice(0, a)
  const after = src.slice(b)
  const middle = (before && !before.endsWith('\n') ? '\n' : '') + tag + (after.startsWith('\n') ? '' : '\n')
  return { text: before + middle + after, caret: before.length + middle.length }
}
