// Сетка тактов трека одним запросом на трек и регистр партии синтезатора.
// Тесты — по условиям задачи, не по коду: chordGridOnce (кэш запроса сетки), partRegister (нижняя нота,
// подпись диапазона, «в регистре гитар»), ключи i18n и проводка в шаблонах (prop open, вызов chordGridOnce,
// рамка шагов «Звук», «Голос», «Готово»).
import { readFileSync } from 'node:fs'
import { describe, it, expect, vi } from 'vitest'
import { chordGridOnce } from './trackGrid.js'
import { partRegister, GUITAR_TOP } from './synthPart.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'
import { templateAst, findElements, dir, attr } from './testkit/sfc.js'

// Кэш модульный и общий на файл — у каждого теста свой jobId.
let nextJob = 9000
const freshJob = () => ++nextJob

// api с управляемыми ответами: каждый вызов chordGrid — новый отложенный Promise
function deferredApi() {
  const pending = []
  const api = {
    chordGrid: vi.fn(() => new Promise((resolve, reject) => pending.push({ resolve, reject }))),
  }
  return { api, pending }
}

const read = (rel) => readFileSync(new URL(`./${rel}`, import.meta.url), 'utf8')

describe('chordGridOnce: один запрос сетки на трек', () => {
  it('два последовательных вызова для одного трека — один запрос, тот же результат', async () => {
    const job = freshJob()
    const grid = { bars: [{ chord: 'Am' }] }
    const api = { chordGrid: vi.fn(async () => grid) }
    const a = await chordGridOnce(api, job)
    const b = await chordGridOnce(api, job)
    expect(api.chordGrid).toHaveBeenCalledTimes(1)
    expect(api.chordGrid).toHaveBeenCalledWith(job)
    expect(a).toBe(grid)
    expect(b).toBe(grid)
  })

  it('два одновременных вызова до ответа — один запрос, оба получают тот же результат', async () => {
    const job = freshJob()
    const { api, pending } = deferredApi()
    const p1 = chordGridOnce(api, job)
    const p2 = chordGridOnce(api, job)
    expect(api.chordGrid).toHaveBeenCalledTimes(1)
    const grid = { bars: [] }
    pending[0].resolve(grid)
    expect(await p1).toBe(grid)
    expect(await p2).toBe(grid)
    expect(api.chordGrid).toHaveBeenCalledTimes(1)
  })

  it('разные треки — разные запросы и свои результаты', async () => {
    const j1 = freshJob()
    const j2 = freshJob()
    const api = { chordGrid: vi.fn(async (id) => ({ id })) }
    const a = await chordGridOnce(api, j1)
    const b = await chordGridOnce(api, j2)
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
    expect(api.chordGrid).toHaveBeenNthCalledWith(1, j1)
    expect(api.chordGrid).toHaveBeenNthCalledWith(2, j2)
    expect(a).toEqual({ id: j1 })
    expect(b).toEqual({ id: j2 })
  })

  it('fresh: true — запрос заново; дальше без fresh отдаётся новый результат', async () => {
    const job = freshJob()
    let n = 0
    const api = { chordGrid: vi.fn(async () => ({ v: ++n })) }
    expect(await chordGridOnce(api, job)).toEqual({ v: 1 })
    expect(await chordGridOnce(api, job, { fresh: true })).toEqual({ v: 2 })
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
    expect(await chordGridOnce(api, job)).toEqual({ v: 2 })
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
  })

  it('fresh: true на первом вызове — тоже один запрос', async () => {
    const job = freshJob()
    const api = { chordGrid: vi.fn(async () => ({ ok: 1 })) }
    expect(await chordGridOnce(api, job, { fresh: true })).toEqual({ ok: 1 })
    expect(api.chordGrid).toHaveBeenCalledTimes(1)
  })

  it('сбой не запоминается: ошибка доходит до вызывающего, следующий вызов спрашивает api снова', async () => {
    const job = freshJob()
    const err = new Error('worker down')
    const api = {
      chordGrid: vi.fn()
        .mockImplementationOnce(() => Promise.reject(err))
        .mockImplementationOnce(async () => ({ bars: ['C'] })),
    }
    await expect(chordGridOnce(api, job)).rejects.toBe(err)
    expect(await chordGridOnce(api, job)).toEqual({ bars: ['C'] })
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
    // успешный результат после сбоя уже запомнен
    expect(await chordGridOnce(api, job)).toEqual({ bars: ['C'] })
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
  })

  it('сбой при одновременных вызовах — ошибку получают оба, следующий вызов — новый запрос', async () => {
    const job = freshJob()
    const { api, pending } = deferredApi()
    const p1 = chordGridOnce(api, job)
    const p2 = chordGridOnce(api, job)
    const err = new Error('timeout')
    pending[0].reject(err)
    await expect(p1).rejects.toBe(err)
    await expect(p2).rejects.toBe(err)
    const p3 = chordGridOnce(api, job)
    expect(api.chordGrid).toHaveBeenCalledTimes(2)
    pending[1].resolve({ bars: [] })
    expect(await p3).toEqual({ bars: [] })
  })
})

