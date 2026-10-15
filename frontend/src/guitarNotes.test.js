// Задача internal-guitar-notes: гитара переиграна другой гитарой по нотам распознавания (фронт).
// Спецификация (карточка, «Условия» 1, 8 и «Устройство (контракт)»: Фронт / Go / MCP, блок gtrnotes):
// - PRESET_GROUPS['guitar-notes'] — семейство guitar, подпись ru «Ноты переиграны другой гитарой»; подписи
//   прежних гитарных подвидов оканчиваются на «— звук, игра та же»;
// - fxPresets: guitar-notes-nylon/-acoustic/-emily/-fsbs/-fuzz — группа guitar-notes, stems ['guitar'], первый
//   блок gtrnotes с набором (нейлон nylonguitar/guitar, полуакустика shinyguitar/acoustic + глушение
//   shinyguitar/mute, Emily emilyguitar/notes + глушение emilyguitar/mute, FSBS fsbsguitar/guitar, фузз —
//   FSBS + перегруз drive); старые 16 гитарных звуков и их id не меняются;
// - fxBlocks.json: gtrnotes («Гитара по нотам»): строки kit (обяз.), mute_kit (по умолч. ""); числа strum
//   (0/1, 1, целое), spread_ms (2…25, 8), output_db (−24…12, 0); служебные поля ставит воркер — не параметры;
// - докачка наборов (ensureKits, kitChain) учитывает kit и mute_kit блока gtrnotes; gtrnotes сам играет ноты
//   фразы (NOTE_BLOCKS воркера) — без обработки (bypass) его наборы тоже нужны;
// - паритет: api.guitarNotes(jobId, force) → биндинг YueGuitarNotes.
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
const byId = (id) => fxPresets.find((p) => p.id === id)

describe('PRESET_GROUPS: разновидность «Ноты переиграны другой гитарой» (условие 1)', () => {
  it('guitar-notes — семейство guitar, ru «Ноты переиграны другой гитарой», en непустая', () => {
    const g = PRESET_GROUPS['guitar-notes']
    expect(g, 'нет группы guitar-notes').toBeTruthy()
    expect(g.family).toBe('guitar')
    expect(g.ru).toBe('Ноты переиграны другой гитарой')
    expect(typeof g.en).toBe('string')
    expect(g.en.trim()).not.toBe('')
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

describe('fxPresets: старые гитарные звуки не меняются (условие 1)', () => {
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

describe('fxPresets: пять звуков «по нотам» (условия 1, 2)', () => {
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

describe('fxBlocks.json: блок gtrnotes («Гитара по нотам»)', () => {
  const b = blocks.gtrnotes
  const str = (id) => (b?.strings || []).find((s) => s.id === id)
  const par = (id) => (b?.params || []).find((p) => p.id === id)

  it('блок описан, подписи ru «Гитара по нотам» и en непустая', () => {
    expect(b, 'нет блока gtrnotes').toBeTruthy()
    expect(b.label.ru).toBe('Гитара по нотам')
    expect(String(b.label.en || '').trim()).not.toBe('')
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

describe('api.guitarNotes → биндинг YueGuitarNotes (условие 8)', () => {
  const saved = globalThis.window
  afterEach(() => {
    globalThis.window = saved
  })

  it('передаёт jobId и force как есть и отдаёт ответ биндинга', async () => {
    const calls = []
    const resp = { notes: 431, engines: ['yourmt3', 'basic-pitch'], cached: false }
    globalThis.window = { go: { main: { App: { YueGuitarNotes: (...a) => { calls.push(a); return Promise.resolve(resp) } } } } }
    expect(typeof api.guitarNotes).toBe('function')
    await expect(api.guitarNotes(919, true)).resolves.toEqual(resp)
    await api.guitarNotes(920, false)
    expect(calls).toEqual([[919, true], [920, false]])
  })

  it('ошибка биндинга (воркер: «распознавание нот не установлено») — отклонённый промис с тем же текстом', async () => {
    const msg = 'распознавание нот не установлено: worker/notes_install.sh'
    globalThis.window = { go: { main: { App: { YueGuitarNotes: () => Promise.reject(new Error(msg)) } } } }
    await expect(api.guitarNotes(1, false)).rejects.toThrow(msg)
  })
})
