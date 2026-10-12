// «Одно „Готово“: понятный список версий»:
//   - resultTitle(v, chainName) → {title, detail, kind} по таблице:
//       dsp-preset-N.flac        → «Пресет: <label>»                 (preset)
//       dsp-preset-N-mix.flac    → «Пресет: микс до мастера»         (premix)
//       overdub-inst-*.flac      → «Микс с правками · N правок»      (mix; N — частей label через « + »)
//       overdub-premaster-*.flac → «Микс до мастера»                 (premix)
//       dsp-fx-*.flac            → label или «Движок»                (engine)
//       dsp-preview-*.flac       → «<цепочка> (превью 15 с)»
//       dsp-<chain>.flac         → имя цепочки или label             (effect)
//       overdub-<id>.flac        → label или «Переиграно поверх оригинала» (layer)
//     detail — полный label; пустой label — запасные имена (заголовок не пустой);
//   - resultsOrder(list) — новые сверху по created_at, без даты — в конце;
//   - resultDate(iso, now) — «10.10 14:00», другой год — с годом;
//   - галочка «подробнее» запоминается, по умолчанию выкл. (readFlag/writeFlag из prefs.js); по дереву шаблона
//     StudioPage.vue строка «Δ крест» — под v-if флага этой галочки (одна галочка на список).
import { describe, it, expect } from 'vitest'
import { resultTitle, resultsOrder, resultDate } from './results.js'
import { readFlag, writeFlag } from './prefs.js'
import { templateAst, findElements, dir, attr, shownIf, idents } from './testkit/sfc.js'

// chainName — функция id цепочки → имя (нет — undefined)
const NAMES = { wall: 'Стена звука' }
const byId = (id) => NAMES[id]
const none = () => undefined
const v = (file, label = '', extra = {}) => ({ file, label, created_at: '2026-10-10T14:00:10', metrics: null, ...extra })

describe('resultTitle: таблица названий', () => {
  it('dsp-preset-N.flac → «Пресет: <label>», kind preset', () => {
    const r = resultTitle(v('dsp-preset-3.flac', 'тёплый ламповый'), none)
    expect(r.title).toBe('Пресет: тёплый ламповый')
    expect(r.kind).toBe('preset')
    expect(r.detail).toBe('тёплый ламповый')
  })

  it('dsp-preset-N-mix.flac → «Пресет: микс до мастера», kind premix (не путается с обычным пресетом)', () => {
    const r = resultTitle(v('dsp-preset-3-mix.flac', 'тёплый ламповый'), none)
    expect(r.title).toBe('Пресет: микс до мастера')
    expect(r.kind).toBe('premix')
    expect(r.detail).toBe('тёплый ламповый')
  })

  it('overdub-inst-*.flac с label «a + b + c» → «Микс с правками · 3 правки», kind mix', () => {
    const label = 'бас: тёплый + барабаны: щётки + гитара: фузз'
    const r = resultTitle(v('overdub-inst-7.flac', label), none)
    expect(r.title).toBe('Микс с правками · 3 правки')
    expect(r.title).toMatch(/\b3\b/)
    expect(r.title).toContain('прав')
    expect(r.kind).toBe('mix')
    expect(r.detail).toBe(label) // полный label — в подсказку
  })

  it.each([
    ['a', '1 правка'],
    ['a + b', '2 правки'],
    ['a + b + c + d', '4 правки'],
    ['a + b + c + d + e', '5 правок'],
    ['a + b + c + d + e + f + g + h + i + j + k', '11 правок'],
  ])('склонение: label «%s» → «Микс с правками · %s»', (label, tail) => {
    expect(resultTitle(v('overdub-inst-0.flac', label), none).title).toBe(`Микс с правками · ${tail}`)
  })

  it('overdub-premaster-*.flac → «Микс до мастера», kind premix', () => {
    const r = resultTitle(v('overdub-premaster-2.flac', 'бас + барабаны'), none)
    expect(r.title).toBe('Микс до мастера')
    expect(r.kind).toBe('premix')
    expect(r.detail).toBe('бас + барабаны')
  })

  it('dsp-fx-*.flac с label → label, kind engine', () => {
    const r = resultTitle(v('dsp-fx-1712345.flac', 'гитара: NAM + кабинет'), none)
    expect(r.title).toBe('гитара: NAM + кабинет')
    expect(r.kind).toBe('engine')
    expect(r.detail).toBe('гитара: NAM + кабинет')
  })

  it('dsp-fx-*.flac без label → «Движок»', () => {
    const r = resultTitle(v('dsp-fx-1712345.flac', ''), none)
    expect(r.title).toBe('Движок')
    expect(r.kind).toBe('engine')
  })

  it('dsp-preview-<chain>.flac → «<цепочка> (превью 15 с)»', () => {
    const r = resultTitle(v('dsp-preview-wall.flac', ''), byId)
    expect(r.title).toBe('Стена звука (превью 15 с)')
  })

  it('dsp-<chain>.flac → имя цепочки, kind effect', () => {
    const r = resultTitle(v('dsp-wall.flac', ''), byId)
    expect(r.title).toBe('Стена звука')
    expect(r.kind).toBe('effect')
  })

  it('dsp-<chain>.flac: есть и имя цепочки, и label — имя цепочки, label в подсказке', () => {
    const r = resultTitle(v('dsp-wall.flac', 'стена из MCP'), byId)
    expect(r.title).toBe('Стена звука')
    expect(r.detail).toBe('стена из MCP')
  })

  it('dsp-<chain>.flac без имени цепочки → label, kind effect', () => {
    const r = resultTitle(v('dsp-wall.flac', 'стена из MCP'), none)
    expect(r.title).toBe('стена из MCP')
    expect(r.kind).toBe('effect')
    expect(r.detail).toBe('стена из MCP')
  })

  describe('пустой label — запасные имена, заголовок не пустой и не обрывается', () => {
    it.each([
      'dsp-preset-3.flac', 'dsp-preset-3-mix.flac', 'overdub-inst-4.flac', 'overdub-premaster-4.flac',
      'dsp-fx-9.flac', 'dsp-preview-zzz.flac', 'dsp-zzz.flac',
    ])('%s', (file) => {
      const r = resultTitle(v(file, ''), none)
      expect(typeof r.title).toBe('string')
      expect(r.title.trim()).not.toBe('')
      expect(r.title).not.toMatch(/[:·]\s*$/)      // нет «Пресет: » / «… · » с пустым хвостом
      expect(r.title).not.toMatch(/undefined|null/)
      expect(r.title).not.toMatch(/\b0 правок/)    // пустой label — не «0 правок»
    })

    it('пустой label пресета — заголовок всё ещё про пресет, kind preset', () => {
      const r = resultTitle(v('dsp-preset-3.flac', ''), none)
      expect(r.title).toMatch(/^Пресет/)
      expect(r.kind).toBe('preset')
    })

    it('пустой label микса — kind mix', () => {
      expect(resultTitle(v('overdub-inst-4.flac', ''), none).kind).toBe('mix')
    })
  })
})

