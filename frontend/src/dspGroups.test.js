import { describe, it, expect } from 'vitest'
import { voiceFxChains, firstSungSec, voicePreviewWindow, timbreWindow, TIMBRE_MIN_SEC, TIMBRE_MAX_SEC } from './dspVoice.js'
import { previewWindow, PREVIEW_LEN } from './fxPreview.js'

// internal-voice-fx: блок
// «Обработка голоса» в шаге «4 Голос». Цепочки — как из биндинга YueDspChains
// (dsp.All): {id, name, group, voice?, for_voice?}.

const ch = (id, group, over = {}) => ({ id, name: 'N-' + id, group, ...over })

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
    const m = ch('megaphone', 'voice', { voice: true, note: 'голос сквозь рупор', params: [{ id: 'mix' }] })
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

// internal-album-voice-draft: окно куска, который «Голоса альбома» отдаёт воркеру.
// Воркер принимает только куски длиной 3–30 с; timbreWindow приводит выбор студии
// к этим границам. durSec = длина трека (0/undefined — неизвестна, верхней границы нет).
describe('internal-album-voice-draft: окно куска «Голоса альбома»', () => {
  it('границы воркера: 3 и 30 секунд', () => {
    expect(TIMBRE_MIN_SEC).toBe(3)
    expect(TIMBRE_MAX_SEC).toBe(30)
  })

  it('кусок 3–30 с не меняется (включая ровно 3 и ровно 30)', () => {
    expect(timbreWindow({ from: 10, to: 20 }, 200)).toEqual({ from: 10, to: 20 })
    expect(timbreWindow({ from: 10, to: 13 }, 200)).toEqual({ from: 10, to: 13 })
    expect(timbreWindow({ from: 10, to: 40 }, 200)).toEqual({ from: 10, to: 40 })
  })

  it('длиннее 30 с — начало сохраняется, конец = начало + 30', () => {
    expect(timbreWindow({ from: 15, to: 120 }, 200)).toEqual({ from: 15, to: 45 })
    expect(timbreWindow({ from: 0, to: 30.5 }, 200)).toEqual({ from: 0, to: 30 })
  })

  it('короче 3 с — растягивается до 3 с вперёд', () => {
    expect(timbreWindow({ from: 50, to: 51 }, 200)).toEqual({ from: 50, to: 53 })
  })

  it('кусок нулевой длины — тоже 3 с', () => {
    expect(timbreWindow({ from: 50, to: 50 }, 200)).toEqual({ from: 50, to: 53 })
  })

  it('короткий кусок у конца трека — сдвигается назад, конец = длина трека', () => {
    expect(timbreWindow({ from: 198.5, to: 199.5 }, 200)).toEqual({ from: 197, to: 200 })
  })

  it('растянутый кусок ровно до конца трека — не сдвигается', () => {
    expect(timbreWindow({ from: 197, to: 198 }, 200)).toEqual({ from: 197, to: 200 })
  })

  it('длина трека неизвестна (0 или undefined) — верхней границы нет', () => {
    expect(timbreWindow({ from: 198.5, to: 199 }, 0)).toEqual({ from: 198.5, to: 201.5 })
    expect(timbreWindow({ from: 198.5, to: 199 }, undefined)).toEqual({ from: 198.5, to: 201.5 })
    expect(timbreWindow({ from: 100, to: 500 }, 0)).toEqual({ from: 100, to: 130 })
  })

  it('отрицательное начало считается нулём до остальных правил', () => {
    expect(timbreWindow({ from: -5, to: 10 }, 200)).toEqual({ from: 0, to: 10 })
    // после обрезки до 0 кусок короче 3 с — растягивается
    expect(timbreWindow({ from: -5, to: 1 }, 200)).toEqual({ from: 0, to: 3 })
    // после обрезки до 0 кусок длиннее 30 с — режется
    expect(timbreWindow({ from: -10, to: 35 }, 200)).toEqual({ from: 0, to: 30 })
  })

  it('трек короче 3 с — весь трек', () => {
    expect(timbreWindow({ from: 0.5, to: 1 }, 2)).toEqual({ from: 0, to: 2 })
    expect(timbreWindow({ from: 0, to: 2 }, 2)).toEqual({ from: 0, to: 2 })
  })

  it('вход не меняется, возвращается новый объект', () => {
    const cases = [
      [{ from: 10, to: 20 }, 200],
      [{ from: 15, to: 120 }, 200],
      [{ from: 198.5, to: 199.5 }, 200],
      [{ from: -5, to: 1 }, 200],
      [{ from: 0.5, to: 1 }, 2],
    ]
    for (const [w, dur] of cases) {
      const snap = { ...w }
      const out = timbreWindow(w, dur)
      expect(out).not.toBe(w)
      expect(w).toEqual(snap)
    }
  })
})
