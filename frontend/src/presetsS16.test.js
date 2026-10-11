// ТК156 (127а–128в, ревью s16) по исходникам редактора и библиотеки пресетов:
// своя занятость save/apply (action), закрытое окно не применяет пресет после waitReady,
// закрытие с правками — через подтверждение, «Сохранить как свой» не создаёт пресет на сервере
// до «Сохранить», «▶ на демо» — занятость на каждый пресет (Set).
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'

const srcDir = dirname(fileURLToPath(import.meta.url))
const read = (p) => readFileSync(p, 'utf8')
const LIBRARY = join(srcDir, 'components', 'PresetLibrary.vue')
const EDITOR = join(srcDir, 'components', 'PresetEditor.vue')

function templateOf(src) {
  const start = src.indexOf('<template')
  const end = src.lastIndexOf('</template>')
  return start < 0 || end < 0 ? '' : src.slice(start, end)
}
function scriptOf(src) {
  const m = src.match(/<script\b[^>]*>([\s\S]*?)<\/script>/g)
  return m ? m.join('\n') : ''
}
// тело функции name (function name(...) {...} или const name = (...) => {...}) по балансу скобок
function bodyOf(script, name) {
  const re = new RegExp(`(?:function\\s+${name}\\s*\\(|(?:const|let)\\s+${name}\\s*=\\s*(?:async\\s*)?(?:\\([^)]*\\)|\\w+)\\s*=>)`)
  const m = re.exec(script)
  if (!m) return ''
  let i = script.indexOf('{', m.index + m[0].length - 1)
  if (i < 0) return ''
  const start = i
  let depth = 0
  for (; i < script.length; i++) {
    if (script[i] === '{') depth++
    else if (script[i] === '}' && --depth === 0) return script.slice(start, i + 1)
  }
  return ''
}
// все <button ...>...</button> шаблона: { attrs, inner }
function buttons(tpl) {
  return [...tpl.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)].map((m) => ({ attrs: m[1], inner: m[2] }))
}
const clickHandler = (attrs) => {
  const m = attrs.match(/@click(?:\.[\w.]+)?="\s*([A-Za-z_$][\w$]*)/)
  return m ? m[1] : ''
}
const disabledExpr = (attrs) => {
  const m = attrs.match(/:disabled="([^"]*)"/)
  return m ? m[1] : ''
}

// --- 127а: своя занятость save/apply ---