// слой «поверх оригинала» от «Переиграть в другом стиле»
describe('resultTitle: overdub-<id>.flac — слой поверх оригинала', () => {
  it('с label → label, kind layer, detail — label', () => {
    const r = resultTitle(v('overdub-12.flac', 'jazz piano, brushes'), none)
    expect(r.title).toBe('jazz piano, brushes')
    expect(r.kind).toBe('layer')
    expect(r.detail).toBe('jazz piano, brushes')
  })

  it('без label → «Переиграно поверх оригинала», kind layer', () => {
    const r = resultTitle(v('overdub-12.flac', ''), none)
    expect(r.title).toBe('Переиграно поверх оригинала')
    expect(r.kind).toBe('layer')
  })

  it('не путается с overdub-inst-* и overdub-premaster-*', () => {
    expect(resultTitle(v('overdub-inst-12.flac', 'a + b'), none).kind).toBe('mix')
    expect(resultTitle(v('overdub-premaster-12.flac', 'a'), none).kind).toBe('premix')
  })
})

describe('resultsOrder: новые сверху, без даты — в конце', () => {
  it('сортирует по created_at по убыванию', () => {
    const list = [
      v('dsp-a.flac', '', { created_at: '2026-10-09T10:00:00' }),
      v('dsp-b.flac', '', { created_at: '2026-10-11T08:00:00' }),
      v('dsp-c.flac', '', { created_at: '2026-10-10T23:59:59' }),
    ]
    expect(resultsOrder(list).map((x) => x.file)).toEqual(['dsp-b.flac', 'dsp-c.flac', 'dsp-a.flac'])
  })

  it('версии без даты — в конце списка', () => {
    const list = [
      v('dsp-nodate1.flac', '', { created_at: '' }),
      v('dsp-old.flac', '', { created_at: '2026-01-01T00:00:00' }),
      v('dsp-nodate2.flac', '', { created_at: undefined }),
      v('dsp-new.flac', '', { created_at: '2026-10-10T14:00:10' }),
      v('dsp-nodate3.flac', '', { created_at: null }),
    ]
    const out = resultsOrder(list).map((x) => x.file)
    expect(out.slice(0, 2)).toEqual(['dsp-new.flac', 'dsp-old.flac'])
    expect(out.slice(2).sort()).toEqual(['dsp-nodate1.flac', 'dsp-nodate2.flac', 'dsp-nodate3.flac'])
  })

  it('время сравнивается внутри одного дня (не только дата)', () => {
    const list = [
      v('dsp-morning.flac', '', { created_at: '2026-10-10T09:05:00' }),
      v('dsp-evening.flac', '', { created_at: '2026-10-10T21:30:00' }),
    ]
    expect(resultsOrder(list).map((x) => x.file)).toEqual(['dsp-evening.flac', 'dsp-morning.flac'])
  })

  it('пустой список → пустой список; все версии на месте', () => {
    expect(resultsOrder([])).toEqual([])
    const list = [v('dsp-a.flac'), v('dsp-b.flac', '', { created_at: '2026-10-11T00:00:00' })]
    expect(resultsOrder(list)).toHaveLength(2)
  })
})

