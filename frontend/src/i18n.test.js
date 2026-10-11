// Тесты словарей i18n: паритет ru/en, плейсхолдеры {name}, корректность вызовов t() и наличие
// в словарях каждого ключа, который исходники передают в t() литералом.
// Ловит класс бага «t('footer.gpu_queue', 3)» — число вместо объекта: {n} остаётся
// в строке, и пользователь видит сырой плейсхолдер.
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { expect, test } from 'vitest'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

// исходники приложения: frontend/src без словарей, тестов, node_modules и привязок Wails
const srcDir = dirname(fileURLToPath(import.meta.url))

const placeholders = (s) => [...String(s).matchAll(/\{(\w+)\}/g)].map((m) => m[1])

function walk(dir, out = []) {
  for (const name of readdirSync(dir)) {
    const p = join(dir, name)
    if (statSync(p).isDirectory()) {
      if (!['i18n', 'node_modules', 'wailsjs'].includes(name)) walk(p, out)
    } else if (/\.(vue|js)$/.test(name) && !name.endsWith('.test.js')) out.push(p)
  }
  return out
}

const SOURCES = walk(srcDir).map((f) => ({ file: f.replace(srcDir, ''), src: readFileSync(f, 'utf8') }))

// все вызовы t('key', …) по исходникам: ключ → аргументы (текстом)
function collectCalls() {
  const calls = []
  for (const { file: f, src } of SOURCES) {
    for (const m of src.matchAll(/\bt\(\s*'([^']+)'\s*(?:,\s*([^{)]+|\{[^)]*\))?)?\)/g)) {
      calls.push({ file: f, key: m[1], arg: (m[2] || '').trim() })
    }
  }
  return calls
}

test('ключи ru и en совпадают', () => {
  const ru = new Set(Object.keys(RU))
  const en = new Set(Object.keys(EN))
  expect([...ru].filter((k) => !en.has(k)), 'нет в en').toEqual([])
  expect([...en].filter((k) => !ru.has(k)), 'нет в ru').toEqual([])
})

test('плейсхолдеры {name} одинаковы в ru и en', () => {
  for (const [key, ru] of Object.entries(RU)) {
    if (!(key in EN)) continue
    expect(placeholders(EN[key]), key).toEqual(placeholders(ru))
  }
})

test('вызовы t() с плейсхолдерами передают объект с нужными именами', () => {
  const bad = []
  for (const c of collectCalls()) {
    const value = RU[c.key] ?? EN[c.key]
    if (value === undefined) continue // ключ-не-словарь (динамические 'x.' + key) не проверяем
    const need = placeholders(value)
    if (need.length === 0) continue
    if (!c.arg.startsWith('{')) {
      bad.push(`${c.file}: t('${c.key}', ${c.arg || '—'}) — в строке {${need}} нужен объект { ${need.map((n) => `${n}: …`).join(', ')} }`)
      continue
    }
    for (const n of need) {
      if (!new RegExp(`\\b${n}\\s*:`).test(c.arg)) {
        bad.push(`${c.file}: t('${c.key}') — не передан плейсхолдер ${n}`)
      }
    }
  }
  expect(bad, bad.join('\n')).toEqual([])
})

// Ключ литералом: t('a.b'), t("a.b") или t(`a.b`) без подстановок. Склейки вида t('studio.dsp.target.' + x)
// сюда не попадают (ключ известен только при работе) — их подписи проверяют тесты своих экранов (drumParts).
test('каждый ключ, переданный в t() литералом, есть в ru и в en', () => {
  const bad = []
  let n = 0
  for (const { file, src } of SOURCES) {
    for (const m of src.matchAll(/(?<![\w$.])t\(\s*(['"`])([\w.-]+)\1\s*[,)]/g)) {
      n++
      for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
        if (typeof dict[m[2]] !== 'string' || dict[m[2]].trim() === '') bad.push(`${file}: t('${m[2]}') — нет в ${lang}`)
      }
    }
  }
  expect(n, 'сверка не нашла ни одного вызова t() — сломан поиск').toBeGreaterThan(500)
  expect(bad, bad.join('\n')).toEqual([])
})
