// Задача internal-guitar-plan-chords: подсказка у звуков «Гитарист вписывается по аккордам» (фронт).
// Спецификация (карточка, условие 9 и «Устройство (контракт)»: фронт):
// - notesHint(chain, t) → t('gnotes.plan'), если в цепочке есть блок type 'gtrnotes' (где бы он ни стоял), иначе '';
// - подсказка «ноты распознаются моделями / распознавание не установлено» удалена: ключей gnotes.missing,
//   gnotes.both, gnotes.bp в словарях нет, /config воркера (notes_*) подсказке не нужен;
// - ключ gnotes.plan есть в i18n/ru.js и i18n/en.js, тексты непустые.
import { describe, it, expect } from 'vitest'
import { notesHint } from './guitarNotesHint.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

// перевод-метка: по результату видно, какой ключ запрошен
const t = (key) => `T[${key}]`
const GTR = { type: 'gtrnotes', kit: 'nylonguitar/guitar', style: 2 }
const DRIVE = { type: 'drive' }
const EQ = { type: 'eq' }

describe('notesHint: без блока gtrnotes подсказки нет', () => {
  it('пустая цепочка → пустая строка', () => {
    expect(notesHint([], t)).toBe('')
  })

  it('цепочка без gtrnotes → пустая строка', () => {
    expect(notesHint([DRIVE, EQ], t)).toBe('')
  })

  it('цепочки нет (null/undefined) → пустая строка, без исключения', () => {
    expect(notesHint(null, t)).toBe('')
    expect(notesHint(undefined, t)).toBe('')
  })
})

describe('notesHint: с блоком gtrnotes — подсказка про аккорды плана', () => {
  it('gtrnotes первым блоком → gnotes.plan', () => {
    expect(notesHint([GTR], t)).toBe('T[gnotes.plan]')
  })

  it('gtrnotes не первым блоком тоже находится', () => {
    expect(notesHint([DRIVE, EQ, GTR], t)).toBe('T[gnotes.plan]')
    expect(notesHint([DRIVE, GTR, EQ], t)).toBe('T[gnotes.plan]')
  })

  it('возвращает именно перевод из t, а не ключ', () => {
    const tr = (key) => (key === 'gnotes.plan' ? 'перевод plan' : '?')
    expect(notesHint([GTR], tr)).toBe('перевод plan')
  })
})

describe('notesHint: тексты в словарях', () => {
  it('gnotes.plan — непустой текст в ru и en', () => {
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      expect(typeof dict['gnotes.plan'], `${lang}: нет ключа gnotes.plan`).toBe('string')
      expect(dict['gnotes.plan'].trim(), `${lang}: пустой gnotes.plan`).not.toBe('')
    }
  })

  it.each(['gnotes.missing', 'gnotes.both', 'gnotes.bp'])('подсказка распознавания %s удалена из ru и en', (key) => {
    expect(RU[key], `ru: остался ${key}`).toBeUndefined()
    expect(EN[key], `en: остался ${key}`).toBeUndefined()
  })

  it('ни один текст ru не отсылает к удалённому notes_install.sh', () => {
    const left = Object.entries(RU).filter(([, v]) => typeof v === 'string' && v.includes('notes_install.sh'))
    expect(left.map(([k]) => k)).toEqual([])
  })
})
