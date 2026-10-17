// ТК8 (internal-window-frame, пачка 3, условие 17): голоса «манера» вместо «в духе».
// У каждой манеры есть подпись в ru и en; ни подписи, ни id не называют исполнителей.
import { describe, it, expect } from 'vitest'
import { VOICE_ARCHETYPES } from './voiceLab.js'
import ru from './i18n/ru.js'
import en from './i18n/en.js'

// Прежние имена из карточки. Ищутся как начало слова (с падежным окончанием до 2 букв),
// чтобы «придыхание» не ловилось как «Рид», а «Летова» — ловилось.
const NAMES = [
  'Летов', 'Йорк', 'Уайт', 'Placebo', 'Interpol', 'Земфира', 'Тролль', 'Кобейн', 'Кейв',
  'Уайнхаус', 'Бьорк', 'Рид', 'Высоцкий', 'БГ',
  'Letov', 'Yorke', 'White', 'Zemfira', 'Cobain', 'Cave', 'Winehouse', 'Bjork', 'Björk', 'Reed', 'Vysotsky',
]
const nameRe = n => new RegExp(`(?<!\\p{L})${n}[\\p{L}]{0,2}(?!\\p{L})`, 'iu')
const NAME_RES = NAMES.map(n => [n, nameRe(n)])
// Имена, найденные в строке.
const namesIn = s => NAME_RES.filter(([, re]) => re.test(s)).map(([n]) => n)

const key = id => 'voicelab.archetype.' + id

describe('проверка имён работает', () => {
  it('ловит имя с окончанием и не ловит обычные слова', () => {
    expect(namesIn('в духе Летова')).toEqual(['Летов'])
    expect(namesIn('like Cobain')).toEqual(['Cobain'])
    expect(namesIn('Bjork-style')).toEqual(['Bjork'])
    expect(namesIn('шёпот с придыханием')).toEqual([])
  })
})

describe('VOICE_ARCHETYPES', () => {
  it('список манер не пуст, id уникальны', () => {
    expect(Array.isArray(VOICE_ARCHETYPES)).toBe(true)
    expect(VOICE_ARCHETYPES.length).toBeGreaterThan(0)
    const ids = VOICE_ARCHETYPES.map(a => a.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('у каждой манеры есть непустая подпись в ru и en', () => {
    for (const { id } of VOICE_ARCHETYPES) {
      for (const [lang, dict] of [['ru', ru], ['en', en]]) {
        const label = dict[key(id)]
        expect(typeof label, `${lang} ${key(id)}`).toBe('string')
        expect(label.trim(), `${lang} ${key(id)}`).not.toBe('')
      }
    }
  })

  it('подписи манер в ru и en не содержат прежних имён', () => {
    for (const [lang, dict] of [['ru', ru], ['en', en]]) {
      for (const [k, v] of Object.entries(dict)) {
        if (!k.startsWith('voicelab.archetype.')) continue
        expect(namesIn(String(v)), `${lang} ${k}: ${v}`).toEqual([])
      }
    }
  })

  it('id манер не содержат прежних имён', () => {
    for (const { id } of VOICE_ARCHETYPES) {
      // id вида dark-drama: делим на части и проверяем каждую как слово.
      const words = String(id).split(/[^\p{L}]+/u).join(' ')
      expect(namesIn(words), id).toEqual([])
    }
  })
})
