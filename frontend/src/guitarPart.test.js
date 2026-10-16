// Задача internal-guitar-plan-chords: гитарист вписывается по аккордам плана (фронт).
// Спецификация (карточка, условия 1, 2, 9, 11 и «Устройство (контракт)»: Go / MCP / фронт):
// - PRESET_GROUPS['guitar-notes'] — семейство guitar, подпись ru «Гитарист вписывается по аккордам»,
//   en «Guitarist fits in by chords»; подписи прежних гитарных подвидов оканчиваются на «— звук, игра та же»;
// - fxPresets: guitar-notes-nylon/-acoustic/-emily/-fsbs/-fuzz (id прежние) — группа guitar-notes, stems ['guitar'],
//   первый блок gtrnotes с набором (как раньше) и манерой style: нейлон 2 (перебор), полуакустика 0 (авто),
//   Emily 3 (бас + аккорд), FSBS 1 (бой), фузз 4 (квинты); старые 16 гитарных звуков и их id не меняются;
// - fxBlocks.json: gtrnotes — прежние строки kit/mute_kit и числа strum/spread_ms/output_db + style
//   (0…4, по умолчанию 0, шаг 1, целое); служебные поля ставит воркер — не параметры;
// - докачка наборов (ensureKits, kitChain) учитывает kit и mute_kit блока gtrnotes;
// - паритет: api.guitarPart(id, style) → биндинг YueGuitarPart(id, style|0); api.guitarNotes удалён.
import { describe, it, expect, afterEach } from 'vitest'
import { PRESET_GROUPS } from './presetGroups.js'
import { fxPresets } from './fxPresets.js'
import blocks from './fxBlocks.json'
import { ensureKits, kitChain } from './engineRun.js'
import { api } from './api.js'

const OLD_GUITAR_GROUPS = ['guitar-clean', 'guitar-drive', 'guitar-space', 'guitar-amp']
// старые 16 гитарных звуков (до задачи) — id не меняются
const OLD_GUITAR_IDS = [
  'guitar-amp-clean', 'guitar-amp-hot', 'guitar-amp-spring', 'guitar-clean', 'guitar-amp-highgain',
  'guitar-postpunk', 'guitar-jangle', 'guitar-surf', 'guitar-fuzz', 'guitar-industrial', 'guitar-symphonic',
  'guitar-altrock', 'guitar-shoegaze', 'guitar-ambient', 'guitar-tremolo', 'guitar-lofi',
]
// пресет → набор и набор глушения ('' — у набора нет глушения)
const NOTE_PRESETS = {
  'guitar-notes-nylon': { kit: 'nylonguitar/guitar', mute: '' },
  'guitar-notes-acoustic': { kit: 'shinyguitar/acoustic', mute: 'shinyguitar/mute' },
  'guitar-notes-emily': { kit: 'emilyguitar/notes', mute: 'emilyguitar/mute' },
  'guitar-notes-fsbs': { kit: 'fsbsguitar/guitar', mute: '' },
  'guitar-notes-fuzz': { kit: 'fsbsguitar/guitar', mute: '' },
}
// манера гитариста (условие 2): 0 авто, 1 бой, 2 перебор, 3 бас + аккорд, 4 квинты
const STYLE_OF = {
  'guitar-notes-nylon': 2,
  'guitar-notes-acoustic': 0,
  'guitar-notes-emily': 3,
  'guitar-notes-fsbs': 1,
  'guitar-notes-fuzz': 4,
}
const byId = (id) => fxPresets.find((p) => p.id === id)

