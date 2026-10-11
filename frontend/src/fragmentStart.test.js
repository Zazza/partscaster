// Тесты карточки internal-own-track, уточнение 123а: готовый «кусок с правками» играет с from − 4 с;
// окно — запомненное from или cont_from джобы; начало раньше 4 с — с 0.
//   - fragmentStart(fj, lead = 4) из results.js — секунда старта проигрывания;
//   - сценарий слияния: строка списка джоб (без from) приходит поверх запомненного объекта куска —
//     окно должно пережить обновление, поэтому watchJob в StudioPage.vue сливает объект, а не заменяет.
// Написаны по карточке, без чтения реализации.
import { readFileSync } from 'node:fs'
import { describe, it, expect } from 'vitest'
import { fragmentStart } from './results.js'

describe('fragmentStart — старт куска за lead секунд до правки (123а)', () => {
  it('from 30 → 26', () => {
    expect(fragmentStart({ from: 30 })).toBe(26)
  })

  it('только cont_from 30 → 26', () => {
    expect(fragmentStart({ cont_from: 30 })).toBe(26)
  })

  it('from важнее cont_from', () => {
    expect(fragmentStart({ from: 30, cont_from: 50 })).toBe(26)
    expect(fragmentStart({ from: 50, cont_from: 30 })).toBe(46)
  })

  it('начало раньше lead — с 0', () => {
    expect(fragmentStart({ from: 3 })).toBe(0)
    expect(fragmentStart({ from: 0 })).toBe(0)
  })

  it('from ровно lead → 0', () => {
    expect(fragmentStart({ from: 4 })).toBe(0)
  })

  it('нет from и cont_from → 0', () => {
    expect(fragmentStart({})).toBe(0)
    expect(fragmentStart({ id: 7, status: 'done', audio_file: 'audio.flac' })).toBe(0)
  })

  it('null / undefined → 0', () => {
    expect(fragmentStart(null)).toBe(0)
    expect(fragmentStart(undefined)).toBe(0)
  })

  it('свой lead 2 → from − 2', () => {
    expect(fragmentStart({ from: 30 }, 2)).toBe(28)
    expect(fragmentStart({ cont_from: 30 }, 2)).toBe(28)
    expect(fragmentStart({ from: 1 }, 2)).toBe(0)
  })
})

describe('окно куска переживает обновление строкой списка джоб (то, что ломалось)', () => {
  const initial = () => ({ id: 7, status: 'queued', from: 30, to: 40 })
  const row = () => ({ id: 7, status: 'done', audio_file: 'audio.flac' })

  it('слияние {...запомненное, ...строка} сохраняет окно → 26', () => {
    const merged = { ...initial(), ...row() }
    expect(merged.status).toBe('done')
    expect(merged.audio_file).toBe('audio.flac')
    expect(fragmentStart(merged)).toBe(26)
  })

  it('замена объекта строкой теряет окно → 0 (поэтому нужно слияние)', () => {
    const replaced = row()
    expect(fragmentStart(replaced)).toBe(0)
  })
})

describe('StudioPage.vue: watchJob сливает строку джобы в запомненный объект', () => {
  const SFC = readFileSync(new URL('./components/StudioPage.vue', import.meta.url), 'utf8')
  const script = (SFC.match(/<script[^>]*>([\s\S]*?)<\/script>/) || [])[1] || ''
  // тело watchJob: от объявления до следующего объявления верхнего уровня
  const start = script.search(/function\s+watchJob\s*\(/)
  const rest = start >= 0 ? script.slice(start) : ''
  const next = rest.slice(1).search(/\n(?:async\s+)?function\s|\n(?:const|let)\s/)
  const body = next >= 0 ? rest.slice(0, next + 1) : rest

  it('watchJob объявлена', () => {
    expect(start).toBeGreaterThanOrEqual(0)
  })

  it('refObj.value присваивается слиянием { ...refObj.value, ...j }', () => {
    expect(body).toMatch(/refObj\.value\s*=\s*\{\s*\.\.\.refObj\.value\s*,\s*\.\.\.\s*\w+\s*\}/)
  })

  it('нет прямой замены refObj.value = <строка джобы>', () => {
    expect(body).not.toMatch(/refObj\.value\s*=\s*(?!\{|null\b|=)[A-Za-z_$][\w$]*\s*(?:[;\n]|$)/)
  })
})
