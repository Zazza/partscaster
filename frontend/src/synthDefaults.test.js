// Тесты карточки internal-own-track, условие 37 «Синт слышен» (тест-кейс ТК63):
// готовые пэды (style pad: Solina, Juno, CS-80, Farfisa, Vox) — octave 1 (на октаву выше гитар);
// Solina — cutoff_hz 6000 у блока synth. Умолчание rel_db синта (−8) — synthLevelDefault.test.js.
// Написаны по карточке, без чтения реализации.
import { describe, it, expect } from 'vitest'
import { fxPresets } from './fxPresets.js'

// оркестр (этап 14, orch-*) — регистр по настоящему инструменту (альты, виолончели, тромбоны ниже скрипок), не по
// правилу «пэд синта октавой выше гитар»; слышимость низких оркестровых пэдов — проверять на слух
const synths = fxPresets.filter((p) => (p.stems || []).includes('synth') && !String(p.group || '').startsWith('orch-'))
const pads = synths.filter((p) => p.style === 'pad')

describe('синт слышен: умолчания (ТК63)', () => {
  // этап 7в (усл. 64) добавил пэды (органы, клавиши) — прежние пять на месте, правило octave 1 — для всех
  it('пэды этапа 4 на месте (Solina, Juno, CS-80, Farfisa, Vox)', () => {
    const ids = pads.map((p) => p.id)
    for (const id of ['synth-solina', 'synth-juno', 'synth-cs80-brass', 'synth-farfisa', 'synth-vox-continental']) expect(ids).toContain(id)
  })

  it('каждый пэд — octave 1 (сообщение — id нарушителей)', () => {
    expect(pads.filter((p) => p.octave !== 1).map((p) => `${p.id}: octave ${p.octave}`)).toEqual([])
  })

  it('Solina — cutoff_hz 6000 у блока synth', () => {
    const solina = fxPresets.find((p) => p.id === 'synth-solina')
    expect(solina).toBeDefined()
    const synth = solina.chain.find((b) => b.type === 'synth')
    expect(synth).toBeDefined()
    expect(synth.cutoff_hz).toBe(6000)
  })
})
