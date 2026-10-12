// Умолчание rel_db синта −8 — блок synth в описании блоков, студия «Синт по аккордам»
// (запасное значение ползунка, когда в цепочке нет блока synth) и подписи; у perc −14.
import { describe, it, expect } from 'vitest'
import BLOCKS from './fxBlocks.json'
import ru from './i18n/ru.js'
import en from './i18n/en.js'
import { synthLevel, SYNTH_REL_DB_DEFAULT } from './synthPart.js'

const def = (type, id) => (BLOCKS[type].params.find((p) => p.id === id) || {}).default

describe('умолчание rel_db синта −8', () => {
  it('fxBlocks: synth rel_db по умолчанию −8', () => {
    expect(def('synth', 'rel_db')).toBe(-8)
  })

  it('fxBlocks: perc rel_db по умолчанию прежний −14', () => {
    expect(def('perc', 'rel_db')).toBe(-14)
  })

  // StudioSynth берёт громкость партии через synthLevel (ползунок «Громкость»)
  it('synthLevel: нет блока synth — −8, как умолчание блока; есть — rel_db блока', () => {
    expect(synthLevel([])).toBe(-8)
    expect(synthLevel([{ type: 'eq', params: {} }])).toBe(-8)
    expect(SYNTH_REL_DB_DEFAULT).toBe(def('synth', 'rel_db'))
    expect(synthLevel([{ type: 'eq', params: {} }, { type: 'synth', params: { rel_db: -3 } }])).toBe(-3)
  })

  it.each([['ru', ru], ['en', en]])('%s: подпись громкости синта — про −8, не −6', (_l, dict) => {
    const tip = dict['synth.level.tip']
    expect(tip).toBeDefined()
    expect(tip).toMatch(/[−-]8/)
    expect(tip).not.toMatch(/[−-]6(?!\d)/)
  })
})
