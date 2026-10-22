// Окно «нужен ffmpeg» по ОС (карточка internal-ffmpeg-pin, ТК2/ТК3).
// ТК2: ОС приходит из Wails Environment().platform; Windows — установка (кнопка «Скачать»),
// macOS — подсказка brew, Linux и всё неизвестное (включая ошибку Environment → undefined/null) —
// подсказка Linux-вида без кнопки «Скачать», без падения.
// ТК3: ключи текстов есть в ru и en, en без кириллицы; текст окна на unix не про 82 МБ/папку приложения.
import { describe, expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'
import { ffmpegVariant } from './ffmpegHint.js'

const CYR = /[Ѐ-ӿ]/

describe('ffmpegVariant: платформа → вариант окна (ТК2)', () => {
  test("'windows' → установка", () => {
    expect(ffmpegVariant('windows')).toEqual({ kind: 'install' })
  })
  test("'darwin' → подсказка macOS", () => {
    expect(ffmpegVariant('darwin')).toEqual({ kind: 'hint', hintKey: 'ffmpeg.hint.mac' })
  })
  test("'linux' → подсказка Linux", () => {
    expect(ffmpegVariant('linux')).toEqual({ kind: 'hint', hintKey: 'ffmpeg.hint.linux' })
  })
  test.each([['freebsd'], [''], ['Windows NT'], [undefined], [null]])(
    'неизвестная платформа %j → подсказка Linux, не установка, не падает',
    (p) => {
      expect(ffmpegVariant(p)).toEqual({ kind: 'hint', hintKey: 'ffmpeg.hint.linux' })
    },
  )
  test('без аргумента → подсказка Linux', () => {
    expect(ffmpegVariant()).toEqual({ kind: 'hint', hintKey: 'ffmpeg.hint.linux' })
  })
  test('ключ подсказки любого варианта есть в словарях', () => {
    for (const p of ['darwin', 'linux', undefined]) {
      const v = ffmpegVariant(p)
      expect(RU[v.hintKey], v.hintKey).toBeTruthy()
      expect(EN[v.hintKey], v.hintKey).toBeTruthy()
    }
  })
})

describe('тексты окна ffmpeg на unix (ТК3)', () => {
  const KEYS = ['ffmpeg.textUnix', 'ffmpeg.hint.linux', 'ffmpeg.hint.mac', 'ffmpeg.ok']

  test.each(KEYS)('ключ %s есть и непустой в ru и en', (k) => {
    expect(typeof RU[k], `ru ${k}`).toBe('string')
    expect(RU[k].trim(), `ru ${k}`).not.toBe('')
    expect(typeof EN[k], `en ${k}`).toBe('string')
    expect(EN[k].trim(), `en ${k}`).not.toBe('')
  })

  test.each(KEYS)('en %s без кириллицы', (k) => {
    expect(EN[k]).not.toMatch(CYR)
  })

  test.each([['ru', RU], ['en', EN]])('%s: подсказка Linux называет apt, dnf и pacman', (_, D) => {
    const s = D['ffmpeg.hint.linux']
    expect(s).toMatch(/\bapt\b/)
    expect(s).toMatch(/\bdnf\b/)
    expect(s).toMatch(/\bpacman\b/)
  })

  test.each([['ru', RU], ['en', EN]])('%s: подсказка macOS — brew install ffmpeg', (_, D) => {
    expect(D['ffmpeg.hint.mac']).toContain('brew install ffmpeg')
  })

  test.each([['ru', RU], ['en', EN]])(
    '%s: текст окна на unix не про размер скачивания и папку приложения',
    (_, D) => {
      const s = D['ffmpeg.textUnix']
      expect(s).not.toContain('82')
      expect(s).not.toMatch(/МБ|\bMB\b/i)
      expect(s).not.toMatch(/папк[уеиа] приложения/i)
      expect(s).not.toMatch(/app(lication)? folder/i)
    },
  )
})
