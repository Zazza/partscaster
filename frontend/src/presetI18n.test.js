// internal-presets-en ТК2: localizePresets(list, locale) — английские name/note у встроенных пресетов
import { describe, it, expect } from 'vitest'
import { PRESET_EN, localizePresets } from './presetI18n.js'

const known = Object.keys(PRESET_EN)[0]

function sample() {
  return [
    { id: 1, slug: known, builtin: true, name: 'Русское · вариант', note: 'Русская подсказка', family: 'Рок' },
    { id: 2, slug: 'no-such-slug-xyz', builtin: true, name: 'Неизвестный', note: 'Как есть' },
    { id: 3, slug: known, builtin: false, name: 'Мой пресет', note: 'Моя заметка' },
    { id: 4, builtin: false, name: 'Без slug', note: '' },
  ]
}

describe('localizePresets', () => {
  it('словарь не пуст', () => {
    expect(known).toBeTruthy()
  })

  it('en: встроенный со slug из словаря получает английские name/note, прочие поля прежние', () => {
    const out = localizePresets(sample(), 'en')
    expect(out[0].name).toBe(PRESET_EN[known][0])
    expect(out[0].note).toBe(PRESET_EN[known][1])
    expect(out[0]).toMatchObject({ id: 1, slug: known, builtin: true, family: 'Рок' })
  })

  it('en: встроенный с неизвестным slug и свои пресеты — без изменений', () => {
    const src = sample()
    const out = localizePresets(src, 'en')
    expect(out).toHaveLength(4)
    expect(out[1]).toEqual(src[1])
    expect(out[2]).toEqual(src[2])
    expect(out[3]).toEqual(src[3])
  })

  it('en: исходный массив и его элементы не мутируют', () => {
    const src = sample()
    const copy = JSON.parse(JSON.stringify(src))
    localizePresets(src, 'en')
    expect(src).toEqual(copy)
  })

  it('ru: тот же список', () => {
    const src = sample()
    expect(localizePresets(src, 'ru')).toEqual(sample())
  })

  it('пустой список → пустой', () => {
    expect(localizePresets([], 'en')).toEqual([])
  })
})
