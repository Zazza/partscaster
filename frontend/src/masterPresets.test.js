// Готовые мастера и место по умолчанию у готовых партий в fxPresets.js.
//   мастера — stems ['master']: «Стриминг −14 LUFS», «Громко −11 LUFS» (последний блок limiter,
//   target_lufs −14 / −11), «Мягкая склейка» — без громкости (без limiter);
//   перкуссии хэт/шейкер/бубен/ковбелл/райд — place.pan от центра (0,2 ≤ |pan| ≤ 0,4);
//   бочка/хлопки/римшот — центр; пэды — place.width 1,5.
// Название мастера ищется по вхождению (без учёта регистра): приставка вроде «Мастер: » допустима.
// Цепочки мастеров проверяются по описанию блоков (fxBlocks.json — копия worker/fx_blocks.json).
import { describe, it, expect } from 'vitest'
import blocks from './fxBlocks.json'
import { fxPresets } from './fxPresets.js'

const masters = fxPresets.filter((p) => (p.stems || []).includes('master'))
const NAMES = ['Стриминг −14 LUFS', 'Громко −11 LUFS', 'Мягкая склейка']
const byName = (name) => masters.filter((p) => p.name.ru.toLowerCase().includes(name.toLowerCase()))

function inRange(v, spec) {
  if (typeof v !== 'number' || !Number.isFinite(v)) return false
  if (spec.zero_off && v === 0) return true
  return v >= spec.min && v <= spec.max
}

describe('готовые мастера', () => {
  it('три мастера со stems [master], по одному на каждое название', () => {
    expect(masters.length).toBe(3)
    for (const p of masters) expect(p.stems, p.id).toEqual(['master'])
    for (const n of NAMES) expect(byName(n).length, n).toBe(1)
  })

  it.each([['Стриминг −14 LUFS', -14], ['Громко −11 LUFS', -11]])('%s: последний блок limiter с target %d',
    (name, target) => {
      const [p] = byName(name)
      expect(p).toBeDefined()
      const last = p.chain[p.chain.length - 1]
      expect(last.type).toBe('limiter')
      expect(last.target_lufs).toBe(target)
    })

  it('«Мягкая склейка» — без limiter (громкость не трогает)', () => {
    const [p] = byName('Мягкая склейка')
    expect(p).toBeDefined()
    expect(p.chain.some((b) => b.type === 'limiter')).toBe(false)
    expect(p.chain.length).toBeGreaterThan(0)
  })

  it('цепочка каждого мастера проходит описание блоков (сообщение — «id: что не так»)', () => {
    const bad = masters.flatMap((p) => {
      const out = p.chain.length <= 16 ? [] : [`${p.id}: блоков ${p.chain.length}`]
      for (const b of p.chain) {
        const spec = blocks[b.type]
        if (!spec) { out.push(`${p.id}: блок ${b.type}`); continue }
        for (const [k, v] of Object.entries(b)) {
          if (k === 'type') continue
          const ps = spec.params.find((x) => x.id === k)
          if (!ps) out.push(`${p.id}: ${b.type}.${k} нет в описании`)
          else if (typeof v === 'number' && !inRange(v, ps)) out.push(`${p.id}: ${b.type}.${k}=${v}`)
        }
      }
      return out
    })
    expect(bad).toEqual([])
  })
})

describe('место готовых партий по умолчанию', () => {
  const percs = fxPresets.filter((p) => (p.stems || []).includes('perc'))
  const pads = fxPresets.filter((p) => (p.stems || []).includes('synth') && p.style === 'pad')
  const SIDE = /хэт|шейкер|бубен|ковбелл|райд/i
  const CENTER = /бочк|хлопк|римшот/i

  it('перкуссии со сдвигом есть (хэт, шейкер, бубен, ковбелл, райд)', () => {
    expect(percs.filter((p) => SIDE.test(p.name.ru)).length).toBeGreaterThanOrEqual(5)
  })

  it('хэт, шейкер, бубен, ковбелл, райд — сдвиг от центра 0,2…0,4 (сообщение — нарушители)', () => {
    const bad = percs.filter((p) => SIDE.test(p.name.ru))
      .filter((p) => !(Math.abs(p.place?.pan ?? 0) >= 0.2 && Math.abs(p.place?.pan ?? 0) <= 0.4))
      .map((p) => `${p.name.ru}: pan ${p.place?.pan}`)
    expect(bad).toEqual([])
  })

  it('бочка, хлопки, римшот — в центре', () => {
    expect(percs.filter((p) => CENTER.test(p.name.ru) && (p.place?.pan ?? 0) !== 0)
      .map((p) => `${p.name.ru}: pan ${p.place.pan}`)).toEqual([])
  })

  it('каждый пэд — ширина 1,5', () => {
    expect(pads.filter((p) => p.place?.width !== 1.5).map((p) => `${p.id}: width ${p.place?.width}`)).toEqual([])
  })
})
