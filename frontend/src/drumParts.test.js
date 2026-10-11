// Карточка internal-roformer-stems, тест-кейс 10: части барабанов в студии.
// Подписи частей (ru/en) — ключи studio.dsp.target.<часть>; паритет словарей
// в целом проверяет i18n.test.js, здесь — что ключи частей есть в обоих и
// русские подписи те, что в условии 7. Переключатели «минуса» — только
// основные дорожки: части барабанов в сумму трека не входят.
import { expect, test } from 'vitest'
import { templateAst, findElements, dir, attr } from './testkit/sfc.js'
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

test('русские подписи частей — из условия карточки', () => {
  for (const p of PARTS) {
    expect(RU[`studio.dsp.target.${p}`].toLowerCase()).toBe(RU_LABELS[p])
  }
})

test('английские подписи частей различимы между собой', () => {
  const en = PARTS.map((p) => EN[`studio.dsp.target.${p}`].toLowerCase())
  expect(new Set(en).size).toBe(PARTS.length)
})

test('переключатели «минуса» — только основные дорожки, без частей барабанов', () => {
  const toggles = findElements(templateAst('components/StudioPage.vue'), (n) => /\bstem-toggle\b/.test(attr(n, 'class') || ''))
  expect(toggles.length, 'не найден список переключателей stem-toggle').toBeGreaterThan(0)
  for (const { node } of toggles) {
    const list = (dir(node, 'for') || '').split(/\s+in\s+/)[1]
    expect(JSON.parse(list.replace(/'/g, '"'))).toEqual(['drums', 'bass', 'other', 'vocals'])
  }
})
