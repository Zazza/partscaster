// Окно «Что это» (WelcomeModal): подпись шага lead (internal-welcome-presets, условие 2, ТК2).
// Отрисовка настоящего компонента (renderHtml: setup + шаблон, серверный рендер), шаг выбирается
// через его же go(i) до отрисовки — так проверяется то, что увидит человек, а не устройство шаблона.
// Подменён только api.js (внешняя граница: HTTP к бэкенду).
import { describe, expect, test, vi, beforeEach } from 'vitest'
import { renderHtml } from './testkit/sfc.js'
import { visibleSteps } from './welcomeSteps.js'
import clips from './welcomeClips.json'

vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
vi.mock('./api.js', () => ({ api: { stopAudio: async () => {}, playAudio: async () => {}, playFile: async () => {} } }))
const { default: WelcomeModal } = await import('./components/WelcomeModal.vue')
const { useI18n } = await import('./i18n/index.js')

const steps = visibleSteps(clips)
const indexOf = (id) => steps.findIndex((s) => s.id === id)
// окно, открытое на шаге i: тот же компонент, перед отрисовкой — переход его собственной кнопкой-шагом go(i)
const onStep = (i) => ({
  ...WelcomeModal,
  setup(props, ctx) {
    const b = WelcomeModal.setup(props, ctx)
    b.go(i)
    return b
  },
})
const html = (id) => renderHtml(onStep(indexOf(id)))
// текст без тегов и с одиночными пробелами — подпись ищется как видимый текст
const text = (h) => h.replace(/<[^>]*>/g, ' ').replace(/&nbsp;/g, ' ').replace(/\s+/g, ' ')
const esc = (s) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;')
  .replace(/'/g, '&#39;')

const presets = clips.steps.find((s) => s.id === 'presets')
const leads = (lang) => clips.steps.filter((s) => s.lead?.[lang]).map((s) => s.lead[lang])

beforeEach(() => { useI18n().setLocale('ru') })

describe('WelcomeModal: подпись шага lead', () => {
  test('данные теста на месте: presets виден и с lead, form виден и без lead', () => {
    expect(indexOf('presets')).toBeGreaterThan(0)
    expect(presets.lead?.ru).toBeTruthy()
    expect(indexOf('form')).toBeGreaterThan(0)
    const form = clips.steps.find((s) => s.id === 'form')
    expect(form.lead?.ru || form.lead?.en || '').toBe('')
  })

  test('шаг presets (ru): подпись выводится текстом lead.ru, а не en', async () => {
    const h = await html('presets')
    expect(text(h)).toContain(text(esc(presets.lead.ru)))
    expect(text(h)).not.toContain(text(esc(presets.lead.en)))
  })

  test('шаг presets (en): подпись выводится текстом lead.en, а не ru', async () => {
    useI18n().setLocale('en')
    const h = await html('presets')
    expect(text(h)).toContain(text(esc(presets.lead.en)))
    expect(text(h)).not.toContain(text(esc(presets.lead.ru)))
  })

  test('подпись — под заголовком шага: текст lead идёт после заголовка presets', async () => {
    const t = text(await html('presets'))
    const title = text(esc(presets.title.ru))
    const lead = text(esc(presets.lead.ru))
    expect(t.indexOf(title)).toBeGreaterThanOrEqual(0)
    expect(t.indexOf(lead)).toBeGreaterThan(t.indexOf(title))
  })

  test('шаг form без lead: ни одной подписи lead нет, окно отрисовано (заголовок и пары на месте)', async () => {
    for (const lang of ['ru', 'en']) {
      useI18n().setLocale(lang)
      const h = await html('form')
      const t = text(h)
      for (const l of leads(lang)) expect(t, `${lang}: чужая подпись на form`).not.toContain(text(esc(l)))
      const form = steps[indexOf('form')]
      expect(t).toContain(text(esc(form.title[lang])))
      expect(t).toContain(text(esc(form.pairs[0].title[lang])))
    }
  })

  test('шаг без lead: на месте подписи нет пустого элемента — разметка form та же, что без поля lead', async () => {
    // окно с шагом form и с шагом form, у которого lead пустой: разметка не отличается
    const form = clips.steps.find((s) => s.id === 'form')
    const before = await html('form')
    const saved = form.lead
    form.lead = { ru: '', en: '' }
    try {
      expect(await html('form')).toBe(before)
    } finally {
      if (saved === undefined) delete form.lead
      else form.lead = saved
    }
  })
})

// internal-welcome-polish, условие 4, ТК1–ТК4: навигация «Назад / Далее / Понятно», точки шагов, вкладка «Что нового».
// Что видит человек — серверная отрисовка настоящего компонента (renderHtml). «Клик» — обработчик @click той
// кнопки шаблона, на которой написан текст из i18n (welcome.prev / welcome.next / welcome.ok / welcome.tab.news,
// у точки — подпись welcome.step), исполненный над привязками setup (как его исполнил бы Vue). Имена функций
// компонента тест не знает: кнопка ищется по подписи, обработчик берётся из шаблона.
// emit close(dontShow) — через mountSetup (testkit): вызовы emit записываются в emitted.
import { templateAst, findElements, dir, textOf, mountSetup } from './testkit/sfc.js'
import { isRef } from 'vue'

const ast = templateAst('components/WelcomeModal.vue')
const keyRe = (key) => new RegExp(`['"]${key.replace(/\./g, '\\.')}['"]`)
// подпись элемента: текст с потомками + все привязанные атрибуты (title, aria-label)
const labelOf = (n) => textOf(n) + ' ' + (n.props || []).map((p) => (p.exp ? p.exp.content : '')).join(' ')

// область видимости выражений шаблона: привязки setup (ref разворачиваются), props, $emit, переменные v-for
function scopeOf(b, props, emit, extra = {}) {
  const own = { ...props, ...b, $emit: emit, $props: props, ...extra }
  return new Proxy(own, {
    has: (o, k) => k in o,
    get: (o, k) => (isRef(o[k]) ? o[k].value : o[k]),
    set: (o, k, v) => { if (isRef(o[k])) o[k].value = v; else o[k] = v; return true },
  })
}
const evalIn = (scope, expr) => new Function('__s', `with (__s) { return (${expr}) }`)(scope)
const runIn = (scope, stmt, ev) => new Function('__s', '$event', `with (__s) { ${stmt} }`)(scope, ev)

// условие показа узла с учётом цепочки v-if / v-else-if / v-else среди соседей
function condOk(node, parent, scope) {
  const sh = dir(node, 'show')
  if (sh !== null && !evalIn(scope, sh)) return false
  const vif = dir(node, 'if')
  if (vif !== null) return !!evalIn(scope, vif)
  const elif = dir(node, 'else-if')
  const isElse = dir(node, 'else') !== null
  if (elif === null && !isElse) return true
  const sibs = (parent?.children || []).filter((c) => c.type === 1)
  for (let i = sibs.indexOf(node) - 1; i >= 0; i--) {
    const c = dir(sibs[i], 'if') ?? dir(sibs[i], 'else-if')
    if (c === null) break
    if (evalIn(scope, c)) return false
    if (dir(sibs[i], 'if') !== null) break
  }
  return elif === null ? true : !!evalIn(scope, elif)
}
const shown = ({ node, ancestors }, scope) =>
  [...ancestors, node].every((n, i, all) => condOk(n, i ? all[i - 1] : ast, scope))

// кнопка с подписью key, видимая сейчас; клик — её @click
function clickLabel(b, props, emit, key) {
  const scope = scopeOf(b, props, emit)
  const hits = findElements(ast, (n) => dir(n, 'on', 'click') !== null && keyRe(key).test(labelOf(n)))
    .filter((h) => !h.ancestors.some((a) => dir(a, 'for') !== null))
    .filter((h) => shown(h, scope))
  expect(hits.length, `видимая кнопка «${key}»`).toBe(1)
  fire(scope, dir(hits[0].node, 'on', 'click'))
}
function fire(scope, h) {
  const ev = { target: { checked: true }, stopPropagation() {}, preventDefault() {} }
  if (/^[\w$.]+$/.test(h.trim())) evalIn(scope, h)(ev)
  else runIn(scope, h, ev)
}
// клик по i-й точке шагов: элемент с подписью welcome.step внутри v-for
function clickDot(b, props, emit, i) {
  const hits = findElements(ast, (n) => keyRe('welcome.step').test(labelOf(n)) && dir(n, 'on', 'click') !== null)
  expect(hits.length, 'точка шагов (подпись welcome.step, клик)').toBe(1)
  const { node, ancestors } = hits[0]
  const forNode = [...ancestors, node].reverse().find((n) => dir(n, 'for') !== null)
  expect(forNode, 'точки — список v-for').toBeTruthy()
  const m = dir(forNode, 'for').match(/^\s*\(?\s*([\w$]+)\s*(?:,\s*([\w$]+))?\s*\)?\s+(?:in|of)\s+(.+)$/)
  const list = evalIn(scopeOf(b, props, emit), m[3])
  const items = typeof list === 'number' ? Array.from({ length: list }, (_, k) => k + 1) : Array.from(list)
  const extra = { [m[1]]: items[i] }
  if (m[2]) extra[m[2]] = i
  fire(scopeOf(b, props, emit, extra), dir(node, 'on', 'click'))
}
// окно после действий act(b, props, emit) — то, что увидит человек
const after = (props, act) => renderHtml({
  ...WelcomeModal,
  setup(p, ctx) {
    const b = WelcomeModal.setup(p, ctx)
    act(b, p, ctx.emit)
    return b
  },
}, props)

const t = (k, v) => useI18n().t(k, v)
// видимые кнопки: тексты <button>, кроме скрытых v-show (display:none)
const buttons = (h) => [...h.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)]
  .filter((m) => !/display:\s*none/.test(m[1]))
  .map((m) => text(m[2]).trim())
