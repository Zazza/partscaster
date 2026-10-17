import { describe, it, expect } from 'vitest'
import { withPending, removeRecord, applyWindow } from './trackDesk.js'
import { withStemPlace } from './mixDesk.js'
import { withEnvelope } from './envelope.js'

// «▶ стало» до «Применить»: черновик правок так, как если бы несохранённую
// правку уже применили. Проверяем по условиям задачи, не по коду.

const snap = a => JSON.parse(JSON.stringify(a))
// withStemPlace выдаёт новой записи места свежий childId (от времени) — два вызова
// не равны побайтно; сравниваем без этого поля.
const noChild = list => list.map(({ childId: _childId, ...rest }) => rest)

// Черновик с разными записями: место гитары, громкость баса, «убрать» голоса.
const draft = () => [
  { instId: 'vol', stems: ['bass'], db: -6, step: 'sound', from: 0, to: 0 },
  { stems: ['guitar'], place: { pan: -0.5, width: 1 } },
  { instId: 'remove', stems: ['vocals'], db: -100, step: 'sound', from: 3, to: 5 },
]

describe('removeRecord', () => {
  it('с выделением — окно выделения', () => {
    const sel = { from: 12, to: 20.5 }
    expect(removeRecord('drums', sel)).toEqual({
      instId: 'remove', stems: ['drums'], db: -100, step: 'sound', ...applyWindow(sel),
    })
    expect(removeRecord('drums', sel)).toMatchObject({ from: 12, to: 20.5 })
  })

  it('без выделения — весь трек, как applyWindow(null)', () => {
    const w = applyWindow(null)
    expect(removeRecord('vocals', null)).toEqual({
      instId: 'remove', stems: ['vocals'], db: -100, step: 'sound', from: w.from, to: w.to,
    })
  })
})

describe('withPending', () => {
  it('без правок — равный список, новый массив, вход не тронут', () => {
    for (const pend of [null, undefined, {}]) {
      const src = draft()
      const before = snap(src)
      const out = withPending(src, 'bass', pend, null)
      expect(out).toEqual(before)
      expect(out).not.toBe(src)
      expect(src).toEqual(before)
    }
  })

  it('пустой список null/undefined — []', () => {
    expect(withPending(null, 'bass', null, null)).toEqual([])
    expect(withPending(undefined, 'bass', {}, null)).toEqual([])
  })

  it('place — как withStemPlace (замена места своей дорожки)', () => {
    const place = { pan: 0.7, width: 1.4 }
    const src = draft()
    const before = snap(src)
    const out = withPending(src, 'guitar', { place }, null)
    expect(noChild(out)).toEqual(noChild(withStemPlace(draft(), 'guitar', place)))
    expect(src).toEqual(before)
  })

  it('place — как withStemPlace (дорожка без места: запись добавляется)', () => {
    const place = { pan: -1, width: 0.5 }
    const src = draft()
    const before = snap(src)
    const out = withPending(src, 'piano', { place }, null)
    expect(noChild(out)).toEqual(noChild(withStemPlace(draft(), 'piano', place)))
    expect(src).toEqual(before)
  })

  it('remove — ровно одна запись removeRecord в конце', () => {
    const sel = { from: 4, to: 9 }
    const src = draft()
    const before = snap(src)
    const out = withPending(src, 'bass', { remove: true }, sel)
    expect(out).toHaveLength(before.length + 1)
    expect(out.slice(0, -1)).toEqual(before)
    expect(out[out.length - 1]).toEqual(removeRecord('bass', sel))
    expect(src).toEqual(before)
  })

  it('remove без выделения — на весь трек', () => {
    const out = withPending([], 'drums', { remove: true }, null)
    expect(out).toEqual([removeRecord('drums', null)])
  })

  it('remove: false — ничего не добавляется', () => {
    const out = withPending(draft(), 'bass', { remove: false }, null)
    expect(out).toEqual(draft())
  })

  it('place и remove — сначала место, «убрать» последней записью', () => {
    const place = { pan: 0.3, width: 1 }
    const sel = { from: 1, to: 2 }
    const src = draft()
    const before = snap(src)
    const out = withPending(src, 'guitar', { place, remove: true }, sel)
    expect(noChild(out)).toEqual(noChild([...withStemPlace(draft(), 'guitar', place), removeRecord('guitar', sel)]))
    expect(src).toEqual(before)
  })
})