describe('resultDate: «10.10 14:00»', () => {
  it('тот же год — день.месяц часы:минуты без года и секунд', () => {
    expect(resultDate('2026-10-10T14:00:10', new Date('2026-10-11'))).toBe('10.10 14:00')
  })

  it('ведущие нули у дня, месяца, часа и минут', () => {
    expect(resultDate('2026-03-05T09:07:00', new Date('2026-10-11'))).toBe('05.03 09:07')
  })

  it('другой год — с годом', () => {
    const s = resultDate('2025-10-10T14:00:10', new Date('2026-10-11'))
    expect(s).toContain('2025')
    expect(s).toContain('10.10')
    expect(s).toContain('14:00')
    expect(s).not.toBe('10.10 14:00')
  })
})

// «подробнее»: дельты метрик под одной галочкой, по умолчанию выкл.

class MemStorage {
  constructor(init = {}) { this.m = new Map(Object.entries(init)) }
  getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
  setItem(k, v) { this.m.set(k, String(v)) }
}

describe('readFlag / writeFlag — запомненная галочка «подробнее» (prefs.js)', () => {
  it('по умолчанию выключена: записи нет → def false', () => {
    expect(readFlag('yue_done_deltas', false, new MemStorage())).toBe(false)
  })

  it('запомненный выбор переживает перезапуск: запись → чтение', () => {
    const st = new MemStorage()
    writeFlag('yue_done_deltas', true, st)
    expect(readFlag('yue_done_deltas', false, st)).toBe(true)
    writeFlag('yue_done_deltas', false, st)
    expect(readFlag('yue_done_deltas', true, st)).toBe(false)
  })

  it('чужое значение → def; хранилище бросает или его нет → def, запись молча пропускается', () => {
    expect(readFlag('k', false, new MemStorage({ k: 'yes' }))).toBe(false)
    const broken = { getItem() { throw new Error('denied') }, setItem() { throw new Error('denied') } }
    expect(readFlag('k', true, broken)).toBe(true)
    expect(() => writeFlag('k', true, broken)).not.toThrow()
    expect(readFlag('k', false, undefined)).toBe(false)
  })
})

// шаблон StudioPage по дереву разбора (vue/compiler-sfc), не регэкспом: дельты — только под флагом-галочкой
describe('StudioPage.vue: «Δ крест» — под галочкой «подробнее»', () => {
  const root = templateAst('components/StudioPage.vue')
  const deltas = findElements(root, (n) => (n.children || []).some((c) => c.type === 2 && c.content.includes('Δ крест')))
  const boxes = findElements(root, (n) => n.tag === 'input' && attr(n, 'type') === 'checkbox' && dir(n, 'model') !== null)

  it('строка дельт есть и каждый её показ зависит от флага, который переключает галочка', () => {
    expect(deltas.length).toBeGreaterThan(0)
    for (const { ancestors } of deltas) {
      const guards = ancestors.map(shownIf).filter(Boolean)
      const flag = boxes.map((b) => dir(b.node, 'model')).find((m) => guards.some((g) => idents(g).includes(m)))
      expect(flag, `условия предков: ${JSON.stringify(guards)}`).toBeTruthy()
    }
  })

  it('галочка «подробнее» — одна на список версий, вне v-for', () => {
    const guards = deltas[0].ancestors.map(shownIf).filter(Boolean)
    const mine = boxes.filter((b) => guards.some((g) => idents(g).includes(dir(b.node, 'model'))))
    expect(mine).toHaveLength(1)
    expect(mine[0].ancestors.some((a) => dir(a, 'for') !== null), 'галочка внутри v-for').toBe(false)
  })
})