describe('PRESET_GROUPS: разновидность «Гитарист вписывается по аккордам» (условие 11)', () => {
  it('guitar-notes — семейство guitar, ru «Гитарист вписывается по аккордам», en «Guitarist fits in by chords»', () => {
    const g = PRESET_GROUPS['guitar-notes']
    expect(g, 'нет группы guitar-notes').toBeTruthy()
    expect(g.family).toBe('guitar')
    expect(g.ru).toBe('Гитарист вписывается по аккордам')
    expect(g.en).toBe('Guitarist fits in by chords')
  })

  it.each(OLD_GUITAR_GROUPS)('прежний подвид %s — семейство guitar, ru оканчивается на «— звук, игра та же»', (id) => {
    const g = PRESET_GROUPS[id]
    expect(g, `нет группы ${id}`).toBeTruthy()
    expect(g.family).toBe('guitar')
    expect(g.ru.endsWith('— звук, игра та же'), `${id}.ru = «${g.ru}»`).toBe(true)
  })

  it('у новой разновидности подпись не «звук, игра та же» — она про другую игру', () => {
    expect(PRESET_GROUPS['guitar-notes'].ru).not.toMatch(/игра та же/)
  })
})

describe('fxPresets: старые гитарные звуки не меняются (условие 11)', () => {
  it('все 16 прежних id на месте и остаются в прежних подвидах', () => {
    for (const id of OLD_GUITAR_IDS) {
      const p = byId(id)
      expect(p, `пропал пресет ${id}`).toBeTruthy()
      expect(OLD_GUITAR_GROUPS, `${id}: группа ${p.group}`).toContain(p.group)
    }
  })

  it('прежние звуки не переигрывают ноты: в их цепочках нет gtrnotes', () => {
    for (const id of OLD_GUITAR_IDS) {
      expect(byId(id).chain.map((b) => b.type), id).not.toContain('gtrnotes')
    }
  })
})

describe('fxPresets: пять звуков гитариста по аккордам (условия 1, 2)', () => {
  it('в группе guitar-notes ровно пять пресетов: nylon, acoustic, emily, fsbs, fuzz', () => {
    const ids = fxPresets.filter((p) => p.group === 'guitar-notes').map((p) => p.id).sort()
    expect(ids).toEqual(Object.keys(NOTE_PRESETS).sort())
  })

  it.each(Object.keys(NOTE_PRESETS))('%s: группа guitar-notes, только дорожка guitar, name/note ru и en', (id) => {
    const p = byId(id)
    expect(p, `нет пресета ${id}`).toBeTruthy()
    expect(p.group).toBe('guitar-notes')
    expect(p.stems).toEqual(['guitar'])
    for (const f of ['name', 'note']) {
      for (const lang of ['ru', 'en']) {
        expect(typeof p[f]?.[lang], `${f}.${lang}`).toBe('string')
        expect(p[f][lang].trim(), `${f}.${lang}`).not.toBe('')
      }
    }
  })

  it.each(Object.entries(NOTE_PRESETS))('%s: первый блок gtrnotes с набором %j', (id, want) => {
    const first = byId(id).chain[0]
    expect(first.type).toBe('gtrnotes')
    expect(first.kit).toBe(want.kit)
    if (want.mute) expect(first.mute_kit).toBe(want.mute)
    else expect(first.mute_kit || '', 'у набора нет глушения — mute_kit не задаётся').toBe('')
  })

  it.each(Object.entries(STYLE_OF))('%s: манера style = %i', (id, style) => {
    const first = byId(id).chain[0]
    expect(first.type).toBe('gtrnotes')
    expect(first.style, `${id}: манера`).toBe(style)
  })

  it('gtrnotes в каждой цепочке ровно один', () => {
    for (const id of Object.keys(NOTE_PRESETS)) {
      expect(byId(id).chain.filter((b) => b.type === 'gtrnotes'), id).toHaveLength(1)
    }
  })

  it('фузз — после переигровки есть перегруз (drive); у чистой FSBS перегруза нет', () => {
    const fuzz = byId('guitar-notes-fuzz').chain.map((b) => b.type)
    expect(fuzz.indexOf('drive'), 'в фуззе нет drive').toBeGreaterThan(0)
    expect(byId('guitar-notes-fsbs').chain.map((b) => b.type)).not.toContain('drive')
  })

  it('пять звуков различаются цепочками (а не только названием)', () => {
    const chains = Object.keys(NOTE_PRESETS).map((id) => JSON.stringify(byId(id).chain))
    expect(new Set(chains).size).toBe(5)
  })
})