// новая линия — обычная запись черновика: окно 0–0 (весь трек), db 0, childId 0 (не вклейка)
describe('withEnvelope', () => {
  const isEnv = it => Array.isArray(it.envelope)
  const envDraft = () => [
    { instId: 'env', stems: ['bass'], envelope: [{ t: 0, db: -3 }, { t: 5, db: 2 }] },
    { instId: 'env', stems: ['drums'], envelope: [{ t: 1, db: -6 }] },
    { instId: 'env', stems: ['mix'], mix: true, envelope: [{ t: 0, db: 4 }] },
    { instId: 'vol', stems: ['bass'], db: -6, step: 'sound', from: 0, to: 0 },
  ]

  it('дорожка: прежняя линия этой дорожки заменяется, остальное на месте', () => {
    const src = envDraft()
    const before = snap(src)
    const pts = [{ t: 0, db: -10 }, { t: 8, db: 3 }]
    const out = withEnvelope(src, 'bass', pts)
    const bassEnv = out.filter(it => isEnv(it) && !it.mix && it.stems[0] === 'bass')
    expect(bassEnv).toEqual([{ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['bass'], envelope: pts }])
    expect(bassEnv[0]).not.toHaveProperty('mix')
    expect(out[out.length - 1]).toEqual({ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['bass'], envelope: pts })
    // чужие линии, линия на весь трек и не-линия той же дорожки остались
    expect(out).toContainEqual(before[1])
    expect(out).toContainEqual(before[2])
    expect(out).toContainEqual(before[3])
    expect(out).toHaveLength(4)
    expect(src).toEqual(before)
  })

  it('дорожка без прежней линии — линия добавляется', () => {
    const pts = [{ t: 2, db: -4 }]
    const out = withEnvelope(envDraft(), 'guitar', pts)
    expect(out).toHaveLength(5)
    expect(out.slice(0, 4)).toEqual(envDraft())
    expect(out[4]).toEqual({ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['guitar'], envelope: pts })
  })

  it('в точках только t и db', () => {
    const pts = [{ t: 0, db: -2, sel: true, id: 7 }, { t: 4, db: 1, x: 'y' }]
    const out = withEnvelope([], 'bass', pts)
    expect(out).toEqual([{ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['bass'], envelope: [{ t: 0, db: -2 }, { t: 4, db: 1 }] }])
  })

  it('весь трек: прежняя линия mix заменяется, линии дорожек на месте', () => {
    const src = envDraft()
    const before = snap(src)
    const pts = [{ t: 0, db: -5 }, { t: 6, db: 0 }]
    const out = withEnvelope(src, '', pts)
    const mixEnv = out.filter(it => isEnv(it) && it.mix)
    expect(mixEnv).toEqual([{ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['mix'], mix: true, envelope: pts }])
    expect(out[out.length - 1]).toEqual({ childId: 0, from: 0, to: 0, db: 0, instId: 'env', stems: ['mix'], mix: true, envelope: pts })
    expect(out).toContainEqual(before[0])
    expect(out).toContainEqual(before[1])
    expect(out).toContainEqual(before[3])
    expect(out).toHaveLength(4)
    expect(src).toEqual(before)
  })

  it('пустая линия у дорожки — только убрать прежнюю', () => {
    for (const pts of [[], null]) {
      const src = envDraft()
      const before = snap(src)
      const out = withEnvelope(src, 'bass', pts)
      expect(out).toEqual([before[1], before[2], before[3]])
      expect(src).toEqual(before)
    }
  })

  it('пустая линия на весь трек — только убрать прежнюю mix', () => {
    for (const pts of [[], null]) {
      const out = withEnvelope(envDraft(), '', pts)
      const d = envDraft()
      expect(out).toEqual([d[0], d[1], d[3]])
    }
  })

  it('ровная линия на весь трек = нет линии: только убрать', () => {
    const src = envDraft()
    const before = snap(src)
    const out = withEnvelope(src, '', [{ t: 0, db: 0 }, { t: 10, db: 0 }])
    expect(out).toEqual([before[0], before[1], before[3]])
    expect(src).toEqual(before)
  })

  it('пустой черновик и пустая линия — пусто', () => {
    expect(withEnvelope([], 'bass', [])).toEqual([])
    expect(withEnvelope([], '', [])).toEqual([])
  })
})
