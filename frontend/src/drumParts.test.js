// Части барабанов в студии.
// Подписи частей (ru/en) — ключи studio.dsp.target.<часть>; паритет словарей
// в целом проверяет i18n.test.js, здесь — что ключи частей есть в обоих и
// русские подписи ожидаемые.
import { expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const PARTS = ['kick', 'snare', 'toms', 'hh', 'ride', 'crash']
const RU_LABELS = {
  kick: 'бочка',
  snare: 'малый барабан',
  toms: 'томы',
  hh: 'хай-хэт',
  ride: 'райд',
  crash: 'крэш',
}

test('подписи частей барабанов есть в ru и en', () => {
  for (const p of PARTS) {
    const key = `studio.dsp.target.${p}`
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      expect(typeof dict[key], `${lang}: нет ${key}`).toBe('string')
      expect(dict[key].trim(), `${lang}: пустая подпись ${key}`).not.toBe('')
      expect(dict[key], `${lang}: подпись ${key} — сырой ключ`).not.toBe(key)
    }
  }
})

test('русские подписи частей', () => {
  for (const p of PARTS) {
    expect(RU[`studio.dsp.target.${p}`].toLowerCase()).toBe(RU_LABELS[p])
  }
})

test('английские подписи частей различимы между собой', () => {
  const en = PARTS.map((p) => EN[`studio.dsp.target.${p}`].toLowerCase())
  expect(new Set(en).size).toBe(PARTS.length)
})
