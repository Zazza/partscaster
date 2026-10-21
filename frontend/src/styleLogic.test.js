// Тесты чистой логики стиля/стиха — по спецификации, как должно работать.
import { describe, it, expect } from 'vitest'
import {
  buildStyleLine, dictStyle, cleanLyrics, effectiveLyrics, instrumentalLyrics, SLOT_ORDER,
  splitSlotTags, slotTagList, pickMultiSlotTag, parseStyleLine,
} from './styleLogic.js'
import { slotOptions, slotKeys, multiSlots } from './slotOptions.js'

describe('buildStyleLine', () => {
  it('склеивает непустые слоты в порядке формы, BPM последним', () => {
    const line = buildStyleLine({
      language: 'Russian', genre: 'блюз', rhythm: '', guitars: 'слайд-гитара',
      keys: '', vocals: '', mood: 'грустно', production: 'чистая студия', bpm: 90,
    })
    expect(line).toBe('Russian, блюз, слайд-гитара, грустно, чистая студия, 90 BPM')
  })

  it('пропускает bpm=0/null и пустую стойку', () => {
    expect(buildStyleLine({ genre: 'панк', bpm: 0, bpm2: null }, '')).toBe('панк')
    expect(buildStyleLine({ genre: 'панк', bpm: null })).toBe('панк')
  })

  it('строка стойки идёт после слотов, до BPM', () => {
    const line = buildStyleLine({ genre: 'рок', bpm: 120 }, 'fuzz lead guitar')
    expect(line).toBe('рок, fuzz lead guitar, 120 BPM')
  })

  it('пустая форма → пустая строка (кнопки отправки заблокированы)', () => {
    expect(buildStyleLine({})).toBe('')
  })
})

describe('dictStyle', () => {
  it('известное русское значение заменяется английским тегом', () => {
    expect(dictStyle('блюз')).toBe('blues')
    expect(dictStyle('Блюз')).toBe('blues')   // регистронезависимо
  })

  it('самописное значение проходит как есть (уйдёт в автоперевод)', () => {
    expect(dictStyle('марши-фанфары космодрома')).toBe('марши-фанфары космодрома')
  })

  it('английское проходит без изменений', () => {
    expect(dictStyle('doom metal')).toBe('doom metal')
  })

  it('список значений переводится поштучно (мультивыбор слотов)', () => {
    expect(dictStyle('блюз, дум-метал')).toBe('blues, doom metal')
    expect(dictStyle('блюз , дум-метал ; панк  ')).toBe('blues, doom metal, punk')
  })

  it('словарная подпись с запятой внутри переводится целиком — и в списке с другими', () => {
    expect(dictStyle('1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски'))
      .toBe('1980s Siberian underground rock, punk energy over post-punk gloom')
    expect(dictStyle('1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски, блюз'))
      .toBe('1980s Siberian underground rock, punk energy over post-punk gloom, blues')
  })

  it('английский словарный тег с запятой не разрушается поштучным переводом', () => {
    // en-значение из словаря само содержит запятую: split/join возвращает его без изменений
    expect(dictStyle('chiptune punk, 8-bit punk')).toBe('chiptune punk, 8-bit punk')
    // рядом со словарным русским — переводится только русское
    expect(dictStyle('чиптюн-панк (нинтендо-панк), блюз')).toBe('chiptune punk, 8-bit punk, blues')
  })

  it('пустая строка и одни разделители → пустая строка', () => {
    expect(dictStyle('')).toBe('')
    expect(dictStyle('  , , ; ')).toBe('')
  })

  it('каждое значение словаря переводится само в себя (целостность словаря)', () => {
    for (const [ru, en] of slotOptions.genre) {
      expect(dictStyle(ru), ru).toBe(en)
    }
  })
})

describe('splitSlotTags', () => {
  it('режет по запятой и точке с запятой, пустые элементы выпадают', () => {
    expect(splitSlotTags('блюз, дум-метал; панк')).toEqual(['блюз', 'дум-метал', 'панк'])
    expect(splitSlotTags('блюз, , ')).toEqual(['блюз'])
    expect(splitSlotTags('')).toEqual([])
  })
})

describe('slotTagList', () => {
  it('куски составного словарного значения склеиваются в один логический тег', () => {
    expect(slotTagList('блюз, дум-метал')).toEqual(['блюз', 'дум-метал'])
    expect(slotTagList('1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски, блюз'))
      .toEqual(['1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски', 'блюз'])
  })

  it('несловарный кусок — самостоятельный тег', () => {
    expect(slotTagList('марши-фанфары космодрома, блюз')).toEqual(['марши-фанфары космодрома', 'блюз'])
    expect(slotTagList('')).toEqual([])
  })
})

describe('pickMultiSlotTag', () => {
  it('первый тег с пустого поля — тег и запятая-приглашение продолжить', () => {
    expect(pickMultiSlotTag('', 'блюз')).toBe('блюз, ')
    expect(pickMultiSlotTag(undefined, 'блюз')).toBe('блюз, ')
  })

  it('следующий тег дописывается к списку', () => {
    expect(pickMultiSlotTag('блюз, ', 'дум-метал')).toBe('блюз, дум-метал, ')
  })

  it('хвост-черновик (им фильтровали подсказки) заменяется выбранным тегом', () => {
    expect(pickMultiSlotTag('блюз, ду', 'дум-метал')).toBe('блюз, дум-метал, ')
  })

  it('уже выбранный тег не дублируется', () => {
    expect(pickMultiSlotTag('блюз, дум-метал, ', 'блюз')).toBe('блюз, дум-метал, ')
    expect(pickMultiSlotTag('блюз, дум-метал, ', 'Дум-метал')).toBe('блюз, дум-метал, ')
  })

  it('составное словарное значение не дублируется и не режется при повторном выборе', () => {
    const v = '1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски'
    expect(pickMultiSlotTag(v + ', ', v)).toBe(v + ', ')
  })
})

