// Авто-разделение на дорожки и «убрать дорожку» вместо «минуса».
// Спецификация: STEMS_AUTO = ['studio', 'always', 'manual']; shouldAutoSplit(loaded, stems, mode) —
// делить, только когда список дорожек прочитан, дорожек нет и режим не 'manual' (неизвестный
// режим старого воркера — делить); removeRecord(stem, sel) — запись громкости −100 дБ на дорожку
// в окне выделения или на весь трек; insertTitle для 'remove' — «убрать · <дорожка>»;
// addMutes сохраняет step записи; ключи i18n новые есть, ключей «минуса» нет; makeMinus из api.js убран.
import { readFileSync } from 'node:fs'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { STEMS_AUTO, shouldAutoSplit, removeRecord } from './trackDesk.js'
import { insertTitle } from './insertLabels.js'
import RU from './i18n/ru.js'
import EN from './i18n/en.js'

// Мок внешней границы — API воркера (как в useInserts*.test.js). Объект общий, поля — в load().
const apiMock = vi.hoisted(() => ({}))
vi.mock('./api.js', () => ({ api: apiMock }))

const srcDir = dirname(fileURLToPath(import.meta.url))

describe('STEMS_AUTO — режимы авто-разделения', () => {
  it('ровно studio, always, manual', () => {
    expect(STEMS_AUTO).toEqual(['studio', 'always', 'manual'])
  })
})

describe('shouldAutoSplit', () => {
  it('список прочитан, дорожек нет, режим studio/always → делить', () => {
    for (const mode of ['studio', 'always']) {
      expect(shouldAutoSplit(true, [], mode), mode).toBe(true)
    }
  })

  it('дорожек нет в любом виде: [], null, undefined → делить', () => {
    for (const stems of [[], null, undefined]) {
      expect(shouldAutoSplit(true, stems, 'studio'), String(stems)).toBe(true)
    }
  })

  it('старый воркер без настройки (undefined) или неизвестный режим → делить', () => {
    expect(shouldAutoSplit(true, [], undefined)).toBe(true)
    expect(shouldAutoSplit(true, [], 'zzz')).toBe(true)
  })

  it('режим manual → не делить', () => {
    expect(shouldAutoSplit(true, [], 'manual')).toBe(false)
    expect(shouldAutoSplit(true, null, 'manual')).toBe(false)
  })

  it('список дорожек ещё не прочитан → не делить', () => {
    for (const mode of ['studio', 'always', undefined]) {
      expect(shouldAutoSplit(false, [], mode), String(mode)).toBe(false)
      expect(shouldAutoSplit(false, null, mode), String(mode)).toBe(false)
    }
  })

  it('дорожки уже есть → не делить', () => {
    for (const mode of ['studio', 'always', undefined]) {
      expect(shouldAutoSplit(true, ['vocals', 'drums', 'bass', 'other'], mode), String(mode)).toBe(false)
      expect(shouldAutoSplit(true, ['vocals'], mode), String(mode)).toBe(false)
    }
  })
})

describe('removeRecord — «убрать дорожку»', () => {
  const base = { instId: 'remove', db: -100, step: 'sound' }

  it('с выделением — окно выделения', () => {
    expect(removeRecord('drums', { from: 12, to: 20.5 })).toEqual({ ...base, from: 12, to: 20.5, stems: ['drums'] })
  })

  it('без выделения (null/undefined) — весь трек 0…0', () => {
    expect(removeRecord('vocals', null)).toEqual({ ...base, from: 0, to: 0, stems: ['vocals'] })
    expect(removeRecord('vocals', undefined)).toEqual({ ...base, from: 0, to: 0, stems: ['vocals'] })
  })

  it('нулевое выделение (to = from) — весь трек', () => {
    expect(removeRecord('bass', { from: 7, to: 7 })).toEqual({ ...base, from: 0, to: 0, stems: ['bass'] })
    expect(removeRecord('bass', { from: 0, to: 0 })).toEqual({ ...base, from: 0, to: 0, stems: ['bass'] })
  })

  it('обратное выделение (to < from) — весь трек', () => {
    expect(removeRecord('other', { from: 30, to: 10 })).toEqual({ ...base, from: 0, to: 0, stems: ['other'] })
  })

  it('дорожка — ровно одна, переданная', () => {
    expect(removeRecord('kick', { from: 1, to: 2 }).stems).toEqual(['kick'])
  })
})

