// Тесты фильтра библиотеки «Пресеты звука» (карточка internal-window-frame, пачка 6, ТК10–ТК12).
// Дерево — как presetTree: [{ family, label, genres: [{ genre, items: [{ id, name, note, family }] }] }],
// семья '' — «Мои». Написаны по условиям карточки, без взгляда на реализацию.
import { describe, it, expect } from 'vitest'
import { minesFirst, familyCounts, filterPresetTree } from './presetFilter.js'

const item = (id, name, note, family) => ({ id, name, note, family })

// Небольшое дерево: «Рок» (два течения), «Электроника» (одно), «Мои» — последней, чтобы проверять перенос.
const makeTree = () => [
  {
    family: 'Рок',
    label: 'Рок',
    genres: [
      { genre: 'пост-панк', items: [item('pp1', 'Холодный бас', 'сухая гитара и хорус', 'Рок'), item('pp2', 'Joy', 'гулкий малый', 'Рок')] },
      { genre: 'гранж', items: [item('gr1', 'Seattle', 'грязный перегруз', 'Рок')] },
    ],
  },
  {
    family: 'Электроника',
    label: 'Электроника',
    genres: [
      { genre: 'синтвейв', items: [item('sw1', 'Neon', 'широкий пэд', 'Электроника'), item('sw2', 'Drive', 'бочка вперёд', 'Электроника')] },
    ],
  },
  {
    family: '',
    label: 'Мои',
    genres: [
      { genre: 'мои', items: [item('m1', 'Альбом-мастер', 'мой финал', '')] },
    ],
  },
]

const families = tree => tree.map(f => f.family)
const ids = tree => tree.flatMap(f => f.genres.flatMap(g => g.items.map(i => i.id)))
const genres = tree => tree.flatMap(f => f.genres.map(g => g.genre))

describe('ТК10 minesFirst', () => {
  it('семья «Мои» переносится в начало, порядок остальных сохраняется', () => {
    expect(families(minesFirst(makeTree()))).toEqual(['', 'Рок', 'Электроника'])
  })

  it('«Мои» из середины — тоже в начало, остальные в прежнем порядке', () => {
    const t = makeTree()
    const tree = [t[0], t[2], t[1]]
    expect(families(minesFirst(tree))).toEqual(['', 'Рок', 'Электроника'])
  })

  it('содержимое семей не теряется', () => {
    const out = minesFirst(makeTree())
    expect(out[0].label).toBe('Мои')
    expect(ids(out).sort()).toEqual(ids(makeTree()).sort())
  })

  it('без «Моих» — порядок как был', () => {
    const tree = makeTree().filter(f => f.family !== '')
    expect(families(minesFirst(tree))).toEqual(['Рок', 'Электроника'])
  })

  it('«Мои» уже первыми — порядок не меняется', () => {
    const t = makeTree()
    expect(families(minesFirst([t[2], t[0], t[1]]))).toEqual(['', 'Рок', 'Электроника'])
  })

  it('пустое дерево — пустой результат', () => {
    expect(minesFirst([])).toEqual([])
  })

  it('вход не мутируется', () => {
    const tree = makeTree()
    const snapshot = JSON.parse(JSON.stringify(tree))
    minesFirst(tree)
    expect(tree).toEqual(snapshot)
  })
})

describe('ТК11 familyCounts', () => {
  it('первым — «все» с общим числом, затем семьи в порядке дерева', () => {
    expect(familyCounts(makeTree())).toEqual([
      { family: null, label: 'all', count: 6 },
      { family: 'Рок', label: 'Рок', count: 3 },
      { family: 'Электроника', label: 'Электроника', count: 2 },
      { family: '', label: 'Мои', count: 1 },
    ])
  })

  it('порядок — как в дереве: после minesFirst «Мои» идут сразу за «все»', () => {
    const out = familyCounts(minesFirst(makeTree()))
    expect(out.map(x => x.family)).toEqual([null, '', 'Рок', 'Электроника'])
  })

  it('пустое дерево — только «все» с нулём', () => {
    expect(familyCounts([])).toEqual([{ family: null, label: 'all', count: 0 }])
  })

  it('вход не мутируется', () => {
    const tree = makeTree()
    const snapshot = JSON.parse(JSON.stringify(tree))
    familyCounts(tree)
    expect(tree).toEqual(snapshot)
  })
})

