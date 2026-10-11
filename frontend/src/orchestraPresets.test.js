// Тесты карточки internal-own-track, этап 14 «Оркестр»: условия 113–114 (тест-кейс ТК143).
//   113 — готовые инструменты оркестра в fxPresets: группы orch-strings / orch-winds / orch-brass / orch-mallets
//     (партии по аккордам: stems ['synth'], блок synth с kit VSCO, стиль pad|arp|pulse|drone, октава),
//     orch-bass (stems ['bass'], блок bass с kit — виолончель, альт, контрабас, фагот, туба),
//     orch-lead (stems ['guitar','other','vocals'], блок bass с fmin_hz 80 / fmax_hz 1500 — скрипка, флейта,
//     гобой, кларнет, труба); без имён групп и исполнителей. Наборы частей — из условия 110.
//   114 — familyOf: stems из guitar/other/piano/vocals (хотя бы одна не vocals) → guitar; только vocals → null.
// Пресеты опознаются по полю group, а не по id. Написаны по карточке, без чтения реализации.
import { describe, it, expect } from 'vitest'
import { fxPresets } from './fxPresets.js'
import { PRESET_GROUPS, groupPresets } from './presetGroups.js'
import { familyOf } from './phraseLoop.js'

// условие 110: наборы VSCO и их части
const WIND_PARTS = ['sus', 'stac']
const KIT_PARTS = {
  'vsco-violin': ['solo', 'ens', 'pizz', 'spic'],
  'vsco-viola': ['ens', 'pizz', 'spic'],
  'vsco-cello': ['ens', 'pizz', 'spic'],
  'vsco-contrabass': ['sus', 'pizz', 'spic'],
  'vsco-harp': ['harp'],
  'vsco-flute': WIND_PARTS,
  'vsco-oboe': WIND_PARTS,
  'vsco-clarinet': WIND_PARTS,
  'vsco-bassoon': WIND_PARTS,
  'vsco-trumpet': WIND_PARTS,
  'vsco-horn': WIND_PARTS,
  'vsco-trombone': WIND_PARTS,
  'vsco-tuba': WIND_PARTS,
  'vsco-mallets': ['glock', 'marimba', 'xylo'],
}

// какие инструменты уместны в группе: для аккордовых групп — по смыслу названия группы («Струнные», «Медь» …);
// для orch-bass / orch-lead — перечень из условия 113 (проверяется, что он покрыт; лишние инструменты допустимы:
// условие перечисляет, но не ограничивает — см. отчёт теста)
const GROUP_KITS = {
  'orch-strings': ['vsco-violin', 'vsco-viola', 'vsco-cello', 'vsco-contrabass', 'vsco-harp'],
  'orch-winds': ['vsco-flute', 'vsco-oboe', 'vsco-clarinet', 'vsco-bassoon'],
  'orch-brass': ['vsco-trumpet', 'vsco-horn', 'vsco-trombone', 'vsco-tuba'],
  'orch-mallets': ['vsco-mallets'],
  'orch-bass': ['vsco-cello', 'vsco-viola', 'vsco-contrabass', 'vsco-bassoon', 'vsco-tuba'],
  'orch-lead': ['vsco-violin', 'vsco-flute', 'vsco-oboe', 'vsco-clarinet', 'vsco-trumpet'],
}
const CHORD_GROUPS = ['orch-strings', 'orch-winds', 'orch-brass', 'orch-mallets']
const ORCH_GROUPS = [...CHORD_GROUPS, 'orch-bass', 'orch-lead']
const STYLES = ['pad', 'arp', 'pulse', 'drone']

const inGroup = (g) => fxPresets.filter((p) => p.group === g)
const orch = fxPresets.filter((p) => typeof p.group === 'string' && p.group.startsWith('orch-'))
const kitBlocks = (p) => p.chain.filter((b) => (b.type === 'synth' || b.type === 'bass') && typeof b.kit === 'string')
const instrumentOf = (kit) => kit.split('/')[0]