describe('insertTitle — запись «убрать»', () => {
  const names = {
    chainName: (c) => 'цепочка ' + c,
    stemName: (s) => ({ drums: 'барабаны', vocals: 'голос' })[s] || s,
    instName: (id) => 'приём ' + id,
    placeName: (p) => 'место ' + p,
  }

  it('начинается с перевода insert.remove (ru: «убрать») и содержит « · барабаны»', () => {
    expect(RU['insert.remove']).toBe('убрать')
    const got = insertTitle({ childId: -3, ...removeRecord('drums', null) }, names)
    expect(got.startsWith('убрать')).toBe(true)
    expect(got).toContain(' · барабаны')
  })

  it('в окне выделения — та же подпись с дорожкой', () => {
    const got = insertTitle({ childId: -4, ...removeRecord('vocals', { from: 5, to: 9 }) }, names)
    expect(got.startsWith('убрать')).toBe(true)
    expect(got).toContain(' · голос')
    expect(got).not.toContain('приём remove')
  })

  it('обычная громкость дорожек — по-прежнему название приёма', () => {
    expect(insertTitle({ childId: -5, instId: 'drumsup', from: 0, to: 0, stems: ['drums'], db: -6 }, names)).toBe('приём drumsup')
  })
})

describe('addMutes — step записи в реестре', () => {
  class MemStorage {
    constructor() { this.m = new Map() }
    getItem(k) { return this.m.has(k) ? this.m.get(k) : null }
    setItem(k, v) { this.m.set(k, String(v)) }
    removeItem(k) { this.m.delete(k) }
    clear() { this.m.clear() }
  }

  async function flush() {
    for (let i = 0; i < 10; i++) await vi.advanceTimersByTimeAsync(0)
  }

  // модуль — синглтон: импорт заново после vi.resetModules() (как в useInserts.test.js)
  async function load() {
    vi.resetModules()
    globalThis.localStorage = new MemStorage()
    localStorage.setItem('yue_insert_queue', '[]')
    localStorage.setItem('yue_insert_applied', '{}')
    vi.useFakeTimers()
    apiMock.jobs = vi.fn(async () => [])
    apiMock.rebuildSections = vi.fn(async (_p, specs) => ({
      variant: { file: 'out.wav' },
      inserts: specs.map(s => ({ child_id: s.child_id, aligned: true, score: 0.9, start_sec: s.from, gain: 1 })),
    }))
    const mod = await import('./composables/useInserts.js')
    return mod.useInserts()
  }

  afterEach(() => {
    vi.clearAllTimers()
    vi.useRealTimers()
  })

  it('запись со step — step в реестре', async () => {
    const ins = await load()
    await ins.addMutes('P', [removeRecord('drums', { from: 2, to: 6 })])
    await flush()
    const reg = ins.appliedFor('P')
    expect(reg.length).toBe(1)
    expect(reg[0].step).toBe('sound')
    expect(reg[0]).toMatchObject({ instId: 'remove', from: 2, to: 6, stems: ['drums'], db: -100 })
  })

  it('запись без step — поля step в реестре нет', async () => {
    const ins = await load()
    await ins.addMutes('P', [{ instId: 'drumsolo', from: 0, to: 5, stems: ['other'], db: -100 }])
    await flush()
    const reg = ins.appliedFor('P')
    expect(reg.length).toBe(1)
    expect('step' in reg[0]).toBe(false)
  })

  it('в одном вызове со step и без — у каждой записи своё', async () => {
    const ins = await load()
    await ins.addMutes('P', [
      { instId: 'drumsolo', from: 0, to: 5, stems: ['other'], db: -100 },
      { instId: 'remove', from: 10, to: 20, stems: ['vocals'], db: -100, step: 'sound' },
    ])
    await flush()
    const byFrom = Object.fromEntries(ins.appliedFor('P').map(x => [x.from, x]))
    expect('step' in byFrom[0]).toBe(false)
    expect(byFrom[10].step).toBe('sound')
  })
})

describe('i18n и api.js — «минус» убран, «убрать дорожку» и авто-разделение есть', () => {
  const present = [
    'settings.stemsAuto', 'settings.stemsAuto.studio', 'settings.stemsAuto.always',
    'settings.stemsAuto.manual', 'settings.stemsAuto.hint',
    'desk.remove', 'desk.remove.tip', 'desk.split.busy', 'desk.split.fast', 'desk.split.slow',
    'insert.remove',
  ]
  const absent = ['studio.minus', 'studio.stems.minus', 'studio.minusLabel', 'studio.wave.minus']

  it('новые ключи есть в ru и en, непустые', () => {
    for (const [name, dict] of [['ru', RU], ['en', EN]]) {
      for (const k of present) {
        expect(typeof dict[k], `${name}: ${k}`).toBe('string')
        expect(dict[k].trim().length, `${name}: ${k}`).toBeGreaterThan(0)
      }
    }
  })

  it('desk.split.busy содержит {time} в ru и en', () => {
    expect(RU['desk.split.busy']).toContain('{time}')
    expect(EN['desk.split.busy']).toContain('{time}')
  })

  it('ключей «минуса» нет ни в ru, ни в en', () => {
    for (const [name, dict] of [['ru', RU], ['en', EN]]) {
      for (const k of absent) expect(k in dict, `${name}: ${k}`).toBe(false)
    }
  })

  it('в исходнике api.js нет makeMinus', () => {
    const src = readFileSync(join(srcDir, 'api.js'), 'utf8')
    expect(src).not.toContain('makeMinus')
  })
})
