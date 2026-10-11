// Условие 136 (internal-own-track, этап 17): fxChain.missingRequired — какие блоки цепочки не дают
// её применить: включённый блок с обязательной строкой без значения (усилитель NAM без захвата
// model, сэмплер/бас без набора kit — по fxBlocks.json, required). Ответ — номера блоков по порядку;
// выключенный блок не уходит воркеру и применению не мешает; необязательная строка (ir у cab) — тоже.
import { describe, it, expect } from 'vitest'
import blocks from './fxBlocks.json'
import { newBlock, missingRequired } from './fxChain.js'

const block = (type, params = {}, on = true) => {
  const b = newBlock(type, blocks)
  return { ...b, on, params: { ...b.params, ...params } }
}

// обязательные строки по описанию блоков — тест опирается на данные, а не на имена в коде
const required = Object.entries(blocks).flatMap(([type, s]) =>
  (s.strings || []).filter((p) => p.required).map((p) => [type, p.id]))

describe('обязательные строки блоков (missingRequired)', () => {
  it('в описании есть обязательные строки (иначе проверка ниже пустая)', () => {
    expect(required).toContainEqual(['amp', 'model'])
  })

  it('пустая цепочка — применять можно', () => {
    expect(missingRequired([], blocks)).toEqual([])
  })

  it('усилитель без захвата — его номер', () => {
    expect(missingRequired([block('gate'), block('amp'), block('cab')], blocks)).toEqual([1])
  })

  it('усилитель с захватом — применять можно', () => {
    expect(missingRequired([block('amp', { model: 'Fender Twin' })], blocks)).toEqual([])
  })

  it('выключенный усилитель без захвата не мешает', () => {
    expect(missingRequired([block('amp', {}, false), block('reverb')], blocks)).toEqual([])
  })

  it('необязательная строка без значения (импульс кабинета) не мешает', () => {
    const cabIr = (blocks.cab.strings || []).find((p) => p.id === 'ir')
    expect(cabIr && !cabIr.required).toBe(true)
    expect(missingRequired([block('cab', { ir: '' })], blocks)).toEqual([])
  })

  it.each(required)('блок %s без строки %s — его номер', (type, id) => {
    expect(missingRequired([block('gain'), block(type, { [id]: '' })], blocks)).toEqual([1])
  })

  it.each(required)('блок %s со строкой %s — применять можно', (type, id) => {
    expect(missingRequired([block(type, { [id]: 'x/y' })], blocks)).toEqual([])
  })

  it('несколько блоков без строк — все номера по порядку', () => {
    const chain = [block('amp'), block('eq'), block('amp', { model: 'ok' }), block('sampler'), block('amp')]
    expect(missingRequired(chain, blocks)).toEqual([0, 3, 4])
  })
})
