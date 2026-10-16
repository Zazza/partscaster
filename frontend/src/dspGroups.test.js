import { describe, it, expect } from 'vitest'
import { chainGroupOptions, voiceFxChains, firstSungSec, voicePreviewWindow } from './dspVoice.js'
import { previewWindow, PREVIEW_LEN } from './fxPreview.js'

// internal-voice-fx: группы в общем списке эффектов «3 Звук» и блок
// «Обработка голоса» в шаге «4 Голос». Цепочки — как из биндинга YueDspChains
// (dsp.All): {id, name, group, voice?, for_voice?}.

const ch = (id, group, over = {}) => ({ id, name: 'N-' + id, group, ...over })
const LABELS = {
  space: 'Пространство', pedals: 'Педали', modulation: 'Модуляция', filter: 'Фильтры',
  dynamics: 'Динамика', time: 'Темп', lofi: 'Лоуфай', voice: 'Голос', other: 'прочее',
}
const label = g => LABELS[g] ?? ('?' + g)
const headers = opts => opts.filter(o => o.header)
const items = opts => opts.filter(o => !o.header)

describe('ТК2 chainGroupOptions — опции общего списка с заголовками групп', () => {
  // вход намеренно перемешан по группам: порядок групп — по разделам документации
  const chains = [
    ch('tape', 'lofi'),
    ch('megaphone', 'voice', { voice: true }),
    ch('reverb-hall', 'space', { for_voice: true }),
    ch('fuzz', 'pedals'),
    ch('stutter', 'time'),
    ch('comp', 'dynamics'),
    ch('phone', 'voice', { voice: true }),
    ch('lowpass', 'filter'),
    ch('chorus', 'modulation'),
    ch('reverb-room', 'space', { for_voice: true }),
    ch('crush', 'lofi'),
  ]

  it('голосовых цепочек (группа voice) в списке нет', () => {
    const vals = chainGroupOptions(chains, label).map(o => o.value)
    expect(vals).not.toContain('megaphone')
    expect(vals).not.toContain('phone')
    expect(headers(chainGroupOptions(chains, label)).map(h => h.label)).not.toContain('Голос')
  })

  it('заголовки групп — в порядке space, pedals, modulation, filter, dynamics, time, lofi', () => {
    expect(headers(chainGroupOptions(chains, label)).map(h => h.label)).toEqual([
      'Пространство', 'Педали', 'Модуляция', 'Фильтры', 'Динамика', 'Темп', 'Лоуфай',
    ])
  })

  it('заголовок стоит перед пунктами своей группы; пункты внутри группы — в порядке входа', () => {
    const opts = chainGroupOptions(chains, label)
    const seq = opts.map(o => (o.header ? '#' + o.label : o.value))
    expect(seq).toEqual([
      '#Пространство', 'reverb-hall', 'reverb-room',
      '#Педали', 'fuzz',
      '#Модуляция', 'chorus',
      '#Фильтры', 'lowpass',
      '#Динамика', 'comp',
      '#Темп', 'stutter',
      '#Лоуфай', 'tape', 'crush',
    ])
  })

  it('заголовок только у непустой группы', () => {
    const opts = chainGroupOptions([ch('fuzz', 'pedals'), ch('tape', 'lofi')], label)
    expect(headers(opts).map(h => h.label)).toEqual(['Педали', 'Лоуфай'])
  })

  it('заголовок — header и disabled, подпись от label(group), value уникален и не совпадает с id', () => {
    const tricky = [...chains, ch('space', 'pedals'), ch('lofi', 'lofi'), ch('header-space', 'space')]
    const opts = chainGroupOptions(tricky, label)
    const hs = headers(opts)
    expect(hs.length).toBe(7)
    for (const h of hs) {
      expect(h.header).toBe(true)
      expect(h.disabled).toBe(true)
      expect(h.value).not.toBeUndefined()
      expect(h.value).not.toBeNull()
    }
    const ids = new Set(tricky.map(c => c.id))
    for (const h of hs) expect(ids.has(h.value)).toBe(false)
    const allVals = opts.map(o => o.value)
    expect(new Set(allVals).size).toBe(allVals.length)
  })

  it('пункт — {value: id, label: name}, не заголовок и не отключён', () => {
    const it0 = items(chainGroupOptions([ch('fuzz', 'pedals')], label))
    expect(it0).toHaveLength(1)
    expect(it0[0].value).toBe('fuzz')
    expect(it0[0].label).toBe('N-fuzz')
    expect(it0[0].header).toBeFalsy()
    expect(it0[0].disabled).toBeFalsy()
  })

  it('цепочки без известной группы — в группе other («прочее») последней', () => {
    const opts = chainGroupOptions([
      ch('weird', 'nope'), ch('fuzz', 'pedals'), ch('nogroup', undefined), ch('tape', 'lofi'),
    ], label)
    const seq = opts.map(o => (o.header ? '#' + o.label : o.value))
    expect(seq).toEqual(['#Педали', 'fuzz', '#Лоуфай', 'tape', '#прочее', 'weird', 'nogroup'])
  })

  it('пустой вход — пустой список; только голосовые — тоже пусто', () => {
    expect(chainGroupOptions([], label)).toEqual([])
    expect(chainGroupOptions([ch('megaphone', 'voice', { voice: true })], label)).toEqual([])
  })

  it('label вызывается с именем группы', () => {
    const seen = []
    chainGroupOptions([ch('fuzz', 'pedals'), ch('x', 'zzz')], g => { seen.push(g); return g })
    expect(seen).toContain('pedals')
    expect(seen).toContain('other')
  })
})