describe('partRegister: регистр партии синтезатора', () => {
  it('GUITAR_TOP экспортирован и равен 72 (C5)', () => {
    expect(GUITAR_TOP).toBe(72)
  })

  it.each([
    ['pad', 0, 'C4–B4', true],
    ['pad', 1, 'C5–B5', false],
    ['pad', -1, 'C3–B3', true],
    ['arp', 0, 'C4–B4', true],
    ['arp', -1, 'C3–B3', true],
    ['drone', 0, 'C4–B4', true],
    ['drone', 2, 'C6–B6', false],
    ['pulse', 0, 'C2–B2', false],
    ['pulse', 2, 'C4–B4', false],
    ['pulse', -2, 'C0–B0', false],
  ])('%s, octave %s → %s, guitars=%s', (style, octave, range, guitars) => {
    expect(partRegister(style, octave)).toEqual({ range, guitars })
  })

  it('без аргументов — как pad, 0', () => {
    expect(partRegister()).toEqual(partRegister('pad', 0))
    expect(partRegister()).toEqual({ range: 'C4–B4', guitars: true })
  })

  it('octave зажат в −2…2', () => {
    expect(partRegister('pad', 5)).toEqual(partRegister('pad', 2))
    expect(partRegister('pad', 5).range).toBe('C6–B6')
    expect(partRegister('pad', -9)).toEqual(partRegister('pad', -2))
    expect(partRegister('pad', -9).range).toBe('C2–B2')
    expect(partRegister('pulse', 3).range).toBe('C4–B4')
  })

  it('дробная октава округляется', () => {
    expect(partRegister('pad', 0.4)).toEqual(partRegister('pad', 0))
    expect(partRegister('pad', 0.6)).toEqual(partRegister('pad', 1))
    expect(partRegister('pad', 1.6)).toEqual(partRegister('pad', 2))
    expect(partRegister('pad', -1.6)).toEqual(partRegister('pad', -2))
  })

  it('граница гитар: нижняя нота ровно C5 — уже не в регистре гитар, ниже — в регистре', () => {
    // pad +1 → нижняя C5 (72) — не ниже GUITAR_TOP
    expect(partRegister('pad', 1).guitars).toBe(false)
    // pad 0 → нижняя C4 (60) — ниже
    expect(partRegister('pad', 0).guitars).toBe(true)
  })

  it('pulse никогда не «в регистре гитар», даже низко', () => {
    for (const o of [-2, -1, 0, 1, 2]) expect(partRegister('pulse', o).guitars).toBe(false)
  })

  it('диапазон — 12 полутонов: от C до B одной октавы', () => {
    for (const style of ['pad', 'arp', 'drone', 'pulse']) {
      for (const o of [-2, -1, 0, 1, 2]) {
        const m = partRegister(style, o).range.match(/^C(-?\d+)–B(-?\d+)$/)
        expect(m, `${style} ${o}`).not.toBeNull()
        expect(m[1]).toBe(m[2])
      }
    }
  })
})