describe('fxBlocks.json: блок gtrnotes («Гитарист по аккордам»)', () => {
  const b = blocks.gtrnotes
  const str = (id) => (b?.strings || []).find((s) => s.id === id)
  const par = (id) => (b?.params || []).find((p) => p.id === id)

  it('блок описан, подписи ru «Гитарист по аккордам» и en непустая, про chords', () => {
    expect(b, 'нет блока gtrnotes').toBeTruthy()
    expect(b.label.ru).toBe('Гитарист по аккордам')
    expect(String(b.label.en || '').trim()).not.toBe('')
    expect(String(b.label.en || '')).toMatch(/chords/i)
  })

  it('kit — набор, обязательный; mute_kit — набор, необязательный, по умолчанию ""', () => {
    expect(str('kit')).toMatchObject({ asset: 'kit', required: true })
    expect(str('mute_kit')).toMatchObject({ asset: 'kit', required: false, default: '' })
  })

  it('strum 0/1 целое по умолчанию 1; spread_ms 2…25 по умолчанию 8; output_db −24…12 по умолчанию 0', () => {
    expect(par('strum')).toMatchObject({ min: 0, max: 1, default: 1, integer: true })
    expect(par('spread_ms')).toMatchObject({ min: 2, max: 25, default: 8 })
    expect(par('output_db')).toMatchObject({ min: -24, max: 12, default: 0 })
  })

  it('style — манера 0…4, по умолчанию 0 (авто), шаг 1, целое', () => {
    expect(par('style')).toMatchObject({ min: 0, max: 4, default: 0, step: 1, integer: true })
  })

  it('у параметров и строк есть подписи ru/en', () => {
    for (const x of [...(b.params || []), ...(b.strings || [])]) {
      expect(String(x.label?.ru || '').trim(), `${x.id}.ru`).not.toBe('')
      expect(String(x.label?.en || '').trim(), `${x.id}.en`).not.toBe('')
    }
  })

  it('служебные поля воркера (notes, _t0, _until, _ref_rms, _tune, _beats) — не параметры пользователя', () => {
    const ids = [...(b.params || []), ...(b.strings || [])].map((x) => x.id)
    for (const id of ['notes', '_t0', '_until', '_ref_rms', '_tune', '_beats']) expect(ids).not.toContain(id)
  })
})

// Фейк api для докачки: наборы ставятся по одному, журнал установок.
function kitApi() {
  const installs = []
  return {
    installs,
    api: {
      async installFxKit(name) {
        installs.push(name)
        return { name, downloaded: true }
      },
    },
  }
}
const kit = (name) => ({ name, samples: 3 })