describe('ТК3 voiceFxChains — эффекты блока «Обработка голоса»', () => {
  it('сначала голосовые, потом for_voice, порядок входа; остальных нет', () => {
    const chains = [
      ch('reverb-hall', 'space', { for_voice: true }),
      ch('megaphone', 'voice', { voice: true }),
      ch('fuzz', 'pedals'),
      ch('haas', 'space', { for_voice: true }),
      ch('phone', 'voice', { voice: true }),
      ch('tape', 'lofi'),
      ch('octaver', 'pedals', { for_voice: true }),
    ]
    expect(voiceFxChains(chains).map(c => c.id)).toEqual([
      'megaphone', 'phone', 'reverb-hall', 'haas', 'octaver',
    ])
  })

  it('возвращает сами цепочки (с описанием и параметрами)', () => {
    const m = ch('megaphone', 'voice', { voice: true, note: 'как у Летова', params: [{ id: 'mix' }] })
    expect(voiceFxChains([m])[0]).toEqual(m)
  })

  it('нет подходящих или пустой вход — пустой массив', () => {
    expect(voiceFxChains([ch('fuzz', 'pedals'), ch('tape', 'lofi')])).toEqual([])
    expect(voiceFxChains([])).toEqual([])
  })
})

describe('ТК4 firstSungSec — секунда первого спетого такта по плану', () => {
  const bar = (idx, voices, start, end, section = 'verse') =>
    ({ idx, section, voices, rests: {}, start_sec: start, end_sec: end })

  it('интро с Vocal = 0 пропускается — первый бар с нотами Vocal', () => {
    const bars = [
      bar(0, { Vocal: 0 }, 0, 2, 'intro'),
      bar(1, { Vocal: 0 }, 2, 4, 'intro'),
      bar(2, { Vocal: 5 }, 4, 6),
      bar(3, { Vocal: 4 }, 6, 8),
    ]
    expect(firstSungSec(bars)).toBe(4)
  })

  it('бары идут не по порядку — берётся самый ранний по start_sec', () => {
    const bars = [
      bar(5, { Vocal: 3 }, 10, 12),
      bar(3, { Vocal: 2 }, 6.5, 8),
      bar(0, { Vocal: 0 }, 0, 2, 'intro'),
    ]
    expect(firstSungSec(bars)).toBe(6.5)
  })

  it('бары Ins с нотами не считаются — голоса идут отдельными записями', () => {
    const bars = [
      bar(0, { Ins: 8 }, 0, 2, 'intro'),
      bar(1, { Ins: 8 }, 2, 4, 'intro'),
      bar(0, { Vocal: 0 }, 0, 2, 'intro'),
      bar(1, { Vocal: 0 }, 2, 4, 'intro'),
      bar(2, { Ins: 6 }, 4, 6),
      bar(2, { Vocal: 3 }, 4, 6),
    ]
    expect(firstSungSec(bars)).toBe(4)
  })

  it('имя голоса Vocal — без учёта регистра', () => {
    expect(firstSungSec([bar(0, { vocal: 0 }, 0, 2), bar(1, { VOCAL: 2 }, 3, 5)])).toBe(3)
  })

  it('первый спетый такт в нуле — 0, а не null', () => {
    expect(firstSungSec([bar(0, { Vocal: 1 }, 0, 2)])).toBe(0)
  })

  it('нигде не поют — null', () => {
    expect(firstSungSec([bar(0, { Vocal: 0 }, 0, 2), bar(0, { Ins: 4 }, 0, 2), bar(1, {}, 2, 4)])).toBeNull()
  })

  it('пустой массив, undefined и null — null', () => {
    expect(firstSungSec([])).toBeNull()
    expect(firstSungSec(undefined)).toBeNull()
    expect(firstSungSec(null)).toBeNull()
  })
})

describe('ТК6 voicePreviewWindow — кусок превью эффекта на голосе', () => {
  it('выделение главнее sungAt и курсора', () => {
    const w = voicePreviewWindow({ from: 50, to: 58 }, 30, 100, 200)
    expect(w).toEqual(previewWindow({ from: 50, to: 58 }, null, 200))
    expect(w).toEqual({ from: 50, to: 58 })
  })

  it('нет выделения — от sungAt, курсор не важен; длина не меньше превью по умолчанию', () => {
    const w = voicePreviewWindow(null, 30, 100, 200)
    expect(w.from).toBe(30)
    expect(w.to - w.from).toBeGreaterThanOrEqual(PREVIEW_LEN)
    expect(w.to).toBeLessThanOrEqual(200)
  })

  it('sungAt = 0 — это секунда 0, а не «нет плана»', () => {
    const w = voicePreviewWindow(null, 0, 100, 200)
    expect(w.from).toBe(0)
    expect(w.to - w.from).toBeGreaterThanOrEqual(PREVIEW_LEN)
  })

  it('пустое выделение (to <= from) — как его нет: от sungAt', () => {
    expect(voicePreviewWindow({ from: 40, to: 40 }, 30, 100, 200).from).toBe(30)
  })

  it('окно от sungAt не выходит за конец трека', () => {
    const w = voicePreviewWindow(null, 195, null, 200)
    expect(w.to).toBeLessThanOrEqual(200)
    expect(w.from).toBeGreaterThanOrEqual(0)
    expect(w.to).toBeGreaterThan(w.from)
  })

  it('нет sungAt (null/undefined) — как previewWindow от курсора', () => {
    expect(voicePreviewWindow(null, null, 100, 200)).toEqual(previewWindow(null, 100, 200))
    expect(voicePreviewWindow(null, undefined, 100, 200)).toEqual(previewWindow(null, 100, 200))
    expect(voicePreviewWindow(null, null, null, 200)).toEqual(previewWindow(null, null, 200))
    expect(voicePreviewWindow(null, null, 195, 200)).toEqual(previewWindow(null, 195, 200))
  })
})
