// Отметка «без слов» в режиме «стиль точной строкой 1:1» (решение человека 2026-10-13):
// строка стиля, вставленная целиком, при включённом «без слов» уходит на воркер
// с «Instrumental, no vocals, » в начале; при выключенном — как есть.
import { describe, it, expect } from 'vitest'
import * as sl from './styleLogic.js'

const PREFIX = 'Instrumental, no vocals, '
const line = (...a) => sl.instrumentalLine(...a)

describe('instrumentalLine: строка стиля 1:1 и отметка «без слов»', () => {
  it('выкл → строка без изменений', () => {
    const s = 'post-punk, shouted male vocals, 140 BPM'
    expect(line(s, false)).toBe(s)
  })

  it('выкл → пробелы по краям тоже не трогаются', () => {
    const s = '  punk, male vocals  '
    expect(line(s, false)).toBe(s)
  })

  it('выкл → пустая строка остаётся пустой', () => {
    expect(line('', false)).toBe('')
  })

  it('вкл → «Instrumental, no vocals, » в начале строки', () => {
    expect(line('post-punk, shouted male vocals', true))
      .toBe(PREFIX + 'post-punk, shouted male vocals')
  })

  it('вкл → строка обрезается от пробелов по краям перед пометкой', () => {
    expect(line('  dark synthwave, 120 BPM \n', true))
      .toBe(PREFIX + 'dark synthwave, 120 BPM')
  })

  it('вкл, строка уже начинается с «Instrumental» → не удваивается', () => {
    const s = 'Instrumental, no vocals, punk'
    expect(line(s, true)).toBe(s)
  })

  it('вкл, «instrumental» в другом регистре → не удваивается', () => {
    expect(line('instrumental post-rock', true)).toBe('instrumental post-rock')
    expect(line('INSTRUMENTAL, guitar', true)).toBe('INSTRUMENTAL, guitar')
  })

  it('вкл, «instrumental» после пробелов в начале → не удваивается', () => {
    const s = '   Instrumental metal'
    expect(line(s, true)).toBe(s)
  })

  it('вкл, «instrumental» не в начале → пометка добавляется', () => {
    expect(line('punk, instrumental break', true))
      .toBe(PREFIX + 'punk, instrumental break')
  })

  it('вкл, пустая строка → пустая (нечего помечать)', () => {
    expect(line('', true)).toBe('')
  })

  it('вкл, строка из пробелов → пустая', () => {
    expect(line('   \t ', true)).toBe('')
  })
})