// Латинское слово с заглавной вне начала предложения — подозрение на имя исполнителя/группы/бренда.
// Разрешены: источник сэмплов (VSCO-2 Community Edition), лицензия CC0, названия нот (A, C#4, Bb2 …).
// «Christmas…» — праздник, не имя (ложное срабатывание на «Christmassy» в заметке колокольчиков).
const ALLOWED_CAPS = new Set(['VSCO', 'VSCO-2', 'Community', 'Edition', 'CC0', 'Christmas', 'Christmassy'])
const NOTE_RE = /^[A-G](#|b)?-?\d?$/
function suspiciousNames(text) {
  const cyr = /[а-яё]/i.test(text)
  const found = []
  for (const m of text.matchAll(/[A-Z][A-Za-z0-9#-]*/g)) {
    const word = m[0]
    if (m.index > 0 && /[A-Za-z0-9]/.test(text[m.index - 1])) continue // не начало слова
    if (ALLOWED_CAPS.has(word) || NOTE_RE.test(word)) continue
    const before = text.slice(0, m.index).trimEnd()
    const sentenceStart = before === '' || /[.!?:;(«"—→]$/.test(before)
    // в английском тексте заглавная в начале предложения — норма; в русском латиница с заглавной подозрительна всегда
    if (sentenceStart && !cyr) continue
    found.push(word)
  }
  return found
}

// правило — один тест: список нарушителей «id: причина»
const violations = (list, check) => list.flatMap((p) => check(p).map((why) => `${p.id ?? p}: ${why}`))

describe('условие 113: группы оркестра (ТК143)', () => {
  it('у каждой группы оркестра есть подписи ru и en в PRESET_GROUPS', () => {
    expect(violations(ORCH_GROUPS, (g) => ['ru', 'en'].filter((l) => !PRESET_GROUPS[g]?.[l]?.trim()).map((l) => `нет ${l}`)))
      .toEqual([])
  })

  it('в каждой группе оркестра не меньше трёх пресетов', () => {
    expect(violations(ORCH_GROUPS, (g) => (inGroup(g).length >= 3 ? [] : [`пресетов ${inGroup(g).length}`]))).toEqual([])
  })

  it('groupPresets выводит все группы оркестра с подписью из PRESET_GROUPS', () => {
    const out = JSON.stringify(groupPresets(fxPresets, 'ru'))
    for (const g of ORCH_GROUPS) expect(out).toContain(PRESET_GROUPS[g].ru)
  })

  it('других групп orch-* нет (все — из перечня условия)', () => {
    expect([...new Set(orch.map((p) => p.group))].sort()).toEqual([...ORCH_GROUPS].sort())
  })
})

// «имена ru/en непустые» у пресетов оркестра — правило fxPresets.test.js «name и note — ru и en, непустые» по всем пресетам
describe('условие 113: пресеты групп оркестра (ТК143)', () => {
  it('каждый: блок synth/bass с kit vsco-<инструмент>/<часть> из условия 110, инструмент — по смыслу группы', () => {
    expect(violations(orch, (p) => {
      const kb = kitBlocks(p)
      if (kb.length === 0) return ['нет блока synth/bass с kit']
      return kb.flatMap((b) => {
        if (!/^vsco-[a-z]+\/[a-z]+$/.test(b.kit)) return [`kit ${b.kit}`]
        const [inst, part] = b.kit.split('/')
        if (!(inst in KIT_PARTS)) return [`инструмент ${inst}`]
        const out = KIT_PARTS[inst].includes(part) ? [] : [`часть ${b.kit}`]
        if (CHORD_GROUPS.includes(p.group) && !GROUP_KITS[p.group].includes(inst)) out.push(`${inst} не в ${p.group}`)
        return out
      })
    })).toEqual([])
  })

  it('каждый: в названии и заметке нет имён исполнителей/групп', () => {
    expect(violations(orch, (p) => [p.name?.ru, p.name?.en, p.note?.ru, p.note?.en].filter(Boolean)
      .flatMap((t) => suspiciousNames(t)))).toEqual([])
  })

  it('партии по аккордам: stems [synth], блок synth с kit vsco-*, стиль и октава −2…+2', () => {
    const chord = orch.filter((p) => CHORD_GROUPS.includes(p.group))
    expect(violations(chord, (p) => [
      ...(JSON.stringify(p.stems) === '["synth"]' ? [] : [`stems ${JSON.stringify(p.stems)}`]),
      ...(p.chain.some((b) => b.type === 'synth' && /^vsco-/.test(b.kit || '')) ? [] : ['нет synth с kit vsco-*']),
      ...(STYLES.includes(p.style) ? [] : [`style ${p.style}`]),
      ...(Number.isInteger(p.octave) && p.octave >= -2 && p.octave <= 2 ? [] : [`octave ${p.octave}`]),
    ])).toEqual([])
  })

  it('Бас → оркестр (orch-bass): stems [bass], блок bass с kit vsco-*', () => {
    expect(violations(inGroup('orch-bass'), (p) => [
      ...(JSON.stringify(p.stems) === '["bass"]' ? [] : [`stems ${JSON.stringify(p.stems)}`]),
      ...(p.chain.some((b) => b.type === 'bass' && /^vsco-/.test(b.kit || '')) ? [] : ['нет bass с kit vsco-*']),
    ])).toEqual([])
  })

  // диапазоны условия 112 (fmin 20…1000, fmax 100…2000) покрывает значение из условия 113: 80 / 1500
  it('Мелодия → оркестр (orch-lead): stems guitar/other/vocals, блок bass с fmin_hz 80 / fmax_hz 1500', () => {
    expect(violations(inGroup('orch-lead'), (p) => {
      const out = [...p.stems].sort().join() === 'guitar,other,vocals' ? [] : [`stems ${JSON.stringify(p.stems)}`]
      const b = p.chain.find((x) => x.type === 'bass' && /^vsco-/.test(x.kit || ''))
      if (!b) return [...out, 'нет bass с kit vsco-*']
      if (b.fmin_hz !== 80 || b.fmax_hz !== 1500) out.push(`fmin_hz ${b.fmin_hz}, fmax_hz ${b.fmax_hz}`)
      return out
    })).toEqual([])
  })
})

describe('условие 113: перечни инструментов из карточки покрыты', () => {
  it.each([
    ['orch-bass', GROUP_KITS['orch-bass']],
    ['orch-lead', GROUP_KITS['orch-lead']],
  ])('в %s есть пресет на каждый инструмент из условия', (g, kits) => {
    const used = new Set(inGroup(g).flatMap((p) => kitBlocks(p).map((b) => instrumentOf(b.kit))))
    for (const k of kits) expect(used).toContain(k)
  })
})

describe('проверка «нет имён» сама различает (самоконтроль фильтра)', () => {
  it('ловит латинское имя в русском тексте и посреди английского предложения', () => {
    expect(suspiciousNames('Струнные как у Metallica')).toEqual(['Metallica'])
    expect(suspiciousNames('Strings like Metallica live')).toEqual(['Metallica'])
  })
  it('не ловит VSCO/CC0, ноты и заглавную в начале английского предложения', () => {
    expect(suspiciousNames('Сэмплы VSCO-2 Community Edition, CC0; нота A4 и C#3.')).toEqual([])
    expect(suspiciousNames('Violins: chords. Samples VSCO-2 Community Edition, CC0.')).toEqual([])
  })
})

describe('условие 114: familyOf для мелодии-замены (ТК143)', () => {
  it.each([
    [['guitar', 'other', 'vocals'], 'guitar'],
    [['other', 'vocals'], 'guitar'],
    [['piano', 'vocals'], 'guitar'],
    [['vocals'], null],
  ])('stems %j → %s', (stems, fam) => {
    expect(familyOf({ id: 'x', stems, chain: [] })).toBe(fam)
  })

  it('пресеты orch-lead попадают в семью guitar', () => {
    for (const p of inGroup('orch-lead')) expect(familyOf(p)).toBe('guitar')
  })
})
