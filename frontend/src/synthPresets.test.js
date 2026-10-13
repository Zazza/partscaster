// Указание автора набора Salamander (Alexander Holm, CC-BY) — в подсказке пресета с этим набором.
import { describe, it, expect } from 'vitest'
import { fxPresets } from './fxPresets.js'
import { PRESET_GROUPS, FAMILIES, groupPresets } from './presetGroups.js'

// синт-бас — в семействе «Бас» (подвид bass-synth), остальные синты — в группах синтов и клавиш
const SYNTH_GROUPS = ['synth-pad', 'synth-organ', 'synth-keys', 'synth-lead', 'synth-toy', 'bass-synth']
const OLD_IDS = ['synth-solina', 'synth-juno', 'synth-moog-bass', 'synth-moog-lead', 'synth-cs80-brass',
  'synth-farfisa', 'synth-vox-continental', 'synth-vltone']

const synthBlock = (p) => p.chain.find((b) => b.type === 'synth')
const synths = fxPresets.filter((p) => JSON.stringify(p.stems) === JSON.stringify(['synth']) && synthBlock(p))
  // оркестр (группы orch-*) — партии по аккордам тем же блоком synth, но не синты: свои группы
  .filter((p) => !String(p.group || '').startsWith('orch-'))
const withKit = (kit) => synths.filter((p) => synthBlock(p).kit === kit)

describe('группы синтов', () => {
  it.each(SYNTH_GROUPS)('группа %s есть в PRESET_GROUPS с подписями ru/en', (g) => {
    expect(PRESET_GROUPS[g]).toBeDefined()
    expect(PRESET_GROUPS[g].ru).toBeTruthy()
    expect(PRESET_GROUPS[g].en).toBeTruthy()
  })

  it('не меньше 20 синтов', () => {
    expect(synths.length).toBeGreaterThanOrEqual(20)
  })

  it('у каждого синта group — одна из групп синтов (сообщение — id нарушителей)', () => {
    expect(synths.filter((p) => !SYNTH_GROUPS.includes(p.group)).map((p) => `${p.id}: ${p.group}`)).toEqual([])
  })

  it('groupPresets по синтам — все группы синтов, каждая непустая, подпись «семейство · подвид»', () => {
    const groups = groupPresets(synths)
    expect(groups.map((g) => g.group).sort()).toEqual([...SYNTH_GROUPS].sort())
    for (const g of groups) {
      expect(g.items.length).toBeGreaterThan(0)
      const pg = PRESET_GROUPS[g.group]
      expect(g.label).toBe(`${FAMILIES.find((f) => f.id === pg.family).ru} · ${pg.ru}`)
    }
  })

  it.each(OLD_IDS)('прежний синт %s на месте', (id) => {
    expect(synths.map((p) => p.id)).toContain(id)
  })
})

describe('состав синтов', () => {
  it.each(['salamander/piano', 'salamander/piano-soft'])('есть синт с kit %s', (kit) => {
    expect(withKit(kit).length).toBeGreaterThanOrEqual(1)
  })

  it('не меньше 4 синтов на FM-генераторе (osc1 5)', () => {
    expect(synths.filter((p) => synthBlock(p).osc1 === 5).length).toBeGreaterThanOrEqual(4)
  })

  it('есть орган с вращающимся динамиком: tremolo stereo > 0', () => {
    const leslie = synths.filter((p) => p.chain.some((b) => b.type === 'tremolo' && b.stereo > 0))
    expect(leslie.length).toBeGreaterThanOrEqual(1)
  })
})

describe('автор набора Salamander в подсказке пресета', () => {
  const pianos = synths.filter((p) => String(synthBlock(p).kit || '').startsWith('salamander/'))

  it('пресеты с набором salamander есть', () => {
    expect(pianos.length).toBeGreaterThanOrEqual(2)
  })

  it('у каждого note ru/en называет автора и лицензию (сообщение — «id note.язык»)', () => {
    const bad = pianos.flatMap((p) => ['ru', 'en']
      .filter((loc) => !/Alexander Holm/.test(p.note?.[loc] || '') || !/CC[- ]?BY/i.test(p.note?.[loc] || ''))
      .map((loc) => `${p.id} note.${loc}`))
    expect(bad).toEqual([])
  })
})
