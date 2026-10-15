// Задача internal-guitar-notes: подсказка, чем распознаются ноты гитары (фронт).
// Спецификация (карточка, условие 5, решение человека): для звуков «Ноты переиграны другой гитарой»
// приложение показывает, какими моделями распознаются ноты, или что распознавание не установлено.
// Контракт: notesHint(chain, cfg, t) → t(ключ) или '':
// - в цепочке нет блока type 'gtrnotes' → '' (подсказка не нужна), где бы он ни стоял — ищется по всей цепочке;
// - cfg — ответ воркера /config, может быть null/undefined (старый воркер или нет связи);
// - 'gnotes.missing' — cfg нет или notes_available не true;
// - 'gnotes.both' — в notes_engines есть и 'yourmt3', и 'basic-pitch';
// - 'gnotes.bp' — только 'basic-pitch';
// - ключи есть в i18n/ru.js и i18n/en.js, тексты непустые; ru для missing называет notes_install.sh.
import { describe, it, expect } from 'vitest'
import { notesHint } from './guitarNotesHint.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

// перевод-метка: по результату видно, какой ключ запрошен
const t = (key) => `T[${key}]`
const GTR = { type: 'gtrnotes', kit: 'nylonguitar/guitar' }
const DRIVE = { type: 'drive' }
const EQ = { type: 'eq' }
const cfgOf = (engines, available = true) => ({ notes_available: available, notes_engines: engines })

describe('notesHint: подсказка не нужна без блока gtrnotes (условие 5)', () => {
  it('пустая цепочка → пустая строка', () => {
    expect(notesHint([], cfgOf(['basic-pitch']), t)).toBe('')
  })

  it('цепочка без gtrnotes → пустая строка даже без распознавания', () => {
    expect(notesHint([DRIVE, EQ], null, t)).toBe('')
    expect(notesHint([DRIVE, EQ], cfgOf(['yourmt3', 'basic-pitch']), t)).toBe('')
  })
})

describe('notesHint: распознавание не установлено (условие 5)', () => {
  it.each([
    ['cfg = null (нет связи)', null],
    ['cfg = undefined (старый воркер)', undefined],
    ['notes_available = false', cfgOf(['basic-pitch'], false)],
    ['notes_available отсутствует', { notes_engines: ['basic-pitch'] }],
    ['notes_available не true, а строка', cfgOf(['basic-pitch'], 'true')],
  ])('%s → gnotes.missing', (_name, cfg) => {
    expect(notesHint([GTR], cfg, t)).toBe('T[gnotes.missing]')
  })
})

describe('notesHint: какими моделями распознаются ноты (условие 5)', () => {
  it('есть yourmt3 и basic-pitch → gnotes.both', () => {
    expect(notesHint([GTR], cfgOf(['yourmt3', 'basic-pitch']), t)).toBe('T[gnotes.both]')
  })

  it('порядок движков в ответе не важен → gnotes.both', () => {
    expect(notesHint([GTR], cfgOf(['basic-pitch', 'yourmt3']), t)).toBe('T[gnotes.both]')
  })

  it('только basic-pitch → gnotes.bp', () => {
    expect(notesHint([GTR], cfgOf(['basic-pitch']), t)).toBe('T[gnotes.bp]')
  })

  it('gtrnotes не первым блоком тоже находится', () => {
    expect(notesHint([DRIVE, EQ, GTR], cfgOf(['basic-pitch']), t)).toBe('T[gnotes.bp]')
    expect(notesHint([DRIVE, GTR, EQ], null, t)).toBe('T[gnotes.missing]')
  })

  it('возвращает именно перевод из t, а не ключ', () => {
    const tr = (key) => (key === 'gnotes.bp' ? 'перевод bp' : '?')
    expect(notesHint([GTR], cfgOf(['basic-pitch']), tr)).toBe('перевод bp')
  })
})

describe('notesHint: тексты подсказки в словарях (условие 5)', () => {
  const KEYS = ['gnotes.missing', 'gnotes.both', 'gnotes.bp']

  it.each(KEYS)('%s — непустой текст в ru и en', (key) => {
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      expect(typeof dict[key], `${lang}: нет ключа ${key}`).toBe('string')
      expect(dict[key].trim(), `${lang}: пустой ${key}`).not.toBe('')
    }
  })

  it('ru gnotes.missing называет скрипт установки notes_install.sh', () => {
    expect(RU['gnotes.missing']).toContain('notes_install.sh')
  })
})