describe('i18n: подписи сетки и регистра', () => {
  const KEYS = ['synth.grid.busy', 'synth.register.guitars', 'synth.register.tip']
  it.each(KEYS)('%s есть и непустой в ru и en', (key) => {
    for (const [lang, dict] of [['ru', RU], ['en', EN]]) {
      expect(typeof dict[key], `${lang}: нет ${key}`).toBe('string')
      expect(dict[key].trim(), `${lang}: пустая подпись ${key}`).not.toBe('')
      expect(dict[key], `${lang}: подпись ${key} — сырой ключ`).not.toBe(key)
    }
  })
})

describe('проводка в компонентах студии', () => {
  // Скрипт компонента (оба блока <script>, если их два)
  const scriptOf = (rel) => [...read(rel).matchAll(/<script[^>]*>([\s\S]*?)<\/script>/g)].map((m) => m[1]).join('\n')

  it.each(['components/StudioSynth.vue', 'components/StudioPerc.vue'])('%s объявляет проп open', (rel) => {
    const js = scriptOf(rel)
    const props = js.match(/defineProps\s*\(([\s\S]*?)\)\s*\n/)
    expect(props, `${rel}: нет defineProps`).not.toBeNull()
    expect(props[1]).toMatch(/\bopen\b/)
  })

  it.each(['components/StudioSynth.vue', 'components/StudioPerc.vue'])('%s берёт сетку через chordGridOnce(', (rel) => {
    expect(scriptOf(rel)).toMatch(/\bchordGridOnce\s*\(/)
  })

  it('StudioPage передаёт :open в StudioSynth и StudioPerc', () => {
    const ast = templateAst('components/StudioPage.vue')
    for (const tag of ['StudioSynth', 'StudioPerc']) {
      const els = findElements(ast, (n) => n.tag === tag)
      expect(els.length, `${tag} не найден в шаблоне StudioPage`).toBeGreaterThan(0)
      for (const { node } of els) expect(dir(node, 'bind', 'open'), `${tag} без :open`).toBeTruthy()
    }
  })

  it('шаги «Звук», «Голос», «Готово» — каждый в .studio-step-box, ровно три рамки', () => {
    const ast = templateAst('components/StudioPage.vue')
    const hasBox = (n) => (attr(n, 'class') || '').split(/\s+/).includes('studio-step-box')
    const boxes = findElements(ast, hasBox)
    expect(boxes.length).toBe(3)
    // шаг узнаём по заголовку: рамка — следующий элемент после заголовка шага
    const steps = []
    for (const { node, ancestors } of boxes) {
      const parent = ancestors[ancestors.length - 1]
      const siblings = (parent.children || []).filter((c) => c.type === 1)
      const prev = siblings[siblings.indexOf(node) - 1]
      expect(prev, 'перед рамкой нет заголовка шага').toBeTruthy()
      steps.push(attr(prev, 'id'))
    }
    expect(steps.sort()).toEqual(['st-done', 'st-sound', 'st-voice'])
  })

  it('в App.vue есть правило .studio-step-box', () => {
    const sfc = read('App.vue')
    const css = [...sfc.matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n')
    const rules = [...css.matchAll(/([^{}]*)\{([^{}]*)\}/g)]
      .filter((r) => r[1].split(',').some((sel) => /\.studio-step-box(?![\w-])/.test(sel)))
    expect(rules.length, '.studio-step-box не найден в стилях App.vue').toBeGreaterThan(0)
    expect(rules.some((r) => r[2].trim() !== ''), 'правило .studio-step-box пустое').toBe(true)
  })
})
