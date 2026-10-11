// Карточка internal-own-track, этап 15, условия 119–121, тест-кейс 147.
// 119: чистая логика активной дорожки — targetFor (цель правок по дорожке)
// и stemForTarget (какую дорожку сделать активной при ручном выборе цели).
// 120: одна активная дорожка в студии — по исходнику StudioPage/PedalBoard:
// сброс целей правок при «весь трек» (без раннего return), PedalBoard
// принимает активную дорожку и сообщает свой выбор.
// 121: галочка «волна сверху» — подпись в обоих словарях.
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, test } from 'vitest'
import { targetFor, stemForTarget } from './activeTrack.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const here = dirname(fileURLToPath(import.meta.url))
const src = (rel) => readFileSync(join(here, rel), 'utf8')

const DRUM_PARTS = ['kick', 'snare', 'toms', 'hh', 'ride', 'crash']
const MAIN_STEMS = ['vocals', 'drums', 'bass', 'other', 'guitar', 'piano']

describe('targetFor — цель правок дорожки (условие 119)', () => {
  test.each(DRUM_PARTS)('часть барабанов %s → drums', (part) => {
    expect(targetFor(part)).toBe('drums')
  })

  test.each(MAIN_STEMS)('основная дорожка %s → она сама', (stem) => {
    expect(targetFor(stem)).toBe(stem)
  })

  test('пустая дорожка (весь трек) → пустая цель', () => {
    expect(targetFor('')).toBe('')
  })

  test('неизвестная дорожка → пустая цель', () => {
    expect(targetFor('kazoo')).toBe('')
  })
})

describe('stemForTarget — активная дорожка по выбранной цели (условие 119)', () => {
  const stems = ['vocals', 'drums', 'bass', 'other', 'kick', 'snare']

  test('пустая цель → весь трек, даже при активной дорожке', () => {
    expect(stemForTarget('', stems, 'bass')).toBe('')
    expect(stemForTarget('', stems, 'kick')).toBe('')
  })

  test('текущая активная уже даёт цель: kick при drums остаётся kick', () => {
    expect(stemForTarget('drums', stems, 'kick')).toBe('kick')
    expect(stemForTarget('drums', stems, 'snare')).toBe('snare')
  })

  test('текущая активная совпадает с целью → она же', () => {
    expect(stemForTarget('bass', stems, 'bass')).toBe('bass')
  })

  test('текущая не даёт цель, есть дорожка с именем цели → она', () => {
    expect(stemForTarget('bass', stems, 'vocals')).toBe('bass')
    expect(stemForTarget('drums', stems, 'vocals')).toBe('drums')
    expect(stemForTarget('vocals', stems, '')).toBe('vocals')
  })

  test('нет дорожки с именем цели → весь трек (guitar без дорожки guitar)', () => {
    expect(stemForTarget('guitar', stems, '')).toBe('')
    expect(stemForTarget('guitar', stems, 'bass')).toBe('')
  })

  test('пустой список дорожек → весь трек', () => {
    expect(stemForTarget('drums', [], '')).toBe('')
  })
})

// Тело вызова fn(...) начиная с позиции открывающей скобки: до парной скобки.
function callBody(text, openIdx) {
  let depth = 0
  for (let i = openIdx; i < text.length; i++) {
    if (text[i] === '(') depth++
    else if (text[i] === ')') {
      depth--
      if (depth === 0) return text.slice(openIdx, i + 1)
    }
  }
  return text.slice(openIdx)
}

function watchBlocks(text) {
  const out = []
  const re = /\bwatch\s*\(/g
  let m
  while ((m = re.exec(text))) out.push(callBody(text, m.index + m[0].length - 1))
  return out
}

// Определение функции name: function name(...) {...} или const name = (...) => ...
function functionDef(text, name) {
  const re = new RegExp(`(?:function\\s+${name}\\s*\\(|(?:const|let)\\s+${name}\\s*=)`)
  const m = re.exec(text)
  if (!m) return ''
  const start = text.indexOf('{', m.index)
  if (start < 0) return ''
  let depth = 0
  for (let i = start; i < text.length; i++) {
    if (text[i] === '{') depth++
    else if (text[i] === '}') {
      depth--
      if (depth === 0) return text.slice(m.index, i + 1)
    }
  }
  return text.slice(m.index)
}

const RESET_ENV = /envTarget\.value\s*=\s*(?:''|""|targetFor\s*\()/
const RESET_DSP = /dspTarget\.value\s*=\s*(?:''|""|targetFor\s*\()/

// Тело watch ставит обе цели сам или через общую функцию, которая
// ставит обе цели из targetFor.
function setsBothTargets(body, text) {
  if (RESET_ENV.test(body) && RESET_DSP.test(body)) return true
  const calls = [...body.matchAll(/\b([A-Za-z_$][\w$]*)\s*\(/g)].map((c) => c[1])
  return calls.some((name) => {
    const def = functionDef(text, name)
    return (
      def &&
      /targetFor\s*\(/.test(def) &&
      /envTarget\.value\s*=/.test(def) &&
      /dspTarget\.value\s*=/.test(def)
    )
  })
}

describe('StudioPage — одна активная дорожка (условие 120)', () => {
  const studio = src('components/StudioPage.vue')
  // watch активной дорожки — тот, что трогает цели правок (напрямую или через функцию).
  const targetWatches = watchBlocks(studio).filter((b) => setsBothTargets(b, studio))

  test('есть watch, который ставит envTarget и dspTarget по активной дорожке', () => {
    expect(targetWatches.length).toBeGreaterThan(0)
  })

  test('в watch активной дорожки нет раннего return без сброса целей', () => {
    for (const b of targetWatches) {
      expect(b, 'ранний return при пустой дорожке оставит старые цели').not.toMatch(
        /if\s*\(\s*!\s*[\w$.]+\s*\)\s*return\b/,
      )
    }
  })

  test('пустая дорожка сбрасывает цели в пустые (через присваивание или targetFor)', () => {
    const ok = targetWatches.some(
      (b) =>
        (/envTarget\.value\s*=\s*(?:''|"")/.test(b) && /dspTarget\.value\s*=\s*(?:''|"")/.test(b)) ||
        setsBothTargets(b, studio),
    )
    expect(ok).toBe(true)
  })
})

describe('PedalBoard — активная дорожка общая (условие 120)', () => {
  const pedal = src('components/PedalBoard.vue')

  test('принимает активную дорожку через prop', () => {
    const m = /defineProps\s*(?:<[^>]*>)?\s*\(/.exec(pedal)
    expect(m, 'нет defineProps').not.toBeNull()
    const props = callBody(pedal, m.index + m[0].length - 1)
    expect(props).toMatch(/\b(?:active|activeStem)\b/)
  })

  test('сообщает выбор дорожки через emit', () => {
    const m = /defineEmits\s*(?:<[^>]*>)?\s*\(/.exec(pedal)
    expect(m, 'нет defineEmits').not.toBeNull()
    const emits = callBody(pedal, m.index + m[0].length - 1)
    expect(emits).toMatch(/['"](?:stem|update:stem)['"]/)
  })
})

describe('Волна сверху (условие 121)', () => {
  test.each([
    ['ru', RU],
    ['en', EN],
  ])('%s: есть подпись галочки studio.wave.pin', (lang, dict) => {
    expect(typeof dict['studio.wave.pin'], `${lang}: нет studio.wave.pin`).toBe('string')
    expect(dict['studio.wave.pin'].trim()).not.toBe('')
  })
})
