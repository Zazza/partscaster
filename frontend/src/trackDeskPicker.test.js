// Тесты карточки internal-own-track, условие 122 «Пульт: „звук ▾“ — выбор инструмента» (тест-кейс ТК148),
// по исходнику components/TrackDesk.vue:
//   - чипов пресетов больше нет (нет кнопок small-btn с v-for по g.items);
//   - выбор «группа → инструмент» — два VSelect: группа и инструмент (второй searchable, options строит
//     itemOptions из presetGroups.js);
//   - редактор блоков ChainEditor — только по флагу «тонко настроить» (v-if), кнопка с t('desk.fine');
//     «изменить» из «Правок трека» (editRecord) открывает его сразу;
//   - раскрытие дорожки (toggleRow) не выбирает первый пресет;
//   - цепочки без дорожек (stems пусто — мастер-склейка) в пульте не показываются;
//   - ключи desk.pick и desk.fine есть в ru и en.
// Написаны по карточке, без чтения реализации.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const SRC = readFileSync(new URL('./components/TrackDesk.vue', import.meta.url), 'utf8')
const script = (SRC.match(/<script[^>]*>([\s\S]*?)<\/script>/) || [])[1] || ''
const template = (SRC.match(/<template>([\s\S]*)<\/template>/) || [])[1] || ''