describe('parseStyleLine', () => {
  it('словарные значения расходятся по своим слотам, «N BPM» — в BPM', () => {
    const r = parseStyleLine('Russian, блюз, слайд-гитара, 120 BPM')
    expect(r.slots).toEqual({ language: 'Русский', genre: 'блюз', guitars: 'слайд-гитара' })
    expect(r.bpm).toBe(120)
    expect(r.unknown).toEqual([])
    expect(r.rack).toEqual([])
  })

  it('мультислот копит несколько значений', () => {
    const r = parseStyleLine('блюз, дум-метал')
    expect(r.slots.genre).toBe('блюз, дум-метал')
  })

  it('английская строка — в поля ложатся русские подписи (locale ru)', () => {
    const r = parseStyleLine('blues, doom metal, 90 BPM')
    expect(r.slots).toEqual({ genre: 'блюз, дум-метал' })
    expect(r.bpm).toBe(90)
  })

  it('фраза стойки с эффектом — карточкой в стойку, с инструментом и эффектом', () => {
    const r = parseStyleLine('fuzz wall of guitars drenched in reverb, sitar')
    expect(r.rack).toEqual([{ id: 'gtr-fuzz', effect: 'reverb' }, { id: 'sitar', effect: '' }])
    expect(r.unknown).toEqual([])
  })

  it('значение, совпадающее и со слотом, и со стойкой, — в слот (словарные значения первичны)', () => {
    const r = parseStyleLine('fuzz wall of guitars')
    expect(r.slots.guitars).toBe('фузз-стена гитар')
    expect(r.rack).toEqual([])
  })

  it('составные подписи (запятая внутри) и «без голоса» склеиваются целиком', () => {
    const r = parseStyleLine('1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски, instrumental, no vocals')
    expect(r.slots.genre).toBe('1980-е сибирский андеграунд, панк-энергия поверх пост-панк тоски')
    expect(r.slots.vocals).toBe('без голоса, только инструменты')
  })

  it('одиночный слот берёт первое значение, второе — в нераспознанные', () => {
    const r = parseStyleLine('Russian, English')
    expect(r.slots.language).toBe('Русский')
    expect(r.unknown).toEqual(['English'])
  })

  it('несловарные куски — в нераспознанные, порядок сохраняется', () => {
    const r = parseStyleLine('blues, hyped 90s radio vocals, doom metal')
    expect(r.slots.genre).toBe('блюз, дум-метал')
    expect(r.unknown).toEqual(['hyped 90s radio vocals'])
  })

  it('пустая строка — пустой разбор', () => {
    const r = parseStyleLine('')
    expect(r).toEqual({ slots: {}, bpm: null, rack: [], unknown: [] })
  })
})

describe('cleanLyrics', () => {
  it('вырезает строки-пометки с #, остальные сохраняет как есть', () => {
    const src = '[Verse]\n# ударения: сАмо\nтекст строки\n  # отступ-пометка\n[Chorus]\nприпев'
    expect(cleanLyrics(src)).toBe('[Verse]\nтекст строки\n[Chorus]\nприпев')
  })

  it('пустой текст → пустой текст', () => {
    expect(cleanLyrics('')).toBe('')
  })
})

// internal-vram-context усл. 6: выбора длительности нет — заглушка всегда из одной секции
describe('instrumentalLyrics / effectiveLyrics', () => {
  it('instrumentalLyrics() без аргумента — одна секция [Instrumental]', () => {
    expect(instrumentalLyrics()).toBe('[Instrumental]')
  })

  it('effectiveLyrics(lyrics, noLyrics): без слов — заглушка, со словами — чистка #', () => {
    expect(effectiveLyrics('[Verse]\nx\n# пометка', true)).toBe('[Instrumental]')
    expect(effectiveLyrics('', true)).toBe('[Instrumental]')
    expect(effectiveLyrics('[Verse]\nтекст\n# пометка', false)).toBe('[Verse]\nтекст')
  })
})

describe('целостность данных слотов', () => {
  it('slotMeta покрывает ровно те поля, что идут в строку стиля (без bpm)', () => {
    const metaKeys = [...slotKeys].sort()
    expect(metaKeys).toEqual([...SLOT_ORDER].sort())
  })

  it('мультивыбор — только существующие поля; язык, ритм и голос одиночные', () => {
    for (const k of multiSlots) expect(slotKeys, k).toContain(k)
    for (const single of ['language', 'rhythm', 'vocals']) expect(multiSlots.has(single)).toBe(false)
  })

  it('каждая опция — пара непустых [ru, en]', () => {
    for (const [key, opts] of Object.entries(slotOptions)) {
      expect(opts.length, key).toBeGreaterThan(0)
      for (const [ru, en] of opts) {
        expect(String(ru).trim(), `${key}: ${ru}`).not.toBe('')
        expect(String(en).trim(), `${key}: ${ru}`).not.toBe('')
      }
    }
  })

  it('русские значения не дублируются внутри одного поля', () => {
    for (const [key, opts] of Object.entries(slotOptions)) {
      const rus = opts.map(([ru]) => ru.trim().toLowerCase())
      expect(new Set(rus).size, key).toBe(rus.length)
    }
  })
})