const hasBtn = (h, key) => buttons(h).includes(text(esc(t(key))).trim())
// подписи точек «Шаг n», видимые в разметке
const dotCount = (h) => steps.filter((_, i) => h.includes(esc(t('welcome.step', { n: i + 1 })))).length
const ABOUT = () => text(esc(t('welcome.about.goal')))
const titleOf = (i) => {
  const s = steps[i]
  if (s.id === 'about') return ABOUT()
  if (s.id === 'gpu') return text(esc(t('welcome.gpu.title')))
  return text(esc(s.title.ru))
}
const onlyStep = (h, i) => {
  const tx = text(h)
  expect(tx, `видна страница шага ${i + 1} (${steps[i].id})`).toContain(titleOf(i))
  for (let j = 0; j < steps.length; j++) {
    if (j !== i && steps[j].id !== 'about' && steps[j].id !== 'gpu') {
      expect(tx, `чужой шаг ${steps[j].id} на шаге ${i + 1}`).not.toContain(titleOf(j))
    }
  }
}
const ABOUT_TAB = { tab: 'about' }
const last = steps.length - 1
const mid = indexOf('fills')

describe('WelcomeModal: навигация «Назад / Далее» (ТК1)', () => {
  test('данные теста: шагов не меньше 4, fills — средний шаг', () => {
    expect(steps.length).toBeGreaterThanOrEqual(4)
    expect(mid).toBeGreaterThan(0)
    expect(mid).toBeLessThan(last)
  })

  test('шаг 1: «Назад» нет, «Далее» есть, «Понятно» нет', async () => {
    const h = await after(ABOUT_TAB, () => {})
    expect(text(h)).toContain(ABOUT())
    expect(hasBtn(h, 'welcome.prev')).toBe(false)
    expect(hasBtn(h, 'welcome.next')).toBe(true)
    expect(hasBtn(h, 'welcome.ok')).toBe(false)
  })

  test('шаг 1: клик «Далее» → шаг 2', async () => {
    const h = await after(ABOUT_TAB, (b, p, e) => clickLabel(b, p, e, 'welcome.next'))
    onlyStep(h, 1)
    expect(text(h)).not.toContain(ABOUT())
  })

  test('средний шаг: есть «Назад» и «Далее»', async () => {
    const h = await after(ABOUT_TAB, (b) => b.go(mid))
    onlyStep(h, mid)
    expect(hasBtn(h, 'welcome.prev')).toBe(true)
    expect(hasBtn(h, 'welcome.next')).toBe(true)
  })

  test('средний шаг: «Назад» → предыдущий шаг, «Далее» → следующий', async () => {
    onlyStep(await after(ABOUT_TAB, (b, p, e) => { b.go(mid); clickLabel(b, p, e, 'welcome.prev') }), mid - 1)
    onlyStep(await after(ABOUT_TAB, (b, p, e) => { b.go(mid); clickLabel(b, p, e, 'welcome.next') }), mid + 1)
  })

  test('«Далее» по всем шагам доводит до последнего, «Назад» — обратно до первого', async () => {
    const fwd = await after(ABOUT_TAB, (b, p, e) => { for (let i = 0; i < last; i++) clickLabel(b, p, e, 'welcome.next') })
    expect(text(fwd)).toContain(titleOf(last))
    const back = await after(ABOUT_TAB, (b, p, e) => {
      b.go(last)
      for (let i = 0; i < last; i++) clickLabel(b, p, e, 'welcome.prev')
    })
    expect(text(back)).toContain(ABOUT())
    expect(hasBtn(back, 'welcome.prev')).toBe(false)
  })
})