// открывающие теги шаблона: {name, attrs, start, selfClosing}; закрывающие — {close: name}
function tags(tpl) {
  const out = []
  const re = /<(\/?)([A-Za-z][\w-]*)((?:\s+[^\s=>/]+(?:\s*=\s*(?:"[^"]*"|'[^']*'))?)*)\s*(\/?)>/g
  let m
  while ((m = re.exec(tpl))) {
    if (m[1]) out.push({ close: m[2], start: m.index })
    else out.push({ name: m[2], attrs: m[3], start: m.index, selfClosing: !!m[4] })
  }
  return out
}

const VOID = new Set(['input', 'br', 'img', 'hr', 'meta', 'link'])

// цепочка предков тега (от ближайшего) — по стеку открытых тегов
function ancestors(tpl, pred) {
  const stack = []
  for (const tg of tags(tpl)) {
    if (tg.close) {
      const i = stack.map((s) => s.name).lastIndexOf(tg.close)
      if (i >= 0) stack.length = i
      continue
    }
    if (pred(tg)) return { tag: tg, parents: [...stack].reverse() }
    if (!tg.selfClosing && !VOID.has(tg.name)) stack.push(tg)
  }
  return null
}

const attr = (attrs, name) => {
  const m = attrs.match(new RegExp(`(?:^|\\s)${name.replace(/[:@]/g, (c) => '\\' + c)}\\s*=\\s*"([^"]*)"`))
  return m ? m[1] : null
}
const hasBoolAttr = (attrs, name) => new RegExp(`(?:^|\\s)${name}(?=\\s|$)`).test(attrs)

// тело функции function name(...) { ... } или const name = (...) => { ... } — по балансу скобок
function body(src, name) {
  const re = new RegExp(`(?:function\\s+${name}\\s*\\(|(?:const|let)\\s+${name}\\s*=\\s*(?:async\\s*)?\\([^)]*\\)\\s*=>\\s*)`)
  const m = re.exec(src)
  if (!m) return null
  const open = src.indexOf('{', m.index + m[0].length - 1)
  let depth = 0
  for (let i = open; i < src.length; i++) {
    if (src[i] === '{') depth++
    else if (src[i] === '}' && --depth === 0) return src.slice(open + 1, i)
  }
  return null
}

const selects = () => tags(template).filter((tg) => tg.name === 'VSelect')
const pickerSelects = () => selects().filter((tg) => !/kitOptions|treatOptions/.test(attr(tg.attrs, ':options') || ''))
// VSelect инструмента: options — вызов itemOptions(...) прямо в шаблоне или переменная скрипта,
// которая строится через itemOptions(...)
function isItemSelect(tg) {
  const expr = attr(tg.attrs, ':options') || ''
  if (/\bitemOptions\s*\(/.test(expr)) return true
  const name = (expr.match(/^\s*([A-Za-z_]\w*)\s*$/) || [])[1]
  if (!name) return false
  const def = new RegExp(`(?:const|let)\\s+${name}\\s*=[^\\n]*\\bitemOptions\\s*\\(`)
  return def.test(script)
}

describe('TrackDesk: выбор инструмента вместо чипов (ТК148)', () => {
  it('нет кнопок small-btn с v-for по g.items (чипы пресетов убраны)', () => {
    const chips = tags(template).filter((tg) => tg.name === 'button'
      && /\.items\b/.test(attr(tg.attrs, 'v-for') || '')
      && /small-btn/.test(attr(tg.attrs, 'class') || ''))
    expect(chips, chips.map((c) => c.attrs).join('\n')).toEqual([])
    expect(template).not.toMatch(/v-for="\s*\w+\s+in\s+g\.items\s*"/)
  })

  it('есть VSelect инструмента: searchable, options построены itemOptions из presetGroups', () => {
    const items = selects().filter(isItemSelect)
    expect(items.length, pickerSelects().map((p) => p.attrs).join('\n')).toBe(1)
    const a = items[0].attrs
    expect(hasBoolAttr(a, 'searchable') || attr(a, ':searchable') === 'true', a).toBe(true)
    expect(script).toMatch(/import\s*\{[^}]*\bitemOptions\b[^}]*\}\s*from\s*'[^']*presetGroups(\.js)?'/)
  })

  it('есть второй VSelect — группа (не инструмент и не набор/комната)', () => {
    const others = pickerSelects().filter((tg) => !isItemSelect(tg))
    expect(others.length, pickerSelects().map((p) => p.attrs).join('\n')).toBeGreaterThanOrEqual(1)
  })

  it('оба VSelect выбора — внутри раскрытой дорожки, группа идёт раньше инструмента', () => {
    const item = selects().find(isItemSelect)
    const group = pickerSelects().filter((tg) => !isItemSelect(tg)).filter((tg) => tg.start < item.start).pop()
    expect(group, 'VSelect группы перед VSelect инструмента').toBeTruthy()
    for (const s of [group, item]) {
      const found = ancestors(template, (tg) => tg.name === 'VSelect' && tg.start === s.start)
      const inRow = found.parents.some((p) => /open\s*===\s*row\.stem/.test(attr(p.attrs, 'v-if') || ''))
      expect(inRow, `VSelect ${attr(s.attrs, ':options')} вне раскрытой дорожки`).toBe(true)
    }
  })
})

describe('TrackDesk: редактор блоков — по «тонко настроить» (ТК148)', () => {
  // флаг — выражение v-if у ChainEditor или ближайшего предка внутри раскрытой дорожки
  function chainGuard() {
    const found = ancestors(template, (tg) => tg.name === 'ChainEditor')
    expect(found, 'ChainEditor в шаблоне').toBeTruthy()
    for (const tg of [found.tag, ...found.parents]) {
      const cond = attr(tg.attrs, 'v-if')
      if (cond && /^\s*open\s*===\s*row\.stem\s*$/.test(cond)) return null
      if (cond) return cond
    }
    return null
  }

  it('ChainEditor под v-if по отдельному флагу (не только «дорожка раскрыта»)', () => {
    const cond = chainGuard()
    expect(cond, 'у ChainEditor нет своего v-if').toBeTruthy()
    expect(cond).toMatch(/[A-Za-z_]\w*/)
  })

  it('кнопка «тонко настроить» — подпись t(\'desk.fine\') в шаблоне', () => {
    expect(template).toMatch(/t\(\s*'desk\.fine'\s*\)/)
  })

  it('«изменить» из «Правок» (editRecord) открывает редактор сразу: флаг = true', () => {
    const cond = chainGuard()
    const flag = (cond || '').match(/[A-Za-z_]\w*/)[0]
    const b = body(script, 'editRecord')
    expect(b, 'editRecord').toBeTruthy()
    expect(b).toMatch(new RegExp(`\\b${flag}\\.value\\s*=\\s*true\\b`))
  })

  it('toggleRow не выбирает первый пресет', () => {
    const b = body(script, 'toggleRow')
    expect(b, 'toggleRow').toBeTruthy()
    expect(b).not.toMatch(/pickPreset\s*\(/)
    expect(b).not.toMatch(/presetsFor\s*\([^)]*\)\s*\[\s*0\s*\]/)
  })
})

describe('TrackDesk: цепочки без дорожек не показываются (ТК148)', () => {
  it('есть фильтр по непустым stems у списка инструментов', () => {
    // формы: .filter((p) => (p.stems || []).length), .filter((p) => p.stems?.length), .filter((p) => p.stems && p.stems.length)
    expect(script).toMatch(/\.filter\(\s*\(?\s*(\w+)\s*\)?\s*=>[^\n]*\b\1\.stems\b[^\n]*\.length/)
  })
})

describe('i18n: desk.pick и desk.fine (ТК148)', () => {
  it.each(['desk.pick', 'desk.fine'])('%s есть в ru и en, непустой', (key) => {
    expect(typeof RU[key]).toBe('string')
    expect(typeof EN[key]).toBe('string')
    expect(RU[key].trim()).not.toBe('')
    expect(EN[key].trim()).not.toBe('')
  })

  it("t('desk.pick') используется в шаблоне", () => {
    expect(template).toMatch(/t\(\s*'desk\.pick'/)
  })
})
