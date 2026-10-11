// Тесты карточки internal-own-track, условие 124 «Одно „Готово“: понятный список версий» (тест-кейс ТК151):
//   - resultTitle(v, chainName) → {title, detail, kind} по таблице условия:
//       dsp-preset-N.flac        → «Пресет: <label>»                 (preset)
//       dsp-preset-N-mix.flac    → «Пресет: микс до мастера»         (premix)
//       overdub-inst-*.flac      → «Микс с правками · N правок»      (mix; N — частей label через « + »)
//       overdub-premaster-*.flac → «Микс до мастера»                 (premix)
//       dsp-fx-*.flac            → label или «Движок»                (engine)
//       dsp-preview-*.flac       → «<цепочка> (превью 15 с)»
//       dsp-<chain>.flac         → имя цепочки или label             (effect)
//       overdub-<id>.flac        → label или «Переиграно поверх оригинала» (layer; уточнение 124а, ТК154)
//     detail — полный label; пустой label — запасные имена (заголовок не пустой);
//   - resultsOrder(list) — новые сверху по created_at, без даты — в конце;
//   - resultDate(iso, now) — «10.10 14:00», другой год — с годом;
//   - по исходнику StudioPage.vue: строка «Δ крест» — под v-if флага «подробнее»
//     (одна галочка на список, по умолчанию выкл.).
// Написаны по карточке, без чтения реализации.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import { resultTitle, resultsOrder, resultDate } from './results.js'

// chainName — функция id цепочки → имя (нет — undefined); форма уточнена по JSDoc results.js: карточка её не задаёт
const NAMES = { wall: 'Стена звука' }
const byId = (id) => NAMES[id]
const none = () => undefined
const v = (file, label = '', extra = {}) => ({ file, label, created_at: '2026-10-10T14:00:10', metrics: null, ...extra })

describe('resultTitle: таблица условия 124', () => {
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

// уточнение 124а (ТК154): слой «поверх оригинала» от «Переиграть в другом стиле»
describe('resultTitle: overdub-<id>.flac — слой поверх оригинала (124а)', () => {
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

// ---------- по исходнику: дельты под «подробнее» ----------

const SRC = readFileSync(new URL('./components/StudioPage.vue', import.meta.url), 'utf8')
const script = (SRC.match(/<script[^>]*>([\s\S]*?)<\/script>/) || [])[1] || ''
const template = (SRC.match(/<template>([\s\S]*)<\/template>/) || [])[1] || ''

// открытые теги шаблона в позиции pos (от ближайшего к корню)
function openAt(tpl, pos) {
  const re = /<(\/?)([A-Za-z][\w-]*)((?:\s+[^\s=>/]+(?:\s*=\s*(?:"[^"]*"|'[^']*'))?)*)\s*(\/?)>/g
  const VOID = new Set(['input', 'br', 'img', 'hr', 'meta', 'link'])
  const stack = []
  let m
  while ((m = re.exec(tpl)) && m.index < pos) {
    if (m[1]) {
      const i = stack.map((s) => s.name).lastIndexOf(m[2])
      if (i >= 0) stack.length = i
    } else if (!m[4] && !VOID.has(m[2])) {
      stack.push({ name: m[2], attrs: m[3] })
    }
  }
  return stack.reverse()
}

const vIfOf = (attrs) => (attrs.match(/(?:^|\s)v-(?:if|show)\s*=\s*"([^"]*)"/) || [])[1] || null
const idents = (expr) => expr.match(/[A-Za-z_$][\w$]*/g) || []

describe('StudioPage.vue: «Δ крест» — под флагом «подробнее»', () => {
  const pos = template.indexOf('Δ крест')

  it('строка дельт есть в шаблоне', () => {
    expect(pos).toBeGreaterThan(-1)
  })

  // флаг «подробнее»: ref в скрипте, переключается галочкой (input checkbox с v-model на него)
  const flagNames = () => {
    const out = []
    const re = /(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*ref\(/g
    let m
    while ((m = re.exec(script))) out.push(m[1])
    return out.filter((name) => {
      const cb = new RegExp(`<input[^>]*type="checkbox"[^>]*v-model="${name}"|<input[^>]*v-model="${name}"[^>]*type="checkbox"`)
      return cb.test(template)
    })
  }

  it('предок строки «Δ крест» скрыт v-if/v-show по флагу-галочке', () => {
    const flags = flagNames()
    expect(flags.length).toBeGreaterThan(0)
    const guards = openAt(template, pos).map((tg) => vIfOf(tg.attrs)).filter(Boolean)
    const guarded = guards.some((expr) => idents(expr).some((id) => flags.includes(id)))
    expect(guarded, `условия предков: ${JSON.stringify(guards)}; флаги-галочки: ${flags}`).toBe(true)
  })

  // «по умолчанию выкл.»: исходник проверяет лишь грубо — флаг не заводится как ref(true) и начальное значение
  // может быть false (запомненный выбор из localStorage допустим); точное поведение — на ручной проверке
  it('флаг «подробнее» по умолчанию выключен (не ref(true), в инициализаторе есть false)', () => {
    const guards = openAt(template, pos).map((tg) => vIfOf(tg.attrs)).filter(Boolean)
    const flag = flagNames().find((f) => guards.some((expr) => idents(expr).includes(f)))
    expect(flag).toBeTruthy()
    const line = script.split('\n').find((l) => new RegExp(`(?:const|let)\\s+${flag}\\s*=\\s*ref\\(`).test(l))
    expect(line).not.toMatch(/ref\(\s*true\s*\)/)
    expect(line).toMatch(/\bfalse\b|=== ?['"](?:1|true)['"]/)
  })

  it('галочка «подробнее» — одна на список, а не своя у каждой версии (вне v-for версий)', () => {
    const flags = flagNames()
    const guards = openAt(template, pos).map((tg) => vIfOf(tg.attrs)).filter(Boolean)
    const flag = flags.find((f) => guards.some((expr) => idents(expr).includes(f)))
    expect(flag).toBeTruthy()
    const cbRe = new RegExp(`<input[^>]*v-model="${flag}"`, 'g')
    const hits = [...template.matchAll(cbRe)]
    expect(hits).toHaveLength(1)
    const parents = openAt(template, hits[0].index)
    const inVariantLoop = parents.some((tg) => /v-for\s*=\s*"[^"]*\bin\b/.test(tg.attrs) && /\bv\b/.test(tg.attrs.match(/v-for\s*=\s*"([^"]*)"/)[1].split(/\bin\b/)[0]))
    expect(inVariantLoop).toBe(false)
  })
})
