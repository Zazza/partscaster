// internal-drop-ollama, условие 4: кнопки тегов секций над полем стихов.
// Чистая функция вставки тега insertSectionTag(text, selStart, selEnd, tag) → {text, caret}:
// тег встаёт в место курсора отдельной строкой — перенос до, если курсор не в начале строки;
// перенос после, если сразу за тегом нет перевода строки; выделение заменяется тегом;
// курсор — сразу после вставленного. Остальной текст не меняется.
import { describe, expect, test } from 'vitest'
import { insertSectionTag, SECTION_TAGS } from './lyricsTags.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

// Общий инвариант: результат = исходное до выделения + (\n?)тег(\n?) + исходное после;
// курсор — на границе вставки и хвоста; тег ни с чем не склеен.
function expectInserted(src, a, b, tag, res) {
  const before = src.slice(0, a)
  const after = src.slice(b)
  expect(res.text.startsWith(before), `начало текста сохранено: ${JSON.stringify(res.text)}`).toBe(true)
  expect(res.text.endsWith(after), `хвост текста сохранён: ${JSON.stringify(res.text)}`).toBe(true)
  const middle = res.text.slice(before.length, res.text.length - after.length)
  expect(middle, 'вставлен только тег и переводы строк').toMatch(
    new RegExp(`^\\n?${tag.replace(/[[\]-]/g, '\\$&')}\\n?$`))
  const at = before.length + middle.indexOf(tag)
  const prev = res.text[at - 1]
  const next = res.text[at + tag.length]
  expect(prev === undefined || prev === '\n', `слева от тега — начало строки, а не ${JSON.stringify(prev)}`).toBe(true)
  expect(next, 'справа от тега — перевод строки').toBe('\n')
  expect(res.caret, 'курсор — сразу после вставки').toBe(before.length + middle.length)
}

describe('insertSectionTag: вставка тега в место курсора отдельной строкой', () => {
  test('пустое поле → "[Verse]\\n", курсор после перевода строки', () => {
    const r = insertSectionTag('', 0, 0, '[Verse]')
    expect(r).toEqual({ text: '[Verse]\n', caret: 8 })
  })

  test('курсор в начале строки перед текстом → тег и перенос, текст — на следующей строке', () => {
    const r = insertSectionTag('hello world', 0, 0, '[Verse]')
    expect(r.text).toBe('[Verse]\nhello world')
    expect(r.caret).toBe(8)
  })

  test('курсор в начале второй строки перед текстом → перенос после, без переноса до', () => {
    const src = 'line one\nline two'
    const r = insertSectionTag(src, 9, 9, '[Chorus]')
    expect(r.text).toBe('line one\n[Chorus]\nline two')
    expect(r.caret).toBe('line one\n[Chorus]\n'.length)
  })

  test('курсор в середине строки → перенос до тега и после', () => {
    const r = insertSectionTag('hello world', 5, 5, '[Bridge]')
    expect(r.text).toBe('hello\n[Bridge]\n world')
    expect(r.caret).toBe('hello\n[Bridge]\n'.length)
  })

  test('курсор в конце непустой последней строки → "\\n[Chorus]\\n", курсор в конце', () => {
    const r = insertSectionTag('la la la', 8, 8, '[Chorus]')
    expect(r.text).toBe('la la la\n[Chorus]\n')
    expect(r.caret).toBe(r.text.length)
  })

  test('курсор на пустой строке между строфами → лишних пустых строк не добавляется', () => {
    const src = 'one\n\ntwo'
    const r = insertSectionTag(src, 4, 4, '[Chorus]')
    expect(r.text).toBe('one\n[Chorus]\ntwo')
    expectInserted(src, 4, 4, '[Chorus]', r)
  })

  test('курсор в конце строки, за которой идёт следующая → перенос до, свой перенос после не дублируется', () => {
    const src = 'one\ntwo'
    const r = insertSectionTag(src, 3, 3, '[Outro]')
    expect(r.text).toBe('one\n[Outro]\ntwo')
    expectInserted(src, 3, 3, '[Outro]', r)
  })

  test('выделение целой строки заменяется тегом на отдельной строке', () => {
    const src = 'a\nOLD\nb'
    const r = insertSectionTag(src, 2, 5, '[Bridge]')
    expect(r.text).toBe('a\n[Bridge]\nb')
    expect(r.text).not.toContain('OLD')
    expectInserted(src, 2, 5, '[Bridge]', r)
  })

  test('выделение внутри строки заменяется тегом, соседний текст — на своих строках', () => {
    const src = 'xxOLDyy'
    const r = insertSectionTag(src, 2, 5, '[Intro]')
    expect(r.text).toBe('xx\n[Intro]\nyy')
    expect(r.caret).toBe('xx\n[Intro]\n'.length)
  })

  test('выделение всего текста → только тег', () => {
    const r = insertSectionTag('old text\nmore', 0, 13, '[Verse]')
    expect(r).toEqual({ text: '[Verse]\n', caret: 8 })
  })

  test('тег никогда не склеивается с соседним текстом — в любом месте курсора', () => {
    const src = '[Verse]\nfirst line\n\nsecond, line\n[Chorus]\nla la'
    for (let p = 0; p <= src.length; p++) {
      const r = insertSectionTag(src, p, p, '[Interlude]')
      expectInserted(src, p, p, '[Interlude]', r)
    }
  })

  test('исходная строка не мутируется, результат — новый объект', () => {
    const src = 'abc'
    const r1 = insertSectionTag(src, 1, 1, '[Verse]')
    const r2 = insertSectionTag(src, 1, 1, '[Verse]')
    expect(src).toBe('abc')
    expect(r1).toEqual(r2)
  })
})

describe('SECTION_TAGS: набор тегов и подписи', () => {
  test('ровно семь тегов карточки, в её порядке', () => {
    expect(SECTION_TAGS.map((s) => s.tag)).toEqual([
      '[Verse]', '[Pre-Chorus]', '[Chorus]', '[Bridge]', '[Intro]', '[Outro]', '[Interlude]',
    ])
  })

  test('id уникальны и непусты', () => {
    const ids = SECTION_TAGS.map((s) => s.id)
    expect(ids.every((id) => typeof id === 'string' && id.length > 0)).toBe(true)
    expect(new Set(ids).size).toBe(ids.length)
  })

  test('подпись каждого тега есть в ru и en (ключ lyrics.tag.<id>)', () => {
    for (const { id } of SECTION_TAGS) {
      expect(RU[`lyrics.tag.${id}`], `ru: lyrics.tag.${id}`).toBeTruthy()
      expect(EN[`lyrics.tag.${id}`], `en: lyrics.tag.${id}`).toBeTruthy()
    }
  })

  test('русские подписи — Куплет, Предприпев, Припев, Бридж, Вступление, Концовка, Проигрыш', () => {
    const ru = SECTION_TAGS.map(({ id }) => (RU[`lyrics.tag.${id}`] || '').trim().toLowerCase())
    expect(ru).toEqual(['куплет', 'предприпев', 'припев', 'бридж', 'вступление', 'концовка', 'проигрыш'])
  })

  test('английские подписи не совпадают с русскими (перевод есть)', () => {
    for (const { id } of SECTION_TAGS) {
      expect(EN[`lyrics.tag.${id}`]).not.toBe(RU[`lyrics.tag.${id}`])
      expect(EN[`lyrics.tag.${id}`]).not.toMatch(/[А-Яа-яЁё]/)
    }
  })
})