describe('докачка наборов учитывает блок gtrnotes и mute_kit (контракт: engineRun)', () => {
  it('набора kit нет → ставится', async () => {
    const f = kitApi()
    await ensureKits(f.api, [{ type: 'gtrnotes', kit: 'nylonguitar/guitar', mute_kit: '' }], [kit('osdk/kick')])
    expect(f.installs).toEqual(['nylonguitar'])
  })

  it('kit есть, mute_kit из другого набора нет → ставится набор mute_kit', async () => {
    const f = kitApi()
    const chain = [{ type: 'gtrnotes', kit: 'fsbsguitar/guitar', mute_kit: 'emilyguitar/mute' }]
    await ensureKits(f.api, chain, [kit('fsbsguitar/guitar')])
    expect(f.installs).toEqual(['emilyguitar'])
  })

  it('kit есть, части глушения того же набора нет → набор ставится (один раз)', async () => {
    const f = kitApi()
    const chain = [{ type: 'gtrnotes', kit: 'emilyguitar/notes', mute_kit: 'emilyguitar/mute' }]
    await ensureKits(f.api, chain, [kit('emilyguitar/notes')])
    expect(f.installs).toEqual(['emilyguitar'])
  })

  it('нет ни kit, ни mute_kit одного набора → набор ставится один раз', async () => {
    const f = kitApi()
    const chain = [{ type: 'gtrnotes', kit: 'shinyguitar/acoustic', mute_kit: 'shinyguitar/mute' }]
    await ensureKits(f.api, chain, [])
    expect(f.installs).toEqual(['shinyguitar'])
  })

  it('всё стоит → ничего не ставится; пустой mute_kit — не набор', async () => {
    const f = kitApi()
    await ensureKits(f.api, [
      { type: 'gtrnotes', kit: 'emilyguitar/notes', mute_kit: 'emilyguitar/mute' },
      { type: 'gtrnotes', kit: 'fsbsguitar/guitar', mute_kit: '' },
    ], [kit('emilyguitar/notes'), kit('emilyguitar/mute'), kit('fsbsguitar/guitar')])
    expect(f.installs).toEqual([])
  })

  it.each(Object.entries(NOTE_PRESETS))('готовый %s на пустом воркере → ставятся все его наборы', async (id, want) => {
    const f = kitApi()
    await ensureKits(f.api, byId(id).chain, [])
    const need = [...new Set([want.kit, want.mute].filter(Boolean).map((k) => k.split('/')[0]))]
    expect(f.installs).toEqual(need)
  })

  it('без обработки (bypass) gtrnotes остаётся в цепочке докачки — он сам играет ноты фразы', () => {
    const chain = [{ type: 'gtrnotes', kit: 'emilyguitar/notes', mute_kit: 'emilyguitar/mute' },
      { type: 'sampler', kit: 'osdk/kick' }, { type: 'drive', gain_db: 20 }]
    expect(kitChain(chain, true).map((b) => b.type)).toEqual(['gtrnotes'])
  })
})

describe('api.guitarPart → биндинг YueGuitarPart (условие 9, паритет)', () => {
  const saved = globalThis.window
  afterEach(() => {
    globalThis.window = saved
  })

  const bind = (fn) => {
    globalThis.window = { go: { main: { App: { YueGuitarPart: fn } } } }
  }

  it('передаёт id и манеру и отдаёт ответ биндинга', async () => {
    const calls = []
    const resp = { style: 2, bars: [{ start: 0.5, end: 2.5, chord: 'G', section: 'verse' }],
      strikes: [{ t: 0.52, midi: [43] }], beats: [0.5, 1.0], tune_cents: 0 }
    bind((...a) => { calls.push(a); return Promise.resolve(resp) })
    expect(typeof api.guitarPart).toBe('function')
    await expect(api.guitarPart(919, 2)).resolves.toEqual(resp)
    await api.guitarPart(920, 4)
    expect(calls).toEqual([[919, 2], [920, 4]])
  })

  it('манера не задана → 0 (авто): YueGuitarPart(id, 0)', async () => {
    const calls = []
    bind((...a) => { calls.push(a); return Promise.resolve({}) })
    await api.guitarPart(7)
    await api.guitarPart(8, undefined)
    await api.guitarPart(9, null)
    expect(calls).toEqual([[7, 0], [8, 0], [9, 0]])
  })

  it('ошибка биндинга (воркер: «сделайте транскрипцию») — отклонённый промис с тем же текстом', async () => {
    const msg = 'у трека нет плана (score.abc) — сделайте транскрипцию'
    bind(() => Promise.reject(new Error(msg)))
    await expect(api.guitarPart(1, 0)).rejects.toThrow(msg)
  })

  it('распознавание нот удалено: api.guitarNotes нет', () => {
    expect(api.guitarNotes).toBeUndefined()
  })
})
