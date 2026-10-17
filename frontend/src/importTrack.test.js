// internal-drop-corpus: импорт своего трека (с internal-window-frame, пачка 3 — пунктом меню «Импорт»
// в строке заголовка списка треков, рядом с «Рецепт трека»); страница «Свои треки» и профили исполнителей удалены со всех
// слоёв фронта; в примерочной голосов нет «в форму →». Проверка по исходникам, как в
// presetsPage.test.js: ловит «страницу удалили, а кнопку импорта не перенесли» и остатки профилей.
import { readFileSync, existsSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const srcDir = dirname(fileURLToPath(import.meta.url))
const read = (p) => readFileSync(p, 'utf8')

const APP = join(srcDir, 'App.vue')
const API = join(srcDir, 'api.js')
const VOICES = join(srcDir, 'components', 'VoicesPage.vue')
const CORPUS_PAGE = join(srcDir, 'components', 'CorpusPage.vue')
const BINDINGS_JS = join(srcDir, 'wailsjs', 'go', 'main', 'App.js')
const BINDINGS_DTS = join(srcDir, 'wailsjs', 'go', 'main', 'App.d.ts')

function templateOf(src) {
  const start = src.indexOf('<template')
  const end = src.lastIndexOf('</template>')
  return start < 0 || end < 0 ? '' : src.slice(start, end)
}

function scriptOf(src) {
  const m = src.match(/<script\b[^>]*>([\s\S]*?)<\/script>/g)
  return m ? m.join('\n') : ''
}

const clickOf = (attrs) => (/@click="([^"]*)"/.exec(attrs) || [])[1] || ''
const tKeys = (s) => [...s.matchAll(/\bt\(\s*['"]([\w.-]+)['"]/g)].map((m) => m[1])

// Тело функции скрипта по имени: от объявления до следующего объявления верхнего уровня.
function fnBody(script, name) {
  const decl = new RegExp(
    `(?:^|\\n)(?:async\\s+)?function\\s+${name}\\s*\\(|(?:^|\\n)const\\s+${name}\\s*=`)
  const m = decl.exec(script)
  if (!m) return ''
  const rest = script.slice(m.index + m[0].length)
  const next = /\n(?:async\s+function|function|const|let|var|watch|onMounted|\/\/)\b/.exec(rest)
  return next ? rest.slice(0, next.index) : rest
}

// Ведёт ли обработчик клика к api.importTrack: прямо в выражении либо через функцию скрипта
// (на одну ступень вложенности — функция может звать другую функцию App.vue).
function leadsToImportTrack(expr, script, depth = 0) {
  if (/\bapi\.importTrack\s*\(/.test(expr)) return true
  if (depth > 2) return false
  for (const [, name] of expr.matchAll(/\b([A-Za-z_$][\w$]*)\s*(?:\(|$)/g)) {
    const body = fnBody(script, name)
    if (body && leadsToImportTrack(body, script, depth + 1)) return true
  }
  return false
}

// Меню «Импорт» в строке заголовка списка треков: <details class="menu-pop down"> … </details>
function importMenu() {
  const src = read(APP)
  const tpl = templateOf(src)
  const m = /<details\b[^>]*class="[^"]*\bmenu-pop\b[^"]*\bdown\b[^"]*"[^>]*>([\s\S]*?)<\/details>/.exec(tpl)
  const block = m ? m[0] : ''
  const items = [...block.matchAll(/<li\b((?:[^>"']|"[^"]*"|'[^']*')*)>([\s\S]*?)<\/li>/g)]
    .map((x) => ({ attrs: x[1], body: x[2] }))
  const summary = (/<summary\b[\s\S]*?<\/summary>/.exec(block) || [''])[0]
  return { src, tpl, block, items, summary, script: scriptOf(src) }
}

describe('условие 1 (пачка 3, условие 16): импорт своего трека — пункт меню «Импорт» в заголовке списка треков', () => {
  test('меню «Импорт» стоит в строке заголовка страницы (слот actions), рядом — «Новый трек»', () => {
    const { tpl, block } = importMenu()
    expect(block, 'меню .menu-pop.down в App.vue').not.toBe('')
    const actions = (/<template #actions>([\s\S]*?)<\/template>/.exec(tpl.slice(tpl.indexOf("t('nav.tracks')"))) || [])[1] || ''
    expect(actions, 'меню внутри #actions страницы списка треков').toContain('menu-pop down')
    expect(actions, '«Новый трек» в той же строке').toContain("t('nav.new')")
  })

  test('в меню — «Рецепт трека» и пункт, который зовёт api.importTrack', () => {
    const { items, script } = importMenu()
    expect(items.some((it) => it.body.includes('recipe.importMenu.btn'))).toBe(true)
    expect(items.some((it) => leadsToImportTrack(clickOf(it.attrs), script)), 'пункт ведёт к api.importTrack').toBe(true)
  })

  test('подпись пункта — из i18n, ключ есть в ru и en, по-русски «Свой трек»', () => {
    const { items, script } = importMenu()
    const it = items.find((x) => leadsToImportTrack(clickOf(x.attrs), script))
    expect(it).toBeTruthy()
    const keys = tKeys(it.body + it.attrs)
    expect(keys.length, 'подпись через t(…)').toBeGreaterThan(0)
    for (const k of keys) {
      expect(RU[k], `ru: ${k}`).toBeTruthy()
      expect(EN[k], `en: ${k}`).toBeTruthy()
    }
    expect(keys.some((k) => /свой\s+трек/i.test(RU[k] || '')), 'ru-подпись «Свой трек»').toBe(true)
    expect(keys.some((k) => /own\s+track/i.test(EN[k] || '')), 'en-подпись «Your own track»').toBe(true)
    expect(keys.filter((k) => k.startsWith('corpus.'))).toEqual([])
  })

  test('пока идёт импорт — повторный импорт не стартует, на кнопке меню «импортирую…»', () => {
    const { items, summary, script } = importMenu()
    const it = items.find((x) => leadsToImportTrack(clickOf(x.attrs), script))
    expect(it).toBeTruthy()
    expect(clickOf(it.attrs), 'клик защищён флагом importBusy').toMatch(/!importBusy\s*&&/)
    const keys = tKeys(summary)
    expect(keys.some((k) => /импортирую/i.test(RU[k] || '')), 'ru: «импортирую…» на кнопке меню').toBe(true)
    expect(keys.every((k) => EN[k]), 'en-перевод каждого ключа кнопки').toBe(true)
  })

  test('ошибка транскрипции при импорте — текст из i18n (ru и en), а не зашитый в App.vue', () => {
    const src = read(APP)
    expect(scriptOf(src), 'русский текст ошибки зашит в App.vue').not.toMatch(/транскрипция не удалась/i)
    const errKeys = Object.keys(RU).filter((k) => /транскрипци\S* не удалась/i.test(RU[k]))
      .filter((k) => !k.startsWith('corpus.'))
    expect(errKeys.length, 'ключ i18n с текстом ошибки транскрипции (не corpus.*)').toBeGreaterThan(0)
    const used = errKeys.filter((k) => tKeys(src).includes(k))
    expect(used.length, 'App.vue показывает ошибку через t(…)').toBeGreaterThan(0)
    for (const k of used) expect(EN[k], `en: ${k}`).toBeTruthy()
  })
})

describe('условие 2: страница «Свои треки» удалена', () => {
  test('CorpusPage.vue нет', () => {
    expect(existsSync(CORPUS_PAGE)).toBe(false)
  })

  test("в App.vue нет секции 'corpus', CorpusPage и обработчиков профиля", () => {
    const src = read(APP)
    expect(src).not.toMatch(/CorpusPage/)
    expect(src).not.toMatch(/sectionFlag\(\s*['"]corpus['"]\s*\)/)
    expect(src).not.toMatch(/section\s*===\s*['"]corpus['"]/)
    expect(src).not.toMatch(/['"]corpus['"]/)
    expect(src).not.toMatch(/\bprofileStyleToLibrary\b/)
    expect(src).not.toMatch(/\bapplyProfileStyle\b/)
    expect(src).not.toMatch(/\bfromProfile\b/)
  })

  test('ключей corpus.* в словарях ru и en нет', () => {
    expect(Object.keys(RU).filter((k) => k.startsWith('corpus.'))).toEqual([])
    expect(Object.keys(EN).filter((k) => k.startsWith('corpus.'))).toEqual([])
  })

  test('текст пустого списка (common.empty) не ссылается на «свои треки» — ru и en', () => {
    expect(RU['common.empty']).toBeTruthy()
    expect(EN['common.empty']).toBeTruthy()
    expect(RU['common.empty']).not.toMatch(/свои\s+треки/i)
    expect(EN['common.empty']).not.toMatch(/my\s+tracks/i)
  })
})

describe('условие 3: профили удалены из слоя фронта', () => {
  test('в api.js нет corpus* (ни методов, ни импортов привязок YueCorpus*)', () => {
    const src = read(API)
    expect(src).not.toMatch(/corpus/i)
  })

  test('api.importTrack остаётся', () => {
    expect(read(API)).toMatch(/\bimportTrack\s*:/)
  })

  test('привязки Wails App.js / App.d.ts без YueCorpus*', () => {
    for (const f of [BINDINGS_JS, BINDINGS_DTS]) {
      if (existsSync(f)) expect(read(f), f).not.toMatch(/YueCorpus/)
    }
  })
})

describe('условие 5: в примерочной голосов нет «в форму →»', () => {
  test('VoicesPage не объявляет и не эмитит apply-voice', () => {
    const src = read(VOICES)
    expect(src).not.toMatch(/apply-voice/)
    expect(src).not.toMatch(/applyVoice/)
  })

  test('App.vue без applyVoice и без подписки на apply-voice', () => {
    const src = read(APP)
    expect(src).not.toMatch(/\bapplyVoice\b/)
    expect(src).not.toMatch(/@apply-voice/)
  })
})
