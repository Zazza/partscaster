// На странице «Инструменты» при выключенной
// обработке (bypass) наборы докачиваются только для блоков synth/perc — их и играет воркер; наборы прочих
// блоков (sampler и т.п.) в этом режиме не нужны и не качаются. kitChain(chain, bypass) из engineRun.js —
// цепочка, которую страница передаёт в ensureKits перед расчётом фразы.
import { describe, it, expect } from 'vitest'
import { kitChain } from './engineRun.js'

const chain = [
  { type: 'synth', kit: 'vsco-violin/ens' },
  { type: 'sampler', kit: 'osdk/kick' },
  { type: 'perc', kit: 'osdk/hh-closed' },
  { type: 'amp', model: '' },
  { type: 'bass', kit: 'growlybass/bass' },
]

describe('kitChain: bypass — наборы только для synth/perc', () => {
  it('без обработки — только synth и perc, в порядке цепочки', () => {
    expect(kitChain(chain, true).map((b) => b.type)).toEqual(['synth', 'perc'])
  })

  it('с обработкой — вся цепочка (наборы эффектов тоже нужны)', () => {
    expect(kitChain(chain, false)).toEqual(chain)
  })

  it('без synth/perc и bypass — качать нечего; вход не меняется', () => {
    const fx = Object.freeze([{ type: 'sampler', kit: 'a/b' }, { type: 'eq' }])
    expect(kitChain(fx, true)).toEqual([])
    expect(fx).toHaveLength(2)
  })
})