test('PresetEditor: отдельный флаг action (ref) и защита от повтора во время запроса', () => {
  const script = scriptOf(read(EDITOR))
  expect(script, 'нет const action = ref(...)').toMatch(/const\s+action\s*=\s*ref\(/)
  expect(script, 'нет выхода при занятом action').toMatch(/if\s*\(\s*action\.value\s*\)\s*return/)
  expect(script).toMatch(/action\.value\s*=\s*(true|['"`\w])/)
})

test('PresetEditor: кнопки «Сохранить» и «Применить» отключены при action', () => {
  const src = read(EDITOR)
  const script = scriptOf(src)
  // :disabled ссылается на action напрямую или через computed, который его читает (canSave)
  const viaComputed = (expr) => (expr.match(/[A-Za-z_$][\w$]*/g) || []).some((id) => {
    const m = script.match(new RegExp(`const\\s+${id}\\s*=\\s*computed\\(([^\\n]*)`))
    return !!m && /\baction\.value\b/.test(m[1])
  })
  const withAction = buttons(templateOf(src)).filter((b) => {
    const e = disabledExpr(b.attrs)
    return /\baction\b/.test(e) || viaComputed(e)
  })
  expect(withAction.length, 'меньше двух кнопок с :disabled по action').toBeGreaterThanOrEqual(2)
  // эти кнопки ведут к сохранению и применению (обработчик сам или через вызов)
  const reach = (h) => {
    const body = bodyOf(script, h)
    return { save: /soundPreset(Create|Update)|\bsave\w*\(/i.test(h + body), apply: /appl/i.test(h + body) }
  }
  const r = withAction.map((b) => reach(clickHandler(b.attrs)))
  expect(r.some((x) => x.save), 'кнопка сохранения без action').toBe(true)
  expect(r.some((x) => x.apply), 'кнопка применения без action').toBe(true)
  // повтор во время запроса — без дубля: каждый обработчик выходит при занятом action
  for (const bt of withAction) {
    const h = clickHandler(bt.attrs)
    expect(bodyOf(script, h), `${h}: нет if (action.value) return`).toMatch(/if\s*\(\s*action\.value\s*\)\s*return/)
  }
})

// --- 128в: закрытое окно не применяет пресет к доготовившемуся демо ---

test('PresetEditor: applyToRef после waitReady проверяет closed и выходит', () => {
  const script = scriptOf(read(EDITOR))
  const body = bodyOf(script, 'applyToRef')
  expect(body, 'нет функции applyToRef').not.toBe('')
  const after = body.slice(body.indexOf('waitReady'))
  expect(body.indexOf('waitReady'), 'applyToRef не ждёт waitReady').toBeGreaterThan(-1)
  expect(after, 'после waitReady нет if (closed) return').toMatch(/if\s*\([^)]*\bclosed\b(\.value)?[^)]*\)\s*\{?\s*return/)
  // закрытие окна выставляет флаг
  expect(script, 'closed нигде не выставляется в true').toMatch(/\bclosed(\.value)?\s*=\s*true/)
})

// --- 127в: подтверждение закрытия с несохранёнными правками ---

test('PresetEditor: × и «Отмена» закрывают через подтверждение (useConfirm/askConfirm)', () => {
  const src = read(EDITOR)
  const script = scriptOf(src)
  expect(script).toMatch(/import\s+\{[^}]*\buseConfirm\b[^}]*\}\s+from\s+['"][^'"]*useConfirm(\.js)?['"]/)
  expect(script).toContain('askConfirm')
  const asks = (h) => !!h && (h === 'askConfirm' || bodyOf(script, h).includes('askConfirm'))
  const btns = buttons(templateOf(src))
  // × — символ или иконка закрытия (AppIcon name="x" с подсказкой common.close)
  const cross = btns.filter((b) => /×|&times;/.test(b.inner) || b.attrs.includes(`t('common.close')`))
  expect(cross.length, 'нет кнопки ×').toBeGreaterThan(0)
  for (const b of cross) expect(asks(clickHandler(b.attrs)), `× (${clickHandler(b.attrs)}) без подтверждения`).toBe(true)
  const cancel = btns.filter((b) => /cancel|Отмена/i.test(b.inner))
  expect(cancel.length, 'нет кнопки «Отмена»').toBeGreaterThan(0)
  for (const b of cancel) expect(asks(clickHandler(b.attrs)), `«Отмена» (${clickHandler(b.attrs)}) без подтверждения`).toBe(true)
})

// --- 127б: копия встроенного — в памяти, на сервере только по «Сохранить» ---

test('PresetLibrary: makeCopy не создаёт пресет на сервере', () => {
  const script = scriptOf(read(LIBRARY))
  const body = bodyOf(script, 'makeCopy')
  expect(body, 'нет функции makeCopy').not.toBe('')
  expect(body).not.toContain('soundPresetCreate')
  // создание на сервере осталось — по «Сохранить» в редакторе
  expect(scriptOf(read(EDITOR))).toContain('soundPresetCreate')
})

// --- 128а: «▶ на демо» — занятость на каждый пресет ---

test('PresetLibrary: demoBusy — Set, кнопка ▶ на демо отключается по has(p.id)', () => {
  const src = read(LIBRARY)
  const script = scriptOf(src)
  expect(script, 'demoBusy не Set').toMatch(/demoBusy\s*=\s*(?:ref|reactive|shallowRef)\(\s*new\s+Set\(/)
  const play = buttons(templateOf(src)).filter((b) => b.attrs.includes(`t('preset.lib.play.tip')`) || b.inner.includes('▶'))
  expect(play.length, 'нет кнопки ▶ на демо').toBeGreaterThan(0)
  expect(play.some((b) => /demoBusy(\.value)?\.has\(\s*p\.id\s*\)/.test(disabledExpr(b.attrs))),
    'кнопка ▶ не отключается по demoBusy.has(p.id)').toBe(true)
  // повтор того же пресета — без дубля: обработчик проверяет has до запуска
  expect(script).toMatch(/if\s*\([^)]*demoBusy(\.value)?\.has\(/)
})

// --- ТК157, кросс-ревью r1 (128ж, 128д, 128е, 128г) по исходнику редактора ---

// аргументы вызова fn( … ) по балансу круглых скобок, начиная с позиции from
function callArgs(script, fn, from = 0) {
  const i = script.indexOf(`${fn}(`, from)
  if (i < 0) return null
  let j = i + fn.length
  const start = j
  let depth = 0
  for (; j < script.length; j++) {
    if (script[j] === '(') depth++
    else if (script[j] === ')' && --depth === 0) return { at: i, text: script.slice(start + 1, j) }
  }
  return null
}
function allCalls(script, fn) {
  const out = []
  for (let c = callArgs(script, fn); c; c = callArgs(script, fn, c.at + fn.length)) out.push(c)
  return out
}
// имена функций, вызванных в тексте (одним уровнем), чтобы не спрятать ensure за обёрткой
const calledNames = (text) => [...text.matchAll(/(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(/g)].map((m) => m[1])

describe('кросс-ревью r1', () => {
  test('128ж: открытие редактора не создаёт демо — в onMounted и в immediate-watch нет demo.ensure(', () => {
    const script = scriptOf(read(EDITOR))
    const mounted = allCalls(script, 'onMounted').map((c) => c.text)
    const immediate = allCalls(script, 'watch').map((c) => c.text).filter((t) => /immediate\s*:\s*true/.test(t))
    for (const text of [...mounted, ...immediate]) {
      expect(text, 'demo.ensure( при открытии').not.toMatch(/\bensure\s*\(/)
      for (const name of new Set(calledNames(text))) {
        const body = bodyOf(script, name)
        expect(body, `${name}() из onMounted зовёт ensure(`).not.toMatch(/\bensure\s*\(/)
      }
    }
    // демо при этом создаётся — при прослушивании/применении (ensure или waitReady где-то есть)
    expect(script).toMatch(/\.(ensure|waitReady)\s*\(/)
  })

  test('128д: applyToRef фиксирует семью демо в локальную константу до первого await и дальше берёт её', () => {
    const body = bodyOf(scriptOf(read(EDITOR)), 'applyToRef')
    expect(body, 'нет функции applyToRef').not.toBe('')
    const m = body.match(/const\s+([A-Za-z_$][\w$]*)\s*=\s*refFamily\.value\b/)
    expect(m, 'нет const <имя> = refFamily.value').not.toBe(null)
    const firstAwait = body.search(/\bawait\b/)
    expect(firstAwait, 'в applyToRef нет await').toBeGreaterThan(-1)
    expect(m.index, 'семья берётся после первого await').toBeLessThan(firstAwait)
    const after = body.slice(firstAwait)
    expect(after, 'после первого await снова читается refFamily.value').not.toMatch(/\brefFamily\.value\b/)
    // ожидание готовности — по зафиксированной семье
    expect(after).toMatch(new RegExp(`waitReady\\(\\s*${m[1]}\\b`))
  })

  test('128е: превью мастера через presetMasterChain из presetEdit.js, свой ограничитель не собирается', () => {
    const script = scriptOf(read(EDITOR))
    expect(script).toMatch(/import\s+\{[^}]*\bpresetMasterChain\b[^}]*\}\s+from\s+['"][^'"]*presetEdit(\.js)?['"]/)
    const uses = allCalls(script, 'presetMasterChain')
    expect(uses.length, 'presetMasterChain не вызывается').toBeGreaterThan(0)
    expect(uses.some((c) => /target_lufs|targetLufs|lufs/i.test(c.text)), 'в вызов не передаётся цель громкости').toBe(true)
    // подстановка цели вручную — расхождение с применением
    expect(script, 'редактор сам собирает ограничитель').not.toMatch(/type\s*:\s*['"]limiter['"]/)
  })

  test('128г: смена своего эталона (watch ownJobId) сбрасывает дорожки ownStems', () => {
    const script = scriptOf(read(EDITOR))
    const w = allCalls(script, 'watch').filter((c) => /^\s*(\(\)\s*=>[^,]*)?\bownJobId\b/.test(c.text))
    expect(w.length, 'нет watch(ownJobId …)').toBeGreaterThan(0)
    expect(w.some((c) => /\bownStems\.value\s*=(?!=)/.test(c.text) || /\bownStems\.value\.clear\(\)/.test(c.text)),
      'watch(ownJobId) не сбрасывает ownStems').toBe(true)
  })
})

// ТК158 (128и, 128к): ответ ensureReference про сменённый эталон не играется — «стало», «было» и мастер
// запоминают выбранный эталон (refKey) до подготовки и сверяют после; превью мастера доступно при пустой
// цепочке, если задана цель громкости.
// значение выражения шаблона в окружении vars (неизвестные имена — undefined)
function evalTpl(expr, vars) {
  const scope = new Proxy(vars, { has: () => true, get: (o, k) => (k === Symbol.unscopables ? undefined : o[k]) })
  return new Function('scope', `with (scope) { return (${expr}) }`)(scope)
}
// условия ранних выходов `if (...) return` в тексте (скобки по балансу)
function earlyReturns(text) {
  const out = []
  for (const m of text.matchAll(/\bif\s*\(/g)) {
    let i = m.index + m[0].length, depth = 1
    for (; i < text.length && depth; i++) { if (text[i] === '(') depth++; else if (text[i] === ')') depth-- }
    if (/^\s*return\b/.test(text.slice(i))) out.push({ at: m.index, cond: text.slice(m.index + m[0].length, i - 1) })
  }
  return out
}

describe('кросс-ревью r2', () => {
  const PLAYERS = { 'стало': 'playAfter', 'было': 'playBefore', 'мастер': 'playMasterAfter' }
  for (const [label, fn] of Object.entries(PLAYERS)) {
    test(`128и: «${label}» (${fn}) запоминает refKey() до ensureReference и не играет, если эталон сменился`, () => {
      const body = bodyOf(scriptOf(read(EDITOR)), fn)
      expect(body, `нет функции ${fn}`).not.toBe('')
      const call = body.search(/\bensureReference\s*\(/)
      expect(call, `${fn} не готовит эталон через ensureReference`).toBeGreaterThan(-1)
      const saved = [...body.slice(0, call).matchAll(/const\s+([A-Za-z_$][\w$]*)\s*=\s*refKey\(\)/g)]
      expect(saved.length, `${fn}: refKey() не запомнен до ensureReference`).toBeGreaterThan(0)
      const name = saved[saved.length - 1][1]
      const cmp = new RegExp(`refKey\\(\\)\\s*!==?\\s*${name}\\b|\\b${name}\\s*!==?\\s*refKey\\(\\)`)
      const guard = earlyReturns(body).filter((r) => r.at > call && cmp.test(r.cond))
      expect(guard.length, `${fn}: после ensureReference нет if (refKey() !== ${name}) return`).toBeGreaterThan(0)
    })
  }

  test('128к: кнопка превью мастера доступна при пустой цепочке с целью громкости', () => {
    const btn = buttons(templateOf(read(EDITOR))).find((b) => clickHandler(b.attrs) === 'playMasterAfter')
    expect(btn, 'нет кнопки @click="playMasterAfter"').toBeTruthy()
    const expr = disabledExpr(btn.attrs)
    const env = (targetLufs) => ({ masterEdit: [], masterMissing: [], refBusy: false, model: { targetLufs } })
    if (expr) {
      expect(evalTpl(expr, env(-14)), `:disabled="${expr}" при пустой цепочке и цели -14`).toBeFalsy()
      expect(evalTpl(expr, env(-6)), 'цель -6').toBeFalsy()
    }
  })

  test('128к: playMasterAfter не выходит раньше времени при пустой цепочке, если цель задана', () => {
    const body = bodyOf(scriptOf(read(EDITOR)), 'playMasterAfter')
    expect(body, 'нет функции playMasterAfter').not.toBe('')
    const call = body.search(/\bensureReference\s*\(/)
    const early = earlyReturns(body).filter((r) => call < 0 || r.at < call)
    for (const r of early.filter((x) => /masterEdit/.test(x.cond))) {
      const vars = { masterEdit: { value: [] }, model: { value: { targetLufs: -14 } } }
      expect(evalTpl(r.cond, vars), `выход «${r.cond}» при пустой цепочке и цели -14`).toBeFalsy()
    }
  })
})