describe('WelcomeModal: последний шаг — «Понятно» (ТК2)', () => {
  test('последний шаг: «Далее» нет, «Понятно» есть, «Назад» есть', async () => {
    const h = await after(ABOUT_TAB, (b) => b.go(last))
    expect(text(h)).toContain(titleOf(last))
    expect(hasBtn(h, 'welcome.next')).toBe(false)
    expect(hasBtn(h, 'welcome.ok')).toBe(true)
    expect(hasBtn(h, 'welcome.prev')).toBe(true)
  })

  for (const dontShow of [true, false]) {
    test(`последний шаг: клик «Понятно» → emit close(${dontShow}) по галочке «Не показывать»`, async () => {
      const { b, props, emitted, unmount } = await mountSetup(WelcomeModal, { tab: 'about' })
      try {
        b.go(last)
        const box = findElements(ast, (n) => n.tag === 'input' && dir(n, 'model') !== null
          && (n.props || []).some((p) => p.name === 'type' && p.value?.content === 'checkbox'))
        expect(box.length, 'галочка «Не показывать» — одна').toBe(1)
        const scope = scopeOf(b, props, (...a) => emitted.push(a))
        runIn(scope, `${dir(box[0].node, 'model')} = ${dontShow}`)
        clickLabel(b, props, (...a) => emitted.push(a), 'welcome.ok')
        const closes = emitted.filter((x) => x[0] === 'close')
        expect(closes).toHaveLength(1)
        expect(closes[0][1]).toBe(dontShow)
      } finally { unmount() }
    })
  }
})

