// Чистая логика активной дорожки — targetFor (цель правок по дорожке)
// и stemForTarget (какую дорожку сделать активной при ручном выборе цели).
// Одна активная дорожка в студии — targetChoice (ручной выбор цели: какую дорожку сделать активной и
// какую цель поставить; «весь трек» сбрасывает цели), PedalBoard принимает активную дорожку и сообщает свой выбор.
// Подпись галочки «волна сверху» — общий тест i18n.test.js.
import { describe, expect, test, vi } from 'vitest'
import { renderHtml } from './testkit/sfc.js'
import { targetFor, stemForTarget, targetChoice } from './activeTrack.js'
import RU from './i18n/ru.js'

vi.hoisted(() => { globalThis.localStorage = { getItem: () => null, setItem() {}, removeItem() {} } })
vi.mock('./api.js', () => ({ api: {} }))
const { default: PedalBoard } = await import('./components/PedalBoard.vue')

const DRUM_PARTS = ['kick', 'snare', 'toms', 'hh', 'ride', 'crash']
const MAIN_STEMS = ['vocals', 'drums', 'bass', 'other', 'guitar', 'piano']

describe('targetFor — цель правок дорожки', () => {
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

describe('stemForTarget — активная дорожка по выбранной цели', () => {
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

describe('targetChoice — ручной выбор цели правок («на что»)', () => {
  const stems = ['vocals', 'drums', 'bass', 'other', 'kick', 'snare']

  test('трек без дорожек: цель держится как выбрана, активная дорожка не меняется', () => {
    expect(targetChoice('bass', [], '')).toEqual({ stem: null, target: 'bass' })
    expect(targetChoice('guitar', null, '')).toEqual({ stem: null, target: 'guitar' })
  })

  test('есть дорожка цели — она становится активной, цель по ней', () => {
    expect(targetChoice('bass', stems, 'vocals')).toEqual({ stem: 'bass', target: 'bass' })
  })

  test('активная уже даёт цель (kick при drums) — дорожка та же, цель drums', () => {
    expect(targetChoice('drums', stems, 'kick')).toEqual({ stem: null, target: 'drums' })
  })

  test('«весь трек» — активной дорожки нет и цели правок пустые (без раннего выхода со старыми целями)', () => {
    expect(targetChoice('', stems, 'bass')).toEqual({ stem: '', target: '' })
    expect(targetChoice('', stems, '')).toEqual({ stem: null, target: '' })
  })

  test('дорожки цели нет — весь трек', () => {
    expect(targetChoice('guitar', stems, 'bass')).toEqual({ stem: '', target: '' })
  })
})

// PedalBoard: активная дорожка — входом (prop active), выбор на доске — наружу (emit stem). Контракт компонента
// берётся из собранного SFC, видимая дорожка — серверной отрисовкой при данном active.
describe('PedalBoard — активная дорожка общая', () => {
  test('принимает prop active и объявляет событие stem', () => {
    expect(Object.keys(PedalBoard.props)).toContain('active')
    expect(PedalBoard.emits).toContain('stem')
  })

  test('active задаёт дорожку доски: показана выбранная дорожка, без active — гитара', async () => {
    const job = { id: 5, title: 't', duration_sec: 60 }
    const pick = (html) => (html.match(/class="vselect-label"[^>]*>([^<]*)</) || [])[1]
    expect(pick(await renderHtml(PedalBoard, { job, active: 'bass' }))).toBe(RU['studio.dsp.target.bass'])
    expect(pick(await renderHtml(PedalBoard, { job, active: null }))).toBe(RU['studio.dsp.target.guitar'])
  })
})