describe('ТК12 filterPresetTree: фишка семьи', () => {
  it('family null и undefined — все семьи и все пресеты', () => {
    expect(ids(filterPresetTree(makeTree(), { q: '', family: null }))).toEqual(ids(makeTree()))
    expect(ids(filterPresetTree(makeTree(), { q: '', family: undefined }))).toEqual(ids(makeTree()))
    expect(ids(filterPresetTree(makeTree(), {}))).toEqual(ids(makeTree()))
  })

  it('family «Рок» — только она, со всеми течениями', () => {
    const out = filterPresetTree(makeTree(), { family: 'Рок' })
    expect(families(out)).toEqual(['Рок'])
    expect(ids(out)).toEqual(['pp1', 'pp2', 'gr1'])
  })

  it("family '' — только «Мои» (не путается с «все»)", () => {
    const out = filterPresetTree(makeTree(), { family: '' })
    expect(families(out)).toEqual([''])
    expect(ids(out)).toEqual(['m1'])
  })

  it('неизвестная семья — пусто', () => {
    expect(filterPresetTree(makeTree(), { family: 'Джаз' })).toEqual([])
  })
})

describe('ТК12 filterPresetTree: поиск', () => {
  it('пустой запрос и пробелы — без фильтра', () => {
    expect(ids(filterPresetTree(makeTree(), { q: '' }))).toEqual(ids(makeTree()))
    expect(ids(filterPresetTree(makeTree(), { q: '   ' }))).toEqual(ids(makeTree()))
    expect(ids(filterPresetTree(makeTree(), { q: undefined }))).toEqual(ids(makeTree()))
  })

  it('ищет подстроку в названии без учёта регистра', () => {
    expect(ids(filterPresetTree(makeTree(), { q: 'NEO' }))).toEqual(['sw1'])
    expect(ids(filterPresetTree(makeTree(), { q: 'холодн' }))).toEqual(['pp1'])
  })

  it('ищет в описании', () => {
    expect(ids(filterPresetTree(makeTree(), { q: 'Перегруз' }))).toEqual(['gr1'])
  })

  it('ищет в названии течения — течение целиком', () => {
    const out = filterPresetTree(makeTree(), { q: 'ПОСТ-панк' })
    expect(genres(out)).toEqual(['пост-панк'])
    expect(ids(out)).toEqual(['pp1', 'pp2'])
  })

  it('ищет в подписи семьи — семья целиком', () => {
    const out = filterPresetTree(makeTree(), { q: 'электро' })
    expect(families(out)).toEqual(['Электроника'])
    expect(ids(out)).toEqual(['sw1', 'sw2'])
  })

  it('«Мои» находятся по подписи семьи', () => {
    expect(ids(filterPresetTree(makeTree(), { q: 'мои' }))).toEqual(['m1'])
  })

  it('течения и семьи без совпадений выпадают', () => {
    const out = filterPresetTree(makeTree(), { q: 'гулкий' })
    expect(families(out)).toEqual(['Рок'])
    expect(genres(out)).toEqual(['пост-панк'])
    expect(ids(out)).toEqual(['pp2'])
  })

  it('совпадения в нескольких семьях — порядок дерева сохраняется', () => {
    // «ий» — в описаниях «гулкий малый» (Рок) и «широкий пэд» (Электроника)
    const out = filterPresetTree(makeTree(), { q: 'ий' })
    expect(families(out)).toEqual(['Рок', 'Электроника'])
    expect(ids(out)).toEqual(['pp2', 'sw1'])
  })

  it('ничего не нашлось — пустой массив', () => {
    expect(filterPresetTree(makeTree(), { q: 'джаз-фьюжн' })).toEqual([])
  })

  it('пустое дерево — пустой результат', () => {
    expect(filterPresetTree([], { q: 'рок', family: 'Рок' })).toEqual([])
  })
})

describe('ТК12 filterPresetTree: поиск и фишка вместе', () => {
  it('запрос ищет только внутри выбранной семьи', () => {
    // «бочка» — у sw2 (Электроника); внутри «Рок» её нет
    expect(ids(filterPresetTree(makeTree(), { q: 'бочка', family: null }))).toEqual(['sw2'])
    expect(filterPresetTree(makeTree(), { q: 'бочка', family: 'Рок' })).toEqual([])
    expect(ids(filterPresetTree(makeTree(), { q: 'гранж', family: 'Рок' }))).toEqual(['gr1'])
  })

  it('вход не мутируется', () => {
    const tree = makeTree()
    const snapshot = JSON.parse(JSON.stringify(tree))
    filterPresetTree(tree, { q: 'гулкий', family: 'Рок' })
    filterPresetTree(tree, { q: '', family: 'Электроника' })
    expect(tree).toEqual(snapshot)
  })
})
