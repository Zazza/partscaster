// Строка хода операции в очереди (internal-preset-progress, условие 6):
// opProgressText(op, t) — чистая функция: {stage, pass, eta_s, stage_since} → «<шаг>[ N] · M:SS[ · осталось ≈ M:SS]».
// Подпись шага — queue.op.stage.<stage> (ru/en); номер сборки — только у render/verify/fix;
// время шага — stage_since в M:SS (секунды вниз до целого); «осталось ≈» — ключ queue.op.eta с {eta}, только
// если eta_s не null; без stage — пустая строка.
import { test, expect, describe } from 'vitest'
import { opProgressText } from './opProgress.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

const STAGES = ['prepare', 'parts', 'levels', 'render', 'verify', 'fix']
const WITH_PASS = new Set(['render', 'verify', 'fix'])

function makeT(dict) {
  return (key, vars) => {
    let s = dict[key] ?? key
    if (vars) for (const [k, v] of Object.entries(vars)) s = s.replaceAll(`{${k}}`, String(v))
    return s
  }
}
const tRu = makeT(RU)
const tEn = makeT(EN)

describe('подписи шагов', () => {
  test.each(STAGES)('ru и en: queue.op.stage.%s', (st) => {
    for (const [lang, d] of [['ru', RU], ['en', EN]]) {
      const v = d['queue.op.stage.' + st]
      expect(v, `${lang}: queue.op.stage.${st}`).toBeTruthy()
      expect(v).not.toContain('queue.op.stage')
    }
  })

  test('ключ «осталось ≈» есть в ru и en и подставляет {eta}', () => {
    expect(RU['queue.op.eta']).toContain('{eta}')
    expect(EN['queue.op.eta']).toContain('{eta}')
    expect(RU['queue.op.eta']).toContain('≈')
  })
})

describe('opProgressText', () => {
  test('пример из задачи (ru)', () => {
    const op = { stage: 'render', pass: 2, eta_s: 80, stage_since: 41 }
    expect(opProgressText(op, tRu)).toBe(`${RU['queue.op.stage.render']} 2 · 0:41 · осталось ≈ 1:20`)
  })

  test.each(STAGES)('шаг %s подписан, номер сборки — только у render/verify/fix', (st) => {
    for (const [t, d] of [[tRu, RU], [tEn, EN]]) {
      const text = opProgressText({ stage: st, pass: 3, eta_s: null, stage_since: 5 }, t)
      const label = d['queue.op.stage.' + st]
      const head = WITH_PASS.has(st) ? `${label} 3` : label
      expect(text).toBe(`${head} · 0:05`)
    }
  })

  test('en: та же сборка строки', () => {
    const text = opProgressText({ stage: 'fix', pass: 2, eta_s: 125, stage_since: 9 }, tEn)
    expect(text.startsWith(`${EN['queue.op.stage.fix']} 2 · 0:09 · `)).toBe(true)
    expect(text.endsWith(EN['queue.op.eta'].replace('{eta}', '2:05'))).toBe(true)
  })

  test.each([
    [0, '0:00'],
    [9, '0:09'],
    [41, '0:41'],
    [41.9, '0:41'],
    [60, '1:00'],
    [125, '2:05'],
    [3605, '60:05'],
  ])('время шага %s с → %s', (sec, want) => {
    expect(opProgressText({ stage: 'levels', pass: 1, eta_s: null, stage_since: sec }, tRu))
      .toBe(`${RU['queue.op.stage.levels']} · ${want}`)
  })

  test('eta null — без «осталось»', () => {
    const text = opProgressText({ stage: 'verify', pass: 1, eta_s: null, stage_since: 3 }, tRu)
    expect(text).toBe(`${RU['queue.op.stage.verify']} 1 · 0:03`)
    expect(text).not.toContain('≈')
  })

  test('eta нет в записи — без «осталось»', () => {
    expect(opProgressText({ stage: 'prepare', pass: 1, stage_since: 2 }, tRu)).not.toContain('≈')
  })

  test('eta 0 — показывается (это значение, не «нет оценки»)', () => {
    expect(opProgressText({ stage: 'verify', pass: 2, eta_s: 0, stage_since: 7 }, tRu))
      .toBe(`${RU['queue.op.stage.verify']} 2 · 0:07 · осталось ≈ 0:00`)
  })

  test('eta дробная — секунды вниз', () => {
    expect(opProgressText({ stage: 'fix', pass: 2, eta_s: 79.6, stage_since: 1 }, tRu))
      .toBe(`${RU['queue.op.stage.fix']} 2 · 0:01 · осталось ≈ 1:19`)
  })

  test.each([
    [{}],
    [{ stage: '' }],
    [{ stage: null, pass: 1, eta_s: 10, stage_since: 5 }],
    [{ state: 'waiting', kind: 'generate' }],
  ])('без stage — пустая строка: %j', (op) => {
    expect(opProgressText(op, tRu)).toBe('')
  })

  test('нет записи — пустая строка, не падение', () => {
    expect(opProgressText(null, tRu)).toBe('')
    expect(opProgressText(undefined, tRu)).toBe('')
  })
})
