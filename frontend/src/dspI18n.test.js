import { describe, it, expect } from 'vitest'
import {
  CHAIN_EN, DSP_PRESET_EN, LABEL_EN, WARNING_EN,
  localizeChains, localizeDspPresets, localizeWarning,
} from './dspI18n.js'

// internal-dsp-en ТК2–ТК3: английские названия цепочек, наборов педалей, подписей ручек и предупреждения.
// Данные — из самих словарей (любой известный id/подпись), чтобы тест не зависел от конкретных текстов.

const chainId = Object.keys(CHAIN_EN)[0]
const presetId = Object.keys(DSP_PRESET_EN)[0]
const label = Object.keys(LABEL_EN)[0]
const label2 = Object.keys(LABEL_EN)[1]

const chains = () => [
  {
    id: chainId, name: 'Русское имя', note: 'Русское описание', group: 'pedals', voice: false,
    params: [
      { id: 'a', label, min: 0, max: 10, step: 1, default: 5 },
      { id: 'b', label: 'неизвестная подпись', min: 0, max: 1, step: 0.1, default: 0 },
    ],
  },
  {
    id: 'no-such-chain', name: 'Неизвестная', note: 'Как пришла', group: 'space',
    params: [{ id: 'c', label: label2, min: 0, max: 1, step: 1, default: 0 }],
  },
]
const presets = () => [
  { id: presetId, name: 'Набор', note: 'Описание набора', steps: [{ chain: chainId, params: { a: 1 } }] },
  { id: 'no-such-preset', name: 'Чужой набор', note: 'Как пришёл', steps: [] },
]

describe('localizeChains', () => {
  it('en: имя, описание и подписи ручек известной цепочки — по-английски, прочие поля как были', () => {
    const [c] = localizeChains(chains(), 'en')
    expect(c.name).toBe(CHAIN_EN[chainId][0])
    expect(c.note).toBe(CHAIN_EN[chainId][1])
    expect(c.params[0]).toEqual({ id: 'a', label: LABEL_EN[label], min: 0, max: 10, step: 1, default: 5 })
    expect(c).toMatchObject({ id: chainId, group: 'pedals', voice: false })
  })
  it('en: неизвестная подпись ручки остаётся как пришла', () => {
    const [c] = localizeChains(chains(), 'en')
    expect(c.params[1].label).toBe('неизвестная подпись')
  })
  it('en: неизвестный id — имя и описание как пришли, известные подписи ручек переведены', () => {
    const c = localizeChains(chains(), 'en')[1]
    expect(c.name).toBe('Неизвестная')
    expect(c.note).toBe('Как пришла')
    expect(c.params[0].label).toBe(LABEL_EN[label2])
  })
  it('ru: список без изменений', () => {
    expect(localizeChains(chains(), 'ru')).toEqual(chains())
  })
  it('исходный список не мутирует', () => {
    const src = chains()
    localizeChains(src, 'en')
    expect(src).toEqual(chains())
  })
  it('пустой список — пустой список', () => {
    expect(localizeChains([], 'en')).toEqual([])
  })
})

describe('localizeDspPresets', () => {
  it('en: имя и описание известного набора — по-английски, шаги как были', () => {
    const [p] = localizeDspPresets(presets(), 'en')
    expect(p.name).toBe(DSP_PRESET_EN[presetId][0])
    expect(p.note).toBe(DSP_PRESET_EN[presetId][1])
    expect(p.steps).toEqual(presets()[0].steps)
  })
  it('en: неизвестный id — как пришёл', () => {
    expect(localizeDspPresets(presets(), 'en')[1]).toEqual(presets()[1])
  })
  it('ru: список без изменений', () => {
    expect(localizeDspPresets(presets(), 'ru')).toEqual(presets())
  })
  it('исходный список не мутирует', () => {
    const src = presets()
    localizeDspPresets(src, 'en')
    expect(src).toEqual(presets())
  })
  it('пустой список — пустой список', () => {
    expect(localizeDspPresets([], 'en')).toEqual([])
  })
})

describe('localizeWarning', () => {
  const ru = 'гитара YuE2 — у трека нет плана'
  it('словарь знает предупреждение «нет плана», перевод без кириллицы', () => {
    expect(WARNING_EN[ru]).toBeTruthy()
    expect(WARNING_EN[ru]).not.toMatch(/[а-яё]/i)
  })
  it('en + известная строка — английская', () => {
    expect(localizeWarning(ru, 'en')).toBe(WARNING_EN[ru])
  })
  it('ru + известная строка — как есть', () => {
    expect(localizeWarning(ru, 'ru')).toBe(ru)
  })
  it('ТК4: en + «<имя пресета>: <известная строка>» — имя как есть, предупреждение по-английски', () => {
    expect(localizeWarning('Синтвейв · ретро: гитара YuE2 — у трека нет плана', 'en'))
      .toBe('Синтвейв · ретро: YuE2 guitar — the track has no plan')
  })
  it('ТК4: ru + «<имя пресета>: <известная строка>» — как есть', () => {
    const w = 'Синтвейв · ретро: гитара YuE2 — у трека нет плана'
    expect(localizeWarning(w, 'ru')).toBe(w)
  })
  it('en + неизвестная строка — как есть', () => {
    expect(localizeWarning('что-то другое', 'en')).toBe('что-то другое')
  })
})
