// Тесты окна «Что это» по шагам (internal-welcome-steps): какие шаги видны и в каком
// порядке (ТК1), с какой секунды играть другую версию пары при переключении «было/стало»
// (ТК2), и проверка данных welcomeClips.json: у шагов и пар есть ru и en (ТК7).
import { describe, it, expect } from 'vitest'
import { visibleSteps, switchStart } from './welcomeSteps.js'
import clips from './welcomeClips.json'

const pair = (id) => ({
  id,
  title: { ru: 'П', en: 'P' },
  before: `${id}-before.mp3`,
  after: `${id}-after.mp3`,
  before_label: { ru: 'было', en: 'before' },
  after_label: { ru: 'стало', en: 'after' },
  dur: 10,
})
const step = (id, n) => ({
  id,
  title: { ru: id, en: id },
  pairs: Array.from({ length: n }, (_, i) => pair(`${id}${i}`)),
})
const ids = (steps) => steps.map((s) => s.id)

describe('visibleSteps', () => {
  it('«Что это» первым, «Видеокарты» последним, между ними шаги с парами', () => {
    const a = step('form', 2)
    const b = step('voice', 1)
    expect(ids(visibleSteps({ steps: [a, b] }))).toEqual(['about', 'form', 'voice', 'gpu'])
  })

  it('шаг примеров без пар не показывается', () => {
    const a = step('form', 1)
    const empty = step('presets', 0)
    const c = step('voice', 3)
    const out = visibleSteps({ steps: [a, empty, c] })
    expect(ids(out)).toEqual(['about', 'form', 'voice', 'gpu'])
  })

  it('порядок шагов — как в данных', () => {
    const a = step('a', 1)
    const b = step('b', 1)
    const c = step('c', 1)
    expect(ids(visibleSteps({ steps: [c, a, b] }))).toEqual(['about', 'c', 'a', 'b', 'gpu'])
  })

  it('шаги примеров — сами объекты из данных (с парами)', () => {
    const a = step('form', 2)
    const out = visibleSteps({ steps: [a] })
    expect(out[1]).toBe(a)
    expect(out[1].pairs).toHaveLength(2)
  })

  it('нет шагов в данных → только «Что это» и «Видеокарты»', () => {
    expect(ids(visibleSteps({}))).toEqual(['about', 'gpu'])
    expect(ids(visibleSteps({ steps: [] }))).toEqual(['about', 'gpu'])
  })

  it('все шаги без пар → только «Что это» и «Видеокарты»', () => {
    expect(ids(visibleSteps({ steps: [step('x', 0), step('y', 0)] }))).toEqual(['about', 'gpu'])
  })

  it('на настоящих данных: первым «Что это», последним «Видеокарты», пустых шагов нет', () => {
    const out = visibleSteps(clips)
    expect(out[0].id).toBe('about')
    expect(out[out.length - 1].id).toBe('gpu')
    for (const s of out.slice(1, -1)) expect(s.pairs.length).toBeGreaterThan(0)
  })
})

describe('switchStart', () => {
  it('играет на 7,3 с → другая версия с 7,3 с', () => {
    expect(switchStart(true, 7.3, 12.02)).toBeCloseTo(7.3, 6)
  })

  it('играет с самого начала → с 0, а не «не играет»', () => {
    expect(switchStart(true, 0, 12.02)).toBe(0)
  })

  it('позиция больше длины клипа → с конца клипа, не дальше', () => {
    expect(switchStart(true, 15, 12.02)).toBeCloseTo(12.02, 6)
  })

  it('отрицательная позиция → с 0', () => {
    expect(switchStart(true, -1, 12.02)).toBe(0)
  })

  it('не играет → null (звука нет, только выбор версии)', () => {
    expect(switchStart(false, 7.3, 12.02)).toBeNull()
    expect(switchStart(false, 0, 12.02)).toBeNull()
  })

  it('длина клипа 0 или неизвестна → позиция как есть, но не меньше 0', () => {
    expect(switchStart(true, 7.3, 0)).toBeCloseTo(7.3, 6)
    expect(switchStart(true, 7.3, undefined)).toBeCloseTo(7.3, 6)
    expect(switchStart(true, -2, 0)).toBe(0)
  })
})

describe('welcomeClips.json', () => {
  const filled = (o) =>
    typeof o?.ru === 'string' && o.ru.trim() !== '' && typeof o?.en === 'string' && o.en.trim() !== ''

  it('есть список шагов', () => {
    expect(Array.isArray(clips.steps)).toBe(true)
  })

  it('у каждого шага заголовок ru и en, пары — массив', () => {
    for (const s of clips.steps) {
      expect(filled(s.title), `шаг ${s.id}: title ru/en`).toBe(true)
      expect(Array.isArray(s.pairs), `шаг ${s.id}: pairs`).toBe(true)
    }
  })

  it('у каждой пары заголовок и подписи «было»/«стало» ru и en', () => {
    for (const s of clips.steps) {
      for (const p of s.pairs) {
        expect(filled(p.title), `${s.id}/${p.id}: title`).toBe(true)
        expect(filled(p.before_label), `${s.id}/${p.id}: before_label`).toBe(true)
        expect(filled(p.after_label), `${s.id}/${p.id}: after_label`).toBe(true)
      }
    }
  })

  it('клипы пары — имена *.mp3 без каталога, длина > 0', () => {
    for (const s of clips.steps) {
      for (const p of s.pairs) {
        for (const f of [p.before, p.after]) {
          expect(typeof f, `${s.id}/${p.id}`).toBe('string')
          expect(f, `${s.id}/${p.id}`).toMatch(/^[^/\\]+\.mp3$/)
        }
        expect(p.dur, `${s.id}/${p.id}: dur`).toBeGreaterThan(0)
      }
    }
  })

  it('id пар уникальны', () => {
    const all = clips.steps.flatMap((s) => s.pairs.map((p) => p.id))
    expect(new Set(all).size).toBe(all.length)
  })
})
