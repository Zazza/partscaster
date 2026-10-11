// Тесты карточки internal-own-track, «Ревью s15 (круг 1) — исправления и уточнения условий» (ТК154),
// по исходнику компонентов (поведение внутри SFC без монтирования не наблюдаемо):
//   123а — готовый кусок (и автоплей по готовности) играет с from − 4 с; окно — из запомненного или cont_from
//          строки джобы: pollFragment передаёт в watchJob свою функцию проигрывания (→ playFragment),
//          playFragment берёт from или cont_from;
//   123б — кусок от начала (from < 1 с): обычный рендер api.submit с max_tokens = (to + 2) · 25 и role 'fragment'
//          (без cont_from), а не /continue;
//   123в — кусок получает приписку стиля приёмов: fragmentJob(..., trickStyleSuffix(...));
//   120а — PedalBoard читает сохранённую доску (load()) до watch([board, stem], save…), не внутри onMounted;
//   120б — трек без дорожек: selectTarget при пустом списке дорожек держит цель (присваивает обе цели
//          envTarget/dspTarget и выходит рано), активную дорожку не ставит.
// 124а (results) — в results.test.js; 123г, 125а — в тестах воркера и MCP.
// Написаны по карточке, без чтения реализации.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'

const read = (rel) => readFileSync(new URL(rel, import.meta.url), 'utf8')
const scriptOf = (sfc) => (sfc.match(/<script[^>]*>([\s\S]*?)<\/script>/) || [])[1] || ''

const STUDIO = scriptOf(read('./components/StudioPage.vue'))
const PEDAL = scriptOf(read('./components/PedalBoard.vue'))

// числовые константы скрипта (const NAME = 4) подставляются значениями: проверяется число из условия, не имя
function resolveConsts(text, src) {
  const consts = Object.fromEntries([...src.matchAll(/(?:const|let)\s+([A-Z_][A-Z0-9_]*)\s*=\s*(-?\d+(?:\.\d+)?)\b/g)].map((m) => [m[1], m[2]]))
  return text.replace(/\b[A-Z_][A-Z0-9_]*\b/g, (id) => consts[id] ?? id)
}

// от позиции открывающей скобки (любой из ( { [) — до парной
function balanced(text, openIdx) {
  const open = text[openIdx]
  const close = { '(': ')', '{': '}', '[': ']' }[open]
  let depth = 0
  for (let i = openIdx; i < text.length; i++) {
    if (text[i] === open) depth++
    else if (text[i] === close && --depth === 0) return text.slice(openIdx, i + 1)
  }
  return text.slice(openIdx)
}