describe('WelcomeModal: вкладка «Что нового» (ТК3)', () => {
  const check = (h) => {
    expect(hasBtn(h, 'welcome.prev'), '«Назад»').toBe(false)
    expect(hasBtn(h, 'welcome.next'), '«Далее»').toBe(false)
    expect(hasBtn(h, 'welcome.ok'), '«Понятно»').toBe(true)
    expect(dotCount(h), 'точки шагов').toBe(0)
  }

  test('окно открыто на «Что нового»: точек и «Назад/Далее» нет, «Понятно» есть', async () => {
    check(await after({ tab: 'news' }, () => {}))
  })

  test('переход на «Что нового» кнопкой в шапке (со среднего шага): то же', async () => {
    check(await after(ABOUT_TAB, (b, p, e) => { b.go(mid); clickLabel(b, p, e, 'welcome.tab.news') }))
  })

  test('«Понятно» на «Что нового» закрывает окно: emit close', async () => {
    const { b, props, emitted, unmount } = await mountSetup(WelcomeModal, { tab: 'news' })
    try {
      clickLabel(b, props, (...a) => emitted.push(a), 'welcome.ok')
      expect(emitted.filter((x) => x[0] === 'close')).toHaveLength(1)
    } finally { unmount() }
  })
})

describe('WelcomeModal: точки шагов (ТК4)', () => {
  test('число точек = числу видимых шагов (первый, средний, последний шаг)', async () => {
    for (const i of [0, mid, last]) {
      expect(dotCount(await after(ABOUT_TAB, (b) => b.go(i))), `шаг ${i + 1}`).toBe(steps.length)
    }
  })

  test('нет точки сверх числа шагов', async () => {
    const h = await after(ABOUT_TAB, () => {})
    expect(h.includes(esc(t('welcome.step', { n: steps.length + 1 })))).toBe(false)
  })

  test('клик по точке i → шаг i (каждая точка)', async () => {
    for (let i = 0; i < steps.length; i++) {
      const from = i === 0 ? last : 0
      onlyStep(await after(ABOUT_TAB, (b, p, e) => { b.go(from); clickDot(b, p, e, i) }), i)
    }
  })
})
