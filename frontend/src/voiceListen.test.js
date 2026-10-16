// «4 Голос» — одна кнопка «прослушать кусок»: чистая функция выбора действия по признакам.
// Карточка internal-studio-draft2, условие 4, тест-кейс ТК6.
// voiceListenPlan({ voice, fx, probeReady, probeVoiceKey, curVoiceKey }) →
//   'render'   — голос выбран, готовой пробы для него нет: черновой рендер куска;
//   'probe'    — проба этого голоса готова (с эффектом или без): играет она, нового рендера нет;
//   'trackFx'  — голоса нет, эффект есть: кусок трека с эффектом на голосе, без рендера;
//   'original' — ничего не выбрано: оригинал трека с первого спетого такта.
import { describe, it, expect } from 'vitest'
import { voiceListenPlan, voiceFxApplied, voiceFxMatch } from './voiceListen.js'

const plan = (over) => voiceListenPlan({
  voice: null, fx: null, probeReady: false, probeVoiceKey: '', curVoiceKey: '', ...over,
})

describe('voiceListenPlan', () => {
  it("голос выбран, пробы нет → 'render'", () => {
    expect(plan({ voice: 'v1', curVoiceKey: 'v1' })).toBe('render')
  })

  it("голос выбран и эффект выбран, пробы нет → 'render'", () => {
    expect(plan({ voice: 'v1', fx: 'reverb-hall', curVoiceKey: 'v1' })).toBe('render')
  })

  it("проба этого голоса готова, эффекта нет → 'probe'", () => {
    expect(plan({ voice: 'v1', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('probe')
  })

  it("проба этого голоса готова, эффект выбран (в т.ч. сменён после пробы) → 'probe', не новый рендер", () => {
    expect(plan({ voice: 'v1', fx: 'reverb-hall', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('probe')
    expect(plan({ voice: 'v1', fx: 'voice-drive', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('probe')
  })

  it("проба готова, но для другого голоса (голос сменили) → 'render'", () => {
    expect(plan({ voice: 'v2', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: 'v2' })).toBe('render')
    expect(plan({ voice: 'v2', fx: 'reverb-hall', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: 'v2' })).toBe('render')
  })

  it("ключ пробы совпадает, но проба не готова → 'render'", () => {
    expect(plan({ voice: 'v1', probeReady: false, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('render')
  })

  it("голоса нет, эффект есть → 'trackFx'", () => {
    expect(plan({ fx: 'reverb-hall' })).toBe('trackFx')
  })

  it("голоса нет, эффект есть, осталась проба прежнего голоса → всё равно 'trackFx'", () => {
    expect(plan({ fx: 'reverb-hall', probeReady: true, probeVoiceKey: 'v1', curVoiceKey: '' })).toBe('trackFx')
  })

  it("ничего не выбрано → 'original'", () => {
    expect(plan({})).toBe('original')
  })

  it("ничего не выбрано, осталась проба прежнего голоса → 'original'", () => {
    expect(plan({ probeReady: true, probeVoiceKey: 'v1' })).toBe('original')
  })

  it('пустые значения — «не выбрано»: пустая строка и undefined', () => {
    expect(voiceListenPlan({ voice: '', fx: '' })).toBe('original')
    expect(voiceListenPlan({ voice: undefined, fx: 'reverb-hall' })).toBe('trackFx')
  })

  it('голос — объект (карточка или «своё»): решает наличие, а не тип', () => {
    expect(plan({ voice: { kind: 'own', text: 'male baritone' }, curVoiceKey: 'own:male baritone' })).toBe('render')
    expect(plan({ voice: { kind: 'card', id: 3 }, probeReady: true, probeVoiceKey: 'card:3', curVoiceKey: 'card:3' })).toBe('probe')
  })
})

describe('voiceListenPlan — проба рендерится (probePending)', () => {
  it("голос выбран, проба этим голосом рендерится → 'wait' (второй рендер не нужен)", () => {
    expect(plan({ voice: 'v1', probePending: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('wait')
    expect(plan({ voice: 'v1', fx: 'reverb-hall', probePending: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('wait')
  })

  it("рендерится проба другого голоса → 'render'", () => {
    expect(plan({ voice: 'v2', probePending: true, probeVoiceKey: 'v1', curVoiceKey: 'v2' })).toBe('render')
  })

  it("готовая проба важнее: probeReady и probePending → 'probe'", () => {
    expect(plan({ voice: 'v1', probeReady: true, probePending: true, probeVoiceKey: 'v1', curVoiceKey: 'v1' })).toBe('probe')
  })
})

// «тот же эффект на голос уже в черновике»: запись step 'voice', stems ['vocals'], та же chain, те же params
// (по значению) и то же окно from/to (нет окна = 0/0)
describe('voiceFxApplied', () => {
  const fx = { chain: 'reverb-hall', params: { wet: 0.3, size: 2 } }
  const rec = (over = {}) => ({ childId: -11, step: 'voice', stems: ['vocals'], chain: 'reverb-hall',
    params: { wet: 0.3, size: 2 }, from: 10, to: 20, ...over })
  const win = { from: 10, to: 20 }

  it('совпадение (params — другой объект, тот же по значению) → true', () => {
    expect(voiceFxApplied([rec()], { chain: 'reverb-hall', params: { size: 2, wet: 0.3 } }, win)).toBe(true)
  })

  it('совпадение среди других записей → true', () => {
    const other = { childId: -3, stems: ['drums'], db: -100, from: 0, to: 0 }
    expect(voiceFxApplied([other, rec()], fx, win)).toBe(true)
  })

  it('нет окна = 0/0: запись на весь трек и win null/undefined → true', () => {
    expect(voiceFxApplied([rec({ from: 0, to: 0 })], fx, null)).toBe(true)
    expect(voiceFxApplied([rec({ from: 0, to: 0 })], fx, undefined)).toBe(true)
    expect(voiceFxApplied([rec({ from: 0, to: 0 })], fx, { from: 0, to: 0 })).toBe(true)
  })

  it('другие ручки → false', () => {
    expect(voiceFxApplied([rec()], { chain: 'reverb-hall', params: { wet: 0.5, size: 2 } }, win)).toBe(false)
    expect(voiceFxApplied([rec()], { chain: 'reverb-hall', params: { wet: 0.3 } }, win)).toBe(false)
  })

  it('другая цепочка → false', () => {
    expect(voiceFxApplied([rec()], { chain: 'reverb-room', params: { wet: 0.3, size: 2 } }, win)).toBe(false)
  })

  it('другое окно → false', () => {
    expect(voiceFxApplied([rec()], fx, { from: 10, to: 30 })).toBe(false)
    expect(voiceFxApplied([rec()], fx, null)).toBe(false)
    expect(voiceFxApplied([rec({ from: 0, to: 0 })], fx, win)).toBe(false)
  })

  it("запись без step 'voice' (эффект «Звука» на голос) → false", () => {
    const noStep = rec()
    delete noStep.step
    expect(voiceFxApplied([noStep], fx, win)).toBe(false)
    expect(voiceFxApplied([rec({ step: 'sound' })], fx, win)).toBe(false)
  })

  it('запись step voice не на голос → false', () => {
    expect(voiceFxApplied([rec({ stems: ['other'] })], fx, win)).toBe(false)
  })

  it('пустой edits → false', () => {
    expect(voiceFxApplied([], fx, win)).toBe(false)
  })

  it('fx null → false', () => {
    expect(voiceFxApplied([rec()], null, win)).toBe(false)
  })

  it('после удаления записи из edits → false (кнопку снова можно нажать)', () => {
    const edits = [rec()]
    expect(voiceFxApplied(edits, fx, win)).toBe(true)
    const after = edits.filter((x) => x.childId !== -11)
    expect(voiceFxApplied(after, fx, win)).toBe(false)
  })
})

// voiceFxMatch — запись черновика с тем же эффектом на голос (в т.ч. выключенная) или null;
// voiceFxApplied — true только при совпавшей включённой записи (выключенную включит «Применить»)
describe('voiceFxMatch / выключенная запись', () => {
  const fx = { chain: 'reverb-hall', params: { wet: 0.3, size: 2 } }
  const win = { from: 10, to: 20 }
  const rec = (over = {}) => ({ childId: -11, step: 'voice', stems: ['vocals'], chain: 'reverb-hall',
    params: { wet: 0.3, size: 2 }, from: 10, to: 20, ...over })

  it('совпала включённая запись → она; applied true', () => {
    const r = rec()
    expect(voiceFxMatch([r], fx, win)).toEqual(r)
    expect(voiceFxMatch([r], fx, win).childId).toBe(-11)
    expect(voiceFxApplied([r], fx, win)).toBe(true)
  })

  it('совпала только выключенная (off: true) → она; applied false', () => {
    const r = rec({ off: true })
    const m = voiceFxMatch([r], fx, win)
    expect(m).not.toBeNull()
    expect(m.childId).toBe(-11)
    expect(m.off).toBe(true)
    expect(voiceFxApplied([r], fx, win)).toBe(false)
  })

  it('нет совпадения → null', () => {
    expect(voiceFxMatch([rec({ chain: 'reverb-room' })], fx, win)).toBeNull()
    expect(voiceFxMatch([], fx, win)).toBeNull()
    expect(voiceFxMatch([rec()], null, win)).toBeNull()
  })

  it('выключенная и включённая с тем же эффектом → applied true', () => {
    expect(voiceFxApplied([rec({ off: true }), rec({ childId: -12 })], fx, win)).toBe(true)
  })
})