// тело функции: function name(...) {...} или const name = (async) (...) => {...}
function body(src, name) {
  const re = new RegExp(`(?:(?:async\\s+)?function\\s+${name}\\s*\\(|(?:const|let)\\s+${name}\\s*=\\s*(?:async\\s*)?(?:\\([^)]*\\)|[A-Za-z_$][\\w$]*)\\s*=>\\s*)`)
  const m = re.exec(src)
  if (!m) return null
  // пропустить список параметров у function name(...)
  let from = m.index + m[0].length
  if (/\($/.test(m[0])) from = m.index + m[0].length - 1 + balanced(src, m.index + m[0].length - 1).length
  const open = src.indexOf('{', from)
  return open < 0 ? null : balanced(src, open)
}

// все вызовы fn(...) в тексте: [{index, args}]
function calls(text, fn) {
  const out = []
  const re = new RegExp(`\\b${fn.replace(/\./g, '\\.')}\\s*\\(`, 'g')
  let m
  while ((m = re.exec(text))) {
    const open = m.index + m[0].length - 1
    out.push({ index: m.index, args: balanced(text, open) })
  }
  return out
}

// блоки if (...) { ... } в тексте: [{cond, block}]
function ifBlocks(text) {
  const out = []
  const re = /\bif\s*\(/g
  let m
  while ((m = re.exec(text))) {
    const condOpen = m.index + m[0].length - 1
    const cond = balanced(text, condOpen)
    const rest = text.slice(condOpen + cond.length)
    const lead = rest.match(/^\s*/)[0].length
    const block = rest[lead] === '{' ? balanced(rest, lead) : rest.slice(lead).split('\n')[0]
    out.push({ cond, block })
  }
  return out
}

describe('123а: готовый кусок играет с from − 4 с', () => {
  it('pollFragment передаёт в watchJob свою функцию проигрывания → playFragment', () => {
    // pollFragment — функция, зовущая watchJob, или результат watchJob(...) (const pollFragment = watchJob(...))
    const direct = STUDIO.match(/(?:const|let)\s+pollFragment\s*=\s*(?=watchJob\s*\()/)
    const b = direct ? calls(STUDIO.slice(direct.index + direct[0].length), 'watchJob')[0].args.replace(/^/, 'watchJob')
      : body(STUDIO, 'pollFragment')
    expect(b, 'pollFragment').toBeTruthy()
    const w = calls(b, 'watchJob')
    expect(w.length, 'pollFragment не зовёт watchJob').toBeGreaterThan(0)
    const passes = w.some(({ args }) => /=>\s*\{?[^]*?\bplayFragment\s*\(/.test(args) || /,\s*playFragment\s*\)\s*$/.test(args))
    expect(passes, w.map((c) => c.args).join('\n')).toBe(true)
  })

  // поведение окна (from/cont_from, отступ 4 с) — в fragmentStart.test.js (поведенческий тест чистой функции);
  // проверки текста формулы здесь были ложно-зелёными (ревью s15 круг 2): здесь — только что playFragment её зовёт
  it('playFragment перематывает на fragmentStart(...) (окно и отступ — чистая функция)', () => {
    const b = body(STUDIO, 'playFragment')
    expect(b, 'playFragment').toBeTruthy()
    expect(b).toMatch(/\bfragmentStart\s*\(/)
    expect(b).toMatch(/\bseekAudio\s*\(/)
  })
})

describe('123б: кусок от начала (from < 1 с) — обычный рендер, а не /continue', () => {
  const submits = () => calls(STUDIO, 'api.submit').filter(({ args }) => /\bmax_tokens\b/.test(args))

  it('есть api.submit с max_tokens и role \'fragment\', без cont_from', () => {
    const s = submits()
    expect(s.length, 'нет api.submit с max_tokens').toBeGreaterThan(0)
    const frag = s.filter(({ args }) => /\brole\s*:\s*['"]fragment['"]/.test(args))
    expect(frag.length, s.map((c) => c.args).join('\n')).toBeGreaterThan(0)
    for (const { args } of frag) expect(args).not.toMatch(/\bcont_from\s*:/)
  })

  it('max_tokens = (to + 2) · 25', () => {
    const frag = submits().filter(({ args }) => /\brole\s*:\s*['"]fragment['"]/.test(args))
      .map((c) => ({ ...c, args: resolveConsts(c.args, STUDIO) }))
    const ok = frag.some(({ args }) => /max_tokens\s*:[^,\n]*\+\s*2\s*\)\s*\*\s*25\b|max_tokens\s*:[^,\n]*25\s*\*\s*\([^)]*\+\s*2\s*\)/.test(args))
    expect(ok, frag.map((c) => c.args).join('\n')).toBe(true)
  })

  it('ветка выбирается порогом 1 с по началу куска', () => {
    const frag = submits().find(({ args }) => /\brole\s*:\s*['"]fragment['"]/.test(args))
    expect(frag).toBeTruthy()
    // условие ветки — выше вызова в той же функции: сравнение с 1 (from < 1 или from >= 1)
    const before = resolveConsts(STUDIO.slice(Math.max(0, frag.index - 400), frag.index), STUDIO)
    expect(before).toMatch(/\bfrom\s*<\s*1\b|\bfrom\s*>=\s*1\b/)
  })
})

describe('123в: кусок получает приписку стиля приёмов', () => {
  it('fragmentJob вызывается с trickStyleSuffix (напрямую или через переменную)', () => {
    const fj = calls(STUDIO, 'fragmentJob')
    expect(fj.length, 'нет вызова fragmentJob').toBeGreaterThan(0)
    const vars = [...STUDIO.matchAll(/(?:const|let)\s+([A-Za-z_$][\w$]*)\s*=\s*[^\n]*\btrickStyleSuffix\s*\(/g)].map((m) => m[1])
    const ok = fj.some(({ args }) => /\btrickStyleSuffix\s*\(/.test(args) || vars.some((v) => new RegExp(`\\b${v}\\b`).test(args)))
    expect(ok, `${fj.map((c) => c.args).join('\n')}; переменные: ${vars}`).toBe(true)
  })
})

describe('120б: трек без дорожек — выбор «на что» держит цель', () => {
  it('selectTarget при пустом списке дорожек: обе цели присваиваются и ранний выход', () => {
    const b = body(STUDIO, 'selectTarget')
    expect(b, 'selectTarget').toBeTruthy()
    const early = ifBlocks(b).filter(({ cond, block }) => /\blength\b/.test(cond) && /\breturn\b/.test(block))
    expect(early.length, `нет раннего выхода по пустому списку дорожек:\n${b}`).toBeGreaterThan(0)
    const holds = early.some(({ block }) =>
      /envTarget\.value\s*=(?!=)\s*(?![\s'"`])/.test(block) && /dspTarget\.value\s*=(?!=)\s*(?![\s'"`])/.test(block))
    expect(holds, early.map((e) => e.cond + ' ' + e.block).join('\n')).toBe(true)
  })

  it('ранний выход не выбирает активную дорожку (stemForTarget не зовётся)', () => {
    const b = body(STUDIO, 'selectTarget')
    const early = ifBlocks(b).filter(({ cond, block }) => /\blength\b/.test(cond) && /\breturn\b/.test(block))
    for (const { block } of early) expect(block).not.toMatch(/\bstemForTarget\s*\(/)
  })
})

describe('120а: PedalBoard читает сохранённую доску до слежения за дорожкой', () => {
  const watchIdx = PEDAL.search(/\bwatch\s*\(\s*\[\s*board\s*,\s*stem\s*\]/)
  // вызовы load() — не определение function load(
  const loadCalls = calls(PEDAL, 'load').filter(({ index }) => !/function\s+$/.test(PEDAL.slice(Math.max(0, index - 12), index)))
  const mounted = calls(PEDAL, 'onMounted')

  it('есть watch([board, stem], …save…)', () => {
    expect(watchIdx).toBeGreaterThan(-1)
    const w = balanced(PEDAL, PEDAL.indexOf('(', watchIdx))
    expect(w).toMatch(/\bsave\b/)
  })

  it('load() вызывается до watch([board, stem]) и не внутри onMounted', () => {
    const inMounted = (i) => mounted.some(({ index, args }) => i > index && i < index + 'onMounted'.length + args.length + 2)
    const top = loadCalls.filter(({ index }) => !inMounted(index))
    expect(top.length, 'load() вызывается только внутри onMounted или не вызывается').toBeGreaterThan(0)
    expect(top.some(({ index }) => index < watchIdx), 'load() стоит после watch([board, stem])').toBe(true)
  })
})

// Кросс-ревью s15 r1 (ТК155): 120в, 121а, 123ж — по исходнику (SFC без монтирования не наблюдаемы).
// 123д, 123з — в worker/test_fragment_ctx.py; 123е — в internal/mcp/fragment_test.go.
describe('кросс-ревью r1', () => {
  const templateOf = (sfc) => (sfc.match(/<template>([\s\S]*)<\/template>/) || [])[1] || ''
  const stylesOf = (sfc) => [...sfc.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n')

  it('120в: PedalBoard в StudioPage получает :active="dspTarget" (цель правок, не targetFor(activeStem))', () => {
    const tpl = templateOf(read('./components/StudioPage.vue'))
    const tags = [...tpl.matchAll(/<PedalBoard\b[^>]*>/g)].map((m) => m[0])
    expect(tags.length, 'PedalBoard в шаблоне StudioPage не найден').toBeGreaterThan(0)
    for (const tag of tags) {
      const active = tag.match(/(?:\s:active|\sv-bind:active)\s*=\s*"([^"]*)"/)
      expect(active, tag).toBeTruthy()
      expect(active[1].trim(), tag).toBe('dspTarget')
    }
  })

  it('120в: дорожки педалей (STEMS в PedalBoard.vue) включают drums', () => {
    const m = PEDAL.match(/(?:const|let)\s+STEMS\s*=\s*/)
    expect(m, 'STEMS в PedalBoard.vue не найден').toBeTruthy()
    const open = PEDAL.slice(m.index + m[0].length).search(/[[{(]/) + m.index + m[0].length
    const list = balanced(PEDAL, open)
    expect(list).toMatch(/['"`]drums['"`]/)
  })

  it('123ж: renderFragment начинается с выхода при isImport — до любого запуска рендера', () => {
    const b = body(STUDIO, 'renderFragment')
    expect(b, 'renderFragment').toBeTruthy()
    const re = /\bif\s*\(/g
    let exitIdx = -1
    let m
    while ((m = re.exec(b))) {
      const cond = balanced(b, m.index + m[0].length - 1)
      const rest = b.slice(m.index + m[0].length - 1 + cond.length)
      const lead = rest.match(/^\s*/)[0].length
      const block = rest[lead] === '{' ? balanced(rest, lead) : rest.slice(lead).split('\n')[0]
      if (/\bisImport\b/.test(cond) && /\breturn\b/.test(block)) { exitIdx = m.index; break }
    }
    expect(exitIdx, `нет выхода по isImport в renderFragment:\n${b}`).toBeGreaterThan(-1)
    // запуск рендера: обычный рендер (api.submit — кусок с начала трека) и кусок продолжением (fragmentJob)
    const starts = [...calls(b, 'api.submit'), ...calls(b, 'fragmentJob'), ...calls(b, 'api.fragmentJob')]
    expect(starts.length, `в renderFragment не найден запуск рендера:\n${b}`).toBeGreaterThan(0)
    for (const { index, args } of starts) expect(index, args).toBeGreaterThan(exitIdx)
  })

  it('121а: у .roll-block в App.vue overflow-x: clip, а не auto (волна сверху прилипает к странице)', () => {
    const css = stylesOf(read('./App.vue'))
    const rules = [...css.matchAll(/([^{}]*)\{([^{}]*)\}/g)]
      .filter((r) => r[1].split(',').some((sel) => /\.roll-block\s*$/.test(sel.trim())))
    expect(rules.length, '.roll-block не найден в стилях App.vue').toBeGreaterThan(0)
    const decls = rules.map((r) => r[2]).join(';')
    expect(decls).toMatch(/\boverflow-x\s*:\s*clip\b/)
    expect(decls).not.toMatch(/\boverflow(?:-x)?\s*:\s*(?:auto|scroll)\b/)
  })
})
